package pixels_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/uid"
)

func TestEncodeOptionSetters(t *testing.T) {
	var o pixels.EncodeOptions
	pixels.WithEncodeTransferSyntax(uid.RLELossless)(&o)
	pixels.WithFragmentsPerFrame(3)(&o)
	pixels.WithBasicOffsetTable(false)(&o)
	pixels.WithExtendedOffsetTable(true)(&o)
	o.Logger = slog.New(slog.DiscardHandler)

	if o.TransferSyntaxUID != uid.RLELossless {
		t.Fatalf("transfer syntax = %s", o.TransferSyntaxUID)
	}
	if o.FragmentsPerFrame != 3 {
		t.Fatalf("fragments per frame = %d", o.FragmentsPerFrame)
	}
	if o.HasBOT == nil || *o.HasBOT {
		t.Fatalf("HasBOT = %v, want an explicit false", o.HasBOT)
	}
	if !o.UseExtendedOffsetTable {
		t.Fatal("extended offset table not set")
	}
	if o.Logger == nil {
		t.Fatal("logger not set")
	}
}

func TestDecodeOptionSetters(t *testing.T) {
	var o pixels.DecodeOptions
	pixels.WithRaw(true)(&o)
	pixels.WithLogger(slog.New(slog.DiscardHandler))(&o)
	if !o.Raw || o.Logger == nil {
		t.Fatalf("got %+v", o)
	}
}

func TestEncodeFrameNativeCopies(t *testing.T) {
	desc := pixels.Descriptor{Rows: 1, Columns: 2, SamplesPerPixel: 1, BitsAllocated: 8}
	src := []byte{1, 2}
	out, err := pixels.EncodeFrame(src, desc, uid.ImplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, src) {
		t.Fatalf("got %v", out)
	}
	out[0] = 9
	if src[0] != 1 {
		t.Fatal("native encode aliased its input")
	}
}

