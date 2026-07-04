package godicom

import (
	"os"
	"path/filepath"
	"testing"
)

// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_ambiguous_element_in_sequence_implicit_using_index
func TestCorrectAmbiguousVRModalityLUTSequenceImplicitIndex(t *testing.T) {
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
			outPath := filepath.Join(t.TempDir(), "modality_lut_implicit.dcm")
			ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
			ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.40")))
			if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true}); err != nil {
				t.Fatal(err)
			}

			reread, err := ReadFile(outPath, nil)
			if err != nil {
				t.Fatal(err)
			}
			seq, ok := reread.GetSequence(MustTag("ModalityLUTSequence"))
			if !ok || seq.Len() != 1 {
				t.Fatal("ModalityLUTSequence missing")
			}
			desc, ok := seq.Get(0).Get(MustTag("LUTDescriptor"))
			if !ok || desc.VR != tt.wantLUTVR {
				t.Fatalf("LUTDescriptor VR = %s, want %s", desc.VR, tt.wantLUTVR)
			}
		})
	}
}

// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_ambiguous_element_sequence_implicit_nearest
func TestCorrectAmbiguousVRModalityLUTSequenceImplicitNearest(t *testing.T) {
	ds := datasetWithModalityLUTSequence(0)
	seq, _ := ds.GetSequence(MustTag("ModalityLUTSequence"))
	seq.Get(0).Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 1))

	implicit := true
	outPath := filepath.Join(t.TempDir(), "modality_lut_implicit_nearest.dcm")
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.41")))
	if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	rseq, ok := reread.GetSequence(MustTag("ModalityLUTSequence"))
	if !ok || rseq.Len() != 1 {
		t.Fatal("ModalityLUTSequence missing")
	}
	desc, ok := rseq.Get(0).Get(MustTag("LUTDescriptor"))
	if !ok || desc.VR != VRSS {
		t.Fatalf("LUTDescriptor VR = %s, want SS", desc.VR)
	}

	ds = datasetWithModalityLUTSequence(1)
	seq, _ = ds.GetSequence(MustTag("ModalityLUTSequence"))
	seq.Get(0).Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 0))
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.42")))
	outPath = filepath.Join(t.TempDir(), "modality_lut_implicit_nearest2.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	reread, err = ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	rseq, ok = reread.GetSequence(MustTag("ModalityLUTSequence"))
	if !ok || rseq.Len() != 1 {
		t.Fatal("ModalityLUTSequence missing")
	}
	desc, ok = rseq.Get(0).Get(MustTag("LUTDescriptor"))
	if !ok || desc.VR != VRUS {
		t.Fatalf("LUTDescriptor VR = %s, want US", desc.VR)
	}
}

func datasetWithBeamModalityLUTSequence(pixelRepr int) *Dataset {
	lutItem := NewDataset()
	lutItem.Set(NewDataElement(MustTag("LUTDescriptor"), VRUsSS, []int{0, 0, 16}))
	lutItem.Set(NewDataElement(MustTag("LUTExplanation"), VRLO, nil))
	lutItem.Set(NewDataElement(MustTag("ModalityLUTType"), VRCS, "US"))
	lutItem.Set(NewDataElement(MustTag("LUTData"), VRObOw, []byte{
		0x00, 0x00, 0x14, 0x9a, 0x1f, 0x1c, 0xc2, 0x63, 0x37,
	}))

	beamItem := NewDataset()
	beamItem.Set(NewDataElement(MustTag("ModalityLUTSequence"), VRSQ, NewSequence([]*Dataset{lutItem})))

	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, pixelRepr))
	ds.Set(NewDataElement(MustTag("BeamSequence"), VRSQ, NewSequence([]*Dataset{beamItem})))
	return ds
}

// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_parent_change_implicit
func TestCorrectAmbiguousVRParentChangeImplicit(t *testing.T) {
	ds := datasetWithBeamModalityLUTSequence(0)
	implicit := true
	outPath := filepath.Join(t.TempDir(), "beam_lut_implicit.dcm")
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.43")))
	if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	ds1, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds1.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 1))
	beamSeq, ok := reread.GetSequence(MustTag("BeamSequence"))
	if !ok {
		t.Fatal("BeamSequence missing from source")
	}
	ds1.Set(NewDataElement(MustTag("BeamSequence"), VRSQ, beamSeq))

	modSeq, ok := ds1.GetSequence(MustTag("BeamSequence"))
	if !ok || modSeq.Len() != 1 {
		t.Fatal("BeamSequence missing on target")
	}
	beamItem := modSeq.Get(0)
	mlutSeq, ok := beamItem.GetSequence(MustTag("ModalityLUTSequence"))
	if !ok || mlutSeq.Len() != 1 {
		t.Fatal("ModalityLUTSequence missing")
	}
	desc, ok := mlutSeq.Get(0).Get(MustTag("LUTDescriptor"))
	if !ok || desc.VR != VRSS {
		t.Fatalf("LUTDescriptor VR = %s, want SS after parent change", desc.VR)
	}
}

// pydicom.tests.test_filewriter.TestWriteFile.test_write_ffff_ffff
func TestWriteFFFFTag(t *testing.T) {
	fd := &FileDataset{
		Dataset:  NewDataset(),
		FileMeta: NewFileMetaDataset(),
	}
	fd.Set(NewDataElement(MustTag(0xFFFFFFFF), VRLO, "123456"))

	implicit := true
	outPath := filepath.Join(t.TempDir(), "ffff.dcm")
	if err := fd.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := reread.Get(MustTag(0xFFFFFFFF))
	if !ok {
		t.Fatal("(FFFF,FFFF) element missing")
	}
	val, ok := elem.Value.([]byte)
	if !ok {
		t.Fatalf("value type = %T, want []byte", elem.Value)
	}
	if string(val) != "123456" {
		t.Fatalf("value = %q, want %q", val, "123456")
	}
}

// pydicom.tests.test_filewriter.TestWriteFile.test_no_preamble
func TestWriteFileNoPreamble(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, preamble := range [][]byte{nil, {}} {
		ds.Preamble = preamble
		outPath := filepath.Join(t.TempDir(), "no_preamble.dcm")
		if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) < 132 {
			t.Fatal("written file too small")
		}
		for i := 0; i < 128; i++ {
			if data[i] != 0 {
				t.Fatalf("preamble byte %d = %d, want 0", i, data[i])
			}
		}
		if string(data[128:132]) != "DICM" {
			t.Fatalf("prefix = %q, want DICM", data[128:132])
		}
	}
}

// pydicom.tests.test_filewriter.TestWriteFile.test_prefix
func TestWriteFilePrefix(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.Preamble = make([]byte, 128)

	outPath := filepath.Join(t.TempDir(), "prefix.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[128:132]) != "DICM" {
		t.Fatalf("prefix = %q, want DICM", data[128:132])
	}
}
