package godicom

import (
	"bytes"
	"fmt"
	"math"
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

func encodeElementExplicitLittle(elem *DataElement) []byte {
	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)
	_ = writeElement(fp, elem, false, true)
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

func TestWriteElementODExplicitLittle(t *testing.T) {
	bytestring := []byte{0, 1, 2, 3, 4, 5, 6, 7, 1, 1, 2, 3, 4, 5, 6, 7}
	elem := NewDataElement(MustTag(0x0070150D), VROD, bytestring)
	got := encodeElementExplicitLittle(elem)
	expected := append([]byte{0x70, 0x00, 0x0D, 0x15, 'O', 'D', 0x00, 0x00, 0x10, 0x00, 0x00, 0x00}, bytestring...)
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}

	elem.Value = []byte{}
	got = encodeElementExplicitLittle(elem)
	expected = []byte{0x70, 0x00, 0x0D, 0x15, 'O', 'D', 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, expected) {
		t.Fatalf("empty got = % X, want % X", got, expected)
	}
}

func TestWriteElementOLExplicitLittle(t *testing.T) {
	bytestring := []byte{0, 1, 2, 3, 4, 5, 6, 7, 1, 1, 2, 3}
	elem := NewDataElement(MustTag(0x00660129), VROL, bytestring)
	got := encodeElementExplicitLittle(elem)
	expected := append([]byte{0x66, 0x00, 0x29, 0x01, 'O', 'L', 0x00, 0x00, 0x0C, 0x00, 0x00, 0x00}, bytestring...)
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}
}

func TestWriteElementUCExplicitLittle(t *testing.T) {
	elem := NewDataElement(MustTag(0x00189908), VRUC, "Test")
	got := encodeElementExplicitLittle(elem)
	expected := []byte{0x18, 0x00, 0x08, 0x99, 'U', 'C', 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 'T', 'e', 's', 't'}
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}

	elem.Value = "Test."
	got = encodeElementExplicitLittle(elem)
	expected = []byte{0x18, 0x00, 0x08, 0x99, 'U', 'C', 0x00, 0x00, 0x06, 0x00, 0x00, 0x00, 'T', 'e', 's', 't', '.', ' '}
	if !bytes.Equal(got, expected) {
		t.Fatalf("odd got = % X, want % X", got, expected)
	}

	elem.Value = ""
	got = encodeElementExplicitLittle(elem)
	expected = []byte{0x18, 0x00, 0x08, 0x99, 'U', 'C', 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, expected) {
		t.Fatalf("empty got = % X, want % X", got, expected)
	}
}

func TestWriteElementURExplicitLittle(t *testing.T) {
	elem := NewDataElement(MustTag(0x00080120), VRUR, "ftp://bits")
	got := encodeElementExplicitLittle(elem)
	expected := []byte{
		0x08, 0x00, 0x20, 0x01, 'U', 'R', 0x00, 0x00, 0x0A, 0x00, 0x00, 0x00,
		'f', 't', 'p', ':', '/', '/', 'b', 'i', 't', 's',
	}
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}

	elem.Value = "ftp://bit"
	got = encodeElementExplicitLittle(elem)
	expected = []byte{
		0x08, 0x00, 0x20, 0x01, 'U', 'R', 0x00, 0x00, 0x0A, 0x00, 0x00, 0x00,
		'f', 't', 'p', ':', '/', '/', 'b', 'i', 't', ' ',
	}
	if !bytes.Equal(got, expected) {
		t.Fatalf("odd got = % X, want % X", got, expected)
	}
}

func TestWriteElementUNExplicitLittle(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRUN, []byte{0x01, 0x02})
	got := encodeElementExplicitLittle(elem)
	expected := []byte{0x10, 0x00, 0x10, 0x00, 'U', 'N', 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01, 0x02}
	if !bytes.Equal(got, expected) {
		t.Fatalf("got = % X, want % X", got, expected)
	}

	elem.Value = []byte{0x01}
	got = encodeElementExplicitLittle(elem)
	expected = []byte{0x10, 0x00, 0x10, 0x00, 'U', 'N', 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01, 0x00}
	if !bytes.Equal(got, expected) {
		t.Fatalf("odd got = % X, want % X", got, expected)
	}
}

func bytesIdentical(a, b []byte) (bool, int) {
	if len(a) != len(b) {
		return false, min(len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			return false, i
		}
	}
	return true, 0
}

func TestWriteFileBytesIdentical(t *testing.T) {
	files := []string{
		"CT_small.dcm",
		"MR_small.dcm",
		"rtplan.dcm",
		"rtdose.dcm",
		"MR_small_bigendian.dcm",
		"JPEG2000.dcm",
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			assertWriteBytesIdentical(t, file)
		})
	}
}

func assertWriteBytesIdentical(t *testing.T, file string) {
	t.Helper()
	path := testFilePath(file)
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
		t.Fatalf("bytes differ at %d (orig=%d written=%d)", pos, len(original), len(written))
	}
}

