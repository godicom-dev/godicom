package godicom

import (
	"encoding/binary"
	"io"
	"os"
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

func readFile(filename string, opts *ReadOptions) (*FileDataset, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if opts == nil {
		opts = &ReadOptions{}
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	f.Close()

	if len(data) < 132 {
		return nil, &InvalidDICOMError{Message: "file too small"}
	}

	preamble := data[:128]
	pos := int64(132)

	if string(data[128:132]) != "DICM" {
		if !opts.Force {
			return nil, &InvalidDICOMError{Message: "missing DICM prefix"}
		}
		// Per pydicom: if force and no DICM, seek to 0
		preamble = nil
		pos = 0
	}
	isLittleEndian := true
	isImplicit := false
	inFileMeta := true

	if pos+6 <= int64(len(data)) {
		isImplicit = !hasExplicitVRAt(data, pos)
	}

	// Read all elements in one pass, then separate file meta
	allElements := make([]*DataElement, 0)
	encoding := DefaultCharacterSet

	for pos+4 <= int64(len(data)) {
		currentTag := readTagBytes(data, pos, isLittleEndian)
		if inFileMeta && currentTag.Group() != 0x0002 {
			inFileMeta = false
			if len(allElements) == 0 {
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
			} else {
				ts := determineTransferSyntaxFromElements(allElements)
				isImplicit = ts.IsImplicitVR()
				isLittleEndian = ts.IsLittleEndian()
				currentTag = readTagBytes(data, pos, isLittleEndian)
			}
		}

		if currentTag == ItemDelimiterTag || currentTag == SequenceDelimiterTag {
			pos += 8
			break
		}

		if opts.StopBeforePixels && currentTag == MustTag(0x7FE00010) {
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
			vr = LookupVR(currentTag)
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
				vr = LookupVR(currentTag)
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
			if vr == VRSQ || vr == "" {
				seq, newPos := readSequenceItems(data, pos+int64(hdrSize), isImplicit, isLittleEndian, encoding, opts)
				elem.Value = seq
				pos = newPos
			} else {
				newPos := skipUntilDelimiter(data, pos+int64(hdrSize), SequenceDelimiterTag, isImplicit, isLittleEndian)
				pos = newPos
			}
			if shouldKeepElement(opts, elem.Tag) {
				allElements = append(allElements, elem)
			}
			continue
		}

		if pos+int64(hdrSize+length) > int64(len(data)) {
			break
		}

		value := data[pos+int64(hdrSize) : pos+int64(hdrSize+length)]

		if opts.DeferSize > 0 && uint32(length) > opts.DeferSize {
			elem.Value = value
		} else {
			raw := &RawDataElement{
				Tag:            currentTag,
				VR:             vr,
				Length:         uint32(length),
				Value:          value,
				IsImplicitVR:   isImplicit,
				IsLittleEndian: isLittleEndian,
				IsRaw:          true,
			}
			converted, err := convertValue(raw)
			if err != nil {
				elem.Value = value
			} else {
				elem.Value = converted
			}
		}

		if shouldKeepElement(opts, elem.Tag) {
			allElements = append(allElements, elem)
		}
		pos += int64(hdrSize + length)

		if currentTag == TagCharset {
			if s, ok := elem.Value.(string); ok && s != "" {
				encoding = s
			}
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

	fd := &FileDataset{
		Dataset:  ds,
		Filename: filename,
		Preamble: preamble,
		FileMeta: fileMeta,
	}

	return fd, nil
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

func readSequenceItems(data []byte, offset int64, isImplicitVR, isLittleEndian bool, encoding string, opts *ReadOptions) (*Sequence, int64) {
	seq := NewSequence(nil)
	seq.IsUndefinedLength = true
	pos := offset

	for pos+4 <= int64(len(data)) {
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

		if itemLength == 0xFFFFFFFF {
			readDatasetElements(data, pos, item, isImplicitVR, isLittleEndian, encoding, opts)
			for pos+4 <= int64(len(data)) {
				if readTagBytes(data, pos, isLittleEndian) == ItemDelimiterTag {
					pos += 8
					break
				}
				pos++
			}
		} else if itemLength > 0 {
			readDatasetElements(data, pos, item, isImplicitVR, isLittleEndian, encoding, opts)
			pos += int64(itemLength)
		}

		seq.Append(item)
	}

	return seq, pos
}

func readDatasetElements(data []byte, offset int64, ds *Dataset, isImplicitVR, isLittleEndian bool, encoding string, opts *ReadOptions) error {
	pos := offset

	for pos+4 <= int64(len(data)) {
		currentTag := readTagBytes(data, pos, isLittleEndian)

		if currentTag == ItemDelimiterTag || currentTag == SequenceDelimiterTag {
			pos += 8
			break
		}

		if opts.StopBeforePixels && currentTag == MustTag(0x7FE00010) {
			break
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
			vr = LookupVR(currentTag)
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
				vr = LookupVR(currentTag)
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
			if vr == VRSQ || vr == "" {
				seq, newPos := readSequenceItems(data, pos+int64(hdrSize), isImplicitVR, isLittleEndian, encoding, opts)
				elem.Value = seq
				pos = newPos
			} else {
				newPos := skipUntilDelimiter(data, pos+int64(hdrSize), SequenceDelimiterTag, isImplicitVR, isLittleEndian)
				pos = newPos
			}
			if shouldKeepElement(opts, elem.Tag) {
				ds.Set(elem)
			}
			continue
		}

		if pos+int64(hdrSize+length) > int64(len(data)) {
			break
		}

		value := data[pos+int64(hdrSize) : pos+int64(hdrSize+length)]

		if opts.DeferSize > 0 && uint32(length) > opts.DeferSize {
			elem.Value = value
		} else {
			raw := &RawDataElement{
				Tag:            currentTag,
				VR:             vr,
				Length:         uint32(length),
				Value:          value,
				IsImplicitVR:   isImplicitVR,
				IsLittleEndian: isLittleEndian,
				IsRaw:          true,
			}
			converted, err := convertValue(raw)
			if err != nil {
				elem.Value = value
			} else {
				elem.Value = converted
			}
		}

		if shouldKeepElement(opts, elem.Tag) {
			ds.Set(elem)
		}
		pos += int64(hdrSize + length)

		if currentTag == TagCharset {
			if s, ok := elem.Value.(string); ok && s != "" {
				encoding = s
			}
		}
	}

	return nil
}

func skipUntilDelimiter(data []byte, offset int64, delimiter Tag, isImplicitVR, isLittleEndian bool) int64 {
	pos := offset
	for pos+4 <= int64(len(data)) {
		if readTagBytes(data, pos, isLittleEndian) == delimiter {
			return pos + 8
		}
		pos++
	}
	return pos
}

// ReadFile reads a DICOM file from filename.
func ReadFile(filename string, opts *ReadOptions) (*FileDataset, error) {
	return readFile(filename, opts)
}

// DcmRead reads a DICOM file from filename.
//
// Deprecated: use ReadFile.
func DcmRead(filename string, opts *ReadOptions) (*FileDataset, error) {
	return ReadFile(filename, opts)
}

// DcmReadFile reads a DICOM file from filename with default options.
//
// Deprecated: use ReadFile with nil options.
func DcmReadFile(filename string) (*FileDataset, error) {
	return ReadFile(filename, nil)
}
