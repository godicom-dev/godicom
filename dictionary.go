package godicom

import (
	"fmt"
	"regexp"
	"strings"
)

// tagForKeyword looks up a tag by keyword string.
func tagForKeyword(keyword string) (Tag, bool) {
	t, ok := keywordToTag[keyword]
	return t, ok
}

// keywordForTag looks up the keyword for a tag.
func keywordForTag(tag Tag) (string, bool) {
	kw, ok := tagToKeyword[tag]
	if ok {
		return kw, ok
	}
	// Check repeaters
	if !tag.IsPrivate() {
		mask := maskMatch(tag)
		if mask != "" {
			if entry, ok := RepeatersDictionaryGo[mask]; ok {
				return entry.Keyword, true
			}
		}
	}
	return "", false
}

// dictionaryVR returns the VR for a given tag.
func dictionaryVR(tag Tag) (VR, error) {
	if entry, ok := DicomDictionaryGo[tag]; ok {
		return VR(entry.VR), nil
	}
	if !tag.IsPrivate() {
		mask := maskMatch(tag)
		if mask != "" {
			if entry, ok := RepeatersDictionaryGo[mask]; ok {
				return VR(entry.VR), nil
			}
		}
	}
	return "", fmt.Errorf("godicom: tag %s not found in dictionary", tag)
}

// vrDisagreesWithDictionary returns the VR the data dictionary gives tag when
// that VR and encoded cannot be the same thing, or "" when they are compatible.
//
// Only a tag the dictionary has an entry for can disagree with anything, so this
// consults dictionaryVR rather than LookupVR: LookupVR launders a missing entry
// into UN, which is a decoding default rather than an expectation. An
// unrecognised standard tag and a private tag have no dictionary VR to be wrong
// about, and reporting every one of them would bury the disagreements that
// matter -- a real file is full of private elements carrying perfectly good
// explicit VRs.
//
// A dictionary entry may name more than one permitted VR; PS3.6 spells these
// "US or SS", "OB or OW" and "US or OW", and any of the alternatives is correct.
// PixelData is one of them, so skipping this would mismatch on nearly every
// image ever written.
func vrDisagreesWithDictionary(tag Tag, encoded VR) VR {
	if encoded == "" || tag.IsPrivate() {
		return ""
	}
	want, err := dictionaryVR(tag)
	if err != nil || want == "" || want == encoded {
		return ""
	}
	for _, alt := range strings.Split(string(want), " or ") {
		if VR(strings.TrimSpace(alt)) == encoded {
			return ""
		}
	}
	return want
}

// dictionaryDescription returns the name for a given tag.
func dictionaryDescription(tag Tag) (string, bool) {
	if entry, ok := DicomDictionaryGo[tag]; ok {
		return entry.Name, true
	}
	if !tag.IsPrivate() {
		mask := maskMatch(tag)
		if mask != "" {
			if entry, ok := RepeatersDictionaryGo[mask]; ok {
				return entry.Name, true
			}
		}
	}
	return "", false
}

// dictionaryHasTag returns true if the tag exists in the dictionary.
func dictionaryHasTag(tag Tag) bool {
	_, ok := DicomDictionaryGo[tag]
	return ok
}

// dictionaryIsRetired returns true if the tag is retired.
func dictionaryIsRetired(tag Tag) bool {
	if entry, ok := DicomDictionaryGo[tag]; ok {
		return entry.Retired
	}
	return false
}

// Repeater masks: precomputed from the RepeatersDictionaryGo keys
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
	for maskStr := range RepeatersDictionaryGo {
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

// Ensure the init runs for the regexp import
var _ = regexp.Compile
