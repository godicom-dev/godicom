package godicom

import "testing"

// Ported from pydicom tests/test_valuerep.py::TestPersonName. Subtest names keep
// the pydicom spelling so the origin of each assertion stays greppable.

func TestParsePersonName(t *testing.T) {
	// pydicom: TestPersonName::test_components_alphabetic,
	// test_components_ideographic, test_components_phonetic, test_components_none
	tests := []struct {
		name        string
		in          string
		alphabetic  string
		ideographic string
		phonetic    string
	}{
		{"alphabetic_only", "Doe^John", "Doe^John", "", ""},
		{"alphabetic_and_ideographic", "Doe^John=山田^太郎", "Doe^John", "山田^太郎", ""},
		{"all_three_groups", "Doe^John=山田^太郎=やまだ^たろう", "Doe^John", "山田^太郎", "やまだ^たろう"},
		{"trailing_separators_are_dropped", "Doe^John==", "Doe^John", "", ""},
		{"leading_separator_is_an_empty_family", "=山田^太郎", "", "山田^太郎", ""},
		{"trailing_padding_is_trimmed", "Doe^John  ", "Doe^John", "", ""},
		{"empty", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pn := ParsePersonName(tt.in)
			if pn.Alphabetic != tt.alphabetic {
				t.Fatalf("Alphabetic = %q, want %q", pn.Alphabetic, tt.alphabetic)
			}
			if pn.Ideographic != tt.ideographic {
				t.Fatalf("Ideographic = %q, want %q", pn.Ideographic, tt.ideographic)
			}
			if pn.Phonetic != tt.phonetic {
				t.Fatalf("Phonetic = %q, want %q", pn.Phonetic, tt.phonetic)
			}
		})
	}
}

