package godicom

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// overrideDictionary answers for the tags it holds and nothing else, which is what
// makes it useful in a composition: put it in front of Standard and exactly one
// entry changes, so a test can attribute an answer to it rather than to PS3.6.
//
// It also counts lookups. "First match wins" is only observable if the dictionary
// behind the one that answered is never consulted, and a count is the only way to
// see that from outside.
type overrideDictionary struct {
	entries map[Tag]DictEntry
	hits    *int
}

func (d overrideDictionary) Lookup(tag Tag, _ string) (DictEntry, bool) {
	if d.hits != nil {
		*d.hits++
	}
	entry, ok := d.entries[tag]
	return entry, ok
}

func overrideVR(tag Tag, vr VR) overrideDictionary {
	return overrideDictionary{entries: map[Tag]DictEntry{tag: {VR: string(vr), VM: "1", Name: "Overridden"}}}
}

// Argument order is the whole interface: the first dictionary that answers is the
// answer, so composing the same two dictionaries the other way round has to give
// the other one.
func TestNewDictionaryFirstMatchWins(t *testing.T) {
	t.Parallel()
	tag := MustTag("PatientName")
	override := overrideVR(tag, VRSH)

	if entry, ok := NewDictionary(override, Standard()).Lookup(tag, ""); !ok || entry.VR != "SH" {
		t.Errorf("override first: VR = %q, %v; want SH", entry.VR, ok)
	}
	if entry, ok := NewDictionary(Standard(), override).Lookup(tag, ""); !ok || entry.VR != "PN" {
		t.Errorf("Standard first: VR = %q, %v; want PN", entry.VR, ok)
	}

	// A tag the override says nothing about still resolves, or composing would be
	// no better than replacing.
	entry, ok := NewDictionary(override, Standard()).Lookup(MustTag("StudyDate"), "")
	if !ok || entry.VR != "DA" || entry.Keyword != "StudyDate" {
		t.Errorf("StudyDate through the composite = %+v, %v; want the PS3.6 entry", entry, ok)
	}
}

// First match wins, not most specific. A dictionary that answers for everything
// ends the search at itself, and the one behind it -- which has a far better entry
// -- is never asked. That is the documented rule, so it needs a test that fails if
// somebody "improves" it into a merge.
func TestNewDictionaryDoesNotLookPastTheFirstAnswer(t *testing.T) {
	t.Parallel()
	tag := MustTag("PatientName")
	hits := 0
	behind := overrideDictionary{entries: map[Tag]DictEntry{tag: {VR: "PN"}}, hits: &hits}

	entry, ok := NewDictionary(overrideVR(tag, VRSH), behind).Lookup(tag, "")
	if !ok || entry.VR != "SH" {
		t.Fatalf("VR = %q, %v; want SH from the first dictionary", entry.VR, ok)
	}
	if hits != 0 {
		t.Errorf("the dictionary behind the answer was consulted %d times, want 0", hits)
	}
}

// An empty composition and a nil member are both ordinary, not errors: threading a
// caller-built list through an options struct produces exactly these when the list
// is empty or half-filled, and panicking there would be a poor trade.
func TestNewDictionarySkipsNilAndAnswersNothingWhenEmpty(t *testing.T) {
	t.Parallel()
	tag := MustTag("PatientName")

	if _, ok := NewDictionary().Lookup(tag, ""); ok {
		t.Error("an empty composition answered something")
	}
	if _, ok := NewDictionary(nil).Lookup(tag, ""); ok {
		t.Error("a composition of one nil dictionary answered something")
	}
	entry, ok := NewDictionary(nil, Standard(), nil).Lookup(tag, "")
	if !ok || entry.VR != "PN" {
		t.Errorf("nils must be skipped, not fatal: VR = %q, %v", entry.VR, ok)
	}
}

