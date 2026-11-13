package encaps_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom/encaps"
)

// Ports of pydicom/tests/test_encaps.py TestEncapsulatedBuffer,
// TestEncapsulateBufferFunc and TestEncapsulateExtendedBuffer, plus the
// dev-time comparison against pydicom 3.1.0.dev0 run with
// scripts/check_encaps_buffer.py: every digest and byte expectation below
// was produced by running pydicom on the same inputs.

// framesSynthetic are the frames the dev-time comparison used: even, odd,
// empty and even again, so every padding path is covered.
var framesSynthetic = [][]byte{
	bytes.Repeat([]byte{0x00, 0x01}, 10),
	{0xff},
	{},
	{0x02, 0x03, 0x04, 0x05},
}

func readersOf(frames [][]byte) []io.ReadSeeker {
	out := make([]io.ReadSeeker, len(frames))
	for i, f := range frames {
		out[i] = bytes.NewReader(f)
	}
	return out
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func equalU64(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestEncapsulatedBufferLengths ports TestLengths and TestOffsets.
func TestEncapsulatedBufferLengths(t *testing.T) {
	tests := []struct {
		name    string
		frames  [][]byte
		lengths []uint64
		offsets []uint64
	}{
		{"empty", [][]byte{{}}, []uint64{8}, []uint64{0}},
		{"two_empty", [][]byte{{}, {}}, []uint64{8, 8}, []uint64{0, 8}},
		{"odd", [][]byte{{0x00}}, []uint64{10}, []uint64{0}},
		{"even", [][]byte{{0x00, 0x01}}, []uint64{10}, []uint64{0}},
		{"odd_even", [][]byte{{0x00}, {0x00, 0x01}}, []uint64{10, 10}, []uint64{0, 10}},
		{
			"three",
			[][]byte{bytes.Repeat([]byte{0x00, 0x01}, 150), {0x02, 0x03, 0x04, 0x05}, bytes.Repeat([]byte{0x00, 0x01, 0x02}, 111)},
			[]uint64{308, 12, 342},
			[]uint64{0, 308, 320},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eb, err := encaps.NewEncapsulatedBuffer(readersOf(tc.frames), false)
			if err != nil {
				t.Fatal(err)
			}
			if got := eb.Lengths(); !equalU64(got, tc.lengths) {
				t.Errorf("Lengths() = %v, want %v", got, tc.lengths)
			}
			if got := eb.Offsets(); !equalU64(got, tc.offsets) {
				t.Errorf("Offsets() = %v, want %v", got, tc.offsets)
			}
			wantLen := int64(8) // the empty Basic Offset Table
			for _, l := range tc.lengths {
				wantLen += int64(l)
			}
			if got := eb.EncapsulatedLength(); got != wantLen {
				t.Errorf("EncapsulatedLength() = %d, want %d", got, wantLen)
			}
		})
	}
}

// TestEncapsulatedBufferBOT ports TestBOTEmpty and TestBOT.
func TestEncapsulatedBufferBOT(t *testing.T) {
	eb, err := encaps.NewEncapsulatedBuffer(readersOf(framesSynthetic[:1]), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := eb.BasicOffsetTable(); !bytes.Equal(got, []byte{0xfe, 0xff, 0x00, 0xe0, 0x00, 0x00, 0x00, 0x00}) {
		t.Errorf("empty BOT = %x", got)
	}

	// pydicom: a single odd-length frame, then two and three and four frames
	tests := []struct {
		frames [][]byte
		header string
		values string
	}{
		{
			[][]byte{{0xff}},
			"feff00e004000000",
			"00000000",
		},
		{
			[][]byte{{0xff}, bytes.Repeat([]byte{0xfe}, 99)},
			"feff00e008000000",
			"000000000a000000",
		},
		{
			[][]byte{{0xff}, bytes.Repeat([]byte{0xfe}, 99), bytes.Repeat([]byte{0xfd}, 1024)},
			"feff00e00c000000",
			"000000000a00000076000000",
		},
		{
			[][]byte{{0xff}, bytes.Repeat([]byte{0xfe}, 99), bytes.Repeat([]byte{0xfd}, 1024), bytes.Repeat([]byte{0xfc}, 18)},
			"feff00e010000000",
			"000000000a000000760000007e040000",
		},
	}

	for i, tc := range tests {
		eb, err := encaps.NewEncapsulatedBuffer(readersOf(tc.frames), true)
		if err != nil {
			t.Fatal(err)
		}
		bot := eb.BasicOffsetTable()
		if hex.EncodeToString(bot[:8]) != tc.header {
			t.Errorf("case %d: BOT header = %s, want %s", i, hex.EncodeToString(bot[:8]), tc.header)
		}
		if hex.EncodeToString(bot[8:]) != tc.values {
			t.Errorf("case %d: BOT values = %s, want %s", i, hex.EncodeToString(bot[8:]), tc.values)
		}
	}
}

// seekTestBuffer returns the same three-buffer value the pydicom seek and
// read tests read from: a 308-byte item, a 12-byte item and a 342-byte item,
// 670 bytes with the empty Basic Offset Table.
func seekTestBuffer(t *testing.T) *encaps.EncapsulatedBuffer {
	t.Helper()
	eb, err := encaps.NewEncapsulatedBuffer(readersOf([][]byte{
		bytes.Repeat([]byte{0x00, 0x01}, 150),
		{0x02, 0x03, 0x04, 0x05},
		bytes.Repeat([]byte{0x06, 0x07, 0x08}, 111),
	}), false)
	if err != nil {
		t.Fatal(err)
	}
	return eb
}

// TestEncapsulatedBufferSeekTell ports TestSeekTell.
func TestEncapsulatedBufferSeekTell(t *testing.T) {
	eb := seekTestBuffer(t)
	if got := eb.EncapsulatedLength(); got != 670 {
		t.Fatalf("EncapsulatedLength() = %d, want 670", got)
	}

	// The dev-time seek trace: (offset, whence) -> resulting position
	for _, tc := range []struct {
		offset int64
		whence int
		want   int64
	}{
		{8, io.SeekStart, 8},
		{23, io.SeekStart, 23},
		{-1, io.SeekCurrent, 22}, // negative is clamped, emulating io.BytesIO
		{1, io.SeekCurrent, 23},
		{30, io.SeekCurrent, 53},
		{0, io.SeekEnd, 670},
		{-7, io.SeekEnd, 663},
		{-17, io.SeekEnd, 653},
	} {
		got, err := eb.Seek(tc.offset, tc.whence)
		if err != nil {
			t.Fatalf("Seek(%d, %d) error = %v", tc.offset, tc.whence, err)
		}
		if got != tc.want || eb.Tell() != tc.want {
			t.Errorf("Seek(%d, %d) = %d (tell %d), want %d", tc.offset, tc.whence, got, eb.Tell(), tc.want)
		}
	}

	if _, err := eb.Seek(-1, io.SeekStart); err == nil {
		t.Error("Seek(-1, SEEK_SET) should fail")
	}
	for _, whence := range []int{3, -1} {
		if _, err := eb.Seek(0, whence); err == nil {
			t.Errorf("Seek(0, %d) should fail", whence)
		}
	}
}

// TestEncapsulatedBufferRead ports TestRead: reads crossing the boundaries
// between the Basic Offset Table and the items.
func TestEncapsulatedBufferRead(t *testing.T) {
	eb, err := encaps.NewEncapsulatedBuffer(readersOf([][]byte{bytes.Repeat([]byte{0x01, 0x02}, 10)}), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := eb.EncapsulatedLength(); got != 36 {
		t.Fatalf("EncapsulatedLength() = %d, want 36", got)
	}

	out := make([]byte, 8)
	if _, err := io.ReadFull(eb, out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, []byte{0xfe, 0xff, 0x00, 0xe0, 0x00, 0x00, 0x00, 0x00}) {
		t.Errorf("first 8 bytes = %x", out)
	}

	if _, err := eb.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	rest, err := io.ReadAll(eb)
	if err != nil {
		t.Fatal(err)
	}
	want := append(
		[]byte{0xfe, 0xff, 0x00, 0xe0, 0x00, 0x00, 0x00, 0x00},
		append([]byte{0xfe, 0xff, 0x00, 0xe0, 0x14, 0x00, 0x00, 0x00}, bytes.Repeat([]byte{0x01, 0x02}, 10)...)...,
	)
	if !bytes.Equal(rest, want) {
		t.Errorf("read all = %x, want %x", rest, want)
	}

	// The dev-time read trace: odd chunk sizes crossing every boundary of
	// the three-buffer value. The per-chunk digests were produced by
	// pydicom's EncapsulatedBuffer.read() on the same chunks; a read at the
	// end of the value returns io.EOF.
	eb = seekTestBuffer(t)
	wantDigests := []string{
		"2921af177a4d30ae9206fc506c197f1464409017fdc1536f00bd184213166b93", // 6 bytes, into the empty BOT
		"db12ddfb88207b8b046713a98d149f9b91da36d6bd9e53a3b5bc4fff5e466379", // 13 bytes, BOT end, first item tag, item value start
		"a90356b62320c2b263059b3fbe6e5eb3bea394fba1233b3dec03b4cd7cd0b9c7", // 294 bytes, through the first item value
		"8779ef509887b1adca7369e6211749e1f7d216db01bd1e534b429c9c0c22b029", // 12 bytes, first value end, second item tag, value start
		"6deab60f548ae550b4c45a4575fc5bc47ca0be3e1424f46d5424d62d48112d41", // 12 bytes, second value end, third item tag, value start
		"856928ad06f775489dbbf6e2db63b888a8c4ee0e00b9fae2291721be98a6546e", // 108 bytes, through the third item value
		"3ed42689581f3f94a4e0d044fbb5cbcdcd47899b5c7dcd79de25e8dc1e3c1399", // the rest of the value, 225 bytes
	}
	for i, size := range []int{6, 13, 294, 12, 12, 108, 8192} {
		chunk := make([]byte, size)
		n, err := eb.Read(chunk)
		if err != nil {
			t.Fatalf("chunk %d: error = %v", i, err)
		}
		if got := digest(chunk[:n]); got != wantDigests[i] {
			t.Errorf("chunk %d digest = %s, want %s", i, got, wantDigests[i])
		}
	}
	if eb.Tell() != 670 {
		t.Errorf("Tell() = %d, want 670", eb.Tell())
	}
	if n, err := eb.Read(make([]byte, 100)); n != 0 || err != io.EOF {
		t.Errorf("read at end: n = %d, err = %v, want 0, io.EOF", n, err)
	}
}

// deadBuffer ports pydicom's FooBuffer: a 20-byte buffer whose Read returns
// nothing.
type deadBuffer struct{}

func (deadBuffer) Read([]byte) (int, error) { return 0, nil }
func (deadBuffer) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekEnd {
		return 20, nil
	}
	return offset, nil
}

func TestEncapsulatedBufferReadNone(t *testing.T) {
	eb, err := encaps.NewEncapsulatedBuffer([]io.ReadSeeker{deadBuffer{}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := eb.EncapsulatedLength(); got != 36 {
		t.Fatalf("EncapsulatedLength() = %d, want 36", got)
	}
	if _, err := eb.Seek(16, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	// Start of the buffer's data, which has none
	n, err := eb.Read(make([]byte, 20))
	if n != 0 || err != io.EOF {
		t.Errorf("read = (%d, %v), want (0, io.EOF)", n, err)
	}
}

// TestEncapsulatedBufferExtendedTables ports TestExtendedLengths and
// TestExtendedOffsets.
func TestEncapsulatedBufferExtendedTables(t *testing.T) {
	tests := []struct {
		name     string
		frames   [][]byte
		offsets  string
		lengths  string
	}{
		{"empty", [][]byte{{}}, "0000000000000000", "0000000000000000"},
		{"two_empty", [][]byte{{}, {}}, "00000000000000000800000000000000", "00000000000000000000000000000000"},
		{"odd_then_empty", [][]byte{{0x00}, {}}, "00000000000000000a00000000000000", "02000000000000000000000000000000"},
		{
			"three",
			[][]byte{bytes.Repeat([]byte{0x00, 0x01}, 150), {0x02, 0x03, 0x04, 0x05}, bytes.Repeat([]byte{0x00, 0x01, 0x02}, 111)},
			"000000000000000034010000000000004001000000000000",
			"2c0100000000000004000000000000004e01000000000000",
		},
		{
			"four",
			[][]byte{bytes.Repeat([]byte{0x00, 0x01}, 150), {0x02, 0x03, 0x04, 0x05}, bytes.Repeat([]byte{0x00, 0x01, 0x02}, 111), bytes.Repeat([]byte{0x00, 0x01, 0x02}, 5)},
			"0000000000000000340100000000000040010000000000009602000000000000",
			"2c0100000000000004000000000000004e010000000000001000000000000000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eb, err := encaps.NewEncapsulatedBuffer(readersOf(tc.frames), false)
			if err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(eb.ExtendedOffsets()); got != tc.offsets {
				t.Errorf("ExtendedOffsets() = %s, want %s", got, tc.offsets)
			}
			if got := hex.EncodeToString(eb.ExtendedLengths()); got != tc.lengths {
				t.Errorf("ExtendedLengths() = %s, want %s", got, tc.lengths)
			}
		})
	}
}

// TestEncapsulateBuffer ports TestEncapsulateBufferFunc against the synthetic
// frames of the dev-time comparison, including an empty frame and odd-length
// frames.
func TestEncapsulateBuffer(t *testing.T) {
	out, err := encaps.EncapsulateBuffer(readersOf(framesSynthetic), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(out); got != 82 {
		t.Errorf("length = %d, want 82", got)
	}
	if got := digest(out); got != "98e8a3b0b1742516dedc96fe109bc825eb8c7c37d0a618154f9af4674d7fafc7" {
		t.Errorf("digest = %s", got)
	}
	// The BOT header and the first item tag
	if got := hex.EncodeToString(out[:12]); got != "feff00e01000000000000000" {
		t.Errorf("head = %s", got)
	}

	out, err = encaps.EncapsulateBuffer(readersOf(framesSynthetic), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(out); got != 66 {
		t.Errorf("length = %d, want 66", got)
	}
	if got := digest(out); got != "f271f8413c3a109e27d67f6227d661eb608ac910817caba58cb05a2f542dca9a" {
		t.Errorf("digest = %s", got)
	}
}

// TestEncapsulateExtendedBuffer ports TestEncapsulateExtendedBuffer against
// the synthetic frames of the dev-time comparison.
func TestEncapsulateExtendedBuffer(t *testing.T) {
	pixelData, offsets, lengths, err := encaps.EncapsulateExtendedBuffer(readersOf(framesSynthetic))
	if err != nil {
		t.Fatal(err)
	}
	// The Pixel Data is the same value encapsulate_buffer leaves empty BOTs for
	if got := digest(pixelData); got != "f271f8413c3a109e27d67f6227d661eb608ac910817caba58cb05a2f542dca9a" {
		t.Errorf("pixel data digest = %s", got)
	}
	if got := hex.EncodeToString(offsets); got != "00000000000000001c0000000000000026000000000000002e00000000000000" {
		t.Errorf("offsets = %s", got)
	}
	if got := hex.EncodeToString(lengths); got != "1400000000000000020000000000000000000000000000000400000000000000" {
		t.Errorf("lengths = %s", got)
	}
}

// TestEncapsulateBufferFixture is the port of pydicom's use of
// JP2K_10FRAME_NOBOT: the frames of the ten-frame JPEG 2000 fixture
// re-encapsulated with and without Basic Offset Table values, compared
// against pydicom 3.1.0.dev0 with scripts/check_encaps_buffer.py.
func TestEncapsulateBufferFixture(t *testing.T) {
	_, pixelData, nf := readPixelData(t, "emri_small_jpeg_2k_lossless.dcm")
	if nf != 10 {
		t.Fatalf("NumberOfFrames = %d, want 10", nf)
	}
	frames, err := encaps.GenerateFrames(pixelData, encaps.FramesOptions{NumberOfFrames: nf, LittleEndian: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 10 {
		t.Fatalf("frames = %d, want 10", len(frames))
	}

	// The offsets pydicom's own TestEncapsulateBufferFunc pins
	wantOffsets := []uint64{0, 3822, 7670, 11512, 15356, 19166, 22946, 26676, 30434, 34196}
	eb, err := encaps.NewEncapsulatedBuffer(readersOf(frames), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := eb.Offsets(); !equalU64(got, wantOffsets) {
		t.Errorf("Offsets() = %v, want %v", got, wantOffsets)
	}

	out, err := encaps.EncapsulateBuffer(readersOf(frames), true)
	if err != nil {
		t.Fatal(err)
	}
	// The first 56 bytes are pydicom's test_encapsulate_bot expectation
	if got := hex.EncodeToString(out[:56]); got != "feff00e02800000000000000ee0e0000f61d0000f82c0000fc3b0000de4a0000a259000034680000e276000094850000feff00e0e60e0000" {
		t.Errorf("head = %s", got)
	}
	if got := digest(out[:8192]); got != "b15a4d6cb28562d6d2a5040ac806d821512af336e195d7dd12a953474d9b8f64" {
		t.Errorf("digest = %s", got)
	}

	out, err = encaps.EncapsulateBuffer(readersOf(frames), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := digest(out[:8192]); got != "3403da0518590b97475ec0230f506a436dad91c0e86a031e5273f1eb8b5ee4d3" {
		t.Errorf("digest = %s", got)
	}

	// The extended tables: digests from the dev-time comparison
	_, offsets, lengths, err := encaps.EncapsulateExtendedBuffer(readersOf(frames))
	if err != nil {
		t.Fatal(err)
	}
	if got := digest(offsets); got != "ef62e09cfb73b96b2542eefd39637316d5caf31b835db93130102be90610b47d" {
		t.Errorf("extended offsets digest = %s", got)
	}
	if got := digest(lengths); got != "71e61685e42eff327ac59ec2dfb1f396028ed6a78d48abd54709119746440e66" {
		t.Errorf("extended lengths digest = %s", got)
	}
}

// TestItemizeFrame ports TestItemizeFrame.
func TestItemizeFrame(t *testing.T) {
	bytestream := bytes.Repeat([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}, 16)

	items, err := encaps.ItemizeFrame(bytestream, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0]) != len(bytestream)+8 {
		t.Fatalf("items = %d, first length = %d", len(items), len(items[0]))
	}
	if !bytes.Equal(items[0][8:], bytestream) {
		t.Error("single item value differs from the frame")
	}

	items, err = encaps.ItemizeFrame(bytestream, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if !bytes.Equal(items[0][8:], bytestream[:len(bytestream)/2]) ||
		!bytes.Equal(items[1][8:], bytestream[len(bytestream)/2:]) {
		t.Error("two item values differ from the frame halves")
	}

	// Odd frame: the last item carries the pad byte (dev-time golden)
	items, err = encaps.ItemizeFrame([]byte{0x01, 0x02, 0x03, 0x04, 0x05}, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"feff00e0020000000102",
		"feff00e0020000000304",
		"feff00e0020000000500",
	}
	for i, item := range items {
		if hex.EncodeToString(item) != want[i] {
			t.Errorf("item %d = %s, want %s", i, hex.EncodeToString(item), want[i])
		}
	}
}

// TestParseFragments ports pydicom's TestParseFragments.
func TestParseFragments(t *testing.T) {
	// pydicom: test_item_undefined_length
	_, _, err := encaps.ParseFragments([]byte("\xfe\xff\x00\xe0\xff\xff\xff\xff\x00\x00\x00\x01"), true)
	if err == nil || !strings.Contains(err.Error(), "undefined item length at offset 4 when parsing the encapsulated pixel data fragments") {
		t.Errorf("undefined length error = %v", err)
	}
	_, _, err = encaps.ParseFragments([]byte("\xff\xfe\xe0\x00\xff\xff\xff\xff\x00\x00\x00\x01"), false)
	if err == nil || !strings.Contains(err.Error(), "undefined item length at offset 4") {
		t.Errorf("big endian undefined length error = %v", err)
	}

	// pydicom: test_item_sequence_delimiter -- parsing stops at the delimiter
	seq := []byte(
		"\xfe\xff\x00\xe0\x04\x00\x00\x00\x01\x00\x00\x00" +
			"\xfe\xff\xdd\xe0\x00\x00\x00\x00" +
			"\xfe\xff\x00\xe0\x04\x00\x00\x00\x02\x00\x00\x00")
	n, offsets, err := encaps.ParseFragments(seq, true)
	if err != nil || n != 1 || !equalI64(offsets, []int64{0}) {
		t.Errorf("sequence delimiter: (%d, %v, %v)", n, offsets, err)
	}
	seqBE := []byte(
		"\xff\xfe\xe0\x00\x00\x00\x00\x04\x00\x00\x00\x01" +
			"\xff\xfe\xe0\xdd\x00\x00\x00\x00" +
			"\xff\xfe\xe0\x00\x00\x00\x00\x04\x00\x00\x00\x02")
	n, offsets, err = encaps.ParseFragments(seqBE, false)
	if err != nil || n != 1 || !equalI64(offsets, []int64{0}) {
		t.Errorf("big endian sequence delimiter: (%d, %v, %v)", n, offsets, err)
	}

	// pydicom: test_item_bad_tag
	bad := []byte(
		"\xfe\xff\x00\xe0\x04\x00\x00\x00\x01\x00\x00\x00" +
			"\x10\x00\x10\x00\x00\x00\x00\x00" +
			"\xfe\xff\x00\xe0\x04\x00\x00\x00\x02\x00\x00\x00")
	_, _, err = encaps.ParseFragments(bad, true)
	if err == nil || !strings.Contains(err.Error(), "unexpected tag '(0010,0010)' at offset 12 when parsing the encapsulated pixel data fragment items") {
		t.Errorf("bad tag error = %v", err)
	}

	// pydicom: test_item_invalid -- the item length cannot be read
	_, _, err = encaps.ParseFragments([]byte("\xfe\xff\x00\xe0\x04\x00\x00"), true)
	if err == nil || !strings.Contains(err.Error(), "unable to determine the length of the item at offset 0 as the end of the data has been reached") {
		t.Errorf("invalid item error = %v", err)
	}

	// pydicom: test_single_fragment_no_delimiter
	n, offsets, err = encaps.ParseFragments([]byte("\xfe\xff\x00\xe0\x04\x00\x00\x00\x01\x00\x00\x00"), true)
	if err != nil || n != 1 || !equalI64(offsets, []int64{0}) {
		t.Errorf("single fragment: (%d, %v, %v)", n, offsets, err)
	}

	// pydicom: test_multi_fragments_no_delimiter
	multi := []byte(
		"\xfe\xff\x00\xe0\x04\x00\x00\x00\x01\x00\x00\x00" +
			"\xfe\xff\x00\xe0\x06\x00\x00\x00\x01\x02\x03\x04\x05\x06")
	n, offsets, err = encaps.ParseFragments(multi, true)
	if err != nil || n != 2 || !equalI64(offsets, []int64{0, 12}) {
		t.Errorf("multi fragments: (%d, %v, %v)", n, offsets, err)
	}

	// pydicom: test_multi_fragments_delimiter
	withDelim := append(multi, []byte("\xfe\xff\xdd\xe0")...)
	n, offsets, err = encaps.ParseFragments(withDelim, true)
	if err != nil || n != 2 || !equalI64(offsets, []int64{0, 12}) {
		t.Errorf("multi fragments with delimiter: (%d, %v, %v)", n, offsets, err)
	}

	// Fewer than four trailing bytes ends the parse silently, as pydicom's
	// unpack of a short tag read does
	n, offsets, err = encaps.ParseFragments([]byte("\xfe\xff\x00\xe0\x04\x00\x00\x00\x01\x00\x00\x00\xfe\xff"), true)
	if err != nil || n != 1 || !equalI64(offsets, []int64{0}) {
		t.Errorf("trailing garbage: (%d, %v, %v)", n, offsets, err)
	}
}

func equalI64(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
