package godicom

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ValueValidationError says why a value cannot be spelled the way its VR
// requires. It is the Go counterpart of the message pydicom's validate_value
// raises, and it exists for the same reason: a value that violates PS3.5
// Table 6.2-1 produces a file that a strict receiver is entitled to refuse,
// and the earlier the caller learns that the cheaper the repair.
//
// The value is carried as it was handed in, untruncated and unpadded, because
// the caller needs to see the actual spelling to fix it.
type ValueValidationError struct {
	// VR the value was validated against.
	VR VR

	// Value is the offending value, exactly as supplied.
	Value string

	// Reasons names every rule the value breaks, in the order they were
	// checked. More than one is normal rather than exceptional: a 20-character
	// CS that is also lowercase breaks the length rule and the character rule,
	// and a caller repairing it should not have to run the check twice to learn
	// both. pydicom reports the same pair in one message.
	Reasons []string
}

func (e *ValueValidationError) Error() string {
	return fmt.Sprintf("godicom: %q is not a valid %s value: %s",
		e.Value, e.VR, strings.Join(e.Reasons, "; "))
}

// valueValidator reports every PS3.5 Table 6.2-1 rule that value breaks for vr.
// An empty result means the value is legal; a nil return is the ordinary case
// and allocates nothing.
//
// pydicom's validators return (is_valid, message) instead. Go spells "did it
// work, and if not why" as a single result whenever the reason is a list, and
// here it genuinely is a list -- pydicom has to join its two messages by hand.
// Returning the reasons rather than a formatted message also keeps the VR and
// the original value out of the validators, so they cannot be reported wrong.
type valueValidator func(vr VR, value string) []string

// valueValidators is pydicom's VALIDATORS table, restricted to the VRs whose
// value is text. Every VR pydicom validates as a string appears here, and no
// other, which is the property that makes "we validate what pydicom validates"
// checkable rather than a claim.
//
// The VRs pydicom validates as something other than text are deliberately
// absent, and for the same reason: in Python a value is an arbitrary object, so
// validate_value has to ask "is this really an int?" and "is this really
// bytes?". By the time a godicom caller holds a value it is already the Go type
// the VR implies -- an int16 cannot hold 0x8000, a []byte cannot be a string --
// so porting validate_number and the OB/OW type checks would add a check that
// can never fail and a message no caller can act on.
//
// Each entry names the pydicom function it ports so the two tables can be read
// side by side.
var valueValidators = map[VR]valueValidator{
	VRAE: validateLengthAndPattern, // pydicom validate_length_and_type_and_regex
	VRAS: validatePattern,          // pydicom validate_type_and_regex
	VRCS: validateLengthAndPattern, // pydicom validate_length_and_type_and_regex
	VRDA: validateDateOrTime,       // pydicom validate_date_time
	VRDS: validateLengthAndPattern, // pydicom validate_length_and_type_and_regex
	VRDT: validateDateOrTime,       // pydicom validate_date_time
	VRIS: validateLengthAndPattern, // pydicom validate_length_and_type_and_regex
	VRLO: validateLength,           // pydicom validate_type_and_length
	VRLT: validateLength,           // pydicom validate_type_and_length
	VRPN: validatePersonName,       // pydicom validate_pn
	VRSH: validateLength,           // pydicom validate_type_and_length
	VRST: validateLength,           // pydicom validate_type_and_length
	VRTM: validateDateOrTime,       // pydicom validate_date_time
	VRUI: validateLengthAndPattern, // pydicom validate_length_and_type_and_regex
	VRUR: validatePattern,          // pydicom validate_type_and_regex
}

// maxValueLength is PS3.5 Table 6.2-1's maximum length per value, in
// characters. It is pydicom's MAX_VALUE_LEN entry for entry -- nine VRs, no
// more -- and the omissions carry meaning: LT, ST, UT, UC, UR and the binary
// VRs have no upper bound, and DA, DT, TM and AS follow from their shape rather
// than from a count, so PS3.5 gives them no number. A flat limit there would
// reject the range forms C.2.2.2.5 allows.
//
// The limits are per value, not per element: a multi-valued element holding
// three 16-character LOs is 48 bytes and perfectly legal. Callers that have
// already split on the backslash, as the writer does, can apply these as is.
var maxValueLength = map[VR]int{
	VRAE: 16,
	VRCS: 16,
	VRDS: 16,
	VRIS: 12,
	VRLO: 64,
	VRLT: 10240,
	VRSH: 16,
	VRST: 1024,
	VRUI: 64,
}

