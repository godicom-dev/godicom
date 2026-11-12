package encaps

// Ports of pydicom/tests/test_encaps.py cases that the exported API can reach.
// pydicom repeats the same ~20 scenarios across six internal layers
// (decode_data_sequence, defragment_data, read_item, parse_fragments,
// generate_pixel_data_*); godicom exposes a single flat API, so each scenario is
// asserted once here against every entry point it applies to.
//
// Subtests carry the pydicom test name where the case is a direct port.

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestFragmentFrameCombinations ports pydicom TestFragmentFrame: every
// combination of even/odd frame length with even/odd fragment count.
func TestFragmentFrameCombinations(t *testing.T) {
	cases := []struct {
		name     string
		frameLen int
		nrFrags  int
		want     []int // fragment lengths, after even-length padding
	}{
		// pydicom: test_single_fragment_even_data
		{"single_fragment_even_data", 8, 1, []int{8}},
		// pydicom: test_single_fragment_odd_data -- odd frame grows by one pad byte
		{"single_fragment_odd_data", 9, 1, []int{10}},
		// pydicom: test_even_fragment_even_data
		{"even_fragment_even_data", 8, 2, []int{4, 4}},
		// pydicom: test_even_fragment_odd_data -- the tail fragment absorbs the pad
		{"even_fragment_odd_data", 9, 2, []int{4, 6}},
		// pydicom: test_odd_fragments_even_data
		{"odd_fragments_even_data", 12, 3, []int{4, 4, 4}},
		// pydicom: test_odd_fragments_odd_data
		{"odd_fragments_odd_data", 13, 3, []int{4, 4, 6}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			frame := make([]byte, c.frameLen)
			for i := range frame {
				frame[i] = byte(i + 1)
			}

			frags, err := FragmentFrame(frame, c.nrFrags)
			if err != nil {
				t.Fatalf("FragmentFrame() error = %v", err)
			}
			if len(frags) != len(c.want) {
				t.Fatalf("FragmentFrame() returned %d fragments, want %d", len(frags), len(c.want))
			}

			total := 0
			for i, frag := range frags {
				if len(frag) != c.want[i] {
					t.Errorf("fragment %d length = %d, want %d", i, len(frag), c.want[i])
				}
				if len(frag)%2 != 0 {
					t.Errorf("fragment %d has odd length %d", i, len(frag))
				}
				total += len(frag)
			}

			// Concatenating the fragments must reproduce the frame, with at most
			// the single pad byte appended.
			joined := bytes.Join(frags, nil)
			if !bytes.HasPrefix(joined, frame) {
				t.Fatalf("fragments do not start with the original frame")
			}
			if len(joined)-len(frame) > 1 {
				t.Fatalf("fragments add %d bytes of padding, want at most 1", len(joined)-len(frame))
			}
			if total%2 != 0 {
				t.Fatalf("total fragment payload %d is odd", total)
			}
		})
	}
}

// TestItemizeFragmentPorts covers pydicom TestEncapsulateFrame: an item header
// followed by the fragment bytes.
func TestItemizeFragmentPorts(t *testing.T) {
	t.Run("single_item", func(t *testing.T) {
		got := ItemizeFragment([]byte{0x01, 0x02, 0x03, 0x04})
		want := []byte{
			0xFE, 0xFF, 0x00, 0xE0, 0x04, 0x00, 0x00, 0x00,
			0x01, 0x02, 0x03, 0x04,
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("ItemizeFragment() = % x, want % x", got, want)
		}
	})

	t.Run("two_items", func(t *testing.T) {
		a := ItemizeFragment([]byte{0x01, 0x02})
		b := ItemizeFragment([]byte{0x03, 0x04})
		stream := join(a, b)

		count, err := CountFragments(stream, true)
		if err != nil {
			t.Fatalf("CountFragments() error = %v", err)
		}
		if count != 2 {
			t.Fatalf("CountFragments() = %d, want 2", count)
		}
	})

	t.Run("empty_fragment", func(t *testing.T) {
		got := ItemizeFragment(nil)
		want := []byte{0xFE, 0xFF, 0x00, 0xE0, 0x00, 0x00, 0x00, 0x00}
		if !bytes.Equal(got, want) {
			t.Fatalf("ItemizeFragment(nil) = % x, want % x", got, want)
		}
	})
}

