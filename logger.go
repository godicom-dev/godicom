package godicom

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// Standard slog attribute keys for godicom debug logs.
const (
	AttrComponent      = "component"
	AttrOffset         = "offset"
	AttrTag            = "tag"
	AttrVR             = "vr"
	AttrLen            = "len"
	AttrTransferSyntax = "transfer_syntax"
	AttrFrame          = "frame"
	AttrPath           = "path"
)

// Component values for AttrComponent.
const (
	ComponentReader = "reader"
	ComponentWriter = "writer"
	ComponentPixels = "pixels"
	ComponentEncaps = "encaps"
)

type loggerContextKey struct{}

var defaultLogger atomic.Pointer[slog.Logger]

func init() {
	defaultLogger.Store(slog.New(slog.DiscardHandler))
}

// SetDefaultLogger sets the process-wide fallback logger used when neither
// context nor Options supply one. Pass nil to restore DiscardHandler (quiet).
// Prefer Options.Logger or WithLogger for call-scoped logging.
func SetDefaultLogger(l *slog.Logger) {
	if l == nil {
		defaultLogger.Store(slog.New(slog.DiscardHandler))
		return
	}
	defaultLogger.Store(l)
}

// DefaultLogger returns the process-wide fallback logger.
func DefaultLogger() *slog.Logger {
	if l := defaultLogger.Load(); l != nil {
		return l
	}
	return slog.New(slog.DiscardHandler)
}

// WithLogger returns a child context that carries l for godicom operations.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil {
		l = slog.New(slog.DiscardHandler)
	}
	return context.WithValue(ctx, loggerContextKey{}, l)
}

// LoggerFromContext returns the logger stored by WithLogger, or DefaultLogger
// when none is present.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return DefaultLogger()
}

// resolveLogger picks Options.Logger over context, then default.
func resolveLogger(ctx context.Context, opt *slog.Logger) *slog.Logger {
	if opt != nil {
		return opt
	}
	return LoggerFromContext(ctx)
}

// loggerContext builds a context carrying the resolved logger and a
// component-scoped child logger.
func loggerContext(parent context.Context, opt *slog.Logger, component string) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	l := resolveLogger(parent, opt).With(AttrComponent, component)
	return WithLogger(parent, l)
}

func logDebug(ctx context.Context, msg string, args ...any) {
	l := LoggerFromContext(ctx)
	if !l.Enabled(ctx, slog.LevelDebug) {
		return
	}
	l.DebugContext(ctx, msg, args...)
}

func logWarn(ctx context.Context, msg string, args ...any) {
	l := LoggerFromContext(ctx)
	if !l.Enabled(ctx, slog.LevelWarn) {
		return
	}
	l.WarnContext(ctx, msg, args...)
}
