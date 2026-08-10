package pixels

import "fmt"

// Array holds decoded pixel samples as float64 values (one per sample),
// with image geometry matching pydicom pixel_array shapes.
type Array struct {
	Samples         []float64
	Frames          int
	Rows            int
	Columns         int
	SamplesPerPixel int
}

// ExpectedSampleCount returns the number of float64 samples Array should contain.
func (a Array) ExpectedSampleCount() int {
	if a.Frames < 1 || a.Rows < 1 || a.Columns < 1 || a.SamplesPerPixel < 1 {
		return 0
	}
	return a.Frames * a.Rows * a.Columns * a.SamplesPerPixel
}

// BuildArray validates sample length and attaches geometry from desc.
func BuildArray(samples []float64, desc Descriptor, frames int) (*Array, error) {
	if frames < 1 {
		frames = 1
	}
	a := &Array{
		Samples:         samples,
		Frames:          frames,
		Rows:            desc.Rows,
		Columns:         desc.Columns,
		SamplesPerPixel: desc.SamplesPerPixel,
	}
	want := a.ExpectedSampleCount()
	if want > 0 && len(samples) != want {
		return nil, fmt.Errorf("pixels: sample count %d, want %d for %d frame(s) %dx%d×%d",
			len(samples), want, frames, desc.Rows, desc.Columns, desc.SamplesPerPixel)
	}
	return a, nil
}