// TestGenerateFragmentedFramesMarkerSplitting ports the "no offset table"
// marker-based splitting cases from pydicom TestGenerateFragmentedFrames.
func TestGenerateFragmentedFramesMarkerSplitting(t *testing.T) {
	// Each fragment is 4 bytes; a frame ends at the fragment containing 0xFFD9.
	mk := func(payloads ...[]byte) []byte {
		parts := [][]byte{botItem()}
		for _, p := range payloads {
			parts = append(parts, item(p))
		}
		return join(parts...)
	}
	// frag is one fragment payload written out byte for byte. 0xFF, 0xD9 inside
	// it is the JPEG End-of-Image marker, which is what delimits a frame when the
	// offset table is empty; the 0x00 after it is the even-length pad.
	frag := func(b ...byte) []byte { return b }

	t.Run("empty_bot_multi_fragments_per_frame", func(t *testing.T) {
		// pydicom: test_empty_bot_multi_fragments_per_frame -- 4 frames in 6
		// fragments, every frame ends on a marker.
		stream := mk(
			frag(0x01, 0x00, 0x00, 0x00),
			frag(0x01, 0xFF, 0xD9, 0x00),
			frag(0x01, 0x00, 0xFF, 0xD9),
			frag(0x01, 0xFF, 0xD9, 0x00),
			frag(0x01, 0x00, 0x00, 0x00),
			frag(0x01, 0xFF, 0xD9, 0x00),
		)

		frames, err := GenerateFragmentedFrames(stream, FramesOptions{NumberOfFrames: 4})
		if err != nil {
			t.Fatalf("GenerateFragmentedFrames() error = %v", err)
		}
		if len(frames) != 4 {
			t.Fatalf("got %d frames, want 4", len(frames))
		}
		wantFragCounts := []int{2, 1, 1, 2}
		for i, want := range wantFragCounts {
			if len(frames[i]) != want {
				t.Errorf("frame %d has %d fragments, want %d", i, len(frames[i]), want)
			}
		}
	})

	t.Run("empty_bot_no_marker", func(t *testing.T) {
		// pydicom: test_empty_bot_no_marker -- 4 frames requested, 6 fragments,
		// one frame lacks a marker.
		stream := mk(
			frag(0x01, 0x00, 0x00, 0x00),
			frag(0x01, 0xFF, 0xD9, 0x00),
			frag(0x01, 0x00, 0x00, 0x00),
			frag(0x01, 0xFF, 0xD9, 0x00),
			frag(0x01, 0xFF, 0xFF, 0xD9),
			frag(0x01, 0xFF, 0x00, 0x00),
		)

		frames, err := GenerateFragmentedFrames(stream, FramesOptions{NumberOfFrames: 4})
		if err != nil {
			t.Fatalf("GenerateFragmentedFrames() error = %v", err)
		}
		// pydicom warns here but still yields four frame groups; godicom's
		// marker scan (last 10 bytes) finds the 0xFFD9 in fragment 5 and the
		// trailing fragment joins it, so the count agrees.
		if len(frames) != 4 {
			t.Fatalf("got %d frames, want 4", len(frames))
		}
	})

	t.Run("empty_bot_missing_marker", func(t *testing.T) {
		// pydicom: test_empty_bot_missing_marker -- ends up with 3 frames
		// because two consecutive fragments carry no marker.
		stream := mk(
			frag(0x01, 0x00, 0x00, 0x00),
			frag(0x01, 0xFF, 0xD9, 0x00),
			frag(0x01, 0x00, 0x00, 0x00),
			frag(0x01, 0xFF, 0x00, 0x00),
			frag(0x01, 0xFF, 0xFF, 0xD9),
			frag(0x01, 0xFF, 0xD9, 0x00),
		)

		frames, err := GenerateFragmentedFrames(stream, FramesOptions{NumberOfFrames: 4})
		if err != nil {
			t.Fatalf("GenerateFragmentedFrames() error = %v", err)
		}
		if len(frames) != 3 {
			t.Fatalf("got %d frames, want 3 (pydicom yields 3 here)", len(frames))
		}
	})

	t.Run("empty_bot_single_fragment_per_frame", func(t *testing.T) {
		// pydicom: test_empty_bot_single_fragment_per_frame
		stream := mk(
			frag(0x01, 0x00, 0x00, 0x00),
			frag(0x02, 0x00, 0x00, 0x00),
			frag(0x03, 0x00, 0x00, 0x00),
		)

		frames, err := GenerateFrames(stream, FramesOptions{NumberOfFrames: 3})
		if err != nil {
			t.Fatalf("GenerateFrames() error = %v", err)
		}
		if len(frames) != 3 {
			t.Fatalf("got %d frames, want 3", len(frames))
		}
		for i, want := range [][]byte{
			{0x01, 0x00, 0x00, 0x00},
			{0x02, 0x00, 0x00, 0x00},
			{0x03, 0x00, 0x00, 0x00},
		} {
			if !bytes.Equal(frames[i], want) {
				t.Errorf("frame %d = % x, want % x", i, frames[i], want)
			}
		}
	})

	t.Run("empty_bot_multi_fragments_per_frame_excess_frames", func(t *testing.T) {
		// pydicom: test_empty_bot_multi_fragments_per_frame_excess_frames -- when
		// there is no offset table the markers win: more frames come out than
		// NumberOfFrames asked for.
		stream := mk(
			frag(0x01, 0x00, 0x00, 0x00),
			frag(0x01, 0xFF, 0xD9, 0x00),
			frag(0x02, 0x00, 0x00, 0x00),
			frag(0x02, 0xFF, 0xD9, 0x00),
		)

		frames, err := GenerateFragmentedFrames(stream, FramesOptions{NumberOfFrames: 3})
		if err != nil {
			t.Fatalf("GenerateFragmentedFrames() error = %v", err)
		}
		if len(frames) != 2 {
			t.Fatalf("got %d frames, want 2 (marker-limited, not NumberOfFrames)", len(frames))
		}
	})
}

