package godicom

import (
	"bytes"
	"testing"
)

// The setters resolve the VR from the data dictionary, so a caller never
// repeats it. Verify the resolved VR rather than trusting the round-trip.
func TestSetters_ResolveDictionaryVR(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	if err := ds.SetString(MustTag("PatientID"), "ABC123"); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetInt(MustTag("Rows"), 64); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetFloat(MustTag("WindowCenter"), 40); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetBytes(MustTag("ICCProfile"), []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	want := map[Tag]VR{
		MustTag("PatientID"):    VRLO,
		MustTag("Rows"):         VRUS,
		MustTag("WindowCenter"): VRDS,
		MustTag("ICCProfile"):   VROB,
	}
	for tag, vr := range want {
		elem, ok := ds.Get(tag)
		if !ok {
			t.Fatalf("%s missing after set", tag)
		}
		if elem.VR != vr {
			t.Errorf("%s VR = %s, want %s", tag, elem.VR, vr)
		}
	}
}

func TestSetters_RoundTripThroughEncode(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	if err := ds.SetPN(MustTag("PatientName"), ParsePersonName("Doe^Jane")); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetString(MustTag("PatientID"), "ABC123"); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetInt(MustTag("Rows"), 64); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetFloat(MustTag("WindowCenter"), 40); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetBytes(MustTag("ICCProfile"), []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	da, err := ParseDA("20260819")
	if err != nil {
		t.Fatal(err)
	}
	if err := ds.SetDA(MustTag("StudyDate"), da); err != nil {
		t.Fatal(err)
	}
	tm, err := ParseTM("101530")
	if err != nil {
		t.Fatal(err)
	}
	if err := ds.SetTM(MustTag("StudyTime"), tm); err != nil {
		t.Fatal(err)
	}
	dt, err := ParseDT("20260819101530")
	if err != nil {
		t.Fatal(err)
	}
	if err := ds.SetDT(MustTag("AcquisitionDateTime"), dt); err != nil {
		t.Fatal(err)
	}
	is, err := ParseIS("7")
	if err != nil {
		t.Fatal(err)
	}
	if err := ds.SetIS(MustTag("InstanceNumber"), is); err != nil {
		t.Fatal(err)
	}

	encoded, err := ds.Encode(ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeDataset(encoded, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}

	if pn, ok := got.GetString(MustTag("PatientName")); !ok || pn != "Doe^Jane" {
		t.Errorf("PatientName = %q, %v", pn, ok)
	}
	if id, ok := got.GetString(MustTag("PatientID")); !ok || id != "ABC123" {
		t.Errorf("PatientID = %q, %v", id, ok)
	}
	if rows, ok := got.GetInt(MustTag("Rows")); !ok || rows != 64 {
		t.Errorf("Rows = %d, %v", rows, ok)
	}
	if wc, ok := got.GetFloat(MustTag("WindowCenter")); !ok || wc != 40 {
		t.Errorf("WindowCenter = %v, %v", wc, ok)
	}
	if icc, ok := got.GetBytes(MustTag("ICCProfile")); !ok || !bytes.Equal(icc, []byte{1, 2, 3, 4}) {
		t.Errorf("ICCProfile = %v, %v", icc, ok)
	}
	if v, ok := got.GetDA(MustTag("StudyDate")); !ok || v.String() != "20260819" {
		t.Errorf("StudyDate = %q, %v", v.String(), ok)
	}
	if v, ok := got.GetTM(MustTag("StudyTime")); !ok || v.String() != "101530" {
		t.Errorf("StudyTime = %q, %v", v.String(), ok)
	}
	if v, ok := got.GetDT(MustTag("AcquisitionDateTime")); !ok || v.String() != "20260819101530" {
		t.Errorf("AcquisitionDateTime = %q, %v", v.String(), ok)
	}
	if v, ok := got.GetInt(MustTag("InstanceNumber")); !ok || v != 7 {
		t.Errorf("InstanceNumber = %d, %v", v, ok)
	}
}

// The plural setters must store a value shape every encoder understands:
// a plain []float64 or []int would encode as an empty value for DS/FD/AT.
func TestSetters_MultiValueEncodesAllComponents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		set  func(*Dataset) error
		want []byte
	}{{
		name: "CS strings",
		set:  func(d *Dataset) error { return d.SetStrings(MustTag("ImageType"), "ORIGINAL", "PRIMARY") },
		want: []byte("ORIGINAL\\PRIMARY"),
	}, {
		name: "DS floats",
		set:  func(d *Dataset) error { return d.SetFloats(MustTag("PixelSpacing"), 0.5, 0.25) },
		want: []byte("0.5\\0.25"),
	}, {
		name: "FD floats",
		set: func(d *Dataset) error {
			return d.SetFloats(MustTag("DiffusionGradientOrientation"), 1, 0, -1)
		},
		want: []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0xbf,
		},
	}, {
		name: "AT tags",
		set: func(d *Dataset) error {
			return d.SetInts(MustTag("FrameIncrementPointer"), 0x00181063, 0x00181065)
		},
		want: []byte{0x18, 0x00, 0x63, 0x10, 0x18, 0x00, 0x65, 0x10},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ds := NewDataset()
			if err := tc.set(ds); err != nil {
				t.Fatal(err)
			}
			encoded, err := ds.Encode(ExplicitVRLittleEndian)
			if err != nil {
				t.Fatal(err)
			}
			// Explicit VR, 16-bit length: 4 tag + 2 VR + 2 length.
			if len(encoded) < 8 {
				t.Fatalf("encoded too short: %x", encoded)
			}
			if got := encoded[8:]; !bytes.Equal(got, tc.want) {
				t.Fatalf("value bytes = %x, want %x", got, tc.want)
			}
		})
	}
}

