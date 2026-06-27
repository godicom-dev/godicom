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

func TestReadFileDeflatedExplicitVRLittleEndian(t *testing.T) {
	ds, err := ReadFile(testFilePath("image_dfl.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	transferSyntax, ok := ds.FileMeta.Get(MustTag("TransferSyntaxUID"))
	if !ok {
		t.Fatal("TransferSyntaxUID missing")
	}
	if transferSyntax.Value != DeflatedExplicitVRLittleEndian {
		t.Fatalf("TransferSyntaxUID = %v, want %v", transferSyntax.Value, DeflatedExplicitVRLittleEndian)
	}
	conversionType, ok := ds.GetString(MustTag("ConversionType"))
	if !ok {
		t.Fatal("ConversionType missing")
	}
	if conversionType != "WSD" {
		t.Fatalf("ConversionType = %q, want WSD", conversionType)
	}
	if ds.originalEnc.IsImplicitVR {
		t.Fatal("IsImplicitVR = true, want false")
	}
	if !ds.originalEnc.IsLittleEndian {
		t.Fatal("IsLittleEndian = false, want true")
	}
}

func TestReadFileExplicitVRBigEndianNoMeta(t *testing.T) {
	ds, err := ReadFile(testFilePath("ExplVR_BigEndNoMeta.dcm"), &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := ds.GetString(MustTag("InstanceCreationDate"))
	if !ok {
		t.Fatal("InstanceCreationDate missing")
	}
	if got != "20150529" {
		t.Fatalf("InstanceCreationDate = %q, want 20150529", got)
	}
	if ds.originalEnc.IsImplicitVR {
		t.Fatal("IsImplicitVR = true, want false")
	}
	if ds.originalEnc.IsLittleEndian {
		t.Fatal("IsLittleEndian = true, want false")
	}
}

func TestReadFileExplicitVRBigEndianWithMeta(t *testing.T) {
	ds, err := ReadFile(testFilePath("MR_small_bigendian.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	transferSyntax, ok := ds.FileMeta.Get(MustTag("TransferSyntaxUID"))
	if !ok {
		t.Fatal("TransferSyntaxUID missing")
	}
	if transferSyntax.Value != ExplicitVRBigEndian {
		t.Fatalf("TransferSyntaxUID = %v, want %v", transferSyntax.Value, ExplicitVRBigEndian)
	}
	name, ok := ds.GetString(MustTag("PatientName"))
	if !ok {
		t.Fatal("PatientName missing")
	}
	if name != "CompressedSamples^MR1" {
		t.Fatalf("PatientName = %q, want CompressedSamples^MR1", name)
	}
	if ds.originalEnc.IsImplicitVR {
		t.Fatal("IsImplicitVR = true, want false")
	}
	if ds.originalEnc.IsLittleEndian {
		t.Fatal("IsLittleEndian = true, want false")
	}
}

func TestReadFileMetaInfo(t *testing.T) {
	ds, err := ReadFile(testFilePath("rtplan.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.FileMeta == nil {
		t.Fatal("FileMeta is nil")
	}
	if ds.FileMeta.Len() != 6 {
		t.Fatalf("file meta len = %d, want 6", ds.FileMeta.Len())
	}
	transferSyntax, ok := ds.FileMeta.Get(MustTag("TransferSyntaxUID"))
	if !ok {
		t.Fatal("TransferSyntaxUID missing")
	}
	if transferSyntax.Value != ImplicitVRLittleEndian {
		t.Fatalf("TransferSyntaxUID = %v, want %v", transferSyntax.Value, ImplicitVRLittleEndian)
	}
	if ds.Has(MustTag("TransferSyntaxUID")) {
		t.Fatal("dataset contains TransferSyntaxUID; file meta should be separate")
	}
}

func TestReadFileCTSmall(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Len() != 258 {
		t.Errorf("expected ~258 elements, got %d", ds.Len())
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
	if ds.Len() != 73 {
		t.Errorf("expected ~73 elements, got %d", ds.Len())
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
	if ds.Len() != 72 {
		t.Errorf("expected ~72 elements, got %d", ds.Len())
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
	if ds.Len() != 36 {
		t.Errorf("expected ~36 elements, got %d", ds.Len())
	}
}

func TestReadFileRTStruct(t *testing.T) {
	ds, err := ReadFile(testFilePath("rtstruct.dcm"), &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Len() != 31 {
		t.Errorf("expected ~31 elements, got %d", ds.Len())
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

func TestReadFileSpecificTags(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), &ReadOptions{
		SpecificTags: []Tag{
			MustTag(0x00100010),
			MustTag("PatientID"),
			MustTag("ImageType"),
			MustTag("ViewName"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := []Tag{
		MustTag(0x00080005),
		MustTag(0x00080008),
		MustTag(0x00100010),
		MustTag(0x00100020),
	}
	got := ds.SortedTags()
	if len(got) != len(expected) {
		t.Fatalf("tags = %v, want %v", got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("tags = %v, want %v", got, expected)
		}
	}
}

func TestReadFileSpecificTagsWithUnknownLengthElements(t *testing.T) {
	ds, err := ReadFile(testFilePath("rtstruct.dcm"), &ReadOptions{
		Force: true,
		SpecificTags: []Tag{
			MustTag("PatientName"),
			MustTag("PatientID"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := []Tag{
		MustTag(0x00080005),
		MustTag(0x00100010),
		MustTag(0x00100020),
	}
	got := ds.SortedTags()
	if len(got) != len(expected) {
		t.Fatalf("tags = %v, want %v", got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("tags = %v, want %v", got, expected)
		}
	}
}

func TestReadFileSpecificTagsPixelData(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), &ReadOptions{
		SpecificTags: []Tag{MustTag("PixelData")},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := ds.SortedTags()
	expected := []Tag{MustTag(0x00080005), MustTag(0x7FE00010)}
	if len(got) != len(expected) || got[0] != expected[0] || got[1] != expected[1] {
		t.Fatalf("tags = %v, want %v", got, expected)
	}
}

func TestReadFileSpecificTagsOnlyCharacterSet(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), &ReadOptions{
		SpecificTags: []Tag{MustTag("SpecificCharacterSet")},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := ds.SortedTags()
	expected := []Tag{MustTag(0x00080005)}
	if len(got) != len(expected) || got[0] != expected[0] {
		t.Fatalf("tags = %v, want %v", got, expected)
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
