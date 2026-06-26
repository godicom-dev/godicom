package godicom

import (
	"testing"
)

func TestDataElementCreation(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test^Name")
	if elem.Tag != MustTag(0x00100010) {
		t.Error("wrong tag")
	}
	if elem.VR != VRPN {
		t.Error("wrong VR")
	}
	if elem.Value != "Test^Name" {
		t.Error("wrong value")
	}
}

func TestDataElementVM(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test")
	if elem.VM() != 1 {
		t.Errorf("VM = %d, want 1", elem.VM())
	}
	elem2 := NewDataElement(MustTag(0x00280010), VRUS, 512)
	if elem2.VM() != 1 {
		t.Errorf("VM = %d, want 1", elem2.VM())
	}
}

func TestDataElementEmpty(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "")
	if !elem.IsEmpty() {
		t.Error("empty PN should be empty")
	}
	elem2 := NewDataElement(MustTag(0x00280010), VRUS, nil)
	if !elem2.IsEmpty() {
		t.Error("nil value should be empty")
	}
}

func TestDataElementName(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test")
	if elem.Name() != "Patient's Name" {
		t.Errorf("Name = %q, want Patient's Name", elem.Name())
	}
}

func TestDataElementKeyword(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test")
	if elem.Keyword() != "PatientName" {
		t.Errorf("Keyword = %q, want PatientName", elem.Keyword())
	}
}

func TestDataElementPrivate(t *testing.T) {
	elem := NewDataElement(MustTag(0x00090010), VRLO, "Private")
	if !elem.IsPrivate() {
		t.Error("private tag should be private")
	}
}

func TestPersonName(t *testing.T) {
	pn := ParsePersonName("Smith^John")
	if pn.Alphabetic != "Smith^John" {
		t.Errorf("Alphabetic = %q", pn.Alphabetic)
	}
	pn2 := ParsePersonName("Smith^John=Doe^Jane")
	if pn2.Alphabetic != "Smith^John" {
		t.Errorf("Alphabetic = %q", pn2.Alphabetic)
	}
	if pn2.Ideographic != "Doe^Jane" {
		t.Errorf("Ideographic = %q", pn2.Ideographic)
	}
}
