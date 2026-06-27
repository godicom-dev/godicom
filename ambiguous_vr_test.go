package godicom

import (
	"path/filepath"
	"testing"
)

func TestCorrectAmbiguousVRPixelRepresentationVMOne(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_pixel_representation_vm_one
	ref := NewDataset()
	ref.Set(NewDataElement(MustTag(0x00280103), VRUS, 0))
	ref.Set(NewDataElement(MustTag(0x00280104), VRUsSS, []byte{0x00, 0x01}))

	ds := cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(MustTag(0x00280104))
	if !ok || elem.VR != VRUS {
		t.Fatalf("VR = %s, want US", elem.VR)
	}
	v, ok := ds.GetInt(MustTag(0x00280104))
	if !ok || v != 256 {
		t.Fatalf("value = %d, want 256", v)
	}

	ref.Set(NewDataElement(MustTag(0x00280103), VRUS, 1))
	ref.Set(NewDataElement(MustTag(0x00280104), VRUsSS, []byte{0x00, 0x01}))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, false, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok = ds.Get(MustTag(0x00280104))
	if !ok || elem.VR != VRSS {
		t.Fatalf("VR = %s, want SS", elem.VR)
	}
	v, ok = ds.GetInt(MustTag(0x00280104))
	if !ok || v != 1 {
		t.Fatalf("value = %d, want 1", v)
	}

	ref = NewDataset()
	ref.Set(NewDataElement(MustTag(0x00280104), VRUsSS, []byte{0x00, 0x01}))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok = ds.Get(MustTag(0x00280104))
	if !ok || elem.VR != VRUS {
		t.Fatalf("VR = %s, want US", elem.VR)
	}

	ref = NewDataset()
	ref.Set(NewDataElement(MustTag(0x00280104), VRUsSS, []byte{0x00, 0x01}))
	ref.Set(NewDataElement(MustTag(0x7FE00010), VROB, []byte("123")))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err == nil {
		t.Fatal("expected error when PixelData present without PixelRepresentation")
	}
}

func TestCorrectAmbiguousVRWriteNewAmbiguous(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_write_new_ambiguous
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00280106), VRUsSS, 0))
	if elem, ok := ds.Get(MustTag(0x00280106)); !ok || elem.VR != VRUsSS {
		t.Fatalf("initial VR = %s, want US or SS", elem.VR)
	}
	ds.Set(NewDataElement(MustTag(0x00280103), VRUS, 0))
	ds.Set(NewDataElement(MustTag(0x00283002), VRUsSS, []int{1, 0}))
	ds.Set(NewDataElement(MustTag(0x00283006), VRUsOw, 0))

	implicit := true
	if err := ds.SaveAs(filepath.Join(t.TempDir(), "ambiguous.dcm"), &WriteOptions{ImplicitVR: &implicit}); err != nil {
		t.Fatal(err)
	}

	elem, ok := ds.Get(MustTag(0x00280106))
	if !ok || elem.VR != VRUS {
		t.Fatalf("SmallestImagePixelValue VR = %s, want US", elem.VR)
	}
	elem, ok = ds.Get(MustTag(0x00283006))
	if !ok || elem.VR != VRUS {
		t.Fatalf("LUTData VR = %s, want US", elem.VR)
	}
	elem, ok = ds.Get(MustTag(0x00283002))
	if !ok || elem.VR != VRUS {
		t.Fatalf("LUTDescriptor VR = %s, want US", elem.VR)
	}
}

func TestValidateFileMetaEnforceStandard(t *testing.T) {
	meta := NewFileMetaDataset()
	meta.Set(NewDataElement(MustTag("MediaStorageSOPClassUID"), VRUI, "1.2.3"))
	meta.Set(NewDataElement(MustTag("MediaStorageSOPInstanceUID"), VRUI, "1.2.3.4"))
	meta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ExplicitVRLittleEndian))

	if err := ValidateFileMeta(meta, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.Get(MustTag("ImplementationClassUID")); !ok {
		t.Fatal("ImplementationClassUID should be added")
	}
	if _, ok := meta.Get(MustTag("FileMetaInformationVersion")); !ok {
		t.Fatal("FileMetaInformationVersion should be added")
	}
}

func TestValidateFileMetaRejectsNonGroup2(t *testing.T) {
	meta := NewFileMetaDataset()
	meta.Set(NewDataElement(MustTag(0x00100010), VRPN, "Test"))
	if err := ValidateFileMeta(meta, false); err == nil {
		t.Fatal("expected error for non-group-2 element in file meta")
	}
}
