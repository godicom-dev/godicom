package godicom

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// failingDataset returns a FileDataset the writer rejects part way through.
// validateWriteSource passes -- the File Meta is complete and no group 0002
// element sits in the dataset -- so the failure comes from encoding an element.
// SliceThickness is a DS, and NaN has no DS spelling; every element before it in
// tag order, PatientName included, has already reached the writer by then.
func failingDataset(t *testing.T) *FileDataset {
	t.Helper()
	ds := NewDataset()
	_ = ds.SetString(MustTag("PatientName"), "Doe^John")
	ds.Set(NewDataElement(MustTag("SliceThickness"), VRDS, math.NaN()))
	return samplePart10With(t, ds, ExplicitVRLittleEndian)
}

// hookedDataset fails the same way through the other route: the value is
// written as it stands unless the hook objects, and this hook objects.
func hookedDataset(t *testing.T) *FileDataset {
	t.Helper()
	ds := NewDataset()
	_ = ds.SetString(MustTag("PatientName"), "Doe^John")
	ds.Set(NewDataElement(MustTag("EchoNumbers"), VRIS, 1.5))
	return samplePart10With(t, ds, ExplicitVRLittleEndian)
}

// strictHook refuses whatever the writer offers it, which is the only way to
// make a write fail after os.Create without inventing a broken disk.
func strictHook() *WriteOptions {
	return &WriteOptions{OnDiagnostic: func(d Diagnostic) error { return d }}
}

// goodDataset numbers its elements and their values so that a partial file is
// distinguishable from a complete one by more than its length.
func goodDataset(t *testing.T, name string) *FileDataset {
	t.Helper()
	ds := NewDataset()
	_ = ds.SetString(MustTag("PatientName"), name)
	_ = ds.SetString(MustTag("PatientID"), "ID-"+name)
	return samplePart10With(t, ds, ExplicitVRLittleEndian)
}

// TestSaveAsFailureKeepsTheExistingFile is the reason the write goes through a
// temporary file: both errors below are raised while the dataset is being
// encoded, which is long after the file was opened, and the file they would
// have truncated is still the one that was there before.
func TestSaveAsFailureKeepsTheExistingFile(t *testing.T) {
	cases := []struct {
		name string
		fd   func(*testing.T) *FileDataset
		opts *WriteOptions
		want string
	}{
		{"value the VR cannot hold", failingDataset, nil, "NaN has no valid DS representation"},
		{"strict diagnostic hook", hookedDataset, strictHook(), "is not an integer string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "report.dcm")
			if err := goodDataset(t, "Doe^John").SaveAs(path, nil); err != nil {
				t.Fatalf("SaveAs() error = %v", err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			err = tc.fd(t).SaveAs(path, tc.opts)
			if err == nil {
				t.Fatal("SaveAs() succeeded; the dataset is supposed to be unwritable")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}

			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("the file a failed write should have left alone is gone: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("a failed write changed %s: %d bytes before, %d after", path, len(before), len(after))
			}
			back, err := ReadFile(path, nil)
			if err != nil {
				t.Fatalf("the surviving file does not read back: %v", err)
			}
			if v, _ := back.GetString(MustTag("PatientName")); v != "Doe^John" {
				t.Errorf("PatientName = %q, want the value from the earlier write", v)
			}
		})
	}
}

// TestSaveAsFailureLeavesNoFile covers the same failure where there was nothing
// to preserve: a new path must not appear at all, and neither must the temporary
// file the write was using.
func TestSaveAsFailureLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.dcm")

	if err := failingDataset(t).SaveAs(path, nil); err == nil {
		t.Fatal("SaveAs() succeeded; the dataset is supposed to be unwritable")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a file exists where the write failed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("the directory holds %d entries after a failed write: %v", len(entries), names)
	}
}

// TestSaveAsLeavesNoTemporaryFile checks the other half: a write that succeeds
// renames its temporary file rather than leaving it beside the target.
func TestSaveAsLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.dcm")

	if err := goodDataset(t, "Doe^John").SaveAs(path, nil); err != nil {
		t.Fatalf("SaveAs() error = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "report.dcm" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory holds %v, want only report.dcm", names)
	}
}

