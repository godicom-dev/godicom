package godicom

// The write side of the per-VR validation added in validvalue.go: the rules
// themselves are covered in validvalue_test.go, and these cases prove the writer
// actually routes through them, reports against the right tag, and still writes
// what the caller asked for unless the hook refuses.

import (
	"bytes"
	"strings"
	"testing"
)

// A lowercase CS reaches the file today with nothing said. It is a value no
// strict receiver accepts -- PS3.5 defines CS as uppercase A-Z, digits, space
// and underscore -- so the hook gets to see it.
func TestWriteReportsLowercaseCS(t *testing.T) {
	t.Parallel()
	tg := MustTag("Modality")
	ds := NewDataset()
	_ = ds.SetString(tg, "ct") // "CT" is the legal spelling

	rec := &diagRecorder{}
	if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
		t.Fatalf("EncodeFile() error = %v", err)
	}

	d := wantOneInvalidValue(t, rec, tg, VRCS, "uppercase")
	verr, ok := d.Err.(*ValueValidationError)
	if !ok {
		t.Fatalf("cause is %T, want *ValueValidationError", d.Err)
	}
	if verr.Value != "ct" {
		t.Errorf("cause carries value %q, want %q", verr.Value, "ct")
	}
}

// The same value with the hook refusing becomes a write failure, which is the
// RAISE half of pydicom's writing_validation_mode.
func TestWriteRefusesInvalidCSWhenHookRejects(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	_ = ds.SetString(MustTag("Modality"), "ct")

	rec := &diagRecorder{reject: true}
	if _, err := writeWithDiagnostics(t, ds, rec); err == nil {
		t.Fatal("EncodeFile() accepted a rejected diagnostic")
	}
}

// A legal value must stay silent: a validator that fires on correct input is
// worse than none, because it trains the caller to ignore it.
func TestWriteStaysSilentOnValidValues(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	_ = ds.SetString(MustTag("Modality"), "CT")
	_ = ds.SetString(MustTag("Manufacturer"), "ACME Medical Systems") // LO, 20 chars
	_ = ds.SetString(MustTag("StudyDate"), "20240101")
	_ = ds.SetString(MustTag("StudyDate"), "20240101-20241231")
	_ = ds.SetString(MustTag("StudyTime"), "0800-1200")
	_ = ds.SetString(MustTag("SOPInstanceUID"), "1.2.840.10008.5.1.4.1.1.2")
	_ = ds.SetString(MustTag("InstitutionName"), strings.Repeat("洪", 20)) // 20 chars, 60 bytes
	_ = ds.SetString(MustTag("ReferringPhysicianName"), "Yamada^Tarou=山田^太郎")

	rec := &diagRecorder{}
	if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
		t.Fatalf("EncodeFile() error = %v", err)
	}
	if len(rec.got) != 0 {
		t.Fatalf("reported %d diagnostics on valid values: %v", len(rec.got), rec.got)
	}
}

// A UID with a leading zero in a component is not a UID a receiver can compare
// against, and it is exactly the kind of thing a hand-built dataset contains.
func TestWriteReportsMalformedUID(t *testing.T) {
	t.Parallel()
	tg := MustTag("SOPInstanceUID")
	ds := NewDataset()
	_ = ds.SetString(tg, "1.2.840.10008.01.2")

	rec := &diagRecorder{}
	if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
		t.Fatalf("EncodeFile() error = %v", err)
	}
	wantOneInvalidValue(t, rec, tg, VRUI, "leading zero")
}

// A DS handed in as an 18-character string is over the 16 PS3.5 allows. The
// existing number-string check already caught this; the per-VR validator must
// not report it a second time under a different cause.
func TestWriteReportsOversizedDSExactlyOnce(t *testing.T) {
	t.Parallel()
	tg := MustTag("PatientWeight")
	ds := NewDataset()
	_ = ds.SetString(tg, "0.3333333333333333") // 18 characters

	rec := &diagRecorder{}
	if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
		t.Fatalf("EncodeFile() error = %v", err)
	}
	wantOneInvalidValue(t, rec, tg, VRDS, "over the 16")
}

// A PN with four component groups is not a name the standard describes, and the
// failure is not cosmetic: a receiver that parses the groups to find the family
// name puts the wrong string in the worklist.
func TestWriteReportsPNWithTooManyGroups(t *testing.T) {
	t.Parallel()
	tg := MustTag("PatientName")
	ds := NewDataset()
	_ = ds.SetString(tg, "Jim=John=Jimmy=Jonny")

	rec := &diagRecorder{}
	if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
		t.Fatalf("EncodeFile() error = %v", err)
	}
	wantOneInvalidValue(t, rec, tg, VRPN, "component groups")
}

// Every value of a multi-valued element is validated on its own: the third
// value is the bad one, and the first two must not produce diagnostics.
func TestWriteReportsPerValueOfMultiValuedElement(t *testing.T) {
	t.Parallel()
	tg := MustTag("ImageType")
	ds := NewDataset()
	_ = ds.SetStrings(tg, "ORIGINAL", "PRIMARY", "AXIAL-3D") // hyphen is not a CS character

	rec := &diagRecorder{}
	if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
		t.Fatalf("EncodeFile() error = %v", err)
	}

	d := wantOneInvalidValue(t, rec, tg, VRCS, "uppercase")
	var verr *ValueValidationError
	if !errorsAsValueValidation(d.Err, &verr) {
		t.Fatalf("cause is %T, want *ValueValidationError", d.Err)
	}
	if verr.Value != "AXIAL-3D" {
		t.Errorf("cause carries value %q, want the third value only", verr.Value)
	}
}

// With no hook the writer must still produce the same bytes it always did: the
// validation is information, not a change of behaviour.
func TestWriteInvalidValueIsUnchangedWithoutHook(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	_ = ds.SetString(MustTag("Modality"), "ct")

	without, err := writeWithDiagnostics(t, ds, nil)
	if err != nil {
		t.Fatalf("EncodeFile() with no hook error = %v", err)
	}

	rec := &diagRecorder{}
	with, err := writeWithDiagnostics(t, ds, rec)
	if err != nil {
		t.Fatalf("EncodeFile() with a hook error = %v", err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("reported %d diagnostics, want 1", len(rec.got))
	}
	if !bytes.Equal(without, with) {
		t.Fatal("setting OnDiagnostic changed the encoded bytes")
	}
}

// errorsAsValueValidation is errors.As with the concrete target spelled out,
// kept local so the assertions above read as one line.
func errorsAsValueValidation(err error, target **ValueValidationError) bool {
	for err != nil {
		if v, ok := err.(*ValueValidationError); ok {
			*target = v
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
