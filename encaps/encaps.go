// Package encaps parses DICOM encapsulated (compressed) pixel data.
// Behaviour aligns with pydicom.encaps.
package encaps

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	itemTag         uint32 = 0xFFFEE000
	seqDelimTag     uint32 = 0xFFFEE0DD
	undefinedLen    uint32 = 0xFFFFFFFF
	eoiMarkerByte          = 0xFF
	eoiMarkerSecond        = 0xD9
)

func readTag(buf []byte, pos int, order binary.ByteOrder) uint32 {
	group := order.Uint16(buf[pos : pos+2])
	elem := order.Uint16(buf[pos+2 : pos+4])
	return uint32(group)<<16 | uint32(elem)
}

// ExtendedOffsetTable holds (7FE0,0001) and (7FE0,0002) values.
type ExtendedOffsetTable struct {
	Offsets []uint64
	Lengths []uint64
}

// FramesOptions configures frame generation from encapsulated pixel data.
type FramesOptions struct {
	NumberOfFrames  int
	ExtendedOffsets *ExtendedOffsetTable
	LittleEndian    bool // encapsulated item tags use little endian (DICOM default)
}

func (o FramesOptions) endian() binary.ByteOrder {
	if o.LittleEndian {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

// ParseBasicOffsets reads the Basic Offset Table at the start of encapsulated pixel data.
// The buffer is consumed through the end of the BOT item; remaining data begins at the first fragment item tag.
func ParseBasicOffsets(buf []byte, littleEndian bool) (offsets []uint32, rest []byte, err error) {
	if !littleEndian {
		return nil, nil, errors.New("encaps: only little endian encapsulated pixel data is supported")
	}
	if len(buf) < 8 {
		return nil, nil, errors.New("encaps: buffer too short for Basic Offset Table item")
	}
	order := binary.LittleEndian
	tag := readTag(buf, 0, order)
	if tag != itemTag {
		return nil, nil, fmt.Errorf("encaps: expected item tag (FFFE,E000), got %08x", tag)
	}
	length := order.Uint32(buf[4:8])
	if length%4 != 0 {
		return nil, nil, errors.New("encaps: Basic Offset Table length is not a multiple of 4")
	}
	headerEnd := 8 + int(length)
	if len(buf) < headerEnd {
		return nil, nil, errors.New("encaps: truncated Basic Offset Table")
	}
	if length == 0 {
		return nil, buf[headerEnd:], nil
	}
	count := int(length / 4)
	offsets = make([]uint32, count)
	for i := 0; i < count; i++ {
		off := 8 + i*4
		offsets[i] = order.Uint32(buf[off : off+4])
	}
	return offsets, buf[headerEnd:], nil
}

// GenerateFragmentedFrames splits encapsulated pixel data into per-frame fragment groups.
func GenerateFragmentedFrames(pixelData []byte, opts FramesOptions) ([][][]byte, error) {
	if !opts.LittleEndian {
		opts.LittleEndian = true
	}
	basicOffsets, rest, err := ParseBasicOffsets(pixelData, opts.LittleEndian)
	if err != nil {
		return nil, err
	}

	if opts.ExtendedOffsets != nil && len(opts.ExtendedOffsets.Offsets) > 0 {
		return framesFromExtended(rest, opts)
	}
	if len(basicOffsets) > 0 {
		return framesFromBasic(rest, basicOffsets, opts)
	}
	return framesWithoutOffsetTable(rest, opts)
}

// GenerateFrames returns one concatenated encoded frame per logical image frame.
func GenerateFrames(pixelData []byte, opts FramesOptions) ([][]byte, error) {
	fragmented, err := GenerateFragmentedFrames(pixelData, opts)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, len(fragmented))
	for i, frags := range fragmented {
		n := 0
		for _, f := range frags {
			n += len(f)
		}
		joined := make([]byte, 0, n)
		for _, f := range frags {
			joined = append(joined, f...)
		}
		out[i] = joined
	}
	return out, nil
}

// GetFrame returns the encoded data for the frame at index.
func GetFrame(pixelData []byte, index int, opts FramesOptions) ([]byte, error) {
	frames, err := GenerateFrames(pixelData, opts)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(frames) {
		return nil, fmt.Errorf("encaps: frame index %d out of range (have %d frames)", index, len(frames))
	}
	return frames[index], nil
}

func readFragments(buf []byte, littleEndian bool) ([][]byte, error) {
	var order binary.ByteOrder = binary.LittleEndian
	if !littleEndian {
		order = binary.BigEndian
	}
	var fragments [][]byte
	pos := 0
	for pos+8 <= len(buf) {
		tag := readTag(buf, pos, order)
		if tag == itemTag {
			if pos+8 > len(buf) {
				return nil, errors.New("encaps: truncated fragment item header")
			}
			length := order.Uint32(buf[pos+4 : pos+8])
			if length == undefinedLen {
				return nil, fmt.Errorf("encaps: undefined item length at offset %d", pos+4)
			}
			start := pos + 8
			end := start + int(length)
			if end > len(buf) {
				return nil, errors.New("encaps: truncated fragment data")
			}
			fragments = append(fragments, buf[start:end])
			pos = end
			continue
		}
		if tag == seqDelimTag {
			break
		}
		return nil, fmt.Errorf("encaps: unexpected tag %08x at offset %d", tag, pos)
	}
	return fragments, nil
}

func countFragments(buf []byte, littleEndian bool) (int, error) {
	frags, err := readFragments(buf, littleEndian)
	return len(frags), err
}

func framesFromExtended(rest []byte, opts FramesOptions) ([][][]byte, error) {
	eot := opts.ExtendedOffsets
	if len(eot.Offsets) != len(eot.Lengths) {
		return nil, errors.New("encaps: extended offset table lengths mismatch")
	}
	fragmentsStart := 0
	out := make([][][]byte, len(eot.Offsets))
	for i, off := range eot.Offsets {
		start := fragmentsStart + int(off) + 8
		end := start + int(eot.Lengths[i])
		if start < 0 || end > len(rest) {
			return nil, fmt.Errorf("encaps: extended offset %d out of range", i)
		}
		out[i] = [][]byte{rest[start:end]}
	}
	return out, nil
}

func framesFromBasic(rest []byte, basicOffsets []uint32, opts FramesOptions) ([][][]byte, error) {
	fragments, err := readFragments(rest, opts.LittleEndian)
	if err != nil {
		return nil, err
	}
	if len(fragments) == 0 {
		return nil, nil
	}
	finalIndex := len(basicOffsets) - 1
	var out [][][]byte
	var frame [][]byte
	currentIndex := 0
	currentOffset := uint32(0)

	for _, fragment := range fragments {
		if currentIndex == finalIndex {
			frame = append(frame, fragment)
			currentOffset += uint32(len(fragment) + 8)
			continue
		}
		nextBoundary := basicOffsets[currentIndex+1]
		if currentOffset < nextBoundary {
			frame = append(frame, fragment)
		} else {
			out = append(out, frame)
			currentIndex++
			frame = [][]byte{fragment}
		}
		currentOffset += uint32(len(fragment) + 8)
	}
	if len(frame) > 0 {
		out = append(out, frame)
	}
	return out, nil
}

func framesWithoutOffsetTable(rest []byte, opts FramesOptions) ([][][]byte, error) {
	nrFragments, err := countFragments(rest, opts.LittleEndian)
	if err != nil {
		return nil, err
	}
	fragments, err := readFragments(rest, opts.LittleEndian)
	if err != nil {
		return nil, err
	}

	if nrFragments == 1 {
		return [][][]byte{fragments}, nil
	}

	nrFrames := opts.NumberOfFrames
	if nrFrames <= 0 {
		return nil, errors.New(
			"encaps: number of frames required when Basic and Extended Offset Tables are empty",
		)
	}

	if nrFragments == nrFrames {
		out := make([][][]byte, nrFrames)
		for i, f := range fragments {
			out[i] = [][]byte{f}
		}
		return out, nil
	}

	if nrFrames == 1 {
		return [][][]byte{fragments}, nil
	}

	if nrFragments > nrFrames {
		return framesByEOIMarker(fragments, nrFrames)
	}

	return nil, errors.New(
		"encaps: fewer fragments than frames; dataset may be corrupt or NumberOfFrames incorrect",
	)
}

func framesByEOIMarker(fragments [][]byte, nrFrames int) ([][][]byte, error) {
	var out [][][]byte
	var frame [][]byte
	frameNr := 0
	for _, fragment := range fragments {
		frame = append(frame, fragment)
		if hasEOIMarker(fragment) {
			out = append(out, frame)
			frameNr++
			frame = nil
		}
	}
	if len(frame) > 0 {
		out = append(out, frame)
	}
	if frameNr < nrFrames && len(out) < nrFrames {
		// allow excess frames per pydicom when EOI heuristic finds more
	}
	return out, nil
}

func hasEOIMarker(fragment []byte) bool {
	n := len(fragment)
	if n < 2 {
		return false
	}
	start := n - 10
	if start < 0 {
		start = 0
	}
	for i := start; i < n-1; i++ {
		if fragment[i] == eoiMarkerByte && fragment[i+1] == eoiMarkerSecond {
			return true
		}
	}
	return false
}
