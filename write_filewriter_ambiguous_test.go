package godicom

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteExplicitVRAmbiguousUnresolved(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteAmbiguousVR.test_write_explicit_vr_raises
	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)

	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PerimeterValue"), VRUsSS, []byte{0x00, 0x01}))

	err := writeDataset(fp, ds, false, true)
	if err == nil {
		t.Fatal("writeDataset error = nil, want ambiguous VR error")
	}
	if !strings.Contains(err.Error(), "ambiguous VR") {
		t.Fatalf("error = %v, want ambiguous VR message", err)
	}
}

func TestWriteExplicitVRLittleEndianAmbiguous(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteAmbiguousVR.test_write_explicit_vr_little_endian
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.10")))
	ds.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 0))
	ds.Set(NewDataElement(MustTag("SmallestValidPixelValue"), VRUsSS, []byte{0x00, 0x01}))

	implicit := false
	outPath := filepath.Join(t.TempDir(), "explicit_ambiguous.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := reread.GetInt(MustTag("SmallestValidPixelValue"))
	if !ok || v != 256 {
		t.Fatalf("SmallestValidPixelValue = %d, %t; want 256", v, ok)
	}
	elem, ok := reread.Get(MustTag("SmallestValidPixelValue"))
	if !ok || elem.VR != VRUS {
		t.Fatalf("VR = %s, want US", elem.VR)
	}
}

func TestWriteExplicitVRBigEndianAmbiguous(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteAmbiguousVR.test_write_explicit_vr_big_endian
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.11")))
	ds.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 1))
	ds.Set(NewDataElement(MustTag("SmallestValidPixelValue"), VRUsSS, []byte{0x00, 0x01}))

	implicit := false
	little := false
	outPath := filepath.Join(t.TempDir(), "explicit_be_ambiguous.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{
		ImplicitVR:        &implicit,
		LittleEndian:      &little,
		EnforceFileFormat: true,
	}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := reread.GetInt(MustTag("SmallestValidPixelValue"))
	if !ok || v != 1 {
		t.Fatalf("SmallestValidPixelValue = %d, %t; want 1", v, ok)
	}
	elem, ok := reread.Get(MustTag("SmallestValidPixelValue"))
	if !ok || elem.VR != VRSS {
		t.Fatalf("VR = %s, want SS", elem.VR)
	}
}

func TestCorrectAmbiguousVRPixelPaddingValue(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVRElement.test_correct_ambiguous_data_element
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PixelPaddingValue"), VRUsSS, []byte{0xfe, 0xff}))
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(MustTag("PixelPaddingValue"))
	if !ok || elem.VR != VRUS {
		t.Fatalf("VR = %s, want US", elem.VR)
	}
	v, ok := ds.GetInt(MustTag("PixelPaddingValue"))
	if !ok || v != 0xFFFE {
		t.Fatalf("PixelPaddingValue = %d, want 65534", v)
	}

	ds.Set(NewDataElement(MustTag("PixelData"), VRObOw, []byte("3456")))
	padding, _ := ds.Get(MustTag("PixelPaddingValue"))
	if err := correctAmbiguousVRElement(padding, ds, true, nil); err == nil {
		t.Fatal("expected error when PixelData present without PixelRepresentation")
	}

	ds.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 0))
	if err := correctAmbiguousVRElement(padding, ds, true, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok = ds.Get(MustTag("PixelPaddingValue"))
	if !ok || elem.VR != VRUS {
		t.Fatalf("VR = %s, want US", elem.VR)
	}
	v, ok = ds.GetInt(MustTag("PixelPaddingValue"))
	if !ok || v != 0xFFFE {
		t.Fatalf("PixelPaddingValue = %d, want 65534", v)
	}
}

func TestCorrectAmbiguousVRNotAmbiguous(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVRElement.test_not_ambiguous
	elem := NewDataElement(MustTag("Modality"), VRCS, "CT")
	ds := NewDataset()
	ds.Set(elem)

	if err := correctAmbiguousVRElement(elem, ds, true, nil); err != nil {
		t.Fatal(err)
	}
	if elem.VR != VRCS {
		t.Fatalf("VR = %s, want CS", elem.VR)
	}
	s, ok := elem.Value.(string)
	if !ok || s != "CT" {
		t.Fatalf("value = %#v, want CT", elem.Value)
	}
}
