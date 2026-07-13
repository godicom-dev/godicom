package godicom

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/godicom-dev/godicom/uid"
)

var (
	sqDepth           int
	visitingSequences map[*Sequence]struct{}
)

func resetWriteGlobals() {
	sqDepth = 0
	visitingSequences = nil
}

// WriteOptions controls DICOM file writing behavior.
type WriteOptions struct {
	ImplicitVR        *bool
	LittleEndian      *bool
	EnforceFileFormat bool
}

type writeSource struct {
	dataset  *Dataset
	fileMeta *FileMetaDataset
	preamble []byte
}

// writeFile writes a Dataset to a DICOM file.
func writeFile(filename string, source writeSource, opts *WriteOptions) error {
	resetWriteGlobals()
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if opts == nil {
		opts = &WriteOptions{}
	}

	if source.dataset == nil {
		return fmt.Errorf("godicom: missing dataset")
	}

	for _, elem := range source.dataset.Iter() {
		switch elem.Tag.Group() {
		case 0x0000:
			return fmt.Errorf(
				"godicom: Command Set elements (0000,eeee) are not allowed when writing a file; write the dataset alone instead",
			)
		case 0x0002:
			return fmt.Errorf(
				"godicom: File Meta Information group elements (0002,eeee) must be in FileDataset.FileMeta, not the dataset",
			)
		}
	}

	fileMeta := source.fileMeta
	if fileMeta != nil {
		fileMeta = cloneFileMeta(fileMeta)
	}

	// Determine encoding (transfer syntax in file meta takes priority over originalEnc).
	isImplicit, isLittleEndian, encErr := determineWriteEncoding(fileMeta, source.dataset, opts)
	if encErr != nil {
		return encErr
	}

	if !opts.EnforceFileFormat && isImplicit && !isLittleEndian {
		return fmt.Errorf("godicom: implicit VR and big endian is not a valid encoding combination")
	}

	if opts.EnforceFileFormat {
		if fileMeta == nil {
			fileMeta = NewFileMetaDataset()
		}

		// Under EnforceFileFormat, fill TransferSyntaxUID from the resolved
		// encoding when missing. Unlike pydicom (which skips Explicit VR LE),
		// godicom also fills Explicit VR Little Endian because NewDataset
		// defaults to that encoding.
		ts, _ := transferSyntaxUID(fileMeta)
		if ts == "" {
			if isImplicit && isLittleEndian {
				fileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ImplicitVRLittleEndian))
			} else if !isImplicit && isLittleEndian {
				fileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ExplicitVRLittleEndian))
			} else if !isImplicit && !isLittleEndian {
				fileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ExplicitVRBigEndian))
			}
		}

		if sopClass, ok := source.dataset.GetString(MustTag("SOPClassUID")); ok {
			if metaClass, ok := fileMeta.GetString(MustTag("MediaStorageSOPClassUID")); !ok || metaClass == "" || metaClass != sopClass {
				fileMeta.Set(NewDataElement(MustTag("MediaStorageSOPClassUID"), VRUI, UID(sopClass)))
			}
		}
		if sopInstance, ok := source.dataset.GetString(MustTag("SOPInstanceUID")); ok {
			if metaInstance, ok := fileMeta.GetString(MustTag("MediaStorageSOPInstanceUID")); !ok || metaInstance == "" || metaInstance != sopInstance {
				fileMeta.Set(NewDataElement(MustTag("MediaStorageSOPInstanceUID"), VRUI, UID(sopInstance)))
			}
		}

		if err := ValidateFileMeta(fileMeta, true); err != nil {
			return err
		}
	}

	if ts, ok := transferSyntaxUID(fileMeta); ok {
		if UID(ts).IsCompressed() {
			if elem, ok := source.dataset.Get(MustTag("PixelData")); ok {
				elem.IsUndefinedLength = true
			}
		}
	}

	preamble := source.preamble
	writePreamble := len(preamble) > 0
	if opts.EnforceFileFormat && !writePreamble {
		preamble = make([]byte, 128)
		writePreamble = true
	}
	if writePreamble {
		if len(preamble) != 128 {
			return fmt.Errorf("godicom: preamble must be 128 bytes, got %d", len(preamble))
		}
		if _, err := f.Write(preamble); err != nil {
			return err
		}
		if _, err := f.Write([]byte("DICM")); err != nil {
			return err
		}
	}

	// Write File Meta Information (always Explicit VR Little Endian)
	fp := newDicomWriter(f)
	fp.SetByteOrder(true)

	if fileMeta != nil && fileMeta.Len() > 0 {
		if err := writeFileMetaInfo(fp, fileMeta, opts.EnforceFileFormat); err != nil {
			return fmt.Errorf("godicom: error writing file meta: %w", err)
		}
	}

	// Write dataset
	fp.SetByteOrder(isLittleEndian)

	tsUID := ""
	if fileMeta != nil {
		tsUID, _ = transferSyntaxUID(fileMeta)
	}

	if UID(tsUID).IsDeflated() {
		var datasetBuf bytes.Buffer
		dsWriter := newDicomWriter(&datasetBuf)
		dsWriter.SetByteOrder(isLittleEndian)
		if err := writeDataset(dsWriter, source.dataset, isImplicit, isLittleEndian, nil, encodingChanged(source.dataset, isImplicit, isLittleEndian)); err != nil {
			return fmt.Errorf("godicom: error writing dataset: %w", err)
		}
		var deflated bytes.Buffer
		fw, err := flate.NewWriter(&deflated, flate.DefaultCompression)
		if err != nil {
			return fmt.Errorf("godicom: error creating deflater: %w", err)
		}
		if _, err := fw.Write(datasetBuf.Bytes()); err != nil {
			return fmt.Errorf("godicom: error deflating dataset: %w", err)
		}
		if err := fw.Close(); err != nil {
			return fmt.Errorf("godicom: error closing deflater: %w", err)
		}
		payload := deflated.Bytes()
		if len(payload)%2 == 1 {
			payload = append(payload, 0)
		}
		if _, err := fp.Write(payload); err != nil {
			return fmt.Errorf("godicom: error writing deflated dataset: %w", err)
		}
		return nil
	}

	if err := writeDataset(fp, source.dataset, isImplicit, isLittleEndian, nil, encodingChanged(source.dataset, isImplicit, isLittleEndian)); err != nil {
		return fmt.Errorf("godicom: error writing dataset: %w", err)
	}

	return nil
}

