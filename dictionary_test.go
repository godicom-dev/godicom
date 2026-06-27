package godicom

import (
	"testing"
)

func TestDictionaryLookup(t *testing.T) {
	vr, err := dictionaryVR(MustTag(0x00100010))
	if err != nil {
		t.Fatal(err)
	}
	if vr != VRPN {
		t.Errorf("VR for PatientName = %s, want PN", vr)
	}
}

func TestDictionaryDescription(t *testing.T) {
	name, ok := dictionaryDescription(MustTag(0x00100010))
	if !ok || name != "Patient's Name" {
		t.Errorf("got %q", name)
	}
}

func TestDictionaryHasTag(t *testing.T) {
	if !dictionaryHasTag(MustTag(0x00100010)) {
		t.Error("PatientName should be in dictionary")
	}
	if dictionaryHasTag(MustTag(0x00090010)) {
		t.Error("private tag should not be in dictionary")
	}
}

func TestDictionaryIsRetired(t *testing.T) {
	if dictionaryIsRetired(MustTag(0x00100010)) {
		t.Error("PatientName is not retired")
	}
}

func TestTagForKeyword(t *testing.T) {
	tag, ok := tagForKeyword("PatientName")
	if !ok || tag != MustTag(0x00100010) {
		t.Errorf("got %v", tag)
	}
	_, ok = tagForKeyword("NonExistent")
	if ok {
		t.Error("should not find non-existent keyword")
	}
}

func TestKeywordForTag(t *testing.T) {
	kw, ok := keywordForTag(MustTag(0x00100010))
	if !ok || kw != "PatientName" {
		t.Errorf("got %q", kw)
	}
}

func TestLookupVR(t *testing.T) {
	vr := LookupVR(MustTag(0x00100010))
	if vr != VRPN {
		t.Errorf("got %s", vr)
	}
	// Private tag should return UN
	vr = LookupVR(MustTag(0x00090010))
	if vr != VRUN {
		t.Errorf("private tag VR = %s, want UN", vr)
	}
}

func TestRepeaterTag(t *testing.T) {
	// (60xx,3000) is a repeater tag
	tag := MustTag(0x60103000)
	if !IsRepeaterTag(tag) {
		t.Error("should be a repeater tag")
	}
}

func TestPrivateDictionary(t *testing.T) {
	vr, err := PrivateDictionaryVR(MustTag(0x00090000), "ACUSON")
	if err != nil {
		t.Fatal(err)
	}
	if vr != VRIS {
		t.Fatalf("VR = %s, want IS", vr)
	}

	vm, err := PrivateDictionaryVM(MustTag(0x00090000), "ACUSON")
	if err != nil {
		t.Fatal(err)
	}
	if vm != "1" {
		t.Fatalf("VM = %q, want 1", vm)
	}

	name, err := PrivateDictionaryDescription(MustTag(0x00090000), "ACUSON")
	if err != nil {
		t.Fatal(err)
	}
	if name != "Unknown" {
		t.Fatalf("Name = %q, want Unknown", name)
	}
}

func TestAddPrivateDictEntry(t *testing.T) {
	t.Cleanup(ResetExtraPrivateDictionaries)

	tag := MustTag(0x0041, 0x0001)
	if err := AddPrivateDictEntry("ACME 3.2", tag, VRUS, "Some Number"); err != nil {
		t.Fatal(err)
	}

	vr, err := PrivateDictionaryVR(tag, "ACME 3.2")
	if err != nil {
		t.Fatal(err)
	}
	if vr != VRUS {
		t.Fatalf("VR = %s, want US", vr)
	}

	if err := AddPrivateDictEntry("ACME 3.2", MustTag(0x0010, 0x0010), VRDS, "Patient"); err == nil {
		t.Fatal("expected error for non-private tag")
	}
}

func TestPrivateDictLookupElementName(t *testing.T) {
	elem := &Element{
		Tag:            MustTag(0x00090000),
		PrivateCreator: "ACUSON",
	}
	if got := elem.Name(); got != "[Unknown]" {
		t.Fatalf("Name() = %q, want [Unknown]", got)
	}
}

func TestPrivateDictionaryGeneratedSize(t *testing.T) {
	if len(PrivateDictionaries) < 400 {
		t.Fatalf("PrivateDictionaries has only %d creators", len(PrivateDictionaries))
	}
	entries := 0
	for _, inner := range PrivateDictionaries {
		entries += len(inner)
	}
	if entries < 10000 {
		t.Fatalf("PrivateDictionaries has only %d entries", entries)
	}
}
