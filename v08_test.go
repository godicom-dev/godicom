package godicom

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func intValuesFromElement(t *testing.T, elem *Element) []int {
	t.Helper()
	switch v := elem.Value.(type) {
	case []int:
		return v
	case *MultiValue[int64]:
		vals := v.Values()
		out := make([]int, len(vals))
		for i, x := range vals {
			out[i] = int(x)
		}
		return out
	case *MultiValue[uint64]:
		vals := v.Values()
		out := make([]int, len(vals))
		for i, x := range vals {
			out[i] = int(x)
		}
		return out
	default:
		t.Fatalf("unexpected LUTDescriptor value type %T", elem.Value)
		return nil
	}
}

func TestCorrectAmbiguousVRPixelRepresentationVMThree(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_pixel_representation_vm_three
	ref := NewDataset()
	ref.Set(NewDataElement(MustTag(0x00280103), VRUS, 0))
	ref.Set(NewDataElement(MustTag(0x00283002), VRUsSS, []byte{0x01, 0x00, 0x00, 0x01, 0x10, 0x00}))

	ds := cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(MustTag(0x00283002))
	if !ok || elem.VR != VRUS {
		t.Fatalf("VR = %s, want US", elem.VR)
	}
	if got := intValuesFromElement(t, elem); !slicesEqual(got, []int{1, 256, 16}) {
		t.Fatalf("LUTDescriptor = %v, want [1 256 16]", got)
	}

	ref.Set(NewDataElement(MustTag(0x00280103), VRUS, 1))
	ref.Set(NewDataElement(MustTag(0x00283002), VRUsSS, []byte{0x01, 0x00, 0x00, 0x01, 0x00, 0x10}))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, false, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok = ds.Get(MustTag(0x00283002))
	if !ok || elem.VR != VRSS {
		t.Fatalf("VR = %s, want SS", elem.VR)
	}
	if got := intValuesFromElement(t, elem); !slicesEqual(got, []int{256, 1, 16}) {
		t.Fatalf("LUTDescriptor = %v, want [256 1 16]", got)
	}

	ref = NewDataset()
	ref.Set(NewDataElement(MustTag(0x00283002), VRUsSS, []byte{0x01, 0x00, 0x00, 0x01, 0x00, 0x10}))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok = ds.Get(MustTag(0x00283002))
	if !ok || elem.VR != VRUS {
		t.Fatalf("VR = %s, want US", elem.VR)
	}

	ref.Set(NewDataElement(MustTag(0x7FE00010), VROB, []byte("123")))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, false, nil); err == nil {
		t.Fatal("expected error when PixelData present without PixelRepresentation")
	} else if !strings.Contains(err.Error(), "PixelRepresentation") {
		t.Fatalf("error = %q, want PixelRepresentation", err.Error())
	}
}

func TestCorrectAmbiguousVRLUTDescriptor(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_lut_descriptor
	ref := NewDataset()
	ref.Set(NewDataElement(MustTag(0x00280103), VRUS, 0))
	ref.Set(NewDataElement(MustTag(0x00283002), VRUsSS, []byte{0x01, 0x00, 0x00, 0x01, 0x10, 0x00}))
	ref.Set(NewDataElement(MustTag(0x00283006), VRUsOw, []byte{0x00, 0x01}))

	ds := cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	desc, ok := ds.Get(MustTag(0x00283002))
	if !ok || desc.VR != VRUS {
		t.Fatalf("LUTDescriptor VR = %s, want US", desc.VR)
	}
	if got := intValuesFromElement(t, desc); len(got) == 0 || got[0] != 1 {
		t.Fatalf("LUTDescriptor[0] = %v, want 1", got)
	}
	v, ok := ds.GetInt(MustTag(0x00283006))
	if !ok || v != 256 {
		t.Fatalf("LUTData = %d, want 256", v)
	}
	lutData, ok := ds.Get(MustTag(0x00283006))
	if !ok || lutData.VR != VRUS {
		t.Fatalf("LUTData VR = %s, want US", lutData.VR)
	}

	ref.Set(NewDataElement(MustTag(0x00283002), VRUsSS, []byte{0x02, 0x00, 0x00, 0x01, 0x10, 0x00}))
	ref.Set(NewDataElement(MustTag(0x00283006), VRUsOw, []byte{0x00, 0x01, 0x00, 0x02}))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	desc, ok = ds.Get(MustTag(0x00283002))
	if !ok || desc.VR != VRUS {
		t.Fatalf("LUTDescriptor VR = %s, want US", desc.VR)
	}
	if got := intValuesFromElement(t, desc); len(got) == 0 || got[0] != 2 {
		t.Fatalf("LUTDescriptor[0] = %v, want 2", got)
	}
	lutData, ok = ds.Get(MustTag(0x00283006))
	if !ok || lutData.VR != VROW {
		t.Fatalf("LUTData VR = %s, want OW", lutData.VR)
	}
	raw, ok := lutData.Value.([]byte)
	if !ok || !bytes.Equal(raw, []byte{0x00, 0x01, 0x00, 0x02}) {
		t.Fatalf("LUTData = %v, want raw bytes", lutData.Value)
	}

	ref = NewDataset()
	ref.Set(NewDataElement(MustTag(0x00283006), VRUsOw, []byte{0x00, 0x01}))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err == nil {
		t.Fatal("expected error when LUTDescriptor missing")
	} else if !strings.Contains(err.Error(), "LUTDescriptor") {
		t.Fatalf("error = %q, want LUTDescriptor", err.Error())
	}
}

