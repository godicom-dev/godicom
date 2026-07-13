package godicom

import (
	"bytes"
	"path/filepath"
	"testing"
)

// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_OD_implicit_little
func TestWriteElementODImplicitLittle(t *testing.T) {
	bytestring := []byte{0, 1, 2, 3, 4, 5, 6, 7, 1, 1, 2, 3, 4, 5, 6, 7}
	elem := NewDataElement(MustTag(0x0070150D), VROD, bytestring)
	got := encodeElementImplicitLittle(elem)
	expected := append([]byte{0x70, 0x00, 0x0d, 0x15, 0x10, 0x00, 0x00, 0x00}, bytestring...)
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}

	elem.Value = []byte{}
	got = encodeElementImplicitLittle(elem)
	expected = []byte{0x70, 0x00, 0x0d, 0x15, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, expected) {
		t.Fatalf("empty got = % X, want % X", got, expected)
	}
}

// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_OL_implicit_little
func TestWriteElementOLImplicitLittle(t *testing.T) {
	bytestring := []byte{0, 1, 2, 3, 4, 5, 6, 7, 1, 1, 2, 3}
	elem := NewDataElement(MustTag(0x00660129), VROL, bytestring)
	got := encodeElementImplicitLittle(elem)
	expected := append([]byte{0x66, 0x00, 0x29, 0x01, 0x0c, 0x00, 0x00, 0x00}, bytestring...)
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}

	elem.Value = []byte{}
	got = encodeElementImplicitLittle(elem)
	expected = []byte{0x66, 0x00, 0x29, 0x01, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, expected) {
		t.Fatalf("empty got = % X, want % X", got, expected)
	}
}

// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_UC_implicit_little
func TestWriteElementUCImplicitLittle(t *testing.T) {
	elem := NewDataElement(MustTag(0x00189908), VRUC, "Test")
	got := encodeElementImplicitLittle(elem)
	expected := []byte{0x18, 0x00, 0x08, 0x99, 0x04, 0x00, 0x00, 0x00, 'T', 'e', 's', 't'}
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}

	elem.Value = "Test."
	got = encodeElementImplicitLittle(elem)
	expected = []byte{0x18, 0x00, 0x08, 0x99, 0x06, 0x00, 0x00, 0x00, 'T', 'e', 's', 't', '.', ' '}
	if !bytes.Equal(got, expected) {
		t.Fatalf("odd got = % X, want % X", got, expected)
	}

	elem.Value = NewMultiValue([]string{"Aa", "B", "C"})
	got = encodeElementImplicitLittle(elem)
	expected = []byte{0x18, 0x00, 0x08, 0x99, 0x06, 0x00, 0x00, 0x00, 'A', 'a', '\\', 'B', '\\', 'C'}
	if !bytes.Equal(got, expected) {
		t.Fatalf("multi even got = % X, want % X", got, expected)
	}

	elem.Value = NewMultiValue([]string{"A", "B", "C"})
	got = encodeElementImplicitLittle(elem)
	expected = []byte{0x18, 0x00, 0x08, 0x99, 0x06, 0x00, 0x00, 0x00, 'A', '\\', 'B', '\\', 'C', ' '}
	if !bytes.Equal(got, expected) {
		t.Fatalf("multi odd got = % X, want % X", got, expected)
	}

	elem.Value = ""
	got = encodeElementImplicitLittle(elem)
	expected = []byte{0x18, 0x00, 0x08, 0x99, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, expected) {
		t.Fatalf("empty got = % X, want % X", got, expected)
	}
}

// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_UC_explicit_little — multi-value cases
func TestWriteElementUCExplicitLittleMultiValue(t *testing.T) {
	elem := NewDataElement(MustTag(0x00189908), VRUC, NewMultiValue([]string{"Aa", "B", "C"}))
	got := encodeElementExplicitLittle(elem)
	expected := []byte{0x18, 0x00, 0x08, 0x99, 'U', 'C', 0x00, 0x00, 0x06, 0x00, 0x00, 0x00, 'A', 'a', '\\', 'B', '\\', 'C'}
	if !bytes.Equal(got, expected) {
		t.Fatalf("multi even got = % X, want % X", got, expected)
	}

	elem.Value = NewMultiValue([]string{"A", "B", "C"})
	got = encodeElementExplicitLittle(elem)
	expected = []byte{0x18, 0x00, 0x08, 0x99, 'U', 'C', 0x00, 0x00, 0x06, 0x00, 0x00, 0x00, 'A', '\\', 'B', '\\', 'C', ' '}
	if !bytes.Equal(got, expected) {
		t.Fatalf("multi odd got = % X, want % X", got, expected)
	}
}

// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_UR_implicit_little
func TestWriteElementURImplicitLittle(t *testing.T) {
	elem := NewDataElement(MustTag(0x00080120), VRUR, "http://github.com/darcymason/pydicom")
	got := encodeElementImplicitLittle(elem)
	expected := append(
		[]byte{0x08, 0x00, 0x20, 0x01, 0x24, 0x00, 0x00, 0x00},
		[]byte("http://github.com/darcymason/pydicom")...,
	)
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}

	elem.Value = "../test/test.py"
	got = encodeElementImplicitLittle(elem)
	expected = append(
		[]byte{0x08, 0x00, 0x20, 0x01, 0x10, 0x00, 0x00, 0x00},
		[]byte("../test/test.py ")...,
	)
	if !bytes.Equal(got, expected) {
		t.Fatalf("odd got = % X, want % X", got, expected)
	}

	elem.Value = ""
	got = encodeElementImplicitLittle(elem)
	expected = []byte{0x08, 0x00, 0x20, 0x01, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, expected) {
		t.Fatalf("empty got = % X, want % X", got, expected)
	}
}

// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_UN_implicit_little
func TestWriteElementUNImplicitLittle(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRUN, []byte{0x01, 0x02})
	got := encodeElementImplicitLittle(elem)
	expected := []byte{0x10, 0x00, 0x10, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01, 0x02}
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite.test_dataset_file_meta_unchanged
func TestWriteDatasetFileMetaUnchanged(t *testing.T) {
	ds := &FileDataset{Dataset: NewDataset(), FileMeta: NewFileMetaDataset()}
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, UID("1.2")))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.3")))

	outPath := filepath.Join(t.TempDir(), "no_meta.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}
	if ds.FileMeta.Len() != 0 {
		t.Fatalf("file meta len = %d, want 0", ds.FileMeta.Len())
	}

	ds.FileMeta.Set(NewDataElement(MustTag("ImplementationClassUID"), VRUI, UID("1.2.3.4")))
	before := ds.FileMeta.Len()
	outPath = filepath.Join(t.TempDir(), "partial_meta.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}
	if ds.FileMeta.Len() != before {
		t.Fatalf("file meta changed: %d -> %d", before, ds.FileMeta.Len())
	}
	if _, ok := ds.FileMeta.Get(MustTag("TransferSyntaxUID")); ok {
		t.Fatal("TransferSyntaxUID should not be added to in-memory file meta")
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite.test_private_tag_vr_from_implicit_data
func TestWritePrivateTagVRFromImplicitData(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.originalEnc.IsImplicitVR {
		t.Fatal("source should be explicit VR")
	}
	ds.FileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ImplicitVRLittleEndian))

	outPath := filepath.Join(t.TempDir(), "ct_implicit.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}
	impl, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !impl.originalEnc.IsImplicitVR {
		t.Fatal("intermediate should be implicit VR")
	}

	impl.FileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ExplicitVRLittleEndian))
	outPath = filepath.Join(t.TempDir(), "ct_explicit.dcm")
	if err := impl.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}
	expl, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if expl.originalEnc.IsImplicitVR {
		t.Fatal("final should be explicit VR")
	}

	checks := []struct {
		tag Tag
		vr  VR
	}{
		{MustTag(0x00090010), VRLO},
		{MustTag(0x00091001), VRLO},
		{MustTag(0x000910E7), VRUL},
		{MustTag(0x00431010), VRUS},
	}
	for _, tt := range checks {
		elem, ok := expl.Get(tt.tag)
		if !ok {
			t.Fatalf("tag %s missing after conversion", tt.tag)
		}
		if elem.VR != tt.vr {
			t.Fatalf("tag %s VR = %s, want %s", tt.tag, elem.VR, tt.vr)
		}
	}
}
