package godicom_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
)

// Regression for https://github.com/godicom-dev/godicom/issues/45
func TestReadImplicitVR_PixelDataGetBytes(t *testing.T) {
	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "MR_small.dcm")
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ts, ok := ds.TransferSyntaxUID(); !ok || ts.IsImplicitVR() {
		t.Fatalf("source TS=%q, want explicit VR", ts)
	}
	orig, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}

	ds.FileMeta.Set(godicom.NewDataElement(godicom.MustTag("TransferSyntaxUID"), godicom.VRUI, godicom.ImplicitVRLittleEndian))
	outPath := filepath.Join(t.TempDir(), "mr_implicit_pixel.dcm")
	if err := ds.SaveAs(outPath, &godicom.WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	reread, err := godicom.ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ts, ok := reread.TransferSyntaxUID(); !ok || !ts.IsImplicitVR() {
		t.Fatalf("converted TS=%q, want implicit VR LE", ts)
	}

	el, ok := reread.Get(tag.PixelData)
	if !ok {
		t.Fatal("PixelData missing")
	}
	if _, ok := el.Value.(string); ok {
		t.Fatalf("PixelData value type = string, want []byte")
	}
	got, ok := reread.GetBytes(tag.PixelData)
	if !ok || len(got) == 0 {
		t.Fatalf("GetBytes(PixelData) = %v, %d bytes", ok, len(got))
	}
	pixelsGot, err := reread.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pixelsGot, orig) {
		t.Fatalf("implicit VR pixel mismatch: %d vs %d bytes", len(pixelsGot), len(orig))
	}
}
