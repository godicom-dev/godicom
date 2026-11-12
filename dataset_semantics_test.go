package godicom

import (
	"strings"
	"testing"
)

// Ported from pydicom tests/test_dataset.py::TestDataset. Cases that depend on
// Python protocols (__getitem__, __contains__, keys/values/items, with-statement,
// pickling) are omitted: godicom exposes typed getters and tag constants instead.

func newSemanticsDataset() *Dataset {
	ds := NewDataset()
	_ = ds.SetString(MustTag(0x00100010), "Doe^John")
	_ = ds.SetString(MustTag(0x00100020), "ID123")
	_ = ds.SetInt(MustTag(0x00280010), 512)
	return ds
}

func TestDatasetGetSetHasDelete(t *testing.T) {
	// pydicom: TestDataset::test_get_missing, test_get_returns_element,
	// test_has_element, test_delete_element, test_len
	ds := newSemanticsDataset()

	if got := ds.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3", got)
	}
	if !ds.Has(MustTag(0x00100010)) {
		t.Fatal("Has(PatientName) = false, want true")
	}
	if ds.Has(MustTag(0x00080016)) {
		t.Fatal("Has(SOPClassUID) = true, want false")
	}
	elem, ok := ds.Get(MustTag(0x00100010))
	if !ok {
		t.Fatal("Get(PatientName) not found")
	}
	if elem.VR != VRPN {
		t.Fatalf("Get(PatientName).VR = %s, want PN", elem.VR)
	}

	ds.Delete(MustTag(0x00100010))
	if ds.Has(MustTag(0x00100010)) {
		t.Fatal("element still present after Delete")
	}
	if got := ds.Len(); got != 2 {
		t.Fatalf("Len() after Delete = %d, want 2", got)
	}

	// pydicom: test_delete_missing_is_a_noop
	ds.Delete(MustTag(0x00080016))
	if got := ds.Len(); got != 2 {
		t.Fatalf("Len() after deleting a missing tag = %d, want 2", got)
	}
}

func TestDatasetSetReplacesExistingElement(t *testing.T) {
	// pydicom: TestDataset::test_set_existing_replaces
	ds := newSemanticsDataset()
	_ = ds.SetString(MustTag(0x00100010), "Roe^Jane")
	if got := ds.Len(); got != 3 {
		t.Fatalf("Len() = %d after replacing a value, want 3", got)
	}
	got, ok := ds.GetString(MustTag(0x00100010))
	if !ok || got != "Roe^Jane" {
		t.Fatalf("GetString(PatientName) = %q, %v, want Roe^Jane", got, ok)
	}
}

func TestDatasetPopSemantics(t *testing.T) {
	// pydicom: TestDataset::test_pop, test_pop_missing
	ds := newSemanticsDataset()
	elem, ok := ds.Pop(MustTag(0x00100020))
	if !ok || elem == nil {
		t.Fatal("Pop(PatientID) not found")
	}
	if ds.Has(MustTag(0x00100020)) {
		t.Fatal("Pop left the element in the dataset")
	}
	if _, ok := ds.Pop(MustTag(0x00100020)); ok {
		t.Fatal("Pop of a missing tag reported ok")
	}
}