// TestGenerateFragmentedFramesBOTTripleFragmentSingleFrame ports
// pydicom test_bot_triple_fragment_single_frame: one BOT entry covering three
// fragments that belong to a single frame.
func TestGenerateFragmentedFramesBOTTripleFragmentSingleFrame(t *testing.T) {
	stream := join(
		botItem(0),
		item([]byte{0x01, 0x02}),
		item([]byte{0x03, 0x04}),
		item([]byte{0x05, 0x06}),
	)

	frames, err := GenerateFragmentedFrames(stream, FramesOptions{})
	if err != nil {
		t.Fatalf("GenerateFragmentedFrames() error = %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	if len(frames[0]) != 3 {
		t.Fatalf("frame 0 has %d fragments, want 3", len(frames[0]))
	}

	joined, err := GenerateFrames(stream, FramesOptions{})
	if err != nil {
		t.Fatalf("GenerateFrames() error = %v", err)
	}
	if !bytes.Equal(joined[0], []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}) {
		t.Fatalf("GenerateFrames() = % x, want the three fragments concatenated", joined[0])
	}
}

// TestGenerateFragmentedFramesStopsAtSequenceDelimiter ports the
// test_item_sequence_delimiter cases: fragments after (FFFE,E0DD) are ignored.
func TestGenerateFragmentedFramesStopsAtSequenceDelimiter(t *testing.T) {
	t.Run("single_fragment_delimiter", func(t *testing.T) {
		stream := join(botItem(), item([]byte{0x01, 0x02}), seqDelimiter)

		frames, err := GenerateFragmentedFrames(stream, FramesOptions{})
		if err != nil {
			t.Fatalf("GenerateFragmentedFrames() error = %v", err)
		}
		if len(frames) != 1 || len(frames[0]) != 1 {
			t.Fatalf("got %d frame(s), want 1 frame with 1 fragment", len(frames))
		}
	})

	t.Run("multi_fragments_delimiter", func(t *testing.T) {
		// Two fragments before the delimiter form a single frame because there is
		// no offset table to split them.
		stream := join(
			botItem(),
			item([]byte{0x01, 0x02}),
			item([]byte{0x03, 0x04}),
			seqDelimiter,
		)

		frames, err := GenerateFragmentedFrames(stream, FramesOptions{NumberOfFrames: 1})
		if err != nil {
			t.Fatalf("GenerateFragmentedFrames() error = %v", err)
		}
		if len(frames) != 1 || len(frames[0]) != 2 {
			t.Fatalf("got %d frame(s), want 1 frame with 2 fragments", len(frames))
		}
	})

	t.Run("sequence_delimiter_zero_length", func(t *testing.T) {
		// pydicom: test_item_sequence_delimiter_zero_length -- a delimiter as the
		// very first item yields no fragments and no error.
		count, err := CountFragments(seqDelimiter, true)
		if err != nil {
			t.Fatalf("CountFragments() error = %v", err)
		}
		if count != 0 {
			t.Fatalf("CountFragments() = %d, want 0", count)
		}
	})
}

