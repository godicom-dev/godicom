package godicom

import "strings"

// PersonName holds a DICOM Person Name (VR=PN) value.
type PersonName struct {
	Alphabetic  string
	Ideographic string
	Phonetic    string
	Original    string
}

// ParsePersonName parses a decoded PN string into components.
func ParsePersonName(s string) PersonName {
	s = strings.TrimRight(s, " \x00")
	parts := strings.Split(s, "=")
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	pn := PersonName{}
	if len(parts) > 0 {
		pn.Alphabetic = parts[0]
	}
	if len(parts) > 1 {
		pn.Ideographic = parts[1]
	}
	if len(parts) > 2 {
		pn.Phonetic = parts[2]
	}
	return pn
}

func (pn PersonName) String() string {
	if pn.Original != "" {
		return strings.TrimRight(pn.Original, " \x00")
	}
	if pn.Phonetic != "" {
		return strings.Join([]string{pn.Alphabetic, pn.Ideographic, pn.Phonetic}, "=")
	}
	if pn.Ideographic != "" {
		return strings.Join([]string{pn.Alphabetic, pn.Ideographic}, "=")
	}
	return pn.Alphabetic
}

func (pn PersonName) IsZero() bool {
	return pn.Original == "" && pn.Alphabetic == "" && pn.Ideographic == "" && pn.Phonetic == ""
}

// Components returns the alphabetic, ideographic, and phonetic groups.
func (pn PersonName) Components() []string {
	var out []string
	if pn.Alphabetic != "" {
		out = append(out, pn.Alphabetic)
	}
	if pn.Ideographic != "" {
		out = append(out, pn.Ideographic)
	}
	if pn.Phonetic != "" {
		out = append(out, pn.Phonetic)
	}
	return out
}

func (pn PersonName) namePart(i int) string {
	parts := strings.Split(pn.Alphabetic, "^")
	if i < len(parts) {
		return parts[i]
	}
	return ""
}

// FamilyName returns the first ^-delimited component of the alphabetic group.
func (pn PersonName) FamilyName() string { return pn.namePart(0) }

// GivenName returns the second ^-delimited component of the alphabetic group.
func (pn PersonName) GivenName() string { return pn.namePart(1) }

// MiddleName returns the third ^-delimited component of the alphabetic group.
func (pn PersonName) MiddleName() string { return pn.namePart(2) }

// NamePrefix returns the fourth ^-delimited component of the alphabetic group.
func (pn PersonName) NamePrefix() string { return pn.namePart(3) }

// NameSuffix returns the fifth ^-delimited component of the alphabetic group.
func (pn PersonName) NameSuffix() string { return pn.namePart(4) }

// FamilyCommaGiven returns "Family, Given" for the alphabetic group.
func (pn PersonName) FamilyCommaGiven() string {
	return pn.FamilyName() + ", " + pn.GivenName()
}

// Formatted substitutes named components into a format string.
// Supported names: family_name, given_name, middle_name, name_prefix, name_suffix,
// ideographic, phonetic.
func (pn PersonName) Formatted(format string) string {
	replacer := strings.NewReplacer(
		"%(family_name)s", pn.FamilyName(),
		"%(given_name)s", pn.GivenName(),
		"%(middle_name)s", pn.MiddleName(),
		"%(name_prefix)s", pn.NamePrefix(),
		"%(name_suffix)s", pn.NameSuffix(),
		"%(ideographic)s", pn.Ideographic,
		"%(phonetic)s", pn.Phonetic,
	)
	return replacer.Replace(format)
}

// FromNamedComponents builds a PersonName from alphabetic name parts.
func FromNamedComponents(family, given, middle, prefix, suffix string) PersonName {
	parts := []string{family, given, middle, prefix, suffix}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	alphabetic := strings.Join(parts, "^")
	return ParsePersonName(alphabetic)
}

// PersonNameParts holds named PN components across alphabetic/ideographic/phonetic groups.
// Mirrors pydicom PersonName.from_named_components keyword arguments.
type PersonNameParts struct {
	FamilyName string
	GivenName  string
	MiddleName string
	NamePrefix string
	NameSuffix string

	FamilyNameIdeographic string
	GivenNameIdeographic  string
	MiddleNameIdeographic string
	NamePrefixIdeographic string
	NameSuffixIdeographic string

	FamilyNamePhonetic string
	GivenNamePhonetic  string
	MiddleNamePhonetic string
	NamePrefixPhonetic string
	NameSuffixPhonetic string
}

// PersonNameFromParts builds a PersonName from structured component parts.
func PersonNameFromParts(p PersonNameParts) PersonName {
	alphabetic := joinNameParts(p.FamilyName, p.GivenName, p.MiddleName, p.NamePrefix, p.NameSuffix)
	ideographic := joinNameParts(p.FamilyNameIdeographic, p.GivenNameIdeographic, p.MiddleNameIdeographic, p.NamePrefixIdeographic, p.NameSuffixIdeographic)
	phonetic := joinNameParts(p.FamilyNamePhonetic, p.GivenNamePhonetic, p.MiddleNamePhonetic, p.NamePrefixPhonetic, p.NameSuffixPhonetic)
	pn := PersonName{
		Alphabetic:  alphabetic,
		Ideographic: ideographic,
		Phonetic:    phonetic,
	}
	return pn
}

// PersonNameFromVeterinary builds a veterinary PN (responsible party ^ patient name).
// Mirrors pydicom PersonName.from_named_components_veterinary.
func PersonNameFromVeterinary(responsibleParty, patientName string) PersonName {
	return FromNamedComponents(responsibleParty, patientName, "", "", "")
}

func joinNameParts(family, given, middle, prefix, suffix string) string {
	parts := []string{family, given, middle, prefix, suffix}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "^")
}

func (pn PersonName) Equal(other PersonName) bool {
	return pn.String() == other.String()
}
