package godicom

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

type ReadOptions struct {
	DeferSize        uint32
	StopBeforePixels bool
	Force            bool
	SpecificTags     []Tag
}

func hasExplicitVRAt(data []byte, pos int64) bool {
	if pos+6 > int64(len(data)) {
		return false
	}
	rawVR := data[pos+4 : pos+6]
	return rawVR[0] >= 0x41 && rawVR[0] <= 0x5A && rawVR[1] >= 0x41 && rawVR[1] <= 0x5A
}

func inflateRaw(data []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	return io.ReadAll(r)
}

func readTagBytes(data []byte, pos int64, isLittleEndian bool) Tag {
	var order binary.ByteOrder = binary.LittleEndian
	if !isLittleEndian {
		order = binary.BigEndian
	}
	group := order.Uint16(data[pos : pos+2])
	element := order.Uint16(data[pos+2 : pos+4])
	return NewTag(int(group), int(element))
}

func shouldKeepElement(opts *ReadOptions, tag Tag) bool {
	if tag.Group() == 0x0002 || tag == TagCharset {
		return true
	}
	if opts == nil || len(opts.SpecificTags) == 0 {
		return true
	}
	for _, specificTag := range opts.SpecificTags {
		if tag == specificTag {
			return true
		}
	}
	return false
}

func readDeferSize(opts *ReadOptions) uint32 {
	if opts == nil {
		return 0
	}
	return opts.DeferSize
}

func creatorStringFromElement(elem *DataElement) string {
	if elem == nil || elem.Value == nil {
		return ""
	}
	switch v := elem.Value.(type) {
	case string:
		return strings.TrimRight(v, " ")
	case []byte:
		return strings.TrimRight(string(v), " \x00")
	default:
		return strings.TrimRight(fmt.Sprintf("%v", v), " ")
	}
}

func privateCreatorFromElements(elements []*DataElement, tag Tag) string {
	creatorTag := tag.PrivateCreator()
	for i := len(elements) - 1; i >= 0; i-- {
		if elements[i].Tag == creatorTag {
			return creatorStringFromElement(elements[i])
		}
	}
	return ""
}

func privateCreatorFromDataset(ds *Dataset, tag Tag) string {
	if ds == nil {
		return ""
	}
	if elem, ok := ds.elements[tag.PrivateCreator()]; ok {
		return creatorStringFromElement(elem)
	}
	return ""
}

func lookupVRDuringRead(tag Tag, creator string) VR {
	if tag.IsPrivate() {
		return lookupVRWithCreator(tag, creator)
	}
	return LookupVR(tag)
}

func readFile(filename string, opts *ReadOptions) (*FileDataset, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	var modTime int64
	if info, statErr := f.Stat(); statErr == nil {
		modTime = info.ModTime().Unix()
	}
	f.Close()

	return readBytes(data, filename, modTime, opts)
}

// ReadBytes parses a Part 10 DICOM file from data (preamble optional when Force).
func ReadBytes(data []byte, opts *ReadOptions) (*FileDataset, error) {
	return readBytes(data, "", 0, opts)
}