// The composite copies the slice but not the dictionaries in it. Both halves
// matter: a caller reusing its slice must not be able to change what a composite
// resolves against, and a PrivateDictionary handed over must stay live, since
// registering the vendor block after building the composite is the natural order
// to write it in.
func TestNewDictionaryCopiesTheSliceAndKeepsMembersLive(t *testing.T) {
	t.Parallel()
	tag := MustTag("PatientName")

	dicts := []Dictionary{overrideVR(tag, VRSH)}
	composed := NewDictionary(dicts...)
	dicts[0] = overrideVR(tag, VRLO)
	if entry, _ := composed.Lookup(tag, ""); entry.VR != "SH" {
		t.Errorf("VR = %q, want SH -- rewriting the caller's slice changed the composite", entry.VR)
	}

	vendor := NewPrivateDictionary()
	live := NewDictionary(vendor, Standard())
	private := MustTag(0x00411001)
	if _, ok := live.Lookup(private, vendorCreator); ok {
		t.Fatalf("%s resolved before anything was added", private)
	}
	if err := vendor.Add(vendorCreator, MustTag(0x00410001), VRUS, "Vendor Number"); err != nil {
		t.Fatal(err)
	}
	if entry, ok := live.Lookup(private, vendorCreator); !ok || entry.VR != "US" {
		t.Errorf("after Add: VR = %q, %v; want US -- the member was copied, not held", entry.VR, ok)
	}
}

func TestPrivateDictionaryAddAndLookup(t *testing.T) {
	t.Parallel()
	d := NewPrivateDictionary()
	if err := d.Add(vendorCreator, MustTag(0x00410001), VRUS, "Vendor Number"); err != nil {
		t.Fatal(err)
	}

	entry, ok := d.Lookup(MustTag(0x00411001), vendorCreator)
	if !ok {
		t.Fatal("the entry just added was not found")
	}
	if entry.VR != "US" || entry.Name != "Vendor Number" {
		t.Errorf("entry = %+v, want US/Vendor Number", entry)
	}
	if entry.VM != "1" {
		t.Errorf("VM = %q, want the default 1", entry.VM)
	}
	// PS3.6 assigns keywords to standard elements; a private one has none, and an
	// empty string here is the honest answer rather than a gap.
	if entry.Keyword != "" {
		t.Errorf("Keyword = %q, want empty", entry.Keyword)
	}

	if err := d.Add(vendorCreator, MustTag(0x00410002), VRDS, "Vendor Numbers", "3"); err != nil {
		t.Fatal(err)
	}
	if entry, _ := d.Lookup(MustTag(0x00411002), vendorCreator); entry.VM != "3" {
		t.Errorf("VM = %q, want the 3 that was passed", entry.VM)
	}

	// Another vendor's creator is another vendor's dictionary. The same tag means
	// something else, or nothing, and nothing is what this one has to say.
	if _, ok := d.Lookup(MustTag(0x00411001), "SOMEONE ELSE"); ok {
		t.Error("an entry resolved under a creator it was not registered under")
	}
	// A standard tag has no business here even by accident.
	if _, ok := d.Lookup(MustTag("PatientName"), vendorCreator); ok {
		t.Error("a standard tag resolved against a private dictionary")
	}
}

// PS3.5 lets a vendor's block land in any of the (gggg,10xx)-(gggg,FFxx) slots, so
// the same element is (0041,1001) in one file and (0041,2001) in the next. An
// entry is registered against the block byte for that reason, and a dictionary
// that only answered for the tag as spelled would be useless on the second file.
func TestPrivateDictionaryResolvesARelocatedBlock(t *testing.T) {
	t.Parallel()
	d := NewPrivateDictionary()
	if err := d.Add(vendorCreator, MustTag(0x00410001), VRUS, "Vendor Number"); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []Tag{
		MustTag(0x00410001),
		MustTag(0x00411001),
		MustTag(0x00412001),
		MustTag(0x0041FF01), // the last block PS3.5 allows
	} {
		if _, ok := d.Lookup(tag, vendorCreator); !ok {
			t.Errorf("Lookup(%s) found nothing; a relocated block must still resolve", tag)
		}
	}
	// The block byte is the low byte of the element, and a different one is a
	// different element.
	if _, ok := d.Lookup(MustTag(0x00411002), vendorCreator); ok {
		t.Error("(0041,1002) resolved against an entry registered for element byte 01")
	}
}

