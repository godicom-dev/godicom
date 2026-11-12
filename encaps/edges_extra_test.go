package encaps

// Internal-package tests: these drive the unexported helpers (readFragments,
// hasEOIMarker, framesFromBasic, framesFromExtended, framesWithoutOffsetTable)
// that the exported API only reaches through a specific malformed input. The
// expected behaviour follows pydicom's encaps.py, with the pydicom test names
// noted where a case is a direct port.

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

// item encodes data as a little-endian (FFFE,E000) item.
func item(data []byte) []byte {
	out := make([]byte, 8+len(data))
	out[0], out[1], out[2], out[3] = 0xFE, 0xFF, 0x00, 0xE0
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(data)))
	copy(out[8:], data)
	return out
}

// bigEndianItem encodes data as a big-endian (FFFE,E000) item: both the tag and
// the length use the same byte order, which is what readFragments reverses.
func bigEndianItem(data []byte) []byte {
	out := make([]byte, 8+len(data))
	out[0], out[1], out[2], out[3] = 0xFF, 0xFE, 0xE0, 0x00
	binary.BigEndian.PutUint32(out[4:8], uint32(len(data)))
	copy(out[8:], data)
	return out
}

// seqDelimiter is the (FFFE,E0DD) Sequence Delimitation Item with zero length.
var seqDelimiter = []byte{0xFE, 0xFF, 0xDD, 0xE0, 0x00, 0x00, 0x00, 0x00}

// botItem builds a Basic Offset Table item holding offsets.
func botItem(offsets ...uint32) []byte {
	data := make([]byte, 4*len(offsets))
	for i, off := range offsets {
		binary.LittleEndian.PutUint32(data[i*4:], off)
	}
	return item(data)
}

// join concatenates byte slices; the tests build encapsulated streams from parts.
func join(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestParseBasicOffsetsTruncatedTable(t *testing.T) {
	// Item declares 8 bytes of offsets but only 4 follow the header.
	buf := []byte{0xFE, 0xFF, 0x00, 0xE0, 0x08, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}

	offsets, rest, err := ParseBasicOffsets(buf, true)
	if err == nil || !strings.Contains(err.Error(), "truncated Basic Offset Table") {
		t.Fatalf("err = %v, want truncated Basic Offset Table", err)
	}
	if offsets != nil || rest != nil {
		t.Fatalf("offsets = %v, rest = %v, want both nil", offsets, rest)
	}
}

func TestReadFragmentsBigEndian(t *testing.T) {
	// readFragments is only reachable through the exported API with little endian
	// (GenerateFragmentedFrames forces it), so the big-endian branch is driven
	// directly. CountFragments rejects !littleEndian before it can get here.
	buf := join(
		bigEndianItem([]byte{0x01, 0x02}),
		bigEndianItem([]byte{0x03, 0x04}),
		// The delimiter's tag is byte-order dependent too: (FFFE,E0DD) big endian.
		[]byte{0xFF, 0xFE, 0xE0, 0xDD, 0x00, 0x00, 0x00, 0x00},
	)

	frags, err := readFragments(buf, false)
	if err != nil {
		t.Fatalf("readFragments: %v", err)
	}
	if len(frags) != 2 {
		t.Fatalf("got %d fragments, want 2", len(frags))
	}
	if got := frags[0]; len(got) != 2 || got[0] != 0x01 || got[1] != 0x02 {
		t.Errorf("fragments[0] = %x, want 0102", got)
	}
	if got := frags[1]; len(got) != 2 || got[0] != 0x03 || got[1] != 0x04 {
		t.Errorf("fragments[1] = %x, want 0304", got)
	}

	// A little-endian stream read as big endian decodes a different tag and must
	// be rejected rather than silently mis-sliced.
	if _, err := readFragments(item([]byte{0x01, 0x02}), false); err == nil {
		t.Fatal("readFragments accepted a little-endian item as big endian")
	}
}

func TestReadFragmentsMalformed(t *testing.T) {
	tests := []struct {
		name string
		buf  []byte
		want string
	}{
		{
			name: "truncated fragment data",
			// Item claims 8 bytes of payload but only 4 are present.
			buf:  []byte{0xFE, 0xFF, 0x00, 0xE0, 0x08, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04},
			want: "truncated fragment data",
		},
		{
			name: "undefined item length",
			buf:  []byte{0xFE, 0xFF, 0x00, 0xE0, 0xFF, 0xFF, 0xFF, 0xFF},
			want: "undefined item length",
		},
		{
			name: "unexpected tag",
			buf:  []byte{0xFE, 0xFF, 0x00, 0xE1, 0x04, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04},
			want: "unexpected tag",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frags, err := readFragments(tc.buf, true)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if frags != nil {
				t.Fatalf("frags = %x, want nil on error", frags)
			}
		})
	}
}