func readBytes(data []byte, filename string, modTime int64, opts *ReadOptions) (*FileDataset, error) {
	if opts == nil {
		opts = &ReadOptions{}
	}

	if len(data) < 8 {
		return nil, &InvalidDICOMError{Message: "file too small"}
	}

	var preamble []byte
	pos := int64(0)

	if len(data) >= 132 && string(data[128:132]) == "DICM" {
		preamble = data[:128]
		pos = 132
	} else if !opts.Force {
		return nil, &InvalidDICOMError{Message: "missing DICM prefix"}
	}
	isLittleEndian := true
	isImplicit := false
	inFileMeta := true

	if pos+6 <= int64(len(data)) {
		isImplicit = !hasExplicitVRAt(data, pos)
	}

	// Read all elements in one pass, then separate file meta
	allElements := make([]*DataElement, 0)
	charsets := []string{DefaultCharacterSet}
	readCtx := &readContext{data: data, filename: filename, modTime: modTime}

	for pos+4 <= int64(len(data)) {
		currentTag := readTagBytes(data, pos, isLittleEndian)
		if inFileMeta && currentTag.Group() != 0x0002 {
			inFileMeta = false
			if len(allElements) > 0 {
				ts := determineTransferSyntaxFromElements(allElements)
				if ts == DeflatedExplicitVRLittleEndian {
					inflated, err := inflateRaw(data[pos:])
					if err != nil {
						return nil, err
					}
					data = inflated
					pos = 0
					isImplicit = false
					isLittleEndian = true
					currentTag = readTagBytes(data, pos, isLittleEndian)
				} else {
					isImplicit = ts.IsImplicitVR()
					isLittleEndian = ts.IsLittleEndian()
					currentTag = readTagBytes(data, pos, isLittleEndian)
				}
			} else {
				littleTag := readTagBytes(data, pos, true)
				bigTag := readTagBytes(data, pos, false)
				switch {
				case hasExplicitVRAt(data, pos) && !dictionaryHasTag(littleTag) && dictionaryHasTag(bigTag):
					isImplicit = false
					isLittleEndian = false
					currentTag = bigTag
				case hasExplicitVRAt(data, pos):
					isImplicit = false
					currentTag = littleTag
				default:
					isImplicit = true
					isLittleEndian = true
					currentTag = littleTag
				}
			}
		}

		if currentTag == ItemDelimiterTag || currentTag == SequenceDelimiterTag {
			pos += 8
			break
		}

		if opts != nil && opts.StopBeforePixels && currentTag == MustTag(0x7FE00010) {
			break
		}

		var vr VR
		var length int
		var hdrSize int

		if isImplicit {
			if pos+8 > int64(len(data)) {
				break
			}
			if isLittleEndian {
				length = int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
			} else {
				length = int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
			}
			hdrSize = 8
			vr = lookupVRDuringRead(currentTag, privateCreatorFromElements(allElements, currentTag))
		} else {
			if pos+8 > int64(len(data)) {
				break
			}
			vrBytes := data[pos+4 : pos+6]
			vrStr := string(vrBytes)
			vr = VR(vrStr)

			// Per pydicom: if VR is not valid ASCII uppercase (AA-ZZ),
			// switch to implicit VR encoding (issue 1067, 1035)
			if vrBytes[0] < 0x41 || vrBytes[0] > 0x5A || vrBytes[1] < 0x41 || vrBytes[1] > 0x5A {
				if isLittleEndian {
					length = int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
				} else {
					length = int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
				}
				hdrSize = 8
				vr = lookupVRDuringRead(currentTag, privateCreatorFromElements(allElements, currentTag))
			} else if ExplicitVRLength16[vr] {
				if isLittleEndian {
					length = int(binary.LittleEndian.Uint16(data[pos+6 : pos+8]))
				} else {
					length = int(binary.BigEndian.Uint16(data[pos+6 : pos+8]))
				}
				hdrSize = 8
			} else {
				if pos+12 > int64(len(data)) {
					break
				}
				if isLittleEndian {
					length = int(binary.LittleEndian.Uint32(data[pos+8 : pos+12]))
				} else {
					length = int(binary.BigEndian.Uint32(data[pos+8 : pos+12]))
				}
				hdrSize = 12
			}
		}

		elem := NewDataElement(currentTag, vr, nil)

		if length == 0 {
			elem.Value = emptyValueForVR(vr)
			pos += int64(hdrSize)
			if shouldKeepElement(opts, elem.Tag) {
				allElements = append(allElements, elem)
			}
			continue
		}

		if length == 0xFFFFFFFF {
			elem.IsUndefinedLength = true
			valueStart := pos + int64(hdrSize)
			if shouldReadUndefinedLengthAsSequence(vr) {
				if vr == VRUN {
					elem.VR = VRSQ
				}
				seq, newPos := readSequenceItems(data, valueStart, isImplicit, isLittleEndian, charsets, opts, readCtx)
				elem.Value = seq
				pos = newPos
			} else {
				if encapsulated, endPos, ok := readEncapsulatedPixelData(data, valueStart, isLittleEndian); ok {
					if shouldDeferElement(currentTag, len(encapsulated), readDeferSize(opts)) {
						markElementDeferred(elem, valueStart, len(encapsulated), isImplicit, isLittleEndian, charsets)
					} else {
						assignElementBytes(elem, encapsulated, vr, isImplicit, isLittleEndian, charsets)
					}
					pos = endPos
				} else {
					raw, newPos := readBytesUntilDelimiter(data, valueStart, SequenceDelimiterTag, isLittleEndian)
					elem.RawValue = raw
					pos = newPos
				}
			}
			if shouldKeepElement(opts, elem.Tag) {
				allElements = append(allElements, elem)
			}
			continue
		}

		if vr == VRSQ {
			seq, newPos := readDefinedLengthSequence(
				data,
				pos+int64(hdrSize),
				length,
				isImplicit,
				isLittleEndian,
				charsets,
				opts,
				readCtx,
			)
			elem.Value = seq
			pos = newPos
			if shouldKeepElement(opts, elem.Tag) {
				allElements = append(allElements, elem)
			}
			continue
		}

		if pos+int64(hdrSize+length) > int64(len(data)) {
			break
		}

		value := data[pos+int64(hdrSize) : pos+int64(hdrSize+length)]
		valueTell := pos + int64(hdrSize)

		if shouldDeferElement(currentTag, length, readDeferSize(opts)) {
			markElementDeferred(elem, valueTell, length, isImplicit, isLittleEndian, charsets)
		} else {
			assignElementBytes(elem, value, vr, isImplicit, isLittleEndian, charsets)
		}

		if shouldKeepElement(opts, elem.Tag) {
			allElements = append(allElements, elem)
		}
		pos += int64(hdrSize + length)

		if currentTag == TagCharset {
			charsets = ParseCharacterSets(elem.Value)
		}
	}

	// Separate file meta (group 0x0002) from dataset
	fileMeta := NewFileMetaDataset()
	ds := NewDataset()

	for _, elem := range allElements {
		if elem.Tag.Group() == 0x0002 {
			fileMeta.Set(elem)
		} else {
			ds.Set(elem)
		}
	}

	if fileMeta.Len() > 0 {
		ts := determineTransferSyntax(fileMeta)
		ds.originalEnc = EncodingInfo{
			IsImplicitVR:   ts.IsImplicitVR(),
			IsLittleEndian: ts.IsLittleEndian(),
		}
	} else {
		ds.originalEnc = EncodingInfo{
			IsImplicitVR:   isImplicit,
			IsLittleEndian: isLittleEndian,
		}
	}
	propagateEncoding(ds, ds.originalEnc)
	captureOriginalCharsets(ds)

	fd := &FileDataset{
		Dataset:  ds,
		Filename: filename,
		Preamble: preamble,
		FileMeta: fileMeta,
	}
	if modTime != 0 {
		fd.Timestamp = fmt.Sprintf("%d", modTime)
	}
	ds.readCtx = readCtx

	return fd, nil
}

