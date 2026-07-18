package godicom

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Read parses a DICOM Part 10 dataset from r.
//
// Prefer a seekable source (*os.File, io.ReadSeeker, io.ReaderAt): the parser
// walks tags without io.ReadAll, so StopBeforePixels / DeferSize / SpecificTags
// can skip large values without buffering them. Deferred elements are reloaded
// later by reopening the path when r is an *os.File (see ReadFile).
//
// Non-seekable readers fall back to buffering the stream (then ReadBytes).
func Read(r io.Reader, opts *ReadOptions) (*FileDataset, error) {
	if r == nil {
		return nil, fmt.Errorf("godicom: nil reader")
	}
	if f, ok := r.(*os.File); ok {
		info, err := f.Stat()
		if err != nil {
			return nil, err
		}
		return readReaderAt(f, info.Size(), f.Name(), info.ModTime().Unix(), opts)
	}
	if rs, ok := r.(io.ReadSeeker); ok {
		size, err := rs.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return readReaderAt(seekerReaderAt{rs: rs}, size, "", 0, opts)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return readBytes(data, "", 0, opts)
}

// seekerReaderAt adapts a ReadSeeker to ReaderAt via Seek+Read.
// Not safe for concurrent ReadAt.
type seekerReaderAt struct {
	rs io.ReadSeeker
}

func (s seekerReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if _, err := s.rs.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	return io.ReadFull(s.rs, p)
}

func readFile(filename string, opts *ReadOptions) (*FileDataset, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return readReaderAt(f, info.Size(), filename, info.ModTime().Unix(), opts)
}

type atView struct {
	ra   io.ReaderAt
	size int64
}

func (v atView) inRange(pos, n int64) bool {
	return pos >= 0 && n >= 0 && pos+n <= v.size
}

func (v atView) bytes(pos, n int64) ([]byte, error) {
	if n == 0 {
		return nil, nil
	}
	if !v.inRange(pos, n) {
		return nil, io.ErrUnexpectedEOF
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(io.NewSectionReader(v.ra, pos, n), buf)
	return buf, err
}

func (v atView) tag(pos int64, littleEndian bool) (Tag, error) {
	b, err := v.bytes(pos, 4)
	if err != nil {
		return 0, err
	}
	var order binary.ByteOrder = binary.LittleEndian
	if !littleEndian {
		order = binary.BigEndian
	}
	return NewTag(int(order.Uint16(b[0:2])), int(order.Uint16(b[2:4]))), nil
}

func (v atView) hasExplicitVR(pos int64) bool {
	b, err := v.bytes(pos+4, 2)
	if err != nil {
		return false
	}
	return b[0] >= 0x41 && b[0] <= 0x5A && b[1] >= 0x41 && b[1] <= 0x5A
}

func readReaderAt(ra io.ReaderAt, size int64, filename string, modTime int64, opts *ReadOptions) (*FileDataset, error) {
	if opts == nil {
		opts = &ReadOptions{}
	}
	if size < 8 {
		return nil, &InvalidDICOMError{Message: "file too small"}
	}

	v := atView{ra: ra, size: size}
	var preamble []byte
	pos := int64(0)

	if size >= 132 {
		magic, err := v.bytes(128, 4)
		if err != nil {
			return nil, err
		}
		if string(magic) == "DICM" {
			preamble, err = v.bytes(0, 128)
			if err != nil {
				return nil, err
			}
			pos = 132
		} else if !opts.Force {
			return nil, &InvalidDICOMError{Message: "missing DICM prefix"}
		}
	} else if !opts.Force {
		return nil, &InvalidDICOMError{Message: "missing DICM prefix"}
	}

	isLittleEndian := true
	isImplicit := false
	inFileMeta := true

	if pos+6 <= size {
		isImplicit = !v.hasExplicitVR(pos)
	}

	allElements := make([]*DataElement, 0)
	charsets := []string{DefaultCharacterSet}
	// No in-memory copy of the file: deferred loads reopen filename.
	readCtx := &readContext{filename: filename, modTime: modTime, size: size}

	for pos+4 <= size {
		currentTag, err := v.tag(pos, isLittleEndian)
		if err != nil {
			break
		}

		if inFileMeta && currentTag.Group() != 0x0002 {
			inFileMeta = false
			if len(allElements) > 0 {
				ts := determineTransferSyntaxFromElements(allElements)
				if ts == DeflatedExplicitVRLittleEndian {
					rest, err := v.bytes(pos, size-pos)
					if err != nil {
						return nil, err
					}
					inflated, err := inflateRaw(rest)
					if err != nil {
						return nil, err
					}
					// Remainder is an in-memory dataset; keep buffer for deferred.
					return finishDeflated(inflated, preamble, allElements, filename, modTime, opts)
				}
				isImplicit = ts.IsImplicitVR()
				isLittleEndian = ts.IsLittleEndian()
				currentTag, err = v.tag(pos, isLittleEndian)
				if err != nil {
					break
				}
			} else {
				littleTag, err1 := v.tag(pos, true)
				bigTag, err2 := v.tag(pos, false)
				if err1 != nil || err2 != nil {
					break
				}
				switch {
				case v.hasExplicitVR(pos) && !dictionaryHasTag(littleTag) && dictionaryHasTag(bigTag):
					isImplicit = false
					isLittleEndian = false
					currentTag = bigTag
				case v.hasExplicitVR(pos):
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

		vr, length, hdrSize, ok := readElementHeaderAt(v, pos, isImplicit, isLittleEndian, allElements, currentTag)
		if !ok {
			break
		}

		elem := NewDataElement(currentTag, vr, nil)
		keep := shouldKeepElement(opts, currentTag)

		if length == 0 {
			elem.Value = emptyValueForVR(vr)
			pos += int64(hdrSize)
			if keep {
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
				seq, endPos, err := readUndefinedSequenceAt(v, valueStart, isImplicit, isLittleEndian, charsets, opts, readCtx)
				if err != nil {
					return nil, err
				}
				if keep {
					elem.Value = seq
					allElements = append(allElements, elem)
				}
				pos = endPos
			} else {
				// Encapsulated / undefined-length OB/OW (typically Pixel Data).
				endPos, encapsulated, err := readOrSkipEncapsulated(v, valueStart, isLittleEndian, keep)
				if err != nil {
					return nil, err
				}
				if keep {
					if shouldDeferElement(currentTag, len(encapsulated), readDeferSize(opts)) {
						markElementDeferred(elem, valueStart, len(encapsulated), isImplicit, isLittleEndian, charsets)
					} else {
						assignElementBytes(elem, encapsulated, vr, isImplicit, isLittleEndian, charsets)
					}
					allElements = append(allElements, elem)
				}
				pos = endPos
			}
			continue
		}

		valueStart := pos + int64(hdrSize)
		next := valueStart + int64(length)

		if vr == VRSQ {
			if keep {
				chunk, err := v.bytes(valueStart, int64(length))
				if err != nil {
					break
				}
				seq, _ := readDefinedLengthSequence(chunk, 0, length, isImplicit, isLittleEndian, charsets, opts, readCtx)
				elem.Value = seq
				allElements = append(allElements, elem)
			}
			pos = next
			continue
		}

		if next > size {
			break
		}

		if keep {
			if shouldDeferElement(currentTag, length, readDeferSize(opts)) {
				markElementDeferred(elem, valueStart, length, isImplicit, isLittleEndian, charsets)
			} else {
				value, err := v.bytes(valueStart, int64(length))
				if err != nil {
					break
				}
				assignElementBytes(elem, value, vr, isImplicit, isLittleEndian, charsets)
			}
			allElements = append(allElements, elem)
			if currentTag == TagCharset {
				charsets = ParseCharacterSets(elem.Value)
			}
		}
		// Skip: advance without allocating the value (Go win vs ReadAll).
		pos = next
	}

	return assembleFileDataset(allElements, preamble, filename, modTime, isImplicit, isLittleEndian, readCtx), nil
}

func readElementHeaderAt(
	v atView,
	pos int64,
	isImplicit, isLittleEndian bool,
	allElements []*DataElement,
	currentTag Tag,
) (vr VR, length, hdrSize int, ok bool) {
	if isImplicit {
		if !v.inRange(pos, 8) {
			return "", 0, 0, false
		}
		b, err := v.bytes(pos+4, 4)
		if err != nil {
			return "", 0, 0, false
		}
		if isLittleEndian {
			length = int(binary.LittleEndian.Uint32(b))
		} else {
			length = int(binary.BigEndian.Uint32(b))
		}
		return lookupVRDuringRead(currentTag, privateCreatorFromElements(allElements, currentTag)), length, 8, true
	}

	if !v.inRange(pos, 8) {
		return "", 0, 0, false
	}
	vrBytes, err := v.bytes(pos+4, 2)
	if err != nil {
		return "", 0, 0, false
	}
	vr = VR(string(vrBytes))
	if vrBytes[0] < 0x41 || vrBytes[0] > 0x5A || vrBytes[1] < 0x41 || vrBytes[1] > 0x5A {
		b, err := v.bytes(pos+4, 4)
		if err != nil {
			return "", 0, 0, false
		}
		if isLittleEndian {
			length = int(binary.LittleEndian.Uint32(b))
		} else {
			length = int(binary.BigEndian.Uint32(b))
		}
		return lookupVRDuringRead(currentTag, privateCreatorFromElements(allElements, currentTag)), length, 8, true
	}
	if ExplicitVRLength16[vr] {
		b, err := v.bytes(pos+6, 2)
		if err != nil {
			return "", 0, 0, false
		}
		if isLittleEndian {
			length = int(binary.LittleEndian.Uint16(b))
		} else {
			length = int(binary.BigEndian.Uint16(b))
		}
		return vr, length, 8, true
	}
	if !v.inRange(pos, 12) {
		return "", 0, 0, false
	}
	b, err := v.bytes(pos+8, 4)
	if err != nil {
		return "", 0, 0, false
	}
	if isLittleEndian {
		length = int(binary.LittleEndian.Uint32(b))
	} else {
		length = int(binary.BigEndian.Uint32(b))
	}
	return vr, length, 12, true
}

func readUndefinedSequenceAt(
	v atView,
	offset int64,
	isImplicit, littleEndian bool,
	charsets []string,
	opts *ReadOptions,
	ctx *readContext,
) (*Sequence, int64, error) {
	// Walk defined-length items without buffering the rest of the file. Stop at
	// SequenceDelimiter or the next non-Item tag (same rule as readSequenceItems).
	pos := offset
	for pos+8 <= v.size {
		tag, err := v.tag(pos, littleEndian)
		if err != nil {
			return nil, pos, err
		}
		if tag == SequenceDelimiterTag {
			chunk, err := v.bytes(offset, pos-offset)
			if err != nil {
				return nil, pos, err
			}
			seq, _ := readSequenceItems(chunk, 0, isImplicit, littleEndian, charsets, opts, ctx)
			return seq, pos + 8, nil
		}
		if tag != ItemTag {
			chunk, err := v.bytes(offset, pos-offset)
			if err != nil {
				return nil, pos, err
			}
			seq, _ := readSequenceItems(chunk, 0, isImplicit, littleEndian, charsets, opts, ctx)
			return seq, pos, nil
		}
		b, err := v.bytes(pos+4, 4)
		if err != nil {
			return nil, pos, err
		}
		var itemLen uint32
		if littleEndian {
			itemLen = binary.LittleEndian.Uint32(b)
		} else {
			itemLen = binary.BigEndian.Uint32(b)
		}
		if itemLen == 0xFFFFFFFF {
			// Nested undefined item: buffer from here and reuse the byte parser
			// (stops at SequenceDelimiter / non-Item, does not require EOF).
			rest, err := v.bytes(offset, v.size-offset)
			if err != nil {
				return nil, pos, err
			}
			seq, end := readSequenceItems(rest, 0, isImplicit, littleEndian, charsets, opts, ctx)
			return seq, offset + end, nil
		}
		pos += 8 + int64(itemLen)
	}
	chunk, err := v.bytes(offset, pos-offset)
	if err != nil {
		return nil, pos, err
	}
	seq, end := readSequenceItems(chunk, 0, isImplicit, littleEndian, charsets, opts, ctx)
	return seq, offset + end, nil
}

// readOrSkipEncapsulated finds the end of encapsulated pixel data.
// If keep is true, returns the encaps bytes; otherwise only advances.
func readOrSkipEncapsulated(v atView, offset int64, littleEndian, keep bool) (endPos int64, value []byte, err error) {
	pos := offset
	for pos+8 <= v.size {
		tag, err := v.tag(pos, littleEndian)
		if err != nil {
			return pos, nil, err
		}
		if tag == SequenceDelimiterTag {
			end := pos + 8
			if !keep {
				return end, nil, nil
			}
			val, err := v.bytes(offset, pos-offset) // exclude sequence delimiter
			return end, val, err
		}
		if tag != ItemTag {
			break
		}
		b, err := v.bytes(pos+4, 4)
		if err != nil {
			return pos, nil, err
		}
		var itemLen uint32
		if littleEndian {
			itemLen = binary.LittleEndian.Uint32(b)
		} else {
			itemLen = binary.BigEndian.Uint32(b)
		}
		pos += 8
		if itemLen == 0xFFFFFFFF {
			rest, err := v.bytes(offset, v.size-offset)
			if err != nil {
				return pos, nil, err
			}
			enc, end, ok := readEncapsulatedPixelData(rest, 0, littleEndian)
			if !ok {
				raw, newPos := readBytesUntilDelimiter(rest, 0, SequenceDelimiterTag, littleEndian)
				if keep {
					return offset + newPos, raw, nil
				}
				return offset + newPos, nil, nil
			}
			if keep {
				return offset + end, enc, nil
			}
			return offset + end, nil, nil
		}
		pos += int64(itemLen)
	}
	if keep {
		val, err := v.bytes(offset, v.size-offset)
		return v.size, val, err
	}
	return v.size, nil, nil
}

func finishDeflated(
	inflated []byte,
	preamble []byte,
	metaElems []*DataElement,
	filename string,
	modTime int64,
	opts *ReadOptions,
) (*FileDataset, error) {
	sub := &ReadOptions{Force: true}
	if opts != nil {
		sub.DeferSize = opts.DeferSize
		sub.StopBeforePixels = opts.StopBeforePixels
		sub.SpecificTags = opts.SpecificTags
	}
	rest, err := readBytes(inflated, filename, modTime, sub)
	if err != nil {
		return nil, err
	}
	fileMeta := NewFileMetaDataset()
	for _, elem := range metaElems {
		if elem.Tag.Group() == 0x0002 {
			fileMeta.Set(elem)
		}
	}
	if fileMeta.Len() > 0 {
		rest.FileMeta = fileMeta
		ts := determineTransferSyntax(fileMeta)
		rest.originalEnc = EncodingInfo{IsImplicitVR: ts.IsImplicitVR(), IsLittleEndian: ts.IsLittleEndian()}
		propagateEncoding(rest.Dataset, rest.originalEnc)
	}
	if len(preamble) > 0 {
		rest.Preamble = preamble
	}
	return rest, nil
}

func assembleFileDataset(
	allElements []*DataElement,
	preamble []byte,
	filename string,
	modTime int64,
	isImplicit, isLittleEndian bool,
	readCtx *readContext,
) *FileDataset {
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
		ds.originalEnc = EncodingInfo{IsImplicitVR: ts.IsImplicitVR(), IsLittleEndian: ts.IsLittleEndian()}
	} else {
		ds.originalEnc = EncodingInfo{IsImplicitVR: isImplicit, IsLittleEndian: isLittleEndian}
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
	return fd
}
