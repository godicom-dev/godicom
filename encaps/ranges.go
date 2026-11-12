package encaps

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// A fragmentRange locates one fragment's value inside encapsulated pixel data.
// start is measured from the first byte of the element's value -- the item tag
// of the Basic Offset Table -- and length is the fragment's value length, so the
// bytes are [start, start+length).
type fragmentRange struct {
	start  int64
	length int64
}

// A rangeReader is all the splitter needs from its input: how long it is, and
// how to read a slice of it. The []byte entry points and the io.ReaderAt ones
// both go through this, so one implementation decides where frames begin and the
// two cannot disagree about a file.
type rangeReader interface {
	size() int64
	at(off, n int64) ([]byte, error)
}

// sliceReader serves ranges out of a buffer that is already in memory. The
// bytes it returns alias that buffer, which is what keeps GenerateFrames
// zero-copy.
type sliceReader struct{ buf []byte }

func (s sliceReader) size() int64 { return int64(len(s.buf)) }

func (s sliceReader) at(off, n int64) ([]byte, error) {
	if off < 0 || n < 0 || off+n > int64(len(s.buf)) {
		return nil, io.ErrUnexpectedEOF
	}
	return s.buf[off : off+n], nil
}

// readerAtReader serves ranges out of a source that is not in memory, reading
// only what the caller asks for. This is what makes GetFrameFrom cheap on a
// multi-gigabyte file: the headers get read, the values that are not wanted do
// not.
type readerAtReader struct {
	src   io.ReaderAt
	bytes int64
}

func (r readerAtReader) size() int64 { return r.bytes }

