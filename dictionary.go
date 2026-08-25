package godicom

import (
	"fmt"
	"strings"
)

// Dictionary resolves a tag to its data dictionary entry. Standard returns the
// one PS3.6 defines, which is what godicom reads and writes with.
//
// creator names the Private Creator of a private tag and is ignored for a
// standard one, so a single method answers for both halves of the dictionary.
// Pass "" for a standard tag, or for a private tag whose creator is unknown -- a
// private tag on its own has no entry to find, because the same tag means
// different things to different vendors.
type Dictionary interface {
	Lookup(tag Tag, creator string) (DictEntry, bool)
}

// Standard returns the DICOM data dictionary from PS3.6: the standard elements,
// the repeating-group elements, and the registered private creators together with
// anything added by AddPrivateDictEntry.
func Standard() Dictionary { return standardDictionary{} }

// standardDictionary has no fields because the tables it reads are package state.
// It is a named type anyway: without one there is nothing for a caller to hold,
// wrap, or count lookups on.
//
// Routing the package's own lookups through Standard costs nothing, which is why
// they do rather than reaching for the maps directly: the type is empty and the
// method set is known, so the compiler inlines Standard and devirtualizes every
// internal Lookup back into the map accesses below.
type standardDictionary struct{}

// Lookup resolves tag in the order PS3.6 leaves no choice about.
//
// An exact entry beats a repeating-group pattern, and (7FE0,0010) is why that
// order is not arbitrary: it is Pixel Data as an exact entry and retired Variable
// Pixel Data through the 7Fxx,0010 mask, and every image ever written means the
// former.
//
// A private tag is never resolved against the standard tables. (0029,xx10) means
// whatever the vendor who wrote it says it means, so without a creator there is
// no answer to give.
func (standardDictionary) Lookup(tag Tag, creator string) (DictEntry, bool) {
	if entry, ok := dicomDictionary[tag]; ok {
		return entry, true
	}
	if tag.IsPrivate() {
		if creator == "" {
			return DictEntry{}, false
		}
		entry, ok := lookupPrivateDictEntry(tag, creator)
		if !ok {
			return DictEntry{}, false
		}
		// Keyword stays empty: PS3.6 assigns keywords to standard elements, and a
		// private element has none to assign.
		return DictEntry{
			VR:      entry.VR,
			VM:      entry.VM,
			Name:    entry.Name,
			Retired: entry.Retired,
		}, true
	}
	if mask := maskMatch(tag); mask != "" {
		if entry, ok := repeatersDictionary[mask]; ok {
			return entry, true
		}
	}
	return DictEntry{}, false
}

// VRs returns the VRs the entry permits, in the order PS3.6 lists them.
//
// Most entries permit one. Thirty-seven permit two -- "OB or OW", which is
// PixelData and so nearly every image ever written, plus "US or SS" and
// "US or OW" -- and (0028,1200) Gray Lookup Table Data permits three,
// "US or SS or OW". Splitting on " or " rather than special-casing pairs is what
// keeps that last form from reading as one VR named "US or SS or OW", which
// matches nothing.
//
// Three entries say "NONE" rather than naming a VR: Item and the two delimitation
// items, which carry no VR at all. That is returned as it stands, because it is
// what the dictionary says.
//
// An entry with no VR yields no VRs rather than one empty VR: a runtime private
// entry can be registered without one, and "" is not a VR that anything matches.
func (e DictEntry) VRs() []VR {
	if e.VR == "" {
		return nil
	}
	parts := strings.Split(e.VR, " or ")
	vrs := make([]VR, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			vrs = append(vrs, VR(part))
		}
	}
	return vrs
}

// tagForKeyword looks up a tag by keyword string.
func tagForKeyword(keyword string) (Tag, bool) {
	t, ok := keywordToTag[keyword]
	return t, ok
}

// keywordForTag looks up the keyword for a tag.
//
// An entry with no keyword counts as no answer rather than as an empty one. Seven
// standard entries are retired blanks that have none, and a caller asking for a
// keyword cannot use "" -- it wants to fall back to printing the tag.
func keywordForTag(tag Tag) (string, bool) {
	entry, ok := Standard().Lookup(tag, "")
	if !ok || entry.Keyword == "" {
		return "", false
	}
	return entry.Keyword, true
}

// dictionaryVR returns the VR for a given tag.
func dictionaryVR(tag Tag) (VR, error) {
	entry, ok := Standard().Lookup(tag, "")
	if !ok {
		return "", fmt.Errorf("godicom: tag %s not found in dictionary", tag)
	}
	return VR(entry.VR), nil
}