func captureOriginalCharsets(ds *Dataset) {
	if ds == nil {
		return
	}
	ds.originalCharsets = datasetCharacterSets(ds)
	for _, elem := range ds.Iter() {
		if elem.VR != VRSQ {
			continue
		}
		seq, ok := elem.Value.(*Sequence)
		if !ok || seq == nil {
			continue
		}
		for _, item := range seq.Items() {
			captureOriginalCharsets(item)
		}
	}
}

func datasetCharacterSets(ds *Dataset) []string {
	if ds == nil {
		return []string{DefaultCharacterSet}
	}
	if elem, ok := ds.elements[TagCharset]; ok {
		return ParseCharacterSets(elem.Value)
	}
	return []string{DefaultCharacterSet}
}

func propagateEncoding(ds *Dataset, enc EncodingInfo) {
	ds.originalEnc = enc
	for _, elem := range ds.Iter() {
		if elem.VR != VRSQ {
			continue
		}
		seq, ok := elem.Value.(*Sequence)
		if !ok {
			continue
		}
		for _, item := range seq.Items() {
			propagateEncoding(item, enc)
		}
	}
}

func determineTransferSyntaxFromElements(elements []*DataElement) UID {
	for _, elem := range elements {
		if elem.Tag == MustTag(0x00020010) {
			if uid, ok := elem.Value.(UID); ok {
				return uid
			}
			if s, ok := elem.Value.(string); ok {
				return UID(s)
			}
		}
	}
	return ImplicitVRLittleEndian
}

