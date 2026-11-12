package godicom

import (
	"encoding"
	"testing"

	"github.com/godicom-dev/godicom/tag"
)

// Ported from pydicom tests/test_tag.py::TestBaseTag. The comparison-operator
// cases (test_le_same_class, test_gt_tuple, ...) are Python operator-overloading
// tests: a Go Tag is a uint32 and uses native ==, so they have no counterpart.

func TestTagGroupAndElement(t *testing.T) {
	// pydicom: TestBaseTag::test_group, test_element
	tests := []struct {
		name        string
		tag         Tag
		wantGroup   int
		wantElement int
	}{
		{"patient_name", MustTag(0x00100010), 0x0010, 0x0010},
		{"pixel_data", MustTag(0x7FE00010), 0x7FE0, 0x0010},
		{"high_group", MustTag(0x00090001), 0x0009, 0x0001},
		{"group_length", MustTag(0x00100000), 0x0010, 0x0000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tag.Group(); got != tt.wantGroup {
				t.Fatalf("Group() = %04X, want %04X", got, tt.wantGroup)
			}
			if got := tt.tag.Element(); got != tt.wantElement {
				t.Fatalf("Element() = %04X, want %04X", got, tt.wantElement)
			}
		})
	}
}

func TestTagIsPrivate(t *testing.T) {
	// pydicom: TestBaseTag::test_private
	tests := []struct {
		name string
		tag  Tag
		want bool
	}{
		{"even_group_is_public", MustTag(0x00100010), false},
		{"odd_group_is_private", MustTag(0x00090001), true},
		{"odd_file_meta_group_is_public", MustTag(0x00020010), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tag.IsPrivate(); got != tt.want {
				t.Fatalf("IsPrivate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTagPrivateCreatorMapping(t *testing.T) {
	// pydicom: TestBaseTag::test_is_private_creator, test_private_creator
	tests := []struct {
		name        string
		tag         Tag
		wantCreator Tag
		wantIsCreat bool
	}{
		{"creator_element_range", MustTag(0x00090010), MustTag(0x00090000), true},
		{"data_element_maps_to_creator", MustTag(0x00091001), MustTag(0x00090010), false},
		{"creator_itself", MustTag(0x000900AB), MustTag(0x00090000), true},
		{"public_tag_creator_is_not_flagged", MustTag(0x00100010), MustTag(0x00100000), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tag.PrivateCreator(); got != tt.wantCreator {
				t.Fatalf("PrivateCreator() = %s, want %s", got, tt.wantCreator)
			}
			if got := tt.tag.IsPrivateCreator(); got != tt.wantIsCreat {
				t.Fatalf("IsPrivateCreator() = %v, want %v", got, tt.wantIsCreat)
			}
		})
	}
}

func TestTagStringAndJSONKey(t *testing.T) {
	// pydicom: TestBaseTag::test_str, test_json_key
	tests := []struct {
		name    string
		tag     Tag
		wantStr string
		wantKey string
	}{
		{"patient_name", MustTag(0x00100010), "(0010,0010)", "00100010"},
		{"pixel_data", MustTag(0x7FE00010), "(7FE0,0010)", "7FE00010"},
		{"item", ItemTag, "(FFFE,E000)", "FFFEE000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tag.String(); got != tt.wantStr {
				t.Fatalf("String() = %q, want %q", got, tt.wantStr)
			}
			if got := tt.tag.JSONKey(); got != tt.wantKey {
				t.Fatalf("JSONKey() = %q, want %q", got, tt.wantKey)
			}
		})
	}
}

func TestTagMarshalText(t *testing.T) {
	// pydicom: TestBaseTag::test_str via JSON serialisation of a tag
	var m encoding.TextMarshaler = MustTag(0x00100010)
	got, err := m.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() error = %v", err)
	}
	if string(got) != "(0010,0010)" {
		t.Fatalf("MarshalText() = %q, want (0010,0010)", got)
	}
}