func (r readerAtReader) at(off, n int64) ([]byte, error) {
	if off < 0 || n < 0 || off+n > r.bytes {
		return nil, io.ErrUnexpectedEOF
	}
	if n == 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(io.NewSectionReader(r.src, off, n), buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// parseBasicOffsetTable reads the Basic Offset Table and returns its offsets
// plus the offset of the first fragment item, which is where the offsets are
// measured from.
func parseBasicOffsetTable(r rangeReader) (offsets []uint32, firstFragment int64, err error) {
	// Encapsulated pixel data is little endian whatever the transfer syntax says
	// about the rest of the dataset (PS3.5 A.4), which is why ParseBasicOffsets
	// refuses any other byte order before this is reached.
	order := binary.LittleEndian
	size := r.size()
	if size < 8 {
		return nil, 0, errors.New("encaps: buffer too short for Basic Offset Table item")
	}
	head, err := r.at(0, 8)
	if err != nil {
		return nil, 0, err
	}
	tag := readTag(head, 0, order)
	if tag != itemTag {
		return nil, 0, fmt.Errorf("encaps: expected item tag (FFFE,E000), got %08x", tag)
	}
	length := int64(order.Uint32(head[4:8]))
	if length%4 != 0 {
		return nil, 0, errors.New("encaps: Basic Offset Table length is not a multiple of 4")
	}
	headerEnd := 8 + length
	if size < headerEnd {
		return nil, 0, errors.New("encaps: truncated Basic Offset Table")
	}
	if length == 0 {
		return nil, headerEnd, nil
	}
	raw, err := r.at(8, length)
	if err != nil {
		return nil, 0, err
	}
	offsets = make([]uint32, length/4)
	for i := range offsets {
		offsets[i] = order.Uint32(raw[i*4 : i*4+4])
	}
	return offsets, headerEnd, nil
}

// fragmentRanges walks the fragment items from firstFragment to the Sequence
// Delimiter, reading the 8-byte item headers and nothing else.
func fragmentRanges(r rangeReader, firstFragment int64) ([]fragmentRange, error) {
	order := binary.LittleEndian
	size := r.size()
	var out []fragmentRange
	pos := firstFragment
	for pos+8 <= size {
		head, err := r.at(pos, 8)
		if err != nil {
			return nil, errors.New("encaps: truncated fragment item header")
		}
		tag := readTag(head, 0, order)
		if tag == itemTag {
			length := int64(order.Uint32(head[4:8]))
			if length == int64(undefinedLen) {
				return nil, fmt.Errorf("encaps: undefined item length at offset %d", pos-firstFragment+4)
			}
			start := pos + 8
			if start+length > size {
				return nil, errors.New("encaps: truncated fragment data")
			}
			out = append(out, fragmentRange{start: start, length: length})
			pos = start + length
			continue
		}
		if tag == seqDelimTag {
			break
		}
		return nil, fmt.Errorf("encaps: unexpected tag %08x at offset %d", tag, pos-firstFragment)
	}
	return out, nil
}

// splitFrames decides which fragments belong to which frame, reading only
// headers and, when frames have to be delimited by their EOI marker, the last
// ten bytes of a fragment.
func splitFrames(r rangeReader, opts FramesOptions) ([][]fragmentRange, error) {
	offsets, firstFragment, err := parseBasicOffsetTable(r)
	if err != nil {
		return nil, err
	}

	if opts.ExtendedOffsets != nil && len(opts.ExtendedOffsets.Offsets) > 0 {
		return framesFromExtendedRanges(r, firstFragment, opts.ExtendedOffsets)
	}

	fragments, err := fragmentRanges(r, firstFragment)
	if err != nil {
		return nil, err
	}
	if len(offsets) > 0 {
		return framesFromBasicRanges(fragments, offsets), nil
	}
	return framesWithoutOffsetTableRanges(r, fragments, opts)
}

func framesFromBasicRanges(fragments []fragmentRange, basicOffsets []uint32) [][]fragmentRange {
	if len(fragments) == 0 {
		return nil
	}
	finalIndex := len(basicOffsets) - 1
	var out [][]fragmentRange
	var frame []fragmentRange
	currentIndex := 0
	currentOffset := uint32(0)

	for _, fragment := range fragments {
		if currentIndex == finalIndex {
			frame = append(frame, fragment)
			currentOffset += uint32(fragment.length + 8)
			continue
		}
		nextBoundary := basicOffsets[currentIndex+1]
		if currentOffset < nextBoundary {
			frame = append(frame, fragment)
		} else {
			out = append(out, frame)
			currentIndex++
			frame = []fragmentRange{fragment}
		}
		currentOffset += uint32(fragment.length + 8)
	}
	if len(frame) > 0 {
		out = append(out, frame)
	}
	return out
}

func framesWithoutOffsetTableRanges(r rangeReader, fragments []fragmentRange, opts FramesOptions) ([][]fragmentRange, error) {
	nrFragments := len(fragments)
	if nrFragments == 1 {
		return [][]fragmentRange{fragments}, nil
	}

	nrFrames := opts.NumberOfFrames
	if nrFrames <= 0 {
		return nil, errors.New(
			"encaps: number of frames required when Basic and Extended Offset Tables are empty",
		)
	}

	if nrFragments == nrFrames {
		out := make([][]fragmentRange, nrFrames)
		for i, f := range fragments {
			out[i] = []fragmentRange{f}
		}
		return out, nil
	}

	if nrFrames == 1 {
		return [][]fragmentRange{fragments}, nil
	}

	if nrFragments > nrFrames {
		return framesByEOIMarkerRanges(r, fragments)
	}

	return nil, errors.New(
		"encaps: fewer fragments than frames; dataset may be corrupt or NumberOfFrames incorrect",
	)
}

// framesByEOIMarkerRanges groups fragments into frames at the JPEG
// End-of-Image marker, reading only the ten bytes a marker can hide in.
func framesByEOIMarkerRanges(r rangeReader, fragments []fragmentRange) ([][]fragmentRange, error) {
	var out [][]fragmentRange
	var frame []fragmentRange
	for _, fragment := range fragments {
		frame = append(frame, fragment)
		hasMarker, err := hasEOIMarkerAt(r, fragment)
		if err != nil {
			return nil, err
		}
		if hasMarker {
			out = append(out, frame)
			frame = nil
		}
	}
	if len(frame) > 0 {
		out = append(out, frame)
	}
	return out, nil
}

func hasEOIMarkerAt(r rangeReader, fragment fragmentRange) (bool, error) {
	n := fragment.length
	if n < 2 {
		return false, nil
	}
	window := int64(10)
	if window > n {
		window = n
	}
	tail, err := r.at(fragment.start+n-window, window)
	if err != nil {
		return false, err
	}
	return hasEOIMarker(tail), nil
}

func framesFromExtendedRanges(r rangeReader, firstFragment int64, eot *ExtendedOffsetTable) ([][]fragmentRange, error) {
	if len(eot.Offsets) != len(eot.Lengths) {
		return nil, errors.New("encaps: extended offset table lengths mismatch")
	}
	out := make([][]fragmentRange, len(eot.Offsets))
	for i, off := range eot.Offsets {
		start := firstFragment + int64(off) + 8
		end := start + int64(eot.Lengths[i])
		if start < 0 || end > r.size() {
			return nil, fmt.Errorf("encaps: extended offset %d out of range", i)
		}
		out[i] = []fragmentRange{{start: start, length: int64(eot.Lengths[i])}}
	}
	return out, nil
}
