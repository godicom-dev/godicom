package godicom

import (
	"path/filepath"
	"testing"
)

// pydicom.tests.test_filewriter.TestWriteFile — byte-identical roundtrip subset.
func TestWriteFileBytesIdenticalExtended(t *testing.T) {
	files := []string{
		"meta_missing_tsyntax.dcm", // test_write_no_ts
		"nested_priv_SQ.dcm",
		"reportsi.dcm",
		"liver_1frame.dcm",
		"waveform_ecg.dcm",
		"examples_palette.dcm",
		"MR_small_implicit.dcm",
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			assertWriteBytesIdentical(t, file)
		})
	}
}

// pydicom.tests.test_filewriter.TestWriteFile.test_None_parent — nested undefined-length SQ.
func TestWriteUndefinedLengthSequenceRoundtrip(t *testing.T) {
	inner := NewDataset()
	inner.Set(NewDataElement(MustTag(0x00409211), VRUS, 0x21FB))
	inner.IsUndefinedLengthSequenceItem = true

	seq := NewSequence([]*Dataset{inner})
	seq.IsUndefinedLength = true

	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00409096), VRSQ, seq))
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.30")))

	outPath := filepath.Join(t.TempDir(), "undef_sq.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	rseq, ok := reread.GetSequence(MustTag(0x00409096))
	if !ok || !rseq.IsUndefinedLength || rseq.Len() != 1 {
		t.Fatalf("sequence = %#v, ok=%t", rseq, ok)
	}
	item := rseq.Get(0)
	if !item.IsUndefinedLengthSequenceItem {
		t.Fatal("expected undefined-length sequence item")
	}
	v, ok := item.GetInt(MustTag(0x00409211))
	if !ok || v != 0x21FB {
		t.Fatalf("RealWorldValueLastValueMapped = %d, want %x", v, 0x21FB)
	}
}

// pydicom.tests.test_filereader.TestReader.test_nested_private_SQ
func TestReadNestedPrivateSQ(t *testing.T) {
	ds, err := ReadFile(testFilePath("nested_priv_SQ.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ds.Has(MustTag(0x7FE00010)) {
		t.Fatal("PixelData missing; dataset not fully read")
	}

	seq0, ok := ds.GetSequence(MustTag(0x00010001))
	if !ok || seq0.Len() == 0 {
		t.Fatal("private (0001,0001) sequence missing")
	}
	seq1, ok := seq0.Get(0).GetSequence(MustTag(0x00010001))
	if !ok || seq1.Len() == 0 {
		t.Fatal("nested private sequence missing")
	}
	val, ok := seq1.Get(0).GetBytes(MustTag(0x00010001))
	if !ok || string(val) != "Double Nested SQ" {
		t.Fatalf("(0001,0001) in nested item = %q, want %q", val, "Double Nested SQ")
	}
	nested, ok := seq0.Get(0).GetBytes(MustTag(0x00010002))
	if !ok || string(nested) != "Nested SQ" {
		t.Fatalf("(0001,0002) = %q, want %q", nested, "Nested SQ")
	}
}
