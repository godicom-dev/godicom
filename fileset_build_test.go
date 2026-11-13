package godicom_test

import (
	"fmt"
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

// privateChain returns a PATIENT -> PRIVATE chain for ds, the two-record
// hierarchy pydicom's add_custom() example uses.
func privateChain(t *testing.T, ds *godicom.FileDataset) *godicom.RecordNode {
	t.Helper()
	patientID, ok := ds.GetString(tag.PatientID)
	if !ok {
		t.Fatal("the instance has no Patient ID")
	}

	topRecord := godicom.NewDataset()
	topRecord.Set(godicom.NewDataElement(tag.DirectoryRecordType, godicom.VRCS, "PATIENT"))
	topRecord.Set(godicom.NewDataElement(tag.PatientID, godicom.VRLO, patientID))
	top := &godicom.RecordNode{Record: topRecord}

	leafRecord := godicom.NewDataset()
	leafRecord.Set(godicom.NewDataElement(tag.DirectoryRecordType, godicom.VRCS, "PRIVATE"))
	leafRecord.Set(godicom.NewDataElement(tag.PrivateRecordUID, godicom.VRUI, "1.2.826.0.1.3680043.8.498.1"))
	leaf := &godicom.RecordNode{Record: leafRecord}

	leaf.Parent = top
	top.Children = []*godicom.RecordNode{leaf}
	return leaf
}

// TestAddCustomPrivateRecord builds the hierarchy a File-set cannot derive: a
// PRIVATE record carries nothing that says where it belongs, so the records
// come from the caller and only the elements that link it to the file are
// filled in.
func TestAddCustomPrivateRecord(t *testing.T) {
	ds := mustRead(t, instancePath("CT_small.dcm"))
	leaf := privateChain(t, ds)

	fs, err := godicom.NewFileSet()
	if err != nil {
		t.Fatal(err)
	}
	inst, err := fs.AddCustom(ds, leaf)
	if err != nil {
		t.Fatalf("AddCustom: %v", err)
	}
	if inst.Node != leaf {
		t.Fatal("the instance is not filed under the supplied leaf")
	}
	if fs.Len() != 1 {
		t.Fatalf("instances = %d, want 1", fs.Len())
	}
	if got := leaf.Parent; got == nil || got.Record == nil || got.Type() != "PATIENT" {
		t.Fatalf("the leaf's parent is %v", got)
	}
	// The three referenced-UID elements are the writer's job, not the
	// caller's: they come from the instance.
	for _, tg := range []godicom.Tag{tag.ReferencedSOPClassUIDInFile, tag.ReferencedSOPInstanceUIDInFile, tag.ReferencedTransferSyntaxUIDInFile} {
		if _, ok := leaf.Record.Get(tg); !ok {
			t.Fatalf("the leaf record is missing %s", tg)
		}
	}

	fs.ID = "GODICOM"
	root := t.TempDir()
	if err := fs.Save(root, nil); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reopened, err := godicom.OpenFileSet(filepath.Join(root, "DICOMDIR"), nil)
	if err != nil {
		t.Fatalf("OpenFileSet: %v", err)
	}
	if reopened.Len() != 1 {
		t.Fatalf("instances after reload = %d, want 1", reopened.Len())
	}
	record := reopened.Instances()[0].Node
	if got := record.Type(); got != "PRIVATE" {
		t.Fatalf("leaf record type = %q, want PRIVATE", got)
	}
	// A private record is numbered by its level: "P" and the depth.
	if got, want := reopened.Instances()[0].FileID(), filepath.Join("PT000000", "P1000000"); got != want {
		t.Fatalf("File ID = %q, want %q", got, want)
	}
	if _, err := os.Stat(reopened.Instances()[0].Path()); err != nil {
		t.Fatalf("the instance was not written: %v", err)
	}
}

// TestAddCustomRefusesADeepChain covers the limit on depth: a File-set is eight
// levels deep, and the leaf is the eighth.
func TestAddCustomRefusesADeepChain(t *testing.T) {
	ds := mustRead(t, instancePath("CT_small.dcm"))
	leaf := privateChain(t, ds)

	// PATIENT -> PRIVATE is two levels; seven more make nine.
	parent := leaf
	for i := 0; i < 7; i++ {
		record := godicom.NewDataset()
		record.Set(godicom.NewDataElement(tag.DirectoryRecordType, godicom.VRCS, "PRIVATE"))
		record.Set(godicom.NewDataElement(tag.PrivateRecordUID, godicom.VRUI, fmt.Sprintf("1.2.826.0.1.3680043.8.498.%d", 100+i)))
		node := &godicom.RecordNode{Record: record, Parent: parent}
		parent.Children = []*godicom.RecordNode{node}
		parent = node
	}

	fs, err := godicom.NewFileSet()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.AddCustom(ds, parent); err == nil || !strings.Contains(err.Error(), "levels down") {
		t.Fatalf("err = %v", err)
	}
	// The tree is untouched by the refusal.
	if len(fs.Tree.Children) != 0 || fs.Len() != 0 {
		t.Fatalf("the refusal left %d records and %d instances", len(fs.Tree.Children), fs.Len())
	}
}

// TestAddCustomRefusesARecordWithChildren covers the leaf half of the same
// rule: the record an instance is filed under cannot have records below it.
func TestAddCustomRefusesARecordWithChildren(t *testing.T) {
	ds := mustRead(t, instancePath("CT_small.dcm"))
	leaf := privateChain(t, ds)
	leaf.Children = []*godicom.RecordNode{{Record: godicom.NewDataset()}}

	fs, err := godicom.NewFileSet()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.AddCustom(ds, leaf); err == nil || !strings.Contains(err.Error(), "records below it") {
		t.Fatalf("err = %v", err)
	}
}

// TestCopyTo copies a File-set read from disk and leaves the original alone,
// which is the point of a copy: the source can still be written afterwards.
func TestCopyTo(t *testing.T) {
	fs := mustFileSet(t, "DICOMDIR")
	want := sopList(t, fs)
	before := make([]string, len(fs.Instances()))
	beforeFiles := make([]string, len(fs.Instances()))
	for i, inst := range fs.Instances() {
		before[i] = strings.Join(mustReferencedFileID(t, inst), `\`)
		beforeFiles[i] = inst.Path()
	}

	dest := filepath.Join(t.TempDir(), "copy")
	copied, err := fs.CopyTo(dest, nil)
	if err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	if got := sopList(t, copied); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("instances changed:/n got %v\nwant %v", got, want)
	}
	if copied.RootPath != dest {
		t.Fatalf("the copy's root is %s, want %s", copied.RootPath, dest)
	}
	for _, inst := range copied.Instances() {
		if _, err := os.Stat(inst.Path()); err != nil {
			t.Fatalf("the copy is missing %s: %v", inst.FileID(), err)
		}
		loaded, err := inst.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if sop, _ := loaded.GetString(tag.SOPInstanceUID); sop != mustSOP(t, inst) {
			t.Fatalf("the copy holds %s, want %s", sop, mustSOP(t, inst))
		}
	}

	// The original: same File IDs, same records, same files on disk.
	for i, inst := range fs.Instances() {
		if got := strings.Join(mustReferencedFileID(t, inst), `\`); got != before[i] {
			t.Fatalf("copying changed the original's Referenced File ID to %q, want %q", got, before[i])
		}
		if _, err := os.Stat(beforeFiles[i]); err != nil {
			t.Fatalf("copying moved the original's file: %v", err)
		}
	}
}

// TestCopyToRefuses covers the two destinations a copy will not write into:
// its own root, and a directory that already holds a File-set.
func TestCopyToRefuses(t *testing.T) {
	fs := mustFileSet(t, "DICOMDIR")
	if _, err := fs.CopyTo(fs.RootPath, nil); err == nil || !strings.Contains(err.Error(), "already its root") {
		t.Fatalf("copying onto itself: err = %v", err)
	}
	if _, err := fs.CopyTo("", nil); err == nil || !strings.Contains(err.Error(), "without a destination") {
		t.Fatalf("copying to nowhere: err = %v", err)
	}

	dest := filepath.Join(t.TempDir(), "copy")
	if _, err := fs.CopyTo(dest, nil); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	if _, err := fs.CopyTo(dest, nil); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("copying over a File-set: err = %v", err)
	}
}

// mustReferencedFileID returns the record's Referenced File ID components.
func mustReferencedFileID(t *testing.T, inst *godicom.FileInstance) []string {
	t.Helper()
	elem, ok := inst.Node.Record.Get(tag.ReferencedFileID)
	if !ok {
		t.Fatal("the record has no Referenced File ID")
	}
	switch v := elem.Value.(type) {
	case string:
		return strings.Split(v, `\`)
	case []string:
		return v
	default:
		return strings.Split(fmt.Sprint(v), `\`)
	}
}

// mustSOP returns the instance's SOP Instance UID.
func mustSOP(t *testing.T, inst *godicom.FileInstance) string {
	t.Helper()
	sop, ok := inst.SOPInstanceUID()
	if !ok {
		t.Fatal("the instance has no SOP Instance UID")
	}
	return sop
}

// TestSaveAFileSetWithOrphans writes back a DICOMDIR whose links were broken.
// The instances the orphaned records named were rebuilt into the tree by the
// reader, so saving keeps every one of them where the files still are.
func TestSaveAFileSetWithOrphans(t *testing.T) {
	fixture := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "dicomdirtests")
	root := filepath.Join(t.TempDir(), "fileset")
	copyDir(t, fixture, root)
	path := filepath.Join(root, "DICOMDIR")

	ds := mustRead(t, path)
	seq, ok := ds.GetSequence(tag.DirectoryRecordSequence)
	if !ok || seq.Len() == 0 {
		t.Fatal("the fixture has no directory records")
	}
	top := seq.Get(0)
	top.Set(godicom.NewDataElement(tag.OffsetOfReferencedLowerLevelDirectoryEntity, godicom.VRUL, uint64(0)))

	fs, err := godicom.LoadFileSet(ds, nil)
	if err != nil {
		t.Fatalf("LoadFileSet: %v", err)
	}
	if fs.Len() != 31 {
		t.Fatalf("instances = %d, want 31", fs.Len())
	}
	if err := fs.Save("", &godicom.FileSetWriteOptions{UseExisting: true}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if fs.Len() != 31 {
		t.Fatalf("instances after save = %d, want 31", fs.Len())
	}

	reopened, err := godicom.OpenFileSet(path, nil)
	if err != nil {
		t.Fatalf("OpenFileSet after save: %v", err)
	}
	if reopened.Len() != 31 {
		t.Fatalf("instances after reload = %d, want 31", reopened.Len())
	}
	for _, inst := range reopened.Instances() {
		if _, err := os.Stat(inst.Path()); err != nil {
			t.Fatalf("%s: %v", inst.FileID(), err)
		}
	}
}