func TestPrivateDictionaryRejectsStandardTags(t *testing.T) {
	t.Parallel()
	d := NewPrivateDictionary()
	standard := MustTag("PatientName")
	err := d.Add(vendorCreator, standard, VRPN, "Patient's Name")
	if err == nil {
		t.Fatal("adding a standard tag to a private dictionary must fail")
	}
	if !strings.Contains(err.Error(), standard.String()) {
		t.Errorf("error %q does not name the tag", err)
	}
	if _, ok := d.Lookup(standard, vendorCreator); ok {
		t.Error("the rejected entry was stored anyway")
	}
}

// The point of the type is that it is not the process-global table. An entry in
// one must not show up in the other, in either direction, or a library that built
// its own dictionary would still be changing what every other caller reads.
func TestPrivateDictionaryIsSeparateFromTheProcessGlobalOne(t *testing.T) {
	// Not parallel: AddPrivateDictEntry mutates process-global state, which is the
	// very thing this test exists to contrast with.
	t.Cleanup(ResetExtraPrivateDictionaries)

	mine := NewPrivateDictionary()
	local := MustTag(0x00410001)
	if err := mine.Add(vendorCreator, local, VRUS, "Mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := PrivateDictionaryVR(local, vendorCreator); err == nil {
		t.Error("an entry added to a PrivateDictionary leaked into the global table")
	}
	if _, ok := Standard().Lookup(local, vendorCreator); ok {
		t.Error("Standard resolved an entry that was never registered with it")
	}

	global := MustTag(0x00430001)
	if err := AddPrivateDictEntry(vendorCreator, global, VRUS, "Theirs"); err != nil {
		t.Fatal(err)
	}
	if _, ok := Standard().Lookup(global, vendorCreator); !ok {
		t.Fatal("AddPrivateDictEntry no longer reaches Standard")
	}
	if _, ok := mine.Lookup(global, vendorCreator); ok {
		t.Error("a global entry leaked into a PrivateDictionary")
	}
}

func TestReadContextDictionaryIsNilSafe(t *testing.T) {
	t.Parallel()
	// A decoder handed no readContext at all still has to resolve VRs, so this is
	// reachable rather than defensive.
	var rc *readContext
	if entry, ok := rc.dictionary().Lookup(MustTag("PatientName"), ""); !ok || entry.VR != "PN" {
		t.Errorf("a nil readContext resolved %+v, %v; want the standard entry", entry, ok)
	}
	if entry, ok := (&readContext{}).dictionary().Lookup(MustTag("PatientName"), ""); !ok || entry.VR != "PN" {
		t.Errorf("an unset dictionary resolved %+v, %v; want the standard entry", entry, ok)
	}
}

const (
	// A creator no dictionary in the repository knows, so an element that resolves
	// under it can only have got there through the dictionary the read was handed.
	vendorCreator  = "GODICOM TEST"
	vendorBlockTag = 0x00410010
	vendorElemTag  = 0x00411001
)

func vendorDictionary(t *testing.T) Dictionary {
	t.Helper()
	if _, ok := Standard().Lookup(MustTag(vendorElemTag), vendorCreator); ok {
		t.Fatalf("%q is now a known creator; pick another for these tests", vendorCreator)
	}
	vendor := NewPrivateDictionary()
	if err := vendor.Add(vendorCreator, MustTag(0x00410001), VRUS, "Vendor Number"); err != nil {
		t.Fatal(err)
	}
	return NewDictionary(vendor, Standard())
}