func writeFileMetaInfo(fp *dicomIO, fileMeta *FileMetaDataset, enforceStandard bool) error {
	if fileMeta == nil {
		return nil
	}
	if err := ValidateFileMeta(fileMeta, false); err != nil {
		return err
	}

	if enforceStandard {
		if _, ok := fileMeta.Get(MustTag("FileMetaInformationGroupLength")); !ok {
			fileMeta.Set(NewDataElement(MustTag("FileMetaInformationGroupLength"), VRUL, uint32(0)))
		}
	}

	var buf bytes.Buffer
	metaWriter := newDicomWriter(&buf)
	metaWriter.SetByteOrder(true)
	for _, elem := range fileMeta.Iter() {
		if elem.Tag.Group() != 0x0002 {
			continue
		}
		if err := writeElement(metaWriter, elem, false, true, nil, false); err != nil {
			return err
		}
	}

	if enforceStandard {
		if elem, ok := fileMeta.Get(MustTag("FileMetaInformationGroupLength")); ok {
			elem.Value = uint32(buf.Len() - 12)
			elem.RawValue = nil
			buf.Reset()
			metaWriter = newDicomWriter(&buf)
			metaWriter.SetByteOrder(true)
			for _, e := range fileMeta.Iter() {
				if e.Tag.Group() != 0x0002 {
					continue
				}
				if err := writeElement(metaWriter, e, false, true, nil, false); err != nil {
					return err
				}
			}
		}
	}

	if _, err := fp.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

func transferSyntaxUID(fileMeta *FileMetaDataset) (string, bool) {
	if fileMeta == nil {
		return "", false
	}
	return fileMeta.GetString(MustTag("TransferSyntaxUID"))
}

// determineWriteEncoding selects implicit VR and endianness for writing.
// Mirrors pydicom.filewriter._determine_encoding (non-force path).
func determineWriteEncoding(fileMeta *FileMetaDataset, ds *Dataset, opts *WriteOptions) (isImplicit, isLittleEndian bool, err error) {
	if opts == nil {
		opts = &WriteOptions{}
	}

	var (
		hasFallback bool
		fallbackImp bool
		fallbackLit bool
	)
	if opts.ImplicitVR != nil {
		fallbackImp = *opts.ImplicitVR
		fallbackLit = true
		hasFallback = true
		if opts.LittleEndian != nil {
			fallbackLit = *opts.LittleEndian
		}
	} else if opts.LittleEndian != nil {
		fallbackImp = ds.originalEnc.IsImplicitVR
		fallbackLit = *opts.LittleEndian
		hasFallback = true
	} else {
		fallbackImp = ds.originalEnc.IsImplicitVR
		fallbackLit = ds.originalEnc.IsLittleEndian
		hasFallback = true
	}

	tsUID, hasTS := transferSyntaxUID(fileMeta)
	if !hasTS || tsUID == "" {
		if !hasFallback {
			return false, false, fmt.Errorf(
				"godicom: unable to determine the encoding to use for writing the dataset; set FileMeta TransferSyntaxUID or WriteOptions ImplicitVR/LittleEndian",
			)
		}
		return fallbackImp, fallbackLit, nil
	}

	ts := uid.UID(tsUID)
	info, known := uid.Known[ts]
	if known && info.IsTransferSyntax {
		if opts.ImplicitVR != nil && *opts.ImplicitVR != info.IsImplicitVR {
			return false, false, fmt.Errorf(
				"godicom: ImplicitVR=%t is inconsistent with transfer syntax %q",
				*opts.ImplicitVR, tsUID,
			)
		}
		if opts.LittleEndian != nil && *opts.LittleEndian != info.IsLittleEndian {
			return false, false, fmt.Errorf(
				"godicom: LittleEndian=%t is inconsistent with transfer syntax %q",
				*opts.LittleEndian, tsUID,
			)
		}
		return info.IsImplicitVR, info.IsLittleEndian, nil
	}

	if known && !info.IsTransferSyntax {
		return false, false, fmt.Errorf(
			"godicom: Transfer Syntax UID %q is not a valid transfer syntax",
			tsUID,
		)
	}

	// Private / unknown UID: require both encoding options.
	if opts.ImplicitVR == nil || opts.LittleEndian == nil {
		return false, false, fmt.Errorf(
			"godicom: ImplicitVR and LittleEndian are required when using a private transfer syntax",
		)
	}
	return *opts.ImplicitVR, *opts.LittleEndian, nil
}

func encodingChanged(ds *Dataset, isImplicit, isLittleEndian bool) bool {
	return isImplicit != ds.originalEnc.IsImplicitVR || isLittleEndian != ds.originalEnc.IsLittleEndian
}

func writeDataset(fp *dicomIO, ds *Dataset, isImplicit, isLittleEndian bool, charsets []string, reencodeValues bool) error {
	if len(charsets) == 0 {
		charsets = []string{DefaultCharacterSet}
	}
	localCharsets := append([]string(nil), charsets...)

	encodingChanged := reencodeValues
	if !isImplicit || encodingChanged {
		if err := CorrectAmbiguousVR(ds, isLittleEndian, nil); err != nil {
			return err
		}
	} else if err := CorrectAmbiguousVRPreservingRaw(ds, isLittleEndian, nil); err != nil {
		return err
	}

	for _, elem := range ds.Iter() {
		if elem.Tag.Group() == 0x0002 {
			continue // Already written as file meta
		}
		// Do not write retired Group Length (see PS3.5, 7.2)
		if elem.Tag.Element() == 0 && elem.Tag.Group() > 6 {
			continue
		}
		if err := writeElement(fp, elem, isImplicit, isLittleEndian, localCharsets, encodingChanged); err != nil {
			return err
		}
		if elem.Tag == TagCharset {
			localCharsets = ParseCharacterSets(elem.Value)
		}
	}
	return nil
}

func writeElementFromRaw(fp *dicomIO, elem *DataElement, isImplicit, isLittleEndian bool) error {
	if !isImplicit && IsAmbiguousVR(elem.VR) {
		return fmt.Errorf(
			"godicom: cannot write ambiguous VR %q for tag %s; set the correct VR or use implicit VR transfer syntax",
			elem.VR, elem.Tag,
		)
	}
	if err := fp.WriteTag(elem.Tag); err != nil {
		return err
	}

	valueLength := uint32(len(elem.RawValue))
	isUndefinedLength := elem.IsUndefinedLength

	if isImplicit {
		length := valueLength
		if isUndefinedLength {
			length = 0xFFFFFFFF
		}
		if err := fp.WriteUint32(length); err != nil {
			return err
		}
	} else {
		if _, err := fp.Write([]byte(string(elem.VR))); err != nil {
			return err
		}
		if isUndefinedLength {
			if !ExplicitVRLength16[elem.VR] {
				if _, err := fp.Write([]byte{0, 0}); err != nil {
					return err
				}
			}
			if _, err := fp.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF}); err != nil {
				return err
			}
		} else if ExplicitVRLength16[elem.VR] {
			if valueLength > 0xFFFF {
				return fmt.Errorf("godicom: value too long for VR %s with 16-bit length: %d", elem.VR, valueLength)
			}
			if err := fp.WriteUint16(uint16(valueLength)); err != nil {
				return err
			}
		} else {
			if _, err := fp.Write([]byte{0, 0}); err != nil {
				return err
			}
			if err := fp.WriteUint32(valueLength); err != nil {
				return err
			}
		}
	}

	if len(elem.RawValue) > 0 {
		if _, err := fp.Write(elem.RawValue); err != nil {
			return err
		}
	}

	if isUndefinedLength {
		if err := fp.WriteTag(SequenceDelimiterTag); err != nil {
			return err
		}
		if err := fp.WriteUint32(0); err != nil {
			return err
		}
	}

	return nil
}

