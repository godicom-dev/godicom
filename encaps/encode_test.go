package encaps_test

import (
	"bytes"
	"testing"

	"github.com/godicom-dev/godicom/encaps"
)

func TestEncapsulate_roundtrip_singleFrame(t *testing.T) {
	frame := []byte{0x01, 0x02, 0x03} // odd → padded
	enc, err := encaps.Encapsulate([][]byte{frame}, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := encaps.GenerateFrames(enc, encaps.FramesOptions{LittleEndian: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames=%d", len(frames))
	}
	want := []byte{0x01, 0x02, 0x03, 0x00}
	if !bytes.Equal(frames[0], want) {
		t.Fatalf("got %v want %v", frames[0], want)
	}
}

func TestEncapsulate_multiFrame_BOT(t *testing.T) {
	framesIn := [][]byte{
		{0xAA, 0xBB},
		{0xCC, 0xDD, 0xEE, 0xFF},
	}
	enc, err := encaps.Encapsulate(framesIn, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := encaps.GenerateFrames(enc, encaps.FramesOptions{
		NumberOfFrames: 2,
		LittleEndian:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("frames=%d", len(frames))
	}
	if !bytes.Equal(frames[0], framesIn[0]) || !bytes.Equal(frames[1], framesIn[1]) {
		t.Fatalf("got %v", frames)
	}
}

func TestEncapsulate_emptyBOT(t *testing.T) {
	enc, err := encaps.Encapsulate([][]byte{{1, 2, 3, 4}}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := encaps.GenerateFrames(enc, encaps.FramesOptions{LittleEndian: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || !bytes.Equal(frames[0], []byte{1, 2, 3, 4}) {
		t.Fatalf("got %v", frames)
	}
}

func TestEncapsulateExtended_roundtrip(t *testing.T) {
	framesIn := [][]byte{
		{1, 2, 3},
		{4, 5, 6, 7},
	}
	pd, offsets, lengths, err := encaps.EncapsulateExtended(framesIn)
	if err != nil {
		t.Fatal(err)
	}
	if len(offsets) != 16 || len(lengths) != 16 {
		t.Fatalf("offset/length sizes %d %d", len(offsets), len(lengths))
	}
	eot := &encaps.ExtendedOffsetTable{
		Offsets: []uint64{0, 0},
		Lengths: []uint64{0, 0},
	}
	for i := 0; i < 2; i++ {
		eot.Offsets[i] = uint64(offsets[i*8]) | uint64(offsets[i*8+1])<<8 | uint64(offsets[i*8+2])<<16 | uint64(offsets[i*8+3])<<24 |
			uint64(offsets[i*8+4])<<32 | uint64(offsets[i*8+5])<<40 | uint64(offsets[i*8+6])<<48 | uint64(offsets[i*8+7])<<56
		eot.Lengths[i] = uint64(lengths[i*8]) | uint64(lengths[i*8+1])<<8 | uint64(lengths[i*8+2])<<16 | uint64(lengths[i*8+3])<<24 |
			uint64(lengths[i*8+4])<<32 | uint64(lengths[i*8+5])<<40 | uint64(lengths[i*8+6])<<48 | uint64(lengths[i*8+7])<<56
	}
	frames, err := encaps.GenerateFrames(pd, encaps.FramesOptions{
		NumberOfFrames:  2,
		LittleEndian:    true,
		ExtendedOffsets: eot,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Odd frame padded on encapsulate
	want0 := []byte{1, 2, 3, 0}
	want1 := []byte{4, 5, 6, 7}
	if !bytes.Equal(frames[0], want0) || !bytes.Equal(frames[1], want1) {
		t.Fatalf("got %v %v", frames[0], frames[1])
	}
}

func TestFragmentFrame_tooMany(t *testing.T) {
	_, err := encaps.FragmentFrame([]byte{1, 2}, 3)
	if err == nil {
		t.Fatal("expected error")
	}
}
