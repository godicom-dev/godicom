package godicom

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// pydicom.tests.test_filewriter.TestWriteFileTransferSyntax.test_convert_big_to_little
func TestWriteTransferSyntaxConvertBigToLittle(t *testing.T) {
	ds, err := ReadFile(testFilePath("MR_small_bigendian.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.originalEnc.IsLittleEndian {
		t.Fatal("source should be big endian")
	}

	ds.FileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ExplicitVRLittleEndian))
	outPath := filepath.Join(t.TempDir(), "mr_le.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}

	out, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.originalEnc.IsLittleEndian {
		t.Fatal("converted file should be little endian")
	}
	if out.originalEnc.IsImplicitVR {
		t.Fatal("converted file should be explicit VR")
	}

	ref, err := ReadFile(testFilePath("MR_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	compareDatasetsExceptPixelData(t, ref, out)
}

// pydicom.tests.test_filewriter.TestWriteFileTransferSyntax.test_convert_little_to_big
func TestWriteTransferSyntaxConvertLittleToBig(t *testing.T) {
	ds, err := ReadFile(testFilePath("MR_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.FileMeta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ExplicitVRBigEndian))
	little := false
	outPath := filepath.Join(t.TempDir(), "mr_be.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{LittleEndian: &little}); err != nil {
		t.Fatal(err)
	}

	out, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.originalEnc.IsLittleEndian {
		t.Fatal("converted file should be big endian")
	}

	ref, err := ReadFile(testFilePath("MR_small_bigendian.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	compareDatasetsExceptPixelData(t, ref, out)
}

func compareDatasetsExceptPixelData(t *testing.T, ref, got *FileDataset) {
	t.Helper()
	pixelTag := MustTag("PixelData")
	gotTags := make(map[Tag]struct{}, got.Len())
	for _, tag := range got.SortedTags() {
		gotTags[tag] = struct{}{}
	}
	for _, tag := range ref.SortedTags() {
		if tag == pixelTag {
			continue
		}
		if _, ok := gotTags[tag]; !ok {
			continue
		}
		re, _ := ref.Get(tag)
		ge, ok := got.Get(tag)
		if !ok {
			t.Fatalf("tag %s missing in converted dataset", tag)
		}
		if err := elementsEqual(re, ge); err != nil {
			t.Fatalf("tag %s: %v", tag, err)
		}
	}
}

// pydicom.tests.test_filewriter.TestWriteFile.test_raw_elements_preserved_implicit_vr
func TestWriteRawElementsPreservedImplicit(t *testing.T) {
	ds, err := ReadFile(testFilePath("rtplan.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}

	rawTags := []Tag{
		MustTag("Manufacturer"),
		MustTag("PatientID"),
		MustTag("RTPlanDate"),
	}
	for _, tag := range rawTags {
		elem, ok := ds.Get(tag)
		if !ok || !elem.IsRaw() {
			t.Fatalf("tag %s should be raw after read", tag)
		}
	}

	outPath := filepath.Join(t.TempDir(), "rtplan_raw.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	for _, tag := range rawTags {
		elem, ok := ds.Get(tag)
		if !ok || !elem.IsRaw() {
			t.Fatalf("tag %s should remain raw after write", tag)
		}
	}
}

// pydicom.tests.test_filewriter.TestWriteFile.test_raw_elements_preserved_explicit_vr
func TestWriteRawElementsPreservedExplicit(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}

	rawTags := []Tag{
		MustTag("Manufacturer"),
		MustTag("PatientID"),
		MustTag("StudyTime"),
	}
	for _, tag := range rawTags {
		elem, ok := ds.Get(tag)
		if !ok || !elem.IsRaw() {
			t.Fatalf("tag %s should be raw after read", tag)
		}
	}

	outPath := filepath.Join(t.TempDir(), "ct_raw.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	for _, tag := range rawTags {
		elem, ok := ds.Get(tag)
		if !ok || !elem.IsRaw() {
			t.Fatalf("tag %s should remain raw after write", tag)
		}
	}
}

// pydicom.tests.test_filewriter.TestWriteFileMetaInfo.test_missing_elements
func TestValidateFileMetaMissingRequired(t *testing.T) {
	meta := NewFileMetaDataset()
	meta.Set(NewDataElement(MustTag("MediaStorageSOPClassUID"), VRUI, "1.1"))
	err := ValidateFileMeta(meta, true)
	if err == nil {
		t.Fatal("expected error for missing required file meta elements")
	}
	if !strings.Contains(err.Error(), "TransferSyntaxUID") {
		t.Fatalf("error = %q, want TransferSyntaxUID mentioned", err)
	}
}

// pydicom.tests.test_filereader.TestReader.test_un_sequence
func TestReadUNSequenceSemantic(t *testing.T) {
	ds, err := ReadFile(testFilePath("UN_sequence.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(MustTag(0x4453100C))
	if !ok {
		t.Fatal("(4453,100C) missing")
	}
	if elem.VR != VRSQ {
		t.Fatalf("VR = %s, want SQ", elem.VR)
	}
	seq, ok := elem.Value.(*Sequence)
	if !ok || seq.Len() != 1 {
		t.Fatalf("sequence = %#v, want 1 item", elem.Value)
	}
	item := seq.Get(0)
	nested, ok := item.GetSequence(MustTag("ReferencedSeriesSequence"))
	if !ok || nested.Len() != 1 {
		t.Fatal("ReferencedSeriesSequence missing or empty")
	}
}

// pydicom.tests.test_filereader.TestReader.test_correct_ambiguous_vr
func TestReadCorrectAmbiguousVRImplicit(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PixelRepresentation"), VRUS, 0))
	ds.Set(NewDataElement(MustTag(0x00280108), VRUsSS, 10))
	ds.Set(NewDataElement(MustTag(0x00280109), VRUsSS, 500))
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, UID("1.2.840.10008.5.1.4.1.1.4")))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.40")))

	implicit := true
	outPath := filepath.Join(t.TempDir(), "ambig_read.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{ImplicitVR: &implicit, EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := reread.Get(MustTag(0x00280108))
	if !ok || elem.VR != VRUS {
		t.Fatalf("SmallestPixelValueInSeries VR = %s, want US", elem.VR)
	}
	v, ok := reread.GetInt(MustTag(0x00280108))
	if !ok || v != 10 {
		t.Fatalf("SmallestPixelValueInSeries = %d, want 10", v)
	}
}

// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_overlay
func TestCorrectAmbiguousVROverlay(t *testing.T) {
	ref := NewDataset()
	ref.originalEnc = EncodingInfo{IsImplicitVR: true, IsLittleEndian: true}
	ref.Set(NewDataElement(MustTag(0x60003000), VRObOw, []byte{0x00}))
	ref.Set(NewDataElement(MustTag(0x601E3000), VRObOw, []byte{0x00}))

	ds := cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []Tag{MustTag(0x60003000), MustTag(0x601E3000)} {
		elem, ok := ds.Get(tag)
		if !ok || elem.VR != VROW {
			t.Fatalf("%s VR = %s, want OW", tag, elem.VR)
		}
	}
	orig, _ := ref.Get(MustTag(0x60003000))
	if orig.VR != VRObOw {
		t.Fatalf("reference dataset should be unchanged, VR = %s", orig.VR)
	}

	ref.originalEnc = EncodingInfo{IsImplicitVR: false, IsLittleEndian: true}
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(MustTag(0x60003000))
	if !ok || elem.VR != VROW {
		t.Fatalf("explicit overlay VR = %s, want OW", elem.VR)
	}
}

// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_waveform_bits_allocated
func TestCorrectAmbiguousVRWaveform(t *testing.T) {
	ref := NewDataset()
	ref.originalEnc = EncodingInfo{IsImplicitVR: false, IsLittleEndian: true}
	ref.Set(NewDataElement(MustTag("WaveformBitsAllocated"), VRUS, 16))
	ref.Set(NewDataElement(MustTag(0x54001010), VRObOw, []byte{0x00, 0x01}))

	ds := cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(MustTag(0x54001010))
	if !ok || elem.VR != VROW {
		t.Fatalf("WaveformData VR = %s, want OW", elem.VR)
	}

	ref.Set(NewDataElement(MustTag("WaveformBitsAllocated"), VRUS, 8))
	ref.Set(NewDataElement(MustTag(0x54001010), VRObOw, []byte{0x01, 0x02}))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok = ds.Get(MustTag(0x54001010))
	if !ok || elem.VR != VROB {
		t.Fatalf("WaveformData VR = %s, want OB", elem.VR)
	}

	ref.originalEnc = EncodingInfo{IsImplicitVR: true, IsLittleEndian: true}
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	elem, ok = ds.Get(MustTag(0x54001010))
	if !ok || elem.VR != VROW {
		t.Fatalf("implicit WaveformData VR = %s, want OW", elem.VR)
	}

	ref = NewDataset()
	ref.Set(NewDataElement(MustTag(0x54001010), VRObOw, []byte{0x00, 0x01}))
	ds = cloneDataset(ref)
	if err := CorrectAmbiguousVR(ds, true, nil); err == nil {
		t.Fatal("expected error when WaveformBitsAllocated missing")
	}
}

// pydicom.tests.test_filewriter.TestWriteFileMetaInfo.test_group_length
func TestWriteFileMetaGroupLengthComputed(t *testing.T) {
	meta := NewFileMetaDataset()
	meta.Set(NewDataElement(MustTag("MediaStorageSOPClassUID"), VRUI, "1.1"))
	meta.Set(NewDataElement(MustTag("MediaStorageSOPInstanceUID"), VRUI, "1.2"))
	meta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, "1.3"))

	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)
	if err := writeFileMetaInfo(fp, meta, true); err != nil {
		t.Fatal(err)
	}
	got, ok := meta.GetInt(MustTag("FileMetaInformationGroupLength"))
	if !ok {
		t.Fatal("FileMetaInformationGroupLength missing")
	}
	if got <= 0 || int(got) != buf.Len()-12 {
		t.Fatalf("FileMetaInformationGroupLength = %d, want %d", got, buf.Len()-12)
	}
}
