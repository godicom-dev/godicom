package pixels_test

import (
	"math"
	"testing"

	"github.com/godicom-dev/godicom/pixels"
)

func TestApplyRescale(t *testing.T) {
	in := []float64{0, 1, 1928}
	out := pixels.ApplyRescale(in, 1, -1024)
	want := []float64{-1024, -1023, 904}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("out[%d]=%v, want %v", i, out[i], want[i])
		}
	}
}

func TestApplyWindowingLINEARUint8(t *testing.T) {
	// Mirrors pydicom test_window_uint8 LINEAR cases.
	arr := []float64{0, 1, 128, 254, 255}
	cfg := pixels.WindowConfig{
		PhotometricInterpretation: "MONOCHROME1",
		PixelRepresentation:       0,
		BitsStored:                8,
		Function:                  "LINEAR",
	}

	cfg.Width, cfg.Center = 1, 0
	out, err := pixels.ApplyWindowing(arr, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range out {
		if v != 255 {
			t.Fatalf("width=1 center=0 out[%d]=%v, want 255", i, v)
		}
	}

	cfg.Width, cfg.Center = 128, 254
	out, err = pixels.ApplyWindowing(arr, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{0, 0, 0, 128.5, 130.5}
	for i := range want {
		if math.Abs(out[i]-want[i]) > 0.1 {
			t.Fatalf("out[%d]=%v, want ~%v", i, out[i], want[i])
		}
	}

	cfg.Function = "LINEAR_EXACT"
	out, err = pixels.ApplyWindowing(arr, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want = []float64{0, 0, 0, 127.5, 129.5}
	for i := range want {
		if math.Abs(out[i]-want[i]) > 0.1 {
			t.Fatalf("exact out[%d]=%v, want ~%v", i, out[i], want[i])
		}
	}
}

func TestApplyWindowingSIGMOID(t *testing.T) {
	arr := []float64{0, 1, 128, 254, 255}
	cfg := pixels.WindowConfig{
		PhotometricInterpretation: "MONOCHROME1",
		PixelRepresentation:       0,
		BitsStored:                8,
		Function:                  "SIGMOID",
		Width:                     128,
		Center:                    254,
	}
	out, err := pixels.ApplyWindowing(arr, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{0.1, 0.1, 4.9, 127.5, 129.5}
	for i := range want {
		if math.Abs(out[i]-want[i]) > 0.1 {
			t.Fatalf("sigmoid out[%d]=%v, want ~%v", i, out[i], want[i])
		}
	}
}

func TestApplyVOILUTPrefer(t *testing.T) {
	arr := []float64{0, 1, 2}
	lut := pixels.LUT{FirstMap: 0, Entries: []uint16{10, 20, 30}, OutputBits: 8}
	win := pixels.WindowConfig{
		PhotometricInterpretation: "MONOCHROME2",
		BitsStored:                8,
		PixelRepresentation:       0,
		Center:                    1,
		Width:                     2,
		Function:                  "LINEAR",
	}
	out, err := pixels.ApplyVOILUT(arr, pixels.VOIParams{PreferLUT: true, LUT: &lut, Window: &win})
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 10 || out[1] != 20 || out[2] != 30 {
		t.Fatalf("prefer LUT got %v", out)
	}
	out, err = pixels.ApplyVOILUT(arr, pixels.VOIParams{PreferLUT: false, LUT: &lut, Window: &win})
	if err != nil {
		t.Fatal(err)
	}
	// Window path returns floats in [0,255], not LUT ints.
	if out[0] == 10 && out[1] == 20 {
		t.Fatalf("expected windowing path, got LUT values %v", out)
	}
}

func TestApplyModalityLUTTable(t *testing.T) {
	arr := []float64{-1, 0, 1, 2, 99}
	lut := pixels.LUT{FirstMap: 0, Entries: []uint16{100, 200, 300}, OutputBits: 16}
	out, err := pixels.ApplyModalityLUT(arr, pixels.ModalityParams{LUT: &lut})
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{100, 100, 200, 300, 300}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("out[%d]=%v, want %v", i, out[i], want[i])
		}
	}
}

func TestInvertValues(t *testing.T) {
	out := pixels.InvertValues([]float64{0, 10, 5})
	if out[0] != 10 || out[1] != 0 || out[2] != 5 {
		t.Fatalf("got %v", out)
	}
}

func TestUnpackSamplesSigned16LE(t *testing.T) {
	// 1928 as int16 LE
	raw := []byte{0x88, 0x07}
	out, err := pixels.UnpackSamples(raw, 16, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != 1928 {
		t.Fatalf("got %v", out)
	}
}