func TestSetInts_RoundTripUS(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	// Rows is VM 1, but the US encoder path is what matters here.
	if err := ds.SetInts(MustTag("Rows"), 64, 128); err != nil {
		t.Fatal(err)
	}
	encoded, err := ds.Encode(ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x40, 0x00, 0x80, 0x00}
	if got := encoded[8:]; !bytes.Equal(got, want) {
		t.Fatalf("US value bytes = %x, want %x", got, want)
	}
}

func TestSetSequence_RoundTrip(t *testing.T) {
	t.Parallel()
	item := NewDataset()
	if err := item.SetString(MustTag("ReferencedSOPInstanceUID"), "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	ds := NewDataset()
	if err := ds.SetSequence(MustTag("ReferencedImageSequence"), NewSequence([]*Dataset{item})); err != nil {
		t.Fatal(err)
	}
	encoded, err := ds.Encode(ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeDataset(encoded, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	seq, ok := got.GetSequence(MustTag("ReferencedImageSequence"))
	if !ok || seq == nil || seq.Len() != 1 {
		t.Fatalf("sequence = %v, %v", seq, ok)
	}
	if uid, ok := seq.Get(0).GetString(MustTag("ReferencedSOPInstanceUID")); !ok || uid != "1.2.3.4" {
		t.Fatalf("item UID = %q, %v", uid, ok)
	}
}

func TestSetValue_SkipsKindCheck(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	// SetInt would reject a PN tag; SetValue trusts the caller.
	if err := ds.SetValue(MustTag("PatientName"), 42); err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(MustTag("PatientName"))
	if !ok || elem.VR != VRPN {
		t.Fatalf("PatientName VR = %v, %v", elem, ok)
	}
	if err := ds.SetValue(NewTag(0x0009, 0x0010), "x"); err == nil {
		t.Fatal("expected error for a tag outside the dictionary")
	}
}

func TestSetters_RejectTagOutsideDictionary(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	private := NewTag(0x0009, 0x1001)
	if err := ds.SetString(private, "x"); err == nil {
		t.Fatal("expected error for a private tag with no dictionary VR")
	}
	if ds.Has(private) {
		t.Fatal("failed set must not store the element")
	}
}

func TestSetters_RejectKindMismatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		set  func(*Dataset) error
	}{
		{"int into PN", func(d *Dataset) error { return d.SetInt(MustTag("PatientName"), 1) }},
		{"float into LO", func(d *Dataset) error { return d.SetFloat(MustTag("PatientID"), 1) }},
		{"string into US", func(d *Dataset) error { return d.SetString(MustTag("Rows"), "64") }},
		{"bytes into PN", func(d *Dataset) error { return d.SetBytes(MustTag("PatientName"), nil) }},
		{"sequence into LO", func(d *Dataset) error {
			return d.SetSequence(MustTag("PatientID"), NewSequence(nil))
		}},
		{"DA into TM", func(d *Dataset) error { return d.SetDA(MustTag("StudyTime"), DA{}) }},
		{"PN into LO", func(d *Dataset) error { return d.SetPN(MustTag("PatientID"), PersonName{}) }},
		{"IS into DS", func(d *Dataset) error { return d.SetIS(MustTag("WindowCenter"), IS{}) }},
		{"floats into CS", func(d *Dataset) error { return d.SetFloats(MustTag("ImageType"), 1) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ds := NewDataset()
			if err := tc.set(ds); err == nil {
				t.Fatal("expected a VR/kind mismatch error")
			}
			if len(ds.Elements()) != 0 {
				t.Fatal("rejected set must not store the element")
			}
		})
	}
}

// DS and IS are both string and numeric VRs, so either setter is admissible.
func TestSetters_NumberStringVRsAcceptBothKinds(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	if err := ds.SetString(MustTag("WindowCenter"), "40.5"); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetString(MustTag("InstanceNumber"), "7"); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetFloat(MustTag("WindowCenter"), 40.5); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetInt(MustTag("InstanceNumber"), 7); err != nil {
		t.Fatal(err)
	}
	encoded, err := ds.Encode(ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeDataset(encoded, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := got.GetFloat(MustTag("WindowCenter")); !ok || v != 40.5 {
		t.Errorf("WindowCenter = %v, %v", v, ok)
	}
	if v, ok := got.GetInt(MustTag("InstanceNumber")); !ok || v != 7 {
		t.Errorf("InstanceNumber = %d, %v", v, ok)
	}
}
