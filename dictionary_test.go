package godicom

import (
	"strings"
	"testing"
)

// A repeater mask with a zero mask field matches every tag -- (t^value)&0 is 0
// whatever t is -- so one unparsed entry does not degrade the dictionary, it
// replaces it: the first repeater in map order answers for every non-private
// tag. That is what happened on 32-bit platforms, where the fields were int and
// a mask like 0xFF00FFFF did not fit, so all 88 silently came out zero.
//
// The nibble arithmetic that replaced the parse cannot fail, which is the point;
// this pins the outcome so a future rewrite cannot quietly go back to zeroes.
func TestRepeaterMasksAllCarryBits(t *testing.T) {
	if len(repeaterMasks) == 0 {
		t.Fatal("no repeater masks were built from RepeatersDictionaryGo")
	}
	if len(repeaterMasks) != len(RepeatersDictionaryGo) {
		t.Errorf("built %d masks from %d keys", len(repeaterMasks), len(RepeatersDictionaryGo))
	}
	for _, rm := range repeaterMasks {
		if rm.mask == 0 {
			t.Errorf("mask %q has no fixed digits, so it matches every tag", rm.maskStr)
		}
	}
}

func TestRepeaterMaskBits(t *testing.T) {
	for _, tt := range []struct {
		in          string
		value, mask uint32
	}{
		{"60xx3000", 0x60003000, 0xFF00FFFF},
		{"50xx0105", 0x50000105, 0xFF00FFFF},
		{"7Fxx0010", 0x7F000010, 0xFF00FFFF},
	} {
		value, mask := repeaterMaskBits(tt.in)
		if value != tt.value || mask != tt.mask {
			t.Errorf("repeaterMaskBits(%q) = %08X/%08X, want %08X/%08X",
				tt.in, value, mask, tt.value, tt.mask)
		}
	}
}

// Group length and a plain data element are not repeaters. They matched one
// while the masks were zero, which is how (0008,0000) came back US instead of
// the UL a group length is.
//
// Being a repeater is per element, not per group: 7Fxx has entries for 0010,
// 0011, 0020, 0030 and 0040, so (7FE0,0099) is a tag in a repeater group that
// is still not a repeater itself.
func TestMaskMatchDoesNotClaimOrdinaryTags(t *testing.T) {
	for _, s := range []string{"00080000", "00280000", "00100010", "7FE00099"} {
		tg, err := ParseTag(s)
		if err != nil {
			t.Fatalf("ParseTag(%q): %v", s, err)
		}
		if got := maskMatch(tg); got != "" {
			t.Errorf("maskMatch(%s) = %q, want no match", s, got)
		}
	}
	// Real repeaters still have to match, or the check above passes vacuously.
	// (7FE0,0010) is one of them: PS3.6 gives Pixel Data as 7Fxx,0010 so it
	// covers the overlay groups, not just 7FE0.
	for _, tt := range []struct{ tag, want string }{
		{"60123000", "60xx3000"},
		{"7FE00010", "7Fxx0010"},
	} {
		tg := MustTag(tt.tag)
		if got := maskMatch(tg); got != tt.want {
			t.Errorf("maskMatch(%s) = %q, want %q", tt.tag, got, tt.want)
		}
	}
}

// Reachability is the property the zero masks destroyed: with mask == 0 the
// first entry in map order answered for every tag, so 87 of the 88 keys became
// dead. Checking that no mask is zero catches that, but only this catches a
// pattern that is merely built wrong -- substituting 0 for each x has to name
// the key it came from.
//
// The exact-key comparison is deterministic rather than dependent on map order:
// no two repeater patterns overlap on each other's canonical tag, so there is
// only ever one candidate to return.
func TestEveryRepeaterKeyIsReachable(t *testing.T) {
	zeroForX := strings.NewReplacer("x", "0", "X", "0")
	for maskStr := range RepeatersDictionaryGo {
		canonical := zeroForX.Replace(maskStr)
		tg, err := ParseTag(canonical)
		if err != nil {
			t.Fatalf("ParseTag(%q) from mask %q: %v", canonical, maskStr, err)
		}
		if got := maskMatch(tg); got != maskStr {
			t.Errorf("maskMatch(%s) = %q, want %q", canonical, got, maskStr)
		}
	}
}

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
	// Private creator tags are LO (DICOM PS3.5).
	vr = LookupVR(MustTag(0x00090010))
	if vr != VRLO {
		t.Errorf("private creator VR = %s, want LO", vr)
	}
	// Other private tags without creator context remain UN.
	vr = LookupVR(MustTag(0x00091001))
	if vr != VRUN {
		t.Errorf("private data VR = %s, want UN", vr)
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