func TestDatasetIterIsInTagOrder(t *testing.T) {
	// pydicom: TestDataset::test_iter_order -- insertion order must not leak
	ds := NewDataset()
	_ = ds.SetInt(MustTag(0x00280010), 512)
	_ = ds.SetString(MustTag(0x00100010), "Doe^John")
	_ = ds.SetString(MustTag(0x00080016), "1.2.3")

	var got []Tag
	for _, elem := range ds.Iter() {
		got = append(got, elem.Tag)
	}
	want := []Tag{MustTag(0x00080016), MustTag(0x00100010), MustTag(0x00280010)}
	if len(got) != len(want) {
		t.Fatalf("Iter() returned %d elements, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Iter()[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestDatasetIterAllIsDepthFirst(t *testing.T) {
	// pydicom: TestDataset::test_iterall -- the sequence element comes before the
	// elements of the items it holds.
	inner := newSemanticsDataset()
	seq := NewSequence([]*Dataset{inner})
	outer := NewDataset()
	_ = outer.SetString(MustTag(0x00080016), "1.2.3")
	_ = outer.SetSequence(MustTag(0x0040A730), seq)

	var order []Tag
	for _, elem := range outer.IterAll() {
		order = append(order, elem.Tag)
	}
	if len(order) != 2+inner.Len() {
		t.Fatalf("IterAll() returned %d elements, want %d", len(order), 2+inner.Len())
	}
	if order[1] != MustTag(0x0040A730) {
		t.Fatalf("IterAll()[1] = %s, want the sequence element first", order[1])
	}
	if order[2] != MustTag(0x00100010) {
		t.Fatalf("IterAll()[2] = %s, want the first item element", order[2])
	}
}

func TestDatasetWalkRecursiveAndFlat(t *testing.T) {
	// pydicom: TestDataset::test_walk, test_walk_recursive
	inner := NewDataset()
	_ = inner.SetString(MustTag(0x00100010), "Doe^John")
	outer := NewDataset()
	_ = outer.SetValue(MustTag(0x0040A730), NewSequence([]*Dataset{inner}))

	var flat int
	outer.Walk(func(_ *Dataset, _ *Element) { flat++ }, false)
	if flat != 1 {
		t.Fatalf("non-recursive Walk visited %d elements, want 1", flat)
	}

	var deep int
	outer.Walk(func(_ *Dataset, _ *Element) { deep++ }, true)
	if deep != 2 {
		t.Fatalf("recursive Walk visited %d elements, want 2", deep)
	}
}

func TestDatasetGroupDatasetSemantics(t *testing.T) {
	// pydicom: TestDataset::test_group_dataset
	ds := NewDataset()
	_ = ds.SetString(MustTag(0x00100010), "Doe^John")
	_ = ds.SetString(MustTag(0x00100020), "ID123")
	_ = ds.SetString(MustTag(0x00080016), "1.2.3")

	group := ds.GroupDataset(0x0010)
	if got := group.Len(); got != 2 {
		t.Fatalf("GroupDataset(0x0010).Len() = %d, want 2", got)
	}
	if group.Has(MustTag(0x00080016)) {
		t.Fatal("GroupDataset(0x0010) contains an element from another group")
	}

	// pydicom: test_group_dataset_returns_copy -- mutating the group must not
	// reach back into the source dataset.
	_ = group.SetString(MustTag(0x00100010), "Roe^Jane")
	if v, _ := ds.GetString(MustTag(0x00100010)); v != "Doe^John" {
		t.Fatalf("source dataset changed through the group copy: %q", v)
	}
}

func TestDatasetRemovePrivateTagsSemantics(t *testing.T) {
	// pydicom: TestDataset::test_remove_private_tags,
	// test_remove_private_tags_in_sequence
	inner := NewDataset()
	_ = inner.SetString(MustTag(0x00100010), "Doe^John")
	_ = inner.SetString(MustTag(0x00091001), "private")

	ds := NewDataset()
	_ = ds.SetString(MustTag(0x00100010), "Doe^John")
	_ = ds.SetString(MustTag(0x00090010), "ACME 1.0")
	_ = ds.SetSequence(MustTag(0x0040A730), NewSequence([]*Dataset{inner}))

	ds.RemovePrivateTags()

	if ds.Has(MustTag(0x00090010)) {
		t.Fatal("top-level private element survived RemovePrivateTags")
	}
	if !ds.Has(MustTag(0x00100010)) {
		t.Fatal("public element was removed")
	}
	seq, _ := ds.GetSequence(MustTag(0x0040A730))
	item := seq.Get(0)
	if item.Has(MustTag(0x00091001)) {
		t.Fatal("nested private element survived RemovePrivateTags")
	}
	if !item.Has(MustTag(0x00100010)) {
		t.Fatal("nested public element was removed")
	}
}

func TestDatasetEqualSemantics(t *testing.T) {
	// pydicom: TestDataset::test_eq, test_ne
	a := newSemanticsDataset()
	b := newSemanticsDataset()
	if !a.Equal(b) {
		t.Fatal("Equal() = false for identical datasets")
	}

	c := newSemanticsDataset()
	_ = c.SetString(MustTag(0x00100010), "Roe^Jane")
	if a.Equal(c) {
		t.Fatal("Equal() = true for datasets with different values")
	}

	d := NewDataset()
	_ = d.SetString(MustTag(0x00100010), "Doe^John")
	if a.Equal(d) {
		t.Fatal("Equal() = true for datasets with different element counts")
	}
	if a.Equal(nil) {
		t.Fatal("Equal(nil) = true")
	}
}

func TestDatasetCloneIsIndependent(t *testing.T) {
	// pydicom: TestDataset::test_copy, test_deepcopy
	src := newSemanticsDataset()
	clone := src.Clone()
	if !src.Equal(clone) {
		t.Fatal("Clone() is not equal to its source")
	}

	_ = clone.SetString(MustTag(0x00100010), "Roe^Jane")
	if v, _ := src.GetString(MustTag(0x00100010)); v != "Doe^John" {
		t.Fatalf("source changed through the clone: %q", v)
	}
}

func TestDatasetTopOmitsSequenceItems(t *testing.T) {
	// pydicom: TestDataset::test_top
	inner := NewDataset()
	_ = inner.SetString(MustTag(0x00100010), "Nested^Name")
	ds := NewDataset()
	_ = ds.SetString(MustTag(0x00100010), "Doe^John")
	_ = ds.SetSequence(MustTag(0x0040A730), NewSequence([]*Dataset{inner}))

	top := ds.Top()
	if !strings.Contains(top, "Doe^John") {
		t.Fatalf("Top() = %q, missing the top-level element", top)
	}
	if strings.Contains(top, "Nested^Name") {
		t.Fatalf("Top() = %q, should not descend into sequence items", top)
	}
}

func TestDatasetFormattedLinesIncludesSequenceItems(t *testing.T) {
	// pydicom: TestDataset::test_formatted_lines
	inner := NewDataset()
	_ = inner.SetString(MustTag(0x00100020), "ID123")
	ds := NewDataset()
	_ = ds.SetString(MustTag(0x00100010), "Doe^John")
	_ = ds.SetSequence(MustTag(0x0040A730), NewSequence([]*Dataset{inner}))

	lines := ds.FormattedLines(nil)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Doe^John") {
		t.Fatalf("FormattedLines() = %q, missing the top-level element", joined)
	}
	if !strings.Contains(joined, "ID123") {
		t.Fatalf("FormattedLines() = %q, missing the sequence item element", joined)
	}
}

func TestDatasetElementByKeywordSemantics(t *testing.T) {
	// pydicom: TestDataset::test_data_element
	ds := newSemanticsDataset()
	elem, ok := ds.ElementByKeyword("PatientName")
	if !ok {
		t.Fatal("ElementByKeyword(PatientName) not found")
	}
	if elem.Tag != MustTag(0x00100010) {
		t.Fatalf("ElementByKeyword(PatientName).Tag = %s", elem.Tag)
	}
	if _, ok := ds.ElementByKeyword("SOPClassUID"); ok {
		t.Fatal("ElementByKeyword(SOPClassUID) found an absent element")
	}
	if _, ok := ds.ElementByKeyword("NotAKeyword"); ok {
		t.Fatal("ElementByKeyword(NotAKeyword) found something")
	}
}
