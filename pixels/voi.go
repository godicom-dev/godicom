package pixels

import (
	"fmt"
	"math"
	"strings"
)

// WindowConfig configures ApplyWindowing (VOI window center/width).
type WindowConfig struct {
	Center                    float64
	Width                     float64
	Function                  string // LINEAR (default), LINEAR_EXACT, SIGMOID
	PhotometricInterpretation string
	BitsStored                int
	PixelRepresentation       int
	// Output range inputs (after modality), matching pydicom apply_windowing.
	HasRescale            bool
	RescaleSlope          float64
	RescaleIntercept      float64
	HasModalityLUT        bool
	ModalityLUTOutputBits int
}

// VOIParams configures ApplyVOILUT (choose VOI LUT vs windowing).
type VOIParams struct {
	PreferLUT bool
	LUT       *LUT
	Window    *WindowConfig
	Index     int // reserved for multi-view; Window/LUT should already be selected
}

// ApplyVOILUT applies a VOI LUT or windowing operation.
// Mirrors pydicom.pixels.processing.apply_voi_lut.
func ApplyVOILUT(arr []float64, p VOIParams) ([]float64, error) {
	hasLUT := p.LUT != nil && len(p.LUT.Entries) > 0
	hasWin := p.Window != nil
	switch {
	case hasLUT && hasWin:
		if p.PreferLUT {
			return ApplyVOI(arr, *p.LUT)
		}
		return ApplyWindowing(arr, *p.Window)
	case hasLUT:
		return ApplyVOI(arr, *p.LUT)
	case hasWin:
		return ApplyWindowing(arr, *p.Window)
	default:
		out := make([]float64, len(arr))
		copy(out, arr)
		return out, nil
	}
}

// ApplyVOI applies a VOI lookup table to arr.
func ApplyVOI(arr []float64, lut LUT) ([]float64, error) {
	return applyLUT(arr, lut)
}

// ApplyWindowing applies Window Center/Width to monochrome samples.
// Mirrors pydicom.pixels.processing.apply_windowing.
func ApplyWindowing(arr []float64, cfg WindowConfig) ([]float64, error) {
	pi := strings.TrimSpace(cfg.PhotometricInterpretation)
	if pi != "MONOCHROME1" && pi != "MONOCHROME2" {
		return nil, fmt.Errorf("pixels: windowing requires MONOCHROME1/2, got %q", pi)
	}

	voiFunc := strings.ToUpper(strings.TrimSpace(cfg.Function))
	if voiFunc == "" {
		voiFunc = "LINEAR"
	}

	yMin, yMax := windowOutputRange(cfg)
	yRange := yMax - yMin

	center := cfg.Center
	width := cfg.Width
	out := make([]float64, len(arr))
	copy(out, arr)

	switch voiFunc {
	case "LINEAR", "LINEAR_EXACT":
		if voiFunc == "LINEAR" {
			if width < 1 {
				return nil, fmt.Errorf("pixels: Window Width must be >= 1 for LINEAR")
			}
			center -= 0.5
			width -= 1
		} else if width <= 0 {
			return nil, fmt.Errorf("pixels: Window Width must be > 0 for LINEAR_EXACT")
		}
		half := width / 2
		low := center - half
		high := center + half
		for i, v := range out {
			switch {
			case v <= low:
				out[i] = yMin
			case v > high:
				out[i] = yMax
			default:
				out[i] = ((v-center)/width+0.5)*yRange + yMin
			}
		}
	case "SIGMOID":
		if width <= 0 {
			return nil, fmt.Errorf("pixels: Window Width must be > 0 for SIGMOID")
		}
		for i, v := range out {
			out[i] = yRange/(1+math.Exp(-4*(v-center)/width)) + yMin
		}
	default:
		return nil, fmt.Errorf("pixels: unsupported VOI LUT Function %q", voiFunc)
	}
	return out, nil
}

func windowOutputRange(cfg WindowConfig) (yMin, yMax float64) {
	if cfg.HasModalityLUT {
		yMin = 0
		bits := cfg.ModalityLUTOutputBits
		if bits <= 0 {
			bits = 16
		}
		yMax = math.Pow(2, float64(bits)) - 1
	} else if cfg.PixelRepresentation == 0 {
		yMin = 0
		yMax = math.Pow(2, float64(cfg.BitsStored)) - 1
	} else {
		yMin = -math.Pow(2, float64(cfg.BitsStored-1))
		yMax = math.Pow(2, float64(cfg.BitsStored-1)) - 1
	}
	if cfg.HasRescale {
		yMin = yMin*cfg.RescaleSlope + cfg.RescaleIntercept
		yMax = yMax*cfg.RescaleSlope + cfg.RescaleIntercept
	}
	return yMin, yMax
}

// InvertValues returns max(arr) - arr[i] for each sample.
// Used for Presentation LUT Shape INVERSE and MONOCHROME1 display invert.
func InvertValues(arr []float64) []float64 {
	if len(arr) == 0 {
		return nil
	}
	max := arr[0]
	for _, v := range arr[1:] {
		if v > max {
			max = v
		}
	}
	out := make([]float64, len(arr))
	for i, v := range arr {
		out[i] = max - v
	}
	return out
}

// ApplyPresentationLUTShape applies IDENTITY or INVERSE presentation shape.
func ApplyPresentationLUTShape(arr []float64, shape string) ([]float64, error) {
	s := strings.ToUpper(strings.TrimSpace(shape))
	switch s {
	case "", "IDENTITY":
		out := make([]float64, len(arr))
		copy(out, arr)
		return out, nil
	case "INVERSE":
		return InvertValues(arr), nil
	default:
		return nil, fmt.Errorf("pixels: unsupported Presentation LUT Shape %q", shape)
	}
}
