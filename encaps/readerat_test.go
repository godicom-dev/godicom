package encaps_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom/encaps"
)

// countingReaderAt counts the bytes a call pulls from the source. The point of
// GetFrameFrom is that this stays near the size of one frame instead of the size
// of the element, so every test here asserts the count rather than trusting it.
type countingReaderAt struct {
	src io.ReaderAt
	n   int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.src.ReadAt(p, off)
	c.n += int64(n)
	return n, err
}

func fragmentItem(data []byte) []byte {
	return append(itemHeader(uint32(len(data))), data...)
}

// streamWithBOT builds encapsulated pixel data with a Basic Offset Table and one
// fragment per frame. Offsets are measured from the first fragment item tag and
// count item headers, as PS3.5 A.4 requires.
func streamWithBOT(frames, frameLen int) []byte {
	bot := make([]byte, 0, frames*4)
	for i := 0; i < frames; i++ {
		off := uint32(i * (frameLen + 8))
		bot = append(bot, byte(off), byte(off>>8), byte(off>>16), byte(off>>24))
	}
	out := append(itemHeader(uint32(len(bot))), bot...)
	for i := 0; i < frames; i++ {
		payload := bytes.Repeat([]byte{byte(i)}, frameLen)
		// A JPEG frame ends with its EOI marker; this stream is delimited by the
		// offset table, but the marker keeps the payload realistic.
		payload[frameLen-2], payload[frameLen-1] = 0xFF, 0xD9
		out = append(out, fragmentItem(payload)...)
	}
	return out
}

// streamWithEOI builds encapsulated pixel data with an empty offset table where
// frames are found by the JPEG End-of-Image marker: fragmentsPerFrame fragments
// per frame, and only the last of each frame carries the marker.
func streamWithEOI(frames, fragmentsPerFrame, fragmentLen int) []byte {
	out := itemHeader(0)
	for f := 0; f < frames; f++ {
		for j := 0; j < fragmentsPerFrame; j++ {
			payload := bytes.Repeat([]byte{byte(f)}, fragmentLen)
			if j == fragmentsPerFrame-1 {
				payload[fragmentLen-2], payload[fragmentLen-1] = 0xFF, 0xD9
			}
			out = append(out, fragmentItem(payload)...)
		}
	}
	return out
}

