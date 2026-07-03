package godicom

import (
	"path/filepath"
	"strings"
	"testing"
)

func datasetWithModalityLUTSequence(pixelRepr int) *Dataset {
	item := NewDataset()
	item.Set(NewDataElement(MustTag("LUTDescriptor"), VRUsSS, []int{0, 0, 16}))
	item.Set(NewDataElement(MustTag("LUTExplanation"), VRLO, nil))
	item.Set(NewDataElement(MustTag("ModalityLUTType"), VRCS, "US"))
	item.Set(NewDataElement(MustTag("LUTData"), VRObOw, []byte{
		0x00, 0x00, 0x14, 0x9a, 0x1f, 0x1c, 0xc2, 0x63, 0x37,
	}))

	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, pixelRepr))
	ds.Set(NewDataElement(MustTag("ModalityLUTSequence"), VRSQ, NewSequence([]*Dataset{item})))
	return ds
}

func TestCorrectAmbiguousVRNestedBeamSequence(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_sequence
	inner := NewDataset()
	inner.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 0))
	inner.Set(NewDataElement(MustTag("SmallestValidPixelValue"), VRUsSS, []byte{0x00, 0x01}))

	beam := NewDataset()
	beam.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 0))
	beam.Set(NewDataElement(MustTag("SmallestValidPixelValue"), VRUsSS, []byte{0x00, 0x01}))
	beam.Set(NewDataElement(MustTag("BeamSequence"), VRSQ, NewSequence([]*Dataset{inner})))

	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("BeamSequence"), VRSQ, NewSequence([]*Dataset{beam})))

	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}

	beamSeq, ok := ds.GetSequence(MustTag("BeamSequence"))
	if !ok || beamSeq.Len() != 1 {
		t.Fatal("BeamSequence missing")
	}
	beamItem := beamSeq.Get(0)
	v, ok := beamItem.GetInt(MustTag("SmallestValidPixelValue"))
	if !ok || v != 256 {
		t.Fatalf("beam SmallestValidPixelValue = %d, want 256", v)
	}
	elem, ok := beamItem.Get(MustTag("SmallestValidPixelValue"))
	if !ok || elem.VR != VRUS {
		t.Fatalf("beam VR = %s, want US", elem.VR)
	}

	nestedSeq, ok := beamItem.GetSequence(MustTag("BeamSequence"))
	if !ok || nestedSeq.Len() != 1 {
		t.Fatal("nested BeamSequence missing")
	}
	nested := nestedSeq.Get(0)
	v, ok = nested.GetInt(MustTag("SmallestValidPixelValue"))
	if !ok || v != 256 {
		t.Fatalf("nested SmallestValidPixelValue = %d, want 256", v)
	}
	elem, ok = nested.Get(MustTag("SmallestValidPixelValue"))
	if !ok || elem.VR != VRUS {
		t.Fatalf("nested VR = %s, want US", elem.VR)
	}
}

func TestCorrectAmbiguousVRModalityLUTSequenceExplicit(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_ambiguous_element_in_sequence_explicit_using_attribute
	tests := []struct {
		name       string
		pixelRepr  int
		wantLUTVR  VR
	}{
		{"unsigned", 0, VRUS},
		{"signed", 1, VRSS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := datasetWithModalityLUTSequence(tt.pixelRepr)
			ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
			ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.20")))

			implicit := false
			outPath := filepath.Join(t.TempDir(), "modality_lut.dcm")
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

func TestCorrectAmbiguousVRModalityLUTSequenceNearest(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_ambiguous_element_sequence_explicit_nearest
	ds := datasetWithModalityLUTSequence(0)
	seq, _ := ds.GetSequence(MustTag("ModalityLUTSequence"))
	seq.Get(0).Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 1))

	implicit := false
	outPath := filepath.Join(t.TempDir(), "modality_lut_nearest.dcm")
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.21")))
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
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.22")))
	outPath = filepath.Join(t.TempDir(), "modality_lut_nearest2.dcm")
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

func TestCorrectAmbiguousVRInvalidValueLength(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_invalid_value_length
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 0))
	ds.Set(NewDataElement(MustTag("SmallestImagePixelValue"), VRUsSS, []byte{0x00, 0x01, 0x02}))

	elem, ok := ds.Get(MustTag("SmallestImagePixelValue"))
	if !ok {
		t.Fatal("SmallestImagePixelValue missing")
	}
	err := correctAmbiguousVRElement(elem, ds, true, nil)
	if err == nil {
		t.Fatal("expected error for invalid value length")
	}
	msg := err.Error()
	if !strings.Contains(msg, "failed to resolve ambiguous VR for tag (0028,0106)") {
		t.Fatalf("error = %q, want tag in message", msg)
	}
	if !strings.Contains(msg, "even multiple of bytes per value") {
		t.Fatalf("error = %q, want length detail", msg)
	}
}
