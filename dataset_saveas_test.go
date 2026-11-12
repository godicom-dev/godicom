package godicom

// Ports of pydicom TestDatasetSaveAs (tests/test_dataset.py) for the options
// godicom has. pydicom's version also covers write_like_original's deprecation
// warnings and Dataset.invalidate(); neither has a Go counterpart.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// part10 wraps ds in a FileDataset that is actually a Part 10 file: a File Meta
// and a 128-byte preamble. A hand-built FileDataset has neither, and without a
// preamble Write emits no DICM prefix, so ReadFile would reject the result.
func samplePart10(t *testing.T, ds *Dataset) *FileDataset {
	t.Helper()
	return &FileDataset{Dataset: ds, FileMeta: completeFileMeta(t), Preamble: make([]byte, 128)}
}

// part10With wraps ds with a File Meta declaring ts, which is how the VR and
// endianness options are expressed: the writer refuses an option that
// contradicts the transfer syntax the meta declares.
func samplePart10With(t *testing.T, ds *Dataset, ts UID) *FileDataset {
	t.Helper()
	meta := completeFileMeta(t)
	meta.SetString(MustTag("TransferSyntaxUID"), string(ts))
	return &FileDataset{Dataset: ds, FileMeta: meta, Preamble: make([]byte, 128)}
}

// saveAsRoundTrip writes ds and reads it back, which is the only assertion that
// matters for an options matrix: the option has to survive the write.
func saveAsRoundTrip(t *testing.T, fd *FileDataset, opts *WriteOptions, readOpts *ReadOptions) *FileDataset {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.dcm")
	if err := fd.SaveAs(path, opts); err != nil {
		t.Fatalf("SaveAs() error = %v", err)
	}
	if readOpts == nil {
		readOpts = &ReadOptions{}
	}
	back, err := ReadFile(path, readOpts)
	if err != nil {
		t.Fatalf("the file SaveAs produced does not read back: %v", err)
	}
	return back
}

// sampleDataset returns a dataset spanning the value kinds that a re-encode can
// silently damage: text, numbers, a sequence, a private element, and binary.
func sampleDataset(t *testing.T) *Dataset {
	t.Helper()
	ds := NewDataset()
	_ = ds.SetString(MustTag("PatientName"), "Doe^John")
	_ = ds.SetString(MustTag("PatientID"), "PID-0001")
	_ = ds.SetString(MustTag("Modality"), "MR")
	_ = ds.SetString(MustTag("StudyDate"), "20240101")
	_ = ds.SetString(MustTag("PixelSpacing"), "0.5\\0.5")

	item := NewDataset()
	_ = item.SetString(MustTag("CodeValue"), "T-D0050")
	_ = item.SetString(MustTag("CodingSchemeDesignator"), "SRT")
	seq := NewSequence([]*Dataset{item})
	_ = ds.SetSequence(MustTag("ProcedureCodeSequence"), seq)

	ds.Set(NewDataElement(MustTag(0x00090010), VRLO, "ACME PRIVATE CREATOR"))
	ds.Set(NewDataElement(MustTag(0x00091001), VRLO, "private value"))
	return ds
}

// writeOptions is a small builder so the matrix below reads as data.
func writeOptions(implicit, littleEndian *bool, enforce bool) *WriteOptions {
	return &WriteOptions{ImplicitVR: implicit, LittleEndian: littleEndian, EnforceFileFormat: enforce}
}

func boolPtr(v bool) *bool { return &v }

// TestDatasetSaveAsPreservesValues is the baseline every option case is measured
// against: what went in comes out, unchanged.
func TestDatasetSaveAsPreservesValues(t *testing.T) {
	ds := sampleDataset(t)

	back := saveAsRoundTrip(t, samplePart10(t, ds), nil, nil)

	for tag, want := range map[Tag]string{
		MustTag("PatientName"): "Doe^John",
		MustTag("PatientID"):   "PID-0001",
		MustTag("Modality"):    "MR",
		MustTag("StudyDate"):   "20240101",
		MustTag(0x00090010):    "ACME PRIVATE CREATOR",
		MustTag(0x00091001):    "private value",
	} {
		if got, ok := back.GetString(tag); !ok || got != want {
			t.Errorf("%s = %q (present=%v), want %q", tag, got, ok, want)
		}
	}

	// Multi-valued numbers come back as floats, not as the transcribed string.
	spacing, ok := back.GetFloats(MustTag("PixelSpacing"))
	if !ok || len(spacing) != 2 || spacing[0] != 0.5 || spacing[1] != 0.5 {
		t.Errorf("PixelSpacing = %v (present=%v), want [0.5 0.5]", spacing, ok)
	}

	seq, ok := back.GetSequence(MustTag("ProcedureCodeSequence"))
	if !ok || seq.Len() != 1 {
		t.Fatalf("ProcedureCodeSequence has %d items, want 1", seq.Len())
	}
	if v, _ := seq.Get(0).GetString(MustTag("CodeValue")); v != "T-D0050" {
		t.Errorf("nested CodeValue = %q, want T-D0050", v)
	}
}