func writeElement(fp *dicomIO, elem *DataElement, isImplicit, isLittleEndian bool, charsets []string, reencodeValues bool) error {
	if elem.RawValue != nil && elem.VR != VRSQ && !reencodeValues {
		return writeElementFromRaw(fp, elem, isImplicit, isLittleEndian)
	}

	if !StandardVRs[elem.VR] && !IsAmbiguousVR(elem.VR) {
		return fmt.Errorf("godicom: unknown Value Representation %q", elem.VR)
	}

	if !isImplicit && IsAmbiguousVR(elem.VR) {
		return fmt.Errorf(
			"godicom: cannot write ambiguous VR %q for tag %s; set the correct VR or use implicit VR transfer syntax",
			elem.VR, elem.Tag,
		)
	}

	fp.SetByteOrder(isLittleEndian)

	isSQ := elem.VR == VRSQ
	var seq *Sequence
	if isSQ {
		seq, _ = elem.Value.(*Sequence)
	}
	undefinedSQ := elem.IsUndefinedLength
	if isSQ && !undefinedSQ && seq != nil {
		undefinedSQ = seq.IsUndefinedLength
	}

	isCircular := false
	if isSQ && seq != nil {
		if visitingSequences != nil {
			_, isCircular = visitingSequences[seq]
		}
	}
	enteredSequence := false
	if isSQ && seq != nil && !isCircular {
		if visitingSequences == nil {
			visitingSequences = make(map[*Sequence]struct{})
		}
		visitingSequences[seq] = struct{}{}
		enteredSequence = true
	}
	if enteredSequence {
		defer delete(visitingSequences, seq)
	}

	// For defined-length SQs with items (and not circular), pre-compute content
	var sqBuf *bytes.Buffer
	if isSQ && !isCircular && !undefinedSQ {
		seq, ok := elem.Value.(*Sequence)
		if ok && seq != nil && !seq.IsEmpty() {
			sqBuf = new(bytes.Buffer)
			sqFp := newDicomWriter(sqBuf)
			sqFp.SetByteOrder(isLittleEndian)

			for _, item := range seq.Items() {
				if err := sqFp.WriteTag(ItemTag); err != nil {
					return err
				}
				var itemBuf bytes.Buffer
				itemFp := newDicomWriter(&itemBuf)
				itemFp.SetByteOrder(isLittleEndian)
				if err := writeDataset(itemFp, item, isImplicit, isLittleEndian, charsets, reencodeValues); err != nil {
					return err
				}
				if err := sqFp.WriteUint32(uint32(itemBuf.Len())); err != nil {
					return err
				}
				if _, err := sqFp.Write(itemBuf.Bytes()); err != nil {
					return err
				}
			}
		}
	}

	// Write tag
	if err := fp.WriteTag(elem.Tag); err != nil {
		return err
	}

	// Get encoded value (nil for SQ)
	encoded := encodeValue(elem, isLittleEndian, charsets)

	// Pad to even length per PS3.5
	encoded = padToEven(elem.VR, encoded)

	// Write VR + length
	if isImplicit {
		length := uint32(len(encoded))
		if sqBuf != nil {
			length = uint32(sqBuf.Len())
		} else if isSQ && undefinedSQ {
			length = 0xFFFFFFFF
		}
		if err := fp.WriteUint32(length); err != nil {
			return err
		}
	} else {
		if _, err := fp.Write([]byte(string(elem.VR))); err != nil {
			return err
		}

		length := uint32(len(encoded))
		if sqBuf != nil {
			length = uint32(sqBuf.Len())
		} else if isSQ && undefinedSQ {
			length = 0xFFFFFFFF
		}

		if ExplicitVRLength16[elem.VR] {
			if length > 0xFFFF {
				return fmt.Errorf("godicom: value too long for VR %s with 16-bit length: %d", elem.VR, length)
			}
			if err := fp.WriteUint16(uint16(length)); err != nil {
				return err
			}
		} else {
			if _, err := fp.Write([]byte{0, 0}); err != nil {
				return err
			}
			if err := fp.WriteUint32(length); err != nil {
				return err
			}
		}
	}

	// Write value content
	if sqBuf != nil {
		if _, err := fp.Write(sqBuf.Bytes()); err != nil {
			return err
		}
	} else if len(encoded) > 0 {
		if _, err := fp.Write(encoded); err != nil {
			return err
		}
	}

	// For undefined-length SQs, write items + delimiters
	if isSQ && sqBuf == nil {
		sqDepth++
		if !isCircular && sqDepth <= 100 {
			if seq, ok := elem.Value.(*Sequence); ok && seq != nil && !seq.IsEmpty() {
				for _, item := range seq.Items() {
					var itemBuf bytes.Buffer
					itemFp := newDicomWriter(&itemBuf)
					itemFp.SetByteOrder(isLittleEndian)
					if err := writeDataset(itemFp, item, isImplicit, isLittleEndian, charsets, reencodeValues); err != nil {
						return err
					}
					if err := fp.WriteTag(ItemTag); err != nil {
						return err
					}
					if item.IsUndefinedLengthSequenceItem {
						if err := fp.WriteUint32(0xFFFFFFFF); err != nil {
							return err
						}
						if _, err := fp.Write(itemBuf.Bytes()); err != nil {
							return err
						}
						if err := fp.WriteTag(ItemDelimiterTag); err != nil {
							return err
						}
						if err := fp.WriteUint32(0); err != nil {
							return err
						}
					} else {
						if err := fp.WriteUint32(uint32(itemBuf.Len())); err != nil {
							return err
						}
						if _, err := fp.Write(itemBuf.Bytes()); err != nil {
							return err
						}
					}
				}
			}
		}
		sqDepth--
		if err := fp.WriteTag(SequenceDelimiterTag); err != nil {
			return err
		}
		if err := fp.WriteUint32(0); err != nil {
			return err
		}
	}

	return nil
}