// valuePatterns is PS3.5 Table 6.2-1's character repertoire per VR, transcribed
// character for character from pydicom's VR_REGEXES so that a value accepted
// there is accepted here. An independent transcription is how the two drift,
// and drift here means godicom refuses files pydicom reads.
//
// The default-repertoire VRs (AE, AS, CS, DA, DS, DT, IS, TM, UI, UR) are
// ISO-IR 6 only; LO, LT, PN, SH, ST, UC and UT may use whatever (0008,0005)
// declares, so they have no pattern, only a length.
var valuePatterns = map[VR]*regexp.Regexp{
	VRAE: regexp.MustCompile(`^[\x20-\x7e]*$`),
	VRAS: regexp.MustCompile(`^\d\d\d[DWMY]$`),
	VRCS: regexp.MustCompile(`^[A-Z0-9 _]*$`),
	VRDS: regexp.MustCompile(`^ *[+\-]?(\d+|\d+\.\d*|\.\d+)([eE][+\-]?\d+)? *$`),
	VRIS: regexp.MustCompile(`^ *[+\-]?\d+ *$`),
	VRDA: rangePattern(`\d{4}(0[1-9]|1[0-2])([0-2]\d|3[01])`),
	VRDT: rangePattern(`\d{4}((0[1-9]|1[0-2])(([0-2]\d|3[01])(([01]\d|2[0-3])` +
		`([0-5]\d((60|[0-5]\d)(\.\d{1,6} ?)?)?)?)?)?)?([+-][01]\d\d\d)?`),
	VRTM: rangePattern(`([01]\d|2[0-3])([0-5]\d((60|[0-5]\d)(\.\d{1,6} ?)?)?)?`),
	VRUI: regexp.MustCompile(`^(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*))*$`),
	VRUR: regexp.MustCompile(`^[A-Za-z_\d:/?#\[\]@!$&'()*+,;=%\-.~]* *$`),
}

// rangePattern is pydicom's _range_regex: the pattern PS3.4 C.2.2.2.5 gives DA,
// DT and TM for a range of values -- "from", "to" or "from-to", each optionally
// followed by a space. Without it a window such as "20240101-20241231", which is
// how (0008,0021) Series Date and friends are legitimately filled, would be
// rejected as one malformed date.
func rangePattern(base string) *regexp.Regexp {
	return regexp.MustCompile(
		`^` + base + `$|^\-` + base + ` ?$|^` + base + `\- ?$|^` + base + `\-` + base + ` ?$`)
}

// ValidateValue checks one value against PS3.5 Table 6.2-1 for vr: length,
// allowed characters, and -- for DA, DT and TM -- the shape of the date or time,
// including the range forms. It is pydicom's validate_value with the
// validation_mode argument removed, for the reasons under "Reporting" below.
//
// value is one value, not an element's full contents: a multi-valued element has
// to be split on the backslash first, which is what the writer does and what an
// application validating user input should do. That is also how pydicom calls
// validate_value -- values.py splits on the backslash and validates each piece --
// so passing a whole "a\\b" here is not a pydicom behaviour we are declining to
// copy, it is a misuse of both.
//
// A nil return means the value is valid. A non-nil return is a
// *ValueValidationError. VRs with no text form are accepted unconditionally
// rather than reported: their correctness is the encoder's business, and asking
// this function about an OW is a category error rather than a finding.
//
// # Reporting
//
// pydicom takes a validation_mode (config.IGNORE, WARN or RAISE) because a pure
// function has no other way to say who acts on the verdict. Go does: the error
// is the verdict and the caller decides. The three pydicom modes are then three
// ways of handling it, and the third is not hypothetical -- it is what
// WriteOptions.OnDiagnostic already offers:
//
//	IGNORE  discard the error
//	WARN    hand it to OnDiagnostic and return nil from the handler
//	RAISE   hand it to OnDiagnostic -- returning the diagnostic aborts the write
//
// Adding a mode parameter here would be a second way to say that, and the second
// way is the one that drifts: the writer would have both a mode field and a
// handler, and they would disagree eventually.
func ValidateValue(vr VR, value string) error {
	validator, ok := valueValidators[vr]
	if !ok {
		return nil
	}

	// Padding is not value. PS3.5 pads every string value to an even length -- a
	// space for most VRs, a NUL for UI -- and a caller holding bytes read off the
	// wire would otherwise see every odd-length value fail. pydicom validates
	// before its own reader strips the padding, so it reports a value that is
	// spelled correctly; we strip first, because a checker that rejects the
	// writer's own output is worse than no checker.
	//
	// This is the one place the behaviour deliberately differs from pydicom, and
	// the difference can only be one character wide: a value that ends in padding
	// was at most one character over the limit once the padding is accounted for.
	trimmed := strings.TrimRight(value, " \x00")

	if reasons := validator(vr, trimmed); len(reasons) > 0 {
		return &ValueValidationError{VR: vr, Value: value, Reasons: reasons}
	}
	return nil
}

// hasValueValidator reports whether ValidateValue has anything to say about vr.
// Callers use it to skip work -- the writer skips the whole diagnostic path for
// a VR whose value cannot be checked -- and keeping the question in one place
// means the writer cannot fall out of step with the table.
func hasValueValidator(vr VR) bool {
	_, ok := valueValidators[vr]
	return ok
}