func TestPersonNameString(t *testing.T) {
	// pydicom: TestPersonName::test_str, test_str_empty, test_str_original
	tests := []struct {
		name string
		pn   PersonName
		want string
	}{
		{"test_str_alphabetic", PersonName{Alphabetic: "Doe^John"}, "Doe^John"},
		{"test_str_two_groups", PersonName{Alphabetic: "Doe^John", Ideographic: "山田^太郎"}, "Doe^John=山田^太郎"},
		{"test_str_three_groups", PersonName{Alphabetic: "A", Ideographic: "B", Phonetic: "C"}, "A=B=C"},
		{"test_str_empty", PersonName{}, ""},
		{"test_str_original_wins", PersonName{Alphabetic: "A", Original: "Raw^Value "}, "Raw^Value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pn.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPersonNameComponents(t *testing.T) {
	// pydicom: TestPersonName::test_components_multi, test_components_empty
	tests := []struct {
		name string
		pn   PersonName
		want []string
	}{
		{"one_group", PersonName{Alphabetic: "A"}, []string{"A"}},
		{"skips_empty_middle", PersonName{Alphabetic: "A", Phonetic: "C"}, []string{"A", "C"}},
		{"all_three", PersonName{Alphabetic: "A", Ideographic: "B", Phonetic: "C"}, []string{"A", "B", "C"}},
		{"none", PersonName{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pn.Components()
			if len(got) != len(tt.want) {
				t.Fatalf("Components() = %q, want %q", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("Components()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPersonNameIsZero(t *testing.T) {
	// pydicom: TestPersonName::test_is_empty
	if !(PersonName{}).IsZero() {
		t.Fatal("zero PersonName should be zero")
	}
	if (PersonName{Alphabetic: "A"}).IsZero() {
		t.Fatal("PersonName with a name should not be zero")
	}
	if (PersonName{Original: "A"}).IsZero() {
		t.Fatal("PersonName with Original should not be zero")
	}
}

func TestPersonNameNamedParts(t *testing.T) {
	// pydicom: TestPersonName::test_family_name, test_given_name, test_middle_name,
	// test_name_prefix, test_name_suffix, test_family_comma_given
	pn := ParsePersonName("Doe^John^Robert^Dr^Jr")
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"family_name", pn.FamilyName(), "Doe"},
		{"given_name", pn.GivenName(), "John"},
		{"middle_name", pn.MiddleName(), "Robert"},
		{"name_prefix", pn.NamePrefix(), "Dr"},
		{"name_suffix", pn.NameSuffix(), "Jr"},
		{"family_comma_given", pn.FamilyCommaGiven(), "Doe, John"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestPersonNameMissingPartsAreEmpty(t *testing.T) {
	// pydicom: TestPersonName::test_family_name_absent, test_given_name_absent
	pn := ParsePersonName("Doe")
	for _, tc := range []struct {
		name string
		got  string
	}{
		{"family_name", pn.FamilyName()},
		{"given_name", pn.GivenName()},
		{"middle_name", pn.MiddleName()},
		{"name_prefix", pn.NamePrefix()},
		{"name_suffix", pn.NameSuffix()},
	} {
		if tc.name == "family_name" {
			if tc.got != "Doe" {
				t.Fatalf("FamilyName() = %q, want Doe", tc.got)
			}
			continue
		}
		if tc.got != "" {
			t.Fatalf("%s = %q, want empty", tc.name, tc.got)
		}
	}
}

func TestPersonNameFormatted(t *testing.T) {
	// pydicom: TestPersonName::test_formatted
	pn := ParsePersonName("Doe^John^Robert=山田^太郎=やまだ^たろう")
	tests := []struct {
		format string
		want   string
	}{
		{"%(family_name)s^%(given_name)s", "Doe^John"},
		{"%(family_name)s, %(given_name)s %(middle_name)s", "Doe, John Robert"},
		{"%(name_prefix)s %(family_name)s", " Doe"},
		{"%(ideographic)s", "山田^太郎"},
		{"%(phonetic)s", "やまだ^たろう"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			if got := pn.Formatted(tt.format); got != tt.want {
				t.Fatalf("Formatted(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestFromNamedComponentsTable(t *testing.T) {
	// pydicom: TestPersonName::test_from_named_components
	tests := []struct {
		name                            string
		family, given, middle, pre, suf string
		want                            string
	}{
		{"all_parts", "Doe", "John", "Robert", "Dr", "Jr", "Doe^John^Robert^Dr^Jr"},
		{"trailing_parts_trimmed", "Doe", "John", "", "", "", "Doe^John"},
		{"middle_only_gap_kept", "Doe", "", "", "Dr", "", "Doe^^^Dr"},
		{"only_family", "Doe", "", "", "", "", "Doe"},
		{"all_empty", "", "", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pn := FromNamedComponents(tt.family, tt.given, tt.middle, tt.pre, tt.suf)
			if pn.String() != tt.want {
				t.Fatalf("String() = %q, want %q", pn.String(), tt.want)
			}
		})
	}
}

func TestPersonNameFromPartsThreeGroups(t *testing.T) {
	// pydicom: TestPersonName::test_from_named_components_ideographic,
	// test_from_named_components_phonetic
	pn := PersonNameFromParts(PersonNameParts{
		FamilyName:            "Doe",
		GivenName:             "John",
		FamilyNameIdeographic: "山田",
		GivenNameIdeographic:  "太郎",
		FamilyNamePhonetic:    "やまだ",
		GivenNamePhonetic:     "たろう",
	})
	if pn.Alphabetic != "Doe^John" {
		t.Fatalf("Alphabetic = %q, want Doe^John", pn.Alphabetic)
	}
	if pn.Ideographic != "山田^太郎" {
		t.Fatalf("Ideographic = %q, want 山田^太郎", pn.Ideographic)
	}
	if pn.Phonetic != "やまだ^たろう" {
		t.Fatalf("Phonetic = %q, want やまだ^たろう", pn.Phonetic)
	}
	if got := pn.String(); got != "Doe^John=山田^太郎=やまだ^たろう" {
		t.Fatalf("String() = %q", got)
	}
}

func TestPersonNameFromVeterinaryTwoParts(t *testing.T) {
	// pydicom: TestPersonName::test_from_named_components_veterinary
	pn := PersonNameFromVeterinary("Doe^John", "Spot")
	if pn.Alphabetic != "Doe^John^Spot" {
		t.Fatalf("Alphabetic = %q, want Doe^John^Spot", pn.Alphabetic)
	}
}

func TestPersonNameEqualityMatrix(t *testing.T) {
	// pydicom: TestPersonName::test_eq, test_ne
	a := ParsePersonName("Doe^John")
	b := ParsePersonName("Doe^John")
	c := ParsePersonName("Doe^Jane")
	if !a.Equal(b) {
		t.Fatal("Equal() = false for identical names")
	}
	if a.Equal(c) {
		t.Fatal("Equal() = true for different names")
	}
}

func TestPersonNameRoundTripsThroughParse(t *testing.T) {
	// pydicom: TestPersonName::test_str_roundtrip, test_components_roundtrip
	inputs := []string{
		"Doe^John",
		"Doe^John=山田^太郎",
		"Doe^John=山田^太郎=やまだ^たろう",
		"王^小东=Wang^XiaoDong",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			pn := ParsePersonName(in)
			if got := pn.String(); got != in {
				t.Fatalf("round trip = %q, want %q", got, in)
			}
			again := ParsePersonName(pn.String())
			if !pn.Equal(again) {
				t.Fatalf("second parse not equal: %+v vs %+v", pn, again)
			}
		})
	}
}
