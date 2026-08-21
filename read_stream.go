package godicom

import (
	"context"
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
// For any other seekable reader there is no path to reopen, so the returned
// dataset retains r itself for deferred loads: keep it open until every deferred
// value has been loaded. When r is only an io.ReadSeeker that also means not
// moving its offset or reading from it concurrently, because each deferred load
// seeks it. A reader that already offers io.ReaderAt plus Size() int64
// (*bytes.Reader, *strings.Reader, *io.SectionReader) is used as-is and carries
// neither restriction.
//
// Non-seekable readers fall back to buffering the stream (then ReadBytes).
func Read(r io.Reader, opts *ReadOptions) (*FileDataset, error) {
	return ReadContext(context.Background(), r, opts)
}

// ReadContext is like Read but uses ctx for cancellation and logging.
func ReadContext(ctx context.Context, r io.Reader, opts *ReadOptions) (*FileDataset, error) {
	if r == nil {
		return nil, fmt.Errorf("godicom: nil reader")
	}
	if f, ok := r.(*os.File); ok {
		info, err := f.Stat()
		if err != nil {
			return nil, err
		}
		return readReaderAt(ctx, f, info.Size(), f.Name(), info.ModTime().Unix(), opts)
	}
	if sized, ok := r.(interface {
		io.ReaderAt
		Size() int64
	}); ok {
		// *bytes.Reader, *strings.Reader and *io.SectionReader already offer
		// random access over a known extent, which is all the parser wants. Using
		// them directly skips the seekerReaderAt wrapper below, so parsing and
		// deferred loads leave the caller's offset alone.
		return readReaderAt(ctx, sized, sized.Size(), "", 0, opts)
	}
	if rs, ok := r.(io.ReadSeeker); ok {
		size, err := rs.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return readReaderAt(ctx, seekerReaderAt{rs: rs}, size, "", 0, opts)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return readBytes(ctx, data, "", 0, opts)
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

func readFile(ctx context.Context, filename string, opts *ReadOptions) (*FileDataset, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return readReaderAt(ctx, f, info.Size(), filename, info.ModTime().Unix(), opts)
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

func readReaderAt(ctx context.Context, ra io.ReaderAt, size int64, filename string, modTime int64, opts *ReadOptions) (*FileDataset, error) {
	if opts == nil {
		opts = &ReadOptions{}
	}
	ctx = loggerContext(ctx, opts.Logger, ComponentReader)
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
			logDebug(ctx, "Reading File Meta Information preamble", AttrOffset, int64(0), AttrOffsetHex, offsetHex(0), AttrPath, filename)
			if len(preamble) >= 8 {
				logDebug(ctx, "preamble sample", AttrOffsetHex, offsetHex(0), AttrHex, bytesHex(preamble[:8]))
			}
			logDebug(ctx, "Reading File Meta Information prefix", AttrOffset, int64(128), AttrOffsetHex, offsetHex(128))
			logDebug(ctx, "'DICM' prefix found", AttrOffset, int64(128), AttrOffsetHex, offsetHex(128), AttrPath, filename)
		} else if !opts.Force {
			return nil, &InvalidDICOMError{Message: "missing DICM prefix"}
		} else {
			logDebug(ctx, "reading without DICM prefix", AttrPath, filename)
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
	// No in-memory copy of the file: deferred loads reopen filename, or re-read
	// through ra when there is no path to reopen (a non-*os.File ReadSeeker).
	readCtx := &readContext{filename: filename, modTime: modTime, size: size, src: ra, ctx: ctx, onDiag: diagnosticHook(opts)}
	creator := elementsCreator(&allElements)

	for pos+4 <= size {
		currentTag, err := v.tag(pos, isLittleEndian)
		if err != nil {
			break
		}

		if inFileMeta && currentTag.Group() != 0x0002 {
			inFileMeta = false
			if len(allElements) > 0 {
				ts := determineTransferSyntaxFromElements(allElements)
				logDebug(ctx, "transfer syntax", AttrTransferSyntax, string(ts), AttrOffset, pos)
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
					return finishDeflated(ctx, inflated, preamble, allElements, filename, modTime, opts)
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
			break
		}

		if opts != nil && opts.StopBeforePixels && currentTag == MustTag(0x7FE00010) {
			break
		}

		h, header, need, ok := readElementHeaderAt(v, pos, isImplicit, isLittleEndian, currentTag, creator)
		if !ok {
			if err := readCtx.report(truncatedHeader(currentTag, pos, need, size)); err != nil {
				return nil, err
			}
			break
		}
		vr, length, hdrSize := h.VR, h.Length, h.Size
		logElementHeader(ctx, pos, header, currentTag, vr, length)

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
				logDebug(ctx, "Reading/parsing undefined length sequence",
					AttrOffset, valueStart, AttrOffsetHex, offsetHex(valueStart), AttrTag, currentTag.String())
				readCtx.pushSeq(currentTag)
				seq, endPos, err := readUndefinedSequenceAt(v, valueStart, isImplicit, isLittleEndian, charsets, opts, readCtx)
				readCtx.popSeq()
				if err != nil {
					return nil, err
				}
				if keep {
					elem.Value = seq
					allElements = append(allElements, elem)
				}
				pos = endPos
			} else {
				logDebug(ctx, "Reading undefined length data element",
					AttrOffset, valueStart, AttrOffsetHex, offsetHex(valueStart), AttrTag, currentTag.String())
				// Encapsulated / undefined-length OB/OW (typically Pixel Data).
				endPos, encapsulated, err := readOrSkipEncapsulated(v, valueStart, isLittleEndian, keep)
				if err != nil {
					return nil, err
				}
				if keep {
					if shouldDeferElement(currentTag, len(encapsulated), readDeferSize(opts)) {
						logDebug(ctx, "Defer size exceeded. Skipping forward to next data element.",
							AttrTag, currentTag.String(), AttrLen, len(encapsulated))
						markElementDeferred(elem, valueStart, len(encapsulated), isImplicit, isLittleEndian, charsets)
					} else {
						logElementValue(ctx, valueStart, encapsulated)
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
				// The declared length can run past the end of the file. Hand the item
				// loop the bytes that are there but keep the declared length as its
				// end bound, so it walks the items that are present and reports the
				// shortfall as a truncated item: the diagnostic ReadBytes already
				// raises from the same bytes, and the point pydicom fails at
				// (read_sequence_item, "No tag to read at file position"). Asking for
				// the declared length instead got ErrUnexpectedEOF and dropped the
				// whole element without a word.
				chunk, err := v.bytes(valueStart, min(int64(length), size-valueStart))
				if err != nil {
					break
				}
				restore := readCtx.setBase(valueStart)
				readCtx.pushSeq(currentTag)
				seq, _, err := readDefinedLengthSequence(chunk, 0, length, isImplicit, isLittleEndian, charsets, opts, readCtx)
				readCtx.popSeq()
				restore()
				if err != nil {
					return nil, err
				}
				elem.Value = seq
				allElements = append(allElements, elem)
			}
			pos = next
			continue
		}

		if next > size {
			if err := readCtx.report(truncatedValue(currentTag, vr, valueStart, int64(length), size)); err != nil {
				return nil, err
			}
			break
		}

		if keep {
			if shouldDeferElement(currentTag, length, readDeferSize(opts)) {
				logDebug(ctx, "Defer size exceeded. Skipping forward to next data element.",
					AttrTag, currentTag.String(), AttrLen, length)
				markElementDeferred(elem, valueStart, length, isImplicit, isLittleEndian, charsets)
			} else {
				value, err := v.bytes(valueStart, int64(length))
				if err != nil {
					break
				}
				logElementValue(ctx, valueStart, value)
				assignElementBytes(elem, value, vr, isImplicit, isLittleEndian, charsets)
			}
			allElements = append(allElements, elem)
			if currentTag == TagCharset {
				charsets = ParseCharacterSets(elem.Value)
			}
		}
		// Skip: advance without allocating the value (Go win vs ReadAll).
		pos = next
		continue
	}

	return assembleFileDataset(allElements, preamble, filename, modTime, isImplicit, isLittleEndian, readCtx), nil
}

// readElementHeaderAt decodes the header at pos by pulling the at most 12 bytes
// a header can occupy and handing them to the shared decoder, so the streaming
// reader and the in-memory reader agree on every malformed case. The header
// bytes are returned for logging, saving a second read.
func readElementHeaderAt(
	v atView,
	pos int64,
	isImplicit, isLittleEndian bool,
	currentTag Tag,
	creator creatorFunc,
) (h elementHeader, header []byte, need int64, ok bool) {
	n := int64(12)
	if pos+n > v.size {
		n = v.size - pos
	}
	buf, err := v.bytes(pos, n)
	if err != nil {
		return elementHeader{}, nil, 8, false
	}
	h, need, ok = decodeElementHeader(buf, 0, currentTag, isImplicit, isLittleEndian, creator)
	if !ok {
		return elementHeader{}, nil, need, false
	}
	return h, buf[:h.Size], need, true
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
	// Every chunk below is copied from offset, so diagnostics raised by the byte
	// parser are shifted back into source coordinates.
	defer ctx.setBase(offset)()
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
			seq, _, err := readSequenceItems(chunk, 0, isImplicit, littleEndian, charsets, opts, ctx)
			return seq, pos + 8, err
		}
		if tag != ItemTag {
			chunk, err := v.bytes(offset, pos-offset)
			if err != nil {
				return nil, pos, err
			}
			seq, _, err := readSequenceItems(chunk, 0, isImplicit, littleEndian, charsets, opts, ctx)
			return seq, pos, err
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
			seq, end, err := readSequenceItems(rest, 0, isImplicit, littleEndian, charsets, opts, ctx)
			return seq, offset + end, err
		}
		pos += 8 + int64(itemLen)
	}
	chunk, err := v.bytes(offset, pos-offset)
	if err != nil {
		return nil, pos, err
	}
	seq, end, err := readSequenceItems(chunk, 0, isImplicit, littleEndian, charsets, opts, ctx)
	return seq, offset + end, err
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
	ctx context.Context,
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
		sub.Logger = opts.Logger
		sub.OnDiagnostic = opts.OnDiagnostic
	}
	rest, err := readBytes(ctx, inflated, filename, modTime, sub)
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
