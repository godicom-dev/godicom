package godicom

import (
	"testing"
)

func TestTagConstruction(t *testing.T) {
	tests := []struct {
		name string
		arg  interface{}
		want Tag
	}{
		{"from int", 0x00100010, Tag(0x00100010)},
		{"from two ints", [2]int{0x0010, 0x0010}, Tag(0x00100010)},
		{"from Tag", Tag(0x00100010), Tag(0x00100010)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTag(tt.arg)
			if err != nil {
				t.Fatalf("ParseTag(%v) error: %v", tt.arg, err)
			}
			if got != tt.want {
				t.Errorf("ParseTag(%v) = %v, want %v", tt.arg, got, tt.want)
			}
		})
	}
}

func TestTagProperties(t *testing.T) {
	tag := MustTag(0x00100010)
	if tag.Group() != 0x0010 {
		t.Errorf("Group() = %04X, want 0010", tag.Group())
	}
	if tag.Element() != 0x0010 {
		t.Errorf("Element() = %04X, want 0010", tag.Element())
	}
	if tag.IsPrivate() {
		t.Error("IsPrivate() = true, want false")
	}
	if tag.String() != "(0010,0010)" {
		t.Errorf("String() = %s, want (0010,0010)", tag.String())
	}
}

func TestTagPrivate(t *testing.T) {
	tag := MustTag(0x00090010)
	if !tag.IsPrivate() {
		t.Error("IsPrivate() = false, want true")
	}
	if !tag.IsPrivateCreator() {
		t.Error("IsPrivateCreator() = false, want true")
	}
}

func TestTagComparison(t *testing.T) {
	a := MustTag(0x00100010)
	b := MustTag(0x00100020)
	if a >= b {
		t.Error("a should be < b")
	}
	if b <= a {
		t.Error("b should be > a")
	}
	aCopy := a
	if a != aCopy {
		t.Error("a should equal itself")
	}
}

func TestTagSpecialTags(t *testing.T) {
	if ItemTag != 0xFFFEE000 {
		t.Error("ItemTag wrong")
	}
	if ItemDelimiterTag != 0xFFFEE00D {
		t.Error("ItemDelimiterTag wrong")
	}
	if SequenceDelimiterTag != 0xFFFEE0DD {
		t.Error("SequenceDelimiterTag wrong")
	}
}

func TestTagPrivateCreator(t *testing.T) {
	tag := MustTag(0x00090020)
	pc := tag.PrivateCreator()
	if pc != MustTag(0x00090000) {
		t.Errorf("PrivateCreator() = %v, want (0009,0000)", pc)
	}
}

func TestTagJSONKey(t *testing.T) {
	tag := MustTag(0x00100010)
	if tag.JSONKey() != "00100010" {
		t.Errorf("JSONKey() = %s, want 00100010", tag.JSONKey())
	}
}

// A tag is an unsigned 32-bit value, so the hex string form has to reach past
// 0x7FFFFFFF. Parsing it as signed silently lost every group from 8000 up --
// including the three tags the encapsulation and sequence machinery is built
// from, which are also the ones a JSONKey round-trip is most likely to hit.
//
// pydicom accepts all of these (tag.py uses int(arg, 16), which has no width),
// so a Go port that rejects them is a parity break, not a stricter reading.
func TestParseTagAcceptsHexStringsAboveInt32(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Tag
	}{
		{"7FE00010", Tag(0x7FE00010)}, // just under, worked before
		{"80000000", Tag(0x80000000)}, // the first value a signed parse rejects
		{"FFFEE000", ItemTag},
		{"FFFEE00D", ItemDelimiterTag},
		{"FFFEE0DD", SequenceDelimiterTag},
		{"FFFFFFFF", Tag(0xFFFFFFFF)},
	} {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseTag(tt.in)
			if err != nil {
				t.Fatalf("ParseTag(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseTag(%q) = %08X, want %08X", tt.in, uint32(got), uint32(tt.want))
			}
			// JSONKey is the inverse, so the pair has to round-trip.
			if key := got.JSONKey(); key != tt.in {
				t.Errorf("ParseTag(%q).JSONKey() = %q", tt.in, key)
			}
		})
	}
}

// Nine hex digits is not a tag, and widening the parse must not start accepting
// one by truncating it.
func TestParseTagRejectsAnOverlongHexString(t *testing.T) {
	if got, err := ParseTag("1FFFEE000"); err == nil {
		t.Errorf("ParseTag(%q) = %08X, want an error", "1FFFEE000", uint32(got))
	}
}
