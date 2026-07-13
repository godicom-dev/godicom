package encaps

import (
	"encoding/binary"
	"fmt"
)

// FragmentFrame splits frame into nrFragments even-sized pieces.
// The last fragment is padded with 0x00 when needed. Mirrors pydicom fragment_frame.
func FragmentFrame(frame []byte, nrFragments int) ([][]byte, error) {
	if nrFragments < 1 {
		return nil, fmt.Errorf("encaps: FragmentsPerFrame must be >= 1")
	}
	frameLen := len(frame)
	if float64(nrFragments) > float64(frameLen+1)/2.0 {
		return nil, fmt.Errorf("encaps: too many fragments requested (minimum fragment size is 2 bytes)")
	}
	length := frameLen / nrFragments
	if length%2 != 0 {
		length++
	}
	out := make([][]byte, 0, nrFragments)
	for offset := 0; offset < length*(nrFragments-1); offset += length {
		frag := make([]byte, length)
		copy(frag, frame[offset:offset+length])
		out = append(out, frag)
	}
	offset := length * (nrFragments - 1)
	frag := append([]byte(nil), frame[offset:]...)
	if (frameLen-offset)%2 != 0 {
		frag = append(frag, 0x00)
	}
	out = append(out, frag)
	return out, nil
}

// ItemizeFragment wraps a fragment as a DICOM Item (FFFE,E000) + length + data.
func ItemizeFragment(fragment []byte) []byte {
	out := make([]byte, 8+len(fragment))
	out[0], out[1], out[2], out[3] = 0xFE, 0xFF, 0x00, 0xE0
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(fragment)))
	copy(out[8:], fragment)
	return out
}

// Encapsulate returns encapsulated pixel data for compressed frames.
// Mirrors pydicom.encaps.encapsulate. Does not append the sequence delimiter;
// writers emit that for undefined-length OB.
//
// fragmentsPerFrame defaults to 1 when <= 0.
func Encapsulate(frames [][]byte, fragmentsPerFrame int, hasBOT bool) ([]byte, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("encaps: no frames to encapsulate")
	}
	if fragmentsPerFrame <= 0 {
		fragmentsPerFrame = 1
	}
	nrFrames := len(frames)

	if hasBOT {
		total := (nrFrames - 1) * 8
		for _, f := range frames[:len(frames)-1] {
			total += len(f)
		}
		if total > (1<<32)-1 {
			return nil, fmt.Errorf("encaps: total encapsulated length %d exceeds Basic Offset Table maximum; use EncapsulateExtended", total)
		}
	}

	var out []byte
	out = append(out, 0xFE, 0xFF, 0x00, 0xE0)
	if hasBOT {
		lenBuf := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBuf, uint32(4*nrFrames))
		out = append(out, lenBuf...)
		for i := 0; i < nrFrames; i++ {
			out = append(out, 0xFF, 0xFF, 0xFF, 0xFF)
		}
	} else {
		out = append(out, 0, 0, 0, 0)
	}

	botOffsets := make([]uint32, 0, nrFrames+1)
	botOffsets = append(botOffsets, 0)
	for i, frame := range frames {
		frags, err := FragmentFrame(frame, fragmentsPerFrame)
		if err != nil {
			return nil, err
		}
		itemisedLen := 0
		for _, frag := range frags {
			item := ItemizeFragment(frag)
			itemisedLen += len(item)
			out = append(out, item...)
		}
		botOffsets = append(botOffsets, botOffsets[i]+uint32(itemisedLen))
	}

	if hasBOT {
		for i := 0; i < nrFrames; i++ {
			off := 8 + i*4
			binary.LittleEndian.PutUint32(out[off:off+4], botOffsets[i])
		}
	}
	return out, nil
}

// EncapsulateExtended returns encapsulated frames (empty BOT) plus Extended Offset
// Table and Lengths element values. Mirrors pydicom.encaps.encapsulate_extended.
func EncapsulateExtended(frames [][]byte) (pixelData, offsets, lengths []byte, err error) {
	if len(frames) == 0 {
		return nil, nil, nil, fmt.Errorf("encaps: no frames to encapsulate")
	}
	nrFrames := len(frames)
	frameLengths := make([]uint64, nrFrames)
	for i, f := range frames {
		n := uint64(len(f))
		if n%2 != 0 {
			n++
		}
		frameLengths[i] = n
	}
	frameOffsets := make([]uint64, nrFrames)
	for i := 1; i < nrFrames; i++ {
		frameOffsets[i] = frameOffsets[i-1] + frameLengths[i-1] + 8
	}
	offsets = make([]byte, nrFrames*8)
	lengths = make([]byte, nrFrames*8)
	for i := 0; i < nrFrames; i++ {
		binary.LittleEndian.PutUint64(offsets[i*8:], frameOffsets[i])
		binary.LittleEndian.PutUint64(lengths[i*8:], frameLengths[i])
	}
	pixelData, err = Encapsulate(frames, 1, false)
	return pixelData, offsets, lengths, err
}
