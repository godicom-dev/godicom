package pixels_test

import (
	"bytes"
	"testing"

	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/uid"
)

func TestEncodeFrame_RLE_roundtrip_synthetic(t *testing.T) {
	desc := pixels.Descriptor{
		TransferSyntaxUID: uid.RLELossless,
		Rows:              2,
		Columns:           2,
		SamplesPerPixel:   1,
		BitsAllocated:     8,
		BitsStored:        8,
	}
	src := []byte{1, 2, 3, 4}
	enc, err := pixels.EncodeFrame(src, desc, uid.RLELossless)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := pixels.DecodeFrame(enc, desc, pixels.DecodeOptions{Raw: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, src) {
		t.Fatalf("got %v want %v", dec, src)
	}
}

func TestEncodeFrames_native(t *testing.T) {
	desc := pixels.Descriptor{Rows: 1, Columns: 2, SamplesPerPixel: 1, BitsAllocated: 8}
	out, err := pixels.EncodeFrames([][]byte{{1, 2}, {3, 4}}, desc, pixels.EncodeOptions{
		TransferSyntaxUID: uid.ExplicitVRLittleEndian,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsEncapsulated || !bytes.Equal(out.PixelData, []byte{1, 2, 3, 4}) {
		t.Fatalf("got encapsulated=%v data=%v", out.IsEncapsulated, out.PixelData)
	}
}