func padToEven(vr VR, encoded []byte) []byte {
	if len(encoded)%2 == 0 {
		return encoded
	}
	var padByte byte
	if vr == VRUI || BytesVR[vr] {
		padByte = 0x00
	} else {
		padByte = 0x20
	}
	return append(encoded, padByte)
}

func encodeValue(elem *DataElement, le bool, charsets []string) []byte {
	if elem.Value == nil {
		return nil
	}

	switch elem.VR {
	case VRAE, VRAS, VRCS, VRDA, VRDT, VRLO, VRLT, VRSH, VRST, VRTM, VRUC, VRUR, VRUT:
		return encodeStringWithCharsets(elem, charsets)
	case VRDS:
		return encodeNumberString(elem)
	case VRIS:
		return encodeNumberString(elem)
	case VRUI:
		return encodeStringWithCharsets(elem, charsets)
	case VRPN:
		return encodePNWithCharsets(elem, charsets)
	case VRFD:
		return encodeFloats(elem, le, 8)
	case VRFL:
		return encodeFloats(elem, le, 4)
	case VRUL:
		return encodeInts(elem, le, 4, false)
	case VRUS:
		return encodeInts(elem, le, 2, false)
	case VRUV:
		return encodeInts(elem, le, 8, false)
	case VRSL:
		return encodeInts(elem, le, 4, true)
	case VRSS:
		return encodeInts(elem, le, 2, true)
	case VRSV:
		return encodeInts(elem, le, 8, true)
	case VRAT:
		return encodeAT(elem, le)
	case VROB, VROD, VROF, VROL, VROW, VROV, VRUN:
		return encodeBytes(elem)
	case VRSQ:
		return nil // Handled separately
	default:
		return encodeStringWithCharsets(elem, charsets)
	}
}

