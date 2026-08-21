package godicom

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// writeWithDiagnostics encodes ds through the *WriteOptions path, the only one
// that can carry a hook. A nil rec writes with no hook at all, which is what
// every caller written before WriteOptions.OnDiagnostic existed gets.
func writeWithDiagnostics(t *testing.T, ds *Dataset, rec *diagRecorder) ([]byte, error) {
	t.Helper()
	opts := &WriteOptions{}
	if rec != nil {
		opts.OnDiagnostic = rec.hook
	}
	return EncodeFile(&FileDataset{Dataset: ds}, opts)
}

// wantOneInvalidValue fails unless rec holds exactly one DiagnosticInvalidValue
// for tag/vr whose cause mentions substr, and returns it.
func wantOneInvalidValue(t *testing.T, rec *diagRecorder, tag Tag, vr VR, substr string) Diagnostic {
	t.Helper()
	if len(rec.got) != 1 {
		t.Fatalf("reported %d diagnostics, want 1: %v", len(rec.got), rec.got)
	}
	d := rec.got[0]
	if d.Kind != DiagnosticInvalidValue {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticInvalidValue)
	}
	if d.Tag != tag {
		t.Errorf("Tag = %s, want %s", d.Tag, tag)
	}
	if d.VR != vr {
		t.Errorf("VR = %s, want %s", d.VR, vr)
	}
	if d.Err == nil {
		t.Fatal("the diagnostic carries no cause")
	}
	if !strings.Contains(d.Err.Error(), substr) {
		t.Errorf("cause = %q, want it to mention %q", d.Err, substr)
	}
	// Nothing was read, so there is no offset to point at.
	if d.Offset != 0 {
		t.Errorf("Offset = %d, want 0 for a write diagnostic", d.Offset)
	}
	return d
}

// A float64 in an IS is formatted "1.5", which no IS parser accepts. The writer
// used to emit it with nothing said anywhere. It still emits it -- there is no
// correct integer spelling to fall back on, and rounding would silently change
// the value -- but the hook now gets the chance to refuse.
func TestWriteReportsFractionalIS(t *testing.T) {
	t.Parallel()
	tg := MustTag("EchoNumbers")
	build := func() *Dataset {
		ds := NewDataset()
		ds.Set(NewDataElement(tg, VRIS, 1.5))
		return ds
	}

	// No hook: byte for byte what godicom wrote before the hook existed.
	silent, err := writeWithDiagnostics(t, build(), nil)
	if err != nil {
		t.Fatalf("a write with no hook must still succeed: %v", err)
	}
	if !bytes.Contains(silent, []byte("1.5")) {
		t.Fatal("the IS value is missing from a hookless write")
	}

	// A hook returning nil observes it and changes nothing.
	rec := &diagRecorder{}
	observed, err := writeWithDiagnostics(t, build(), rec)
	if err != nil {
		t.Fatalf("a hook returning nil must not fail the write: %v", err)
	}
	if !bytes.Equal(observed, silent) {
		t.Error("observing a diagnostic changed the bytes written")
	}
	wantOneInvalidValue(t, rec, tg, VRIS, `"1.5" is not an integer string`)

	// Returning the diagnostic fails the write with it.
	reject := &diagRecorder{reject: true}
	_, err = writeWithDiagnostics(t, build(), reject)
	if err == nil {
		t.Fatal("a hook returning the diagnostic must fail the write")
	}
	var d Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("write error does not unwrap to a Diagnostic: %v", err)
	}
	if d.Kind != DiagnosticInvalidValue {
		t.Errorf("failed with Kind %q, want %q", d.Kind, DiagnosticInvalidValue)
	}
}

// PS3.5 caps a DS at 16 bytes. A DS parsed from a file keeps whatever string the
// file held so it round-trips byte for byte (TestDSFromFileKeepsOverlongOriginal),
// which means re-encoding one can put an over-long DS back on the wire. Those
// bytes must not change -- but the caller is now told.
func TestWriteReportsOverlongDS(t *testing.T) {
	t.Parallel()
	const overlong = "0.3333333333333333" // 18 bytes, as some scanners emit
	tg := MustTag("SliceThickness")
	val, err := ParseDS(overlong)
	if err != nil {
		t.Fatal(err)
	}
	ds := NewDataset()
	ds.Set(NewDataElement(tg, VRDS, val))

	rec := &diagRecorder{}
	encoded, err := writeWithDiagnostics(t, ds, rec)
	if err != nil {
		t.Fatalf("a hook returning nil must not fail the write: %v", err)
	}
	if !bytes.Contains(encoded, []byte(overlong)) {
		t.Errorf("the over-long DS did not survive the write; byte fidelity is the point of keeping Original")
	}
	wantOneInvalidValue(t, rec, tg, VRDS, "18 bytes, over the 16")
}

