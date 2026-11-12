// Package encaps parses DICOM encapsulated (compressed) pixel data.
// Behaviour aligns with pydicom.encaps.
package encaps

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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

// ParseBasicOffsets reads the Basic Offset Table at the start of encapsulated pixel data.
// The buffer is consumed through the end of the BOT item; remaining data begins at the first fragment item tag.
func ParseBasicOffsets(buf []byte, littleEndian bool) (offsets []uint32, rest []byte, err error) {
	if !littleEndian {
		return nil, nil, errors.New("encaps: only little endian encapsulated pixel data is supported")
	}
	offsets, firstFragment, err := parseBasicOffsetTable(sliceReader{buf: buf})
	if err != nil {
		return nil, nil, err
	}
	return offsets, buf[firstFragment:], nil
}

// GenerateFragmentedFrames splits encapsulated pixel data into per-frame fragment groups.
// The fragments alias pixelData; nothing is copied.
func GenerateFragmentedFrames(pixelData []byte, opts FramesOptions) ([][][]byte, error) {
	groups, err := splitFrames(sliceReader{buf: pixelData}, opts)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, nil
	}
	out := make([][][]byte, len(groups))
	for i, group := range groups {
		fragments := make([][]byte, len(group))
		for j, r := range group {
			fragments[j] = pixelData[r.start : r.start+r.length]
		}
		out[i] = fragments
	}
	return out, nil
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
	return GetFrameFrom(bytes.NewReader(pixelData), int64(len(pixelData)), index, opts)
}

// GetFrameFrom returns the encoded data for the frame at index, reading from src
// only the bytes that frame occupies.
//
// src holds the encapsulated pixel data of one element: open a file with
// os.Open and describe the element's value with the size and the offset,
//
//	section := io.NewSectionReader(f, valueOffset, valueLength)
//	frame, err := encaps.GetFrameFrom(section, valueLength, 8, opts)
//
// Frame boundaries come from the Basic or Extended Offset Table when one is
// present, so locating a frame costs the item headers and nothing more. With no
// offset table the boundaries are the JPEG End-of-Image markers, and finding
// them costs the last ten bytes of each fragment. In neither case is a fragment
// this call does not return read.
//
// The frames GetFrameFrom produces are the ones GetFrame produces, byte for
// byte; the difference is that GetFrame already has the whole element in memory
// and this does not.
func GetFrameFrom(src io.ReaderAt, size int64, index int, opts FramesOptions) ([]byte, error) {
	if src == nil {
		return nil, errors.New("encaps: nil source")
	}
	r := readerAtReader{src: src, bytes: size}
	groups, err := splitFrames(r, opts)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(groups) {
		return nil, fmt.Errorf("encaps: frame index %d out of range (have %d frames)", index, len(groups))
	}

	group := groups[index]
	total := int64(0)
	for _, fragment := range group {
		total += fragment.length
	}
	out := make([]byte, 0, total)
	for _, fragment := range group {
		b, err := r.at(fragment.start, fragment.length)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

// CountFragments returns the number of fragment items in encapsulated pixel data
// starting at the first fragment item (after the Basic Offset Table, if any).
func CountFragments(fragmentData []byte, littleEndian bool) (int, error) {
	if !littleEndian {
		return 0, errors.New("encaps: only little endian encapsulated pixel data is supported")
	}
	return countFragments(fragmentData, littleEndian)
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
