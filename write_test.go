package godicom

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileReadback(t *testing.T) {
	src := testFilePath("CT_small.dcm")
	ds, err := ReadFile(src, nil)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "output.dcm")
	err = ds.SaveAs(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file exists and has content
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Error("output file is empty")
	}

	// Read back
	ds2, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds2.Len() != ds.Len() {
		t.Errorf("element count mismatch: %d vs %d", ds2.Len(), ds.Len())
	}
}

func TestWriteFileImplicitVR(t *testing.T) {
	src := testFilePath("CT_small.dcm")
	ds, err := ReadFile(src, nil)
	if err != nil {
		t.Fatal(err)
	}

	implicit := true
	opts := &WriteOptions{ImplicitVR: &implicit}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "implicit.dcm")
	err = ds.SaveAs(outPath, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Read back with force (no file meta may cause issues)
	ds2, err := ReadFile(outPath, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if ds2.Len() == 0 {
		t.Error("no elements read back")
	}
}

func TestWriteFileEmptyDataset(t *testing.T) {
	ds := NewDataset()
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "empty.dcm")
	err := ds.SaveAs(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWriteFileSequence(t *testing.T) {
	ds := NewDataset()
	item := NewDataset()
	item.Set(NewDataElement(MustTag(0x00100010), VRPN, "SeqPatient"))
	seq := NewSequence([]*Dataset{item})
	ds.Set(NewDataElement(MustTag(0x00321060), VRSQ, seq))

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "seq.dcm")
	err := ds.SaveAs(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWriteFileAllVRTypes(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00080005), VRCS, "ISO_IR 100"))
	ds.Set(NewDataElement(MustTag(0x00100010), VRPN, "Test^Patient"))
	ds.Set(NewDataElement(MustTag(0x00100020), VRLO, "ID123"))
	ds.Set(NewDataElement(MustTag(0x00100030), VRDA, "20000101"))
	ds.Set(NewDataElement(MustTag(0x00280010), VRUS, 512))
	ds.Set(NewDataElement(MustTag(0x00280011), VRUS, 512))
	ds.Set(NewDataElement(MustTag(0x00280100), VRUS, 8))
	ds.Set(NewDataElement(MustTag(0x00280101), VRUS, 8))
	ds.Set(NewDataElement(MustTag(0x00280002), VRUS, 1))
	ds.Set(NewDataElement(MustTag(0x00280004), VRCS, "MONOCHROME2"))
	ds.Set(NewDataElement(MustTag(0x7FE00010), VROB, []byte{0, 0, 0, 0}))

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "all_vr.dcm")
	err := ds.SaveAs(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	ds2, err := ReadFile(outPath, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if ds2.Len() != ds.Len() {
		t.Errorf("element count: %d vs %d", ds2.Len(), ds.Len())
	}
}
