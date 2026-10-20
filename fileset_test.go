package godicom_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

func dicomdirPath(name string) string {
	return filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "dicomdirtests", name)
}

func TestOpenFileSet(t *testing.T) {
	fs := mustFileSet(t, "DICOMDIR")
	if fs.Len() != 31 {
		t.Fatalf("instances = %d, want 31", fs.Len())
	}
	if got := len(fs.Find(map[godicom.Tag]string{tag.PatientID: "77654033"})); got != 7 {
		t.Fatalf("PatientID 77654033 = %d, want 7", got)
	}
	if got := len(fs.Find(map[godicom.Tag]string{tag.PatientID: "98890234"})); got != 24 {
		t.Fatalf("PatientID 98890234 = %d, want 24", got)
	}
	matches := fs.Find(map[godicom.Tag]string{
		tag.PatientID: "98890234",
		tag.StudyDate: "20030505",
	})
	if len(matches) != 17 {
		t.Fatalf("study matches = %d, want 17", len(matches))
	}
	seen := map[string]bool{}
	for _, inst := range matches {
		sop, ok := inst.SOPInstanceUID()
		if !ok {
			t.Fatal("match missing SOP Instance UID")
		}
		seen[sop] = true
	}
	if len(seen) != 17 {
		t.Fatalf("unique SOP Instance UIDs = %d, want 17", len(seen))
	}

	inst := fs.Instances()[0]
	if !strings.Contains(inst.Path(), filepath.Join("77654033", "CR1", "6154")) {
		t.Fatalf("path = %s", inst.Path())
	}
	if !fileExists(inst.Path()) {
		t.Fatalf("referenced file missing: %s", inst.Path())
	}
	var types []string
	for n := inst.Node.Parent; n != nil && n.Record != nil; n = n.Parent {
		types = append(types, n.Type())
	}
	if strings.Join(types, ",") != "SERIES,STUDY,PATIENT" {
		t.Fatalf("ancestors = %v", types)
	}
	sop, _ := inst.SOPInstanceUID()
	if sop != "1.3.6.1.4.1.5962.1.1.0.0.0.1196527414.5534.0.11" {
		t.Fatalf("SOP Instance UID = %s", sop)
	}
	class, _ := inst.SOPClassUID()
	if class != "1.2.840.10008.5.1.4.1.1.1" {
		t.Fatalf("SOP Class UID = %s", class)
	}
	ts, _ := inst.TransferSyntaxUID()
	if ts != string(uid.ExplicitVRLittleEndian) {
		t.Fatalf("Transfer Syntax = %s", ts)
	}

	seq, ok := fs.Dataset.GetSequence(tag.DirectoryRecordSequence)
	if !ok || seq.Len() == 0 {
		t.Fatal("missing directory records")
	}
	first := seq.Get(0).ItemOffset
	got, ok := fs.Dataset.GetInt(tag.OffsetOfTheFirstDirectoryRecordOfTheRootDirectoryEntity)
	if !ok || int64(got) != first {
		t.Fatalf("first record offset = %d, file says %d", first, got)
	}
}

func TestFileSetSameInstances(t *testing.T) {
	ref := sopList(t, mustFileSet(t, "DICOMDIR"))
	for _, name := range []string{"DICOMDIR-reordered", "DICOMDIR-nooffset", "DICOMDIR-bigEnd", "DICOMDIR-implicit"} {
		got := sopList(t, mustFileSet(t, name))
		if strings.Join(got, ",") != strings.Join(ref, ",") {
			t.Fatalf("%s SOP Instance UIDs differ", name)
		}
	}
}

func TestFileSetBadSOPClass(t *testing.T) {
	ds := mustDICOMDIR(t, "DICOMDIR")
	ds.FileMeta.Set(godicom.NewDataElement(tag.MediaStorageSOPClassUID, godicom.VRUI, "1.2.3"))
	_, err := godicom.LoadFileSet(ds, nil)
	if err == nil || !strings.Contains(err.Error(), "not a 'Media Storage Directory' instance") {
		t.Fatalf("err = %v", err)
	}
}

func TestFileSetBadFilename(t *testing.T) {
	ds := mustDICOMDIR(t, "DICOMDIR")
	ds.Filename = "bad"
	_, err := godicom.LoadFileSet(ds, nil)
	if err == nil || !strings.Contains(err.Error(), "not a valid path") {
		t.Fatalf("err = %v", err)
	}
	ds.Filename = ""
	_, err = godicom.LoadFileSet(ds, nil)
	if err == nil || !strings.Contains(err.Error(), "filename") {
		t.Fatalf("empty filename err = %v", err)
	}
}

