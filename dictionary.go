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

// NewDictionary composes dicts into one, trying each in turn and returning the
// first entry found. Later dictionaries are the fallback, so
//
//	godicom.NewDictionary(vendor, godicom.Standard())
//
// reads vendor's entries where it has them and PS3.6's everywhere else, and
// reversing the arguments makes PS3.6 win instead. Composing rather than
// replacing is the point: a caller that only wants to add a vendor block should
// not have to carry the other 5,189 entries itself.
//
// First match wins, not most specific: a dictionary that answers for a tag ends
// the search even when a later one has a better entry. That is the rule because
// it is the only one a caller can reason about from the argument order alone.
//
// A dictionary with nothing in it is fine and answers nothing. NewDictionary()
// with no arguments is that dictionary, and is not an error -- it is what an
// options struct threading a caller-built list ends up with when the list is
// empty.
//
// The result holds the slice, not a copy of what the dictionaries contain, so a
// PrivateDictionary among them stays live: an Add after composition is visible
// through the composite. dicts itself is copied, so appending to the caller's
// slice afterwards is not.
func NewDictionary(dicts ...Dictionary) Dictionary {
	return multiDictionary(append([]Dictionary(nil), dicts...))
}

type multiDictionary []Dictionary

func (m multiDictionary) Lookup(tag Tag, creator string) (DictEntry, bool) {
	for _, d := range m {
		if d == nil {
			continue
		}
		if entry, ok := d.Lookup(tag, creator); ok {
			return entry, true
		}
	}
	return DictEntry{}, false
}

// dictionaryOrStandard is how every internal caller reaches the dictionary in
// effect: nil means PS3.6, so the read path never has to nil-check and a caller
// who set nothing gets exactly what godicom has always done.
func dictionaryOrStandard(d Dictionary) Dictionary {
	if d == nil {
		return Standard()
	}
	return d
}

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

// vrDisagreesWithDictionary returns the VR dict gives tag when that VR and
// encoded cannot be the same thing, or "" when they are compatible.
//
// Only a tag the dictionary has an entry for can disagree with anything, so this
// consults the dictionary directly rather than LookupVR: LookupVR launders a
// missing entry into UN, which is a decoding default rather than an expectation.
// An unrecognised standard tag and a private tag have no dictionary VR to be
// wrong about, and reporting every one of them would bury the disagreements that
// matter -- a real file is full of private elements carrying perfectly good
// explicit VRs.
//
// Private tags stay exempt even when the read was given a private dictionary that
// does have an entry: the creator that entry hangs off is not available here, and
// resolving it per element would charge the quiet path for a diagnostic nobody
// asked for.
//
// An entry may permit more than one VR, and any of them is correct. PixelData is
// one such entry -- "OB or OW" -- so treating the whole string as a single VR
// would mismatch on nearly every image ever written. VRs does that splitting.
func vrDisagreesWithDictionary(dict Dictionary, tag Tag, encoded VR) VR {
	if encoded == "" || tag.IsPrivate() {
		return ""
	}
	entry, ok := dict.Lookup(tag, "")
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
//
// ReadOptions.Dictionary does not reach here either, and should not. This runs
// before the transfer syntax is known, to guess a byte order from the first tag in
// the dataset; a caller's private block cannot help -- a private tag is private in
// both byte orders -- and letting a supplied dictionary answer would make the
// guess depend on how many entries the caller happened to add.
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

// LookupVR returns the VR the standard data dictionary gives tag, or UN when it
// has none to give.
//
// A private tag other than a Private Creator element answers UN, because its VR
// depends on the vendor who wrote it and this function is not told who that was.
// The read path resolves those through the dictionary it was given, which has
// the creator in hand; see ReadOptions.Dictionary.
func LookupVR(tag Tag) VR {
	return vrResolver{}.vrFor(tag)
}

// vrResolver answers the one question a header decoder asks the dictionary: what
// VR does this tag have, when the encoding did not carry one?
//
// It pairs the dictionary in effect with the means of finding a private tag's
// Private Creator, because neither answers alone. A private tag's VR depends on
// who wrote the file, and a creator is only worth resolving against a dictionary
// that has that vendor's block in it -- which is the whole reason a caller
// supplies one.
//
// The zero value resolves against Standard with no creator, which is what a
// caller holding neither should get: a header decoded outside a parse, or a test.
type vrResolver struct {
	dict    Dictionary
	creator creatorFunc
}

// vrFor resolves tag's VR, mirroring pydicom datadict.dictionary_VR plus the UN
// fallback filereader relies on.
//
// A tag with no entry is UN rather than an error. PS3.5 reads an element of
// unknown VR as UN, and a file full of one vendor's private elements stays
// readable without that vendor's dictionary -- as UN byte strings, which is
// exactly what they are to a reader that cannot name them.
//
// An entry that names no VR counts as no entry. The generated tables contain no
// such thing, but a caller's Dictionary may, and "" is not a VR that anything
// downstream matches, while UN is.
func (r vrResolver) vrFor(tag Tag) VR {
	// PS3.5 fixes the Private Creator element at LO whatever a dictionary says,
	// and none of them list the (gggg,0010-00FF) block anyway, so asking would
	// only ever answer UN.
	if tag.IsPrivateCreator() {
		return VRLO
	}
	// Resolving a creator costs a scan of what has been parsed so far, so it is
	// only paid for by a private tag that needs it.
	creator := ""
	if tag.IsPrivate() && r.creator != nil {
		creator = r.creator(tag)
	}
	entry, ok := dictionaryOrStandard(r.dict).Lookup(tag, creator)
	if !ok || entry.VR == "" {
		return VRUN
	}
	// Returned as it stands, compound forms included: an implicit-VR PixelData
	// resolves to "OB or OW", and vr.go's ambiguous-VR handling is what reads
	// that. Splitting it here would pick one of two the dictionary declines to
	// choose between.
	return VR(entry.VR)
}

// IsRepeaterTag returns true if the tag matches a repeater pattern.
func IsRepeaterTag(tag Tag) bool {
	return maskMatch(tag) != ""
}
