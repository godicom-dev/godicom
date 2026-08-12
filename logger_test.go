package godicom_test

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
)

type memHandler struct {
	level slog.Level
	buf   *bytes.Buffer
	attrs []slog.Attr
}

func (h *memHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *memHandler) Handle(_ context.Context, r slog.Record) error {
	h.buf.WriteString(r.Level.String())
	h.buf.WriteByte(' ')
	h.buf.WriteString(r.Message)
	for _, a := range h.attrs {
		h.buf.WriteByte(' ')
		h.buf.WriteString(a.Key)
		h.buf.WriteByte('=')
		h.buf.WriteString(a.Value.String())
	}
	r.Attrs(func(a slog.Attr) bool {
		h.buf.WriteByte(' ')
		h.buf.WriteString(a.Key)
		h.buf.WriteByte('=')
		h.buf.WriteString(a.Value.String())
		return true
	})
	h.buf.WriteByte('\n')
	return nil
}

func (h *memHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := &memHandler{level: h.level, buf: h.buf}
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return next
}

func (h *memHandler) WithGroup(name string) slog.Handler { return h }

func TestReadFile_LoggerEmitsDebugElements(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(&memHandler{level: slog.LevelDebug, buf: &buf})

	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "CT_small.dcm")
	_, err := godicom.ReadFile(path, &godicom.ReadOptions{Logger: l})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "DICM prefix found") {
		t.Fatalf("missing DICM log:\n%s", out)
	}
	if !strings.Contains(out, "component=reader") {
		t.Fatalf("missing component=reader:\n%s", out)
	}
	if !strings.Contains(out, "element") || !strings.Contains(out, "tag=") {
		t.Fatalf("missing element logs:\n%s", out)
	}
	if !strings.Contains(out, "transfer syntax") {
		t.Fatalf("missing transfer syntax log:\n%s", out)
	}
}

func TestReadFileContext_UsesContextLogger(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(&memHandler{level: slog.LevelDebug, buf: &buf})
	ctx := godicom.WithLogger(context.Background(), l)

	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "MR_small.dcm")
	_, err := godicom.ReadFileContext(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "DICM prefix found") {
		t.Fatalf("expected context logger output, got:\n%s", buf.String())
	}
}

func TestDefaultLoggerIsQuiet(t *testing.T) {
	prev := godicom.DefaultLogger()
	t.Cleanup(func() { godicom.SetDefaultLogger(prev) })

	godicom.SetDefaultLogger(nil)
	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "CT_small.dcm")
	if _, err := godicom.ReadFile(path, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWriteFile_LoggerEmitsEncoding(t *testing.T) {
	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "CT_small.dcm")
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	l := slog.New(&memHandler{level: slog.LevelDebug, buf: &buf})
	outPath := filepath.Join(t.TempDir(), "out.dcm")
	if err := godicom.WriteFileContext(context.Background(), outPath, ds.Dataset, &godicom.WriteOptions{
		Logger:            l,
		EnforceFileFormat: true,
	}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "component=writer") {
		t.Fatalf("missing writer component:\n%s", out)
	}
	if !strings.Contains(out, "write encoding") {
		t.Fatalf("missing write encoding:\n%s", out)
	}
}

func TestOptionsLoggerOverridesContext(t *testing.T) {
	var ctxBuf, optBuf bytes.Buffer
	ctxL := slog.New(&memHandler{level: slog.LevelDebug, buf: &ctxBuf})
	optL := slog.New(&memHandler{level: slog.LevelDebug, buf: &optBuf})
	ctx := godicom.WithLogger(context.Background(), ctxL)

	path := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "CT_small.dcm")
	_, err := godicom.ReadFileContext(ctx, path, &godicom.ReadOptions{Logger: optL})
	if err != nil {
		t.Fatal(err)
	}
	if ctxBuf.Len() != 0 {
		t.Fatalf("context logger should be unused when Options.Logger set, got:\n%s", ctxBuf.String())
	}
	if !strings.Contains(optBuf.String(), "DICM prefix found") {
		t.Fatalf("options logger unused:\n%s", optBuf.String())
	}
}
