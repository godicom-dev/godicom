package godicom

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// The tests here are about Diagnostic.Path naming *which item* of a sequence an
// anomaly came from. PS3.5 gives sequence items an ordinal position and nothing
// else to identify them by, so without the index a forty-item sequence produces
// forty identical-looking diagnostics.
//
// They provoke the anomaly with a VR the data dictionary disagrees with, because
// it is the one diagnostic that can be placed in an arbitrary item without
// corrupting the bytes around it: every element still encodes and decodes
// normally, so the path is the only thing under test.

// mismatchInItem returns an item whose PatientName carries SH instead of PN, so
// reading it raises exactly one DiagnosticVRMismatch from inside that item.
func mismatchInItem(uid string) *Dataset {
	item := NewDataset()
	item.Set(NewDataElement(MustTag("ReferencedSOPInstanceUID"), VRUI, uid))
	item.Set(NewDataElement(MustTag("PatientName"), VRSH, "Doe^Jane"))
	return item
}

// conformantItem returns an item nothing can be said about, used as padding so
// the item under test does not sit at index 0.
func conformantItem(uid string) *Dataset {
	item := NewDataset()
	item.Set(NewDataElement(MustTag("ReferencedSOPInstanceUID"), VRUI, uid))
	return item
}

func readWithDiagnostics(t *testing.T, ds *Dataset, ts UID) []Diagnostic {
	t.Helper()
	data, err := EncodeDataset(ds, ts)
	if err != nil {
		t.Fatal(err)
	}
	rec := &diagRecorder{}
	if _, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook}); err != nil {
		t.Fatalf("a hook returning nil must not fail the read: %v", err)
	}
	return rec.got
}

// onlyMismatch returns the single VR mismatch among got, failing if there is not
// exactly one. The other kinds are not this file's business.
func onlyMismatch(t *testing.T, got []Diagnostic) Diagnostic {
	t.Helper()
	var found []Diagnostic
	for _, d := range got {
		if d.Kind == DiagnosticVRMismatch {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		t.Fatalf("got %d VR mismatches, want 1: %v", len(found), found)
	}
	return found[0]
}

// The index has to be the item the anomaly is actually in, not the first one.
func TestDiagnosticPathNamesTheItem(t *testing.T) {
	t.Parallel()
	seqTag := MustTag("ReferencedImageSequence")

	for _, at := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("item %d", at), func(t *testing.T) {
			t.Parallel()
			items := []*Dataset{conformantItem("1.2.1"), conformantItem("1.2.2"), conformantItem("1.2.3")}
			items[at] = mismatchInItem("1.2.9")

			ds := NewDataset()
			if err := ds.SetSequence(seqTag, NewSequence(items)); err != nil {
				t.Fatal(err)
			}

			d := onlyMismatch(t, readWithDiagnostics(t, ds, ExplicitVRLittleEndian))
			want := pathStep(seqTag, at)
			if len(d.Path) != 1 || d.Path[0] != want {
				t.Fatalf("Path = %v, want [%s]", d.Path, want)
			}
			if msg := d.Error(); !strings.Contains(msg, want.String()) {
				t.Errorf("message %q does not name %s", msg, want)
			}
		})
	}
}

// Nesting has to carry an index at every level: knowing the anomaly is in the
// third item of the inner sequence is no use without knowing which outer item
// that inner sequence belongs to.
func TestDiagnosticPathNamesTheItemAtEveryDepth(t *testing.T) {
	t.Parallel()
	outerTag := MustTag("ReferencedImageSequence")
	innerTag := MustTag("ReferencedStudySequence")

	inner := []*Dataset{conformantItem("1.3.1"), mismatchInItem("1.3.2")}
	middle := NewDataset()
	middle.Set(NewDataElement(MustTag("ReferencedSOPInstanceUID"), VRUI, "1.2.2"))
	if err := middle.SetSequence(innerTag, NewSequence(inner)); err != nil {
		t.Fatal(err)
	}

	ds := NewDataset()
	if err := ds.SetSequence(outerTag, NewSequence([]*Dataset{conformantItem("1.2.1"), middle})); err != nil {
		t.Fatal(err)
	}

	d := onlyMismatch(t, readWithDiagnostics(t, ds, ExplicitVRLittleEndian))
	want := []PathStep{pathStep(outerTag, 1), pathStep(innerTag, 1)}
	if len(d.Path) != 2 || d.Path[0] != want[0] || d.Path[1] != want[1] {
		t.Fatalf("Path = %v, want %v", d.Path, want)
	}
	if msg := d.Error(); !strings.Contains(msg, "in (0008,1140)[1] > (0008,1110)[1]") {
		t.Errorf("message %q does not spell the nested path", msg)
	}
}