// vendorDataset encodes a headerless implicit VR little endian dataset holding the
// vendor's private creator and one of its elements. Implicit VR is the point: the
// file carries no VRs at all, so what the private element means is entirely up to
// the dictionary the reader was given.
func vendorDataset(t *testing.T, value any) []byte {
	t.Helper()
	ds := NewDataset()
	if err := ds.SetString(MustTag("PatientID"), "ABCD1234"); err != nil {
		t.Fatal(err)
	}
	ds.Set(NewDataElement(MustTag(vendorBlockTag), VRLO, vendorCreator))
	ds.Set(NewDataElement(MustTag(vendorElemTag), VRUS, value))
	data, err := EncodeDataset(ds, ImplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The end of the whole exercise: a vendor's private element read as the vendor
// meant it, without registering anything process-wide. Without the dictionary the
// same bytes are UN, which is correct too -- a reader that cannot name an element
// says so rather than guessing.
func TestReadOptionsDictionaryResolvesAPrivateVR(t *testing.T) {
	t.Parallel()
	data := vendorDataset(t, 4095)
	tag := MustTag(vendorElemTag)

	plain, err := ReadBytes(data, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := plain.Dataset.Get(tag)
	if !ok {
		t.Fatal("the private element was dropped by a plain read")
	}
	if elem.VR != VRUN {
		t.Fatalf("without a dictionary VR = %s, want UN", elem.VR)
	}

	withVendor, err := ReadBytes(data, &ReadOptions{Force: true, Dictionary: vendorDictionary(t)})
	if err != nil {
		t.Fatal(err)
	}
	elem, ok = withVendor.Dataset.Get(tag)
	if !ok {
		t.Fatal("the private element was dropped by a read with the vendor dictionary")
	}
	if elem.VR != VRUS {
		t.Fatalf("with the vendor dictionary VR = %s, want US", elem.VR)
	}
	if elem.Value != uint64(4095) {
		t.Errorf("Value = %#v, want the decoded uint16 4095", elem.Value)
	}

	// Composing rather than replacing: the standard elements still resolve.
	if id, ok := withVendor.Dataset.GetString(MustTag("PatientID")); !ok || id != "ABCD1234" {
		t.Errorf("PatientID = %q, %v; the standard half of the composition broke", id, ok)
	}
}

// A deferred value is decoded on Get, after the read has returned, and the reload
// rejects an element whose VR no longer matches the one it was first read under.
// So the dictionary has to outlive the parse -- which is why it lives on
// readContext and not on the codecContext the rest of the decoding state is in.
// Dropping it would turn every deferred private element into a mismatch error.
func TestReadOptionsDictionarySurvivesADeferredLoad(t *testing.T) {
	t.Parallel()
	// Twenty US values: longer than the 12-byte private creator below it, so the
	// element defers and the creator that gives it meaning does not.
	values := make([]int, 20)
	for i := range values {
		values[i] = i + 1
	}
	data := vendorDataset(t, NewMultiValue(values))
	tag := MustTag(vendorElemTag)

	path := filepath.Join(t.TempDir(), "vendor.dcm")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	eager, err := ReadBytes(data, &ReadOptions{Force: true, Dictionary: vendorDictionary(t)})
	if err != nil {
		t.Fatal(err)
	}
	want, ok := eager.Dataset.Get(tag)
	if !ok {
		t.Fatal("the private element was not parsed eagerly")
	}

	readers := map[string]func() (*FileDataset, error){
		"ReadBytes": func() (*FileDataset, error) {
			return ReadBytes(data, &ReadOptions{
				Force: true, DeferSize: 12, Dictionary: vendorDictionary(t),
			})
		},
		"ReadFile": func() (*FileDataset, error) {
			return ReadFile(path, &ReadOptions{
				Force: true, DeferSize: 12, Dictionary: vendorDictionary(t),
			})
		},
	}

	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fd, err := read()
			if err != nil {
				t.Fatal(err)
			}
			elem, ok := fd.Dataset.elements[tag]
			if !ok {
				t.Fatal("the private element was not parsed")
			}
			if !elem.Deferred {
				t.Fatalf("element is not deferred (length %d); the test no longer covers the reload path",
					elem.ValueLength)
			}
			if elem.VR != VRUS {
				t.Fatalf("VR on the first pass = %s, want US", elem.VR)
			}
			// The creator has to have been read eagerly, or the reload would fail
			// for want of a creator rather than for want of a dictionary.
			if fd.Dataset.elements[MustTag(vendorBlockTag)].Deferred {
				t.Fatal("the private creator was deferred too; the test proves nothing")
			}

			if err := fd.LoadDeferred(tag); err != nil {
				t.Fatalf("LoadDeferred: %v -- the dictionary did not survive the parse", err)
			}
			if elem.VR != VRUS {
				t.Errorf("VR after reload = %s, want US", elem.VR)
			}
			if !reflect.DeepEqual(elem.Value, want.Value) {
				t.Errorf("reloaded value = %#v, want the eager %#v", elem.Value, want.Value)
			}
		})
	}
}

