package godicom_test

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
)

func TestPixelSamples_CT_small_modality(t *testing.T) {
	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "CT_small.dcm")
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	samples, err := ds.PixelSamples(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	// Center pixel (64,64) on 128×128 image.
	idx := 64*128 + 64
	if samples[idx] != 1928 {
		t.Fatalf("stored sample=%v, want 1928", samples[idx])
	}
	hu, err := ds.ApplyModalityLUT(samples)
	if err != nil {
		t.Fatal(err)
	}
	if hu[idx] != 904 {
		t.Fatalf("HU=%v, want 904", hu[idx])
	}
	if math.Abs(hu[0]-(-896)) > 0.1 && hu[0] != -1024+samples[0] {
		// Spot-check rescale applied everywhere.
		slope, _ := ds.GetFloat(tag.RescaleSlope)
		intercept, _ := ds.GetFloat(tag.RescaleIntercept)
		if hu[0] != samples[0]*slope+intercept {
			t.Fatalf("rescale mismatch at 0: %v vs %v", hu[0], samples[0]*slope+intercept)
		}
	}
}

func TestApplyVOILUT_window_with_rescale(t *testing.T) {
	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "693_J2KI.dcm")
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	samples, err := ds.PixelSamples(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	hu, err := ds.ApplyModalityLUT(samples)
	if err != nil {
		t.Fatal(err)
	}
	win, err := ds.ApplyVOILUT(hu, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(win) != len(hu) {
		t.Fatalf("len %d != %d", len(win), len(hu))
	}
	// Output should sit within rescaled full stored range for signed 16-bit CT-like.
	// Just assert finite and not identical to HU (windowing changes values).
	changed := false
	for i := range win {
		if math.IsNaN(win[i]) || math.IsInf(win[i], 0) {
			t.Fatalf("non-finite at %d", i)
		}
		if win[i] != hu[i] {
			changed = true
		}
	}
	if !changed {
		t.Fatal("windowing left all samples unchanged")
	}
}
