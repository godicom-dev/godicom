package godicom

import (
	"testing"
)

func TestParseDS(t *testing.T) {
	ds, err := ParseDS("1.5")
	if err != nil {
		t.Fatal(err)
	}
	if ds.Value != 1.5 || ds.String() != "1.5" {
		t.Fatalf("got Value=%g String=%q", ds.Value, ds.String())
	}
}

func TestParseDSPreservesOriginal(t *testing.T) {
	ds, err := ParseDS("2.260000")
	if err != nil {
		t.Fatal(err)
	}
	if ds.Original != "2.260000" || ds.String() != "2.260000" {
		t.Fatalf("Original=%q String=%q", ds.Original, ds.String())
	}
}

func TestParseIS(t *testing.T) {
	is, err := ParseIS("-128")
	if err != nil {
		t.Fatal(err)
	}
	if is.Value != -128 || is.String() != "-128" {
		t.Fatalf("got Value=%d String=%q", is.Value, is.String())
	}
}

func TestConvertDSFromBytes(t *testing.T) {
	v, err := convertDSString([]byte("0.3125"))
	if err != nil {
		t.Fatal(err)
	}
	ds, ok := v.(DS)
	if !ok || ds.Value != 0.3125 {
		t.Fatalf("got %T %v", v, v)
	}
}

func TestGetDSFromDataset(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PixelSpacing"), VRDS, "0.5\\0.5"))

	if _, ok := ds.GetDS(MustTag("PixelSpacing")); ok {
		t.Fatal("expected single-value GetDS to fail on multi-value")
	}
	if _, ok := ds.GetFloat(MustTag("PixelSpacing")); ok {
		t.Fatal("GetFloat should not match multi-value DS")
	}
}

func TestDSWriteRoundtrip(t *testing.T) {
	val, err := ParseDS("2.260000")
	if err != nil {
		t.Fatal(err)
	}
	out := encodeNumberString(NewDataElement(MustTag("SliceThickness"), VRDS, val))
	if string(out) != "2.260000" {
		t.Fatalf("encoded = %q", string(out))
	}
}
