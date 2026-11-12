package godicom

// Ports of pydicom TestWriteFileMetaInfoToStandard (tests/test_filewriter.py)
// and the File Meta half of TestDCMWrite_EnforceFileFormat.
//
// Where pydicom reaches into a BytesIO with seek/unpack, these cases read the
// encoded bytes back with ReadBytes and assert on the parsed File Meta, which
// checks the same thing without hard-coding element offsets.

import (
	"bytes"
	"strings"
	"testing"
)

// completeFileMeta returns a File Meta carrying exactly the three elements
// PS3.5 makes mandatory, and nothing else.
func completeFileMeta(t *testing.T) *FileMetaDataset {
	t.Helper()
	meta := NewFileMetaDataset()
	meta.SetString(MustTag("MediaStorageSOPClassUID"), "1.1")
	meta.SetString(MustTag("MediaStorageSOPInstanceUID"), "1.2")
	meta.SetString(MustTag("TransferSyntaxUID"), string(ExplicitVRLittleEndian))
	return meta
}

// encodeStandardFile writes ds with the meta required for a Part 10 file and
// EnforceFileFormat on, which is the path that fills the derived elements.
func encodeStandardFile(t *testing.T, ds *Dataset, meta *FileMetaDataset) []byte {
	t.Helper()
	out, err := EncodeFile(&FileDataset{Dataset: ds, FileMeta: meta},
		&WriteOptions{EnforceFileFormat: true})
	if err != nil {
		t.Fatalf("EncodeFile() error = %v", err)
	}
	return out
}

// TestWriteFileMetaInformationVersion checks the version value a standard file
// carries. pydicom: test_version.
func TestWriteFileMetaInformationVersion(t *testing.T) {
	out := encodeStandardFile(t, NewDataset(), completeFileMeta(t))

	fd, err := ReadBytes(out, nil)
	if err != nil {
		t.Fatalf("ReadBytes() error = %v", err)
	}
	elem, ok := fd.FileMeta.Get(MustTag("FileMetaInformationVersion"))
	if !ok {
		t.Fatal("FileMetaInformationVersion is missing from the written meta")
	}
	got, ok := elem.Value.([]byte)
	if !ok {
		t.Fatalf("FileMetaInformationVersion value is %T, want []byte", elem.Value)
	}
	if !bytes.Equal(got, []byte{0x00, 0x01}) {
		t.Fatalf("FileMetaInformationVersion = % x, want 00 01", got)
	}
}

// TestWriteImplementationElementLengths checks the two length ceilings the
// standard puts on the elements godicom fills in itself: VR SH is 16 bytes,
// VR UI is 64. pydicom: test_implementation_version_name_length and
// test_implementation_class_uid_length.
func TestWriteImplementationElementLengths(t *testing.T) {
	out := encodeStandardFile(t, NewDataset(), completeFileMeta(t))

	fd, err := ReadBytes(out, nil)
	if err != nil {
		t.Fatalf("ReadBytes() error = %v", err)
	}

	if v, ok := fd.FileMeta.GetString(MustTag("ImplementationVersionName")); !ok {
		t.Fatal("ImplementationVersionName is missing")
	} else if len(v) > 16 {
		t.Errorf("ImplementationVersionName %q is %d characters, over the 16 an SH allows", v, len(v))
	}

	if v, ok := fd.FileMeta.GetString(MustTag("ImplementationClassUID")); !ok {
		t.Fatal("ImplementationClassUID is missing")
	} else if len(v) > 64 {
		t.Errorf("ImplementationClassUID %q is %d characters, over the 64 a UI allows", v, len(v))
	}
}

// TestValidateFileMetaMissingRequired walks the four states of the three
// mandatory elements. pydicom: test_missing_elements.
func TestValidateFileMetaRequiredElementsMatrix(t *testing.T) {
	cases := []struct {
		name     string
		build    func() *FileMetaDataset
		wantErr  bool
		mentions []string
	}{
		{
			name:     "all_missing",
			build:    func() *FileMetaDataset { return NewFileMetaDataset() },
			wantErr:  true,
			mentions: []string{"MediaStorageSOPClassUID", "MediaStorageSOPInstanceUID", "TransferSyntaxUID"},
		},
		{
			name: "class_present",
			build: func() *FileMetaDataset {
				m := NewFileMetaDataset()
				m.SetString(MustTag("MediaStorageSOPClassUID"), "1.1")
				return m
			},
			wantErr:  true,
			mentions: []string{"MediaStorageSOPInstanceUID", "TransferSyntaxUID"},
		},
		{
			name: "instance_present",
			build: func() *FileMetaDataset {
				m := NewFileMetaDataset()
				m.SetString(MustTag("MediaStorageSOPClassUID"), "1.1")
				m.SetString(MustTag("MediaStorageSOPInstanceUID"), "1.2")
				return m
			},
			wantErr:  true,
			mentions: []string{"TransferSyntaxUID"},
		},
		{
			name:    "complete",
			build:   func() *FileMetaDataset { return completeFileMeta(t) },
			wantErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateFileMeta(c.build(), true)
			if !c.wantErr {
				if err != nil {
					t.Fatalf("ValidateFileMeta() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ValidateFileMeta() returned no error")
			}
			for _, want := range c.mentions {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %s", err, want)
				}
			}
		})
	}
}

