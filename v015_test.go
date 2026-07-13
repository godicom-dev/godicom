package godicom

import (
	"math"
	"testing"
	"time"
)

// pydicom.tests.test_valuerep.TestIsValidDS
func TestIsValidDS(t *testing.T) {
	valid := []string{
		"1",
		"3.14159265358979",
		"-1234.456e78",
		"1.234E-5",
		"1.234E+5",
		"+1",
		"    42",
		"42    ",
	}
	for _, s := range valid {
		if !IsValidDS(s) {
			t.Fatalf("IsValidDS(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"nan",
		"-inf",
		"3.141592653589793", // too long
		"1,000",
		"1 000",
		"127.0.0.1",
		"1.e",
	}
	for _, s := range invalid {
		if IsValidDS(s) {
			t.Fatalf("IsValidDS(%q) = true, want false", s)
		}
	}
}

// pydicom.tests.test_valuerep.TestTruncateFloatForDS.test_auto_format
func TestFormatNumberAsDS(t *testing.T) {
	cases := []struct {
		val  float64
		want string
	}{
		{1.0, "1.0"},
		{0.0, "0.0"},
		{math.Copysign(0, -1), "-0.0"},
		{0.123, "0.123"},
		{-0.321, "-0.321"},
		{0.00001, "1e-05"},
		{3.14159265358979323846, "3.14159265358979"},
		{-3.14159265358979323846, "-3.1415926535898"},
		{5.3859401928763739403e-7, "5.3859401929e-07"},
		{-5.3859401928763739403e-7, "-5.385940193e-07"},
		{1.2342534378125532912998323e10, "12342534378.1255"},
		{6.40708699858767842501238e13, "64070869985876.8"},
		{1.7976931348623157e308, "1.797693135e+308"},
	}
	for _, tt := range cases {
		got, err := FormatNumberAsDS(tt.val)
		if err != nil {
			t.Fatalf("FormatNumberAsDS(%g) error = %v", tt.val, err)
		}
		if got != tt.want {
			t.Fatalf("FormatNumberAsDS(%g) = %q, want %q", tt.val, got, tt.want)
		}
		if !IsValidDS(got) {
			t.Fatalf("FormatNumberAsDS(%g) produced invalid DS %q", tt.val, got)
		}
	}
}

func TestFormatNumberAsDSRejectsNonFinite(t *testing.T) {
	if _, err := FormatNumberAsDS(math.NaN()); err == nil {
		t.Fatal("expected error for NaN")
	}
	if _, err := FormatNumberAsDS(math.Inf(1)); err == nil {
		t.Fatal("expected error for +Inf")
	}
}

func TestFormatNumberAsDSPowersOfPi(t *testing.T) {
	exps := []int{-101, -100, 100, 101}
	for e := -16; e <= 16; e++ {
		exps = append(exps, e)
	}
	for _, exp := range exps {
		val := math.Pi * math.Pow(10, float64(exp))
		s, err := FormatNumberAsDS(val)
		if err != nil {
			t.Fatalf("exp=%d: %v", exp, err)
		}
		if !IsValidDS(s) || stringsHasSuffixDot(s) {
			t.Fatalf("exp=%d: invalid DS %q", exp, s)
		}
	}
}

func stringsHasSuffixDot(s string) bool {
	return len(s) > 0 && s[len(s)-1] == '.'
}

func TestIsValidISAndRange(t *testing.T) {
	if !IsValidIS("42") || !IsValidIS("   -128  ") {
		t.Fatal("expected valid IS")
	}
	if IsValidIS("1234567890123") { // >12 chars
		t.Fatal("expected length rejection")
	}
	if IsValidIS("1.5") {
		t.Fatal("expected non-integer rejection")
	}
	if !ISInRange(0) || !ISInRange(minISValue) || !ISInRange(maxISValue) {
		t.Fatal("range bounds should be included/excluded correctly")
	}
	if ISInRange(int64(maxISValue) + 1) {
		t.Fatal("2^31 should be out of range")
	}
}

func TestDSFromFloat(t *testing.T) {
	ds, err := DSFromFloat(math.Pi)
	if err != nil {
		t.Fatal(err)
	}
	if ds.String() != "3.14159265358979" {
		t.Fatalf("String = %q", ds.String())
	}
	if !ds.Equal(DS{Value: math.Pi, Original: "3.14159265358979"}) {
		t.Fatal("Equal failed")
	}
}

func TestDATEQual(t *testing.T) {
	a, _ := ParseDA("20000101")
	b := DA{Time: time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)}
	if !a.Equal(b) {
		t.Fatal("DA Equal should compare calendar date")
	}
	c, _ := ParseTM("115500")
	d, _ := ParseTM("115500.000000")
	if !c.Equal(d) {
		t.Fatal("TM Equal should ignore Original formatting")
	}
}

// pydicom.tests.test_valuerep.TestPersonName.test_from_named_components
func TestPersonNameFromParts(t *testing.T) {
	pn := PersonNameFromParts(PersonNameParts{
		FamilyName: "Adams",
		GivenName:  "John Robert Quincy",
		NamePrefix: "Rev.",
		NameSuffix: "B.A. M.Div.",
	})
	if pn.String() != "Adams^John Robert Quincy^^Rev.^B.A. M.Div." {
		t.Fatalf("String = %q", pn.String())
	}
	if pn.FamilyName() != "Adams" || pn.GivenName() != "John Robert Quincy" {
		t.Fatalf("components = %q / %q", pn.FamilyName(), pn.GivenName())
	}
	if pn.NamePrefix() != "Rev." || pn.NameSuffix() != "B.A. M.Div." {
		t.Fatalf("prefix/suffix = %q / %q", pn.NamePrefix(), pn.NameSuffix())
	}
}

func TestPersonNameFromPartsMultiGroup(t *testing.T) {
	pn := PersonNameFromParts(PersonNameParts{
		FamilyName:            "Hong",
		GivenName:             "Gildong",
		FamilyNameIdeographic: "洪",
		GivenNameIdeographic:  "吉洞",
		FamilyNamePhonetic:    "홍",
		GivenNamePhonetic:     "길동",
	})
	if pn.Alphabetic != "Hong^Gildong" {
		t.Fatalf("alphabetic = %q", pn.Alphabetic)
	}
	if pn.Ideographic != "洪^吉洞" {
		t.Fatalf("ideographic = %q", pn.Ideographic)
	}
	if pn.Phonetic != "홍^길동" {
		t.Fatalf("phonetic = %q", pn.Phonetic)
	}
}

func TestPersonNameFromVeterinary(t *testing.T) {
	pn := PersonNameFromVeterinary("ABC Farms", "Running on Water")
	if pn.String() != "ABC Farms^Running on Water" {
		t.Fatalf("String = %q", pn.String())
	}
}

func TestPersonNameEqual(t *testing.T) {
	a := ParsePersonName("Family^Given")
	b := FromNamedComponents("Family", "Given", "", "", "")
	if !a.Equal(b) {
		t.Fatal("expected equal person names")
	}
}