// validateLength applies pydicom's validate_vr_length: the per-value maximum
// from Table 6.2-1.
func validateLength(vr VR, value string) []string {
	maxLen, ok := maxValueLength[vr]
	if !ok {
		return nil
	}
	// Characters, not bytes: a 64-character LO of Chinese names is 192 bytes in
	// UTF-8 and still legal. The ASCII-only VRs never hold a multi-byte rune, so
	// counting runes is exact for both.
	if n := utf8.RuneCountInString(value); n > maxLen {
		return []string{fmt.Sprintf(
			"the value length (%d) exceeds the maximum length of %d allowed for VR %s",
			n, maxLen, vr)}
	}
	return nil
}

// validatePattern applies pydicom's validate_regex. An empty value is always
// valid: Table 6.2-1 permits zero length for every string VR, and "no value" is
// the commonest correct answer in a real dataset.
//
// The trailing-newline check pydicom performs by hand is not repeated here. It
// exists because Python's "$" also matches before a final newline, so "AB\n"
// passes its anchored pattern and has to be rejected separately; RE2 anchors at
// the end of the text, so the pattern already refuses it. The two end up
// agreeing, which is the only thing that matters.
func validatePattern(vr VR, value string) []string {
	if value == "" {
		return nil
	}
	pattern, ok := valuePatterns[vr]
	if !ok {
		return nil
	}
	if !pattern.MatchString(value) {
		return []string{fmt.Sprintf("invalid value for VR %s: %q; %s",
			vr, value, describeValuePattern(vr))}
	}
	return nil
}

// validateLengthAndPattern applies pydicom's
// validate_length_and_type_and_regex, which reports both faults rather than
// stopping at the first: a caller fixing a value should not have to run the
// check twice to learn everything wrong with it.
func validateLengthAndPattern(vr VR, value string) []string {
	return append(validateLength(vr, value), validatePattern(vr, value)...)
}

// validateDateOrTime applies pydicom's validate_date_time, whose only other
// branch accepts an already-parsed datetime.date/datetime and skips the check.
// Go has no such value at this boundary: DA, TM and DT arrive as the spelling
// they were read or set with, so the pattern is the whole check.
func validateDateOrTime(vr VR, value string) []string {
	return validatePattern(vr, value)
}

// validatePersonName applies pydicom's validate_pn: at most three component
// groups, at most 64 characters in each.
//
// This is not cosmetic. A receiver that parses the groups to find the family
// name puts the wrong string in the worklist when a fourth group shows up, and
// nothing about the file announces the problem.
func validatePersonName(vr VR, value string) []string {
	if value == "" {
		return nil
	}
	groups := strings.Split(value, "=")
	if len(groups) > 3 {
		return []string{fmt.Sprintf(
			"the number of PN component groups (%d) exceeds the maximum allowed number of 3", len(groups))}
	}
	for _, group := range groups {
		if n := utf8.RuneCountInString(group); n > 64 {
			return []string{fmt.Sprintf(
				"the PN component length (%d) exceeds the maximum allowed length of 64", n)}
		}
	}
	return nil
}

// describeValuePattern explains what the VR expects, in the terms the standard
// uses. pydicom closes its message with a link to Table 6.2-1; naming the rule is
// shorter and does not rot when the standard is republished.
func describeValuePattern(vr VR) string {
	switch vr {
	case VRAE:
		return "only printable ASCII (0x20-0x7E) is allowed"
	case VRAS:
		return `expected 3 digits followed by D, W, M or Y, such as "032Y"`
	case VRCS:
		return "only uppercase A-Z, digits, space and underscore are allowed"
	case VRDS:
		return `expected a decimal string of at most 16 characters, such as "1.5" or "-2.5e-3"`
	case VRIS:
		return "expected an integer string of at most 12 characters"
	case VRDA:
		return `expected YYYYMMDD, or a range such as "20240101-20241231"`
	case VRDT:
		return "expected YYYYMMDDHHMMSS.FFFFFF&ZZXX, or a range of two such values"
	case VRTM:
		return `expected HHMMSS.FFFFFF, or a range such as "0800-1200"`
	case VRUI:
		return "expected dot-separated components of digits without leading zeros, such as \"1.2.840.10008\""
	case VRUR:
		return "expected a URI using only the characters RFC 3986 allows"
	}
	return "the value does not have the form the standard gives this VR"
}

// valueAllowsBackslash reports whether a VR's values may themselves contain the
// backslash that otherwise separates the values of a multi-valued element.
// PS3.5 Table 6.2-1 allows it for the unlimited-text VRs, where a backslash is
// just a character, so those have to be validated whole instead of split.
func valueAllowsBackslash(vr VR) bool {
	switch vr {
	case VRLT, VRST, VRUT, VRUC:
		return true
	}
	return false
}
