package godicom_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/tag"
)

func instancePath(name string) string {
	return filepath.Join("pydicom", "src", "pydicom", "data", "test_files", name)
}

func mustRead(t *testing.T, path string) *godicom.FileDataset {
	t.Helper()
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return ds
}

// TestNewFileSetAddAndSave builds a File-set from two instances and reads it
// back, which is the whole path: the records, the File IDs they imply, the
// files copied to those File IDs and the DICOMDIR that names them.
func TestNewFileSetAddAndSave(t *testing.T) {
	fs, err := godicom.NewFileSet()
	if err != nil {
		t.Fatalf("NewFileSet: %v", err)
	}
	if fs.UID == "" {
		t.Fatal("a new File-set has no UID")
	}
	if fs.Len() != 0 {
		t.Fatalf("a new File-set holds %d instances", fs.Len())
	}

	first, err := fs.Add(mustRead(t, instancePath("CT_small.dcm")))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := fs.AddFile(instancePath("MR_small.dcm")); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	fs.ID = "GODICOM"
	if fs.Len() != 2 {
		t.Fatalf("instances = %d, want 2", fs.Len())
	}
	if first.Node.Type() != "IMAGE" {
		t.Fatalf("leaf record type = %q, want IMAGE", first.Node.Type())
	}
	if got := len(fs.Tree.Children); got != 2 {
		t.Fatalf("top-level records = %d, want one PATIENT each", got)
	}

	root := t.TempDir()
	if err := fs.Save(root, nil); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reopened, err := godicom.OpenFileSet(filepath.Join(root, "DICOMDIR"), nil)
	if err != nil {
		t.Fatalf("OpenFileSet: %v", err)
	}
	if reopened.Len() != 2 {
		t.Fatalf("instances after reload = %d, want 2", reopened.Len())
	}
	if reopened.ID != "GODICOM" {
		t.Fatalf("File-set ID = %q, want GODICOM", reopened.ID)
	}
	if reopened.UID != fs.UID {
		t.Fatalf("File-set UID = %s, want %s", reopened.UID, fs.UID)
	}
	for _, inst := range reopened.Instances() {
		path := inst.Path()
		if !strings.HasPrefix(path, root) {
			t.Fatalf("instance path %s is outside the File-set root %s", path, root)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("instance file missing: %v", err)
		}
		loaded, err := inst.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		sop, _ := inst.SOPInstanceUID()
		loadedSOP, _ := loaded.GetString(tag.SOPInstanceUID)
		if sop != loadedSOP {
			t.Fatalf("instance %s loads %s", sop, loadedSOP)
		}
	}

	// The two instances have different patients, so the File-set must have two
	// PATIENT records, each with the full chain below it.
	for _, patient := range reopened.Tree.Children {
		if got := patient.Type(); got != "PATIENT" {
			t.Fatalf("top-level record = %q, want PATIENT", got)
		}
		if len(patient.Children) != 1 || patient.Children[0].Type() != "STUDY" {
			t.Fatalf("PATIENT has %d children", len(patient.Children))
		}
	}
	for _, inst := range reopened.Instances() {
		if got := inst.FileID(); !godicom.IsConformantFileID(strings.Split(filepath.ToSlash(got), "/")) {
			t.Fatalf("File ID %q is not conformant", got)
		}
	}
	if got := reopened.Instances()[0].FileID(); !strings.HasPrefix(got, "PT") {
		t.Fatalf("File ID %q does not start with PT", got)
	}
}

// TestAddTheSameInstanceTwice covers the dedup: a File-set holds one record per
// instance, so adding the same SOP Instance UID again returns what is already
// there.
func TestAddTheSameInstanceTwice(t *testing.T) {
	fs, err := godicom.NewFileSet()
	if err != nil {
		t.Fatal(err)
	}
	first, err := fs.Add(mustRead(t, instancePath("CT_small.dcm")))
	if err != nil {
		t.Fatal(err)
	}
	again, err := fs.Add(mustRead(t, instancePath("CT_small.dcm")))
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Fatal("the same instance was added twice")
	}
	if fs.Len() != 1 {
		t.Fatalf("instances = %d, want 1", fs.Len())
	}
}