func encodeStringWithCharsets(elem *DataElement, charsets []string) []byte {
	if elem.Value == nil {
		return nil
	}
	useCharsets := vrUsesCharacterSet(elem.VR) && needsCharsetEncode(charsets)

	switch v := elem.Value.(type) {
	case string:
		if useCharsets {
			return EncodeBytesWithCharsets(v, charsets)
		}
		return []byte(v)
	case []byte:
		return v
	case []string:
		if useCharsets {
			parts := make([][]byte, len(v))
			for i, part := range v {
				parts[i] = EncodeBytesWithCharsets(part, charsets)
			}
			return bytes.Join(parts, []byte{'\\'})
		}
		return []byte(strings.Join(v, "\\"))
	case UID:
		return []byte(string(v))
	case PersonName:
		if useCharsets {
			return EncodePersonNameWithCharsets(v, charsets)
		}
		return []byte(v.String())
	case DA:
		return []byte(v.String())
	case TM:
		return []byte(v.String())
	case DT:
		return []byte(v.String())
	case DS:
		return []byte(v.String())
	case IS:
		return []byte(v.String())
	case []DA:
		return []byte(joinStringParts(len(v), func(i int) string { return v[i].String() }))
	case []TM:
		return []byte(joinStringParts(len(v), func(i int) string { return v[i].String() }))
	case []DT:
		return []byte(joinStringParts(len(v), func(i int) string { return v[i].String() }))
	case *MultiValue[DA]:
		return []byte(joinStringParts(v.Len(), func(i int) string { return v.Values()[i].String() }))
	case *MultiValue[TM]:
		return []byte(joinStringParts(v.Len(), func(i int) string { return v.Values()[i].String() }))
	case *MultiValue[DT]:
		return []byte(joinStringParts(v.Len(), func(i int) string { return v.Values()[i].String() }))
	case *MultiValue[string]:
		if useCharsets {
			parts := make([][]byte, v.Len())
			for i, part := range v.Values() {
				parts[i] = EncodeBytesWithCharsets(part, charsets)
			}
			return bytes.Join(parts, []byte{'\\'})
		}
		return []byte(joinStringParts(v.Len(), func(i int) string { return v.Values()[i] }))
	case *MultiValue[PersonName]:
		if useCharsets {
			parts := make([][]byte, v.Len())
			for i, pn := range v.Values() {
				parts[i] = EncodePersonNameWithCharsets(pn, charsets)
			}
			return bytes.Join(parts, []byte{'\\'})
		}
		return []byte(joinStringParts(v.Len(), func(i int) string { return v.Values()[i].String() }))
	case fmt.Stringer:
		if useCharsets {
			return EncodeBytesWithCharsets(v.String(), charsets)
		}
		return []byte(v.String())
	}
	return []byte(fmt.Sprintf("%v", elem.Value))
}