func determineTransferSyntax(fileMeta *FileMetaDataset) UID {
	if elem, ok := fileMeta.Get(MustTag(0x00020010)); ok {
		if uid, ok2 := elem.Value.(UID); ok2 {
			return uid
		}
		if s, ok2 := elem.Value.(string); ok2 {
			return UID(s)
		}
	}
	return ImplicitVRLittleEndian
}

func readSequenceItems(data []byte, offset int64, isImplicitVR, isLittleEndian bool, charsets []string, opts *ReadOptions, ctx *readContext) (*Sequence, int64) {
	seq, newPos := readSequenceItemsUntil(data, offset, int64(len(data)), true, isImplicitVR, isLittleEndian, charsets, opts, ctx)
	seq.IsUndefinedLength = true
	return seq, newPos
}

func readDefinedLengthSequence(data []byte, offset int64, length int, isImplicitVR, isLittleEndian bool, charsets []string, opts *ReadOptions, ctx *readContext) (*Sequence, int64) {
	return readSequenceItemsUntil(
		data,
		offset,
		offset+int64(length),
		false,
		isImplicitVR,
		isLittleEndian,
		charsets,
		opts,
		ctx,
	)
}

func readSequenceItemsUntil(
	data []byte,
	offset int64,
	end int64,
	undefinedLength bool,
	isImplicitVR bool,
	isLittleEndian bool,
	charsets []string,
	opts *ReadOptions,
	ctx *readContext,
) (*Sequence, int64) {
	seq := NewSequence(nil)
	seq.IsUndefinedLength = undefinedLength
	pos := offset

	for pos+8 <= end && pos+4 <= int64(len(data)) {
		currentTag := readTagBytes(data, pos, isLittleEndian)

		if currentTag == SequenceDelimiterTag {
			pos += 8
			break
		}

		if currentTag != ItemTag {
			break
		}

		if pos+8 > int64(len(data)) {
			break
		}

		var itemLength int
		if isLittleEndian {
			itemLength = int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		} else {
			itemLength = int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
		}
		pos += 8

		item := NewDataset()
		item.parent = seq
		item.readCtx = ctx

		if itemLength == 0xFFFFFFFF {
			item.IsUndefinedLengthSequenceItem = true
			var err error
			pos, err = readDatasetElements(data, pos, int64(len(data)), item, isImplicitVR, isLittleEndian, charsets, opts, ctx)
			if err != nil {
				return seq, pos
			}
		} else if itemLength > 0 {
			itemEnd := pos + int64(itemLength)
			var err error
			pos, err = readDatasetElements(data, pos, itemEnd, item, isImplicitVR, isLittleEndian, charsets, opts, ctx)
			if err != nil {
				return seq, pos
			}
			if pos < itemEnd {
				pos = itemEnd
			}
		} else {
			pos += int64(itemLength)
		}

		seq.Append(item)
	}

	return seq, pos
}

