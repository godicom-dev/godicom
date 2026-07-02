package godicom

import (
	"testing"
)

func TestConvertTag(t *testing.T) {
	tag := convertTag([]byte{0x10, 0x00, 0x20, 0x00}, true)
	if tag != MustTag(0x00100020) {
		t.Errorf("got %s", tag)
	}
	tag = convertTag([]byte{0x00, 0x10, 0x00, 0x20}, false)
	if tag != MustTag(0x00100020) {
		t.Errorf("got %s", tag)
	}
}

func TestConvertAEString(t *testing.T) {
	s, err := convertAEString([]byte("  AE_TITLE "))
	if err != nil {
		t.Fatal(err)
	}
	if s != "AE_TITLE" {
		t.Errorf("got %q", s)
	}
}

func TestConvertATValue(t *testing.T) {
	v, err := convertATValue([]byte{0x10, 0x00, 0x20, 0x00}, true)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := v.(Tag)
	if !ok || tag != MustTag(0x00100020) {
		t.Errorf("got %v", v)
	}
}

func TestConvertInts(t *testing.T) {
	// US: 2 bytes, little endian, unsigned
	v, err := convertInts([]byte{0x00, 0x02}, true, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if v != uint64(512) {
		t.Errorf("got %v", v)
	}
}

func TestConvertFloats(t *testing.T) {
	// FD: 8 bytes, little endian
	v, err := convertFloats([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x40}, true, 8)
	if err != nil {
		t.Fatal(err)
	}
	f, ok := v.(float64)
	if !ok || f != 3.0 {
		t.Errorf("got %v", f)
	}
}

func TestConvertUI(t *testing.T) {
	uid, err := convertUI([]byte("1.2.840.10008.1.2\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if uid != "1.2.840.10008.1.2" {
		t.Errorf("got %q", uid)
	}
}

func TestConvertPN(t *testing.T) {
	pn, err := convertPN([]byte("Smith^John"))
	if err != nil {
		t.Fatal(err)
	}
	if pn.Alphabetic != "Smith^John" {
		t.Errorf("got %q", pn.Alphabetic)
	}
}

func TestConvertDSString(t *testing.T) {
	v, err := convertDSString([]byte("3.14"))
	if err != nil {
		t.Fatal(err)
	}
	f, ok := v.(DS)
	if !ok || f.Value != 3.14 {
		t.Errorf("got %v", v)
	}
}

func TestConvertISString(t *testing.T) {
	v, err := convertISString([]byte("42"))
	if err != nil {
		t.Fatal(err)
	}
	i, ok := v.(IS)
	if !ok || i.Value != 42 {
		t.Errorf("got %v", v)
	}
}
