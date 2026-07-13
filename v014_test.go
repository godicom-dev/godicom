package godicom

import (
	"testing"
)

// pydicom.tests.test_dataset.TestDataset.test_clear
func TestDatasetClear(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "CITIZEN^Jan"))
	ds.Set(NewDataElement(MustTag("PatientID"), VRLO, "1234"))
	if ds.Len() != 2 {
		t.Fatalf("len = %d", ds.Len())
	}
	ds.Clear()
	if ds.Len() != 0 {
		t.Fatalf("after Clear len = %d", ds.Len())
	}
	if _, ok := ds.Get(MustTag("PatientName")); ok {
		t.Fatal("PatientName still present")
	}
}

// pydicom.tests.test_dataset.TestDataset.test_pop
func TestDatasetPop(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "CITIZEN^Jan"))
	ds.Set(NewDataElement(MustTag("PatientID"), VRLO, "1234"))

	elem, ok := ds.Pop(MustTag("PatientName"))
	if !ok {
		t.Fatal("Pop failed")
	}
	switch v := elem.Value.(type) {
	case string:
		if v != "CITIZEN^Jan" {
			t.Fatalf("Pop value = %q", v)
		}
	case PersonName:
		if v.String() != "CITIZEN^Jan" {
			t.Fatalf("Pop value = %q", v.String())
		}
	default:
		t.Fatalf("Pop type = %T", elem.Value)
	}
	if _, ok := ds.Get(MustTag("PatientName")); ok {
		t.Fatal("PatientName still present after Pop")
	}
	if _, ok := ds.Pop(MustTag("PatientName")); ok {
		t.Fatal("second Pop should fail")
	}
}

// pydicom.tests.test_dataset.TestDataset.test_update_with_dataset
func TestDatasetUpdate(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "A"))
	ds.Set(NewDataElement(MustTag("PatientID"), VRLO, "1"))

	other := NewDataset()
	other.Set(NewDataElement(MustTag("PatientName"), VRPN, "B"))
	other.Set(NewDataElement(MustTag("StudyDate"), VRDA, "20200101"))
	ds.Update(other)

	name, _ := ds.GetString(MustTag("PatientName"))
	if name != "B" {
		t.Fatalf("PatientName = %q", name)
	}
	id, _ := ds.GetString(MustTag("PatientID"))
	if id != "1" {
		t.Fatalf("PatientID = %q", id)
	}
	date, _ := ds.GetString(MustTag("StudyDate"))
	if date != "20200101" {
		t.Fatalf("StudyDate = %q", date)
	}
}

// pydicom.tests.test_dataset.TestDataset.test_group_dataset
func TestDatasetGroupDataset(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00080018), VRUI, "1.2.3"))
	ds.Set(NewDataElement(MustTag(0x00100010), VRPN, "Name"))
	ds.Set(NewDataElement(MustTag(0x00100020), VRLO, "ID"))

	g10 := ds.GroupDataset(0x0010)
	if g10.Len() != 2 {
		t.Fatalf("group 0010 len = %d", g10.Len())
	}
	if _, ok := g10.Get(MustTag(0x00080018)); ok {
		t.Fatal("group 0008 element leaked into group dataset")
	}
	if _, ok := g10.Get(MustTag(0x00100010)); !ok {
		t.Fatal("PatientName missing from group dataset")
	}
}

// pydicom.tests.test_dataset.TestDataset.test_remove_private_tags
func TestDatasetRemovePrivateTags(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00000000), VRUL, uint32(120)))
	ds.Set(NewDataElement(MustTag(0x00089460), VRCS, "TEST"))
	ds.Set(NewDataElement(MustTag(0x00090001), VRPN, "CITIZEN^1"))
	ds.Set(NewDataElement(MustTag(0x00090010), VRPN, "CITIZEN^10"))
	ds.Set(NewDataElement(MustTag(0x00100010), VRPN, "CITIZEN^Jan"))

	item := NewDataset()
	item.Set(NewDataElement(MustTag(0x00090001), VRLO, "nested-private"))
	item.Set(NewDataElement(MustTag(0x00100020), VRLO, "keep"))
	ds.Set(NewDataElement(MustTag(0x300A00B0), VRSQ, NewSequence([]*Dataset{item})))

	ds.RemovePrivateTags()

	if _, ok := ds.Get(MustTag(0x00090001)); ok {
		t.Fatal("private tag still present")
	}
	if _, ok := ds.Get(MustTag(0x00090010)); ok {
		t.Fatal("private creator still present")
	}
	if _, ok := ds.Get(MustTag(0x00100010)); !ok {
		t.Fatal("PatientName removed")
	}
	seq, ok := ds.GetSequence(MustTag(0x300A00B0))
	if !ok || seq.Len() != 1 {
		t.Fatal("sequence missing")
	}
	if _, ok := seq.Get(0).Get(MustTag(0x00090001)); ok {
		t.Fatal("nested private tag still present")
	}
	if _, ok := seq.Get(0).Get(MustTag(0x00100020)); !ok {
		t.Fatal("nested public tag removed")
	}
}

// pydicom.tests.test_dataset.TestDataset.test_data_element
func TestDatasetElementByKeyword(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "ANON"))
	ds.Set(NewDataElement(MustTag("BeamSequence"), VRSQ, NewSequence([]*Dataset{NewDataset()})))

	elem, ok := ds.ElementByKeyword("PatientName")
	if !ok || elem.Tag != MustTag("PatientName") {
		t.Fatalf("ElementByKeyword(PatientName) = %v ok=%v", elem, ok)
	}
	if _, ok := ds.ElementByKeyword("not an element keyword"); ok {
		t.Fatal("expected missing keyword")
	}
	seqElem, ok := ds.ElementByKeyword("BeamSequence")
	if !ok || seqElem.VR != VRSQ {
		t.Fatal("BeamSequence missing")
	}
}