// TestSaveAsKeepsTheFileMode checks that the rename does not quietly narrow the
// mode. os.Create keeps the mode of a file it truncates, so a file that was 0600
// has to stay 0600 -- a temporary file created 0600 and chmod'ed would be fine,
// but one created 0666 would widen it to whatever the umask allows.
func TestSaveAsKeepsTheFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports 0666 or 0444, not the bits that were set")
	}
	path := filepath.Join(t.TempDir(), "report.dcm")
	if err := os.WriteFile(path, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := goodDataset(t, "Doe^John").SaveAs(path, nil); err != nil {
		t.Fatalf("SaveAs() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %04o after the write, want 0600", got)
	}
}

// TestSaveAsFollowsASymlink checks the path that does not go through a rename. A
// rename replaces the name, so writing through a symlink that way would leave a
// regular file where the caller had a link -- and would silently stop updating
// whatever the link pointed at.
//
// The target starts out holding an older dataset, so the write has something to
// overwrite rather than a zero-length file, and os.SameFile says whether the
// file the link pointed at before the write is still the file it points at
// after. A rename would have swapped in a different file and this would be
// false; writing through leaves the same file with new contents.
func TestSaveAsFollowsASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink needs a privilege the test runner may not have")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real.dcm")
	link := filepath.Join(dir, "link.dcm")
	if err := goodDataset(t, "Old^Name").SaveAs(real, nil); err != nil {
		t.Fatalf("SaveAs(real) error = %v", err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	before, err := os.Stat(real)
	if err != nil {
		t.Fatal(err)
	}

	if err := goodDataset(t, "New^Name").SaveAs(link, nil); err != nil {
		t.Fatalf("SaveAs(link) error = %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
	after, err := os.Stat(real)
	if err != nil {
		t.Fatalf("the link's target is gone: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Error("the write replaced the target instead of writing into it")
	}
	back, err := ReadFile(real, nil)
	if err != nil {
		t.Fatalf("the link's target does not read back as a DICOM file: %v", err)
	}
	if v, _ := back.GetString(MustTag("PatientName")); v != "New^Name" {
		t.Errorf("PatientName = %q, want the value written through the link", v)
	}
	// The in-place path has no temporary file to leave behind, so the directory
	// holds the target and the link and nothing else.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory holds %v, want only real.dcm and link.dcm", names)
	}
}

// TestSaveAsCreatesTheTargetOfADanglingSymlink pins the case where the link
// points at a name that does not exist yet. os.Create follows the link and
// creates the target, which is what pydicom and dcm4che do with the same path;
// nothing replaces the link with a file. It also means the writer follows a link
// through a directory it was not told about, so a chain is only ever as
// writable as its final target.
func TestSaveAsCreatesTheTargetOfADanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink needs a privilege the test runner may not have")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "not-yet.dcm")
	link := filepath.Join(dir, "link.dcm")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := goodDataset(t, "Doe^John").SaveAs(link, nil); err != nil {
		t.Fatalf("SaveAs() error = %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the dangling symlink was replaced by a regular file")
	}
	back, err := ReadFile(real, nil)
	if err != nil {
		t.Fatalf("the write did not create the link's target: %v", err)
	}
	if v, _ := back.GetString(MustTag("PatientName")); v != "Doe^John" {
		t.Errorf("PatientName = %q, want Doe^John", v)
	}
}

// TestSaveAsWritesToADevice checks the other non-regular path. os.DevNull is
// not a file a rename can produce, so the writer has to fall back to writing
// through it -- the point of the test is that SaveAs still returns nil rather
// than failing on a rename.
func TestSaveAsWritesToADevice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.DevNull is a reserved name there, not a path that can be stat'ed")
	}
	if err := goodDataset(t, "Doe^John").SaveAs(os.DevNull, nil); err != nil {
		t.Errorf("SaveAs(%q) error = %v", os.DevNull, err)
	}
}

// TestSaveAsCreatesTheFileBesideTheTarget pins where the temporary file goes:
// the rename has to stay inside one filesystem, so the path handed to
// createSibling has to share a directory with the target rather than being
// os.CreateTemp's default location.
func TestSaveAsCreatesTheFileBesideTheTarget(t *testing.T) {
	dir := t.TempDir()
	f, err := createSibling(filepath.Join(dir, "report.dcm"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if got := filepath.Dir(f.Name()); got != dir {
		t.Errorf("temporary file is in %s, want %s", got, dir)
	}
	if base := filepath.Base(f.Name()); !strings.HasPrefix(base, ".") {
		t.Errorf("temporary file is named %q, want a dot-prefixed name", base)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("mode = %04o, want the 0600 that was asked for", got)
		}
	}
}