// TestAddInstancesOfOneSeriesShareTheirRecords checks that a second instance of
// an existing series joins it instead of building a second PATIENT/STUDY/SERIES
// chain.
func TestAddInstancesOfOneSeriesShareTheirRecords(t *testing.T) {
	fs, err := godicom.NewFileSet()
	if err != nil {
		t.Fatal(err)
	}
	ct := mustRead(t, instancePath("CT_small.dcm"))
	if _, err := fs.Add(ct); err != nil {
		t.Fatal(err)
	}
	// The same instance under a new SOP Instance UID: the patient, study and
	// series are the same, so only the leaf is new.
	second := mustRead(t, instancePath("CT_small.dcm"))
	second.Set(godicom.NewDataElement(tag.SOPInstanceUID, godicom.VRUI, "1.2.3.4.5.6.7.8.9"))
	second.Set(godicom.NewDataElement(tag.InstanceNumber, godicom.VRIS, "2"))
	if _, err := fs.Add(second); err != nil {
		t.Fatal(err)
	}

	if got := len(fs.Tree.Children); got != 1 {
		t.Fatalf("top-level records = %d, want 1", got)
	}
	patient := fs.Tree.Children[0]
	if len(patient.Children) != 1 {
		t.Fatalf("study records = %d, want 1", len(patient.Children))
	}
	series := patient.Children[0].Children[0]
	if len(series.Children) != 2 {
		t.Fatalf("image records = %d, want 2", len(series.Children))
	}
	if got, _ := series.Children[1].Record.GetString(tag.ReferencedFileID); got != "" {
		t.Fatalf("a new record already has a Referenced File ID: %q", got)
	}
}

// TestRemoveInstance takes an instance out and prunes the records it was the
// only child of.
func TestRemoveInstance(t *testing.T) {
	fs, err := godicom.NewFileSet()
	if err != nil {
		t.Fatal(err)
	}
	inst, err := fs.Add(mustRead(t, instancePath("CT_small.dcm")))
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Remove(inst); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if fs.Len() != 0 {
		t.Fatalf("instances = %d, want 0", fs.Len())
	}
	if len(fs.Tree.Children) != 0 {
		t.Fatalf("records left behind: %d", len(fs.Tree.Children))
	}
	if err := fs.Remove(inst); err == nil || !strings.Contains(err.Error(), "not in this File-set") {
		t.Fatalf("removing twice: err = %v", err)
	}
}

// TestRemoveRefusesARecordWithChildren keeps the tree consistent: a PATIENT
// record cannot go while a STUDY still hangs from it.
func TestRemoveRefusesARecordWithChildren(t *testing.T) {
	fs, err := godicom.NewFileSet()
	if err != nil {
		t.Fatal(err)
	}
	inst, err := fs.Add(mustRead(t, instancePath("CT_small.dcm")))
	if err != nil {
		t.Fatal(err)
	}
	patient := fs.Tree.Children[0]
	parent := &godicom.FileInstance{Node: patient}
	if err := fs.Remove(parent); err == nil || !strings.Contains(err.Error(), "still has records below it") {
		t.Fatalf("err = %v", err)
	}
	if fs.Len() != 1 {
		t.Fatalf("instances = %d, want 1", fs.Len())
	}
	_ = inst
}

// TestSaveRefusesToMoveAnExistingFileSet covers the root: an existing File-set
// is written in place, not moved.
func TestSaveRefusesToMoveAnExistingFileSet(t *testing.T) {
	fs, err := godicom.NewFileSet()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Add(mustRead(t, instancePath("CT_small.dcm"))); err != nil {
		t.Fatal(err)
	}
	if err := fs.Save(t.TempDir(), nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := fs.Save(t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "cannot change the root") {
		t.Fatalf("err = %v", err)
	}
}

// TestSaveANewFileSetNeedsARoot covers the other end of the same rule.
func TestSaveANewFileSetNeedsARoot(t *testing.T) {
	fs, err := godicom.NewFileSet()
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Save("", nil); err == nil || !strings.Contains(err.Error(), "without a root") {
		t.Fatalf("err = %v", err)
	}
}

// TestSaveExistingLayoutKeepsTheFiles covers UseExisting: the DICOMDIR is
// rewritten and the files stay where they were.
func TestSaveExistingLayoutKeepsTheFiles(t *testing.T) {
	fixture := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "dicomdirtests")
	root := filepath.Join(t.TempDir(), "fileset")
	copyDir(t, fixture, root)

	fs, err := godicom.OpenFileSet(filepath.Join(root, "DICOMDIR"), nil)
	if err != nil {
		t.Fatalf("OpenFileSet: %v", err)
	}
	paths := make([]string, 0, fs.Len())
	for _, inst := range fs.Instances() {
		paths = append(paths, inst.Path())
	}
	if err := fs.Save("", &godicom.FileSetWriteOptions{UseExisting: true}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("the file moved: %v", err)
		}
	}
	if fs.Len() != 31 {
		t.Fatalf("instances after save = %d, want 31", fs.Len())
	}
}