// pydicom.tests.test_dataset.TestDataset.test_iterall
func TestDatasetIterAll(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00000000), VRUL, uint32(120)))
	ds.Set(NewDataElement(MustTag(0x00089460), VRCS, "TEST"))
	ds.Set(NewDataElement(MustTag(0x00090001), VRPN, "CITIZEN^1"))
	item := NewDataset()
	item.Set(NewDataElement(MustTag(0x00100010), VRPN, "ANON"))
	ds.Set(NewDataElement(MustTag(0x300A00B0), VRSQ, NewSequence([]*Dataset{item})))

	got := ds.IterAll()
	want := []Tag{
		MustTag(0x00000000),
		MustTag(0x00089460),
		MustTag(0x00090001),
		MustTag(0x300A00B0),
		MustTag(0x00100010),
	}
	if len(got) != len(want) {
		t.Fatalf("IterAll len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Tag != want[i] {
			t.Fatalf("IterAll[%d] = %s, want %s", i, got[i].Tag, want[i])
		}
	}
}

// pydicom.tests.test_dataset.TestDataset.test_equality_no_sequence / test_equality_sequence
func TestDatasetEqual(t *testing.T) {
	if !NewDataset().Equal(NewDataset()) {
		t.Fatal("empty datasets should be equal")
	}

	d := NewDataset()
	d.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, "1.2.3.4"))
	d.Set(NewDataElement(MustTag("PatientName"), VRPN, "Test"))

	e := NewDataset()
	e.Set(NewDataElement(MustTag("PatientName"), VRPN, "Test"))
	e.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, "1.2.3.4"))
	if !d.Equal(e) {
		t.Fatal("datasets with same elements should be equal")
	}

	e.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, "1.2.3.5"))
	if d.Equal(e) {
		t.Fatal("different UID should not be equal")
	}

	// Sequence equality
	d = NewDataset()
	d.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, "1.2.3.4"))
	item := NewDataset()
	item.Set(NewDataElement(MustTag("PatientID"), VRLO, "1234"))
	d.Set(NewDataElement(MustTag("BeamSequence"), VRSQ, NewSequence([]*Dataset{item})))

	e = d.Clone()
	if !d.Equal(e) {
		t.Fatal("cloned sequence dataset should be equal")
	}
	seq, _ := e.GetSequence(MustTag("BeamSequence"))
	seq.Get(0).Set(NewDataElement(MustTag("PatientID"), VRLO, "9999"))
	if d.Equal(e) {
		t.Fatal("modified nested value should not be equal")
	}
}

// pydicom.tests.test_dataset.TestDataset.test_is_original_encoding
func TestDatasetIsOriginalEncoding(t *testing.T) {
	ds := NewDataset()
	if ds.IsOriginalEncoding() {
		t.Fatal("new dataset should not report original encoding")
	}

	ds.Set(NewDataElement(MustTag("SpecificCharacterSet"), VRCS, "ISO_IR 100"))
	ds.SetOriginalEncoding(true, true, []string{"ISO_IR 100"})
	if !ds.IsOriginalEncoding() {
		t.Fatal("expected original encoding after SetOriginalEncoding")
	}

	ds.Set(NewDataElement(MustTag("SpecificCharacterSet"), VRCS, "ISO_IR 192"))
	if ds.IsOriginalEncoding() {
		t.Fatal("changed character set should not be original")
	}
	ds.Set(NewDataElement(MustTag("SpecificCharacterSet"), VRCS, "ISO_IR 100"))
	if !ds.IsOriginalEncoding() {
		t.Fatal("restored character set should be original")
	}

	ds.SetWriteEncoding(true, false)
	if ds.IsOriginalEncoding() {
		t.Fatal("endianness change should not be original")
	}
	ds.SetWriteEncoding(true, true)
	if !ds.IsOriginalEncoding() {
		t.Fatal("restored endianness should be original")
	}
	ds.SetWriteEncoding(false, true)
	if ds.IsOriginalEncoding() {
		t.Fatal("VR change should not be original")
	}
}

func TestDatasetIsOriginalEncodingAfterRead(t *testing.T) {
	ds, err := ReadFile(requireCharsetFile(t, "chrFren.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ds.IsOriginalEncoding() {
		t.Fatal("freshly read dataset should be original encoding")
	}
	ds.Set(NewDataElement(MustTag("SpecificCharacterSet"), VRCS, "ISO_IR 192"))
	if ds.IsOriginalEncoding() {
		t.Fatal("charset change after read should clear original encoding")
	}
}

func TestDatasetWalkNonRecursive(t *testing.T) {
	ds := NewDataset()
	item := NewDataset()
	item.Set(NewDataElement(MustTag(0x00100010), VRPN, "Nested"))
	ds.Set(NewDataElement(MustTag(0x00321060), VRSQ, NewSequence([]*Dataset{item})))
	ds.Set(NewDataElement(MustTag(0x00100020), VRLO, "ID"))

	var tags []Tag
	ds.Walk(func(_ *Dataset, elem *Element) {
		tags = append(tags, elem.Tag)
	}, false)

	if len(tags) != 2 {
		t.Fatalf("non-recursive walk len = %d, want 2 (%v)", len(tags), tags)
	}
}

func TestFileDatasetClonePreservesMeta(t *testing.T) {
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	clone := ds.Clone()
	if !clone.Equal(ds.Dataset) {
		t.Fatal("cloned dataset content mismatch")
	}
	if !clone.IsOriginalEncoding() {
		t.Fatal("clone should preserve original encoding state")
	}
}
