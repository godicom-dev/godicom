package encaps_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom/encaps"
)

func TestParseBasicOffsets_badTag(t *testing.T) {
	// pydicom.tests.test_encaps.TestParseBasicOffsets.test_bad_tag
	stream := []byte{0xFE, 0xFF, 0x00, 0xE1, 0x08, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	_, _, err := encaps.ParseBasicOffsets(stream, true)
	if err == nil {
		t.Fatal("expected error for bad BOT tag")
	}
	if !strings.Contains(err.Error(), "FFFE,E000") && !strings.Contains(err.Error(), "item tag") {
		t.Fatalf("error = %q, want BOT tag message", err.Error())
	}
}

func TestParseBasicOffsets_truncated(t *testing.T) {
	_, _, err := encaps.ParseBasicOffsets([]byte{0xFE, 0xFF, 0x00, 0xE0}, true)
	if err == nil {
		t.Fatal("expected error for truncated BOT header")
	}
}

func TestCountFragments_singleFragment(t *testing.T) {
	// pydicom.tests.test_encaps.TestGetNrFragments.test_single_fragment_no_delimiter
	stream := itemHeader(4)
	stream = append(stream, 0x01, 0x00, 0x00, 0x00)
	n, err := encaps.CountFragments(stream, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("fragments = %d, want 1", n)
	}
}

func TestCountFragments_multiFragments(t *testing.T) {
	// pydicom.tests.test_encaps.TestGetNrFragments.test_multi_fragments_no_delimiter
	stream := itemHeader(4)
	stream = append(stream, 0x01, 0x00, 0x00, 0x00)
	stream = append(stream, itemHeader(6)...)
	stream = append(stream, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06)
	n, err := encaps.CountFragments(stream, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("fragments = %d, want 2", n)
	}
}

func TestCountFragments_stopsAtSequenceDelimiter(t *testing.T) {
	// pydicom.tests.test_encaps.TestGetNrFragments.test_item_sequence_delimiter
	stream := itemHeader(4)
	stream = append(stream, 0x01, 0x00, 0x00, 0x00)
	stream = append(stream, 0xFE, 0xFF, 0xDD, 0xE0, 0x00, 0x00, 0x00, 0x00)
	stream = append(stream, itemHeader(4)...)
	stream = append(stream, 0x02, 0x00, 0x00, 0x00)
	n, err := encaps.CountFragments(stream, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("fragments = %d, want 1 (stop at delimiter)", n)
	}
}

func TestCountFragments_undefinedLength(t *testing.T) {
	// pydicom.tests.test_encaps.TestGetNrFragments.test_item_undefined_length
	stream := []byte{0xFE, 0xFF, 0x00, 0xE0, 0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x01}
	_, err := encaps.CountFragments(stream, true)
	if err == nil {
		t.Fatal("expected error for undefined fragment length")
	}
}

func TestCountFragments_badTag(t *testing.T) {
	// pydicom.tests.test_encaps.TestGetNrFragments.test_item_bad_tag
	stream := itemHeader(4)
	stream = append(stream, 0x01, 0x00, 0x00, 0x00)
	stream = append(stream, 0x10, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00)
	_, err := encaps.CountFragments(stream, true)
	if err == nil {
		t.Fatal("expected error for unexpected tag in fragment stream")
	}
}

func TestCountFragments_notLittleEndian(t *testing.T) {
	stream := itemHeader(4)
	_, err := encaps.CountFragments(stream, false)
	if err == nil {
		t.Fatal("expected error for big endian fragment stream")
	}
}

func TestGetFrame_emptyBOT_singleFragment(t *testing.T) {
	// pydicom.tests.test_encaps.TestGetFrame.test_empty_bot_single_fragment
	stream := itemHeader(0)
	stream = append(stream, itemHeader(4)...)
	stream = append(stream, 0x01, 0x00, 0x00, 0x00)
	got, err := encaps.GetFrame(stream, 0, encaps.FramesOptions{LittleEndian: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{0x01, 0x00, 0x00, 0x00}) {
		t.Fatalf("frame = %v", got)
	}
}

func TestGetFrame_emptyBOT_tripleFragmentSingleFrame(t *testing.T) {
	// pydicom.tests.test_encaps.TestGetFrame.test_empty_bot_triple_fragment_single_frame
	stream := itemHeader(0)
	for _, b := range []byte{0x01, 0x02, 0x03} {
		stream = append(stream, itemHeader(4)...)
		stream = append(stream, b, 0x00, 0x00, 0x00)
	}
	want := []byte{0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00}
	got, err := encaps.GetFrame(stream, 0, encaps.FramesOptions{
		NumberOfFrames: 1,
		LittleEndian:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("frame = %v, want %v", got, want)
	}
}

func TestGetFrame_BOT_multiFrame(t *testing.T) {
	// pydicom.tests.test_encaps.TestGetFrame BOT multi-frame access
	stream := itemHeader(12)
	stream = append(stream,
		0x00, 0x00, 0x00, 0x00,
		0x0C, 0x00, 0x00, 0x00,
		0x18, 0x00, 0x00, 0x00,
	)
	for _, b := range []byte{0x01, 0x02, 0x03} {
		stream = append(stream, itemHeader(4)...)
		stream = append(stream, b, 0x00, 0x00, 0x00)
	}
	opts := encaps.FramesOptions{LittleEndian: true}
	for i, wantByte := range []byte{0x01, 0x02, 0x03} {
		got, err := encaps.GetFrame(stream, i, opts)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if got[0] != wantByte {
			t.Fatalf("frame[%d][0] = %02x, want %02x", i, got[0], wantByte)
		}
	}
}

func TestGetFrame_EOIMarker_perFrame(t *testing.T) {
	// pydicom.tests.test_encaps.TestGetFrame.test_empty_bot_multi_fragments_per_frame
	stream := itemHeader(0)
	parts := [][]byte{
		{0x01, 0x00, 0x00, 0x00},
		{0x01, 0xFF, 0xD9, 0x00},
		{0x01, 0x00, 0xFF, 0xD9},
		{0x01, 0xFF, 0xD9, 0x00},
		{0x01, 0x00, 0x00, 0x00},
		{0x01, 0xFF, 0xD9, 0x00},
	}
	for _, p := range parts {
		stream = append(stream, itemHeader(uint32(len(p)))...)
		stream = append(stream, p...)
	}
	opts := encaps.FramesOptions{NumberOfFrames: 3, LittleEndian: true}
	want := [][]byte{
		{0x01, 0x00, 0x00, 0x00, 0x01, 0xFF, 0xD9, 0x00},
		{0x01, 0x00, 0xFF, 0xD9},
		{0x01, 0xFF, 0xD9, 0x00},
	}
	for i, w := range want {
		got, err := encaps.GetFrame(stream, i, opts)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if !bytes.Equal(got, w) {
			t.Fatalf("frame[%d] = %v, want %v", i, got, w)
		}
	}
	_, err := encaps.GetFrame(stream, 4, opts)
	if err == nil {
		t.Fatal("expected error for frame index beyond EOI-split frames")
	}
}

func TestGenerateFrames_extendedOffsetTable_lengthMismatch(t *testing.T) {
	stream := itemHeader(4)
	stream = append(stream, 0x00, 0x00, 0x00, 0x00)
	stream = append(stream, itemHeader(4)...)
	stream = append(stream, 0x01, 0x00, 0x00, 0x00)
	_, err := encaps.GenerateFrames(stream, encaps.FramesOptions{
		LittleEndian: true,
		ExtendedOffsets: &encaps.ExtendedOffsetTable{
			Offsets: []uint64{0},
			Lengths: []uint64{4, 4},
		},
	})
	if err == nil {
		t.Fatal("expected error for offset/length table mismatch")
	}
}
