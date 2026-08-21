package godicom

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/godicom-dev/godicom/uid"
)

type writeState struct {
	sqDepth           int
	visitingSequences map[*Sequence]struct{}
	ctx               context.Context

	// onDiag and seqPath are the write side of readContext's diagnostic
	// bookkeeping: the hook to offer a rejected value to, and the sequences
	// enclosing whatever is being written, so a Diagnostic can name its Path.
	onDiag  func(Diagnostic) error
	seqPath []Tag
}

func newWriteState() *writeState {
	return &writeState{visitingSequences: make(map[*Sequence]struct{}), ctx: context.Background()}
}

func newWriteStateCtx(ctx context.Context) *writeState {
	if ctx == nil {
		ctx = context.Background()
	}
	return &writeState{visitingSequences: make(map[*Sequence]struct{}), ctx: ctx}
}

// WriteOptions controls DICOM file writing behavior.
type WriteOptions struct {
	ImplicitVR        *bool
	LittleEndian      *bool
	EnforceFileFormat bool
	// Logger overrides the call-scoped slog logger for this write.
	// When nil, LoggerFromContext / DefaultLogger is used.
	Logger *slog.Logger
	// OnDiagnostic observes values the writer would otherwise encode silently
	// even though godicom's own reader would raise a diagnostic on the result --
	// a fractional float in an IS, a DS longer than the 16 bytes PS3.5 allows.
	// Returning nil keeps the old behaviour (write the value as it stands);
	// returning a non-nil error fails the write with it, so
	//
	//	OnDiagnostic: func(d Diagnostic) error { return d }
	//
	// refuses to produce a file a strict receiver would reject. It mirrors
	// ReadOptions.OnDiagnostic and receives the same Diagnostic type, with Kind
	// DiagnosticInvalidValue and no Offset -- nothing has been read.
	//
	// Values written straight from the bytes they were read as are not offered:
	// re-encoding is skipped for them, and the read that produced them already
	// had its own chance to report. Only write paths that take a *WriteOptions
	// can carry a hook, so EncodeDataset and WriteDataset never call one.
	OnDiagnostic func(Diagnostic) error
}

type writeSource struct {
	dataset  *Dataset
	fileMeta *FileMetaDataset
	preamble []byte
}

// writeFile writes a Dataset to a DICOM file.
func writeFile(ctx context.Context, filename string, source writeSource, opts *WriteOptions) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeTo(ctx, f, source, opts)
}

