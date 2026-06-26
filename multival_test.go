package godicom

import (
	"testing"
)

func TestMultiValueInt(t *testing.T) {
	mv := NewMultiValue([]int{1, 2, 3})
	if mv.Len() != 3 {
		t.Errorf("Len = %d", mv.Len())
	}
	if mv.Get(0) != 1 {
		t.Errorf("Get(0) = %d", mv.Get(0))
	}
	mv.Append(4)
	if mv.Len() != 4 {
		t.Errorf("after append Len = %d", mv.Len())
	}
	mv.Set(0, 10)
	if mv.Get(0) != 10 {
		t.Errorf("after Set(0) = %d", mv.Get(0))
	}
	if mv.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestMultiValueEmpty(t *testing.T) {
	mv := NewMultiValue([]int{})
	if !mv.IsEmpty() {
		t.Error("should be empty")
	}
	if mv.Len() != 0 {
		t.Errorf("Len = %d", mv.Len())
	}
}

func TestMultiValueString(t *testing.T) {
	mv := NewMultiValue([]string{"a", "b"})
	if mv.Get(0) != "a" {
		t.Errorf("Get(0) = %q", mv.Get(0))
	}
}

func TestMultiValueFloat(t *testing.T) {
	mv := NewMultiValue([]float64{1.1, 2.2})
	if mv.Get(1) != 2.2 {
		t.Errorf("Get(1) = %f", mv.Get(1))
	}
}