func TestEncodeFrameJPEGBaselineRoundtrip(t *testing.T) {
	desc := pixels.Descriptor{
		Rows:                      8,
		Columns:                   8,
		SamplesPerPixel:           1,
		BitsAllocated:             8,
		BitsStored:                8,
		PhotometricInterpretation: "MONOCHROME2",
	}
	src := make([]byte, 8*8)
	for i := range src {
		src[i] = byte(i * 3)
	}
	enc, err := pixels.EncodeFrame(src, desc, uid.JPEGBaseline8Bit)
	skipWithoutNativeCodec(t, err)
	if err != nil {
		t.Fatal(err)
	}
	decDesc := desc
	decDesc.TransferSyntaxUID = uid.JPEGBaseline8Bit
	dec, err := pixels.DecodeFrame(enc, decDesc, pixels.DecodeOptions{Raw: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(dec) != len(src) {
		t.Fatalf("decoded %d bytes, want %d", len(dec), len(src))
	}
}

func TestEncodeFrameJ2KLosslessRoundtrip(t *testing.T) {
	desc := pixels.Descriptor{
		Rows:                      8,
		Columns:                   8,
		SamplesPerPixel:           1,
		BitsAllocated:             8,
		BitsStored:                8,
		PhotometricInterpretation: "MONOCHROME2",
	}
	src := make([]byte, 8*8)
	for i := range src {
		src[i] = byte(i)
	}
	enc, err := pixels.EncodeFrame(src, desc, uid.JPEG2000Lossless)
	skipWithoutNativeCodec(t, err)
	if err != nil {
		t.Fatal(err)
	}
	decDesc := desc
	decDesc.TransferSyntaxUID = uid.JPEG2000Lossless
	dec, err := pixels.DecodeFrame(enc, decDesc, pixels.DecodeOptions{Raw: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, src) {
		t.Fatal("J2K lossless roundtrip changed pixels")
	}
}

func TestEncodeFrameRGBJ2KAndHTJ2K(t *testing.T) {
	// The colour-space branch of the J2K/HTJ2K encoders is separate from the
	// monochrome one; RGB also turns on the multi-component transform.
	desc := pixels.Descriptor{
		Rows:                      8,
		Columns:                   8,
		SamplesPerPixel:           3,
		BitsAllocated:             8,
		BitsStored:                8,
		PhotometricInterpretation: "RGB",
	}
	src := make([]byte, 8*8*3)
	for i := range src {
		src[i] = byte(i)
	}
	for _, ts := range []uid.UID{uid.JPEG2000Lossless, uid.HTJ2KLossless} {
		t.Run(string(ts), func(t *testing.T) {
			enc, err := pixels.EncodeFrame(src, desc, ts)
			skipWithoutNativeCodec(t, err)
			if err != nil {
				t.Fatal(err)
			}
			decDesc := desc
			decDesc.TransferSyntaxUID = ts
			dec, err := pixels.DecodeFrame(enc, decDesc, pixels.DecodeOptions{Raw: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(dec) != len(src) {
				t.Fatalf("decoded %d bytes, want %d", len(dec), len(src))
			}
		})
	}
}

func TestPackDisplayU8DegenerateRanges(t *testing.T) {
	if got := pixels.PackDisplayU8(nil); got != nil {
		t.Fatalf("empty input = %v, want nil", got)
	}
	// A flat frame has no range to scale; it must stay black rather than divide
	// by zero.
	got := pixels.PackDisplayU8([]float64{7, 7, 7})
	if !bytes.Equal(got, []byte{0, 0, 0}) {
		t.Fatalf("flat frame = %v", got)
	}
	// Wider than one byte: scale so the minimum maps to 0 and the maximum to 255.
	got = pixels.PackDisplayU8([]float64{-1000, 0, 1000})
	if got[0] != 0 || got[2] != 255 {
		t.Fatalf("scaled frame = %v", got)
	}
}

func TestBuildArrayValidation(t *testing.T) {
	desc := pixels.Descriptor{Rows: 2, Columns: 2, SamplesPerPixel: 1}
	a, err := pixels.BuildArray([]float64{1, 2, 3, 4}, desc, 0)
	if err != nil {
		t.Fatal(err)
	}
	if a.Frames != 1 || a.ExpectedSampleCount() != 4 {
		t.Fatalf("got %+v", a)
	}
	if _, err := pixels.BuildArray([]float64{1, 2}, desc, 1); err == nil {
		t.Fatal("expected sample count error")
	}
	// A descriptor with a zero dimension cannot state an expected count, so
	// build must not reject the array either.
	loose := pixels.Descriptor{}
	if _, err := pixels.BuildArray([]float64{1, 2, 3}, loose, 1); err != nil {
		t.Fatalf("zero-dimension descriptor: %v", err)
	}
	if got := (pixels.Array{}).ExpectedSampleCount(); got != 0 {
		t.Fatalf("empty array expected %d samples", got)
	}
}

func TestDecodePixelDataCompressedFrameIndexOutOfRange(t *testing.T) {
	desc := pixels.Descriptor{
		Rows:            2,
		Columns:         2,
		SamplesPerPixel: 1,
		BitsAllocated:   8,
		BitsStored:      8,
	}
	enc, err := pixels.EncodeFrames([][]byte{{1, 2, 3, 4}}, desc, pixels.EncodeOptions{
		TransferSyntaxUID: uid.DeflatedImageFrameCompression,
	})
	if err != nil {
		t.Fatal(err)
	}
	src := monochromeSource(2, 2, 1, enc.PixelData)
	src.ts = uid.DeflatedImageFrameCompression
	if _, err := pixels.DecodePixelData(src, pixels.WithFrameIndex(1)); err == nil {
		t.Fatal("expected out-of-range frame index error")
	}
	frames, err := pixels.DecodePixelData(src, pixels.WithFrameIndex(0), pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("got %d frames", len(frames))
	}
}
