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
