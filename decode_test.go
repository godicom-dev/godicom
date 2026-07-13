package godicom

import (
	"bytes"
	"testing"
)

func TestDecodeDatasetRoundtripImplicit(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "Tube^HeNe"))
	ds.Set(NewDataElement(MustTag("PatientID"), VRLO, "Test1101"))

	encoded, err := EncodeDataset(ds, string(ImplicitVRLittleEndian))
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeDataset(encoded, string(ImplicitVRLittleEndian))
	if err != nil {
		t.Fatal(err)
	}
	name, ok := got.GetString(MustTag("PatientName"))
	if !ok || name != "Tube^HeNe" {
		t.Fatalf("PatientName=%q ok=%v", name, ok)
	}
	id, ok := got.GetString(MustTag("PatientID"))
	if !ok || id != "Test1101" {
		t.Fatalf("PatientID=%q ok=%v", id, ok)
	}
}

func TestDecodeDatasetRoundtripExplicit(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, "1.2.840.10008.5.1.4.1.1.7"))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, "1.2.3.4.5"))

	encoded, err := ds.Encode(string(ExplicitVRLittleEndian))
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeDatasetEncoding(encoded, false, true)
	if err != nil {
		t.Fatal(err)
	}
	sop, _ := got.GetString(MustTag("SOPClassUID"))
	if sop != "1.2.840.10008.5.1.4.1.1.7" {
		t.Fatalf("SOPClassUID=%q", sop)
	}
}

func TestDecodeDatasetMatchesPynetdicomFindIdentifier(t *testing.T) {
	// c_find_rq_ds without MCH: QueryRetrieveLevel=PATIENT, PatientID=*
	raw := []byte{
		0x08, 0x00, 0x52, 0x00, 0x08, 0x00, 0x00, 0x00, 'P', 'A', 'T', 'I', 'E', 'N', 'T', ' ',
		0x10, 0x00, 0x20, 0x00, 0x02, 0x00, 0x00, 0x00, '*', ' ',
	}
	ds, err := DecodeDatasetEncoding(raw, true, true)
	if err != nil {
		t.Fatal(err)
	}
	level, _ := ds.GetString(MustTag("QueryRetrieveLevel"))
	if level != "PATIENT" {
		t.Fatalf("QueryRetrieveLevel=%q", level)
	}
	pid, _ := ds.GetString(MustTag("PatientID"))
	if pid != "*" {
		t.Fatalf("PatientID=%q", pid)
	}

	// roundtrip encode
	encoded, err := EncodeDatasetEncoding(ds, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, raw) {
		t.Fatalf("roundtrip\ngot  %x\nwant %x", encoded, raw)
	}
}

func TestDecodeDatasetEmpty(t *testing.T) {
	ds, err := DecodeDatasetEncoding(nil, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Len() != 0 {
		t.Fatalf("len=%d", ds.Len())
	}
}
