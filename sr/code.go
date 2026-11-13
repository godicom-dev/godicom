// Package sr holds the DICOM coded terminology: the codes of PS3.16 and the
// contexts (CIDs) that group them.
//
// This is the counterpart of pydicom's pydicom.sr, which since 3.0 is the code
// dictionary and nothing else -- the SR document reader was removed from
// pydicom, so there is none here either.
//
// # Getting a code
//
//	scheme, err := sr.Scheme("SCT")
//	lung, err := scheme.Lookup("Lung")        // (SCT, "39607008", "Lung")
//
//	context, err := sr.CID(2)                 // the contexts named in PS3.16
//	transverse, err := context.Lookup("Transverse")
//
//	pair, err := sr.CID(244)                   // a context also answers
//	right, err := pair.Lookup("Right")         // which scheme a word is in
//
// A keyword is the camel case form of a code meaning, as in pydicom:
// "FontanelOfSkull" for (SCT, "79361005", "Fontanel of skull"). Two keywords
// are not alphanumeric -- "None_" for (SCT, "260413007", "None") and
// "_125Iodine" for (SCT, "68630002", "^125^Iodine") -- because Go identifiers
// and the DICOM meanings disagree about what may start a word.
//
// # Comparing codes
//
// Two codes are equal when they mean the same concept, so the meaning and the
// coding scheme version do not enter into it, and a SNOMED code given under the
// retired "SRT" designator equals the same code under "SCT". That comparison is
// Code.Equal; a Code cannot be a map key with it, because Go's == is field by
// field. Codes are therefore not keys: index a Code by Value and
// SchemeDesignator when you need a set.
//
// # The data
//
// codes.txt is generated from pydicom by generate_sr_dict.py and read once, on
// first use, rather than built into map literals that a program pays for at
// start-up whether it looks at a code or not.
package sr

import "strings"

// Code is a coded concept: a value in a coding scheme, its meaning, and
// optionally the version of the scheme the value was drawn from.
//
// The zero Code has no scheme and no value; it is not a valid code.
type Code struct {
	// Value is the code itself, such as "39607008" in SCT or "s" in UCUM.
	Value string

	// SchemeDesignator names the coding scheme, such as "SCT" or "DCM".
	SchemeDesignator string

	// Meaning is the human readable text the scheme gives the value.
	Meaning string

	// SchemeVersion is the version of the scheme, empty when unspecified.
	SchemeVersion string
}

// New returns a code with the value, scheme designator and meaning.
func New(value, schemeDesignator, meaning string) Code {
	return Code{Value: value, SchemeDesignator: schemeDesignator, Meaning: meaning}
}

// WithVersion returns the code with the scheme version set.
func (c Code) WithVersion(version string) Code {
	c.SchemeVersion = version
	return c
}

// Equal reports whether c and other denote the same concept.
//
// The meaning does not take part: the same value in the same scheme is the same
// concept whatever text either side carries. Neither does a difference of
// designator between "SRT" and "SCT": SNOMED codes were published under both and
// pydicom treats them as one, so a code given under either designator equals the
// same code given under the other. A "SRT" code with no "SCT" counterpart
// equals only another code with the same value and designator.
func (c Code) Equal(other Code) bool {
	c = c.snomedEquivalent()
	other = other.snomedEquivalent()

	return c.Value == other.Value &&
		c.SchemeDesignator == other.SchemeDesignator &&
		c.SchemeVersion == other.SchemeVersion
}

// snomedEquivalent returns the code under the "SCT" designator when it is a
// SNOMED code given under the retired one, and c unchanged otherwise.
func (c Code) snomedEquivalent() Code {
	if c.SchemeDesignator != "SRT" {
		return c
	}
	if value, ok := data().srtToSCT[c.Value]; ok {
		c.Value = value
		c.SchemeDesignator = "SCT"
	}
	return c
}

// String returns the code as (scheme, "value", "meaning"), with the scheme
// version appended when there is one.
func (c Code) String() string {
	var b strings.Builder
	b.WriteString("(")
	b.WriteString(c.SchemeDesignator)
	b.WriteString(", ")
	b.WriteString(quote(c.Value))
	b.WriteString(", ")
	b.WriteString(quote(c.Meaning))
	if c.SchemeVersion != "" {
		b.WriteString(", ")
		b.WriteString(quote(c.SchemeVersion))
	}
	b.WriteString(")")

	return b.String()
}

// quote returns s in double quotes, escaping the two characters that would
// otherwise end the quote early.
func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)

	return `"` + s + `"`
}
