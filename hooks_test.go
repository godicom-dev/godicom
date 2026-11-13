package godicom

import (
	"encoding/hex"
	"testing"

	"github.com/godicom-dev/godicom/tag"
)

// The expectations here were produced by running pydicom 3.1.0.dev0 with its
// hooks module on the same bytes -- scripts/check_hooks.py prints them -- and
// are ported as literals, the way the digest-pinned tests pin theirs.

// hooksGoldenInput is the explicit VR little endian dataset
// scripts/check_hooks.py builds for the fix_separator scenario: Patient's
// Weight carrying "70:5" and Patient's Age carrying "3:0", both using a colon
// where PS3.5 wants a backslash.
const hooksGoldenInput = "100030104453040037303a351000101049530400333a3020"

// retryGoldenInput is the same script's retry scenario: Rows declared US but
// carrying five bytes, the odd count that makes the numeric converter give up.
const retryGoldenInput = "28000001555305003900390039"

// resolveGoldenInput is the script's resolve scenario: an implicit VR little
// endian dataset whose only element is the private (0009,1001) with two bytes,
// with no private creator and no dictionary entry to name a VR.
const resolveGoldenInput = "09000110020000003900"

func hooksInput(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestFixSeparatorValueParity ports the script's fix_separator scenario.
func TestFixSeparatorValueParity(t *testing.T) {
	t.Parallel()
	data := hooksInput(t, hooksGoldenInput)

	opts := &ReadOptions{
		Force: true,
		Hooks: &Hooks{ConvertValue: FixSeparatorValue(":", VRDS, VRIS)},
	}
	fd, err := ReadBytes(data, opts)
	if err != nil {
		t.Fatal(err)
	}

	// pydicom: ds[0x00101030].value == [DSfloat("70"), DSfloat("5")], with each
	// original_string kept, and the same shape for the IS element.
	ds := fd.Dataset
	weight, ok := ds.Get(tag.PatientWeight)
	if !ok {
		t.Fatal("PatientWeight missing")
	}
	wantDS := []DS{{Value: 70, Original: "70"}, {Value: 5, Original: "5"}}
	if got, ok := weight.Value.(*MultiValue[DS]); !ok || !equalDSs(got.values, wantDS) {
		t.Errorf("PatientWeight = %#v, want %#v", weight.Value, wantDS)
	}
	age, ok := ds.Get(tag.PatientAge)
	if !ok {
		t.Fatal("PatientAge missing")
	}
	wantIS := []IS{{Value: 3, Original: "3"}, {Value: 0, Original: "0"}}
	if got, ok := age.Value.(*MultiValue[IS]); !ok || !equalISs(got.values, wantIS) {
		t.Errorf("PatientAge = %#v, want %#v", age.Value, wantIS)
	}
	// pydicom's data["VR"] keeps the file's VR; so does godicom's.
	if weight.VR != VRDS || age.VR != VRIS {
		t.Errorf("VRs = %s, %s, want DS, IS", weight.VR, age.VR)
	}

	// Unhooked, pydicom hands back the unparsed string; godicom keeps the raw
	// bytes and no value. The difference is the standing one -- pydicom's
	// validation modes decide on the Python side, godicom reports damage
	// through OnDiagnostic -- pinned here so it stays a decision.
	fd, err = ReadBytes(data, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	weight, ok = fd.Dataset.Get(tag.PatientWeight)
	if !ok {
		t.Fatal("PatientWeight missing")
	}
	if got, ok := weight.Value.([]byte); !ok || string(got) != "70:5" {
		t.Errorf("unhooked PatientWeight = %#v, want the raw bytes \"70:5\"", weight.Value)
	}
}

// TestRetryValueParity ports the script's retry scenario.
func TestRetryValueParity(t *testing.T) {
	t.Parallel()
	data := hooksInput(t, retryGoldenInput)

	opts := &ReadOptions{
		Force: true,
		Hooks: &Hooks{ConvertValue: RetryValue(map[VR][]VR{VRUS: {VROB, VRSS}})},
	}
	fd, err := ReadBytes(data, opts)
	if err != nil {
		t.Fatal(err)
	}
	// pydicom: the US conversion raises BytesLengthException, the retry
	// converts the five bytes as OB, and data["VR"] becomes OB so the value
	// and the name it is read under agree.
	rows, ok := fd.Dataset.Get(tag.BitsAllocated)
	if !ok {
		t.Fatal("BitsAllocated missing")
	}
	if rows.VR != VROB {
		t.Errorf("BitsAllocated.VR = %s, want OB", rows.VR)
	}
	want := []byte{0x39, 0x00, 0x39, 0x00, 0x39}
	if got, ok := rows.Value.([]byte); !ok || string(got) != string(want) {
		t.Errorf("BitsAllocated.Value = %#v, want % x", rows.Value, want)
	}

	// Unhooked, pydicom's read fails outright with BytesLengthException;
	// godicom keeps the bytes and stays silent, which is the standing design
	// the writer's diagnostics document. Pinned so it stays a decision.
	fd, err = ReadBytes(data, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok = fd.Dataset.Get(tag.BitsAllocated)
	if !ok {
		t.Fatal("BitsAllocated missing")
	}
	if rows.VR != VRUS {
		t.Errorf("unhooked BitsAllocated.VR = %s, want US", rows.VR)
	}
	if got, ok := rows.Value.([]byte); !ok || string(got) != string(want) {
		t.Errorf("unhooked BitsAllocated.Value = %#v, want the raw bytes", rows.Value)
	}
}

// TestResolveVRHookParity ports the script's resolve scenario.
func TestResolveVRHookParity(t *testing.T) {
	t.Parallel()
	data := hooksInput(t, resolveGoldenInput)

	// pydicom: the default raw_element_vr answers UN and the value stays the
	// two raw bytes.
	fd, err := ReadBytes(data, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	private := MustTag("(0009,1001)")
	elem, ok := fd.Dataset.Get(private)
	if !ok {
		t.Fatal("private element missing")
	}
	if elem.VR != VRUN {
		t.Errorf("default VR = %s, want UN", elem.VR)
	}
	if got, ok := elem.Value.([]byte); !ok || len(got) != 2 {
		t.Errorf("default value = %#v, want the two raw bytes", elem.Value)
	}

	// The script's callback delegates to the default and then forces US for
	// that one tag -- the middleware shape: resolve, then decide.
	opts := &ReadOptions{
		Force: true,
		Hooks: &Hooks{ResolveVR: func(raw RawDataElement, resolve func() VR) VR {
			if raw.Tag != private {
				t.Errorf("hook saw %s, want only %s", raw.Tag, private)
			}
			if raw.VR != "" {
				t.Errorf("hook saw VR %q, want empty", raw.VR)
			}
			if def := resolve(); def != VRUN {
				t.Errorf("resolve() = %s, want UN", def)
			}
			return VRUS
		}},
	}
	fd, err = ReadBytes(data, opts)
	if err != nil {
		t.Fatal(err)
	}
	elem, ok = fd.Dataset.Get(private)
	if !ok {
		t.Fatal("private element missing")
	}
	// pydicom: ds[0x00091001].VR == "US" and .value == 57.
	if elem.VR != VRUS {
		t.Errorf("forced VR = %s, want US", elem.VR)
	}
	if got, ok := elem.Value.(uint64); !ok || got != 57 {
		t.Errorf("forced value = %#v, want 57", elem.Value)
	}

	// A hook that only delegates reads exactly as no hook at all.
	opts = &ReadOptions{Force: true, Hooks: &Hooks{ResolveVR: func(raw RawDataElement, resolve func() VR) VR {
		return resolve()
	}}}
	fd, err = ReadBytes(data, opts)
	if err != nil {
		t.Fatal(err)
	}
	elem, ok = fd.Dataset.Get(private)
	if !ok {
		t.Fatal("private element missing")
	}
	if elem.VR != VRUN {
		t.Errorf("delegating hook VR = %s, want UN", elem.VR)
	}
}

// TestResolveVRHookSeesThePrivateDictionary pins that resolve() carries the
// whole default chain: with the private creator element and an entry registered
// for the tag, the delegate answers the dictionary's VR, not UN.
func TestResolveVRHookSeesThePrivateDictionary(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("(0009,0010)"), VRLO, "GODICOM"))
	unknown := MustTag("(0009,1001)")
	ds.Set(NewDataElement(unknown, VRUS, []uint64{57}))
	data, err := EncodeDataset(ds, ImplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}

	if err := AddPrivateDictEntry("GODICOM", unknown, VRUS, "Test element"); err != nil {
		t.Fatal(err)
	}

	var seen []VR
	opts := &ReadOptions{Force: true, Hooks: &Hooks{ResolveVR: func(raw RawDataElement, resolve func() VR) VR {
		vr := resolve()
		seen = append(seen, vr)
		return vr
	}}}
	fd, err := ReadBytes(data, opts)
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := fd.Dataset.Get(unknown)
	if !ok {
		t.Fatal("private element missing")
	}
	if elem.VR != VRUS {
		t.Errorf("VR = %s, want US from the private dictionary", elem.VR)
	}
	if len(seen) != 2 || seen[0] != VRLO || seen[1] != VRUS {
		t.Errorf("resolve() answers = %v, want [LO US] -- the creator and the element", seen)
	}
}

// TestConvertValueNotOnDeferredReload pins that a deferred reload keeps the
// historical fallback: the hook fires for the elements converted during the
// read, and never for a value loaded after it.
func TestConvertValueNotOnDeferredReload(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "Doe^John"))
	ds.Set(NewDataElement(MustTag("StudyDescription"), VRLO, "A description long enough to be deferred when DeferSize is small"))
	data, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}

	var converted int
	opts := &ReadOptions{
		Force:     true,
		DeferSize: 32,
		Hooks: &Hooks{ConvertValue: func(raw *RawDataElement, _ []string, convert func() (any, error)) (any, error) {
			converted++
			return convert()
		}},
	}
	fd, err := ReadBytes(data, opts)
	if err != nil {
		t.Fatal(err)
	}
	if converted != 1 {
		t.Fatalf("hook fired %d times during the read, want 1 -- the name, not the deferred description", converted)
	}

	desc, ok := fd.Dataset.Get(MustTag("StudyDescription"))
	if !ok {
		t.Fatal("StudyDescription missing")
	}
	if desc.Value == nil {
		t.Fatal("the deferred value did not load")
	}
	if converted != 1 {
		t.Errorf("hook fired %d times after the reload, want 1 -- a reload keeps the fallback", converted)
	}
	if _, ok := desc.Value.(string); !ok {
		t.Errorf("reloaded value = %#v, want the converted string", desc.Value)
	}
}

func equalDSs(a, b []DS) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalISs(a, b []IS) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
