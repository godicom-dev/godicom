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

// A DS element holding a plain float64 -- what SetFloat stores -- has to reach
// the file the way the DS type would render it. fmt's %g is unbounded while
// PS3.5 caps DS at 16 bytes, so every value needing more significant characters
// than that used to be written over-long and invalid.
func TestDSFloatWriteObeysDSLength(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		val  float64
		want string
	}{
		{"one third", 1.0 / 3.0, "0.33333333333333"},
		{"two thirds", 2.0 / 3.0, "0.66666666666667"},
		{"negative third", -1.0 / 3.0, "-0.3333333333333"},
		{"large", 123456789012345678.0, "1.2345678901e+17"},
		{"tiny", 0.000000123456789012345, "1.2345678901e-07"},
		// Short enough already: must pass through untouched, not gain digits.
		{"exact", 2.5, "2.5"},
		{"integral", 5.0, "5.0"},
		{"zero", 0.0, "0.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := string(encodeNumberString(NewDataElement(MustTag("SliceThickness"), VRDS, c.val)))
			if got != c.want {
				t.Errorf("encoded %v as %q, want %q", c.val, got, c.want)
			}
			if len(got) > 16 {
				t.Errorf("encoded %v as %d bytes, PS3.5 caps DS at 16", c.val, len(got))
			}
			if !IsValidDS(got) {
				t.Errorf("encoded %v as %q, which IsValidDS rejects", c.val, got)
			}
			// The writer and the DS type must not disagree about the same value.
			viaType, err := DSFromFloat(c.val)
			if err != nil {
				t.Fatal(err)
			}
			if viaType.String() != got {
				t.Errorf("DSFromFloat renders %q but the writer wrote %q", viaType.String(), got)
			}
		})
	}
}

// The public setters are the reachable path to the bug above: SetFloat stores a
// bare float64 because DS is a float VR, so it went straight to the unbounded
// formatter. Check the whole encode/decode round trip, not just the formatter.
func TestSetFloatOnDSWritesValidDecimalString(t *testing.T) {
	t.Parallel()
	ds := NewDataset()
	if err := ds.SetFloat(MustTag("SliceThickness"), 1.0/3.0); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetFloats(MustTag("PixelSpacing"), 1.0/3.0, 2.0/3.0); err != nil {
		t.Fatal(err)
	}
	data, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeDataset(data, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}

	single, ok := back.Get(MustTag("SliceThickness"))
	if !ok {
		t.Fatal("SliceThickness absent after round trip")
	}
	got, ok := single.Value.(DS)
	if !ok {
		t.Fatalf("SliceThickness came back as %T", single.Value)
	}
	if !IsValidDS(got.Original) {
		t.Errorf("wrote SliceThickness as %q, which IsValidDS rejects", got.Original)
	}
	// 14 significant digits survive the 16-byte cap; the value is not mangled.
	if diff := got.Value - 1.0/3.0; diff > 1e-14 || diff < -1e-14 {
		t.Errorf("SliceThickness came back as %v, want ~%v", got.Value, 1.0/3.0)
	}

	multi, ok := back.Get(MustTag("PixelSpacing"))
	if !ok {
		t.Fatal("PixelSpacing absent after round trip")
	}
	mv, ok := multi.Value.(*MultiValue[DS])
	if !ok {
		t.Fatalf("PixelSpacing came back as %T", multi.Value)
	}
	if mv.Len() != 2 {
		t.Fatalf("PixelSpacing has %d values, want 2", mv.Len())
	}
	for i, v := range mv.Values() {
		if !IsValidDS(v.Original) {
			t.Errorf("PixelSpacing[%d] = %q, which IsValidDS rejects", i, v.Original)
		}
	}
}

// A DS read from a file keeps whatever string the file held, even an over-long
// one. Byte fidelity for values godicom did not compute outranks validity here,
// and the fix above must not be extended to this path -- doing so would break
// the byte-identical round trips.
func TestDSFromFileKeepsOverlongOriginal(t *testing.T) {
	t.Parallel()
	const overlong = "0.3333333333333333" // 18 bytes, as some scanners emit
	val, err := ParseDS(overlong)
	if err != nil {
		t.Fatal(err)
	}
	got := string(encodeNumberString(NewDataElement(MustTag("SliceThickness"), VRDS, val)))
	if got != overlong {
		t.Errorf("rewrote a parsed DS as %q, want the original %q", got, overlong)
	}
}

// IS is the other VR encodeNumberString serves, and it has its own rules. The
// setters reject a float for an IS tag, so this only guards that the DS change
// did not leak across VRs.
func TestISFloatFormattingUnchanged(t *testing.T) {
	t.Parallel()
	got := string(encodeNumberString(NewDataElement(MustTag("EchoNumbers"), VRIS, 1.5)))
	if got != "1.5" {
		t.Errorf("IS float encoded as %q, want %q", got, "1.5")
	}
	if err := NewDataset().SetFloat(MustTag("EchoNumbers"), 1.5); err == nil {
		t.Error("SetFloat on an IS tag should be rejected by the dictionary VR check")
	}
}
