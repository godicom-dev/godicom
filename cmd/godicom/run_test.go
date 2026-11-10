package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
)

// captureStdout runs fn with os.Stdout redirected to a temporary file and
// returns what it wrote. The commands print to os.Stdout directly, and the point
// of these tests is the entry points themselves -- the formatting is covered by
// writeShow's own tests -- so the redirect is what makes the wiring testable
// without a subprocess.
//
// A file rather than a pipe: nothing reads a pipe until fn returns, so on a
// platform whose pipe buffer is smaller than the output -- 4 KiB on Windows --
// the writer blocks and the test hangs instead of failing.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}

	old := os.Stdout
	os.Stdout = f
	// Restored twice: right after fn so nothing below writes into a closed file,
	// and again on the way out in case fn panics and skips the first one.
	defer func() { os.Stdout = old }()
	fn()
	os.Stdout = old

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRunShowWiring(t *testing.T) {
	path := cliTestFile("MR_small.dcm")
	out := captureStdout(t, func() { runShow([]string{path}) })
	if !strings.Contains(out, "File: ") || !strings.Contains(out, "Elements: ") {
		t.Fatalf("runShow output:\n%s", out)
	}
	if !strings.Contains(out, "--- File Meta ---") {
		t.Fatalf("runShow should print file meta by default:\n%s", out)
	}

	filtered := captureStdout(t, func() {
		runShow([]string{"-no-meta", "-t", "PatientName", path})
	})
	if strings.Contains(filtered, "--- File Meta ---") {
		t.Fatalf("-no-meta still printed file meta:\n%s", filtered)
	}
	if !strings.Contains(filtered, "Matching elements: 1") {
		t.Fatalf("filter did not narrow the output:\n%s", filtered)
	}

	// -tag is an alias for -t, and both accumulate.
	both := captureStdout(t, func() {
		runShow([]string{"-no-meta", "-t", "PatientName", "-tag", "Rows", path})
	})
	if !strings.Contains(both, "Matching elements: 2") {
		t.Fatalf("-t and -tag should accumulate:\n%s", both)
	}
}

func TestRunShowRejectsUnknownKeyword(t *testing.T) {
	// An unparsable -t value ends the run through os.Exit, so it cannot be called
	// in-process; what can be checked here is that parseShowTags is what rejects
	// it, which runShow relies on before reading the file.
	if _, err := parseShowTags([]string{"NotAKeyword"}); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRunReadCopyRoundtrip(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "copy.dcm")
	src := cliTestFile("MR_small.dcm")

	out := captureStdout(t, func() { runReadCopy([]string{src, dst}) })
	if !strings.Contains(out, "Read ") || !strings.Contains(out, "Written to ") {
		t.Fatalf("runReadCopy output:\n%s", out)
	}
	if !strings.Contains(out, "Re-read ") {
		t.Fatalf("runReadCopy should re-read what it wrote:\n%s", out)
	}

	original, err := godicom.ReadFile(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := godicom.ReadFile(dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Len() != original.Len() {
		t.Fatalf("element count %d, want %d", copied.Len(), original.Len())
	}
	name, ok := copied.GetString(godicom.MustTag("PatientName"))
	wantName, _ := original.GetString(godicom.MustTag("PatientName"))
	if !ok || name != wantName {
		t.Fatalf("name %q, want %q", name, wantName)
	}
}
