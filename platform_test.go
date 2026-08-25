package godicom_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/godicom-dev/golibjpeg"
	"github.com/godicom-dev/goopenjpeg"
)

// skipWithoutNativeCodec turns "this platform has no prebuilt codec library" into
// a skip rather than a failure.
//
// golibjpeg and goopenjpeg embed a shared library per platform and between them
// cover the six desktop GOOS/GOARCH pairs. Everywhere else they still build, and
// every entry point returns an error wrapping ErrUnsupportedPlatform rather than
// panicking -- which is what lets a program import godicom on a platform with no
// prebuilt library. Without this guard `go test ./...` on such a platform reports
// a dozen failures for the one reason the caller cannot do anything about, and
// any real failure hides among them.
//
// It cannot mask a regression on a supported platform: ErrUnsupportedPlatform is
// only ever returned where no library was embedded, which on the six covered
// pairs is never.
//
// It deliberately does not skip when err is nil, so a test still asserts on a
// successful decode.
func skipWithoutNativeCodec(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if errors.Is(err, golibjpeg.ErrUnsupportedPlatform) || errors.Is(err, goopenjpeg.ErrUnsupportedPlatform) {
		t.Skipf("no prebuilt codec library for %s/%s: %v", runtime.GOOS, runtime.GOARCH, err)
	}
}