// IS has a range PS3.5 gives the VR but its spelling does not: "3000000000" is a
// well-formed integer string that no IS may hold. ISInRange already knew this;
// the writer did not consult it.
func TestWriteReportsISOutOfRange(t *testing.T) {
	t.Parallel()
	tg := MustTag("EchoNumbers")
	ds := NewDataset()
	ds.Set(NewDataElement(tg, VRIS, 3000000000))

	rec := &diagRecorder{}
	if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
		t.Fatalf("a hook returning nil must not fail the write: %v", err)
	}
	wantOneInvalidValue(t, rec, tg, VRIS, "outside [-2147483648, 2147483647]")
}

// Everything a DS or an IS may legitimately hold must reach the file without a
// word, or the hook is noise a caller learns to ignore.
func TestWriteReportsNothingForValidNumberStrings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		tag   string
		vr    VR
		value any
	}{
		{"DS from a float64", "SliceThickness", VRDS, 1.0 / 3.0},
		{"DS as a string", "SliceThickness", VRDS, "1.5"},
		{"DS in scientific notation", "SliceThickness", VRDS, "1.234e-5"},
		{"DS at exactly 16 bytes", "SliceThickness", VRDS, "1.23456789012345"},
		{"DS multi-value", "PixelSpacing", VRDS, NewMultiValue([]float64{1.0 / 3.0, 2.0 / 3.0})},
		// PS3.5 allows an absent value in a multi-valued element, and pydicom's
		// validators skip an empty one too, so neither may be reported.
		{"DS multi-value with an empty part", "PixelSpacing", VRDS, `1.0\\3.0`},
		{"IS from an int", "EchoNumbers", VRIS, 42},
		{"IS at the low bound", "EchoNumbers", VRIS, -2147483648},
		{"IS at the high bound", "EchoNumbers", VRIS, 2147483647},
		{"IS with a sign", "EchoNumbers", VRIS, "+5"},
		{"IS multi-value", "EchoNumbers", VRIS, NewMultiValue([]int{1, 2, 3})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ds := NewDataset()
			ds.Set(NewDataElement(MustTag(tc.tag), tc.vr, tc.value))
			rec := &diagRecorder{reject: true}
			if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
				t.Fatalf("write failed: %v", err)
			}
			if len(rec.got) != 0 {
				t.Errorf("reported %v", rec.got)
			}
		})
	}
}

// A value inside a sequence item must name the sequences enclosing it, the way a
// parse diagnostic does. Defined-length and undefined-length sequences are
// written by different loops, so both are checked.
func TestWriteDiagnosticCarriesSequencePath(t *testing.T) {
	t.Parallel()
	seqTag := MustTag("ReferencedImageSequence")
	inner := MustTag("EchoNumbers")

	for _, undefined := range []bool{false, true} {
		name := "defined length"
		if undefined {
			name = "undefined length"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			item := NewDataset()
			item.Set(NewDataElement(inner, VRIS, 1.5))
			elem := NewDataElement(seqTag, VRSQ, NewSequence([]*Dataset{item}))
			elem.IsUndefinedLength = undefined
			ds := NewDataset()
			ds.Set(elem)

			rec := &diagRecorder{}
			if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
				t.Fatalf("write failed: %v", err)
			}
			d := wantOneInvalidValue(t, rec, inner, VRIS, "not an integer string")
			if len(d.Path) != 1 || d.Path[0] != seqTag {
				t.Errorf("Path = %v, want [%s]", d.Path, seqTag)
			}
		})
	}
}

// The path must be popped on the way out, or an element after a sequence inherits
// the sequence it is not in.
func TestWriteDiagnosticPathIsPoppedAfterASequence(t *testing.T) {
	t.Parallel()
	seqTag := MustTag("ReferencedImageSequence")
	inner := MustTag("EchoNumbers")
	after := MustTag("InstanceNumber")

	item := NewDataset()
	item.Set(NewDataElement(inner, VRIS, 1.5))
	ds := NewDataset()
	ds.Set(NewDataElement(seqTag, VRSQ, NewSequence([]*Dataset{item})))
	ds.Set(NewDataElement(after, VRIS, 2.5))

	rec := &diagRecorder{}
	if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if len(rec.got) != 2 {
		t.Fatalf("reported %d diagnostics, want 2: %v", len(rec.got), rec.got)
	}
	byTag := map[Tag]Diagnostic{}
	for _, d := range rec.got {
		byTag[d.Tag] = d
	}
	if d, ok := byTag[inner]; !ok {
		t.Errorf("nothing reported for the element inside the sequence")
	} else if len(d.Path) != 1 || d.Path[0] != seqTag {
		t.Errorf("inside the sequence Path = %v, want [%s]", d.Path, seqTag)
	}
	if d, ok := byTag[after]; !ok {
		t.Errorf("nothing reported for the element after the sequence")
	} else if len(d.Path) != 0 {
		t.Errorf("after the sequence Path = %v, want none", d.Path)
	}
}