func readDatasetElements(data []byte, offset int64, end int64, ds *Dataset, isImplicitVR, isLittleEndian bool, charsets []string, opts *ReadOptions, ctx *readContext) (int64, error) {
	ds.readCtx = ctx
	if len(charsets) == 0 {
		charsets = []string{DefaultCharacterSet}
	}
	pos := offset

	for pos+4 <= end && pos+4 <= int64(len(data)) {
		currentTag := readTagBytes(data, pos, isLittleEndian)

		if currentTag == ItemDelimiterTag || currentTag == SequenceDelimiterTag {
			return pos + 8, nil
		}

		if opts != nil && opts.StopBeforePixels && currentTag == MustTag(0x7FE00010) {
			return pos, nil
		}

		var vr VR
		var length int
		var hdrSize int

		if isImplicitVR {
			if pos+8 > int64(len(data)) {
				break
			}
			if isLittleEndian {
				length = int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
			} else {
				length = int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
			}
			hdrSize = 8
			vr = lookupVRDuringRead(currentTag, privateCreatorFromDataset(ds, currentTag))
		} else {
			if pos+8 > int64(len(data)) {
				break
			}
			vrBytes := data[pos+4 : pos+6]
			vrStr := string(vrBytes)
			vr = VR(vrStr)

			if vrBytes[0] < 0x41 || vrBytes[0] > 0x5A || vrBytes[1] < 0x41 || vrBytes[1] > 0x5A {
				if isLittleEndian {
					length = int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
				} else {
					length = int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
				}
				hdrSize = 8
				vr = lookupVRDuringRead(currentTag, privateCreatorFromDataset(ds, currentTag))
			} else if ExplicitVRLength16[vr] {
				if isLittleEndian {
					length = int(binary.LittleEndian.Uint16(data[pos+6 : pos+8]))
				} else {
					length = int(binary.BigEndian.Uint16(data[pos+6 : pos+8]))
				}
				hdrSize = 8
			} else {
				if pos+12 > int64(len(data)) {
					break
				}
				if isLittleEndian {
					length = int(binary.LittleEndian.Uint32(data[pos+8 : pos+12]))
				} else {
					length = int(binary.BigEndian.Uint32(data[pos+8 : pos+12]))
				}
				hdrSize = 12
			}
		}

		elem := NewDataElement(currentTag, vr, nil)

		if length == 0 {
			elem.Value = emptyValueForVR(vr)
			pos += int64(hdrSize)
			if shouldKeepElement(opts, elem.Tag) {
				ds.Set(elem)
			}
			continue
		}

		if length == 0xFFFFFFFF {
			elem.IsUndefinedLength = true
			valueStart := pos + int64(hdrSize)
			if shouldReadUndefinedLengthAsSequence(vr) {
				if vr == VRUN {
					elem.VR = VRSQ
				}
				seq, newPos := readSequenceItems(data, valueStart, isImplicitVR, isLittleEndian, charsets, opts, ctx)
				elem.Value = seq
				pos = newPos
			} else {
				if encapsulated, endPos, ok := readEncapsulatedPixelData(data, valueStart, isLittleEndian); ok {
					if shouldDeferElement(currentTag, len(encapsulated), readDeferSize(opts)) {
						markElementDeferred(elem, valueStart, len(encapsulated), isImplicitVR, isLittleEndian, charsets)
					} else {
						assignElementBytes(elem, encapsulated, vr, isImplicitVR, isLittleEndian, charsets)
					}
					pos = endPos
				} else {
					raw, newPos := readBytesUntilDelimiter(data, valueStart, SequenceDelimiterTag, isLittleEndian)
					elem.RawValue = raw
					pos = newPos
				}
			}
			if shouldKeepElement(opts, elem.Tag) {
				ds.Set(elem)
			}
			continue
		}

		if vr == VRSQ {
			seq, newPos := readDefinedLengthSequence(
				data,
				pos+int64(hdrSize),
				length,
				isImplicitVR,
				isLittleEndian,
				charsets,
				opts,
				ctx,
			)
			elem.Value = seq
			pos = newPos
			if shouldKeepElement(opts, elem.Tag) {
				ds.Set(elem)
			}
			continue
		}

		if pos+int64(hdrSize+length) > int64(len(data)) {
			break
		}

		value := data[pos+int64(hdrSize) : pos+int64(hdrSize+length)]
		valueTell := pos + int64(hdrSize)

		if shouldDeferElement(currentTag, length, readDeferSize(opts)) {
			markElementDeferred(elem, valueTell, length, isImplicitVR, isLittleEndian, charsets)
		} else {
			assignElementBytes(elem, value, vr, isImplicitVR, isLittleEndian, charsets)
		}

		if shouldKeepElement(opts, elem.Tag) {
			ds.Set(elem)
		}
		pos += int64(hdrSize + length)

		if currentTag == TagCharset {
			charsets = ParseCharacterSets(elem.Value)
		}
	}

	return pos, nil
}