func TestReadFragmentsZeroLengthItem(t *testing.T) {
	// A zero-length item is legal item framing (it just carries no payload) and
	// must not stall the scan: the position still advances past the 8-byte header.
	buf := join(item(nil), item([]byte{0xAA, 0xBB}), seqDelimiter)

	frags, err := readFragments(buf, true)
	if err != nil {
		t.Fatalf("readFragments: %v", err)
	}
	if len(frags) != 2 {
		t.Fatalf("got %d fragments, want 2", len(frags))
	}
	if len(frags[0]) != 0 {
		t.Errorf("fragments[0] = %x, want empty", frags[0])
	}
	if len(frags[1]) != 2 {
		t.Errorf("fragments[1] = %x, want 2 bytes", frags[1])
	}
}

func TestGenerateFramesBasicOffsetTableWithoutFragments(t *testing.T) {
	// An offset table that promises frames but carries no fragments yields no
	// frames at all. pydicom instead yields a single empty tuple here; godicom
	// returns zero frames so callers never hand an empty buffer to a codec.
	frames, err := GenerateFrames(join(botItem(0), seqDelimiter), FramesOptions{})
	if err != nil {
		t.Fatalf("GenerateFrames: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("got %d frames, want 0", len(frames))
	}
}

func TestGenerateFramesBasicOffsetTableBadFragmentTag(t *testing.T) {
	buf := join(
		botItem(0),
		[]byte{0xFE, 0xFF, 0x00, 0xE1, 0x04, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04},
	)

	_, err := GenerateFrames(buf, FramesOptions{})
	if err == nil || !strings.Contains(err.Error(), "unexpected tag") {
		t.Fatalf("err = %v, want unexpected tag", err)
	}
}

func TestGenerateFramesBotVariedRatio(t *testing.T) {
	// Port of pydicom TestGenerateFragmentedFrames.test_multi_frame_varied_ratio:
	// 3 frames where the 1st is 1 fragment, the 2nd is 3 and the 3rd is 2.
	buf := join(
		botItem(0, 0x0E, 0x32),
		item([]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x01}),
		item([]byte{0x02, 0x00}),
		item([]byte{0x02, 0x00, 0x00, 0x00}),
		item([]byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x02}),
		item([]byte{0x03, 0x00, 0x00, 0x00}),
		item([]byte{0x02, 0x04}),
	)

	frames, err := GenerateFrames(buf, FramesOptions{})
	if err != nil {
		t.Fatalf("GenerateFrames: %v", err)
	}
	want := [][]byte{
		{0x01, 0x00, 0x00, 0x00, 0x00, 0x01},
		{0x02, 0x00, 0x02, 0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x02},
		{0x03, 0x00, 0x00, 0x00, 0x02, 0x04},
	}
	if len(frames) != len(want) {
		t.Fatalf("got %d frames, want %d", len(frames), len(want))
	}
	for i := range want {
		if string(frames[i]) != string(want[i]) {
			t.Errorf("frame %d = %x, want %x", i, frames[i], want[i])
		}
	}
}

func TestGenerateFragmentedFramesDefaultsToLittleEndian(t *testing.T) {
	// FramesOptions.LittleEndian is a zero-value footgun: false must mean "use the
	// DICOM default (little endian)", not "big endian".
	buf := join(botItem(0), item([]byte{0xDE, 0xAD}))

	framed, err := GenerateFragmentedFrames(buf, FramesOptions{})
	if err != nil {
		t.Fatalf("GenerateFragmentedFrames: %v", err)
	}
	if len(framed) != 1 || len(framed[0]) != 1 || len(framed[0][0]) != 2 {
		t.Fatalf("framed = %v, want 1 frame of 1 fragment of 2 bytes", framed)
	}
}

