package godicom_test

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/pixels"
)

var testDataDir = filepath.Join("pydicom", "src", "pydicom", "data", "test_files")

func testFilePath(name string) string {
	return filepath.Join(testDataDir, name)
}

func TestPixelBytes_native_CT_small(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 128*128*2 {
		t.Fatalf("len = %d, want %d", len(raw), 128*128*2)
	}
}

func TestPixelBytes_J2K_MR_small(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("MR_small_jp2klossless.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64*64*2 {
		t.Fatalf("len = %d, want %d", len(raw), 64*64*2)
	}
	// pydicom reference: arr[0,31:34] == (422, 319, 361) for int16 LE
	if got := int16(binary.LittleEndian.Uint16(raw[(0*64+31)*2:])); got != 422 {
		t.Fatalf("arr[0,31] = %d, want 422", got)
	}
	if got := int16(binary.LittleEndian.Uint16(raw[(31*64+0)*2:])); got != 366 {
		t.Fatalf("arr[31,0] = %d, want 366", got)
	}
}

func TestPixelBytes_RLE_MR_small(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("MR_small_RLE.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64*64*2 {
		t.Fatalf("len = %d, want %d", len(raw), 64*64*2)
	}
	if got := int16(binary.LittleEndian.Uint16(raw[(0*64+31)*2:])); got != 422 {
		t.Fatalf("arr[0,31] = %d, want 422", got)
	}
	if got := int16(binary.LittleEndian.Uint16(raw[(31*64+0)*2:])); got != 366 {
		t.Fatalf("arr[31,0] = %d, want 366", got)
	}
}

func TestPixelFrames_singleIndex(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("MR_small_jp2klossless.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := ds.PixelFrames(pixels.WithRaw(true), pixels.WithFrameIndex(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || len(frames[0]) != 64*64*2 {
		t.Fatalf("unexpected frames: %d lengths", len(frames))
	}
}
