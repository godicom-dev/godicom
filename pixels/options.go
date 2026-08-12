package pixels

import (
	"context"
	"log/slog"
)

// DecodeOptions configures pixel data decoding.
type DecodeOptions struct {
	// Raw skips photometric colour transforms and planar normalization.
	// When false (default), YBR_FULL/_422 is converted to RGB and planar
	// configuration 1 is converted to color-by-pixel.
	Raw bool
	// FrameIndex selects a single frame; nil decodes all frames.
	FrameIndex *int
	// Logger receives debug events for this decode; nil discards.
	Logger *slog.Logger
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

// WithLogger sets the slog logger for pixel decode debug events.
func WithLogger(l *slog.Logger) DecodeOption {
	return func(o *DecodeOptions) {
		o.Logger = l
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

func (o DecodeOptions) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.New(slog.DiscardHandler)
}

func (o DecodeOptions) debug(msg string, args ...any) {
	l := o.logger()
	if !l.Enabled(context.TODO(), slog.LevelDebug) {
		return
	}
	l.Debug(msg, append([]any{"component", "pixels"}, args...)...)
}