func TestWriteFileDeflatedInflatedIdentical(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteFile.test_write_deflated_deflates_post_file_meta
	const postMetaOffset = 0x14E
	path := testFilePath("image_dfl.dcm")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	ds, err := ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(t.TempDir(), "image_dfl.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	origInflated, err := inflateRaw(original[postMetaOffset:])
	if err != nil {
		t.Fatal(err)
	}
	writInflated, err := inflateRaw(written[postMetaOffset:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(origInflated, writInflated) {
		t.Fatalf("inflated dataset differs (orig=%d written=%d)", len(origInflated), len(writInflated))
	}
}

func TestWriteFileDeflatedRetainsElements(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteFile.test_write_deflated_retains_elements
	original, err := ReadFile(testFilePath("image_dfl.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(t.TempDir(), "image_dfl.dcm")
	if err := original.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	if original.Len() != reread.Len() {
		t.Fatalf("element count: original=%d reread=%d", original.Len(), reread.Len())
	}
	for _, tag := range original.SortedTags() {
		origElem, _ := original.Get(tag)
		rereadElem, ok := reread.Get(tag)
		if !ok {
			t.Fatalf("tag %s missing on reread", tag)
		}
		if err := elementsEqual(origElem, rereadElem); err != nil {
			t.Fatalf("tag %s: %v", tag, err)
		}
	}
}

func TestWriteFileRemovesGroupLength(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteFile.test_write_removes_grouplength
	// color-pl.dcm is not in the submodule; use any file that contains a retired group length.
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Inject a retired group length element (group > 6, element 0).
	ds.Set(NewDataElement(MustTag(0x00080000), VRUL, uint32(42)))
	if !ds.Has(MustTag(0x00080000)) {
		t.Fatal("expected injected group length element")
	}

	outPath := filepath.Join(t.TempDir(), "no_group_length.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Has(MustTag(0x00080000)) {
		t.Fatal("retired group length element should not be written")
	}
}

func TestWriteElementRawUndefinedExplicitLongVR(t *testing.T) {
	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)
	elem := NewDataElement(MustTag("PixelData"), VROB, nil)
	elem.RawValue = []byte{0x01, 0x02}
	elem.IsUndefinedLength = true

	if err := writeElement(fp, elem, false, true); err != nil {
		t.Fatal(err)
	}

	expected := []byte{
		0xE0, 0x7F, 0x10, 0x00,
		'O', 'B', 0x00, 0x00,
		0xFF, 0xFF, 0xFF, 0xFF,
		0x01, 0x02,
		0xFE, 0xFF, 0xDD, 0xE0,
		0x00, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Fatalf("got = % X, want % X", buf.Bytes(), expected)
	}
}

func TestWriteFileEnforceFileFormatDefaultsExplicitLittleEndian(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.123")))

	outPath := filepath.Join(t.TempDir(), "enforced.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts, ok := reread.FileMeta.GetString(MustTag("TransferSyntaxUID"))
	if !ok || ts != string(ExplicitVRLittleEndian) {
		t.Fatalf("TransferSyntaxUID = %q, %t; want ExplicitVRLittleEndian", ts, ok)
	}
}

func TestWriteFileMetaMissingTransferSyntaxRoundtrip(t *testing.T) {
	// pydicom test_filewriter uses meta_missing_tsyntax.dcm for non-standard file meta
	original, err := ReadFile(testFilePath("meta_missing_tsyntax.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "meta_missing.dcm")
	if err := original.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}
	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if original.Len() != reread.Len() {
		t.Fatalf("element count %d vs %d", original.Len(), reread.Len())
	}
	for _, tag := range original.FileMeta.SortedTags() {
		oe, _ := original.FileMeta.Get(tag)
		re, ok := reread.FileMeta.Get(tag)
		if !ok {
			t.Fatalf("file meta tag %s missing on reread", tag)
		}
		if err := elementsEqual(oe, re); err != nil {
			t.Fatalf("file meta %s: %v", tag, err)
		}
	}
}

func TestWriteFileRoundtripValues(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"CT", "CT_small.dcm"},
		{"MR", "MR_small.dcm"},
		{"RTPlan", "rtplan.dcm"},
		{"RTDose", "rtdose.dcm"},
		{"JPEG2000", "JPEG2000.dcm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, err := ReadFile(testFilePath(tt.file), nil)
			if err != nil {
				t.Fatal(err)
			}

			outPath := filepath.Join(t.TempDir(), "roundtrip_"+tt.name+".dcm")
			if err := original.SaveAs(outPath, nil); err != nil {
				t.Fatal(err)
			}

			reread, err := ReadFile(outPath, nil)
			if err != nil {
				t.Fatal(err)
			}

			if original.Len() != reread.Len() {
				t.Fatalf("element count: original=%d reread=%d", original.Len(), reread.Len())
			}

			if original.FileMeta.Len() != reread.FileMeta.Len() {
				t.Fatalf("file meta count: original=%d reread=%d", original.FileMeta.Len(), reread.FileMeta.Len())
			}

			tsOriginal, _ := original.FileMeta.GetString(MustTag("TransferSyntaxUID"))
			tsReread, ok := reread.FileMeta.GetString(MustTag("TransferSyntaxUID"))
			if !ok || tsOriginal != tsReread {
				t.Fatalf("TransferSyntaxUID: original=%q reread=%q", tsOriginal, tsReread)
			}

			for _, tag := range original.SortedTags() {
				origElem, _ := original.Get(tag)
				rereadElem, ok := reread.Get(tag)
				if !ok {
					t.Fatalf("tag %s missing on reread", tag)
				}
				if err := elementsEqual(origElem, rereadElem); err != nil {
					t.Fatalf("tag %s: %v", tag, err)
				}
			}
		})
	}
}

func TestWriteFileRTPlanSequenceRoundtrip(t *testing.T) {
	// pydicom read_test RTPlan nested sequence assertions after write/read
	original, err := ReadFile(testFilePath("rtplan.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "rtplan_roundtrip.dcm")
	if err := original.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}
	plan, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	beamSeq, ok := plan.GetSequence(MustTag("BeamSequence"))
	if !ok || beamSeq.Len() != 1 {
		t.Fatalf("BeamSequence missing or len=%d", beamSeq.Len())
	}
	beam := beamSeq.Get(0)
	machineName, ok := beam.GetString(MustTag("TreatmentMachineName"))
	if !ok || machineName != "unit001" {
		t.Fatalf("TreatmentMachineName = %q, want unit001", machineName)
	}
	cpSeq, ok := beam.GetSequence(MustTag("ControlPointSequence"))
	if !ok || cpSeq.Len() < 2 {
		t.Fatal("ControlPointSequence missing or too short")
	}
	cp1 := cpSeq.Get(1)
	doseSeq, ok := cp1.GetSequence(MustTag("ReferencedDoseReferenceSequence"))
	if !ok || doseSeq.Len() == 0 {
		t.Fatal("ReferencedDoseReferenceSequence missing")
	}
	doseRef := doseSeq.Get(0)
	coeff, ok := doseRef.GetFloat(MustTag("CumulativeDoseReferenceCoefficient"))
	if !ok || math.Abs(coeff-0.9990268) > 1e-9 {
		t.Fatalf("CumulativeDoseReferenceCoefficient = %g, want 0.9990268", coeff)
	}
}

func TestWriteFileMetaGroupLengthUpdated(t *testing.T) {
	// pydicom test_filewriter.TestWriteFileMetaInfoNonStandard.test_group_length_updated
	meta := NewFileMetaDataset()
	meta.Set(NewDataElement(MustTag("FileMetaInformationGroupLength"), VRUL, uint32(100)))
	meta.Set(NewDataElement(MustTag("MediaStorageSOPClassUID"), VRUI, "1.1"))
	meta.Set(NewDataElement(MustTag("MediaStorageSOPInstanceUID"), VRUI, "1.2"))
	meta.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, "1.3"))
	meta.Set(NewDataElement(MustTag("ImplementationClassUID"), VRUI, "1.4"))

	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)
	if err := writeFileMetaInfo(fp, meta, true); err != nil {
		t.Fatal(err)
	}
	if got, ok := meta.GetInt(MustTag("FileMetaInformationGroupLength")); !ok || got != 48 {
		t.Fatalf("FileMetaInformationGroupLength = %d, want 48", got)
	}
}

func elementsEqual(a, b *Element) error {
	if a.VR != b.VR {
		return fmt.Errorf("VR: original=%s reread=%s", a.VR, b.VR)
	}
	if a.RawValue != nil || b.RawValue != nil {
		if !bytes.Equal(a.RawValue, b.RawValue) {
			return fmt.Errorf("RawValue differs")
		}
		return nil
	}
	return valuesEqual(a.Value, b.Value)
}

func valuesEqual(a, b interface{}) error {
	switch av := a.(type) {
	case *Sequence:
		bv, ok := b.(*Sequence)
		if !ok {
			return fmt.Errorf("value type mismatch: %T vs %T", a, b)
		}
		if av.Len() != bv.Len() {
			return fmt.Errorf("sequence len %d vs %d", av.Len(), bv.Len())
		}
		for i := 0; i < av.Len(); i++ {
			ai, bi := av.Get(i), bv.Get(i)
			if ai.Len() != bi.Len() {
				return fmt.Errorf("sequence item %d element count %d vs %d", i, ai.Len(), bi.Len())
			}
			for _, tag := range ai.SortedTags() {
				ae, _ := ai.Get(tag)
				be, ok := bi.Get(tag)
				if !ok {
					return fmt.Errorf("sequence item %d missing tag %s", i, tag)
				}
				if err := elementsEqual(ae, be); err != nil {
					return fmt.Errorf("sequence item %d tag %s: %w", i, tag, err)
				}
			}
		}
		return nil
	default:
		oa := fmt.Sprintf("%v", a)
		ob := fmt.Sprintf("%v", b)
		if oa != ob {
			return fmt.Errorf("value: original=%q reread=%q", oa, ob)
		}
		return nil
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
