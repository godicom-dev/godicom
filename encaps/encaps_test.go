package encaps_test

import (
	"bytes"
	"testing"

	"github.com/godicom-dev/godicom/encaps"
)

// itemHeader builds a little-endian DICOM item tag (FFFE,E000) plus length.
func itemHeader(length uint32) []byte {
	return []byte{
		0xFE, 0xFF, 0x00, 0xE0,
		byte(length), byte(length >> 8), byte(length >> 16), byte(length >> 24),
	}
}

func TestParseBasicOffsets_zeroLength(t *testing.T) {
	stream := itemHeader(0)
	offsets, rest, err := encaps.ParseBasicOffsets(stream, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(offsets) != 0 {
		t.Fatalf("offsets = %v, want empty", offsets)
	}
	if len(rest) != 0 {
		t.Fatalf("rest len = %d, want 0", len(rest))
	}
}

func TestGenerateFrames_emptyBOT_singleFragment(t *testing.T) {
	stream := itemHeader(0)
	stream = append(stream, itemHeader(4)...)
	stream = append(stream, 0x01, 0x00, 0x00, 0x00)
	frames, err := encaps.GenerateFrames(stream, encaps.FramesOptions{LittleEndian: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	want := []byte{0x01, 0x00, 0x00, 0x00}
	if !bytes.Equal(frames[0], want) {
		t.Fatalf("frame = %v, want %v", frames[0], want)
	}
}

func TestGenerateFrames_emptyBOT_tripleFragmentSingleFrame(t *testing.T) {
	stream := itemHeader(0)
	stream = append(stream, itemHeader(4)...)
	stream = append(stream, 0x01, 0x00, 0x00, 0x00)
	stream = append(stream, itemHeader(4)...)
	stream = append(stream, 0x02, 0x00, 0x00, 0x00)
	stream = append(stream, itemHeader(4)...)
	stream = append(stream, 0x03, 0x00, 0x00, 0x00)
	frames, err := encaps.GenerateFrames(stream, encaps.FramesOptions{
		NumberOfFrames: 1,
		LittleEndian:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00}
	if len(frames) != 1 || !bytes.Equal(frames[0], want) {
		t.Fatalf("frame = %v, want %v", frames[0], want)
	}
}

func TestGenerateFrames_BOT_singleFragment(t *testing.T) {
	stream := itemHeader(4)
	stream = append(stream, 0x00, 0x00, 0x00, 0x00)
	stream = append(stream, itemHeader(4)...)
	stream = append(stream, 0x01, 0x00, 0x00, 0x00)
	frames, err := encaps.GenerateFrames(stream, encaps.FramesOptions{LittleEndian: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || !bytes.Equal(frames[0], []byte{0x01, 0x00, 0x00, 0x00}) {
		t.Fatalf("got %v", frames[0])
	}
}

func TestGenerateFrames_oneFragmentPerFrame(t *testing.T) {
	stream := itemHeader(0)
	stream = append(stream, itemHeader(2)...)
	stream = append(stream, 0xAA, 0xBB)
	stream = append(stream, itemHeader(2)...)
	stream = append(stream, 0xCC, 0xDD)
	frames, err := encaps.GenerateFrames(stream, encaps.FramesOptions{
		NumberOfFrames: 2,
		LittleEndian:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	if !bytes.Equal(frames[0], []byte{0xAA, 0xBB}) || !bytes.Equal(frames[1], []byte{0xCC, 0xDD}) {
		t.Fatalf("frames = %x %x", frames[0], frames[1])
	}
}