// A value written straight from the bytes it was read as is not offered to the
// hook: it is not re-encoded, and the read that produced it already had its own
// chance to report. Reporting it here would make every round trip of a file with
// an over-long DS noisy about a value the caller never chose.
func TestWriteDoesNotReportValuesWrittenFromRawBytes(t *testing.T) {
	t.Parallel()
	const overlong = "0.3333333333333333"
	tg := MustTag("SliceThickness")

	val, err := ParseDS(overlong)
	if err != nil {
		t.Fatal(err)
	}
	source := NewDataset()
	source.Set(NewDataElement(tg, VRDS, val))
	encoded, err := EncodeDataset(source, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}

	fd, err := ReadBytes(encoded, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := fd.Dataset.Get(tg)
	if !ok {
		t.Fatal("the DS was not parsed")
	}
	if elem.RawValue == nil {
		t.Fatal("the parsed element kept no raw bytes; this test no longer covers the passthrough path")
	}

	rec := &diagRecorder{reject: true}
	out, err := EncodeFile(fd, &WriteOptions{OnDiagnostic: rec.hook})
	if err != nil {
		t.Fatalf("rewriting a file read from disk must not fail: %v", err)
	}
	if len(rec.got) != 0 {
		t.Errorf("rewriting raw bytes reported %v", rec.got)
	}
	if !bytes.Contains(out, []byte(overlong)) {
		t.Error("the over-long DS did not survive the round trip")
	}
}

// A write diagnostic has no source offset, so neither its message nor its log
// line may claim one. A parse diagnostic still carries its offset.
func TestDiagnosticMessageOmitsOffsetOnlyWhenWriting(t *testing.T) {
	t.Parallel()
	write := Diagnostic{
		Kind: DiagnosticInvalidValue,
		Tag:  MustTag("EchoNumbers"),
		VR:   VRIS,
		Err:  errors.New(`"1.5" is not an integer string`),
	}
	msg := write.Error()
	if strings.Contains(msg, "offset") {
		t.Errorf("write diagnostic message claims an offset: %q", msg)
	}
	for _, want := range []string{"invalid_value", "(0018,0086)", "IS", "1.5"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not mention %q", msg, want)
		}
	}

	read := Diagnostic{Kind: DiagnosticTruncatedHeader, Offset: 16}
	if got := read.Error(); !strings.Contains(got, "offset 16") {
		t.Errorf("parse diagnostic message lost its offset: %q", got)
	}
}

// numberStringReason is the whole judgement, so table it directly rather than
// reaching every shape through a file write.
func TestNumberStringReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		vr     VR
		part   string
		reason string // empty means valid
	}{
		{VRDS, "1.5", ""},
		{VRDS, " 1.5 ", ""},
		{VRDS, "-.5", ""},
		{VRDS, "1.23456789012345", ""}, // 16 bytes, the cap
		{VRDS, "", ""},
		{VRDS, "0.3333333333333333", "18 bytes, over the 16"},
		{VRDS, "NaN", "not a decimal string"},
		{VRDS, "1,5", "not a decimal string"},
		{VRIS, "42", ""},
		{VRIS, "-2147483648", ""},
		{VRIS, "2147483647", ""},
		{VRIS, "", ""},
		{VRIS, "1.5", "not an integer string"},
		{VRIS, "1e5", "not an integer string"},
		{VRIS, "9999999999999", "13 bytes, over the 12"},
		{VRIS, "2147483648", "outside [-2147483648, 2147483647]"},
		{VRIS, "999999999999", "outside [-2147483648, 2147483647]"},
	}

	for _, tc := range cases {
		t.Run(string(tc.vr)+"/"+tc.part, func(t *testing.T) {
			t.Parallel()
			err := numberStringReason(tc.vr, tc.part)
			if tc.reason == "" {
				if err != nil {
					t.Fatalf("%s %q rejected: %v", tc.vr, tc.part, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("%s %q accepted, want %q", tc.vr, tc.part, tc.reason)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("reason = %q, want it to mention %q", err, tc.reason)
			}
		})
	}
}

// A 17-byte DS is over the cap, so the length branch must not be shadowed by the
// spelling branch: both are wrong about it, only one says something useful.
func TestNumberStringReasonPrefersLengthOverSpelling(t *testing.T) {
	t.Parallel()
	err := numberStringReason(VRDS, "1.234567890123456") // 17 bytes, well formed
	if err == nil {
		t.Fatal("a 17-byte DS was accepted")
	}
	if !strings.Contains(err.Error(), "17 bytes, over the 16") {
		t.Errorf("reason = %q, want the length", err)
	}
}
