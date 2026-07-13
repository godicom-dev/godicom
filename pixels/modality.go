package pixels

import "fmt"

// LUT describes a DICOM lookup table (Modality or VOI).
// Entries are unsigned 8- or 16-bit values stored as uint16.
type LUT struct {
	FirstMap   int
	Entries    []uint16
	OutputBits int // 8 or 16 (informational; entries are always uint16-capable)
}

// ModalityParams configures ApplyModalityLUT.
// If LUT is non-nil it takes precedence over Rescale (pydicom behaviour).
type ModalityParams struct {
	LUT              *LUT
	HasRescale       bool
	RescaleSlope     float64
	RescaleIntercept float64
}

// ApplyModalityLUT applies a modality LUT or rescale operation to arr.
// Mirrors pydicom.pixels.processing.apply_modality_lut.
// Returns a new slice; arr is not modified.
func ApplyModalityLUT(arr []float64, p ModalityParams) ([]float64, error) {
	if p.LUT != nil {
		return applyLUT(arr, *p.LUT)
	}
	if p.HasRescale {
		return ApplyRescale(arr, p.RescaleSlope, p.RescaleIntercept), nil
	}
	out := make([]float64, len(arr))
	copy(out, arr)
	return out, nil
}

// ApplyRescale applies slope*x + intercept (pydicom apply_rescale alias path).
func ApplyRescale(arr []float64, slope, intercept float64) []float64 {
	out := make([]float64, len(arr))
	for i, v := range arr {
		out[i] = v*slope + intercept
	}
	return out
}

func applyLUT(arr []float64, lut LUT) ([]float64, error) {
	if len(lut.Entries) == 0 {
		return nil, fmt.Errorf("pixels: empty LUT")
	}
	n := len(lut.Entries)
	out := make([]float64, len(arr))
	for i, v := range arr {
		idx := int(v) - lut.FirstMap
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		out[i] = float64(lut.Entries[idx])
	}
	return out, nil
}
