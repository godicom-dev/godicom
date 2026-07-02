package godicom

import (
	"testing"
)

func TestDatasetCreateAndSet(t *testing.T) {
	ds := NewDataset()
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test^Name")
	ds.Set(elem)

	got, ok := ds.Get(MustTag(0x00100010))
	if !ok {
		t.Fatal("element not found")
	}
	if got.Value != "Test^Name" {
		t.Errorf("value = %v, want Test^Name", got.Value)
	}
}

func TestDatasetDelete(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00100010), VRPN, "Test"))
	ds.Delete(MustTag(0x00100010))
	if ds.Has(MustTag(0x00100010)) {
		t.Error("element should be deleted")
	}
}

func TestDatasetHas(t *testing.T) {
	ds := NewDataset()
	if ds.Has(MustTag(0x00100010)) {
		t.Error("should not have element")
	}
	ds.Set(NewDataElement(MustTag(0x00100010), VRPN, "Test"))
	if !ds.Has(MustTag(0x00100010)) {
		t.Error("should have element")
	}
}

func TestDatasetGetString(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00100010), VRPN, "Test^Name"))
	s, ok := ds.GetString(MustTag(0x00100010))
	if !ok || s != "Test^Name" {
		t.Errorf("got %q, %v", s, ok)
	}
}

func TestDatasetGetInt(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00280010), VRUS, 512))
	v, ok := ds.GetInt(MustTag(0x00280010))
	if !ok || v != 512 {
		t.Errorf("got %d, %v", v, ok)
	}
}

func TestDatasetGetFloat(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00186050), VRDS, 3.14))
	v, ok := ds.GetFloat(MustTag(0x00186050))
	if !ok || v != 3.14 {
		t.Errorf("got %f, %v", v, ok)
	}
}

func TestDatasetGetBytes(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x7FE00010), VROB, []byte{1, 2, 3}))
	v, ok := ds.GetBytes(MustTag(0x7FE00010))
	if !ok || len(v) != 3 || v[0] != 1 {
		t.Errorf("got %v, %v", v, ok)
	}
}

func TestDatasetSortedTags(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00100020), VRLO, "ID1"))
	ds.Set(NewDataElement(MustTag(0x00100010), VRPN, "Name1"))
	ds.Set(NewDataElement(MustTag(0x00080005), VRCS, "ISO_IR 100"))

	tags := ds.SortedTags()
	if len(tags) != 3 {
		t.Fatalf("got %d tags", len(tags))
	}
	if tags[0] != MustTag(0x00080005) {
		t.Errorf("first tag should be (0008,0005), got %s", tags[0])
	}
	if tags[1] != MustTag(0x00100010) {
		t.Errorf("second tag should be (0010,0010), got %s", tags[1])
	}
	if tags[2] != MustTag(0x00100020) {
		t.Errorf("third tag should be (0010,0020), got %s", tags[2])
	}
}

func TestDatasetIter(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00100020), VRLO, "ID1"))
	ds.Set(NewDataElement(MustTag(0x00100010), VRPN, "Name1"))

	elems := ds.Iter()
	if len(elems) != 2 {
		t.Fatalf("got %d elements", len(elems))
	}
	// Should be sorted by tag
	if elems[0].Tag != MustTag(0x00100010) {
		t.Errorf("first should be (0010,0010), got %s", elems[0].Tag)
	}
}

func TestDatasetElementsReturnsCopy(t *testing.T) {
	ds := NewDataset()
	tag := MustTag(0x00100010)
	ds.Set(NewDataElement(tag, VRPN, "Test^Name"))

	elements := ds.Elements()
	if len(elements) != 1 {
		t.Fatalf("len(Elements) = %d, want 1", len(elements))
	}
	delete(elements, tag)
	if !ds.Has(tag) {
		t.Fatal("mutating Elements map changed dataset")
	}
}

func TestDatasetGetSequence(t *testing.T) {
	ds := NewDataset()
	seqTag := MustTag(0x300A00B0)
	seq := NewSequence([]*Dataset{NewDataset(), NewDataset()})
	ds.Set(NewDataElement(seqTag, VRSQ, seq))

	got, ok := ds.GetSequence(seqTag)
	if !ok {
		t.Fatal("GetSequence ok = false")
	}
	if got != seq {
		t.Fatal("GetSequence returned wrong sequence")
	}
}

func TestDatasetGetSequenceMissingOrWrongType(t *testing.T) {
	ds := NewDataset()
	if got, ok := ds.GetSequence(MustTag(0x300A00B0)); ok || got != nil {
		t.Fatalf("missing GetSequence = %v, %t, want nil false", got, ok)
	}

	tag := MustTag(0x300A00B0)
	ds.Set(NewDataElement(tag, VRLO, "not sequence"))
	if got, ok := ds.GetSequence(tag); ok || got != nil {
		t.Fatalf("wrong type GetSequence = %v, %t, want nil false", got, ok)
	}
}

