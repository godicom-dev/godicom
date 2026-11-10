package pixels_test

import (
	"math"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom/pixels"
)

func TestApplyWindowingErrors(t *testing.T) {
	base := pixels.WindowConfig{
		PhotometricInterpretation: "MONOCHROME2",
		PixelRepresentation:       0,
		BitsStored:                8,
		Center:                    128,
		Width:                     128,
		Function:                  "LINEAR",
	}
	for _, tc := range []struct {
		name   string
		mutate func(*pixels.WindowConfig)
	}{
		{"rgb photometric", func(c *pixels.WindowConfig) { c.PhotometricInterpretation = "RGB" }},
		{"empty photometric", func(c *pixels.WindowConfig) { c.PhotometricInterpretation = "" }},
		// pydicom compares (0028,0004) verbatim, so a lower-case value is
		// rejected there too; only VOI LUT Function is case-folded.
		{"lower-case photometric", func(c *pixels.WindowConfig) { c.PhotometricInterpretation = "monochrome2" }},
		{"linear width below one", func(c *pixels.WindowConfig) { c.Width = 0 }},
		{"linear exact width zero", func(c *pixels.WindowConfig) { c.Function = "LINEAR_EXACT"; c.Width = 0 }},
		{"sigmoid width zero", func(c *pixels.WindowConfig) { c.Function = "SIGMOID"; c.Width = 0 }},
		{"unknown function", func(c *pixels.WindowConfig) { c.Function = "CUBIC" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.mutate(&cfg)
			if _, err := pixels.ApplyWindowing([]float64{1}, cfg); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestApplyWindowingSignedRange(t *testing.T) {
	// PixelRepresentation 1 with 8 stored bits spans [-128, 127]. LINEAR shifts
	// centre by -0.5 and width by -1, so width 3 covers (-1.5, 0.5].
	cfg := pixels.WindowConfig{
		PhotometricInterpretation: "MONOCHROME2",
		PixelRepresentation:       1,
		BitsStored:                8,
		Center:                    0,
		Width:                     3,
		Function:                  "  linear  ",
	}
	out, err := pixels.ApplyWindowing([]float64{-2, 0, 1}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{-128, 63.25, 127}
	for i := range want {
		if math.Abs(out[i]-want[i]) > 1e-9 {
			t.Fatalf("out[%d] = %v, want %v (all %v)", i, out[i], want[i], out)
		}
	}
}

func TestApplyWindowingOutputRangeModalityLUT(t *testing.T) {
	// A modality LUT replaces the output range with [0, 2^bits - 1]; a missing
	// bit count defaults to 16, which pydicom also does.
	for _, tc := range []struct {
		name string
		bits int
		want float64
	}{
		{"explicit ten bits", 10, 1023},
		{"default sixteen bits", 0, 65535},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := pixels.WindowConfig{
				PhotometricInterpretation: "MONOCHROME2",
				BitsStored:                8,
				PixelRepresentation:       0,
				Center:                    0,
				Width:                     1,
				Function:                  "LINEAR",
				HasModalityLUT:            true,
				ModalityLUTOutputBits:     tc.bits,
			}
			out, err := pixels.ApplyWindowing([]float64{0, 5}, cfg)
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range out {
				if v != tc.want {
					t.Fatalf("got %v, want %v everywhere", out, tc.want)
				}
			}
		})
	}
}

func TestApplyWindowingOutputRangeRescale(t *testing.T) {
	// Rescale moves the output range with the samples: 0..255 * 1 + 100.
	cfg := pixels.WindowConfig{
		PhotometricInterpretation: "MONOCHROME2",
		BitsStored:                8,
		PixelRepresentation:       0,
		Center:                    0,
		Width:                     1,
		Function:                  "LINEAR",
		HasRescale:                true,
		RescaleSlope:              1,
		RescaleIntercept:          100,
	}
	out, err := pixels.ApplyWindowing([]float64{0}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 355 {
		t.Fatalf("got %v, want 355", out[0])
	}
}

func TestApplyVOILUTWithoutTableIsIdentityCopy(t *testing.T) {
	arr := []float64{1, 2, 3}
	out, err := pixels.ApplyVOILUT(arr, pixels.VOIParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(arr) || out[0] != 1 || out[2] != 3 {
		t.Fatalf("got %v", out)
	}
	out[0] = 9
	if arr[0] != 1 {
		t.Fatal("ApplyVOILUT aliased the input")
	}
}

func TestApplyVOIEmptyTableIsAnError(t *testing.T) {
	// An empty LUT is a broken file, not "no LUT": the caller who reached this
	// path asked for the table and must hear that it is empty.
	if _, err := pixels.ApplyVOI([]float64{1}, pixels.LUT{}); err == nil {
		t.Fatal("expected empty LUT error")
	}
	empty := pixels.LUT{Entries: nil}
	p := pixels.VOIParams{LUT: &empty}
	if _, err := pixels.ApplyVOILUT([]float64{1}, p); err != nil {
		// VOIParams treats a nil-or-empty LUT as "no LUT" and copies instead.
		t.Fatalf("empty table should fall through to the identity path: %v", err)
	}
}

func TestApplyModalityLUTWithoutTableIsIdentityCopy(t *testing.T) {
	arr := []float64{7, 8}
	out, err := pixels.ApplyModalityLUT(arr, pixels.ModalityParams{})
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 7 || out[1] != 8 {
		t.Fatalf("got %v", out)
	}
	out[0] = 0
	if arr[0] != 7 {
		t.Fatal("ApplyModalityLUT aliased the input")
	}
}

func TestApplyModalityLUTPrefersTableOverRescale(t *testing.T) {
	lut := pixels.LUT{FirstMap: 0, Entries: []uint16{1, 2}, OutputBits: 8}
	out, err := pixels.ApplyModalityLUT([]float64{0, 1}, pixels.ModalityParams{
		LUT:              &lut,
		HasRescale:       true,
		RescaleSlope:     10,
		RescaleIntercept: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 1 || out[1] != 2 {
		t.Fatalf("got %v, want the LUT path", out)
	}
}

func TestApplyPresentationLUTShape(t *testing.T) {
	arr := []float64{0, 10, 5}
	for _, shape := range []string{"", "IDENTITY", " identity "} {
		out, err := pixels.ApplyPresentationLUTShape(arr, shape)
		if err != nil {
			t.Fatalf("%q: %v", shape, err)
		}
		if out[0] != 0 || out[1] != 10 || out[2] != 5 {
			t.Fatalf("%q: got %v", shape, out)
		}
	}
	out, err := pixels.ApplyPresentationLUTShape(arr, "inverse")
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 10 || out[1] != 0 || out[2] != 5 {
		t.Fatalf("inverse got %v", out)
	}
	if _, err := pixels.ApplyPresentationLUTShape(arr, "SQUARE"); err == nil {
		t.Fatal("expected error for unknown shape")
	} else if !strings.Contains(err.Error(), "SQUARE") {
		t.Fatalf("error should name the shape: %v", err)
	}
}

func TestInvertValuesEmpty(t *testing.T) {
	if got := pixels.InvertValues(nil); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestApplyLUTClampsOutOfRangeIndices(t *testing.T) {
	lut := pixels.LUT{FirstMap: 10, Entries: []uint16{5, 6, 7}}
	out, err := pixels.ApplyModalityLUT([]float64{9, 10, 12, 200}, pixels.ModalityParams{LUT: &lut})
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{5, 5, 7, 7}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("out[%d] = %v, want %v", i, out[i], want[i])
		}
	}
}

func TestDisplayOptionsSetters(t *testing.T) {
	var o pixels.DisplayOptions
	pixels.WithDisplayWindowIndex(2)(&o)
	pixels.WithPreferVOILUT(false)(&o)
	if o.WindowIndex != 2 || o.PreferLUT {
		t.Fatalf("got %+v", o)
	}
}