func TestGenerateFragmentedFramesPropagatesOffsetTableError(t *testing.T) {
	buf := []byte{0xFE, 0xFF, 0x00, 0xE1, 0x00, 0x00, 0x00, 0x00}

	_, err := GenerateFragmentedFrames(buf, FramesOptions{})
	if err == nil || !strings.Contains(err.Error(), "expected item tag") {
		t.Fatalf("err = %v, want expected item tag", err)
	}
}

func TestFramesFromExtendedOutOfRange(t *testing.T) {
	rest := item([]byte{0x01, 0x02})

	tests := []struct {
		name    string
		offsets []uint64
		lengths []uint64
	}{
		// start + length runs past the fragment data.
		{name: "end past buffer", offsets: []uint64{0}, lengths: []uint64{90}},
		// A huge offset overflows int() to a negative value, so start goes negative.
		{name: "negative start", offsets: []uint64{math.MaxUint64 - 15}, lengths: []uint64{2}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := framesFromExtendedRanges(sliceReader{buf: rest}, 0, &ExtendedOffsetTable{
				Offsets: tc.offsets, Lengths: tc.lengths,
			})
			if err == nil || !strings.Contains(err.Error(), "extended offset 0 out of range") {
				t.Fatalf("err = %v, want extended offset 0 out of range", err)
			}
		})
	}
}

func TestGenerateFragmentedFramesWithoutOffsetTableBadFragment(t *testing.T) {
	// Empty BOT plus a malformed first fragment: countFragments rejects it before
	// any frame boundary heuristics run.
	buf := join(botItem(), []byte{0xFE, 0xFF, 0x00, 0xE1, 0x04, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04})

	_, err := GenerateFragmentedFrames(buf, FramesOptions{NumberOfFrames: 2})
	if err == nil || !strings.Contains(err.Error(), "unexpected tag") {
		t.Fatalf("err = %v, want unexpected tag", err)
	}
}

