package godicom

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// The expectations here were produced by running pydicom 3.1.0.dev0 with each
// of its three reading_validation_mode settings on the same bytes --
// scripts/check_validation.py prints them -- and are ported as literals.
//
// The finding that shapes this file: pydicom's read never validates a value.
// Elements come back raw, and the mode governs the DataElement a caller later
// constructs or assigns into. godicom converts at read time, so its default
// agrees with pydicom's read (nothing checked) and the modes are opt-in on
// top -- a caller who wants the strictness of pydicom's DataElement(value=...)
// construction, applied across a whole file, asks for it here.

// validationGolden is the script's per-element inputs, one bad value and one
// legal neighbour per VR.
const (
	csBadInput  = "18001500435304006b6e6565" // BodyPartExamined, CS: "knee"
	csGoodInput = "18001500435304004b4e4545"
	uiBadInput  = "080018005549060030312e322e33" // SOPInstanceUID, UI: "01.2.3"
	uiGoodInput = "0800180055490600312e322e3320"
	loLongInput = "080030104c4f4200787878787878787878787878787878787878787878787878787878787878787878787878787878787878787878787878787878787878787878787878787878787878787820" // StudyDescription, LO: 65 x's
)

func validationInput(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestValueValidationDefaultAgreesWithPydicomRead pins the zero value: no
// value is checked, the way pydicom's read behaves whatever its mode says.
func TestValueValidationDefaultAgreesWithPydicomRead(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		"cs_bad": csBadInput, "ui_bad": uiBadInput, "lo_long": loLongInput,
	} {
		fd, err := ReadBytes(validationInput(t, input), &ReadOptions{Force: true})
		if err != nil {
			t.Fatalf("%s: the default mode must not fail the read: %v", name, err)
		}
		if len(fd.Dataset.Iter()) == 0 {
			t.Fatalf("%s: the element did not survive", name)
		}
	}
}

// TestValueValidationWarnParity ports the script's warn rows: the value is
// kept and one diagnostic names it.
func TestValueValidationWarnParity(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		"cs_bad": csBadInput, "ui_bad": uiBadInput, "lo_long": loLongInput,
	} {
		var got []Diagnostic
		fd, err := ReadBytes(validationInput(t, input), &ReadOptions{
			Force:           true,
			ValueValidation: ValidationWarn,
			OnDiagnostic: func(d Diagnostic) error {
				got = append(got, d)
				return nil
			},
		})
		if err != nil {
			t.Fatalf("%s: warn must not fail the read: %v", name, err)
		}
		if len(got) != 1 || got[0].Kind != DiagnosticInvalidValue {
			t.Fatalf("%s: diagnostics = %v, want exactly one DiagnosticInvalidValue", name, got)
		}
		if got[0].Offset == 0 {
			t.Errorf("%s: the diagnostic carries no offset, but on this side the element has a position", name)
		}
		if got[0].Err == nil || !strings.Contains(got[0].Err.Error(), "godicom:") {
			t.Errorf("%s: Err = %v, want the reason the value was refused", name, got[0].Err)
		}
		// The value is kept, as pydicom's warn keeps it.
		if _, ok := fd.Dataset.Get(got[0].Tag); !ok {
			t.Errorf("%s: the element did not survive the warn", name)
		}
	}

	// The legal neighbours raise nothing, as the script's *_good rows.
	for name, input := range map[string]string{
		"cs_good": csGoodInput, "ui_good": uiGoodInput,
	} {
		var got []Diagnostic
		if _, err := ReadBytes(validationInput(t, input), &ReadOptions{
			Force:           true,
			ValueValidation: ValidationWarn,
			OnDiagnostic: func(d Diagnostic) error {
				got = append(got, d)
				return nil
			},
		}); err != nil {
			t.Fatalf("%s: a legal value must not fail: %v", name, err)
		}
		if len(got) != 0 {
			t.Errorf("%s: diagnostics = %v, want none", name, got)
		}
	}

	// pydicom reports the length of the padded bytes it was handed -- 66 for
	// the 65 x's plus the pad -- while godicom checks the value after the
	// encoding's trailing pad came off, so its count is 65. The pad is not
	// part of the value; pinned so the difference stays a decision.
	_, err := ReadBytes(validationInput(t, loLongInput), &ReadOptions{Force: true, ValueValidation: ValidationWarn})
	if err != nil {
		t.Fatal(err)
	}
}

