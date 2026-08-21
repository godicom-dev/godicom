package godicom

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
)

// Standard slog attribute keys for godicom debug logs.
// Content mirrors pydicom filereader debug / dcm4che DicomInputStream diagnostics.
const (
	AttrComponent      = "component"
	AttrOffset         = "offset"     // absolute file offset (int)
	AttrOffsetHex      = "offset_hex" // pydicom-style "%08x"
	AttrHex            = "hex"        // header or sample bytes as hex
	AttrTag            = "tag"        // "(GGGG,EEEE)"
	AttrVR             = "vr"
	AttrLen            = "len" // element length; -1 means undefined (0xFFFFFFFF)
	AttrUndefined      = "undefined_length"
	AttrValueHex       = "value_hex" // first ≤20 value bytes as hex
	AttrValue          = "value"     // first ≤20 value bytes as Go quoted string
	AttrTransferSyntax = "transfer_syntax"
	AttrFrame          = "frame"
	AttrPath           = "path"          // file path
	AttrKind           = "kind"          // DiagnosticKind
	AttrSequencePath   = "sequence_path" // enclosing sequence tags, outermost first
	AttrNeed           = "need"          // bytes the encoding called for
	AttrHave           = "have"          // bytes actually available
	AttrError          = "error"
)

// Component values for AttrComponent.
const (
	ComponentReader = "reader"
	ComponentWriter = "writer"
	ComponentPixels = "pixels"
	ComponentEncaps = "encaps"
)

const debugValuePreview = 20

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

func offsetHex(off int64) string {
	return fmt.Sprintf("%08x", uint64(off))
}

// logDiagnostic records an anomaly godicom recovered from. Diagnostics are
// recovered from, so they are logged at warn level rather than returned as
// errors; ReadOptions.OnDiagnostic / WriteOptions.OnDiagnostic is what turns one
// into a failure.
func logDiagnostic(ctx context.Context, d Diagnostic) {
	l := LoggerFromContext(ctx)
	if !l.Enabled(ctx, slog.LevelWarn) {
		return
	}
	args := []any{
		AttrKind, string(d.Kind),
	}
	if !d.Kind.raisedWhileWriting() {
		args = append(args, AttrOffset, d.Offset, AttrOffsetHex, offsetHex(d.Offset))
	}
	if d.Tag != 0 {
		args = append(args, AttrTag, d.Tag.String())
	}
	if d.VR != "" {
		args = append(args, AttrVR, string(d.VR))
	}
	if len(d.Path) > 0 {
		parts := make([]string, len(d.Path))
		for i, t := range d.Path {
			parts[i] = t.String()
		}
		args = append(args, AttrSequencePath, strings.Join(parts, " > "))
	}
	if d.Need != 0 || d.Have != 0 {
		args = append(args, AttrNeed, d.Need, AttrHave, d.Have)
	}
	if d.Err != nil {
		args = append(args, AttrError, d.Err.Error())
	}
	msg := "parse diagnostic"
	if d.Kind.raisedWhileWriting() {
		msg = "write diagnostic"
	}
	l.WarnContext(ctx, msg, args...)
}

func bytesHex(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return hex.EncodeToString(b)
}

// logElementHeader mirrors pydicom's per-element header line:
// offset + header hex + tag + VR + Length / Undefined length.
func logElementHeader(ctx context.Context, offset int64, header []byte, tag Tag, vr VR, length int) {
	l := LoggerFromContext(ctx)
	if !l.Enabled(ctx, slog.LevelDebug) {
		return
	}
	args := []any{
		AttrOffset, offset,
		AttrOffsetHex, offsetHex(offset),
		AttrHex, bytesHex(header),
		AttrTag, tag.String(),
	}
	if vr != "" {
		args = append(args, AttrVR, string(vr))
	}
	if length == 0xFFFFFFFF {
		args = append(args, AttrLen, -1, AttrUndefined, true)
	} else {
		args = append(args, AttrLen, length, AttrUndefined, false)
	}
	l.DebugContext(ctx, "data element", args...)
}

// logElementValue mirrors pydicom's value preview (first 20 bytes hex + repr).
func logElementValue(ctx context.Context, valueTell int64, value []byte) {
	l := LoggerFromContext(ctx)
	if !l.Enabled(ctx, slog.LevelDebug) {
		return
	}
	preview := value
	if len(preview) > debugValuePreview {
		preview = preview[:debugValuePreview]
	}
	l.DebugContext(ctx, "data element value",
		AttrOffset, valueTell,
		AttrOffsetHex, offsetHex(valueTell),
		AttrValueHex, bytesHex(preview),
		AttrValue, fmt.Sprintf("%q", preview),
		AttrLen, len(value),
	)
}