// The VR-mismatch diagnostic judges the file against the dictionary the read was
// given, not always against PS3.6. A caller who overrode an entry said what they
// expect the file to contain, and measuring against a different expectation than
// the parse used would be reporting on nothing.
func TestReadOptionsDictionaryDrivesTheVRMismatchDiagnostic(t *testing.T) {
	t.Parallel()
	tag := MustTag("PatientName")
	data := mismatchedVRFile(t, tag, VRSH, "Doe^Jane")

	// The file says SH and this dictionary says SH, so the disagreement PS3.6 sees
	// is not a disagreement here.
	quiet := &diagRecorder{}
	if _, err := ReadBytes(data, &ReadOptions{
		Force:        true,
		OnDiagnostic: quiet.hook,
		Dictionary:   NewDictionary(overrideVR(tag, VRSH), Standard()),
	}); err != nil {
		t.Fatal(err)
	}
	if len(quiet.got) != 0 {
		t.Errorf("reported %v against a dictionary that agrees with the file", quiet.got)
	}

	// And an override in the other direction reports its own VR, not PS3.6's.
	loud := &diagRecorder{}
	if _, err := ReadBytes(data, &ReadOptions{
		Force:        true,
		OnDiagnostic: loud.hook,
		Dictionary:   NewDictionary(overrideVR(tag, VRLO), Standard()),
	}); err != nil {
		t.Fatal(err)
	}
	if len(loud.got) != 1 {
		t.Fatalf("reported %d diagnostics, want 1: %v", len(loud.got), loud.got)
	}
	if got := loud.got[0].ExpectedVR; got != VRLO {
		t.Errorf("ExpectedVR = %s, want the supplied dictionary's LO", got)
	}
}

// A Deflated dataset is parsed by a second read that finishDeflated sets up by
// copying ReadOptions field by field, so a new field is dropped unless it is
// copied explicitly. Dropping this one would read the inflated dataset against
// PS3.6 alone: the same file would mean one thing deflated and another undeflated.
func TestReadOptionsDictionaryReachesTheDeflatedDataset(t *testing.T) {
	t.Parallel()
	path := testFilePath("image_dfl.dcm")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// ConversionType sits in the deflated half, past the File Meta, so only the
	// inner read can report on it.
	tag := MustTag("ConversionType")
	opts := func(rec *diagRecorder) *ReadOptions {
		return &ReadOptions{
			OnDiagnostic: rec.hook,
			Dictionary:   NewDictionary(overrideVR(tag, VRPN), Standard()),
		}
	}

	readers := map[string]func(*diagRecorder) (*FileDataset, error){
		// The streaming reader inflates into a fresh parse: finishDeflated.
		"ReadFile": func(rec *diagRecorder) (*FileDataset, error) {
			return ReadFile(path, opts(rec))
		},
		// The in-memory reader inflates in place and keeps its readContext.
		"ReadBytes": func(rec *diagRecorder) (*FileDataset, error) {
			return ReadBytes(data, opts(rec))
		},
	}

	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := &diagRecorder{}
			fd, err := read(rec)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := fd.Dataset.Get(tag); !ok {
				t.Fatal("ConversionType is not in this file any more; pick another tag")
			}
			if len(rec.got) != 1 {
				t.Fatalf("reported %d diagnostics, want 1: %v", len(rec.got), rec.got)
			}
			d := rec.got[0]
			if d.Tag != tag {
				t.Errorf("Tag = %s, want %s", d.Tag, tag)
			}
			if d.ExpectedVR != VRPN {
				t.Errorf("ExpectedVR = %s, want the supplied dictionary's PN", d.ExpectedVR)
			}
		})
	}
}