func joinStringParts(n int, part func(int) string) string {
	if n == 0 {
		return ""
	}
	s := part(0)
	for i := 1; i < n; i++ {
		s += "\\" + part(i)
	}
	return s
}

func encodeNumberString(elem *DataElement) []byte {
	if elem.Value == nil {
		return nil
	}
	switch v := elem.Value.(type) {
	case string:
		return []byte(v)
	case int:
		return []byte(fmt.Sprintf("%d", v))
	case float64:
		return []byte(fmt.Sprintf("%g", v))
	case DS:
		return []byte(v.String())
	case IS:
		return []byte(v.String())
	case *MultiValue[int]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += fmt.Sprintf("%d", val)
		}
		return []byte(s)
	case *MultiValue[float64]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += fmt.Sprintf("%g", val)
		}
		return []byte(s)
	case *MultiValue[DS]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += val.String()
		}
		return []byte(s)
	case *MultiValue[IS]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += val.String()
		}
		return []byte(s)
	case *MultiValue[interface{}]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += fmt.Sprintf("%v", val)
		}
		return []byte(s)
	}
	return []byte(fmt.Sprintf("%v", elem.Value))
}

func encodePNWithCharsets(elem *DataElement, charsets []string) []byte {
	if elem.Value == nil {
		return nil
	}
	useCharsets := needsCharsetEncode(charsets)
	switch v := elem.Value.(type) {
	case PersonName:
		if useCharsets {
			return EncodePersonNameWithCharsets(v, charsets)
		}
		return []byte(v.String())
	case string:
		if useCharsets {
			return EncodeBytesWithCharsets(v, charsets)
		}
		return []byte(v)
	case *MultiValue[PersonName]:
		if useCharsets {
			parts := make([][]byte, v.Len())
			for i, pn := range v.Values() {
				parts[i] = EncodePersonNameWithCharsets(pn, charsets)
			}
			return bytes.Join(parts, []byte{'\\'})
		}
		return []byte(joinStringParts(v.Len(), func(i int) string { return v.Values()[i].String() }))
	}
	return []byte(fmt.Sprintf("%v", elem.Value))
}