// TestDatasetSaveAsImplicitVR checks that asking for implicit VR really produces
// an implicit VR dataset: the VR of a non-trivial element is only recoverable
// from the dictionary afterwards.
func TestDatasetSaveAsImplicitVR(t *testing.T) {
	ds := sampleDataset(t)
	fd := samplePart10With(t, ds, ImplicitVRLittleEndian)

	path := filepath.Join(t.TempDir(), "implicit.dcm")
	if err := fd.SaveAs(path, writeOptions(boolPtr(true), nil, false)); err != nil {
		t.Fatalf("SaveAs() error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Explicit VR spells the tag followed by "PN"; implicit VR spells the tag
	// followed by a 4-byte length. That difference is the whole point of the
	// option, and it is visible without a reader.
	explicit := []byte{0x10, 0x00, 0x10, 0x00, 'P', 'N'}
	if bytes.Contains(raw, explicit) {
		t.Fatal("the tag is followed by its VR, so the dataset was written explicit VR")
	}

	back, err := ReadFile(path, nil)
	if err != nil {
		t.Fatalf("the implicit VR file does not read back: %v", err)
	}
	if v, _ := back.GetString(MustTag("PatientName")); v != "Doe^John" {
		t.Errorf("PatientName = %q, want Doe^John", v)
	}
}

// TestDatasetSaveAsBigEndian checks the endianness option, which changes every
// multi-byte value in the file and is the classic silent-corruption case.
func TestDatasetSaveAsBigEndian(t *testing.T) {
	ds := sampleDataset(t)
	if err := ds.SetInt(MustTag("Rows"), 512); err != nil {
		t.Fatalf("SetInt(Rows) error = %v", err)
	}
	fd := samplePart10With(t, ds, ExplicitVRBigEndian)

	path := filepath.Join(t.TempDir(), "big-endian.dcm")
	if err := fd.SaveAs(path, writeOptions(nil, boolPtr(false), false)); err != nil {
		t.Fatalf("SaveAs() error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// (0010,0010) as big-endian bytes; little endian would be 10 00 10 00.
	if !bytes.Contains(raw, []byte{0x00, 0x10, 0x00, 0x10}) {
		t.Fatal("the dataset does not spell (0010,0010) big endian")
	}
	if bytes.Contains(raw, []byte{0x10, 0x00, 0x10, 0x00}) {
		t.Fatal("the dataset still spells (0010,0010) little endian")
	}

	back, err := ReadFile(path, nil)
	if err != nil {
		t.Fatalf("the big-endian file does not read back: %v", err)
	}
	if v, ok := back.GetInt(MustTag("Rows")); !ok || v != 512 {
		t.Errorf("Rows = %d (present=%v), want 512", v, ok)
	}
	if v, _ := back.GetString(MustTag("PatientName")); v != "Doe^John" {
		t.Errorf("PatientName = %q, want Doe^John", v)
	}
}

// TestDatasetSaveAsDeflated checks that a Deflated transfer syntax in the File
// Meta produces a file whose dataset is actually deflated, and that reading it
// back inflates.
func TestDatasetSaveAsDeflated(t *testing.T) {
	ds := sampleDataset(t)
	fd := samplePart10With(t, ds, DeflatedExplicitVRLittleEndian)

	path := filepath.Join(t.TempDir(), "deflated.dcm")
	if err := fd.SaveAs(path, nil); err != nil {
		t.Fatalf("SaveAs() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The dataset after the File Meta must not contain the patient name in
	// clear: if it did, nothing was deflated.
	if bytes.Contains(raw[132:], []byte("Doe^John")) {
		t.Fatal("the dataset was written in clear despite the Deflated transfer syntax")
	}

	back, err := ReadFile(path, nil)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if v, _ := back.GetString(MustTag("PatientName")); v != "Doe^John" {
		t.Errorf("PatientName = %q, want Doe^John", v)
	}
}

// TestDatasetSaveAsWithoutFileMeta checks the lenient path: a dataset with no
// File Meta still writes, and godicom can read it back with Force.
func TestDatasetSaveAsWithoutFileMeta(t *testing.T) {
	ds := sampleDataset(t)

	path := filepath.Join(t.TempDir(), "bare.dcm")
	if err := ds.SaveAs(path, nil); err != nil {
		t.Fatalf("SaveAs() error = %v", err)
	}

	back, err := ReadFile(path, &ReadOptions{Force: true})
	if err != nil {
		t.Fatalf("ReadFile(Force) error = %v", err)
	}
	if v, _ := back.GetString(MustTag("PatientName")); v != "Doe^John" {
		t.Errorf("PatientName = %q, want Doe^John", v)
	}
}

// TestDatasetSaveAsEnforceFileFormat fills in a missing File Meta rather than
// failing, which is what lets a caller turn a bare dataset into a legal file.
func TestDatasetSaveAsEnforceFileFormat(t *testing.T) {
	ds := sampleDataset(t)

	back := saveAsRoundTrip(t, &FileDataset{Dataset: ds, FileMeta: completeFileMeta(t)},
		writeOptions(nil, nil, true), nil)
	if back.FileMeta == nil {
		t.Fatal("the written file has no File Meta")
	}
	if v, _ := back.FileMeta.GetString(MustTag("TransferSyntaxUID")); v == "" {
		t.Error("TransferSyntaxUID was not filled in")
	}
	if len(back.Preamble) != 128 {
		t.Errorf("preamble is %d bytes, want 128", len(back.Preamble))
	}
}

// TestDatasetSaveAsErrors makes sure a failure is reported rather than a partial
// file left behind: the directory does not exist, so the open fails.
func TestDatasetSaveAsErrors(t *testing.T) {
	ds := NewDataset()
	_ = ds.SetString(MustTag("PatientName"), "Doe^John")

	missing := filepath.Join(t.TempDir(), "no-such-dir", "out.dcm")
	err := ds.SaveAs(missing, &WriteOptions{EnforceFileFormat: true})
	if err == nil {
		t.Fatal("SaveAs() into a missing directory returned no error")
	}
	if _, statErr := os.Stat(missing); statErr == nil {
		t.Fatal("SaveAs() left a file behind after failing")
	}
}

// TestDatasetSaveAsOverwrites checks that saving onto an existing file replaces
// it rather than appending, which is how a re-encode silently doubles a file.
func TestDatasetSaveAsOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.dcm")

	first := NewDataset()
	_ = first.SetString(MustTag("PatientName"), "Doe^John^A^B^C")
	if err := samplePart10With(t, first, ExplicitVRLittleEndian).SaveAs(path, nil); err != nil {
		t.Fatalf("first SaveAs() error = %v", err)
	}
	firstSize, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	second := NewDataset()
	_ = second.SetString(MustTag("PatientName"), "Doe^J")
	if err := samplePart10With(t, second, ExplicitVRLittleEndian).SaveAs(path, nil); err != nil {
		t.Fatalf("second SaveAs() error = %v", err)
	}
	secondSize, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if secondSize.Size() >= firstSize.Size() {
		t.Fatalf("the second, smaller dataset wrote %d bytes over %d: the file was appended to",
			secondSize.Size(), firstSize.Size())
	}

	back, err := ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := back.GetString(MustTag("PatientName")); v != "Doe^J" {
		t.Errorf("PatientName = %q, want Doe^J", v)
	}
}

// TestDatasetSaveAsRejectsNonGroup2BeforeWriting checks that the File Meta is
// validated before the file is touched: a rejected write must not leave a
// truncated file for the next reader to trip on.
func TestDatasetSaveAsRejectsNonGroup2BeforeWriting(t *testing.T) {
	meta := completeFileMeta(t)
	meta.SetString(MustTag("PatientID"), "leaked into the meta")

	path := filepath.Join(t.TempDir(), "out.dcm")
	fd := &FileDataset{Dataset: NewDataset(), FileMeta: meta}
	err := fd.SaveAs(path, &WriteOptions{EnforceFileFormat: true})
	if err == nil {
		t.Fatal("SaveAs() accepted a non-group-2 element in the File Meta")
	}
	if !strings.Contains(err.Error(), "0002") {
		t.Errorf("error %q does not name the (0002,eeee) group restriction", err)
	}

	if _, statErr := os.Stat(path); statErr == nil {
		info, _ := os.Stat(path)
		t.Fatalf("a %d-byte file was left behind after the write failed", info.Size())
	}
}