func TestDatasetValueWrappers(t *testing.T) {
	ds := NewDataset()
	stringTag := MustTag(0x00100010)
	intTag := MustTag(0x00280010)
	floatTag := MustTag(0x00186050)
	bytesTag := MustTag(0x7FE00010)
	sequenceTag := MustTag(0x300A00B0)
	seq := NewSequence([]*Dataset{NewDataset()})

	ds.Set(NewDataElement(stringTag, VRPN, "Test^Name"))
	ds.Set(NewDataElement(intTag, VRUS, uint16(512)))
	ds.Set(NewDataElement(floatTag, VRDS, 3.14))
	ds.Set(NewDataElement(bytesTag, VROB, []byte{1, 2, 3}))
	ds.Set(NewDataElement(sequenceTag, VRSQ, seq))

	if got, ok := ds.StringValue(stringTag); !ok || got != "Test^Name" {
		t.Fatalf("StringValue = %q, %t, want Test^Name true", got, ok)
	}
	if got, ok := ds.IntValue(intTag); !ok || got != 512 {
		t.Fatalf("IntValue = %d, %t, want 512 true", got, ok)
	}
	if got, ok := ds.FloatValue(floatTag); !ok || got != 3.14 {
		t.Fatalf("FloatValue = %f, %t, want 3.14 true", got, ok)
	}
	if got, ok := ds.BytesValue(bytesTag); !ok || len(got) != 3 || got[0] != 1 {
		t.Fatalf("BytesValue = %v, %t, want [1 2 3] true", got, ok)
	}
	if got, ok := ds.SequenceValue(sequenceTag); !ok || got != seq {
		t.Fatalf("SequenceValue = %v, %t, want sequence true", got, ok)
	}
}

func TestDatasetGetDataElement(t *testing.T) {
	ds := NewDataset()
	tag := MustTag(0x00100010)
	elem := NewDataElement(tag, VRPN, "Test^Name")
	ds.Set(elem)

	if got := ds.GetDataElement(tag); got != elem {
		t.Fatalf("GetDataElement = %v, want %v", got, elem)
	}
	if got := ds.GetDataElement(MustTag(0x00100020)); got != nil {
		t.Fatalf("missing GetDataElement = %v, want nil", got)
	}
}

func TestPrivateBlockSet(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00090010), VRLO, "MY_CREATOR"))
	pb := ds.PrivateBlock(0x0009, "MY_CREATOR")
	if pb == nil {
		t.Fatal("private block not found")
	}

	pb.Set(0x02, VRLO, "PrivateValue2")
	elem, ok := ds.Get(MustTag(0x00091002))
	if !ok {
		t.Fatal("private element not set")
	}
	if elem.Value != "PrivateValue2" {
		t.Fatalf("private element value = %v, want PrivateValue2", elem.Value)
	}
}
func TestDatasetPrivateBlock(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00090010), VRLO, "MY_CREATOR"))
	ds.Set(NewDataElement(MustTag(0x00091001), VRLO, "PrivateValue"))

	pb := ds.PrivateBlock(0x0009, "MY_CREATOR")
	if pb == nil {
		t.Fatal("private block not found")
	}

	elem, ok := pb.Get(0x01)
	if !ok {
		t.Fatal("private element not found")
	}
	if elem.Value != "PrivateValue" {
		t.Errorf("value = %v", elem.Value)
	}
}

func TestDatasetWalkRecursive(t *testing.T) {
	ds := NewDataset()
	item := NewDataset()
	item.Set(NewDataElement(MustTag(0x00100010), VRPN, "Nested"))
	ds.Set(NewDataElement(MustTag(0x00321060), VRSQ, NewSequence([]*Dataset{item})))

	var tags []Tag
	ds.Walk(func(_ *Dataset, elem *Element) {
		tags = append(tags, elem.Tag)
	}, true)

	want := []Tag{MustTag(0x00321060), MustTag(0x00100010)}
	if len(tags) != len(want) {
		t.Fatalf("walk tags = %v, want %v", tags, want)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Fatalf("walk tags = %v, want %v", tags, want)
		}
	}
}

func TestDatasetCloneDeepSequence(t *testing.T) {
	ds := NewDataset()
	item := NewDataset()
	item.Set(NewDataElement(MustTag(0x00100010), VRPN, "Nested"))
	ds.Set(NewDataElement(MustTag(0x00321060), VRSQ, NewSequence([]*Dataset{item})))

	clone := ds.Clone()
	item.Set(NewDataElement(MustTag(0x00100010), VRPN, "Changed"))

	seq, ok := clone.GetSequence(MustTag(0x00321060))
	if !ok {
		t.Fatal("sequence missing on clone")
	}
	name, ok := seq.Get(0).GetString(MustTag(0x00100010))
	if !ok || name != "Nested" {
		t.Fatalf("cloned nested name = %q, want Nested", name)
	}
}
