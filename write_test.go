package godicom

import (
	"bytes"
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

func TestWriteFilePreservesFileMeta(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.FileMeta == nil || ds.FileMeta.Len() == 0 {
		t.Fatal("source file meta is empty")
	}

	outPath := filepath.Join(t.TempDir(), "file_meta.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}

	out, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	sourceTS, ok := ds.FileMeta.Get(MustTag("TransferSyntaxUID"))
	if !ok {
		t.Fatal("source TransferSyntaxUID missing")
	}
	outTS, ok := out.FileMeta.Get(MustTag("TransferSyntaxUID"))
	if !ok {
		t.Fatal("output TransferSyntaxUID missing")
	}
	if sourceTS.Value != outTS.Value {
		t.Fatalf("TransferSyntaxUID = %v, want %v", outTS.Value, sourceTS.Value)
	}

	sourceClass, ok := ds.FileMeta.Get(MustTag("MediaStorageSOPClassUID"))
	if !ok {
		t.Fatal("source MediaStorageSOPClassUID missing")
	}
	outClass, ok := out.FileMeta.Get(MustTag("MediaStorageSOPClassUID"))
	if !ok {
		t.Fatal("output MediaStorageSOPClassUID missing")
	}
	if sourceClass.Value != outClass.Value {
		t.Fatalf("MediaStorageSOPClassUID = %v, want %v", outClass.Value, sourceClass.Value)
	}
}

func TestWriteFilePreservesPreamble(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	customPreamble := append([]byte{1, 2, 3, 4}, make([]byte, 124)...)
	ds.Preamble = customPreamble

	outPath := filepath.Join(t.TempDir(), "preamble.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:128]) != string(customPreamble) {
		t.Fatalf("preamble = % X, want % X", data[:4], customPreamble[:4])
	}
	if string(data[128:132]) != "DICM" {
		t.Fatalf("prefix = %q, want DICM", data[128:132])
	}
}

func TestWriteFileRejectsInvalidPreamble(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.Preamble = make([]byte, 127)

	outPath := filepath.Join(t.TempDir(), "bad_preamble.dcm")
	if err := ds.SaveAs(outPath, nil); err == nil {
		t.Fatal("SaveAs error = nil, want invalid preamble error")
	}
}
func encodeElementImplicitLittle(elem *DataElement) []byte {
	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)
	_ = writeElement(fp, elem, true, true)
	return buf.Bytes()
}

func TestWriteElementASCIIVRWithPadding(t *testing.T) {
	tests := []struct {
		name     string
		elem     *DataElement
		expected []byte
	}{
		{
			name:     "AE odd padded with space",
			elem:     NewDataElement(MustTag(0x00080054), VRAE, "CONQUESTSRV"),
			expected: []byte{0x08, 0x00, 0x54, 0x00, 0x0C, 0x00, 0x00, 0x00, 'C', 'O', 'N', 'Q', 'U', 'E', 'S', 'T', 'S', 'R', 'V', ' '},
		},
		{
			name:     "UI odd padded with NUL",
			elem:     NewDataElement(MustTag(0x00080062), VRUI, "1.2.3"),
			expected: []byte{0x08, 0x00, 0x62, 0x00, 0x06, 0x00, 0x00, 0x00, '1', '.', '2', '.', '3', 0x00},
		},
		{
			name:     "CS odd padded with space",
			elem:     NewDataElement(MustTag(0x00080060), VRCS, "REG"),
			expected: []byte{0x08, 0x00, 0x60, 0x00, 0x04, 0x00, 0x00, 0x00, 'R', 'E', 'G', ' '},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeElementImplicitLittle(tt.elem)
			if !bytes.Equal(got, tt.expected) {
				t.Fatalf("got = % X, want % X", got, tt.expected)
			}
		})
	}
}

func TestWriteElementOBOdd(t *testing.T) {
	value := []byte{0x00, 0x01, 0x02}
	elem := NewDataElement(MustTag(0x7FE00010), VROB, value)
	got := encodeElementImplicitLittle(elem)
	expected := append([]byte{0xE0, 0x7F, 0x10, 0x00, 0x04, 0x00, 0x00, 0x00}, value...)
	expected = append(expected, 0x00)
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}

	elem.Value = []byte{}
	got = encodeElementImplicitLittle(elem)
	expected = []byte{0xE0, 0x7F, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, expected) {
		t.Fatalf("empty got = % X, want % X", got, expected)
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