func TestFileSetEmptyFileID(t *testing.T) {
	ds := mustDICOMDIR(t, "DICOMDIR")
	seq, _ := ds.GetSequence(tag.DirectoryRecordSequence)
	item := seq.Get(5)
	uidValue, _ := item.GetString(tag.ReferencedSOPInstanceUIDInFile)
	item.Set(godicom.NewDataElement(tag.ReferencedFileID, godicom.VRCS, ""))
	fs, err := godicom.LoadFileSet(ds, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fs.Len() != 30 {
		t.Fatalf("instances = %d, want 30", fs.Len())
	}
	if len(fs.Find(map[godicom.Tag]string{tag.SOPInstanceUID: uidValue})) != 0 {
		t.Fatal("empty File ID instance was kept")
	}
}

func TestFileSetMissingFile(t *testing.T) {
	ds := mustDICOMDIR(t, "DICOMDIR")
	seq, _ := ds.GetSequence(tag.DirectoryRecordSequence)
	item := seq.Get(5)
	uidValue, _ := item.GetString(tag.ReferencedSOPInstanceUIDInFile)
	elem, ok := item.Get(tag.ReferencedFileID)
	if !ok {
		t.Fatal("missing Referenced File ID")
	}
	raw, ok := elem.Value.(string)
	if !ok {
		t.Fatalf("Referenced File ID type %T", elem.Value)
	}
	parts := strings.Split(raw, `\`)
	parts[len(parts)-1] = "MISSING"
	item.Set(godicom.NewDataElement(tag.ReferencedFileID, godicom.VRCS, strings.Join(parts, `\`)))
	fs, err := godicom.LoadFileSet(ds, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fs.Len() != 30 {
		t.Fatalf("instances = %d, want 30", fs.Len())
	}
	if len(fs.Find(map[godicom.Tag]string{tag.SOPInstanceUID: uidValue})) != 0 {
		t.Fatal("missing file was kept")
	}
}

func TestFileSetOrphans(t *testing.T) {
	fs := mustFileSet(t, "DICOMDIR")
	patient := fs.Tree.Children[0]
	if patient.Type() != "PATIENT" {
		t.Fatalf("root child = %s", patient.Type())
	}
	dropped := countInstances(patient)
	patient.Record.Set(godicom.NewDataElement(tag.OffsetOfReferencedLowerLevelDirectoryEntity, godicom.VRUL, uint64(0)))

	_, err := godicom.LoadFileSet(fs.Dataset, &godicom.FileSetOptions{RaiseOrphans: true})
	if err == nil || !strings.Contains(err.Error(), "orphaned directory records") {
		t.Fatalf("err = %v", err)
	}
	loaded, err := godicom.LoadFileSet(fs.Dataset, nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Len() != fs.Len()-dropped {
		t.Fatalf("instances = %d, want %d", loaded.Len(), fs.Len()-dropped)
	}
}

func TestFileSetEmpty(t *testing.T) {
	fs := mustFileSet(t, "DICOMDIR-empty.dcm")
	if fs.Len() != 0 {
		t.Fatalf("instances = %d, want 0", fs.Len())
	}
}

func countInstances(n *godicom.RecordNode) int {
	if n == nil {
		return 0
	}
	total := 0
	if n.Instance != nil {
		total++
	}
	for _, child := range n.Children {
		total += countInstances(child)
	}
	return total
}

func TestFileSetFileIDEscapesRoot(t *testing.T) {
	ds := mustDICOMDIR(t, "DICOMDIR")
	seq, _ := ds.GetSequence(tag.DirectoryRecordSequence)
	seq.Get(5).Set(godicom.NewDataElement(tag.ReferencedFileID, godicom.VRCS, `..\secret`))
	_, err := godicom.LoadFileSet(ds, nil)
	if err == nil || !strings.Contains(err.Error(), "must be inside the DICOMDIR root path") {
		t.Fatalf("err = %v", err)
	}
}

func mustFileSet(t *testing.T, name string) *godicom.FileSet {
	t.Helper()
	fs, err := godicom.OpenFileSet(dicomdirPath(name), nil)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func mustDICOMDIR(t *testing.T, name string) *godicom.FileDataset {
	t.Helper()
	ds, err := godicom.ReadFile(dicomdirPath(name), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func sopList(t *testing.T, fs *godicom.FileSet) []string {
	t.Helper()
	out := make([]string, 0, fs.Len())
	for _, inst := range fs.Instances() {
		sop, ok := inst.SOPInstanceUID()
		if !ok {
			t.Fatal("instance missing SOP Instance UID")
		}
		out = append(out, sop)
	}
	return out
}
