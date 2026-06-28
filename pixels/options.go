package pixels

// DecodeOptions configures pixel data decoding.
type DecodeOptions struct {
	// Raw skips photometric colour transforms (library v2 / raw decode path).
	Raw bool
	// FrameIndex selects a single frame; nil decodes all frames.
	FrameIndex *int
}

// DecodeOption configures DecodeOptions.
type DecodeOption func(*DecodeOptions)

// WithRaw sets whether to return raw decoded bytes without colour transforms.
func WithRaw(raw bool) DecodeOption {
	return func(o *DecodeOptions) {
		o.Raw = raw
	}
}

// WithFrameIndex limits decoding to a single frame index (0-based).
func WithFrameIndex(index int) DecodeOption {
	return func(o *DecodeOptions) {
		o.FrameIndex = &index
	}
}

func applyDecodeOptions(opts []DecodeOption) DecodeOptions {
	out := DecodeOptions{}
	for _, fn := range opts {
		if fn != nil {
			fn(&out)
		}
	}
	return out
}
