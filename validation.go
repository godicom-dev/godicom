package godicom

import "errors"

// ValidationMode is pydicom's IGNORE / WARN / RAISE choice, spelled as an enum
// rather than left to a hook's return value. The zero value is the mode godicom
// has always run in, and the modes are opt-in: a caller re-reading a file it
// did not create should not pay for checks it never asked for, which is the
// same reason the write side spent its first releases with the choice spelled
// by the OnDiagnostic hook's presence and return value alone.
//
//	ValidationDefault -- the historical behaviour, which differs by side. On a
//	read, no value validation at all, the quiet path every existing caller
//	gets. On a write, every value checked and every rejection offered to
//	OnDiagnostic, the value written when the hook returns nil.
//	ValidationIgnore -- no validation. On a write this skips the checks
//	entirely, which is what a caller round-tripping a file at speed pays for.
//	ValidationWarn -- invalid values are offered to OnDiagnostic as
//	DiagnosticInvalidValue (the write side has always done this; the read side
//	gains it here), and the read or write carries on when the hook returns
//	nil. A hook that returns the diagnostic makes Warn as strict as Raise,
//	which is the composition pydicom cannot spell: a mode that a per-call
//	hook can sharpen.
//	ValidationRaise -- the first invalid value fails the whole read or write
//	with its Diagnostic, hook or no hook. pydicom's RAISE, which raises
//	ValueError out of dcmread; here the Diagnostic is the error, and
//	errors.As to it reaches the Tag, VR and reason.
type ValidationMode uint8

const (
	// ValidationDefault is the zero value: no value validation on a read, and
	// check-and-offer on a write.
	ValidationDefault ValidationMode = iota
	// ValidationIgnore skips value validation entirely.
	ValidationIgnore
	// ValidationWarn reports invalid values through OnDiagnostic and carries
	// on when the hook returns nil.
	ValidationWarn
	// ValidationRaise fails the whole read or write on the first invalid
	// value.
	ValidationRaise
)

// validateElementValue offers the converted value of one element to the PS3.5
// Table 6.2-1 checks, the way pydicom's DataElement validation_mode does on
// the values a read produces. Only the text VRs have anything to say -- the
// numeric and byte VRs cannot be wrong in Go the way they can in Python, a
// property the validator table already documents -- so everything else passes
// without a check.
//
// Warn offers each rejection to OnDiagnostic and keeps the value; Raise turns
// the first rejection into the error the read fails with, which is why the
// path and offset a report would have carried are filled in by hand. A
// deferred reload has no read context and no mode, and keeps the historical
// fallback; so does the character-set reinterpretation pass, which re-decodes
// bytes that already converted once and carries its own error handling.
func (rc *readContext) validateElementValue(elem *DataElement, vr VR, valueStart int64) error {
	if rc == nil || rc.valueValidation == ValidationDefault || rc.valueValidation == ValidationIgnore {
		return nil
	}
	if !hasValueValidator(vr) {
		return nil
	}
	parts, ok := stringValueParts(elem)
	if !ok {
		return nil
	}
	for _, part := range parts {
		err := ValidateValue(vr, part)
		if err == nil {
			continue
		}
		var verr *ValueValidationError
		if !errors.As(err, &verr) {
			return err
		}
		d := Diagnostic{
			Kind:   DiagnosticInvalidValue,
			Tag:    elem.Tag,
			VR:     vr,
			Offset: valueStart,
			Err:    verr,
		}
		if rc.valueValidation == ValidationRaise {
			if len(rc.seqPath) > 0 {
				d.Path = append([]PathStep(nil), rc.seqPath...)
			}
			d.Offset += rc.baseOffset
			return d
		}
		if err := rc.report(d); err != nil {
			return err
		}
	}
	return nil
}