// vrDisagreesWithDictionary returns the VR the data dictionary gives tag when
// that VR and encoded cannot be the same thing, or "" when they are compatible.
//
// Only a tag the dictionary has an entry for can disagree with anything, so this
// consults the dictionary directly rather than LookupVR: LookupVR launders a
// missing entry into UN, which is a decoding default rather than an expectation.
// An unrecognised standard tag and a private tag have no dictionary VR to be
// wrong about, and reporting every one of them would bury the disagreements that
// matter -- a real file is full of private elements carrying perfectly good
// explicit VRs.
//
// An entry may permit more than one VR, and any of them is correct. PixelData is
// one such entry -- "OB or OW" -- so treating the whole string as a single VR
// would mismatch on nearly every image ever written. VRs does that splitting.
func vrDisagreesWithDictionary(tag Tag, encoded VR) VR {
	if encoded == "" || tag.IsPrivate() {
		return ""
	}
	entry, ok := Standard().Lookup(tag, "")
	if !ok || entry.VR == "" {
		return ""
	}
	for _, permitted := range entry.VRs() {
		if permitted == encoded {
			return ""
		}
	}
	return VR(entry.VR)
}

// dictionaryDescription returns the name for a given tag.
func dictionaryDescription(tag Tag) (string, bool) {
	entry, ok := Standard().Lookup(tag, "")
	if !ok {
		return "", false
	}
	return entry.Name, true
}

// dictionaryHasTag reports whether tag has an entry of its very own.
//
// Deliberately not Standard().Lookup: this is the probe the endianness heuristic
// runs, and it needs "this exact tag is a known element", not "something in the
// dictionary covers this tag". The 88 repeating-group patterns cover 108,688 tags
// between them against 5,189 exact entries, and the heuristic picks a byte order
// by which of the two candidate readings is the recognised one -- the more
// readings it recognises, the less it is deciding anything.
//
// No test in the suite currently tells the two apart on a real file, so nothing
// would have caught this being widened; TestDictionaryHasTagIgnoresRepeaters is
// what keeps the narrow question narrow.
func dictionaryHasTag(tag Tag) bool {
	_, ok := dicomDictionary[tag]
	return ok
}

// Repeater masks: precomputed from the repeatersDictionary keys
type repeaterMask struct {
	maskStr string
	// A tag is 32 unsigned bits, so these are too. They were int, and int is 32
	// bits wide on a 32-bit platform -- one bit short of holding a mask like
	// 0xFF00FFFF. fmt.Sscanf failed there and left the field zero, which made
	// (t^mask1)&mask2 == 0 true for every tag: the whole dictionary resolved
	// through whichever repeater happened to be first.
	value uint32 // the fixed digits, with each x as 0
	mask  uint32 // F where the digit is fixed, 0 where it may vary
}

var repeaterMasks []repeaterMask

func init() {
	for maskStr := range repeatersDictionary {
		value, mask := repeaterMaskBits(maskStr)
		repeaterMasks = append(repeaterMasks, repeaterMask{
			maskStr: maskStr,
			value:   value,
			mask:    mask,
		})
	}
}

// repeaterMaskBits turns a key like "60xx3000" into the pair maskMatch compares
// against. Shifting nibbles rather than building two strings and parsing them
// back means there is no error to drop on the floor and no platform-dependent
// width to overflow.
func repeaterMaskBits(maskStr string) (value, mask uint32) {
	for _, c := range maskStr {
		value <<= 4
		mask <<= 4
		if c == 'x' || c == 'X' {
			continue
		}
		mask |= 0xF
		switch {
		case c >= '0' && c <= '9':
			value |= uint32(c - '0')
		case c >= 'a' && c <= 'f':
			value |= uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			value |= uint32(c-'A') + 10
		}
	}
	return value, mask
}

func maskMatch(tag Tag) string {
	t := uint32(tag)
	for _, rm := range repeaterMasks {
		if (t^rm.value)&rm.mask == 0 {
			return rm.maskStr
		}
	}
	return ""
}

// TagFromKeyword returns the tag for a given keyword.
func TagFromKeyword(keyword string) (Tag, error) {
	tag, ok := tagForKeyword(keyword)
	if !ok {
		return 0, fmt.Errorf("godicom: unknown keyword %q", keyword)
	}
	return tag, nil
}

// LookupVR returns the VR for a tag, with fallback for unknown tags.
func LookupVR(tag Tag) VR {
	if tag.IsPrivate() {
		if tag.IsPrivateCreator() {
			return VRLO
		}
		return VRUN
	}
	vr, err := dictionaryVR(tag)
	if err != nil {
		return VRUN
	}
	return vr
}

// lookupVRWithCreator resolves VR for a private tag using its creator string.
// Mirrors pydicom datadict.dictionary_VR for private elements during implicit read.
func lookupVRWithCreator(tag Tag, creator string) VR {
	if !tag.IsPrivate() {
		return LookupVR(tag)
	}
	if tag.IsPrivateCreator() {
		return VRLO
	}
	if creator == "" {
		return VRUN
	}
	if vr, ok := privateDictionaryVR(tag, creator); ok {
		return vr
	}
	return VRUN
}

// IsRepeaterTag returns true if the tag matches a repeater pattern.
func IsRepeaterTag(tag Tag) bool {
	return maskMatch(tag) != ""
}
