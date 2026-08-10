package godicom_test

import (
	"path/filepath"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/pixels"
)

func TestPixelArray_CT_small(t *testing.T) {
	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "CT_small.dcm")
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	arr, err := ds.PixelArray(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if arr.Frames != 1 || arr.Rows != 128 || arr.Columns != 128 || arr.SamplesPerPixel != 1 {
		t.Fatalf("shape = %+v", arr)
	}
	if len(arr.Samples) != 128*128 {
		t.Fatalf("len = %d", len(arr.Samples))
	}
	idx := 64*128 + 64
	if arr.Samples[idx] != 1928 {
		t.Fatalf("center sample = %v, want 1928", arr.Samples[idx])
	}
}

func TestDisplayFrame_CT_small(t *testing.T) {
	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "CT_small.dcm")
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := ds.DisplayFrame(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) != 128*128 {
		t.Fatalf("len = %d, want %d", len(frame), 128*128)
	}
	changed := false
	for _, b := range frame {
		if b != 0 {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("display frame is all zeros")
	}
}

func TestDisplayFrame_SC_rgb_jpeg(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("SC_rgb_jpeg_gdcm.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := ds.DisplayFrame(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) != 100*100*3 {
		t.Fatalf("len = %d", len(frame))
	}
	r, g, b := rgbAt(frame, 100, 5, 50)
	if r != 255 || g != 0 || b != 0 {
		t.Fatalf("arr[5,50,:] = (%d,%d,%d), want (255,0,0)", r, g, b)
	}
}

func TestPixelArray_matchesPixelSamples(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	arr, err := ds.PixelArray(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	samples, err := ds.PixelSamples(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(arr.Samples) != len(samples) {
		t.Fatalf("len mismatch %d vs %d", len(arr.Samples), len(samples))
	}
	for i := range samples {
		if arr.Samples[i] != samples[i] {
			t.Fatalf("sample[%d] = %v, want %v", i, arr.Samples[i], samples[i])
		}
	}
}