func TestGetFrameFromMatchesGetFrameOnFixtures(t *testing.T) {
	// The frames have to be the ones GetFrame produces, byte for byte, or the
	// streaming entry point is a second implementation of the same rule.
	for _, name := range []string{
		"emri_small_jpeg_2k_lossless.dcm",
		"emri_small_jpeg_ls_lossless.dcm",
	} {
		t.Run(name, func(t *testing.T) {
			_, pixelData, nf := readPixelData(t, name)
			opts := encaps.FramesOptions{NumberOfFrames: nf, LittleEndian: true}
			for i := 0; i < nf; i++ {
				want, err := encaps.GetFrame(pixelData, i, opts)
				if err != nil {
					t.Fatalf("GetFrame(%d) error = %v", i, err)
				}
				got, err := encaps.GetFrameFrom(bytes.NewReader(pixelData), int64(len(pixelData)), i, opts)
				if err != nil {
					t.Fatalf("GetFrameFrom(%d) error = %v", i, err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("frame %d differs: got %d bytes, want %d", i, len(got), len(want))
				}
			}
		})
	}
}

func TestGetFrameFromMatchesGetFrameSynthetic(t *testing.T) {
	streams := map[string][]byte{
		"basic offset table":  streamWithBOT(4, 64),
		"eoi marker grouping": streamWithEOI(3, 2, 32),
	}
	for name, stream := range streams {
		t.Run(name, func(t *testing.T) {
			opts := encaps.FramesOptions{NumberOfFrames: 4, LittleEndian: true}
			if name == "eoi marker grouping" {
				opts.NumberOfFrames = 3
			}
			want, err := encaps.GenerateFrames(stream, opts)
			if err != nil {
				t.Fatal(err)
			}
			for i := range want {
				got, err := encaps.GetFrameFrom(bytes.NewReader(stream), int64(len(stream)), i, opts)
				if err != nil {
					t.Fatalf("GetFrameFrom(%d) error = %v", i, err)
				}
				if !bytes.Equal(got, want[i]) {
					t.Fatalf("frame %d = %x, want %x", i, got, want[i])
				}
			}
		})
	}
}

func TestGetFrameFromReadsOnlyTheRequestedFrame(t *testing.T) {
	// Eight frames of 64 KiB: the element is 512 KiB and one frame is 64 KiB, so
	// a read that returns one frame must not cost the other seven.
	const frameLen = 64 << 10
	stream := streamWithBOT(8, frameLen)
	src := &countingReaderAt{src: bytes.NewReader(stream)}

	frame, err := encaps.GetFrameFrom(src, int64(len(stream)), 5, encaps.FramesOptions{LittleEndian: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) != frameLen {
		t.Fatalf("frame length = %d, want %d", len(frame), frameLen)
	}
	if want := byte(5); frame[0] != want {
		t.Fatalf("frame starts with %d, want %d", frame[0], want)
	}

	// One frame's bytes, the offset table and eight item headers. A read of the
	// whole element would be len(stream); anything close to that means the
	// boundary scan is buffering values it does not need.
	limit := int64(frameLen + 8*8 + 8 + 32 + 64)
	if src.n > limit {
		t.Fatalf("read %d bytes for one frame, want at most %d of %d", src.n, limit, len(stream))
	}
}

func TestGetFrameFromEOIMarkerReadsTailsOnly(t *testing.T) {
	// With no offset table the boundary is the EOI marker, which lives in the
	// last two bytes of a fragment. Finding it must cost those bytes, not the
	// fragment.
	const fragmentLen = 4096
	stream := streamWithEOI(4, 3, fragmentLen)
	src := &countingReaderAt{src: bytes.NewReader(stream)}

	frame, err := encaps.GetFrameFrom(src, int64(len(stream)), 2, encaps.FramesOptions{
		NumberOfFrames: 4,
		LittleEndian:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) != 3*fragmentLen {
		t.Fatalf("frame length = %d, want %d", len(frame), 3*fragmentLen)
	}

	// Twelve headers, twelve ten-byte tails, then the three fragments of the
	// frame itself. The element is ~48 KiB; this must stay well under it.
	limit := int64(12*8 + 12*10 + 3*fragmentLen + 72)
	if src.n > limit {
		t.Fatalf("read %d bytes for one frame, want at most %d of %d", src.n, limit, len(stream))
	}
}

func TestGetFrameFromExtendedOffsetTable(t *testing.T) {
	// The extended offset table names each frame's byte range outright, so no
	// fragment header is needed to locate one.
	lengths := []int{48, 32, 16}
	stream := itemHeader(0)
	for i, n := range lengths {
		payload := bytes.Repeat([]byte{byte(i)}, n)
		stream = append(stream, fragmentItem(payload)...)
	}
	offsets := make([]uint64, len(lengths))
	pos := 0
	for i, n := range lengths {
		offsets[i] = uint64(pos)
		pos += n + 8
	}

	opts := encaps.FramesOptions{
		LittleEndian:    true,
		ExtendedOffsets: &encaps.ExtendedOffsetTable{Offsets: offsets, Lengths: []uint64{48, 32, 16}},
	}
	want, err := encaps.GenerateFrames(stream, opts)
	if err != nil {
		t.Fatal(err)
	}
	for i := range want {
		got, err := encaps.GetFrameFrom(bytes.NewReader(stream), int64(len(stream)), i, opts)
		if err != nil {
			t.Fatalf("GetFrameFrom(%d) error = %v", i, err)
		}
		if !bytes.Equal(got, want[i]) {
			t.Fatalf("frame %d = %x, want %x", i, got, want[i])
		}
	}
}

func TestGetFrameFromErrors(t *testing.T) {
	stream := streamWithBOT(2, 16)
	opts := encaps.FramesOptions{LittleEndian: true}

	tests := []struct {
		name   string
		src    io.ReaderAt
		size   int64
		index  int
		expect string
	}{
		{name: "nil source", src: nil, size: int64(len(stream)), expect: "nil source"},
		{name: "empty source", src: bytes.NewReader(nil), size: 0, expect: "buffer too short"},
		{name: "index out of range", src: bytes.NewReader(stream), size: int64(len(stream)), index: 2, expect: "out of range"},
		{name: "truncated inside a fragment", src: bytes.NewReader(stream), size: int64(len(stream)) - 4, expect: "truncated fragment data"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := encaps.GetFrameFrom(tc.src, tc.size, tc.index, opts)
			if err == nil || !strings.Contains(err.Error(), tc.expect) {
				t.Fatalf("err = %v, want one containing %q", err, tc.expect)
			}
		})
	}
}

func TestGetFrameFromOutOfRangeMatchesGetFrame(t *testing.T) {
	// Callers switching between the two entry points must not see the error
	// change shape, since both name the frame count the same way.
	stream := streamWithBOT(2, 16)
	opts := encaps.FramesOptions{LittleEndian: true}

	_, heapErr := encaps.GetFrame(stream, 7, opts)
	_, streamErr := encaps.GetFrameFrom(bytes.NewReader(stream), int64(len(stream)), 7, opts)
	if heapErr == nil || streamErr == nil {
		t.Fatalf("want an error from both, got %v and %v", heapErr, streamErr)
	}
	if heapErr.Error() != streamErr.Error() {
		t.Fatalf("errors differ:\n GetFrame:     %v\n GetFrameFrom: %v", heapErr, streamErr)
	}
}

func TestGenerateFragmentedFramesAliasesInput(t *testing.T) {
	// GenerateFrames is the in-memory path and must stay zero-copy: a caller that
	// keeps one fragment should not pay for a second copy of the element.
	stream := streamWithBOT(2, 16)
	groups, err := encaps.GenerateFragmentedFrames(stream, encaps.FramesOptions{LittleEndian: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || len(groups[0]) != 1 {
		t.Fatalf("groups = %d frames, want 2", len(groups))
	}
	// The table is 8 bytes of item header plus two 4-byte offsets, so the first
	// fragment's value starts at 24.
	if &groups[0][0][0] != &stream[24] {
		t.Fatal("fragment does not alias the input buffer")
	}
}

func TestGetFrameFromSourceShorterThanSize(t *testing.T) {
	// A caller passes the size the element claims, so a file truncated on disk
	// arrives here as a source shorter than the range it was told about. Each
	// place that reads has to turn that into an error rather than a panic.
	t.Run("offset table header", func(t *testing.T) {
		// Four bytes where the size says there are twelve: even the item header
		// of the table is not there.
		_, err := encaps.GetFrameFrom(bytes.NewReader([]byte{0xFE, 0xFF, 0x00, 0xE0}), 12, 0, encaps.FramesOptions{})
		if err == nil {
			t.Fatal("want an error for a source shorter than the table header")
		}
	})
	t.Run("basic offset table", func(t *testing.T) {
		// The table says two offsets follow; the bytes stop at the header.
		stream := itemHeader(8)
		_, err := encaps.GetFrameFrom(bytes.NewReader(stream), int64(len(stream))+8, 0, encaps.FramesOptions{})
		if err == nil {
			t.Fatal("want an error for a table that runs past the source")
		}
	})
	t.Run("fragment header", func(t *testing.T) {
		stream := streamWithBOT(2, 16)
		_, err := encaps.GetFrameFrom(bytes.NewReader(stream), int64(len(stream))+64, 0, encaps.FramesOptions{})
		if err == nil || !strings.Contains(err.Error(), "truncated fragment") {
			t.Fatalf("err = %v, want truncated fragment", err)
		}
	})
	t.Run("fragment value", func(t *testing.T) {
		// The extended offset table locates the frame without walking fragments,
		// so the scan succeeds and only the read of the value fails.
		stream := append(itemHeader(0), fragmentItem([]byte{1, 2, 3, 4})...)
		opts := encaps.FramesOptions{
			ExtendedOffsets: &encaps.ExtendedOffsetTable{Offsets: []uint64{0}, Lengths: []uint64{40}},
		}
		_, err := encaps.GetFrameFrom(bytes.NewReader(stream), int64(len(stream))+64, 0, opts)
		if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want an end-of-file error", err)
		}
	})
}

func TestGetFrameFromRejectsUndefinedItemLength(t *testing.T) {
	// A fragment item with an undefined length would make its extent unknowable;
	// the reader refuses it rather than reading to the end of the element.
	stream := append(itemHeader(0), 0xFE, 0xFF, 0x00, 0xE0, 0xFF, 0xFF, 0xFF, 0xFF)
	_, err := encaps.GetFrameFrom(bytes.NewReader(stream), int64(len(stream)), 0, encaps.FramesOptions{})
	if err == nil || !strings.Contains(err.Error(), "undefined item length") {
		t.Fatalf("err = %v, want undefined item length", err)
	}
}

func TestGenerateFramesEOIMarkerShortFragment(t *testing.T) {
	// A fragment shorter than the two bytes a marker needs cannot be one, so the
	// grouping has to step over it instead of reading a window it does not have.
	stream := itemHeader(0)
	stream = append(stream, fragmentItem([]byte{0x01})...)
	stream = append(stream, fragmentItem(nil)...)
	stream = append(stream, fragmentItem([]byte{0x02, 0x03, 0xFF, 0xD9})...)

	frames, err := encaps.GenerateFrames(stream, encaps.FramesOptions{
		NumberOfFrames: 2,
		LittleEndian:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	if want := []byte{0x01, 0x02, 0x03, 0xFF, 0xD9}; !bytes.Equal(frames[0], want) {
		t.Fatalf("frame = %x, want %x", frames[0], want)
	}

	// The same stream through the streaming entry point, whose read of the empty
	// fragment is the one place a zero-length range is asked for.
	got, err := encaps.GetFrameFrom(bytes.NewReader(stream), int64(len(stream)), 0, encaps.FramesOptions{
		NumberOfFrames: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, frames[0]) {
		t.Fatalf("GetFrameFrom = %x, want %x", got, frames[0])
	}
}
