package godicom

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"testing"
)

// populatedSequence finds a sequence in path that has at least one item with at
// least one element, so a test can tell "the items were parsed" from "the items
// were emptied".
func populatedSequence(t *testing.T, path string) (Tag, int) {
	t.Helper()
	ds, err := ReadFile(path, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, elem := range ds.Iter() {
		if elem.VR != VRSQ {
			continue
		}
		seq, ok := elem.Value.(*Sequence)
		if !ok || seq == nil || seq.Len() == 0 {
			continue
		}
		if n := seq.Items()[0].Len(); n > 0 {
			return elem.Tag, n
		}
	}
	t.Skipf("%s has no sequence with a populated first item", path)
	return 0, 0
}

// TestSpecificTagsKeepsSequenceItems pins the scope of SpecificTags to the
// top-level dataset, which is where pydicom applies it: read_sequence_item calls
// read_dataset without passing specific_tags along, so a sequence that survives
// the filter is parsed whole. Filtering inside it instead kept the sequence
// element and emptied every item -- a dataset no file could have produced.
func TestSpecificTagsKeepsSequenceItems(t *testing.T) {
	t.Parallel()
	path := testFilePath("rtstruct.dcm")
	seqTag, wantItemLen := populatedSequence(t, path)

	ds, err := ReadFile(path, &ReadOptions{
		Force:        true,
		SpecificTags: []Tag{seqTag},
	})
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(seqTag)
	if !ok {
		t.Fatalf("SpecificTags dropped %s", seqTag)
	}
	seq, ok := elem.Value.(*Sequence)
	if !ok || seq.Len() == 0 {
		t.Fatalf("%s is not a populated sequence: %#v", seqTag, elem.Value)
	}
	if got := seq.Items()[0].Len(); got != wantItemLen {
		t.Errorf("%s item 0 has %d elements, want %d (SpecificTags must not filter inside a sequence)",
			seqTag, got, wantItemLen)
	}
}

func TestOnElementSkipKeepsOnlyWhatItAsksFor(t *testing.T) {
	t.Parallel()
	want := map[Tag]bool{
		MustTag("PatientName"): true,
		MustTag("PatientID"):   true,
	}
	ds, err := ReadFile(testFilePath("CT_small.dcm"), &ReadOptions{
		OnElement: func(h RawDataElement, path []PathStep) ElementAction {
			if want[h.Tag] {
				return ElementKeep
			}
			return ElementSkip
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Specific Character Set is exempt from the hook, so it comes along.
	expected := []Tag{TagCharset, MustTag("PatientName"), MustTag("PatientID")}
	got := ds.SortedTags()
	if len(got) != len(expected) {
		t.Fatalf("tags = %v, want %v", got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("tags = %v, want %v", got, expected)
		}
	}
	if ds.FileMeta.Len() == 0 {
		t.Error("File Meta was skipped; group 0x0002 is exempt from the hook")
	}
}

func TestOnElementStop(t *testing.T) {
	t.Parallel()
	stopAt := MustTag("PatientID")
	var seen []Tag
	ds, err := ReadFile(testFilePath("CT_small.dcm"), &ReadOptions{
		OnElement: func(h RawDataElement, path []PathStep) ElementAction {
			seen = append(seen, h.Tag)
			if h.Tag == stopAt {
				return ElementStop
			}
			return ElementKeep
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Has(stopAt) {
		t.Errorf("%s was stored, but ElementStop discards the element it stops at", stopAt)
	}
	if seen[len(seen)-1] != stopAt {
		t.Errorf("hook saw %s after the stop, want %s last", seen[len(seen)-1], stopAt)
	}
	if ds.Has(MustTag("PixelData")) {
		t.Error("PixelData survived a stop that happened before it")
	}
	if ds.Len() == 0 {
		t.Error("everything before the stop was discarded too")
	}
}

// TestOnElementStopMatchesStopBeforePixels checks the two against each other,
// because StopBeforePixels is now implemented on the same decision point: a hook
// stopping at (7FE0,0010) has to produce the dataset the option does.
func TestOnElementStopMatchesStopBeforePixels(t *testing.T) {
	t.Parallel()
	path := testFilePath("CT_small.dcm")
	viaOption, err := ReadFile(path, &ReadOptions{StopBeforePixels: true})
	if err != nil {
		t.Fatal(err)
	}
	viaHook, err := ReadFile(path, &ReadOptions{
		OnElement: func(h RawDataElement, path []PathStep) ElementAction {
			if h.Tag == MustTag("PixelData") {
				return ElementStop
			}
			return ElementKeep
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	optionTags, hookTags := viaOption.SortedTags(), viaHook.SortedTags()
	if len(optionTags) != len(hookTags) {
		t.Fatalf("StopBeforePixels kept %d tags, an equivalent hook kept %d", len(optionTags), len(hookTags))
	}
	for i := range optionTags {
		if optionTags[i] != hookTags[i] {
			t.Fatalf("tag %d: StopBeforePixels %s, hook %s", i, optionTags[i], hookTags[i])
		}
	}
}

// TestOnElementSkipsInsideSequence exercises the depth the hook is called at: a
// caller can step over elements nested in a sequence, which is the case
// SpecificTags cannot express at all.
func TestOnElementSkipsInsideSequence(t *testing.T) {
	t.Parallel()
	path := testFilePath("rtstruct.dcm")
	seqTag, wantItemLen := populatedSequence(t, path)

	var deepest int
	ds, err := ReadFile(path, &ReadOptions{
		Force: true,
		OnElement: func(h RawDataElement, path []PathStep) ElementAction {
			if len(path) > deepest {
				deepest = len(path)
			}
			if len(path) > 0 && path[0].Tag == seqTag {
				return ElementSkip
			}
			return ElementKeep
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if deepest == 0 {
		t.Fatal("hook was never called inside a sequence; path was always empty")
	}
	elem, ok := ds.Get(seqTag)
	if !ok {
		t.Fatalf("%s was dropped, but only its contents were skipped", seqTag)
	}
	seq := elem.Value.(*Sequence)
	if seq.Len() == 0 {
		t.Fatalf("%s lost its items", seqTag)
	}
	if got := seq.Items()[0].Len(); got != 0 {
		t.Errorf("%s item 0 kept %d of %d elements, want 0", seqTag, got, wantItemLen)
	}
}

// TestOnElementSeesPath checks the path a nested element is reported under names
// the sequence and the item, which is what makes a nested decision possible.
func TestOnElementSeesPath(t *testing.T) {
	t.Parallel()
	path := testFilePath("rtstruct.dcm")
	seqTag, _ := populatedSequence(t, path)

	var got []PathStep
	_, err := ReadFile(path, &ReadOptions{
		Force: true,
		OnElement: func(h RawDataElement, p []PathStep) ElementAction {
			if got == nil && len(p) > 0 && p[0].Tag == seqTag {
				got = append([]PathStep(nil), p...)
			}
			return ElementKeep
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatalf("no element was reported inside %s", seqTag)
	}
	if got[0].Tag != seqTag {
		t.Errorf("path[0].Tag = %s, want %s", got[0].Tag, seqTag)
	}
	if got[0].Item < 0 {
		t.Errorf("path[0].Item = %d, want the index of the item being parsed", got[0].Item)
	}
}

// TestOnElementHeaderFields checks what the hook is handed. The value is the one
// thing it must not carry -- not having read it is the point.
func TestOnElementHeaderFields(t *testing.T) {
	t.Parallel()
	path := testFilePath("CT_small.dcm")
	full, err := ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// CT_small.dcm carries PatientID at the top level and again inside
	// (0010,1002) Other Patient IDs Sequence, so the path has to be part of the
	// match or the assertions land on whichever came last.
	target := MustTag("PatientID")
	var seen RawDataElement
	var found bool
	_, err = ReadFile(path, &ReadOptions{
		OnElement: func(h RawDataElement, p []PathStep) ElementAction {
			if h.Tag == target && len(p) == 0 {
				seen, found = h, true
			}
			return ElementKeep
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("hook never saw %s", target)
	}
	if seen.Value != nil {
		t.Errorf("Value = %v, want nil: the hook runs before the value is read", seen.Value)
	}
	if !seen.IsRaw {
		t.Error("IsRaw = false, want true")
	}
	wantElem, ok := full.Get(target)
	if !ok {
		t.Fatalf("a full read has no %s", target)
	}
	want := wantElem.RawValue
	if int(seen.Length) != len(want) {
		t.Errorf("Length = %d, want %d", seen.Length, len(want))
	}
	if seen.VR != wantElem.VR {
		t.Errorf("VR = %s, want %s", seen.VR, wantElem.VR)
	}
	if got := data[seen.ValueTell : seen.ValueTell+int64(seen.Length)]; !bytes.Equal(got, want) {
		t.Errorf("ValueTell %d does not point at the value: %q vs %q", seen.ValueTell, got, want)
	}
}

// TestOnElementValueTellInsideSequence pins ValueTell to file coordinates at
// depth. The streaming reader parses a sequence out of a chunk it copied out of
// the file, so the offset its loop holds is relative to that chunk; reporting it
// unadjusted would hand the caller a number that points into the wrong element.
func TestOnElementValueTellInsideSequence(t *testing.T) {
	t.Parallel()
	path := testFilePath("CT_small.dcm")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	full, err := ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A nested element whose value is known from a full read, so the bytes at the
	// reported offset can be compared against it.
	seq, ok := full.GetSequence(MustTag(0x00101002)) // Other Patient IDs Sequence
	if !ok || seq.Len() == 0 {
		t.Skip("CT_small.dcm has no Other Patient IDs Sequence to descend into")
	}
	nested := MustTag("PatientID")
	wantElem, ok := seq.Items()[0].Get(nested)
	if !ok {
		t.Skipf("item 0 has no %s", nested)
	}
	want := wantElem.RawValue

	var checked bool
	_, err = ReadFile(path, &ReadOptions{
		OnElement: func(h RawDataElement, p []PathStep) ElementAction {
			if checked || h.Tag != nested || len(p) == 0 || p[0].Item != 0 {
				return ElementKeep
			}
			checked = true
			if h.ValueTell+int64(h.Length) > int64(len(data)) {
				t.Errorf("ValueTell %d + Length %d runs past the %d-byte file",
					h.ValueTell, h.Length, len(data))
				return ElementKeep
			}
			if got := data[h.ValueTell : h.ValueTell+int64(h.Length)]; !bytes.Equal(got, want) {
				t.Errorf("ValueTell %d points at %q, want %q", h.ValueTell, got, want)
			}
			return ElementKeep
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatalf("%s was never reported inside item 0", nested)
	}
}

// countingReaderAt counts the bytes a parse actually pulls from the source.
type countingReaderAt struct {
	ra    io.ReaderAt
	size  int64
	bytes int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.ra.ReadAt(p, off)
	c.bytes += int64(n)
	return n, err
}

func (c *countingReaderAt) Size() int64 { return c.size }

// Read is never called: ReadContext takes the ReaderAt+Size path, which is the
// point. It is here only to satisfy io.Reader.
func (c *countingReaderAt) Read([]byte) (int, error) { return 0, io.EOF }

// TestOnElementSkipDoesNotReadTheValue is the point of the whole exercise: a
// caller after a handful of index tags should not pay to move the rest of the
// file. Counted in bytes off the source rather than in allocations, because the
// I/O is what the caller is trying not to do and the count is exact.
func TestOnElementSkipDoesNotReadTheValue(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(testFilePath("CT_small.dcm"))
	if err != nil {
		t.Fatal(err)
	}
	read := func(opts *ReadOptions) int64 {
		t.Helper()
		c := &countingReaderAt{ra: bytes.NewReader(data), size: int64(len(data))}
		if _, err := Read(c, opts); err != nil {
			t.Fatal(err)
		}
		return c.bytes
	}

	full := read(nil)
	keep := MustTag("PatientID")
	skipped := read(&ReadOptions{
		OnElement: func(h RawDataElement, p []PathStep) ElementAction {
			if len(p) > 0 || h.Tag == keep {
				return ElementKeep
			}
			return ElementSkip
		},
	})
	if skipped >= full {
		t.Errorf("skipping every value but one read %d bytes, no better than the %d a full read costs",
			skipped, full)
	}
	// The headers still have to be walked, so the saving is bounded by the value
	// bytes. Pixel Data alone is most of this file, so more than half must go.
	if skipped > full/2 {
		t.Errorf("skipping read %d of %d bytes; the values are most of the file, so this is not skipping them",
			skipped, full)
	}
}

// TestOnElementSkipUndefinedLengthLandsOnTheNextElement covers the one skip that
// cannot be done by arithmetic. A defined-length value declares where it ends, so
// stepping over it is addition; an undefined-length one does not, so the item
// stream has to be walked to find the end -- and if that walk stops at the wrong
// byte, everything after it is lost. MR_small_RLE.dcm is the case that shows it:
// encapsulated Pixel Data with (FFFC,FFFC) behind it.
func TestOnElementSkipUndefinedLengthLandsOnTheNextElement(t *testing.T) {
	t.Parallel()
	path := testFilePath("MR_small_RLE.dcm")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// uint32, not a bare literal: 0xFFFCFFFC overflows int on a 32-bit build, and
	// ParseTag's int case is what a bare constant lands in.
	pixels, trailing := MustTag("PixelData"), MustTag(uint32(0xFFFCFFFC))

	full, err := ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if elem, ok := full.Get(pixels); !ok || !elem.IsUndefinedLength {
		t.Skipf("%s does not carry %s at undefined length", path, pixels)
	}
	if !full.Has(trailing) {
		t.Skipf("%s has nothing after %s to land on", path, pixels)
	}
	want := rawValue(t, full, trailing)

	// Both byte-slice loops: ReadBytes for the top level, and the same file through
	// the streaming reader as a cross-check that the two agree on the end.
	readers := map[string]func(*ReadOptions) (*FileDataset, error){
		"ReadBytes": func(o *ReadOptions) (*FileDataset, error) { return ReadBytes(data, o) },
		"ReadFile":  func(o *ReadOptions) (*FileDataset, error) { return ReadFile(path, o) },
	}
	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			ds, err := read(&ReadOptions{
				OnElement: func(h RawDataElement, p []PathStep) ElementAction {
					if len(p) == 0 && h.Tag == pixels {
						return ElementSkip
					}
					return ElementKeep
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if ds.Has(pixels) {
				t.Errorf("%s was kept despite ElementSkip", pixels)
			}
			if !ds.Has(trailing) {
				t.Fatalf("%s was lost, so the skip past %s stopped at the wrong byte", trailing, pixels)
			}
			if got := rawValue(t, ds, trailing); !bytes.Equal(got, want) {
				t.Errorf("%s = %q, want %q: the skip landed mid-element", trailing, got, want)
			}
		})
	}
}

// allocBytes is how many bytes the heap grew over one call, which is the figure a
// skip is trying to move. Counted rather than timed because the number is
// deterministic: the value either got copied or it did not.
func allocBytes(t *testing.T, f func()) uint64 {
	t.Helper()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestOnElementSkipUndefinedLengthDoesNotCopyIt is the byte-slice counterpart of
// TestOnElementSkipDoesNotReadTheValue. Over a byte slice there is no I/O left to
// save -- the file is already in memory -- so what a skip has to avoid is the copy,
// and an undefined-length value is the case where avoiding it takes real work: it
// declares no length, so the end can only be found by walking the item stream.
// Walking it is cheap; copying it is the whole cost of the element.
func TestOnElementSkipUndefinedLengthDoesNotCopyIt(t *testing.T) {
	data, err := os.ReadFile(testFilePath("MR_small_RLE.dcm"))
	if err != nil {
		t.Fatal(err)
	}
	pixels := MustTag("PixelData")
	elem, ok := mustReadBytes(t, data, nil).Get(pixels)
	if !ok || !elem.IsUndefinedLength {
		t.Skip("MR_small_RLE.dcm does not carry Pixel Data at undefined length")
	}
	valueSize := uint64(len(elem.RawValue))

	full := allocBytes(t, func() { mustReadBytes(t, data, nil) })
	skipped := allocBytes(t, func() {
		mustReadBytes(t, data, &ReadOptions{
			OnElement: func(h RawDataElement, p []PathStep) ElementAction {
				if len(p) == 0 && h.Tag == pixels {
					return ElementSkip
				}
				return ElementKeep
			},
		})
	})
	// The value is 6 KB of the file's 8, so not copying it has to show up as a drop
	// of at least that much. Measured against the value's own size rather than a
	// fraction of the total, because the rest of the read allocates whatever it
	// allocates and that is not what this test is about.
	if full < skipped+valueSize {
		t.Errorf("a full read allocated %d bytes and skipping the %d-byte Pixel Data allocated %d; the saving is %d, want at least %d",
			full, valueSize, skipped, int64(full)-int64(skipped), valueSize)
	}
}

// mustReadBytes is ReadBytes with the error check folded in, so an allocation
// measurement is not measuring the check.
func mustReadBytes(t *testing.T, data []byte, opts *ReadOptions) *FileDataset {
	t.Helper()
	ds, err := ReadBytes(data, opts)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

// rawValue is the element's undecoded bytes, which is what a skip test wants to
// compare: it is the same for every VR, where the decoded value is a string for
// one element and a []byte for the next.
func rawValue(t *testing.T, ds *FileDataset, tag Tag) []byte {
	t.Helper()
	elem, ok := ds.Get(tag)
	if !ok {
		return nil
	}
	return elem.RawValue
}

// TestOnElementSkipWholeFileAgreesWithFullRead is the round-trip that catches a
// skip that advanced the wrong number of bytes: skipping everything but one tag
// must still find that tag, whatever came before it.
func TestOnElementSkipWholeFileAgreesWithFullRead(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"CT_small.dcm", "MR_small.dcm", "rtstruct.dcm"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := testFilePath(name)
			full, err := ReadFile(path, &ReadOptions{Force: true})
			if err != nil {
				t.Fatal(err)
			}
			// Every top-level tag, one read each, keeping only that one.
			for _, want := range full.SortedTags() {
				keepOnly := want
				partial, err := ReadFile(path, &ReadOptions{
					Force: true,
					OnElement: func(h RawDataElement, p []PathStep) ElementAction {
						if len(p) > 0 || h.Tag == keepOnly {
							return ElementKeep
						}
						return ElementSkip
					},
				})
				if err != nil {
					t.Fatalf("keeping only %s: %v", keepOnly, err)
				}
				if !partial.Has(keepOnly) {
					t.Errorf("keeping only %s: it was not found, so a skip mislanded", keepOnly)
					continue
				}
				if a, b := rawValue(t, full, keepOnly), rawValue(t, partial, keepOnly); !bytes.Equal(a, b) {
					t.Errorf("%s: value differs between a full read and a skip-everything-else read: %d vs %d bytes",
						keepOnly, len(a), len(b))
				}
			}
		})
	}
}

// TestOnElementSkipOnEveryReadPath runs the same hook through all three parse
// loops -- ReadFile over a ReaderAt, ReadBytes over a byte slice, and Read over a
// non-seekable reader -- because each has its own copy of the element loop.
func TestOnElementSkipOnEveryReadPath(t *testing.T) {
	t.Parallel()
	path := testFilePath("CT_small.dcm")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	keep := MustTag("PatientID")
	opts := func() *ReadOptions {
		return &ReadOptions{
			OnElement: func(h RawDataElement, p []PathStep) ElementAction {
				if len(p) > 0 || h.Tag == keep {
					return ElementKeep
				}
				return ElementSkip
			},
		}
	}

	readers := map[string]func() (*FileDataset, error){
		"ReadFile":    func() (*FileDataset, error) { return ReadFile(path, opts()) },
		"ReadBytes":   func() (*FileDataset, error) { return ReadBytes(data, opts()) },
		"bytesReader": func() (*FileDataset, error) { return Read(bytes.NewReader(data), opts()) },
		"nonSeekable": func() (*FileDataset, error) { return Read(nonSeekableReader{bytes.NewReader(data)}, opts()) },
	}
	want, err := ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantValue := rawValue(t, want, keep)

	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			ds, err := read()
			if err != nil {
				t.Fatal(err)
			}
			if !ds.Has(keep) {
				t.Fatalf("%s was skipped away", keep)
			}
			if got := rawValue(t, ds, keep); !bytes.Equal(got, wantValue) {
				t.Errorf("value = %q, want %q", got, wantValue)
			}
			if ds.Has(MustTag("PixelData")) {
				t.Error("PixelData was kept despite ElementSkip")
			}
		})
	}
}

// nonSeekableReader hides the ReadSeeker under it so ReadContext takes the
// io.ReadAll path, where skipping saves the decode but not the I/O.
type nonSeekableReader struct{ r *bytes.Reader }

func (n nonSeekableReader) Read(p []byte) (int, error) { return n.r.Read(p) }

// TestOnElementNotCalledForExemptTags pins the two carve-outs. Skipping either
// would change what the elements after it mean, not just how many are kept.
func TestOnElementNotCalledForExemptTags(t *testing.T) {
	t.Parallel()
	var offered []Tag
	_, err := ReadFile(testFilePath("CT_small.dcm"), &ReadOptions{
		OnElement: func(h RawDataElement, _ []PathStep) ElementAction {
			offered = append(offered, h.Tag)
			return ElementKeep
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offered) == 0 {
		t.Fatal("hook was never called")
	}
	for _, tag := range offered {
		if tag.Group() == 0x0002 {
			t.Errorf("File Meta element %s was offered to the hook", tag)
		}
		if tag == TagCharset {
			t.Errorf("Specific Character Set %s was offered to the hook", tag)
		}
	}
}

// diagnosticsFor reads path and returns everything the reader reported.
func diagnosticsFor(t *testing.T, path string, opts *ReadOptions) []Diagnostic {
	t.Helper()
	var got []Diagnostic
	opts.Force = true
	opts.OnDiagnostic = func(d Diagnostic) error { got = append(got, d); return nil }
	if _, err := ReadFile(path, opts); err != nil {
		t.Fatal(err)
	}
	return got
}

// TestOnElementSkipStillDiagnoses pins what a skip does to the diagnostics:
// nothing. They describe the file rather than the subset the caller kept, so
// narrowing a read is not a reason to stop hearing that the bytes are malformed --
// and a caller who wants them quiet has OnDiagnostic for that. rtdose_rle.dcm
// raises a vr_mismatch on 35 elements, which is enough to tell "unchanged" from
// "suppressed": rtstruct.dcm raises none at all, so it proved nothing.
func TestOnElementSkipStillDiagnoses(t *testing.T) {
	t.Parallel()
	path := testFilePath("rtdose_rle.dcm")

	baseline := diagnosticsFor(t, path, &ReadOptions{})
	if len(baseline) == 0 {
		t.Skipf("%s raises no diagnostics", path)
	}
	got := diagnosticsFor(t, path, &ReadOptions{
		OnElement: func(RawDataElement, []PathStep) ElementAction { return ElementSkip },
	})
	if len(got) != len(baseline) {
		t.Errorf("skipping every element raised %d diagnostics, want the %d a full read raises",
			len(got), len(baseline))
	}
}

// TestOnElementStopEndsDiagnostics is the other half: a stop abandons the parse,
// so nothing past it is looked at and nothing past it is reported. This is the one
// case where the hook does change what the caller hears.
func TestOnElementStopEndsDiagnostics(t *testing.T) {
	t.Parallel()
	path := testFilePath("rtdose_rle.dcm")

	baseline := diagnosticsFor(t, path, &ReadOptions{})
	if len(baseline) < 2 {
		t.Skipf("%s raises %d diagnostics, too few to stop in the middle of", path, len(baseline))
	}
	// Stop at the element the second diagnostic names, so the first is reported and
	// the rest are not.
	stopAt := baseline[1].Tag
	got := diagnosticsFor(t, path, &ReadOptions{
		OnElement: func(h RawDataElement, p []PathStep) ElementAction {
			if len(p) == 0 && h.Tag == stopAt {
				return ElementStop
			}
			return ElementKeep
		},
	})
	if len(got) >= len(baseline) {
		t.Errorf("stopping at %s raised %d of %d diagnostics, want fewer: the parse ended there",
			stopAt, len(got), len(baseline))
	}
	for _, d := range got {
		if d.Tag == stopAt {
			t.Errorf("%s was diagnosed, but ElementStop does not parse the element it stops at", stopAt)
		}
	}
}

// TestOnElementRunsAfterSpecificTags pins the composition order: SpecificTags
// filters first, so a hook can only narrow what it kept.
func TestOnElementRunsAfterSpecificTags(t *testing.T) {
	t.Parallel()
	var offered []Tag
	ds, err := ReadFile(testFilePath("CT_small.dcm"), &ReadOptions{
		SpecificTags: []Tag{MustTag("PatientName"), MustTag("PatientID")},
		OnElement: func(h RawDataElement, _ []PathStep) ElementAction {
			offered = append(offered, h.Tag)
			if h.Tag == MustTag("PatientName") {
				return ElementSkip
			}
			return ElementKeep
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range offered {
		if tag != MustTag("PatientName") && tag != MustTag("PatientID") {
			t.Errorf("hook was offered %s, which SpecificTags had already skipped", tag)
		}
	}
	if ds.Has(MustTag("PatientName")) {
		t.Error("the hook's ElementSkip did not override SpecificTags keeping PatientName")
	}
	if !ds.Has(MustTag("PatientID")) {
		t.Error("PatientID was lost")
	}
}
