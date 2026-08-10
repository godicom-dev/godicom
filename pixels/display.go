package pixels

// DisplayOptions configures DisplayFrame-style presentation processing.
type DisplayOptions struct {
	WindowIndex int
	PreferLUT   bool
}

// DisplayOption configures DisplayOptions.
type DisplayOption func(*DisplayOptions)

// WithDisplayWindowIndex selects Window Center/Width or VOI LUT index (default 0).
func WithDisplayWindowIndex(index int) DisplayOption {
	return func(o *DisplayOptions) {
		o.WindowIndex = index
	}
}

// WithPreferVOILUT sets whether VOI LUT takes precedence over windowing (default true).
func WithPreferVOILUT(prefer bool) DisplayOption {
	return func(o *DisplayOptions) {
		o.PreferLUT = prefer
	}
}

func applyDisplayOptions(opts []DisplayOption) DisplayOptions {
	out := DisplayOptions{PreferLUT: true}
	for _, fn := range opts {
		if fn != nil {
			fn(&out)
		}
	}
	return out
}

// PackDisplayU8 converts processed samples to 8-bit display values.
// Values already in [0,255] are clipped; wider ranges are linearly scaled.
func PackDisplayU8(samples []float64) []byte {
	if len(samples) == 0 {
		return nil
	}
	min, max := samples[0], samples[0]
	for _, v := range samples[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	out := make([]byte, len(samples))
	if max <= min {
		return out
	}
	if min >= 0 && max <= 255 {
		for i, v := range samples {
			out[i] = clipU8(v)
		}
		return out
	}
	scale := 255.0 / (max - min)
	for i, v := range samples {
		out[i] = clipU8((v - min) * scale)
	}
	return out
}
