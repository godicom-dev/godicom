package godicom

import (
	"path/filepath"
	"testing"
)

func TestPersonNameLastFirst(t *testing.T) {
	// pydicom.tests.test_valuerep.TestPersonName.test_last_first
	value := "Family^Given"
	pn := ParsePersonName(value)
	if pn.Alphabetic != value {
		t.Fatalf("Alphabetic = %q, want %q", pn.Alphabetic, value)
	}
	if pn.FamilyName() != "Family" {
		t.Fatalf("FamilyName = %q, want Family", pn.FamilyName())
	}
	if pn.GivenName() != "Given" {
		t.Fatalf("GivenName = %q, want Given", pn.GivenName())
	}
	if pn.NameSuffix() != "" {
		t.Fatalf("NameSuffix = %q, want empty", pn.NameSuffix())
	}
	if pn.Phonetic != "" {
		t.Fatalf("Phonetic = %q, want empty", pn.Phonetic)
	}
}

func TestPersonNameNoComponents(t *testing.T) {
	// pydicom.tests.test_valuerep.TestPersonName.test_no_components
	pn := ParsePersonName("")
	if pn.Alphabetic != "" || pn.Ideographic != "" || pn.Phonetic != "" {
		t.Fatalf("empty PN = %#v, want all empty", pn)
	}
	if !pn.IsZero() {
		t.Fatal("empty PN should be zero")
	}
}

func TestPersonNameThreeComponent(t *testing.T) {
	// pydicom.tests.test_valuerep.TestPersonName.test_three_component
	pn := ParsePersonName(
		"Hong^Gildong^Andrews=" +
			"\033$)C\373\363^\033$)C\321\316\324\327=" +
			"\033$)C\310\253^\033$)C\261\346\265\277",
	)
	if pn.FamilyName() != "Hong" || pn.GivenName() != "Gildong" || pn.MiddleName() != "Andrews" {
		t.Fatalf("names = %q %q %q", pn.FamilyName(), pn.GivenName(), pn.MiddleName())
	}
	if pn.Alphabetic != "Hong^Gildong^Andrews" {
		t.Fatalf("Alphabetic = %q", pn.Alphabetic)
	}
	if pn.Ideographic != "\033$)C\373\363^\033$)C\321\316\324\327" {
		t.Fatalf("Ideographic mismatch")
	}
	if pn.Phonetic != "\033$)C\310\253^\033$)C\261\346\265\277" {
		t.Fatalf("Phonetic mismatch")
	}
}

func TestPersonNameFormatting(t *testing.T) {
	// pydicom.tests.test_valuerep.TestPersonName.test_formatting
	pn := ParsePersonName("Family^Given")
	if pn.FamilyCommaGiven() != "Family, Given" {
		t.Fatalf("FamilyCommaGiven = %q", pn.FamilyCommaGiven())
	}
	if got := pn.Formatted("%(family_name)s, %(given_name)s"); got != "Family, Given" {
		t.Fatalf("Formatted = %q", got)
	}
}

func TestPersonNameNotEqual(t *testing.T) {
	// pydicom.tests.test_valuerep.TestPersonName.test_not_equal
	a := ParsePersonName("Smith^John")
	b := ParsePersonName("Smith^Jane")
	if a.Alphabetic == b.Alphabetic {
		t.Fatal("expected different given names")
	}
}

func TestPersonNameComponentsLength(t *testing.T) {
	// pydicom.tests.test_valuerep.TestPersonName.test_length
	pn := ParsePersonName("A^B=C^D=E^F")
	if len(pn.Components()) != 3 {
		t.Fatalf("components = %d, want 3", len(pn.Components()))
	}
}

func TestFromNamedComponents(t *testing.T) {
	// pydicom.tests.test_valuerep.TestPersonName.test_from_named_components
	pn := FromNamedComponents("Family", "Given", "", "", "")
	if pn.Alphabetic != "Family^Given" {
		t.Fatalf("Alphabetic = %q", pn.Alphabetic)
	}
	pn = FromNamedComponents("Family", "Given", "Middle", "Prefix", "Suffix")
	if pn.Alphabetic != "Family^Given^Middle^Prefix^Suffix" {
		t.Fatalf("Alphabetic = %q", pn.Alphabetic)
	}
}

func TestPersonNameOriginalRoundtripWrite(t *testing.T) {
	pn := ParsePersonName("Family^Given")
	elem := NewDataElement(MustTag("PatientName"), VRPN, pn)
	out := encodeElementExplicitLittle(elem)
	if !bytesHasSuffix(out, []byte("Family^Given")) {
		t.Fatalf("encoded = % X", out)
	}
}

func TestGetPNFromDataset(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, ParsePersonName("Smith^John")))
	pn, ok := ds.GetPN(MustTag("PatientName"))
	if !ok {
		t.Fatal("GetPN failed")
	}
	if pn.FamilyName() != "Smith" || pn.GivenName() != "John" {
		t.Fatalf("got %q %q", pn.FamilyName(), pn.GivenName())
	}
}

func TestPersonNameReadWriteRoundtrip(t *testing.T) {
	ds, err := ReadFile(testFilePath("MR_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	name, ok := ds.GetString(MustTag("PatientName"))
	if !ok || name == "" {
		t.Fatal("PatientName missing")
	}
	outPath := filepath.Join(t.TempDir(), "pn.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}
	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	pn, ok := reread.GetPN(MustTag("PatientName"))
	if !ok {
		t.Fatal("GetPN on reread failed")
	}
	if pn.String() != name {
		t.Fatalf("PN = %q, want %q", pn.String(), name)
	}
}
