package godicom

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pydicom.tests.test_filewriter.TestDCMWrite.test_command_set_raises
func TestWriteCommandSetRaises(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("MessageID"), VRUS, 1))
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.3")))

	implicit := true
	outPath := filepath.Join(t.TempDir(), "command_set.dcm")
	err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true})
	if err == nil {
		t.Fatal("expected error for command set elements")
	}
	if !strings.Contains(err.Error(), "0000") {
		t.Fatalf("error = %q, want group 0000 mention", err.Error())
	}
}

// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_unknown_vr_raises
func TestWriteUnknownVRRaises(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VR("ZZ"), "Test")
	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)
	err := writeElement(fp, elem, true, true, nil, false)
	if err == nil {
		t.Fatal("expected error for unknown VR")
	}
	if !strings.Contains(err.Error(), "unknown Value Representation") {
		t.Fatalf("error = %q", err.Error())
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite_EnforceFileFormat.test_file_meta_none
func TestWriteEnforceFileMetaNone(t *testing.T) {
	ds, err := ReadFile(testFilePath("rtplan.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	wantTS, ok := ds.FileMeta.GetString(MustTag("TransferSyntaxUID"))
	if !ok {
		t.Fatal("source TransferSyntaxUID missing")
	}
	ds.FileMeta = NewFileMetaDataset()

	outPath := filepath.Join(t.TempDir(), "meta_none.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	out, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	sopClass, _ := ds.GetString(MustTag("SOPClassUID"))
	sopInst, _ := ds.GetString(MustTag("SOPInstanceUID"))
	gotClass, _ := out.FileMeta.GetString(MustTag("MediaStorageSOPClassUID"))
	gotInst, _ := out.FileMeta.GetString(MustTag("MediaStorageSOPInstanceUID"))
	gotTS, _ := out.FileMeta.GetString(MustTag("TransferSyntaxUID"))
	if gotClass != sopClass || gotInst != sopInst {
		t.Fatalf("SOP UIDs = %q/%q, want %q/%q", gotClass, gotInst, sopClass, sopInst)
	}
	if gotTS != wantTS {
		t.Fatalf("TransferSyntaxUID = %q, want %q", gotTS, wantTS)
	}
	if _, ok := out.FileMeta.Get(MustTag("ImplementationClassUID")); !ok {
		t.Fatal("ImplementationClassUID missing")
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite_EnforceFileFormat.test_file_meta_no_syntax
func TestWriteEnforceFileMetaNoSyntax(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, UID("1.2")))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.3")))

	implicit := true
	outPath := filepath.Join(t.TempDir(), "no_syntax_impl.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}
	out, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts, _ := out.FileMeta.GetString(MustTag("TransferSyntaxUID"))
	if ts != string(ImplicitVRLittleEndian) {
		t.Fatalf("TS = %q, want ImplicitVRLittleEndian", ts)
	}

	implicit = false
	little := false
	outPath = filepath.Join(t.TempDir(), "no_syntax_be.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{
		ImplicitVR:        &implicit,
		LittleEndian:      &little,
		EnforceFileFormat: true,
	}); err != nil {
		t.Fatal(err)
	}
	out, err = ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts, _ = out.FileMeta.GetString(MustTag("TransferSyntaxUID"))
	if ts != string(ExplicitVRBigEndian) {
		t.Fatalf("TS = %q, want ExplicitVRBigEndian", ts)
	}

	little = true
	outPath = filepath.Join(t.TempDir(), "no_syntax_expl.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{
		ImplicitVR:        &implicit,
		LittleEndian:      &little,
		EnforceFileFormat: true,
	}); err != nil {
		t.Fatal(err)
	}
	out, err = ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts, _ = out.FileMeta.GetString(MustTag("TransferSyntaxUID"))
	// godicom fills Explicit VR LE when missing (intentional divergence from
	// pydicom, which leaves TransferSyntaxUID unset and fails validation).
	if ts != string(ExplicitVRLittleEndian) {
		t.Fatalf("TS = %q, want ExplicitVRLittleEndian", ts)
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite_EnforceFileFormat.test_file_meta_sop_class_sop_instance
func TestWriteEnforceFileMetaSOPSync(t *testing.T) {
	ds := &FileDataset{Dataset: NewDataset(), FileMeta: NewFileMetaDataset()}
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, UID("1.2")))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.3")))

	implicit := true
	outPath := filepath.Join(t.TempDir(), "sop_sync.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}
	out, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotClass, _ := out.FileMeta.GetString(MustTag("MediaStorageSOPClassUID"))
	gotInst, _ := out.FileMeta.GetString(MustTag("MediaStorageSOPInstanceUID"))
	if gotClass != "1.2" || gotInst != "1.2.3" {
		t.Fatalf("MediaStorage SOP = %q/%q, want 1.2/1.2.3", gotClass, gotInst)
	}

	ds.FileMeta = NewFileMetaDataset()
	ds.FileMeta.Set(NewDataElement(MustTag("MediaStorageSOPClassUID"), VRUI, UID("1.2")))
	ds.FileMeta.Set(NewDataElement(MustTag("MediaStorageSOPInstanceUID"), VRUI, UID("1.2.3")))
	ds.Delete(MustTag("SOPClassUID"))
	ds.Delete(MustTag("SOPInstanceUID"))
	implicit = false
	little := false
	outPath = filepath.Join(t.TempDir(), "sop_keep.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{
		ImplicitVR:        &implicit,
		LittleEndian:      &little,
		EnforceFileFormat: true,
	}); err != nil {
		t.Fatal(err)
	}
	out, err = ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotClass, _ = out.FileMeta.GetString(MustTag("MediaStorageSOPClassUID"))
	gotInst, _ = out.FileMeta.GetString(MustTag("MediaStorageSOPInstanceUID"))
	if gotClass != "1.2" || gotInst != "1.2.3" {
		t.Fatalf("kept MediaStorage SOP = %q/%q", gotClass, gotInst)
	}

	ds.FileMeta = nil
	implicit = true
	outPath = filepath.Join(t.TempDir(), "sop_missing.dcm")
	err = ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true})
	if err == nil {
		t.Fatal("expected error when SOP UIDs missing from dataset and file meta")
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite_EnforceFileFormat.test_bad_preamble
func TestWriteEnforceBadPreamble(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{127, 129} {
		ds.Preamble = make([]byte, n)
		outPath := filepath.Join(t.TempDir(), "bad_preamble.dcm")
		if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err == nil {
			t.Fatalf("preamble len %d: expected error", n)
		}
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite_EnforceFileFormat.test_preamble_custom
func TestWriteEnforceCustomPreamble(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	custom := append([]byte{0x01, 0x02, 0x03, 0x04}, make([]byte, 124)...)
	ds.Preamble = custom

	outPath := filepath.Join(t.TempDir(), "custom_preamble.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data[:128], custom) {
		t.Fatal("custom preamble not preserved")
	}
	if string(data[128:132]) != "DICM" {
		t.Fatalf("prefix = %q", data[128:132])
	}
}

// pydicom.tests.test_filewriter.TestDetermineEncoding.test_invalid_transfer_syntax_raises
func TestDetermineEncodingInvalidTransferSyntax(t *testing.T) {
	meta := NewFileMetaDataset()
	meta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, CTImageStorage))
	ds := NewDataset()
	_, _, err := determineWriteEncoding(meta, ds, nil)
	if err == nil {
		t.Fatal("expected error for non-transfer-syntax UID")
	}
	if !strings.Contains(err.Error(), "not a valid transfer syntax") {
		t.Fatalf("error = %q", err.Error())
	}
}

// pydicom.tests.test_filewriter.TestDetermineEncoding.test_private_transfer_syntax_raises
func TestDetermineEncodingPrivateTransferSyntaxRequiresArgs(t *testing.T) {
	meta := NewFileMetaDataset()
	meta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, UID("1.2.3")))
	ds := NewDataset()
	_, _, err := determineWriteEncoding(meta, ds, nil)
	if err == nil {
		t.Fatal("expected error for private transfer syntax without args")
	}

	implicit := true
	little := true
	imp, lit, err := determineWriteEncoding(meta, ds, &WriteOptions{ImplicitVR: &implicit, LittleEndian: &little})
	if err != nil {
		t.Fatal(err)
	}
	if !imp || !lit {
		t.Fatalf("encoding = (%t,%t), want (true,true)", imp, lit)
	}
}

// pydicom.tests.test_filewriter.TestDetermineEncoding.test_mismatch_raises
func TestDetermineEncodingMismatchRaises(t *testing.T) {
	meta := NewFileMetaDataset()
	meta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ImplicitVRLittleEndian))
	ds := NewDataset()

	implicit := true
	little := false
	_, _, err := determineWriteEncoding(meta, ds, &WriteOptions{ImplicitVR: &implicit, LittleEndian: &little})
	if err == nil || !strings.Contains(err.Error(), "LittleEndian") {
		t.Fatalf("error = %v, want LittleEndian mismatch", err)
	}

	implicit = false
	little = true
	_, _, err = determineWriteEncoding(meta, ds, &WriteOptions{ImplicitVR: &implicit, LittleEndian: &little})
	if err == nil || !strings.Contains(err.Error(), "ImplicitVR") {
		t.Fatalf("error = %v, want ImplicitVR mismatch", err)
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite.test_convert_implicit_to_explicit_vr
func TestWriteConvertImplicitToExplicitVR(t *testing.T) {
	ds, err := ReadFile(testFilePath("MR_small_implicit.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ds.originalEnc.IsImplicitVR {
		t.Fatal("source should be implicit VR")
	}
	ds.FileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ExplicitVRLittleEndian))

	outPath := filepath.Join(t.TempDir(), "mr_explicit.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}
	out, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.originalEnc.IsImplicitVR {
		t.Fatal("converted file should be explicit VR")
	}

	ref, err := ReadFile(testFilePath("MR_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []Tag{
		MustTag("PatientName"),
		MustTag("PatientID"),
		MustTag("Rows"),
		MustTag("Columns"),
	} {
		a, okA := ref.Get(tag)
		b, okB := out.Get(tag)
		if !okA || !okB {
			t.Fatalf("tag %s missing: ref=%t out=%t", tag, okA, okB)
		}
		if err := elementsEqual(a, b); err != nil {
			t.Fatalf("tag %s: %v", tag, err)
		}
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite.test_convert_explicit_to_implicit_vr
func TestWriteConvertExplicitToImplicitVR(t *testing.T) {
	ds, err := ReadFile(testFilePath("MR_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.originalEnc.IsImplicitVR {
		t.Fatal("source should be explicit VR")
	}
	ds.FileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ImplicitVRLittleEndian))

	outPath := filepath.Join(t.TempDir(), "mr_implicit.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}
	out, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.originalEnc.IsImplicitVR {
		t.Fatal("converted file should be implicit VR")
	}

	ref, err := ReadFile(testFilePath("MR_small_implicit.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []Tag{
		MustTag("PatientName"),
		MustTag("PatientID"),
		MustTag("Rows"),
		MustTag("Columns"),
	} {
		a, okA := ref.Get(tag)
		b, okB := out.Get(tag)
		if !okA || !okB {
			t.Fatalf("tag %s missing: ref=%t out=%t", tag, okA, okB)
		}
		if err := elementsEqual(a, b); err != nil {
			t.Fatalf("tag %s: %v", tag, err)
		}
	}
}