// An index left over from a sequence already finished with would mislabel
// everything after it, and popSeq alone does not catch that: the index lives in
// the stack entry, so the next push has to start clean.
func TestDiagnosticPathItemDoesNotLeakBetweenSequences(t *testing.T) {
	t.Parallel()
	first := MustTag("ReferencedImageSequence")
	second := MustTag("ReferencedStudySequence")

	ds := NewDataset()
	// Three items in the first sequence, so its index climbs to 2 before the
	// second sequence -- whose only item is index 0 -- is entered.
	if err := ds.SetSequence(first, NewSequence([]*Dataset{
		conformantItem("1.2.1"), conformantItem("1.2.2"), conformantItem("1.2.3"),
	})); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetSequence(second, NewSequence([]*Dataset{mismatchInItem("1.3.1")})); err != nil {
		t.Fatal(err)
	}

	d := onlyMismatch(t, readWithDiagnostics(t, ds, ExplicitVRLittleEndian))
	want := pathStep(second, 0)
	if len(d.Path) != 1 || d.Path[0] != want {
		t.Fatalf("Path = %v, want [%s] -- the previous sequence's index leaked", d.Path, want)
	}
}

// An undefined-length sequence is walked by a different item loop than a
// defined-length one, on both readers. All of them have to name the item.
func TestDiagnosticPathNamesTheItemForEveryReader(t *testing.T) {
	t.Parallel()
	seqTag := MustTag("ReferencedImageSequence")

	for _, undefined := range []bool{false, true} {
		name := "defined length"
		if undefined {
			name = "undefined length"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			seq := NewSequence([]*Dataset{conformantItem("1.2.1"), mismatchInItem("1.2.2")})
			seq.IsUndefinedLength = undefined
			ds := NewDataset()
			if err := ds.SetSequence(seqTag, seq); err != nil {
				t.Fatal(err)
			}
			data, err := EncodeDataset(ds, ExplicitVRLittleEndian)
			if err != nil {
				t.Fatal(err)
			}

			// ReadBytes and the streaming ReadFile reach the item loop by
			// different routes; the path must not depend on which.
			readers := map[string]func(*ReadOptions) (*FileDataset, error){
				"ReadBytes": func(o *ReadOptions) (*FileDataset, error) { return ReadBytes(data, o) },
				"Read":      func(o *ReadOptions) (*FileDataset, error) { return Read(bytes.NewReader(data), o) },
			}
			for reader, read := range readers {
				t.Run(reader, func(t *testing.T) {
					t.Parallel()
					rec := &diagRecorder{}
					if _, err := read(&ReadOptions{Force: true, OnDiagnostic: rec.hook}); err != nil {
						t.Fatalf("a hook returning nil must not fail the read: %v", err)
					}
					d := onlyMismatch(t, rec.got)
					want := pathStep(seqTag, 1)
					if len(d.Path) != 1 || d.Path[0] != want {
						t.Fatalf("Path = %v, want [%s]", d.Path, want)
					}
				})
			}
		})
	}
}

// The writer keeps its own path, so it needs its own proof. It also has two item
// loops -- one that buffers a defined-length SQ, one that streams an
// undefined-length one -- and neither may report the wrong item.
func TestWriteDiagnosticPathNamesTheItem(t *testing.T) {
	t.Parallel()
	seqTag := MustTag("ReferencedImageSequence")

	for _, undefined := range []bool{false, true} {
		name := "defined length"
		if undefined {
			name = "undefined length"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// A fractional float in an IS has no integer spelling, so the writer
			// reports it. Put it in the third item of three.
			bad := NewDataset()
			bad.Set(NewDataElement(MustTag("EchoNumbers"), VRIS, 1.5))
			seq := NewSequence([]*Dataset{conformantItem("1.2.1"), conformantItem("1.2.2"), bad})
			seq.IsUndefinedLength = undefined

			ds := NewDataset()
			if err := ds.SetSequence(seqTag, seq); err != nil {
				t.Fatal(err)
			}

			rec := &diagRecorder{}
			if _, err := writeWithDiagnostics(t, ds, rec); err != nil {
				t.Fatalf("a hook returning nil must not fail the write: %v", err)
			}

			d := wantOneInvalidValue(t, rec, MustTag("EchoNumbers"), VRIS, "not an integer string")
			want := pathStep(seqTag, 2)
			if len(d.Path) != 1 || d.Path[0] != want {
				t.Fatalf("Path = %v, want [%s]", d.Path, want)
			}
		})
	}
}