// writeTo encodes a Part 10 DICOM file to w.
func writeTo(ctx context.Context, w io.Writer, source writeSource, opts *WriteOptions) error {
	if opts == nil {
		opts = &WriteOptions{}
	}
	ctx = loggerContext(ctx, opts.Logger, ComponentWriter)

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
	enc, encErr := determineWriteEncoding(fileMeta, source.dataset, opts)
	if encErr != nil {
		return encErr
	}
	if ts, ok := transferSyntaxUID(fileMeta); ok && ts != "" {
		logDebug(ctx, "write encoding", AttrTransferSyntax, string(ts))
	} else {
		logDebug(ctx, "write encoding",
			"implicit_vr", enc.IsImplicitVR,
			"little_endian", enc.IsLittleEndian,
		)
	}

	if !opts.EnforceFileFormat && enc.IsImplicitVR && !enc.IsLittleEndian {
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
			if enc.IsImplicitVR && enc.IsLittleEndian {
				fileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ImplicitVRLittleEndian))
			} else if !enc.IsImplicitVR && enc.IsLittleEndian {
				fileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ExplicitVRLittleEndian))
			} else if !enc.IsImplicitVR && !enc.IsLittleEndian {
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
		if ts.IsCompressed() {
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
		if _, err := w.Write(preamble); err != nil {
			return err
		}
		if _, err := w.Write([]byte("DICM")); err != nil {
			return err
		}
	}

	// Write File Meta Information (always Explicit VR Little Endian)
	fp := newDicomWriter(w)
	fp.SetByteOrder(true)

	if fileMeta != nil && fileMeta.Len() > 0 {
		if err := writeFileMetaInfo(fp, fileMeta, opts.EnforceFileFormat); err != nil {
			return fmt.Errorf("godicom: error writing file meta: %w", err)
		}
	}

	// Write dataset
	fp.SetByteOrder(enc.IsLittleEndian)

	tsUID := UID("")
	if fileMeta != nil {
		tsUID, _ = transferSyntaxUID(fileMeta)
	}

	cc := codecContext{EncodingInfo: enc}
	reencode := encodingChanged(source.dataset, enc)

	// The write state, not codecContext, carries the diagnostic hook: it is not
	// encoding state, and writeState already exists to hold what spans one write
	// the way readContext does for one parse.
	st := newWriteStateCtx(ctx)
	st.onDiag = opts.OnDiagnostic

	if tsUID.IsDeflated() {
		var datasetBuf bytes.Buffer
		dsWriter := newDicomWriter(&datasetBuf)
		dsWriter.SetByteOrder(enc.IsLittleEndian)
		if err := writeDatasetState(dsWriter, source.dataset, cc, reencode, st); err != nil {
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

	if err := writeDatasetState(fp, source.dataset, cc, reencode, st); err != nil {
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
		if err := writeElement(metaWriter, elem, fileMetaCodec, false); err != nil {
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
				if err := writeElement(metaWriter, e, fileMetaCodec, false); err != nil {
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

func transferSyntaxUID(fileMeta *FileMetaDataset) (UID, bool) {
	if fileMeta == nil {
		return "", false
	}
	s, ok := fileMeta.GetString(MustTag("TransferSyntaxUID"))
	return UID(s), ok
}

// determineWriteEncoding selects implicit VR and endianness for writing.
// Mirrors pydicom.filewriter._determine_encoding (non-force path).
func determineWriteEncoding(fileMeta *FileMetaDataset, ds *Dataset, opts *WriteOptions) (EncodingInfo, error) {
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

	ts, hasTS := transferSyntaxUID(fileMeta)
	if !hasTS || ts == "" {
		if !hasFallback {
			return EncodingInfo{}, fmt.Errorf(
				"godicom: unable to determine the encoding to use for writing the dataset; set FileMeta TransferSyntaxUID or WriteOptions ImplicitVR/LittleEndian",
			)
		}
		return EncodingInfo{IsImplicitVR: fallbackImp, IsLittleEndian: fallbackLit}, nil
	}

	info, known := uid.Known[ts]
	if known && info.IsTransferSyntax {
		if opts.ImplicitVR != nil && *opts.ImplicitVR != info.IsImplicitVR {
			return EncodingInfo{}, fmt.Errorf(
				"godicom: ImplicitVR=%t is inconsistent with transfer syntax %q",
				*opts.ImplicitVR, ts,
			)
		}
		if opts.LittleEndian != nil && *opts.LittleEndian != info.IsLittleEndian {
			return EncodingInfo{}, fmt.Errorf(
				"godicom: LittleEndian=%t is inconsistent with transfer syntax %q",
				*opts.LittleEndian, ts,
			)
		}
		return EncodingInfo{IsImplicitVR: info.IsImplicitVR, IsLittleEndian: info.IsLittleEndian}, nil
	}

	if known && !info.IsTransferSyntax {
		return EncodingInfo{}, fmt.Errorf(
			"godicom: Transfer Syntax UID %q is not a valid transfer syntax",
			ts,
		)
	}

	// Private / unknown UID: require both encoding options.
	if opts.ImplicitVR == nil || opts.LittleEndian == nil {
		return EncodingInfo{}, fmt.Errorf(
			"godicom: ImplicitVR and LittleEndian are required when using a private transfer syntax",
		)
	}
	return EncodingInfo{IsImplicitVR: *opts.ImplicitVR, IsLittleEndian: *opts.LittleEndian}, nil
}

func encodingChanged(ds *Dataset, enc EncodingInfo) bool {
	return enc != ds.originalEnc
}

// charsetChanged reports whether SpecificCharacterSet differs from the value
// captured when the dataset was read. Changing the character set requires
// re-encoding text/PN values from decoded Unicode (pydicom test_changed_character_set).
func charsetChanged(ds *Dataset) bool {
	if ds == nil || ds.originalCharsets == nil {
		return false
	}
	current := ConvertCharacterSets(datasetCharacterSets(ds))
	original := ConvertCharacterSets(ds.originalCharsets)
	if len(current) != len(original) {
		return true
	}
	for i := range current {
		if current[i] != original[i] {
			return true
		}
	}
	return false
}

// writeDataset writes ds with a fresh write state, and so with no diagnostic
// hook. Callers that have a *WriteOptions build the state themselves and use
// writeDatasetState, so their hook survives the recursion into sequence items.
func writeDataset(ctx context.Context, fp *dicomIO, ds *Dataset, cc codecContext, reencodeValues bool) error {
	return writeDatasetState(fp, ds, cc, reencodeValues, newWriteStateCtx(ctx))
}

func writeDatasetState(fp *dicomIO, ds *Dataset, cc codecContext, reencodeValues bool, st *writeState) error {
	if st == nil {
		st = newWriteState()
	}
	if len(cc.Charsets) == 0 {
		cc.Charsets = []string{DefaultCharacterSet}
	}
	cc.Charsets = append([]string(nil), cc.Charsets...)

	reencode := reencodeValues || charsetChanged(ds)
	if !cc.IsImplicitVR || reencode {
		if err := CorrectAmbiguousVR(ds, cc.IsLittleEndian, nil); err != nil {
			return err
		}
	} else if err := CorrectAmbiguousVRPreservingRaw(ds, cc.IsLittleEndian, nil); err != nil {
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
		if err := writeElementState(fp, elem, cc, reencode, st); err != nil {
			return err
		}
		if elem.Tag == TagCharset {
			cc = cc.withCharsets(ParseCharacterSets(elem.Value))
		}
	}
	return nil
}

func writeElementFromRaw(fp *dicomIO, elem *DataElement, cc codecContext) error {
	if !cc.IsImplicitVR && IsAmbiguousVR(elem.VR) {
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

	if cc.IsImplicitVR {
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

func writeElement(fp *dicomIO, elem *DataElement, cc codecContext, reencodeValues bool) error {
	return writeElementState(fp, elem, cc, reencodeValues, newWriteState())
}

func writeElementState(fp *dicomIO, elem *DataElement, cc codecContext, reencodeValues bool, st *writeState) error {
	if st == nil {
		st = newWriteState()
	}
	logDebug(st.ctx, "data element",
		AttrTag, elem.Tag.String(),
		AttrVR, string(elem.VR),
		AttrUndefined, elem.IsUndefinedLength,
	)
	if elem.RawValue != nil && elem.VR != VRSQ && !reencodeValues {
		return writeElementFromRaw(fp, elem, cc)
	}

	if !StandardVRs[elem.VR] && !IsAmbiguousVR(elem.VR) {
		return fmt.Errorf("godicom: unknown Value Representation %q", elem.VR)
	}

	if !cc.IsImplicitVR && IsAmbiguousVR(elem.VR) {
		return fmt.Errorf(
			"godicom: cannot write ambiguous VR %q for tag %s; set the correct VR or use implicit VR transfer syntax",
			elem.VR, elem.Tag,
		)
	}

	fp.SetByteOrder(cc.IsLittleEndian)

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
		if st.visitingSequences != nil {
			_, isCircular = st.visitingSequences[seq]
		}
	}
	enteredSequence := false
	if isSQ && seq != nil && !isCircular {
		if st.visitingSequences == nil {
			st.visitingSequences = make(map[*Sequence]struct{})
		}
		st.visitingSequences[seq] = struct{}{}
		enteredSequence = true
	}
	if enteredSequence {
		defer delete(st.visitingSequences, seq)
	}

	// For defined-length SQs with items (and not circular), pre-compute content
	var sqBuf *bytes.Buffer
	if isSQ && !isCircular && !undefinedSQ {
		seq, ok := elem.Value.(*Sequence)
		if ok && seq != nil && !seq.IsEmpty() {
			sqBuf = new(bytes.Buffer)
			sqFp := newDicomWriter(sqBuf)
			sqFp.SetByteOrder(cc.IsLittleEndian)

			for _, item := range seq.Items() {
				if err := sqFp.WriteTag(ItemTag); err != nil {
					return err
				}
				var itemBuf bytes.Buffer
				itemFp := newDicomWriter(&itemBuf)
				itemFp.SetByteOrder(cc.IsLittleEndian)
				st.pushSeq(elem.Tag)
				err := writeDatasetState(itemFp, item, cc, reencodeValues, st)
				st.popSeq()
				if err != nil {
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
	encoded, err := encodeValue(elem, cc, st)
	if err != nil {
		return err
	}

	// Pad to even length per PS3.5
	encoded = padToEven(elem.VR, encoded)

	// Write VR + length
	if cc.IsImplicitVR {
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
		st.sqDepth++
		if !isCircular && st.sqDepth <= 100 {
			if seq, ok := elem.Value.(*Sequence); ok && seq != nil && !seq.IsEmpty() {
				for _, item := range seq.Items() {
					var itemBuf bytes.Buffer
					itemFp := newDicomWriter(&itemBuf)
					itemFp.SetByteOrder(cc.IsLittleEndian)
					st.pushSeq(elem.Tag)
					err := writeDatasetState(itemFp, item, cc, reencodeValues, st)
					st.popSeq()
					if err != nil {
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
		st.sqDepth--
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

// encodeValue renders elem's value to its on-the-wire bytes. It returns an error
// only when the value cannot be represented in elem's VR at all -- a non-finite
// float in a DS, say -- rather than letting a value godicom's own validators
// reject reach the file. A value that is merely misspelled for its VR goes to
// st's diagnostic hook instead, which decides whether to write it or fail.
func encodeValue(elem *DataElement, cc codecContext, st *writeState) ([]byte, error) {
	if elem.Value == nil {
		return nil, nil
	}

	le := cc.IsLittleEndian
	switch elem.VR {
	case VRAE, VRAS, VRCS, VRDA, VRDT, VRLO, VRLT, VRSH, VRST, VRTM, VRUC, VRUR, VRUT:
		return encodeStringWithCharsets(elem, cc.Charsets), nil
	case VRDS, VRIS:
		encoded, err := encodeNumberString(elem)
		if err != nil {
			return nil, err
		}
		if err := reportInvalidNumberString(elem, encoded, st); err != nil {
			return nil, err
		}
		return encoded, nil
	case VRUI:
		return encodeStringWithCharsets(elem, cc.Charsets), nil
	case VRPN:
		return encodePNWithCharsets(elem, cc.Charsets), nil
	case VRFD:
		return encodeFloats(elem, le, 8), nil
	case VRFL:
		return encodeFloats(elem, le, 4), nil
	case VRUL:
		return encodeInts(elem, le, 4, false), nil
	case VRUS:
		return encodeInts(elem, le, 2, false), nil
	case VRUV:
		return encodeInts(elem, le, 8, false), nil
	case VRSL:
		return encodeInts(elem, le, 4, true), nil
	case VRSS:
		return encodeInts(elem, le, 2, true), nil
	case VRSV:
		return encodeInts(elem, le, 8, true), nil
	case VRAT:
		return encodeAT(elem, le), nil
	case VROB, VROD, VROF, VROL, VROW, VROV, VRUN:
		return encodeBytes(elem), nil
	case VRSQ:
		return nil, nil // Handled separately
	default:
		return encodeStringWithCharsets(elem, cc.Charsets), nil
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

func encodeNumberString(elem *DataElement) ([]byte, error) {
	if elem.Value == nil {
		return nil, nil
	}
	switch v := elem.Value.(type) {
	case string:
		return []byte(v), nil
	case int:
		return []byte(fmt.Sprintf("%d", v)), nil
	case float64:
		s, err := formatDecimalString(elem.VR, v)
		if err != nil {
			return nil, fmt.Errorf("godicom: %s %s: %w", elem.Tag, elem.VR, err)
		}
		return []byte(s), nil
	case DS:
		return []byte(v.String()), nil
	case IS:
		return []byte(v.String()), nil
	case *MultiValue[int]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += fmt.Sprintf("%d", val)
		}
		return []byte(s), nil
	case *MultiValue[float64]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			part, err := formatDecimalString(elem.VR, val)
			if err != nil {
				return nil, fmt.Errorf("godicom: %s %s value %d: %w", elem.Tag, elem.VR, i, err)
			}
			s += part
		}
		return []byte(s), nil
	case *MultiValue[DS]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += val.String()
		}
		return []byte(s), nil
	case *MultiValue[IS]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += val.String()
		}
		return []byte(s), nil
	case *MultiValue[interface{}]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			if f, ok := val.(float64); ok {
				part, err := formatDecimalString(elem.VR, f)
				if err != nil {
					return nil, fmt.Errorf("godicom: %s %s value %d: %w", elem.Tag, elem.VR, i, err)
				}
				s += part
				continue
			}
			s += fmt.Sprintf("%v", val)
		}
		return []byte(s), nil
	}
	return []byte(fmt.Sprintf("%v", elem.Value)), nil
}

// formatDecimalString renders a float for a DS element the way the DS type
// already does, so a value stored as a plain float64 reaches the file
// identically to the same value stored as a DS.
//
// fmt's %g has no length bound, but PS3.5 caps DS at 16 bytes, so %g overran it
// for anything needing more than 16 significant characters: SetFloat with 1.0/3.0
// wrote "0.3333333333333333" (18 bytes), which godicom's own IsValidDS rejects
// and a strict receiver is entitled to refuse. FormatNumberAsDS is the same
// truncation pydicom's format_number_as_ds applies.
//
// A non-finite float has no DS or IS spelling at all -- %g renders it "NaN",
// "+Inf" or "-Inf", none of which is a decimal or integer string -- so it is
// refused rather than written. Refusing is the only honest option: the DS the
// caller asked for does not exist.
//
// Only VRDS gets the DS length and precision rules. A fractional float in an IS
// is invalid too, but it has no correct spelling to fall back on -- rounding it
// would silently change the value -- so it keeps its %g form and is reported to
// WriteOptions.OnDiagnostic, which is where the caller decides between writing
// it and failing. See reportInvalidNumberString.
func formatDecimalString(vr VR, val float64) (string, error) {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return "", fmt.Errorf("%g has no valid %s representation", val, vr)
	}
	if vr != VRDS {
		return fmt.Sprintf("%g", val), nil
	}
	return FormatNumberAsDS(val)
}

// reportInvalidNumberString offers every part of an encoded DS or IS value that
// godicom's own reader would raise a diagnostic on to the write-side hook.
//
// The writer used to hand these to the file with nothing said. A float64 of 1.5
// in an IS was formatted "1.5", which no IS parser accepts. A DS handed in as an
// 18-character string went out at 18 characters, over the 16 PS3.5 allows. An
// int of 3000000000 in an IS is spelled correctly but outside the range PS3.5
// gives the VR.
//
// The value is still written when the hook returns nil, or when there is no hook
// at all, which is what every existing caller gets: a caller round-tripping a
// file it did not create should not be forced to repair values it never chose.
// Returning the diagnostic makes it a write failure instead. This is the same
// three-way choice pydicom spells IGNORE / WARN / RAISE in
// config.settings.writing_validation_mode, without a mode enum: the hook's
// presence and its return value say which one the caller wants.
//
// The parts are split on the value multiplicity backslash, which neither a DS
// nor an IS may contain, and each is checked on its own -- the joined string
// would fail every check for the separator alone. An empty part is not reported:
// PS3.5 allows an absent value in a multi-valued element, and pydicom's
// validators skip it too.
func reportInvalidNumberString(elem *DataElement, encoded []byte, st *writeState) error {
	if len(encoded) == 0 || (elem.VR != VRDS && elem.VR != VRIS) {
		return nil
	}
	for _, part := range strings.Split(string(encoded), "\\") {
		reason := numberStringReason(elem.VR, part)
		if reason == nil {
			continue
		}
		if err := st.report(Diagnostic{
			Kind: DiagnosticInvalidValue,
			Tag:  elem.Tag,
			VR:   elem.VR,
			Err:  reason,
		}); err != nil {
			return err
		}
	}
	return nil
}

// numberStringReason returns why part is not a valid value for a DS or IS
// element, or nil when it is one. The checks are the ones godicom already
// applies when reading: IsValidDS and IsValidIS for length and spelling, plus
// ISInRange for the range PS3.5 gives an IS but its spelling does not.
func numberStringReason(vr VR, part string) error {
	if part == "" {
		return nil
	}
	if vr == VRDS {
		if IsValidDS(part) {
			return nil
		}
		if len(part) > maxDSLength {
			return fmt.Errorf("%q is %d bytes, over the %d a DS allows", part, len(part), maxDSLength)
		}
		return fmt.Errorf("%q is not a decimal string", part)
	}
	if !IsValidIS(part) {
		if len(part) > maxISLength {
			return fmt.Errorf("%q is %d bytes, over the %d an IS allows", part, len(part), maxISLength)
		}
		return fmt.Errorf("%q is not an integer string", part)
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && !ISInRange(n) {
		return fmt.Errorf("%q is outside [%d, %d], the range an IS allows", part, minISValue, maxISValue)
	}
	return nil
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
	case []byte:
		// Already-encoded PN bytes (mirrors pydicom write_PN raw path).
		return v
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
	case []Tag:
		tags = v
	case []int:
		for _, x := range v {
			tags = append(tags, Tag(x))
		}
	case *MultiValue[Tag]:
		tags = v.Values()
	case *MultiValue[int]:
		for _, x := range v.Values() {
			tags = append(tags, Tag(x))
		}
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

	// PS3.5 7.1.1: AT is a pair of 16-bit values (group then element), not one
	// 32-bit value. The two differ under little endian, and convertTag reads the
	// pair form, so encoding a uint32 here round-trips with the halves swapped.
	buf := make([]byte, len(tags)*4)
	for i, t := range tags {
		order.PutUint16(buf[i*4:], uint16(t.Group()))
		order.PutUint16(buf[i*4+2:], uint16(t.Element()))
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
	return WriteFileContext(context.Background(), filename, ds, opts)
}

// WriteFileContext is like WriteFile but uses ctx for cancellation and logging.
func WriteFileContext(ctx context.Context, filename string, ds *Dataset, opts *WriteOptions) error {
	return writeFile(ctx, filename, writeSource{dataset: ds}, opts)
}

// EncodeFile encodes fd as a Part 10 DICOM file (preamble + DICM + File Meta + dataset).
func EncodeFile(fd *FileDataset, opts *WriteOptions) ([]byte, error) {
	return EncodeFileContext(context.Background(), fd, opts)
}

// EncodeFileContext is like EncodeFile but uses ctx for cancellation and logging.
func EncodeFileContext(ctx context.Context, fd *FileDataset, opts *WriteOptions) ([]byte, error) {
	if fd == nil {
		return nil, fmt.Errorf("godicom: missing FileDataset")
	}
	var buf bytes.Buffer
	if err := WriteContext(ctx, &buf, fd, opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Write encodes fd as a Part 10 DICOM file to w.
func Write(w io.Writer, fd *FileDataset, opts *WriteOptions) error {
	return WriteContext(context.Background(), w, fd, opts)
}

// WriteContext is like Write but uses ctx for cancellation and logging.
func WriteContext(ctx context.Context, w io.Writer, fd *FileDataset, opts *WriteOptions) error {
	if fd == nil {
		return fmt.Errorf("godicom: missing FileDataset")
	}
	return writeTo(ctx, w, writeSource{
		dataset:  fd.Dataset,
		fileMeta: fd.FileMeta,
		preamble: fd.Preamble,
	}, opts)
}

// EncodeDataset encodes ds as a DICOM dataset only (no preamble / File Meta),
// using ts for VR, endianness, and Deflated compression when applicable.
// Suitable for DIMSE C-STORE / C-FIND payloads.
func EncodeDataset(ds *Dataset, ts UID) ([]byte, error) {
	info, known := uid.Known[ts]
	if !known || !info.IsTransferSyntax {
		return nil, fmt.Errorf(
			"godicom: Transfer Syntax UID %q is not a known transfer syntax; use EncodeDatasetEncoding",
			ts,
		)
	}
	if ts.IsCompressed() {
		if elem, ok := ds.Get(MustTag("PixelData")); ok {
			elem.IsUndefinedLength = true
		}
	}
	return encodeDataset(ds, info.IsImplicitVR, info.IsLittleEndian, ts.IsDeflated())
}

// EncodeDatasetEncoding encodes ds with explicit VR/endian flags (no preamble /
// File Meta). Matches pynetdicom.dsutils.encode / pydicom write_dataset.
func EncodeDatasetEncoding(ds *Dataset, isImplicitVR, isLittleEndian bool) ([]byte, error) {
	if isImplicitVR && !isLittleEndian {
		return nil, fmt.Errorf("godicom: implicit VR and big endian is not a valid encoding combination")
	}
	return encodeDataset(ds, isImplicitVR, isLittleEndian, false)
}

// WriteDataset encodes ds with transfer syntax ts and writes the result to w.
func WriteDataset(w io.Writer, ds *Dataset, ts UID) error {
	b, err := EncodeDataset(ds, ts)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func encodeDataset(ds *Dataset, isImplicit, isLittleEndian, deflated bool) ([]byte, error) {
	if ds == nil {
		return nil, fmt.Errorf("godicom: missing dataset")
	}
	for _, elem := range ds.Iter() {
		switch elem.Tag.Group() {
		case 0x0000:
			return nil, fmt.Errorf(
				"godicom: Command Set elements (0000,eeee) are not allowed in EncodeDataset",
			)
		case 0x0002:
			return nil, fmt.Errorf(
				"godicom: File Meta Information group elements (0002,eeee) are not allowed in EncodeDataset; encode the dataset alone",
			)
		}
	}

	var datasetBuf bytes.Buffer
	fp := newDicomWriter(&datasetBuf)
	fp.SetByteOrder(isLittleEndian)
	enc := EncodingInfo{IsImplicitVR: isImplicit, IsLittleEndian: isLittleEndian}
	if err := writeDataset(context.Background(), fp, ds, codecContext{EncodingInfo: enc}, encodingChanged(ds, enc)); err != nil {
		return nil, fmt.Errorf("godicom: error encoding dataset: %w", err)
	}

	if !deflated {
		return datasetBuf.Bytes(), nil
	}

	var deflatedBuf bytes.Buffer
	fw, err := flate.NewWriter(&deflatedBuf, flate.DefaultCompression)
	if err != nil {
		return nil, fmt.Errorf("godicom: error creating deflater: %w", err)
	}
	if _, err := fw.Write(datasetBuf.Bytes()); err != nil {
		return nil, fmt.Errorf("godicom: error deflating dataset: %w", err)
	}
	if err := fw.Close(); err != nil {
		return nil, fmt.Errorf("godicom: error closing deflater: %w", err)
	}
	payload := deflatedBuf.Bytes()
	if len(payload)%2 == 1 {
		payload = append(payload, 0)
	}
	return payload, nil
}

// Ensure binary is used
var _ = binary.BigEndian
var _ = io.Discard