// TestGenerateFragmentedFramesRejectsItemInvalid ports pydicom
// test_item_invalid: a fragment whose declared length runs past the buffer.
func TestGenerateFragmentedFramesRejectsItemInvalid(t *testing.T) {
	bad := []byte{0xFE, 0xFF, 0x00, 0xE0, 0x08, 0x00, 0x00, 0x00, 0x01, 0x02}
	stream := join(botItem(), bad)

	if _, err := GenerateFragmentedFrames(stream, FramesOptions{NumberOfFrames: 1}); err == nil {
		t.Fatal("GenerateFragmentedFrames() accepted a truncated fragment")
	}
}

// TestGetFrameIndexOutOfRange ports the two pydicom "index greater than frames
// raises" cases.
func TestGetFrameIndexOutOfRange(t *testing.T) {
	t.Run("empty_bot_index_greater_than_frames_raises", func(t *testing.T) {
		// pydicom: test_empty_bot_index_greater_than_frames_raises
		stream := join(botItem(), item([]byte{0x01, 0x02}))

		if _, err := GetFrame(stream, 1, FramesOptions{NumberOfFrames: 1}); err == nil {
			t.Fatal("GetFrame(1) on a 1-frame stream returned no error")
		}
	})

	t.Run("empty_bot_index_greater_than_multi_frames_raises", func(t *testing.T) {
		// pydicom: test_empty_bot_index_greater_than_multi_frames_raises
		stream := join(
			botItem(),
			item([]byte{0x01, 0xFF, 0xD9}),
			item([]byte{0x02, 0xFF, 0xD9}),
		)

		if _, err := GetFrame(stream, 5, FramesOptions{NumberOfFrames: 2}); err == nil {
			t.Fatal("GetFrame(5) on a 2-frame stream returned no error")
		}
	})

	t.Run("negative_index_raises", func(t *testing.T) {
		stream := join(botItem(), item([]byte{0x01, 0x02}))

		if _, err := GetFrame(stream, -1, FramesOptions{}); err == nil {
			t.Fatal("GetFrame(-1) returned no error")
		}
	})
}

// TestEncapsulateThenParseRoundTrip exercises the writer against every reader
// entry point, including multi-fragment frames.
func TestEncapsulateThenParseRoundTrip(t *testing.T) {
	// Every frame ends with the JPEG EOI marker: without a Basic Offset Table the
	// reader has nothing else to split on.
	frames := [][]byte{
		{0x01, 0x02, 0x03, 0x04, 0xFF, 0xD9},
		{0x11, 0x12, 0x13, 0x14, 0xFF, 0xD9},
		{0x21, 0x22, 0x23, 0x24, 0xFF, 0xD9},
	}

	for _, fragsPerFrame := range []int{1, 2, 3} {
		for _, hasBOT := range []bool{true, false} {
			name := "frags=" + itoa(fragsPerFrame)
			if hasBOT {
				name += "_bot"
			} else {
				name += "_no_bot"
			}

			t.Run(name, func(t *testing.T) {
				stream, err := Encapsulate(frames, fragsPerFrame, hasBOT)
				if err != nil {
					t.Fatalf("Encapsulate() error = %v", err)
				}

				got, err := GenerateFrames(stream, FramesOptions{NumberOfFrames: len(frames)})
				if err != nil {
					t.Fatalf("GenerateFrames() error = %v", err)
				}
				if len(got) != len(frames) {
					t.Fatalf("got %d frames, want %d", len(got), len(frames))
				}
				for i := range frames {
					// Frames that were split may carry one pad byte.
					if !bytes.HasPrefix(got[i], frames[i]) {
						t.Errorf("frame %d = % x, want prefix % x", i, got[i], frames[i])
					}
					if len(got[i])-len(frames[i]) > 1 {
						t.Errorf("frame %d gained %d pad bytes, want at most 1", i, len(got[i])-len(frames[i]))
					}
				}

				// GetFrame must agree with GenerateFrames frame by frame.
				for i := range frames {
					one, err := GetFrame(stream, i, FramesOptions{NumberOfFrames: len(frames)})
					if err != nil {
						t.Fatalf("GetFrame(%d) error = %v", i, err)
					}
					if !bytes.Equal(one, got[i]) {
						t.Errorf("GetFrame(%d) disagrees with GenerateFrames", i)
					}
				}

				// CountFragments must see fragmentsPerFrame items per frame.
				_, rest, err := ParseBasicOffsets(stream, true)
				if err != nil {
					t.Fatalf("ParseBasicOffsets() error = %v", err)
				}
				count, err := CountFragments(rest, true)
				if err != nil {
					t.Fatalf("CountFragments() error = %v", err)
				}
				if want := len(frames) * fragsPerFrame; count != want {
					t.Errorf("CountFragments() = %d, want %d", count, want)
				}
			})
		}
	}
}

