package godicom

import (
	"os"
	"path/filepath"
	"testing"
)

var testDataDir = filepath.Join("pydicom", "src", "pydicom", "data", "test_files")

func testFilePath(name string) string {
	return filepath.Join(testDataDir, name)
}

func TestReadFileCTSmall(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Len() != 264 {
		t.Errorf("expected ~264 elements, got %d", ds.Len())
	}
	// Check key values
	pn, ok := ds.GetString(MustTag(0x00100010))
	if !ok {
		t.Error("PatientName not found")
	}
	if pn != "CompressedSamples^CT1" {
		t.Errorf("PatientName = %q", pn)
	}
	id, ok := ds.GetString(MustTag(0x00100020))
	if !ok {
		t.Error("PatientID not found")
	}
	if id != "1CT1" {
		t.Errorf("PatientID = %q", id)
	}
}

func TestReadFileMRSmall(t *testing.T) {
	ds, err := ReadFile(testFilePath("MR_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Len() != 79 {
		t.Errorf("expected ~79 elements, got %d", ds.Len())
	}
	pn, ok := ds.GetString(MustTag(0x00100010))
	if !ok {
		t.Error("PatientName not found")
	}
	if pn != "CompressedSamples^MR1" {
		t.Errorf("PatientName = %q", pn)
	}
}

func TestReadFileMRImplicit(t *testing.T) {
	ds, err := ReadFile(testFilePath("MR_small_implicit.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Len() != 78 {
		t.Errorf("expected ~78 elements, got %d", ds.Len())
	}
	pn, ok := ds.GetString(MustTag(0x00100010))
	if !ok {
		t.Error("PatientName not found")
	}
	if pn != "CompressedSamples^MR1" {
		t.Errorf("PatientName = %q", pn)
	}
}

func TestReadFileRTPlan(t *testing.T) {
	ds, err := ReadFile(testFilePath("rtplan.dcm"), &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Len() != 38 {
		t.Errorf("expected ~38 elements, got %d", ds.Len())
	}
}

func TestReadFileRTStruct(t *testing.T) {
	ds, err := ReadFile(testFilePath("rtstruct.dcm"), &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Len() != 30 {
		t.Errorf("expected ~30 elements, got %d", ds.Len())
	}
}

func TestReadFileAllTestFiles(t *testing.T) {
	entries, err := os.ReadDir(testDataDir)
	if err != nil {
		t.Skipf("test data directory not found: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".dcm" {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			path := filepath.Join(testDataDir, entry.Name())
			_, err := ReadFile(path, &ReadOptions{Force: true})
			if err != nil {
				t.Errorf("failed to read %s: %v", entry.Name(), err)
			}
		})
	}
}

func TestReadWriteRoundtrip(t *testing.T) {
	// Read, write to temp file, read back, compare element count
	src := testFilePath("CT_small.dcm")
	ds1, err := ReadFile(src, nil)
	if err != nil {
		t.Fatal(err)
	}

	tmpFile := filepath.Join(t.TempDir(), "roundtrip.dcm")
	err = ds1.SaveAs(tmpFile, nil)
	if err != nil {
		t.Fatal(err)
	}

	ds2, err := ReadFile(tmpFile, nil)
	if err != nil {
		t.Fatal(err)
	}

	if ds1.Len() != ds2.Len() {
		t.Errorf("element count mismatch: %d vs %d", ds1.Len(), ds2.Len())
	}
}

func TestReadFileStopBeforePixels(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), &ReadOptions{StopBeforePixels: true})
	if err != nil {
		t.Fatal(err)
	}
	// Should not have pixel data
	if ds.Has(MustTag(0x7FE00010)) {
		t.Error("should not have pixel data")
	}
}

func TestReadFileDeferSize(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), &ReadOptions{DeferSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	// Should still have all elements
	if ds.Len() < 200 {
		t.Errorf("too few elements: %d", ds.Len())
	}
}
