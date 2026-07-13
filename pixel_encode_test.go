package godicom_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/uid"
)

func TestCompressPixelData_RLE_CT_small_roundtrip(t *testing.T) {
	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "CT_small.dcm")
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	orig, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := ds.CompressPixelData(string(uid.RLELossless)); err != nil {
		t.Fatal(err)
	}
	ts, ok := ds.TransferSyntaxUID()
	if !ok || ts != string(uid.RLELossless) {
		t.Fatalf("TS=%q", ts)
	}
	got, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, orig) {
		t.Fatalf("RLE roundtrip pixel mismatch: len %d vs %d", len(got), len(orig))
	}

	tmp := filepath.Join(t.TempDir(), "ct_rle.dcm")
	if err := ds.SaveAs(tmp, nil); err != nil {
		t.Fatal(err)
	}
	reread, err := godicom.ReadFile(tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := reread.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got2, orig) {
		t.Fatal("save/read RLE pixel mismatch")
	}
	info, err := os.Stat(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("empty output file")
	}
}

func TestCompressPixelData_Deflated_MR_small(t *testing.T) {
	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "MR_small.dcm")
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	orig, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := ds.CompressPixelData(string(uid.DeflatedImageFrameCompression)); err != nil {
		t.Fatal(err)
	}
	got, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, orig) {
		t.Fatal("deflated frame roundtrip mismatch")
	}
}
