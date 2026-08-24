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
	mask1   int
	mask2   int
}

var repeaterMasks []repeaterMask

func init() {
	for maskStr := range RepeatersDictionaryGo {
		// Convert "60xx3000" -> mask1, mask2
		mask1Str := strings.ReplaceAll(maskStr, "x", "0")
		mask2Str := ""
		for _, c := range maskStr {
			if c == 'x' {
				mask2Str += "0"
			} else {
				mask2Str += "F"
			}
		}
		mask1 := 0
		mask2 := 0
		fmt.Sscanf(mask1Str, "%x", &mask1)
		fmt.Sscanf(mask2Str, "%x", &mask2)
		repeaterMasks = append(repeaterMasks, repeaterMask{
			maskStr: maskStr,
			mask1:   mask1,
			mask2:   mask2,
		})
	}
}

func maskMatch(tag Tag) string {
	t := int(tag)
	for _, rm := range repeaterMasks {
		if (t^rm.mask1)&rm.mask2 == 0 {
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