// TestEncapsulateBOTOffsetsAreExact checks the Basic Offset Table values against
// the byte offsets the fragments actually land on.
func TestEncapsulateBOTOffsetsAreExact(t *testing.T) {
	frames := [][]byte{
		{0x01, 0x02, 0x03, 0x04},
		{0x11, 0x12, 0x13, 0x14},
	}
	stream, err := Encapsulate(frames, 1, true)
	if err != nil {
		t.Fatalf("Encapsulate() error = %v", err)
	}

	offsets, rest, err := ParseBasicOffsets(stream, true)
	if err != nil {
		t.Fatalf("ParseBasicOffsets() error = %v", err)
	}
	if len(offsets) != 2 {
		t.Fatalf("got %d BOT entries, want 2", len(offsets))
	}
	if offsets[0] != 0 {
		t.Errorf("offsets[0] = %d, want 0", offsets[0])
	}
	// First frame is one item: 8-byte header + 4-byte payload.
	if want := uint32(12); offsets[1] != want {
		t.Errorf("offsets[1] = %d, want %d", offsets[1], want)
	}

	// Walking the offsets must land exactly on item headers.
	for i, off := range offsets {
		pos := int(off)
		if pos+8 > len(rest) {
			t.Fatalf("offset %d points past the fragment data", i)
		}
		tag := readTag(rest, pos, binary.LittleEndian)
		if tag != itemTag {
			t.Errorf("offset %d lands on tag %08x, want the item tag", i, tag)
		}
	}
}

// TestEncapsulateExtendedLengthsMatchFrames checks the (7FE0,0002) lengths the
// writer emits against the encoded frames.
func TestEncapsulateExtendedLengthsMatchFrames(t *testing.T) {
	frames := [][]byte{
		{0x01, 0x02, 0x03, 0x04},
		{0x11, 0x12, 0x13}, // odd: padded in the stream
	}
	_, offsets, lengths, err := EncapsulateExtended(frames)
	if err != nil {
		t.Fatalf("EncapsulateExtended() error = %v", err)
	}

	if len(offsets) != 16 || len(lengths) != 16 {
		t.Fatalf("got %d offset bytes and %d length bytes, want 16 and 16", len(offsets), len(lengths))
	}

	got := make([]uint64, 2)
	for i := 0; i < 2; i++ {
		got[i] = binary.LittleEndian.Uint64(lengths[i*8:])
	}
	if got[0] != 4 {
		t.Errorf("length[0] = %d, want 4", got[0])
	}
	// Odd frames are padded to an even length in the encapsulated stream.
	if got[1] != 4 {
		t.Errorf("length[1] = %d, want 4 (odd frame padded)", got[1])
	}

	off0 := binary.LittleEndian.Uint64(offsets[0:])
	off1 := binary.LittleEndian.Uint64(offsets[8:])
	if off0 != 0 {
		t.Errorf("offset[0] = %d, want 0", off0)
	}
	if want := uint64(4 + 8); off1 != want {
		t.Errorf("offset[1] = %d, want %d", off1, want)
	}
}

// TestGenerateFragmentedFramesExtendedTableNoBasicOffsets ports the eot cases:
// an extended offset table alone is enough to split frames.
func TestGenerateFragmentedFramesExtendedTableNoBasicOffsets(t *testing.T) {
	stream := join(
		botItem(),
		item([]byte{0x01, 0x02}),
		item([]byte{0x03, 0x04}),
	)
	eot := &ExtendedOffsetTable{
		Offsets: []uint64{0, 10},
		Lengths: []uint64{2, 2},
	}

	frames, err := GenerateFragmentedFrames(stream, FramesOptions{ExtendedOffsets: eot})
	if err != nil {
		t.Fatalf("GenerateFragmentedFrames() error = %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if !bytes.Equal(frames[0][0], []byte{0x01, 0x02}) {
		t.Errorf("frame 0 = % x, want 01 02", frames[0][0])
	}
	if !bytes.Equal(frames[1][0], []byte{0x03, 0x04}) {
		t.Errorf("frame 1 = % x, want 03 04", frames[1][0])
	}
}

// TestCountFragmentsEmptyPayload ports the zero-length fragment cases.
func TestCountFragmentsEmptyPayload(t *testing.T) {
	stream := join(item(nil), item(nil))

	count, err := CountFragments(stream, true)
	if err != nil {
		t.Fatalf("CountFragments() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("CountFragments() = %d, want 2", count)
	}
}

// itoa avoids pulling strconv into the assertion helpers above.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
