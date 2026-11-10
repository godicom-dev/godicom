package godicom

import (
	"testing"

	"github.com/godicom-dev/godicom/tag"
)

// The typed getters share one shape: a value the parser stored as that type, a
// value stored as its source string, and everything else refused. These tests
// walk all three so a change to one getter does not silently change the rest.

func TestGetStringFromEveryStoredType(t *testing.T) {
	ds := NewDataset()
	cases := []struct {
		name  string
		tg    Tag
		vr    VR
		value interface{}
		want  string
	}{
		{"plain string", tag.PatientID, VRLO, "ID1", "ID1"},
		{"person name", tag.PatientName, VRPN, PersonName{Alphabetic: "CITIZEN^Jan"}, "CITIZEN^Jan"},
		{"date", tag.StudyDate, VRDA, DA{Original: "20240102"}, "20240102"},
		{"time", tag.StudyTime, VRTM, TM{Original: "120000"}, "120000"},
		{"datetime", tag.AcquisitionDateTime, VRDT, DT{Original: "20240102120000"}, "20240102120000"},
		{"decimal", tag.PatientWeight, VRDS, DS{Value: 1.5, Original: "1.5"}, "1.5"},
		{"integer", tag.NumberOfFrames, VRIS, IS{Value: 3, Original: "3"}, "3"},
		{"uid", tag.SOPInstanceUID, VRUI, UID("1.2.3"), "1.2.3"},
		{"fallback", tag.Rows, VRUS, 512, "512"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds.Set(NewDataElement(tc.tg, tc.vr, tc.value))
			got, ok := ds.GetString(tc.tg)
			if !ok || got != tc.want {
				t.Fatalf("got %q %v, want %q", got, ok, tc.want)
			}
		})
	}
	ds.Set(NewDataElement(tag.PatientID, VRLO, nil))
	if _, ok := ds.GetString(tag.PatientID); ok {
		t.Fatal("an element with no value should not report a string")
	}
}

func TestGetTMAndDTFromValuesAndStrings(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(tag.StudyTime, VRTM, TM{Original: "120000"}))
	if v, ok := ds.GetTM(tag.StudyTime); !ok || v.Original != "120000" {
		t.Fatalf("TM from TM = %+v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.StudyTime, VRTM, "130000"))
	if v, ok := ds.GetTM(tag.StudyTime); !ok || v.Original != "130000" {
		t.Fatalf("TM from string = %+v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.StudyTime, VRTM, "bad"))
	if _, ok := ds.GetTM(tag.StudyTime); ok {
		t.Fatal("an unparsable TM string should be refused")
	}
	ds.Set(NewDataElement(tag.StudyTime, VRTM, 5))
	if _, ok := ds.GetTM(tag.StudyTime); ok {
		t.Fatal("an integer is not a TM")
	}

	ds.Set(NewDataElement(tag.AcquisitionDateTime, VRDT, DT{Original: "20240102120000"}))
	if v, ok := ds.GetDT(tag.AcquisitionDateTime); !ok || v.Original != "20240102120000" {
		t.Fatalf("DT from DT = %+v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.AcquisitionDateTime, VRDT, "20240102130000"))
	if v, ok := ds.GetDT(tag.AcquisitionDateTime); !ok || v.Original != "20240102130000" {
		t.Fatalf("DT from string = %+v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.AcquisitionDateTime, VRDT, "bad"))
	if _, ok := ds.GetDT(tag.AcquisitionDateTime); ok {
		t.Fatal("an unparsable DT string should be refused")
	}
	ds.Set(NewDataElement(tag.AcquisitionDateTime, VRDT, 5))
	if _, ok := ds.GetDT(tag.AcquisitionDateTime); ok {
		t.Fatal("an integer is not a DT")
	}
}

func TestGetPNFromValueAndString(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(tag.PatientName, VRPN, PersonName{Alphabetic: "A^B"}))
	if v, ok := ds.GetPN(tag.PatientName); !ok || v.Alphabetic != "A^B" {
		t.Fatalf("PN from PN = %+v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.PatientName, VRPN, "C^D=E^F"))
	pn, ok := ds.GetPN(tag.PatientName)
	if !ok || pn.Alphabetic != "C^D" || pn.Ideographic != "E^F" {
		t.Fatalf("PN from string = %+v %v", pn, ok)
	}
	ds.Set(NewDataElement(tag.PatientName, VRPN, 5))
	if _, ok := ds.GetPN(tag.PatientName); ok {
		t.Fatal("an integer is not a PN")
	}
}

func TestGetISFromEverySpelling(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(tag.NumberOfFrames, VRIS, IS{Value: 3, Original: "03"}))
	if v, ok := ds.GetIS(tag.NumberOfFrames); !ok || v.Original != "03" {
		t.Fatalf("IS from IS = %+v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.NumberOfFrames, VRIS, 4))
	if v, ok := ds.GetIS(tag.NumberOfFrames); !ok || v.Value != 4 || v.String() != "4" {
		t.Fatalf("IS from int = %+v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.NumberOfFrames, VRIS, int64(5)))
	if v, ok := ds.GetIS(tag.NumberOfFrames); !ok || v.Value != 5 {
		t.Fatalf("IS from int64 = %+v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.NumberOfFrames, VRIS, "6"))
	if v, ok := ds.GetIS(tag.NumberOfFrames); !ok || v.Value != 6 {
		t.Fatalf("IS from string = %+v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.NumberOfFrames, VRIS, "6.5"))
	if _, ok := ds.GetIS(tag.NumberOfFrames); ok {
		t.Fatal("a fractional string is not an IS")
	}
	ds.Set(NewDataElement(tag.NumberOfFrames, VRIS, 5.0))
	if _, ok := ds.GetIS(tag.NumberOfFrames); ok {
		t.Fatal("a float is not an IS")
	}
}

func TestGetFloatsFromFloat32(t *testing.T) {
	// dicomjson stores FL values as float32 and FL multiples as
	// MultiValue[float32]; both have to reach the float64 accessor or a parsed
	// JSON dataset reads as if it had no values.
	ds := NewDataset()
	ds.Set(NewDataElement(tag.PatientWeight, VRFL, float32(1.5)))
	if v, ok := ds.GetFloat(tag.PatientWeight); !ok || v != 1.5 {
		t.Fatalf("float32 = %v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.PatientWeight, VRFL, NewMultiValue([]float32{1.5, 2.5})))
	vals, ok := ds.GetFloats(tag.PatientWeight)
	if !ok || len(vals) != 2 || vals[1] != 2.5 {
		t.Fatalf("MultiValue[float32] = %v %v", vals, ok)
	}
}

func TestGetStringOnMissingAndEmptyElements(t *testing.T) {
	ds := NewDataset()
	if _, ok := ds.GetString(tag.PatientID); ok {
		t.Fatal("missing element reported a value")
	}
	if _, ok := ds.GetDA(tag.StudyDate); ok {
		t.Fatal("missing element reported a DA")
	}
	if _, ok := ds.GetTM(tag.StudyTime); ok {
		t.Fatal("missing element reported a TM")
	}
	if _, ok := ds.GetDT(tag.AcquisitionDateTime); ok {
		t.Fatal("missing element reported a DT")
	}
	if _, ok := ds.GetPN(tag.PatientName); ok {
		t.Fatal("missing element reported a PN")
	}
	if _, ok := ds.GetIS(tag.NumberOfFrames); ok {
		t.Fatal("missing element reported an IS")
	}
	if _, ok := ds.GetFloats(tag.PatientWeight); ok {
		t.Fatal("missing element reported floats")
	}
}
