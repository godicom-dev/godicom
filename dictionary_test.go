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
		t.Fatal("no repeater masks were built from repeatersDictionary")
	}
	if len(repeaterMasks) != len(repeatersDictionary) {
		t.Errorf("built %d masks from %d keys", len(repeaterMasks), len(repeatersDictionary))
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
	for maskStr := range repeatersDictionary {
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

// Retired is read off the entry now that Lookup returns the whole thing. The
// dictionaryIsRetired helper this replaces consulted only the exact table, so it
// answered "not retired" for all 72 retired repeating-group entries -- and it had
// no caller outside this test, so nothing ever noticed.
func TestLookupReportsRetired(t *testing.T) {
	for _, tt := range []struct {
		name string
		tag  Tag
		want bool
	}{
		{"PatientName", MustTag(0x00100010), false},
		// (0008,0010) Recognition Code, retired in PS3.6 and an exact entry.
		{"RecognitionCode", MustTag(0x00080010), true},
		// A repeating-group entry: 002031xx Source Image IDs is retired, and only
		// resolving through the masks can see that.
		{"SourceImageIDs", MustTag(0x00203105), true},
	} {
		entry, ok := Standard().Lookup(tt.tag, "")
		if !ok {
			t.Fatalf("%s: no dictionary entry for %s", tt.name, tt.tag)
		}
		if entry.Retired != tt.want {
			t.Errorf("%s: Retired = %v, want %v", tt.name, entry.Retired, tt.want)
		}
	}
}

// Lookup resolves the exact table, the repeating-group masks and the private
// dictionaries, and refuses a private tag with no creator. Each case fails
// differently if the resolution order is wrong, which is why they are one test:
// the order is the thing under test, not the individual answers.
func TestLookup(t *testing.T) {
	t.Cleanup(ResetExtraPrivateDictionaries)

	for _, tt := range []struct {
		name    string
		tag     Tag
		creator string
		wantOK  bool
		wantVR  string
		wantKw  string
	}{
		{"exact entry", MustTag(0x00100010), "", true, "PN", "PatientName"},
		// Overlay Data has no exact entry at all -- PS3.6 gives it only as
		// 60xx,3000 -- so this case is the mask path or nothing.
		{"repeating group", MustTag(0x60123000), "", true, "OB or OW", "OverlayData"},
		// (7FE0,0010) matches both tables: Pixel Data exactly, and retired Variable
		// Pixel Data through 7Fxx,0010. The exact entry has to win.
		{"exact beats mask", MustTag(0x7FE00010), "", true, "OB or OW", "PixelData"},
		// A creator is irrelevant to a standard tag rather than an error.
		{"exact entry, creator ignored", MustTag(0x00100010), "ACUSON", true, "PN", "PatientName"},
		// Private entries carry no keyword; PS3.6 assigns none.
		{"private with creator", MustTag(0x00090000), "ACUSON", true, "IS", ""},
		{"private without creator", MustTag(0x00090000), "", false, "", ""},
		{"private with wrong creator", MustTag(0x00090000), "NOT A VENDOR", false, "", ""},
		{"unknown standard tag", MustTag(0x00091001), "", false, "", ""},
	} {
		entry, ok := Standard().Lookup(tt.tag, tt.creator)
		if ok != tt.wantOK {
			t.Errorf("%s: Lookup(%s, %q) ok = %v, want %v", tt.name, tt.tag, tt.creator, ok, tt.wantOK)
			continue
		}
		if entry.VR != tt.wantVR {
			t.Errorf("%s: VR = %q, want %q", tt.name, entry.VR, tt.wantVR)
		}
		if entry.Keyword != tt.wantKw {
			t.Errorf("%s: Keyword = %q, want %q", tt.name, entry.Keyword, tt.wantKw)
		}
	}

	// A runtime addition has to be visible through the same method, or callers get
	// one dictionary from Lookup and a different one from PrivateDictionaryVR.
	tag := MustTag(0x0041, 0x0001)
	if err := AddPrivateDictEntry("ACME 3.2", tag, VRUS, "Some Number"); err != nil {
		t.Fatal(err)
	}
	entry, ok := Standard().Lookup(tag, "ACME 3.2")
	if !ok || entry.VR != "US" || entry.Name != "Some Number" {
		t.Errorf("Lookup of runtime entry = %+v, %v; want US/Some Number", entry, ok)
	}
}

// A private tag is never resolved against the standard repeating-group masks.
// (0029,xx10) is a real vendor block and 0029 is odd, so it is private by
// definition; answering it from the standard tables would give a VR from a
// pattern that has nothing to do with whoever wrote the file.
func TestLookupDoesNotMaskPrivateTags(t *testing.T) {
	// A private tag that a standard mask would otherwise match: 1000xxx0 covers
	// (1000,0000)-(1000,FFF0), and group 1001 is private and odd.
	tag := MustTag(0x10010000)
	if !tag.IsPrivate() {
		t.Fatalf("%s should be private", tag)
	}
	if _, ok := Standard().Lookup(tag, ""); ok {
		t.Errorf("Lookup(%s) resolved a private tag against the standard tables", tag)
	}
}

// dictionaryHasTag asks a narrower question than Lookup on purpose: the
// endianness heuristic needs an exact match, and the masks would hand it tens of
// thousands of extra "known" tags. Overlay Data pins the difference -- Lookup
// finds it through 60xx,3000, and there is no exact entry to find.
func TestDictionaryHasTagIgnoresRepeaters(t *testing.T) {
	tag := MustTag(0x60123000)
	if _, ok := Standard().Lookup(tag, ""); !ok {
		t.Fatalf("Lookup(%s) should resolve through the repeater masks", tag)
	}
	if dictionaryHasTag(tag) {
		t.Errorf("dictionaryHasTag(%s) = true; it must not resolve through masks", tag)
	}
}

// keywordForTag has to answer "no keyword" rather than "the empty keyword".
// Seven standard entries are retired blanks with a name and no keyword at all,
// and a caller that gets ("", true) prints nothing instead of falling back to
// the tag.
func TestKeywordForTagRejectsBlankEntries(t *testing.T) {
	blank := MustTag(0x00080202)
	entry, ok := Standard().Lookup(blank, "")
	if !ok || entry.Keyword != "" {
		t.Fatalf("%s: entry = %+v, ok = %v; want an entry with no keyword", blank, entry, ok)
	}
	if kw, ok := keywordForTag(blank); ok {
		t.Errorf("keywordForTag(%s) = %q, true; want no keyword", blank, kw)
	}
	// A repeater still resolves, or the check above passes for the wrong reason.
	if kw, ok := keywordForTag(MustTag(0x60123000)); !ok || kw != "OverlayData" {
		t.Errorf("keywordForTag((6012,3000)) = %q, %v; want OverlayData, true", kw, ok)
	}
}

// VRs splits the compound forms PS3.6 writes as prose. The three-way form is the
// one that matters: it is PixelData, and reading "US or SS or OW" as a single VR
// makes every image in the world look like it disagrees with the dictionary.
func TestDictEntryVRs(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want []VR
	}{
		{"PN", []VR{"PN"}},
		{"US or SS", []VR{"US", "SS"}},
		{"OB or OW", []VR{"OB", "OW"}},
		{"US or OW", []VR{"US", "OW"}},
		{"US or SS or OW", []VR{"US", "SS", "OW"}},
		{"", nil},
	} {
		got := DictEntry{VR: tt.in}.VRs()
		if len(got) != len(tt.want) {
			t.Errorf("DictEntry{VR: %q}.VRs() = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("DictEntry{VR: %q}.VRs() = %v, want %v", tt.in, got, tt.want)
				break
			}
		}
	}

	// Every VR the dictionary actually contains has to split into either a real
	// two-letter VR or the literal NONE, which Item and the two delimitation items
	// carry in place of one. Counting the NONEs rather than exempting them keeps
	// this from becoming a blanket escape hatch: a form nobody anticipated --
	// another three-way, a separator other than " or " -- shows up here rather than
	// as a spurious diagnostic on somebody's file.
	none := 0
	for tag, entry := range dicomDictionary {
		for _, vr := range entry.VRs() {
			switch {
			case vr == "NONE":
				none++
			case len(vr) != 2:
				t.Errorf("%s: VR %q from %q is not a two-letter VR", tag, vr, entry.VR)
			}
		}
	}
	if none != 3 {
		t.Errorf("found %d NONE VRs, want 3 (Item and the two delimitation items)", none)
	}
}

// The compound entries are why vrDisagreesWithDictionary cannot compare strings.
// (0028,1200) is the only three-way entry, "US or SS or OW", so each of the three
// is correct and only something outside the set is a disagreement.
func TestVRDisagreesWithDictionaryAcceptsEveryAlternative(t *testing.T) {
	grayLUT := MustTag(0x00281200)
	entry, ok := Standard().Lookup(grayLUT, "")
	if !ok {
		t.Fatalf("no entry for %s", grayLUT)
	}
	if entry.VR != "US or SS or OW" {
		t.Fatalf("(0028,1200) VR = %q, want the three-way compound form", entry.VR)
	}
	for _, vr := range []VR{"US", "SS", "OW"} {
		if got := vrDisagreesWithDictionary(grayLUT, vr); got != "" {
			t.Errorf("vrDisagreesWithDictionary((0028,1200), %s) = %q, want no disagreement", vr, got)
		}
	}
	// A VR outside the set still has to be reported, or this test would pass with
	// the function stubbed out to return "".
	if got := vrDisagreesWithDictionary(grayLUT, VRPN); got != "US or SS or OW" {
		t.Errorf("vrDisagreesWithDictionary((0028,1200), PN) = %q, want the dictionary VR", got)
	}
	// PixelData is the two-way case, and the one that actually turns up: an image
	// encoded OW and an image encoded OB are both right.
	pixelData := MustTag(0x7FE00010)
	for _, vr := range []VR{"OB", "OW"} {
		if got := vrDisagreesWithDictionary(pixelData, vr); got != "" {
			t.Errorf("vrDisagreesWithDictionary(PixelData, %s) = %q, want no disagreement", vr, got)
		}
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
	if len(privateDictionaries) < 400 {
		t.Fatalf("privateDictionaries has only %d creators", len(privateDictionaries))
	}
	entries := 0
	for _, inner := range privateDictionaries {
		entries += len(inner)
	}
	if entries < 10000 {
		t.Fatalf("privateDictionaries has only %d entries", entries)
	}
}