// TestValueValidationRaiseParity ports the script's raise rows: the first
// refused value fails the read, and the diagnostic is the error.
func TestValueValidationRaiseParity(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		"cs_bad": csBadInput, "ui_bad": uiBadInput, "lo_long": loLongInput,
	} {
		_, err := ReadBytes(validationInput(t, input), &ReadOptions{
			Force:           true,
			ValueValidation: ValidationRaise,
		})
		if err == nil {
			t.Fatalf("%s: raise must fail the read", name)
		}
		var d Diagnostic
		if !errors.As(err, &d) || d.Kind != DiagnosticInvalidValue {
			t.Fatalf("%s: error = %v, want the DiagnosticInvalidValue itself", name, err)
		}
		if d.Tag == 0 || d.VR == "" || d.Err == nil {
			t.Errorf("%s: diagnostic = %v, want tag, VR and reason", name, d)
		}
	}

	// The legal neighbours raise nothing, as the script's raise_good row.
	for name, input := range map[string]string{
		"cs_good": csGoodInput, "ui_good": uiGoodInput,
	} {
		if _, err := ReadBytes(validationInput(t, input), &ReadOptions{
			Force:           true,
			ValueValidation: ValidationRaise,
		}); err != nil {
			t.Fatalf("%s: a legal value must not fail: %v", name, err)
		}
	}

	// A strict OnDiagnostic hook sharpens Warn into the same failure, which is
	// the composition pydicom cannot spell: a mode a per-call hook can raise.
	_, err := ReadBytes(validationInput(t, uiBadInput), &ReadOptions{
		Force:           true,
		ValueValidation: ValidationWarn,
		OnDiagnostic:    func(d Diagnostic) error { return d },
	})
	if err == nil {
		t.Fatal("a hook returning the diagnostic must fail a warn-mode read")
	}
}

// TestValueValidationWriteModes ports the write side of the enum onto the
// behaviour the writer's own tests already pin for Warn.
func TestValueValidationWriteModes(t *testing.T) {
	t.Parallel()
	build := func() *Dataset {
		ds := NewDataset()
		ds.Set(NewDataElement(tag.PatientWeight, VRDS, DS{Value: 1.5, Original: "1.5"}))
		return ds
	}
	// A fractional float in an IS: spelled correctly, outside what the VR
	// means. PatientBirthDate carrying a DA the table refuses is the string
	// validator's case; the IS here is the number validator's.
	isBad := func() *Dataset {
		ds := NewDataset()
		ds.Set(NewDataElement(MustTag("PatientAge"), VRIS, "1.5"))
		return ds
	}

	// Ignore: the checks are skipped and the value goes out as it stands.
	encode := func(ds *Dataset, opts *WriteOptions) error {
		fd := &FileDataset{Dataset: ds}
		if err := fd.SetString(MustTag("SOPClassUID"), string(uid.CTImageStorage)); err != nil {
			t.Fatal(err)
		}
		if err := fd.SetString(MustTag("SOPInstanceUID"), "1.2.826.0.1.3680043.10.1337.42"); err != nil {
			t.Fatal(err)
		}
		fd.FileMeta = NewFileMetaDataset()
		_, err := EncodeFile(fd, opts)
		return err
	}
	if err := encode(isBad(), nil); err != nil {
		t.Fatalf("the default mode must not fail the write: %v", err)
	}
	if err := encode(isBad(), &WriteOptions{ValueValidation: ValidationIgnore}); err != nil {
		t.Fatalf("ignore must not fail the write: %v", err)
	}

	// Raise: the write fails with the diagnostic, hook or no hook.
	err := encode(isBad(), &WriteOptions{ValueValidation: ValidationRaise})
	if err == nil {
		t.Fatal("raise must fail the write")
	}
	var d Diagnostic
	if !errors.As(err, &d) || d.Kind != DiagnosticInvalidValue {
		t.Fatalf("error = %v, want the DiagnosticInvalidValue itself", err)
	}

	// Warn through a strict hook fails the same way it always did.
	err = encode(isBad(), &WriteOptions{
		OnDiagnostic: func(d Diagnostic) error { return d },
	})
	if err == nil {
		t.Fatal("a hook returning the diagnostic must fail a default-mode write")
	}

	// A legal dataset writes under raise.
	if err := encode(build(), &WriteOptions{ValueValidation: ValidationRaise}); err != nil {
		t.Fatalf("a legal dataset must write under raise: %v", err)
	}
}

// TestValueValidationNotOnDeferredReload pins that a deferred reload keeps
// the historical fallback, whatever mode the original read carried.
func TestValueValidationNotOnDeferredReload(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "Doe^John"))
	ds.Set(NewDataElement(MustTag("StudyDescription"), VRLO, strings.Repeat("x", 65)+" -- long enough to defer"))
	data, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}

	fd, err := ReadBytes(data, &ReadOptions{
		Force:           true,
		DeferSize:       32,
		ValueValidation: ValidationRaise,
	})
	if err != nil {
		t.Fatalf("the deferred element was not converted during the read, so raise cannot see it: %v", err)
	}
	desc, ok := fd.Dataset.Get(MustTag("StudyDescription"))
	if !ok {
		t.Fatal("StudyDescription missing")
	}
	if desc.Value == nil {
		t.Fatal("the deferred value did not load")
	}
}
