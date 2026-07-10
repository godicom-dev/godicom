package godicom

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// pydicom.tests.test_filewriter.TestWriteFile.test_write_double_filemeta
func TestWriteDoubleFileMeta(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, UID("1.1")))

	outPath := filepath.Join(t.TempDir(), "double_meta.dcm")
	err = ds.SaveAs(outPath, nil)
	if err == nil {
		t.Fatal("expected error when group 2 element is in dataset")
	}
	if !strings.Contains(err.Error(), "0002") {
		t.Fatalf("error = %q, want group 2 mention", err.Error())
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite.test_implicit_big_raises
func TestWriteImplicitBigEndianRaises(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "Test"))

	implicit := true
	little := false
	outPath := filepath.Join(t.TempDir(), "implicit_be.dcm")
	err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, LittleEndian: &little})
	if err == nil {
		t.Fatal("expected error for implicit VR big endian")
	}
	if !strings.Contains(err.Error(), "implicit VR and big endian") {
		t.Fatalf("error = %q", err.Error())
	}
}

// pydicom.tests.test_filewriter.TestDCMWrite.test_file_meta_raises
func TestWriteFileMetaInDatasetRaises(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ImplicitVRLittleEndian))

	implicit := true
	outPath := filepath.Join(t.TempDir(), "meta_in_dataset.dcm")
	err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit})
	if err == nil {
		t.Fatal("expected error for file meta element in dataset")
	}
}

// pydicom.tests.test_filewriter.TestWriteFileMetaInfoNonStandard.test_bad_elements
func TestWriteFileMetaInfoBadElements(t *testing.T) {
	meta := NewFileMetaDataset()
	meta.Set(NewDataElement(MustTag("PatientID"), VRLO, "12345678"))
	meta.Set(NewDataElement(MustTag("MediaStorageSOPClassUID"), VRUI, "1.1"))
	meta.Set(NewDataElement(MustTag("MediaStorageSOPInstanceUID"), VRUI, "1.2"))
	meta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, "1.3"))
	meta.Set(NewDataElement(MustTag("ImplementationClassUID"), VRUI, "1.4"))

	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)
	if err := writeFileMetaInfo(fp, meta, false); err == nil {
		t.Fatal("expected error for non-group-2 element in file meta")
	}
}

// pydicom.tests.test_filewriter.TestCorrectAmbiguousVRElement.test_correct_ambiguous_raw_data_element
func TestCorrectAmbiguousVRRawDataElement(t *testing.T) {
	elem := NewDataElement(MustTag(0x00280120), VRUsSS, nil)
	elem.RawValue = []byte{0xfe, 0xff}
	ds := NewDataset()
	ds.Set(elem)
	ds.Set(NewDataElement(MustTag(0x00280103), VRUS, 0))

	if err := correctAmbiguousVRElement(elem, ds, true, nil); err != nil {
		t.Fatal(err)
	}
	if elem.VR != VRUS {
		t.Fatalf("VR = %s, want US", elem.VR)
	}
	v, ok := ds.GetInt(MustTag(0x00280120))
	if !ok || v != 0xFFFE {
		t.Fatalf("PixelPaddingValue = %d, want 65534", v)
	}
	if len(elem.RawValue) > 0 {
		t.Fatal("expected raw bytes to be converted to typed value")
	}
}

// pydicom.tests.test_filewriter.TestCorrectAmbiguousVRElement.test_not_ambiguous_raw_data_element
func TestCorrectAmbiguousVRNotAmbiguousRawElement(t *testing.T) {
	elem := NewDataElement(MustTag(0x60003000), VROB, nil)
	elem.RawValue = []byte{0x00}
	ds := NewDataset()
	ds.Set(elem)

	if err := correctAmbiguousVRElement(elem, ds, true, nil); err != nil {
		t.Fatal(err)
	}
	if elem.VR != VROB {
		t.Fatalf("VR = %s, want OB", elem.VR)
	}
	if !bytes.Equal(elem.RawValue, []byte{0x00}) {
		t.Fatalf("RawValue = %v, want unchanged", elem.RawValue)
	}
}

// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_ambiguous_element_in_sequence_explicit_using_index
func TestCorrectAmbiguousVRModalityLUTSequenceExplicitIndex(t *testing.T) {
	tests := []struct {
		name      string
		pixelRepr int
		wantLUTVR VR
	}{
		{"unsigned", 0, VRUS},
		{"signed", 1, VRSS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := datasetWithModalityLUTSequence(tt.pixelRepr)
			implicit := false
			outPath := filepath.Join(t.TempDir(), "modality_lut_explicit_index.dcm")
			ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
			ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.60")))
			if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true}); err != nil {
				t.Fatal(err)
			}

			reread, err := ReadFile(outPath, nil)
			if err != nil {
				t.Fatal(err)
			}
			seq, ok := reread.GetSequence(MustTag(0x00283000))
			if !ok || seq.Len() != 1 {
				t.Fatal("ModalityLUTSequence missing")
			}
			desc, ok := seq.Get(0).Get(MustTag(0x00283002))
			if !ok || desc.VR != tt.wantLUTVR {
				t.Fatalf("LUTDescriptor VR = %s, want %s", desc.VR, tt.wantLUTVR)
			}
		})
	}
}

// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_ambiguous_element_in_sequence_implicit_using_index
func TestCorrectAmbiguousVRModalityLUTSequenceImplicitTagIndex(t *testing.T) {
	tests := []struct {
		name      string
		pixelRepr int
		wantLUTVR VR
	}{
		{"unsigned", 0, VRUS},
		{"signed", 1, VRSS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := datasetWithModalityLUTSequence(tt.pixelRepr)
			implicit := true
			outPath := filepath.Join(t.TempDir(), "modality_lut_implicit_index.dcm")
			ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
			ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.61")))
			if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true}); err != nil {
				t.Fatal(err)
			}

			reread, err := ReadFile(outPath, nil)
			if err != nil {
				t.Fatal(err)
			}
			seq, ok := reread.GetSequence(MustTag(0x00283000))
			if !ok || seq.Len() != 1 {
				t.Fatal("ModalityLUTSequence missing")
			}
			desc, ok := seq.Get(0).Get(MustTag(0x00283002))
			if !ok || desc.VR != tt.wantLUTVR {
				t.Fatalf("LUTDescriptor VR = %s, want %s", desc.VR, tt.wantLUTVR)
			}
		})
	}
}
