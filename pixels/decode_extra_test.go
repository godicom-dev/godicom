package pixels_test

import (
	"bytes"
	"testing"

	"github.com/godicom-dev/godicom/encaps"
	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// fakeSource is the smallest thing that satisfies pixels.FileSource. The decode
// entry points only ever ask it for a handful of top-level elements, and a fake
// keeps these tests from depending on a fixture file for behaviour that has
// nothing to do with parsing.
type fakeSource struct {
	ts    uid.UID
	hasTS bool
	ints  map[tag.Tag]int
	strs  map[tag.Tag]string
	byts  map[tag.Tag][]byte
}

func newFakeSource() *fakeSource {
	return &fakeSource{
		ts:    uid.ExplicitVRLittleEndian,
		hasTS: true,
		ints:  map[tag.Tag]int{},
		strs:  map[tag.Tag]string{},
		byts:  map[tag.Tag][]byte{},
	}
}

func (f *fakeSource) TransferSyntaxUID() (uid.UID, bool) { return f.ts, f.hasTS }
func (f *fakeSource) GetInt(t tag.Tag) (int, bool)       { v, ok := f.ints[t]; return v, ok }
func (f *fakeSource) GetString(t tag.Tag) (string, bool) { v, ok := f.strs[t]; return v, ok }
func (f *fakeSource) GetBytes(t tag.Tag) ([]byte, bool)  { v, ok := f.byts[t]; return v, ok }

// monochromeSource builds a source whose descriptor is complete: rows x columns,
// one sample per pixel, 8 bits, unsigned.
func monochromeSource(rows, columns, frames int, data []byte) *fakeSource {
	f := newFakeSource()
	f.ints[tag.Rows] = rows
	f.ints[tag.Columns] = columns
	f.ints[tag.SamplesPerPixel] = 1
	f.ints[tag.BitsAllocated] = 8
	f.ints[tag.BitsStored] = 8
	f.ints[tag.PixelRepresentation] = 0
	f.ints[tag.NumberOfFrames] = frames
	f.strs[tag.PhotometricInterpretation] = "MONOCHROME2"
	f.byts[tag.PixelData] = data
	return f
}

func TestDecodePixelDataNativeSingleFrame(t *testing.T) {
	src := monochromeSource(2, 2, 1, []byte{1, 2, 3, 4})
	frames, err := pixels.DecodePixelData(src, pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || !bytes.Equal(frames[0], []byte{1, 2, 3, 4}) {
		t.Fatalf("got %v", frames)
	}
}

func TestDecodePixelDataNativeMultiFrame(t *testing.T) {
	src := monochromeSource(1, 2, 3, []byte{1, 2, 3, 4, 5, 6})
	frames, err := pixels.DecodePixelData(src, pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 {
		t.Fatalf("got %d frames", len(frames))
	}
	if !bytes.Equal(frames[1], []byte{3, 4}) || !bytes.Equal(frames[2], []byte{5, 6}) {
		t.Fatalf("frame split wrong: %v", frames)
	}
	// DecodeAllFrames must put them back in native order.
	all, err := pixels.DecodeAllFrames(src, pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(all, []byte{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("DecodeAllFrames = %v", all)
	}
}

func TestDecodePixelDataFrameIndex(t *testing.T) {
	src := monochromeSource(1, 2, 3, []byte{1, 2, 3, 4, 5, 6})
	frames, err := pixels.DecodePixelData(src, pixels.WithRaw(true), pixels.WithFrameIndex(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || !bytes.Equal(frames[0], []byte{5, 6}) {
		t.Fatalf("got %v", frames)
	}
	if _, err := pixels.DecodePixelData(src, pixels.WithFrameIndex(3)); err == nil {
		t.Fatal("expected out-of-range frame index error")
	}
	if _, err := pixels.DecodePixelData(src, pixels.WithFrameIndex(-1)); err == nil {
		t.Fatal("expected negative frame index error")
	}
}

func TestDecodePixelDataNativeLengthMismatch(t *testing.T) {
	src := monochromeSource(2, 2, 1, []byte{1, 2, 3})
	if _, err := pixels.DecodePixelData(src); err == nil {
		t.Fatal("expected native length error")
	}
}

func TestDecodePixelDataMissingPixelData(t *testing.T) {
	src := monochromeSource(2, 2, 1, nil)
	delete(src.byts, tag.PixelData)
	if _, err := pixels.DecodePixelData(src); err == nil {
		t.Fatal("expected missing PixelData error")
	}
}

func TestDecodePixelDataMissingTransferSyntax(t *testing.T) {
	src := monochromeSource(2, 2, 1, []byte{1, 2, 3, 4})
	src.hasTS = false
	if _, err := pixels.DecodePixelData(src); err == nil {
		t.Fatal("expected missing transfer syntax error")
	}
}

func TestDecodePixelDataYBR422NativeFrameSize(t *testing.T) {
	// A YBR_FULL_422 frame carries two bytes per pixel, so a 1x2 image is four
	// bytes, not six. Getting this wrong is the difference between decoding and
	// "native pixel data length 4, want 6".
	src := newFakeSource()
	src.ints[tag.Rows] = 1
	src.ints[tag.Columns] = 2
	src.ints[tag.SamplesPerPixel] = 3
	src.ints[tag.BitsAllocated] = 8
	src.ints[tag.BitsStored] = 8
	src.ints[tag.NumberOfFrames] = 1
	src.strs[tag.PhotometricInterpretation] = "YBR_FULL_422"
	src.byts[tag.PixelData] = []byte{10, 20, 30, 40}

	raw, err := pixels.DecodePixelData(src, pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1 || !bytes.Equal(raw[0], []byte{10, 20, 30, 40}) {
		t.Fatalf("raw got %v", raw)
	}

	// Without Raw the sub-sampled frame is expanded and converted to RGB, so the
	// output is 1x2x3 bytes.
	conv, err := pixels.DecodePixelData(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(conv[0]) != 6 {
		t.Fatalf("converted frame is %d bytes, want 6", len(conv[0]))
	}
}

func TestDecodeFrameDeflatedRoundtrip(t *testing.T) {
	desc := pixels.Descriptor{
		TransferSyntaxUID: uid.DeflatedImageFrameCompression,
		Rows:              2,
		Columns:           2,
		SamplesPerPixel:   1,
		BitsAllocated:     8,
	}
	src := []byte{0, 1, 2, 3}
	enc, err := pixels.EncodeFrame(src, desc, uid.DeflatedImageFrameCompression)
	if err != nil {
		t.Fatal(err)
	}
	if len(enc) == 0 {
		t.Fatal("deflate produced no output")
	}
	dec, err := pixels.DecodeFrame(enc, desc, pixels.DecodeOptions{Raw: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, src) {
		t.Fatalf("roundtrip got %v want %v", dec, src)
	}
}

func TestDecodePixelDataDeflatedEncapsulatedRoundtrip(t *testing.T) {
	desc := pixels.Descriptor{
		Rows:            2,
		Columns:         2,
		SamplesPerPixel: 1,
		BitsAllocated:   8,
	}
	enc, err := pixels.EncodeFrames([][]byte{{1, 2, 3, 4}, {5, 6, 7, 8}}, desc, pixels.EncodeOptions{
		TransferSyntaxUID: uid.DeflatedImageFrameCompression,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !enc.IsEncapsulated {
		t.Fatal("deflated pixel data must be encapsulated")
	}

	src := monochromeSource(2, 2, 2, enc.PixelData)
	src.ts = uid.DeflatedImageFrameCompression
	frames, err := pixels.DecodePixelData(src, pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || !bytes.Equal(frames[0], []byte{1, 2, 3, 4}) || !bytes.Equal(frames[1], []byte{5, 6, 7, 8}) {
		t.Fatalf("got %v", frames)
	}
}

func TestDecodeFrameUnsupportedTransferSyntax(t *testing.T) {
	desc := pixels.Descriptor{TransferSyntaxUID: "1.2.3.4.5"}
	if _, err := pixels.DecodeFrame([]byte{1}, desc, pixels.DecodeOptions{}); err == nil {
		t.Fatal("expected unsupported transfer syntax error")
	}
}

func TestDecodeOptionsFrameIndexPointer(t *testing.T) {
	// WithFrameIndex stores a pointer, not a value: two options must not share
	// one address, or the second call would silently move the first one's frame.
	var o pixels.DecodeOptions
	pixels.WithFrameIndex(1)(&o)
	if o.FrameIndex == nil || *o.FrameIndex != 1 {
		t.Fatalf("got %+v", o)
	}
}

func TestDescriptorDefaultsAndMissingElements(t *testing.T) {
	f := newFakeSource()
	f.ints[tag.Rows] = 2
	f.ints[tag.Columns] = 3
	f.ints[tag.SamplesPerPixel] = 1
	f.ints[tag.BitsAllocated] = 16
	f.ints[tag.NumberOfFrames] = 0
	desc, err := pixels.DescriptorFromFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if desc.BitsStored != 16 {
		t.Fatalf("BitsStored default = %d, want BitsAllocated", desc.BitsStored)
	}
	if desc.NumberOfFrames != 1 {
		t.Fatalf("NumberOfFrames = %d, want 1 for a zero value", desc.NumberOfFrames)
	}
	if desc.PixelRepresentation != 0 {
		t.Fatalf("PixelRepresentation = %d, want 0", desc.PixelRepresentation)
	}
	if desc.BytesPerSample() != 2 || desc.UnpackedFrameBytes() != 2*3*1*2 {
		t.Fatalf("sizes: %d bytes/sample, %d frame bytes", desc.BytesPerSample(), desc.UnpackedFrameBytes())
	}

	empty := newFakeSource()
	if _, err := pixels.DescriptorFromDataset(empty, uid.ExplicitVRLittleEndian); err == nil {
		t.Fatal("expected missing-element error")
	} else {
		for _, name := range []string{"Rows", "Columns", "SamplesPerPixel", "BitsAllocated"} {
			if !bytes.Contains([]byte(err.Error()), []byte(name)) {
				t.Fatalf("error should name %s: %v", name, err)
			}
		}
	}
}

func TestDescriptorBytesPerSampleForBitsAllocatedOne(t *testing.T) {
	// A 1-bit image is unpacked into one byte per sample, so the frame size is
	// rows*columns bytes rather than an eighth of that.
	d := pixels.Descriptor{BitsAllocated: 1, Rows: 2, Columns: 2, SamplesPerPixel: 1}
	if d.BytesPerSample() != 1 {
		t.Fatalf("BytesPerSample = %d, want 1", d.BytesPerSample())
	}
	if d.UnpackedFrameBytes() != 4 {
		t.Fatalf("UnpackedFrameBytes = %d, want 4", d.UnpackedFrameBytes())
	}
}

func TestDescriptorExtendedOffsets(t *testing.T) {
	f := newFakeSource()
	f.ints[tag.Rows] = 1
	f.ints[tag.Columns] = 1
	f.ints[tag.SamplesPerPixel] = 1
	f.ints[tag.BitsAllocated] = 8
	f.byts[tag.ExtendedOffsetTable] = []byte{
		1, 0, 0, 0, 0, 0, 0, 0,
		2, 0, 0, 0, 0, 0, 0, 0,
	}
	f.byts[tag.ExtendedOffsetTableLengths] = []byte{
		3, 0, 0, 0, 0, 0, 0, 0,
		4, 0, 0, 0, 0, 0, 0, 0,
	}
	desc, err := pixels.DescriptorFromFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if desc.ExtendedOffsets == nil {
		t.Fatal("extended offsets not parsed")
	}
	if len(desc.ExtendedOffsets.Offsets) != 2 || desc.ExtendedOffsets.Offsets[1] != 2 {
		t.Fatalf("offsets = %v", desc.ExtendedOffsets.Offsets)
	}
	if desc.ExtendedOffsets.Lengths[1] != 4 {
		t.Fatalf("lengths = %v", desc.ExtendedOffsets.Lengths)
	}

	// A lengths table that disagrees with the offsets table is discarded whole:
	// half a table would silently frame the wrong bytes.
	f.byts[tag.ExtendedOffsetTableLengths] = []byte{3, 0, 0, 0, 0, 0, 0, 0}
	desc, err = pixels.DescriptorFromFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if desc.ExtendedOffsets != nil {
		t.Fatalf("mismatched tables should be ignored, got %+v", desc.ExtendedOffsets)
	}

	f.byts[tag.ExtendedOffsetTable] = nil
	desc, err = pixels.DescriptorFromFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if desc.ExtendedOffsets != nil {
		t.Fatal("empty offsets should leave the field nil")
	}
}

func TestUnpackSamplesAllPaths(t *testing.T) {
	for _, tc := range []struct {
		name      string
		data      []byte
		bits      int
		signed    int
		little    bool
		want      []float64
		expectErr bool
	}{
		{name: "8 bit unsigned", data: []byte{0, 128, 255}, bits: 8, want: []float64{0, 128, 255}},
		{name: "8 bit signed", data: []byte{0, 127, 128, 255}, bits: 8, signed: 1, want: []float64{0, 127, -128, -1}},
		{name: "16 bit little endian", data: []byte{0x00, 0x01, 0xFF, 0xFF}, bits: 16, little: true, want: []float64{256, 65535}},
		{name: "16 bit big endian signed", data: []byte{0x00, 0x01, 0xFF, 0xFF}, bits: 16, signed: 1, little: false, want: []float64{1, -1}},
		{name: "16 bit odd length", data: []byte{0x00}, bits: 16, little: true, expectErr: true},
		{name: "unsupported bit depth", data: []byte{0, 0, 0, 0}, bits: 32, expectErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := pixels.UnpackSamples(tc.data, tc.bits, tc.signed, tc.little)
			if tc.expectErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != len(tc.want) {
				t.Fatalf("got %v, want %v", out, tc.want)
			}
			for i := range tc.want {
				if out[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", out, tc.want)
				}
			}
		})
	}
}

func TestEncodeFramesOptionErrors(t *testing.T) {
	desc := pixels.Descriptor{Rows: 1, Columns: 1, SamplesPerPixel: 1, BitsAllocated: 8}
	if _, err := pixels.EncodeFrames(nil, desc, pixels.EncodeOptions{TransferSyntaxUID: uid.ExplicitVRLittleEndian}); err == nil {
		t.Fatal("expected error for no frames")
	}
	if _, err := pixels.EncodeFrames([][]byte{{1}}, desc, pixels.EncodeOptions{}); err == nil {
		t.Fatal("expected error for missing transfer syntax")
	}
}

func TestEncodeFramesFragmentsAndOffsetTables(t *testing.T) {
	desc := pixels.Descriptor{Rows: 2, Columns: 2, SamplesPerPixel: 1, BitsAllocated: 8, BitsStored: 8}
	frame := []byte{1, 2, 3, 4}

	// Two fragments per frame with no Basic Offset Table: the reader has to fall
	// back on fragment counting, which is the path the option exists for.
	out, err := pixels.EncodeFrames([][]byte{frame}, desc, pixels.EncodeOptions{
		TransferSyntaxUID: uid.DeflatedImageFrameCompression,
		FragmentsPerFrame: 2,
		HasBOT:            boolPtr(false),
	})
	skipWithoutNativeCodec(t, err)
	if err != nil {
		t.Fatal(err)
	}
	offsets, rest, err := encaps.ParseBasicOffsets(out.PixelData, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(offsets) != 0 {
		t.Fatalf("BOT should be empty, got %v", offsets)
	}
	if len(rest) == 0 {
		t.Fatal("expected fragments after the empty BOT")
	}
	frames, err := encaps.GenerateFrames(out.PixelData, encaps.FramesOptions{NumberOfFrames: 1, LittleEndian: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1 joined from two fragments", len(frames))
	}

	// Extended offset table: the per-frame offsets and lengths come back as
	// separate byte arrays instead of being written into the encapsulated value.
	ext, err := pixels.EncodeFrames([][]byte{frame, frame}, desc, pixels.EncodeOptions{
		TransferSyntaxUID:      uid.DeflatedImageFrameCompression,
		UseExtendedOffsetTable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ext.ExtendedOffsetTable) != 16 || len(ext.ExtendedOffsetTableLengths) != 16 {
		t.Fatalf("EOT sizes = %d/%d, want 16/16", len(ext.ExtendedOffsetTable), len(ext.ExtendedOffsetTableLengths))
	}
}

func boolPtr(b bool) *bool { return &b }
