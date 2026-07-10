package godicom

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// pydicom.tests.test_filewriter.TestWriteNoPreamble.test_filemeta_dataset
func TestWriteNonStandardFileMetaDataset(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.Preamble = nil

	outPath := filepath.Join(t.TempDir(), "filemeta_only.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) >= 128 && bytes.Equal(data[:128], make([]byte, 128)) {
		t.Fatal("file starts with zero preamble")
	}
	if len(data) >= 4 && string(data[:4]) == "DICM" {
		t.Fatal("file starts with DICM without preamble")
	}

	reread, err := ReadFile(outPath, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if reread.Preamble != nil {
		t.Fatal("expected nil preamble on readback")
	}
	if reread.FileMeta == nil || reread.FileMeta.Len() == 0 {
		t.Fatal("expected file meta on readback")
	}
	if _, ok := reread.FileMeta.Get(MustTag("ImplementationClassUID")); !ok {
		t.Fatal("ImplementationClassUID missing from file meta")
	}
	if _, ok := reread.GetString(MustTag("PatientID")); !ok {
		t.Fatal("PatientID missing from dataset")
	}
}

// pydicom.tests.test_filewriter.TestWriteNoPreamble.test_preamble_filemeta_dataset
func TestWriteNonStandardPreambleFileMetaRoundtrip(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	wantPreamble := append([]byte(nil), ds.Preamble...)
	wantTS, _ := ds.FileMeta.GetString(MustTag("TransferSyntaxUID"))

	outPath := filepath.Join(t.TempDir(), "preamble_filemeta.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reread.Preamble, wantPreamble) {
		t.Fatal("preamble mismatch after roundtrip")
	}
	gotTS, ok := reread.FileMeta.GetString(MustTag("TransferSyntaxUID"))
	if !ok || gotTS != wantTS {
		t.Fatalf("TransferSyntaxUID = %q, want %q", gotTS, wantTS)
	}
	if _, ok := reread.GetString(MustTag("PatientID")); !ok {
		t.Fatal("PatientID missing")
	}
}

// pydicom.tests.test_filewriter.TestWriteNoPreamble.test_ds_unchanged
func TestWriteDatasetUnchangedOnSave(t *testing.T) {
	ds, err := ReadFile(testFilePath("rtplan.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := ReadFile(testFilePath("rtplan.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(t.TempDir(), "unchanged.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}

	if err := datasetsEqual(ref, ds); err != nil {
		t.Fatalf("dataset changed after SaveAs: %v", err)
	}
}

// pydicom.tests.test_filewriter.TestWriteFileMetaInfoNonStandard.test_transfer_syntax_not_added
func TestWriteFileMetaInfoTransferSyntaxNotAdded(t *testing.T) {
	ds, err := ReadFile(testFilePath("meta_missing_tsyntax.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.FileMeta == nil {
		t.Fatal("file meta missing")
	}
	if _, ok := ds.FileMeta.Get(MustTag("TransferSyntaxUID")); ok {
		t.Fatal("source file meta should not contain TransferSyntaxUID")
	}

	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)
	if err := writeFileMetaInfo(fp, ds.FileMeta, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := ds.FileMeta.Get(MustTag("TransferSyntaxUID")); ok {
		t.Fatal("TransferSyntaxUID should not be added to in-memory file meta")
	}
	if _, ok := ds.FileMeta.Get(MustTag("ImplementationClassUID")); !ok {
		t.Fatal("ImplementationClassUID should remain in file meta")
	}

	tmpPath := filepath.Join(t.TempDir(), "meta_only.bin")
	if err := os.WriteFile(tmpPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := ReadFile(tmpPath, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := written.FileMeta.Get(MustTag("ImplementationClassUID")); !ok {
		t.Fatal("written file meta missing ImplementationClassUID")
	}
	if _, ok := written.FileMeta.Get(MustTag("TransferSyntaxUID")); ok {
		t.Fatal("written file meta should not contain TransferSyntaxUID")
	}
}

// pydicom.tests.test_filewriter.TestWriteFileMetaInfoNonStandard.test_missing_elements
func TestWriteFileMetaInfoMissingElementsNoError(t *testing.T) {
	steps := []func(*FileMetaDataset){
		func(m *FileMetaDataset) {},
		func(m *FileMetaDataset) {
			m.Set(NewDataElement(MustTag("MediaStorageSOPClassUID"), VRUI, "1.1"))
		},
		func(m *FileMetaDataset) {
			m.Set(NewDataElement(MustTag("MediaStorageSOPInstanceUID"), VRUI, "1.2"))
		},
		func(m *FileMetaDataset) {
			m.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, "1.3"))
		},
		func(m *FileMetaDataset) {
			m.Set(NewDataElement(MustTag("ImplementationClassUID"), VRUI, "1.4"))
		},
	}

	meta := NewFileMetaDataset()
	for i, step := range steps {
		step(meta)
		var buf bytes.Buffer
		fp := newDicomWriter(&buf)
		fp.SetByteOrder(true)
		if err := writeFileMetaInfo(fp, meta, false); err != nil {
			t.Fatalf("step %d: writeFileMetaInfo error = %v", i, err)
		}
	}
}

// pydicom.tests.test_filewriter.TestWriteFile.test_read_write_identical — charset subset
func TestWriteFileBytesIdenticalCharset(t *testing.T) {
	for _, file := range []string{"chrH31.dcm", "chrFrenMulti.dcm"} {
		t.Run(file, func(t *testing.T) {
			path := requireCharsetFile(t, file)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ds, err := ReadFile(path, nil)
			if err != nil {
				t.Fatal(err)
			}
			outPath := filepath.Join(t.TempDir(), file)
			if err := ds.SaveAs(outPath, nil); err != nil {
				t.Fatal(err)
			}
			written, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			same, pos := bytesIdentical(original, written)
			if !same {
				t.Fatalf("bytes differ at %d", pos)
			}
		})
	}
}

func datasetsEqual(a, b *FileDataset) error {
	if a.Len() != b.Len() {
		return errDatasetDiff("element count", a.Len(), b.Len())
	}
	for _, tag := range a.SortedTags() {
		ae, ok := a.Get(tag)
		if !ok {
			return errDatasetDiff("missing tag on a", tag, nil)
		}
		be, ok := b.Get(tag)
		if !ok {
			return errDatasetDiff("missing tag on b", tag, nil)
		}
		if err := elementsEqual(ae, be); err != nil {
			return err
		}
	}
	return nil
}

type datasetDiffError struct {
	msg string
}

func (e datasetDiffError) Error() string { return e.msg }

func errDatasetDiff(msg string, a, b interface{}) error {
	return datasetDiffError{msg: msg}
}
