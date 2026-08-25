package pixels_test

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
// golibjpeg and goopenjpeg embed a shared library per platform and cover the six
// desktop GOOS/GOARCH pairs. Everywhere else they still build, and every entry
// point returns an error wrapping ErrUnsupportedPlatform instead of panicking --
// which is the behaviour godicom relies on to stay importable anywhere Go builds.
// Without this, `go test ./...` on such a platform reports failures for the one
// reason the caller cannot do anything about, and the real failures hide among
// them.
//
// It deliberately does not skip when err is nil: a test must still assert on a
// successful encode.
func skipWithoutNativeCodec(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if errors.Is(err, golibjpeg.ErrUnsupportedPlatform) || errors.Is(err, goopenjpeg.ErrUnsupportedPlatform) {
		t.Skipf("no prebuilt codec library for %s/%s: %v", runtime.GOOS, runtime.GOARCH, err)
	}
}
