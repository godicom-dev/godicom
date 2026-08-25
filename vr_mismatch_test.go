package godicom

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// mismatchedVRFile encodes tag with a VR that is not the one the dictionary gives
// it, so reading the result raises exactly one DiagnosticVRMismatch. A conformant
// element is written first, so the mismatch does not land at offset 0 and the
// reported offset has to have been threaded rather than left at its zero value.
func mismatchedVRFile(t *testing.T, tag Tag, encodeAs VR, value any) []byte {
	t.Helper()
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("StudyDate"), VRDA, "20260824"))
	ds.Set(NewDataElement(tag, encodeAs, value))
	data, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A file that says SH where the dictionary says PN is not corrected -- what the
// file says is what the file means -- but the disagreement is reported, because
// it is the first thing you want to know when a device rejects your output.
func TestReadReportsVRDictionaryDisagreement(t *testing.T) {
	t.Parallel()
	tag := MustTag("PatientName")
	data := mismatchedVRFile(t, tag, VRSH, "Doe^Jane")

	rec := &diagRecorder{}
	fd, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err != nil {
		t.Fatalf("a hook returning nil must not fail the read: %v", err)
	}

	if len(rec.got) != 1 {
		t.Fatalf("reported %d diagnostics, want 1: %v", len(rec.got), rec.got)
	}
	d := rec.got[0]
	if d.Kind != DiagnosticVRMismatch {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticVRMismatch)
	}
	if d.Tag != tag {
		t.Errorf("Tag = %s, want %s", d.Tag, tag)
	}
	// VR is what the file carried; ExpectedVR is what the dictionary wanted.
	if d.VR != VRSH {
		t.Errorf("VR = %s, want the encoded %s", d.VR, VRSH)
	}
	if d.ExpectedVR != VRPN {
		t.Errorf("ExpectedVR = %s, want the dictionary's %s", d.ExpectedVR, VRPN)
	}
	// Unlike a write diagnostic, this one points into the source. The helper puts
	// a conformant element first, so a zero here would mean the offset was never
	// threaded through.
	if d.Offset <= 0 {
		t.Errorf("Offset = %d, want the element's position in the source", d.Offset)
	}
	if msg := d.Error(); !strings.Contains(msg, "dictionary says PN") {
		t.Errorf("message %q does not name the expected VR", msg)
	}

	// The parse is unchanged: godicom kept the VR the file gave it.
	elem, ok := fd.Dataset.Get(tag)
	if !ok {
		t.Fatal("the element was dropped")
	}
	if elem.VR != VRSH {
		t.Errorf("read back as %s, want the encoded %s -- the diagnostic must not correct the parse", elem.VR, VRSH)
	}
}

// Returning the diagnostic makes the disagreement a hard read failure, which is
// what a caller who refuses to touch non-conformant data wants.
func TestReadVRMismatchHookCanFailTheRead(t *testing.T) {
	t.Parallel()
	data := mismatchedVRFile(t, MustTag("PatientName"), VRSH, "Doe^Jane")

	rec := &diagRecorder{reject: true}
	_, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err == nil {
		t.Fatal("a hook returning the diagnostic must fail the read")
	}
	var d Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("read error does not unwrap to a Diagnostic: %v", err)
	}
	if d.Kind != DiagnosticVRMismatch || d.ExpectedVR != VRPN {
		t.Errorf("failed with %+v, want a VR mismatch expecting PN", d)
	}
}

// Without a hook the read must behave exactly as it did before this check
// existed -- same success, same VR, and no cost anybody asked for.
func TestReadVRMismatchSilentWithoutHook(t *testing.T) {
	t.Parallel()
	tag := MustTag("PatientName")
	data := mismatchedVRFile(t, tag, VRSH, "Doe^Jane")

	fd, err := ReadBytes(data, &ReadOptions{Force: true})
	if err != nil {
		t.Fatalf("read without a hook failed: %v", err)
	}
	elem, ok := fd.Dataset.Get(tag)
	if !ok {
		t.Fatal("the element was dropped")
	}
	if elem.VR != VRSH {
		t.Errorf("read back as %s, want %s", elem.VR, VRSH)
	}
}

// Implicit VR carries no VR of its own, so the VR in effect came from this same
// dictionary and cannot disagree with it. The check must not fire at all.
func TestReadVRMismatchNotRaisedForImplicitVR(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "Doe^Jane"))
	ds.Set(NewDataElement(MustTag("SliceThickness"), VRDS, "1.5"))
	data, err := EncodeDataset(ds, ImplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}

	rec := &diagRecorder{reject: true}
	if _, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook}); err != nil {
		t.Fatalf("reading implicit VR raised a diagnostic: %v", err)
	}
	for _, d := range rec.got {
		if d.Kind == DiagnosticVRMismatch {
			t.Errorf("implicit VR raised a VR mismatch: %v", d)
		}
	}
}