func TestCorrectAmbiguousVRPixelData(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_pixel_data
	ref := NewDataset()
	ref.Set(NewDataElement(MustTag(0x00280100), VRUS, 16))
	ref.Set(NewDataElement(MustTag(0x7FE00010), VRObOw, []byte{0x00, 0x01}))

	ds := cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	pixel, ok := ds.Get(MustTag(0x7FE00010))
	if !ok || pixel.VR != VROW {
		t.Fatalf("PixelData VR = %s, want OW", pixel.VR)
	}
	raw, ok := pixel.Value.([]byte)
	if !ok || !bytes.Equal(raw, []byte{0x00, 0x01}) {
		t.Fatalf("PixelData = %v", pixel.Value)
	}

	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, false, nil); err != nil {
		t.Fatal(err)
	}
	pixel, ok = ds.Get(MustTag(0x7FE00010))
	if !ok || pixel.VR != VROW {
		t.Fatalf("PixelData VR = %s, want OW", pixel.VR)
	}

	ref = NewDataset()
	ref.Set(NewDataElement(MustTag(0x00280100), VRUS, 8))
	ref.Set(NewDataElement(MustTag(0x00280010), VRUS, 2))
	ref.Set(NewDataElement(MustTag(0x00280011), VRUS, 2))
	ref.Set(NewDataElement(MustTag(0x7FE00010), VRObOw, []byte{0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00}))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	pixel, ok = ds.Get(MustTag(0x7FE00010))
	if !ok || pixel.VR != VROB {
		t.Fatalf("PixelData VR = %s, want OB", pixel.VR)
	}

	ref = NewDataset()
	ref.Set(NewDataElement(MustTag(0x7FE00010), VRObOw, []byte{0x00, 0x01}))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err == nil {
		t.Fatal("expected error when BitsAllocated missing")
	} else if !strings.Contains(err.Error(), "BitsAllocated") {
		t.Fatalf("error = %q, want BitsAllocated", err.Error())
	}
}

func TestCorrectAmbiguousVRPixelReprNoneInNearerImplicit(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_pixel_repr_none_in_nearer_implicit
	ds := datasetWithModalityLUTSequence(0)
	seq, ok := ds.GetSequence(MustTag(0x00283000))
	if !ok || seq.Len() != 1 {
		t.Fatal("ModalityLUTSequence missing")
	}
	seq.Get(0).Delete(MustTag(0x00280103))

	implicit := true
	outPath := filepath.Join(t.TempDir(), "pixel_repr_none_nearer.dcm")
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.50")))
	if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	rseq, ok := reread.GetSequence(MustTag(0x00283000))
	if !ok || rseq.Len() != 1 {
		t.Fatal("ModalityLUTSequence missing after read")
	}
	desc, ok := rseq.Get(0).Get(MustTag(0x00283002))
	if !ok || desc.VR != VRUS {
		t.Fatalf("LUTDescriptor VR = %s, want US", desc.VR)
	}
}