func encodeFloats(elem *DataElement, le bool, size int) []byte {
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}

	var floats []float64
	switch v := elem.Value.(type) {
	case float64:
		floats = []float64{v}
	case float32:
		floats = []float64{float64(v)}
	case int:
		floats = []float64{float64(v)}
	case *MultiValue[float64]:
		floats = v.Values()
	case *MultiValue[interface{}]:
		for _, item := range v.Values() {
			switch x := item.(type) {
			case float64:
				floats = append(floats, x)
			case float32:
				floats = append(floats, float64(x))
			case int:
				floats = append(floats, float64(x))
			}
		}
	default:
		return nil
	}

	buf := make([]byte, len(floats)*size)
	for i, f := range floats {
		if size == 4 {
			order.PutUint32(buf[i*4:], math.Float32bits(float32(f)))
		} else {
			order.PutUint64(buf[i*8:], math.Float64bits(f))
		}
	}
	return buf
}

func encodeInts(elem *DataElement, le bool, size int, signed bool) []byte {
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}

	var ints []uint64
	switch v := elem.Value.(type) {
	case int:
		ints = []uint64{uint64(v)}
	case uint16:
		ints = []uint64{uint64(v)}
	case uint32:
		ints = []uint64{uint64(v)}
	case int32:
		ints = []uint64{uint64(v)}
	case int64:
		ints = []uint64{uint64(v)}
	case uint64:
		ints = []uint64{v}
	case *MultiValue[int]:
		for _, x := range v.Values() {
			ints = append(ints, uint64(x))
		}
	case []int:
		for _, x := range v {
			ints = append(ints, uint64(x))
		}
	case []int64:
		for _, x := range v {
			ints = append(ints, uint64(x))
		}
	case *MultiValue[int64]:
		for _, x := range v.Values() {
			ints = append(ints, uint64(x))
		}
	case *MultiValue[uint64]:
		ints = v.Values()
	case *MultiValue[interface{}]:
		for _, item := range v.Values() {
			switch x := item.(type) {
			case int:
				ints = append(ints, uint64(x))
			case int64:
				ints = append(ints, uint64(x))
			case uint64:
				ints = append(ints, x)
			}
		}
	default:
		return nil
	}

	buf := make([]byte, len(ints)*size)
	for i, v := range ints {
		switch size {
		case 2:
			order.PutUint16(buf[i*2:], uint16(v))
		case 4:
			order.PutUint32(buf[i*4:], uint32(v))
		case 8:
			order.PutUint64(buf[i*8:], v)
		}
	}
	return buf
}

func encodeAT(elem *DataElement, le bool) []byte {
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}

	var tags []Tag
	switch v := elem.Value.(type) {
	case Tag:
		tags = []Tag{v}
	case int:
		tags = []Tag{Tag(v)}
	case *MultiValue[Tag]:
		tags = v.Values()
	case *MultiValue[interface{}]:
		for _, item := range v.Values() {
			switch x := item.(type) {
			case Tag:
				tags = append(tags, x)
			case int:
				tags = append(tags, Tag(x))
			}
		}
	default:
		return nil
	}

	buf := make([]byte, len(tags)*4)
	for i, t := range tags {
		order.PutUint32(buf[i*4:], uint32(t))
	}
	return buf
}

func encodeBytes(elem *DataElement) []byte {
	if elem.Value == nil {
		return nil
	}
	switch v := elem.Value.(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	}
	return nil
}

// WriteFile writes a Dataset to a DICOM file.
func WriteFile(filename string, ds *Dataset, opts *WriteOptions) error {
	return writeFile(filename, writeSource{dataset: ds}, opts)
}

// Ensure binary is used
var _ = binary.BigEndian
var _ = io.Discard