func shouldReadUndefinedLengthAsSequence(vr VR) bool {
	if vr == VRSQ || vr == "" {
		return true
	}
	// PS3.5 6.2.2: undefined-length UN values are encoded as sequences.
	// Private tags resolve to UN via LookupVR and follow the same rule.
	if vr == VRUN {
		return true
	}
	return false
}

func readBytesUntilDelimiter(data []byte, offset int64, delimiter Tag, isLittleEndian bool) (value []byte, endPos int64) {
	pos := offset
	for pos+4 <= int64(len(data)) {
		if readTagBytes(data, pos, isLittleEndian) == delimiter {
			return append([]byte(nil), data[offset:pos]...), pos + 8
		}
		pos++
	}
	return append([]byte(nil), data[offset:pos]...), pos
}

// readEncapsulatedPixelData reads undefined-length encapsulated pixel data
// (PS3.5 A.4) as a contiguous item stream ending before the sequence delimiter.
func readEncapsulatedPixelData(data []byte, offset int64, isLittleEndian bool) (value []byte, endPos int64, ok bool) {
	start := offset
	pos := offset
	for pos+4 <= int64(len(data)) {
		tag := readTagBytes(data, pos, isLittleEndian)
		if tag == SequenceDelimiterTag {
			if pos+8 > int64(len(data)) {
				return nil, offset, false
			}
			return append([]byte(nil), data[start:pos]...), pos + 8, true
		}
		if tag != ItemTag {
			return nil, offset, false
		}
		if pos+8 > int64(len(data)) {
			return nil, offset, false
		}
		var itemLen uint32
		if isLittleEndian {
			itemLen = binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		} else {
			itemLen = binary.BigEndian.Uint32(data[pos+4 : pos+8])
		}
		if itemLen == 0xFFFFFFFF {
			return nil, offset, false
		}
		pos += 8 + int64(itemLen)
		if pos > int64(len(data)) {
			return nil, offset, false
		}
	}
	return nil, offset, false
}

func cloneElementBytes(value []byte) []byte {
	return append([]byte(nil), value...)
}

func assignElementBytes(elem *DataElement, value []byte, vr VR, isImplicit, isLittleEndian bool, charsets []string) {
	elem.RawValue = cloneElementBytes(value)
	var decodeCharsets []string
	if vrUsesCharacterSet(vr) {
		decodeCharsets = charsets
	}
	raw := &RawDataElement{
		Tag:            elem.Tag,
		VR:             vr,
		Length:         uint32(len(value)),
		Value:          value,
		IsImplicitVR:   isImplicit,
		IsLittleEndian: isLittleEndian,
		IsRaw:          true,
	}
	converted, err := convertValueWithCharsets(raw, decodeCharsets)
	if err != nil {
		elem.Value = value
		return
	}
	elem.Value = converted
}

// ReadFile reads a DICOM file from filename.
func ReadFile(filename string, opts *ReadOptions) (*FileDataset, error) {
	return readFile(filename, opts)
}
