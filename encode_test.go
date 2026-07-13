package godicom

import (
	"bytes"
	"testing"
)

func TestEncodeDatasetImplicitLittleEndian(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "Tube^HeNe"))
	ds.Set(NewDataElement(MustTag("PatientID"), VRLO, "Test1101"))

	got, err := EncodeDataset(ds, string(ImplicitVRLittleEndian))
	if err != nil {
		t.Fatal(err)
	}
	// Matches pynetdicom encoded_dimse_msg.c_store_ds (without MCH).
	want := []byte{
		0x10, 0x00, 0x10, 0x00, 0x0a, 0x00, 0x00, 0x00, 'T', 'u', 'b', 'e', '^', 'H', 'e', 'N', 'e', ' ',
		0x10, 0x00, 0x20, 0x00, 0x08, 0x00, 0x00, 0x00, 'T', 'e', 's', 't', '1', '1', '0', '1',
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeDataset mismatch\ngot  %x\nwant %x", got, want)
	}

	got2, err := ds.EncodeEncoding(true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got2, want) {
		t.Fatalf("EncodeEncoding mismatch")
	}
}

func TestEncodeDatasetExplicitLittleEndian(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, "1.2.840.10008.5.1.4.1.1.7"))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, "1.2.3.4.5"))

	got, err := ds.Encode(string(ExplicitVRLittleEndian))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 20 {
		t.Fatalf("encoded too short: %d", len(got))
	}
	// Explicit VR: after tag, VR "UI" should appear for SOP Class UID.
	if string(got[4:6]) != "UI" {
		t.Fatalf("expected explicit UI VR, got %q in %x", got[4:6], got[:16])
	}
}

func TestEncodeDatasetRejectsCommandAndFileMeta(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(NewTag(0x0000, 0x0100), VRUS, uint16(0x0030)))
	if _, err := EncodeDataset(ds, string(ImplicitVRLittleEndian)); err == nil {
		t.Fatal("expected error for command set element")
	}

	ds2 := NewDataset()
	ds2.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, ImplicitVRLittleEndian))
	if _, err := EncodeDataset(ds2, string(ImplicitVRLittleEndian)); err == nil {
		t.Fatal("expected error for file meta element in dataset")
	}
}

func TestEncodeDatasetUnknownTransferSyntax(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientID"), VRLO, "X"))
	if _, err := EncodeDataset(ds, "1.2.3.4.5.6.7.8.9"); err == nil {
		t.Fatal("expected error for unknown transfer syntax")
	}
}

func TestEncodeDatasetEncodingInvalidCombo(t *testing.T) {
	ds := NewDataset()
	if _, err := EncodeDatasetEncoding(ds, true, false); err == nil {
		t.Fatal("expected error for implicit VR big endian")
	}
}

func TestWriteDataset(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientID"), VRLO, "ABC"))
	var buf bytes.Buffer
	if err := WriteDataset(&buf, ds, string(ImplicitVRLittleEndian)); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("empty write")
	}
}