// The exclusions are the whole design. A check that fires on every private
// element or every unrecognised tag is a firehose, not a diagnostic.
func TestVRDisagreesWithDictionary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		tag     Tag
		encoded VR
		want    VR // "" means compatible, so nothing to report
	}{
		{"a plain disagreement", MustTag("PatientName"), VRSH, VRPN},
		{"UN standing in for a known VR", MustTag("PatientName"), VRUN, VRPN},
		{"the dictionary VR itself", MustTag("PatientName"), VRPN, ""},
		// PixelData is "OB or OW" and both spellings appear in real files.
		{"PixelData as OB", MustTag(0x7FE00010), VROB, ""},
		{"PixelData as OW", MustTag(0x7FE00010), VROW, ""},
		{"PixelData as something else", MustTag(0x7FE00010), VRUN, VR("OB or OW")},
		{"US or SS taken as US", MustTag("SmallestImagePixelValue"), VRUS, ""},
		{"US or SS taken as SS", MustTag("SmallestImagePixelValue"), VRSS, ""},
		// A private tag has no dictionary VR without its creator, and the
		// creator's VRs are vendor-defined. Reporting them all would be noise.
		{"a private tag", Tag(0x00090001), VRLO, ""},
		{"a private creator", Tag(0x00090010), VRLO, ""},
		// An unrecognised standard tag has no expectation to fall short of.
		// LookupVR would launder this into UN; dictionaryVR does not.
		{"a tag absent from the dictionary", Tag(0x00089999), VRLO, ""},
		{"no encoded VR at all", MustTag("PatientName"), "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := vrDisagreesWithDictionary(Standard(), tc.tag, tc.encoded); got != tc.want {
				t.Errorf("vrDisagreesWithDictionary(%s, %q) = %q, want %q", tc.tag, tc.encoded, got, tc.want)
			}
		})
	}
}

// The "absent from the dictionary" case above is only meaningful if the tag really
// is absent, and the LookupVR contrast is the reason the helper does not use it.
func TestVRMismatchExclusionPremises(t *testing.T) {
	t.Parallel()
	absent := Tag(0x00089999)
	if _, err := dictionaryVR(absent); err == nil {
		t.Fatalf("%s is in the dictionary; pick another tag for the absent-tag case", absent)
	}
	if got := LookupVR(absent); got != VRUN {
		t.Errorf("LookupVR(%s) = %s, want UN -- the laundering this helper avoids", absent, got)
	}
	if got := LookupVR(Tag(0x00090001)); got != VRUN {
		t.Errorf("LookupVR of a private tag = %s, want UN", got)
	}
}

// TestVRMismatchCorpusIsQuiet reads every file in testdata with a hook attached
// and requires the VR check to say nothing. It exists because the check runs once
// per explicit VR element -- effectively once per element -- so one wrong
// exclusion turns it from a signal into a firehose.
//
// The corpus is not vacuous for the case most likely to break: it holds 17
// PixelData elements, encoded as both OB and OW against a dictionary VR of
// "OB or OW", plus US-or-SS elements. Dropping the alternative matching in
// vrDisagreesWithDictionary makes this test fail with 23 findings.
func TestVRMismatchCorpusIsQuiet(t *testing.T) {
	t.Parallel()
	var files []string
	err := filepath.WalkDir("testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".dcm") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata files found; this test would pass vacuously")
	}

	perTag := map[string]int{}
	total, compound := 0, 0
	for _, path := range files {
		rec := &diagRecorder{}
		fd, err := ReadFile(path, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
		if err != nil {
			// A file godicom cannot read at all is not this test's business.
			continue
		}
		for _, d := range rec.got {
			if d.Kind != DiagnosticVRMismatch {
				continue
			}
			total++
			perTag[d.Tag.String()+" encoded "+string(d.VR)+", dictionary "+string(d.ExpectedVR)]++
		}
		if fd.Dataset == nil {
			continue
		}
		for _, tg := range fd.Dataset.SortedTags() {
			if want, err := dictionaryVR(tg); err == nil && strings.Contains(string(want), " or ") {
				compound++
			}
		}
	}

	if compound == 0 {
		t.Error("the corpus holds no multi-VR tags, so a quiet result proves nothing about the alternative matching")
	}
	if total != 0 {
		keys := make([]string, 0, len(perTag))
		for k := range perTag {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			t.Errorf("%3d x %s", perTag[k], k)
		}
		t.Errorf("%d VR mismatches across %d conformant files; the check has become noise", total, len(files))
	}
	t.Logf("%d files, %d multi-VR elements exercised, %d mismatches", len(files), compound, total)
}