func TestTagOrdering(t *testing.T) {
	// pydicom: TestBaseTag::test_lt_same_class, test_le_same_class,
	// test_gt_same_class, test_ge_same_class (native Go comparisons)
	small := MustTag(0x00100010)
	large := MustTag(0x00100020)
	if !(small < large) {
		t.Fatal("small < large should hold")
	}
	if !(large > small) {
		t.Fatal("large > small should hold")
	}
	// Reflexive, reached by two routes rather than one: comparing a value with
	// itself is a constant expression in Go and asserts nothing.
	parsed, err := tag.Parse("00100010")
	if err != nil {
		t.Fatalf("Parse(00100010) error = %v", err)
	}
	if !(small <= parsed) || !(small >= parsed) || small != parsed {
		t.Fatal("the same tag parsed twice should compare equal both ways")
	}
}

func TestTagDelimiterConstants(t *testing.T) {
	// pydicom: module-level ITEM_TAG / ITEM_DELIMITER_TAG / SEQUENCE_DELIMITER_TAG
	if ItemTag != 0xFFFEE000 {
		t.Fatalf("ItemTag = %s, want (FFFE,E000)", ItemTag)
	}
	if ItemDelimiterTag != 0xFFFEE00D {
		t.Fatalf("ItemDelimiterTag = %s, want (FFFE,E00D)", ItemDelimiterTag)
	}
	if SequenceDelimiterTag != 0xFFFEE0DD {
		t.Fatalf("SequenceDelimiterTag = %s, want (FFFE,E0DD)", SequenceDelimiterTag)
	}
}

func TestParseTagAcceptsEveryDocumentedForm(t *testing.T) {
	// pydicom: tests/test_tag.py::TestTag::test_constructor_str and friends --
	// every spelling a caller may hold has to reach the same tag.
	tests := []struct {
		name string
		in   interface{}
		want Tag
	}{
		{"hex_int", 0x00100010, MustTag(0x00100010)},
		{"hex_string", "00100010", MustTag(0x00100010)},
		{"tuple_string", "(0010,0010)", MustTag(0x00100010)},
		{"keyword", "PatientName", MustTag(0x00100010)},
		{"uint32", uint32(0x00100010), MustTag(0x00100010)},
		{"pair_of_ints", [2]int{0x0010, 0x0010}, MustTag(0x00100010)},
		{"group_element_args", []int{0x0010, 0x0010}, MustTag(0x00100010)},
		{"delimiter_hex_string", "FFFEE000", ItemTag},
		{"delimiter_tuple_string", "(FFFE,E000)", ItemTag},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Tag
			var err error
			if args, ok := tt.in.([]int); ok {
				got = MustTag(args[0], args[1])
			} else {
				got, err = ParseTag(tt.in)
			}
			if err != nil {
				t.Fatalf("ParseTag(%v) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseTag(%v) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTagRejectsUnknownKeyword(t *testing.T) {
	// pydicom: TestTag::test_constructor_str_invalid
	if _, err := ParseTag("NotAKeyword"); err == nil {
		t.Fatal("ParseTag(NotAKeyword) = nil error, want an error")
	}
	if _, err := ParseTag(3.5); err == nil {
		t.Fatal("ParseTag(float64) = nil error, want an error")
	}
}

func TestTagPackageParseHighBitValues(t *testing.T) {
	// The godicom ParseTag comment says a tag is an unsigned 32-bit value and a
	// signed parse would reject everything from 0x80000000 up. The tag package's
	// own Parse has to agree with it, delimiters included.
	tests := []string{"FFFEE000", "FFFEE00D", "FFFEE0DD", "80000000"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			got, err := tag.Parse(in)
			if err != nil {
				t.Fatalf("tag.Parse(%q) error = %v", in, err)
			}
			if want, _ := ParseTag(in); got != want {
				t.Fatalf("tag.Parse(%q) = %s, godicom.ParseTag = %s", in, got, want)
			}
		})
	}
}