// TestValidateFileMetaFillsDerivedElements checks that enforcing the standard
// supplies the three elements that have a defined default, and leaves the
// caller's own values alone. pydicom: test_group_length_updated's assertions on
// the returned dataset.
func TestValidateFileMetaFillsDerivedElements(t *testing.T) {
	meta := NewFileMetaDataset()
	meta.SetString(MustTag("MediaStorageSOPClassUID"), "1.1")
	meta.SetString(MustTag("MediaStorageSOPInstanceUID"), "1.2")
	meta.SetString(MustTag("TransferSyntaxUID"), "1.3")

	if err := ValidateFileMeta(meta, true); err != nil {
		t.Fatalf("ValidateFileMeta() error = %v", err)
	}

	if elem, ok := meta.Get(MustTag("FileMetaInformationVersion")); !ok || elem.IsEmpty() {
		t.Error("FileMetaInformationVersion was not filled in")
	}
	if v, _ := meta.GetString(MustTag("ImplementationClassUID")); v != string(GodicomImplementationUID) {
		t.Errorf("ImplementationClassUID = %q, want %q", v, GodicomImplementationUID)
	}
	if v, ok := meta.GetString(MustTag("ImplementationVersionName")); !ok || v == "" {
		t.Error("ImplementationVersionName was not filled in")
	}

	// The caller's values survive untouched.
	for tag, want := range map[Tag]string{
		MustTag("MediaStorageSOPClassUID"):    "1.1",
		MustTag("MediaStorageSOPInstanceUID"): "1.2",
		MustTag("TransferSyntaxUID"):          "1.3",
	} {
		if v, _ := meta.GetString(tag); v != want {
			t.Errorf("%s = %q, want %q", tag, v, want)
		}
	}
}

// A validate call that is not enforcing the standard must not modify anything:
// it is the cheap check the writer runs on every file, including ones it is
// deliberately writing non-standard.
func TestValidateFileMetaDoesNotMutateWhenNotEnforcing(t *testing.T) {
	meta := NewFileMetaDataset()
	meta.SetString(MustTag("MediaStorageSOPClassUID"), "1.1")

	before := len(meta.Elements())
	if err := ValidateFileMeta(meta, false); err != nil {
		t.Fatalf("ValidateFileMeta(_, false) error = %v", err)
	}
	if after := len(meta.Elements()); after != before {
		t.Fatalf("validate without enforcing added %d elements", after-before)
	}
}

// A non-group-2 element in the File Meta block would shift every offset a
// reader computes, and no reader expects one there. pydicom: test_bad_elements.
func TestWriteFileMetaRejectsNonGroup2Element(t *testing.T) {
	meta := completeFileMeta(t)
	meta.SetString(MustTag("PatientID"), "12345678") // (0010,0020)

	if _, err := EncodeFile(&FileDataset{Dataset: NewDataset(), FileMeta: meta},
		&WriteOptions{EnforceFileFormat: true}); err == nil {
		t.Fatal("EncodeFile() accepted a non-group-2 element in the File Meta")
	}
}

// EnforceFileFormat has to make a bare dataset into a legal Part 10 file:
// preamble, DICM, a filled-in File Meta, then the dataset. pydicom:
// TestDCMWrite_EnforceFileFormat.test_file_meta_sop_class_sop_instance.
func TestEnforceFileFormatBuildsCompleteFile(t *testing.T) {
	ds := NewDataset()
	_ = ds.SetString(MustTag("SOPClassUID"), "1.2.840.10008.5.1.4.1.1.2")
	_ = ds.SetString(MustTag("SOPInstanceUID"), "1.2.3.4.5")
	_ = ds.SetString(MustTag("PatientName"), "Doe^John")

	meta := NewFileMetaDataset()
	meta.SetString(MustTag("MediaStorageSOPClassUID"), "1.2.840.10008.5.1.4.1.1.2")
	meta.SetString(MustTag("MediaStorageSOPInstanceUID"), "1.2.3.4.5")
	meta.SetString(MustTag("TransferSyntaxUID"), string(ExplicitVRLittleEndian))

	out := encodeStandardFile(t, ds, meta)

	if len(out) < 132 || string(out[128:132]) != "DICM" {
		t.Fatal("the encoded file has no DICM prefix")
	}
	if !bytes.Equal(out[:128], make([]byte, 128)) {
		t.Fatal("the preamble is not 128 zero bytes")
	}

	fd, err := ReadBytes(out, nil)
	if err != nil {
		t.Fatalf("the file it produced does not read back: %v", err)
	}
	if v, _ := fd.FileMeta.GetString(MustTag("TransferSyntaxUID")); v != string(ExplicitVRLittleEndian) {
		t.Errorf("TransferSyntaxUID = %q, want the one supplied", v)
	}
	if v, _ := fd.GetString(MustTag("PatientName")); v != "Doe^John" {
		t.Errorf("PatientName = %q, want Doe^John", v)
	}
	if len(fd.Preamble) != 128 {
		t.Errorf("read-back preamble is %d bytes, want 128", len(fd.Preamble))
	}
}

// The File Meta the writer fills in must survive a second encode unchanged: a
// reader that writes back what it read must not keep adding elements.
func TestEnforceFileFormatIsIdempotent(t *testing.T) {
	ds := NewDataset()
	_ = ds.SetString(MustTag("SOPClassUID"), "1.2.840.10008.5.1.4.1.1.2")

	first := encodeStandardFile(t, ds, completeFileMeta(t))

	fd, err := ReadBytes(first, nil)
	if err != nil {
		t.Fatalf("ReadBytes() error = %v", err)
	}
	second := encodeStandardFile(t, fd.Dataset, fd.FileMeta)

	if len(first) != len(second) {
		t.Fatalf("re-encoding changed the file length from %d to %d", len(first), len(second))
	}
	if !bytes.Equal(first, second) {
		t.Fatal("re-encoding a file godicom wrote produced different bytes")
	}
}
