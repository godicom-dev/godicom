package godicom

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// These tests pin the port itself rather than a single VR's rules: which VRs the
// validator table covers, and the three places where a faithful transcription
// needed a decision. If someone later "simplifies" one of them, the comment and
// the test go together.

// pydicomStringVRs is VALIDATORS in pydicom/valuerep.py, filtered to the entries
// that validate a str. The rest -- FD, FL, SL, SS, SV, UL, US, UV, OB, OD, OF,
// OL, OW, OV -- validate a Python object's type or numeric range, which is a
// question Go's type system has already answered by the time a value exists.
var pydicomStringVRs = []VR{
	VRAE, VRAS, VRCS, VRDA, VRDS, VRDT, VRIS,
	VRLO, VRLT, VRPN, VRSH, VRST, VRTM, VRUI, VRUR,
}

func TestValueValidatorTableMatchesPydicom(t *testing.T) {
	var have []string
	for vr := range valueValidators {
		have = append(have, string(vr))
	}
	want := make([]string, len(pydicomStringVRs))
	for i, vr := range pydicomStringVRs {
		want[i] = string(vr)
	}
	slices.Sort(have)
	slices.Sort(want)

	if !slices.Equal(have, want) {
		t.Fatalf("validator table covers %v, want %v (pydicom VALIDATORS' string entries)", have, want)
	}
}

func TestValueValidatorTableEntryIsUsed(t *testing.T) {
	// A VR in the table with a validator that accepts everything would make the
	// table longer without making the library stricter. Each entry has to have a
	// value it refuses, or it does not belong.
	refusedBy := map[VR]string{
		VRAE: "café",
		VRAS: "032X",
		VRCS: "lower",
		VRDA: "2024",
		VRDS: "1,5",
		VRDT: "2024-01-01",
		VRIS: "1.5",
		VRLO: strings.Repeat("x", 65),
		VRLT: strings.Repeat("x", 10241),
		VRPN: strings.Repeat("x", 65),
		VRSH: strings.Repeat("x", 17),
		VRST: strings.Repeat("x", 1025),
		VRTM: "2460",
		VRUI: "1..2",
		VRUR: "no spaces\x01",
	}

	for vr := range valueValidators {
		value, ok := refusedBy[vr]
		if !ok {
			t.Fatalf("VR %s is in the table but the test has no value it should refuse", vr)
		}
		if err := ValidateValue(vr, value); err == nil {
			t.Errorf("ValidateValue(%s, %q) = nil, want the value refused", vr, value)
		}
	}
}

func TestValidateValueRejectsTrailingNewline(t *testing.T) {
	// pydicom rejects a trailing newline with an explicit check, because Python's
	// "$" also matches just before a final newline. RE2 anchors at the end of the
	// text, so the pattern refuses it on its own -- but only as long as nobody
	// adds (?m) to these expressions. The outcome is what has to stay equal.
	for _, tc := range []struct {
		vr    VR
		value string
	}{
		{VRCS, "ABC\n"},
		{VRUI, "1.2.3\n"},
		{VRDS, "1.5\n"},
		{VRDT, "20240101\n"},
		{VRUR, "http://example.com\n"},
	} {
		if err := ValidateValue(tc.vr, tc.value); err == nil {
			t.Errorf("ValidateValue(%s, %q) = nil, want a trailing newline refused", tc.vr, tc.value)
		}
	}
}

func TestValidateValueStripsPaddingBeforeChecking(t *testing.T) {
	// The one deliberate difference from pydicom: it validates before stripping
	// the padding its own reader leaves in place. Padding is not value, and a
	// checker that rejects the bytes it just read back is a checker callers turn
	// off. UI is the sharp case -- PS3.5 pads it with NUL, which no VR pattern
	// allows -- so it is the case this test exists for.
	for _, tc := range []struct {
		name  string
		vr    VR
		value string
	}{
		{"ui_padded_with_nul", VRUI, "1.2.840.10008.1.2\x00"},
		{"ui_padded_with_space", VRUI, "1.2.840.10008.1.2 "},
		{"ds_padded_with_space", VRDS, "-1.5 "},
		{"cs_padded_with_nul", VRCS, "ORIGINAL\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateValue(tc.vr, tc.value); err != nil {
				t.Fatalf("ValidateValue(%s, %q) = %v, want the padding ignored",
					tc.vr, tc.value, err)
			}
		})
	}

	// Padding being ignored must not turn into a longer value being accepted: the
	// limit still applies to what is left.
	if err := ValidateValue(VRUI, "1.2.3"+strings.Repeat(".4", 40)+" "); err == nil {
		t.Fatal("a value over 64 characters was accepted because it ended in padding")
	}
}

func TestValidateValueReportsLengthAndPatternTogether(t *testing.T) {
	// pydicom's validate_length_and_type_and_regex joins both messages instead of
	// stopping at the first. Failing both is the common case for a hand-built
	// value, and making the caller run the check twice to see the second fault
	// would be a strange thing to inherit.
	value := strings.Repeat("a", 20) // 20 characters, and lowercase: two faults
	err := ValidateValue(VRCS, value)
	if err == nil {
		t.Fatalf("ValidateValue(CS, %q) = nil, want both faults reported", value)
	}
	var verr *ValueValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error is %T, want *ValueValidationError", err)
	}
	if len(verr.Reasons) != 2 {
		t.Fatalf("Reasons = %q, want the length fault and the character fault separately",
			verr.Reasons)
	}
	for _, want := range []string{"length (20)", "invalid value for VR CS"} {
		if !strings.Contains(strings.Join(verr.Reasons, " "), want) {
			t.Errorf("Reasons = %q, want one of them to mention %q", verr.Reasons, want)
		}
	}
}

func TestValidateValueAcceptsImpossibleCalendarDates(t *testing.T) {
	// Both sides accept 31 February: the pattern bounds the day to 01-31 and the
	// month to 01-12, and neither checks the two against each other. Pinned
	// rather than fixed -- a value that came from another system has to be
	// rewritable, and refusing it here would fail a file the DICOM standard lets
	// a sender produce. The month and day shapes are still enforced.
	if err := ValidateValue(VRDA, "20240231"); err != nil {
		t.Errorf("ValidateValue(DA, %q) = %v, want the day range accepted as pydicom does", "20240231", err)
	}
	for _, bad := range []string{"20241301", "20240001", "20240132"} {
		if err := ValidateValue(VRDA, bad); err == nil {
			t.Errorf("ValidateValue(DA, %q) = nil, want the month or day range refused", bad)
		}
	}
}

func TestValidateValueSkipsVRsWithNoSpelling(t *testing.T) {
	// UT and UC have neither a length limit nor a pattern in Table 6.2-1, and the
	// numeric and binary VRs carry no text at all. Asking about them is not a
	// finding, and the writer relies on this to skip the diagnostic path — a busy
	// dataset is mostly numbers.
	for _, vr := range []VR{VRUT, VRUC, VRUS, VRSS, VROB, VRUN, VRAT} {
		if hasValueValidator(vr) {
			t.Errorf("hasValueValidator(%s) = true, want false", vr)
		}
		if err := ValidateValue(vr, "\x00\x01 not text at all"); err != nil {
			t.Errorf("ValidateValue(%s, ...) = %v, want nil", vr, err)
		}
	}
}
