package godicom

import (
	"testing"
)

func TestSequenceEmpty(t *testing.T) {
	s := NewSequence(nil)
	if s.Len() != 0 {
		t.Errorf("Len = %d", s.Len())
	}
	if !s.IsEmpty() {
		t.Error("should be empty")
	}
}

func TestSequenceAppend(t *testing.T) {
	s := NewSequence(nil)
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00100010), VRPN, "Test"))
	s.Append(ds)
	if s.Len() != 1 {
		t.Errorf("Len = %d", s.Len())
	}
	got := s.Get(0)
	if got == nil {
		t.Fatal("Get returned nil")
	}
	v, ok := got.GetString(MustTag(0x00100010))
	if !ok || v != "Test" {
		t.Errorf("value = %q", v)
	}
}

func TestSequenceMultiple(t *testing.T) {
	ds1 := NewDataset()
	ds1.Set(NewDataElement(MustTag(0x00100010), VRPN, "Patient1"))
	ds2 := NewDataset()
	ds2.Set(NewDataElement(MustTag(0x00100010), VRPN, "Patient2"))

	s := NewSequence([]*Dataset{ds1, ds2})
	if s.Len() != 2 {
		t.Errorf("Len = %d", s.Len())
	}
}

func TestSequenceItems(t *testing.T) {
	ds1 := NewDataset()
	ds2 := NewDataset()
	s := NewSequence([]*Dataset{ds1, ds2})
	items := s.Items()
	if len(items) != 2 {
		t.Errorf("got %d items", len(items))
	}
}

func TestSequenceIsUndefinedLength(t *testing.T) {
	s := NewSequence(nil)
	if s.IsUndefinedLength {
		t.Error("default should be false")
	}
	s.IsUndefinedLength = true
	if !s.IsUndefinedLength {
		t.Error("should be true")
	}
}