func TestCorrectAmbiguousVRPixelReprNoneInFurtherImplicit(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_pixel_repr_none_in_further_implicit
	item := NewDataset()
	item.Set(NewDataElement(MustTag("LUTDescriptor"), VRUsSS, []int{0, 0, 16}))
	item.Set(NewDataElement(MustTag("LUTExplanation"), VRLO, nil))
	item.Set(NewDataElement(MustTag("ModalityLUTType"), VRCS, "US"))
	item.Set(NewDataElement(MustTag("LUTData"), VRObOw, []byte{
		0x00, 0x00, 0x14, 0x9a, 0x1f, 0x1c, 0xc2, 0x63, 0x37,
	}))
	item.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 0))

	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("ModalityLUTSequence"), VRSQ, NewSequence([]*Dataset{item})))

	implicit := true
	outPath := filepath.Join(t.TempDir(), "pixel_repr_none_further.dcm")
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.51")))
	if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	rseq, ok := reread.GetSequence(MustTag(0x00283000))
	if !ok || rseq.Len() != 1 {
		t.Fatal("ModalityLUTSequence missing after read")
	}
	desc, ok := rseq.Get(0).Get(MustTag(0x00283002))
	if !ok || desc.VR != VRUS {
		t.Fatalf("LUTDescriptor VR = %s, want US", desc.VR)
	}
}

func TestCorrectAmbiguousVRModalityLUTSequenceImplicitAttribute(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_ambiguous_element_in_sequence_implicit_using_attribute
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
			outPath := filepath.Join(t.TempDir(), "modality_lut_implicit_attr.dcm")
			if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit}); err != nil {
				t.Fatal(err)
			}

			reread, err := ReadFile(outPath, &ReadOptions{Force: true})
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

func TestWriteNonStandardNoPreamble(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteNoPreamble.test_no_preamble
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.Preamble = nil

	outPath := filepath.Join(t.TempDir(), "no_preamble_raw.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) >= 128 && bytes.Equal(data[:128], make([]byte, 128)) {
		t.Fatal("file starts with 128 zero bytes preamble")
	}
	if len(data) >= 4 && string(data[:4]) == "DICM" {
		t.Fatal("file starts with DICM prefix without preamble")
	}
}

func TestWriteNonStandardFileMetaUnchanged(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteNoPreamble.test_file_meta_unchanged
	ds, err := ReadFile(testFilePath("rtplan.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.FileMeta = NewFileMetaDataset()

	outPath := filepath.Join(t.TempDir(), "file_meta_unchanged.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}
	if ds.FileMeta.Len() != 0 {
		t.Fatalf("file meta len = %d, want 0", ds.FileMeta.Len())
	}
}

func TestWriteNonStandardDatasetOnly(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteNoPreamble.test_dataset
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.Preamble = nil
	ds.FileMeta = nil

	outPath := filepath.Join(t.TempDir(), "dataset_only.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if reread.Preamble != nil {
		t.Fatal("expected nil preamble on readback")
	}
	if reread.FileMeta != nil && reread.FileMeta.Len() > 0 {
		t.Fatal("expected empty file meta on readback")
	}
	if _, ok := reread.GetString(MustTag("PatientID")); !ok {
		t.Fatal("PatientID missing from dataset-only file")
	}
}

func TestWriteNonStandardPreambleDataset(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteNoPreamble.test_preamble_dataset
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	preamble := append([]byte(nil), ds.Preamble...)
	ds.FileMeta = nil

	outPath := filepath.Join(t.TempDir(), "preamble_dataset.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 132 {
		t.Fatal("written data too short")
	}
	if !bytes.Equal(data[:128], preamble) {
		t.Fatal("preamble mismatch")
	}
	if string(data[128:132]) != "DICM" {
		t.Fatalf("prefix = %q, want DICM", data[128:132])
	}

	reread, err := ReadFile(outPath, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if reread.FileMeta != nil && reread.FileMeta.Len() > 0 {
		t.Fatal("expected empty file meta")
	}
	if _, ok := reread.GetString(MustTag("PatientID")); !ok {
		t.Fatal("PatientID missing")
	}
}

func slicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