func TestGetFrameErrors(t *testing.T) {
	good := join(botItem(), item([]byte{0x01, 0x02}))

	tests := []struct {
		name  string
		pixel []byte
		index int
		want  string
	}{
		{name: "negative index", pixel: good, index: -1, want: "out of range"},
		{name: "index past end", pixel: good, index: 1, want: "out of range"},
		{
			name:  "malformed offset table",
			pixel: []byte{0xFE, 0xFF, 0x00, 0xE1, 0x00, 0x00, 0x00, 0x00},
			index: 0,
			want:  "expected item tag",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GetFrame(tc.pixel, tc.index, FramesOptions{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestHasEOIMarker(t *testing.T) {
	tests := []struct {
		name string
		frag []byte
		want bool
	}{
		{name: "nil", frag: nil, want: false},
		{name: "single byte", frag: []byte{0xFF}, want: false},
		{name: "marker at end", frag: []byte{0x12, 0x34, 0xFF, 0xD9}, want: true},
		// The window is the last 10 bytes, so a marker starting exactly 10 bytes
		// before the end is still inside it.
		{name: "marker at window start", frag: []byte{0xFF, 0xD9, 1, 2, 3, 4, 5, 6, 7, 8}, want: true},
		// ...and 11 bytes before the end is outside it.
		{name: "marker before window", frag: []byte{0xFF, 0xD9, 1, 2, 3, 4, 5, 6, 7, 8, 9}, want: false},
		{name: "single 0xFF tail", frag: []byte{1, 2, 3, 0xFF}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasEOIMarker(tc.frag); got != tc.want {
				t.Fatalf("hasEOIMarker(%x) = %v, want %v", tc.frag, got, tc.want)
			}
		})
	}
}

func TestFramesByEOIMarkerFlushesTrailingFrame(t *testing.T) {
	// A final group of fragments with no EOI marker is corrupt but still yielded,
	// mirroring pydicom's "fewer frames than expected" warning path.
	buf := append(item([]byte{0x01, 0xFF, 0xD9}), item([]byte{0x02, 0x00})...)
	r := sliceReader{buf: buf}
	fragments, err := fragmentRanges(r, 0)
	if err != nil {
		t.Fatalf("fragmentRanges: %v", err)
	}

	frames, err := framesByEOIMarkerRanges(r, fragments)
	if err != nil {
		t.Fatalf("framesByEOIMarkerRanges: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if len(frames[0]) != 1 || len(frames[1]) != 1 {
		t.Fatalf("frames = %v, want one fragment in each", frames)
	}
}

func TestFragmentFrameParity(t *testing.T) {
	// Port of pydicom TestFragmentFrame. pydicom yields fragments; the expected
	// lengths and padding below are copied from its assertions.
	even := []byte{0xFE, 0xFF, 0x00, 0xE1}
	odd := []byte{0xFE, 0xFF, 0x00}

	tests := []struct {
		name     string
		frame    []byte
		n        int
		wantLens []int
	}{
		{name: "single fragment even data", frame: even, n: 1, wantLens: []int{4}},
		{name: "single fragment odd data", frame: odd, n: 1, wantLens: []int{4}},
		{name: "even fragments even data", frame: even, n: 2, wantLens: []int{2, 2}},
		{name: "even fragments odd data", frame: odd, n: 2, wantLens: []int{2, 2}},
		// byte.Repeat({FE FF 00 E1}, 31) == 124 bytes, 7 fragments.
		{name: "odd fragments even data",
			frame: repeat(even, 31), n: 7, wantLens: []int{18, 18, 18, 18, 18, 18, 16}},
		// 93 bytes, 7 fragments.
		{name: "odd fragments odd data",
			frame: repeat(odd, 31), n: 7, wantLens: []int{14, 14, 14, 14, 14, 14, 10}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frags, err := FragmentFrame(tc.frame, tc.n)
			if err != nil {
				t.Fatalf("FragmentFrame: %v", err)
			}
			if len(frags) != len(tc.wantLens) {
				t.Fatalf("got %d fragments, want %d", len(frags), len(tc.wantLens))
			}
			for i, want := range tc.wantLens {
				if len(frags[i]) != want {
					t.Errorf("fragments[%d] length = %d, want %d", i, len(frags[i]), want)
				}
			}

			// Reassembling the fragments must reproduce the frame, with only the
			// final padding byte possibly appended -- an odd-length frame is always
			// padded, so byte equality only holds for even-length input.
			joined := join(frags...)
			if len(tc.frame)%2 == 0 && string(joined) != string(tc.frame) {
				t.Errorf("reassembled = %x, want the frame verbatim %x", joined, tc.frame)
			}
			if !strings.HasPrefix(string(joined), string(tc.frame)) {
				t.Errorf("reassembled = %x, want prefix %x", joined, tc.frame)
			}
			if len(joined)-len(tc.frame) > 1 {
				t.Errorf("%d padding bytes appended, want at most 1", len(joined)-len(tc.frame))
			}
		})
	}
}

func TestFragmentFrameRejectsBadFragmentCount(t *testing.T) {
	if _, err := FragmentFrame([]byte{1, 2}, 0); err == nil {
		t.Fatal("FragmentFrame accepted 0 fragments")
	}
	if _, err := FragmentFrame([]byte{1, 2}, -1); err == nil {
		t.Fatal("FragmentFrame accepted -1 fragments")
	}
	// 93 bytes allows at most (93+1)/2 = 47 fragments; pydicom's boundary.
	if _, err := FragmentFrame(repeat([]byte{0xFE, 0xFF, 0x00}, 31), 47); err != nil {
		t.Fatalf("FragmentFrame(47) = %v, want success", err)
	}
	if _, err := FragmentFrame(repeat([]byte{0xFE, 0xFF, 0x00}, 31), 48); err == nil {
		t.Fatal("FragmentFrame accepted 48 fragments for 93 bytes")
	}
}

func TestEncapsulateErrorsAndDefaults(t *testing.T) {
	if _, err := Encapsulate(nil, 1, false); err == nil {
		t.Fatal("Encapsulate accepted no frames")
	}

	frames := [][]byte{{0x01, 0x02, 0x03, 0x04}, {0x05, 0x06, 0x07, 0x08}}

	// fragmentsPerFrame <= 0 means one fragment per frame; the output must match
	// the explicit value.
	for _, n := range []int{0, -1} {
		got, err := Encapsulate(frames, n, true)
		if err != nil {
			t.Fatalf("Encapsulate(%d): %v", n, err)
		}
		want, err := Encapsulate(frames, 1, true)
		if err != nil {
			t.Fatalf("Encapsulate(1): %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("Encapsulate(%d) differs from Encapsulate(1)", n)
		}
	}

	// A per-frame fragment count that cannot be satisfied in 2-byte units must
	// surface the FragmentFrame error rather than emit a malformed item.
	if _, err := Encapsulate([][]byte{{0x01, 0x02}}, 2, false); err == nil {
		t.Fatal("Encapsulate accepted 2 fragments for a 2-byte frame")
	}
}

func TestEncapsulateFragmentsPerFrameOffsets(t *testing.T) {
	frames := [][]byte{{0x01, 0x02, 0x03, 0x04}, {0x05, 0x06, 0x07, 0x08}}

	out, err := Encapsulate(frames, 2, true)
	if err != nil {
		t.Fatalf("Encapsulate: %v", err)
	}

	offsets, _, err := ParseBasicOffsets(out, true)
	if err != nil {
		t.Fatalf("ParseBasicOffsets: %v", err)
	}
	// Two 2-byte items per frame: (8+2) * 2 = 20 bytes per frame.
	want := []uint32{0, 20}
	if len(offsets) != len(want) {
		t.Fatalf("offsets = %v, want %v", offsets, want)
	}
	for i := range want {
		if offsets[i] != want[i] {
			t.Fatalf("offsets = %v, want %v", offsets, want)
		}
	}

	// The offsets must actually point where the fragments are.
	back, err := GenerateFrames(out, FramesOptions{})
	if err != nil {
		t.Fatalf("GenerateFrames: %v", err)
	}
	if len(back) != len(frames) {
		t.Fatalf("roundtrip gave %d frames, want %d", len(back), len(frames))
	}
	for i := range frames {
		if string(back[i]) != string(frames[i]) {
			t.Errorf("frame %d = %x, want %x", i, back[i], frames[i])
		}
	}
}

func TestEncapsulateExtendedOddLength(t *testing.T) {
	if _, _, _, err := EncapsulateExtended(nil); err == nil {
		t.Fatal("EncapsulateExtended accepted no frames")
	}

	// Port of pydicom test_encapsulate_odd_length: 1-byte frames are padded to
	// even, so each item is 10 bytes and the offsets step by 10.
	frames := [][]byte{{0x00}, {0x01}, {0x02}}
	pixelData, offsets, lengths, err := EncapsulateExtended(frames)
	if err != nil {
		t.Fatalf("EncapsulateExtended: %v", err)
	}
	if len(pixelData) == 0 {
		t.Fatal("EncapsulateExtended returned no pixel data")
	}

	wantOffsets := []uint64{0, 10, 20}
	wantLengths := []uint64{2, 2, 2}
	for i, want := range wantOffsets {
		if got := binary.LittleEndian.Uint64(offsets[i*8:]); got != want {
			t.Errorf("offsets[%d] = %d, want %d", i, got, want)
		}
	}
	for i, want := range wantLengths {
		if got := binary.LittleEndian.Uint64(lengths[i*8:]); got != want {
			t.Errorf("lengths[%d] = %d, want %d", i, got, want)
		}
	}

	// Feeding the extended tables back in must recover the padded frames.
	eot := &ExtendedOffsetTable{}
	for i := 0; i < len(frames); i++ {
		eot.Offsets = append(eot.Offsets, binary.LittleEndian.Uint64(offsets[i*8:]))
		eot.Lengths = append(eot.Lengths, binary.LittleEndian.Uint64(lengths[i*8:]))
	}
	framed, err := GenerateFragmentedFrames(pixelData, FramesOptions{ExtendedOffsets: eot})
	if err != nil {
		t.Fatalf("GenerateFragmentedFrames: %v", err)
	}
	if len(framed) != len(frames) {
		t.Fatalf("got %d frames, want %d", len(framed), len(frames))
	}
	for i, want := range [][]byte{{0x00, 0x00}, {0x01, 0x00}, {0x02, 0x00}} {
		if len(framed[i]) != 1 || string(framed[i][0]) != string(want) {
			t.Errorf("frame %d = %x, want %x", i, framed[i], want)
		}
	}
}

// repeat concatenates n copies of b.
func repeat(b []byte, n int) []byte {
	out := make([]byte, 0, len(b)*n)
	for i := 0; i < n; i++ {
		out = append(out, b...)
	}
	return out
}
