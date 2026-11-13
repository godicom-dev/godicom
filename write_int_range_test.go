package godicom

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom/uid"
)

// The value encoders used to answer a value whose Go type they could not encode
// with nil, and the writer wrote that nil as a zero-length element: the value
// disappeared and the file still opened. Narrow integer VRs were worse -- the
// uint32-to-US path truncated, so 70000 came back as 4464. Both are now errors.
//
// pydicom refuses the same values, at pack time:
//
//	US 70000 -> OSError: 'H' format requires 0 <= number <= 65535
//	SS 40000 -> OSError: 'h' format requires -32768 <= number <= 32767
//	UL    -5 -> OSError: 'L' format requires 0 <= number <= 4294967295
//	OW [1,2] -> TypeError: a bytes-like object is required

func encodeOne(t *testing.T, vr VR, value any) ([]byte, error) {
	t.Helper()
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00090010), vr, value))
	return ds.Encode(uid.ExplicitVRLittleEndian)
}

func TestWriteRejectsIntegerOutOfRange(t *testing.T) {
	cases := []struct {
		vr    VR
		value any
	}{
		{VRUS, 65536},
		{VRUS, 70000},
		{VRUS, -1},
		{VRSS, 32768},
		{VRSS, 40000},
		{VRSS, -32769},
		{VRUL, -5},
		{VRUL, int64(1 << 32)},
		{VRSL, int64(1 << 31)},
		{VRSL, int64(-(1 << 31) - 1)},
	}
	for _, tc := range cases {
		err := errorFrom(t, tc.vr, tc.value)
		if !strings.Contains(err.Error(), "does not fit in "+string(tc.vr)) {
			t.Errorf("%s with %v: error = %q, want it to name the VR", tc.vr, tc.value, err)
		}
	}
}

// errorFrom re-runs the encoding to hand back its error for message assertions.
func errorFrom(t *testing.T, vr VR, value any) error {
	t.Helper()
	_, err := encodeOne(t, vr, value)
	if err == nil {
		t.Fatalf("%s with %v encoded without error", vr, value)
	}
	return err
}

func TestWriteAcceptsIntegerBoundaries(t *testing.T) {
	cases := []struct {
		vr    VR
		value any
		want  []byte
	}{
		{VRUS, 0, []byte{0, 0}},
		{VRUS, 65535, []byte{0xFF, 0xFF}},
		{VRSS, -32768, []byte{0x00, 0x80}},
		{VRSS, 32767, []byte{0xFF, 0x7F}},
		{VRUL, int64(4294967295), []byte{0xFF, 0xFF, 0xFF, 0xFF}},
		{VRSL, -2147483648, []byte{0x00, 0x00, 0x00, 0x80}},
		{VRSL, 2147483647, []byte{0xFF, 0xFF, 0xFF, 0x7F}},
	}
	for _, tc := range cases {
		out, err := encodeOne(t, tc.vr, tc.value)
		if err != nil {
			t.Errorf("%s with %v: unexpected error %v", tc.vr, tc.value, err)
			continue
		}
		back, ok := readBare(t, out).Get(MustTag(0x00090010))
		if !ok {
			t.Fatalf("%s with %v: element missing from the encoded dataset", tc.vr, tc.value)
		}
		if len(back.RawValue) != len(tc.want) {
			t.Errorf("%s with %v: %d bytes, want %d", tc.vr, tc.value, len(back.RawValue), len(tc.want))
			continue
		}
		for i := range tc.want {
			if back.RawValue[i] != tc.want[i] {
				t.Errorf("%s with %v: bytes = % x, want % x", tc.vr, tc.value, back.RawValue, tc.want)
				break
			}
		}
	}
}

// Go has no implicit integer conversions, so a caller who wrote uint32(7) meant
// seven. Widening the accepted types is what keeps the range check from turning
// a correct call into an error; only the range itself is refused.
func TestWriteAcceptsAnyIntegerTypeInRange(t *testing.T) {
	values := []any{int(7), int8(7), uint8(7), int16(7), uint16(7), int32(7), uint32(7), int64(7), uint64(7)}
	for _, value := range values {
		out, err := encodeOne(t, VRUS, value)
		if err != nil {
			t.Errorf("US with %T(7): unexpected error %v", value, err)
			continue
		}
		if !strings.HasSuffix(string(out), "\x02\x00\x07\x00") {
			t.Errorf("US with %T(7): bytes = % x, want a 2-byte 7", value, out)
		}
	}

	// The slice forms are how a multi-valued US or OW is set, and uint16 is the
	// width the VR itself has.
	slices := []any{[]int{7, 8}, []uint16{7, 8}, []uint32{7, 8}, []uint64{7, 8}}
	for _, value := range slices {
		out, err := encodeOne(t, VRUS, value)
		if err != nil {
			t.Errorf("US with %T{7,8}: unexpected error %v", value, err)
			continue
		}
		if !strings.HasSuffix(string(out), "\x04\x00\x07\x00\x08\x00") {
			t.Errorf("US with %T{7,8}: bytes = % x, want two 2-byte values", value, out)
		}
	}
}

func TestWriteRejectsValueTheVRCannotHold(t *testing.T) {
	cases := []struct {
		vr    VR
		value any
		want  string
	}{
		{VRUS, "abc", "cannot encode string value as US"},
		{VRUL, 1.5, "cannot encode float64 value as UL"},
		{VRFL, "abc", "cannot encode string value as FL"},
		{VRFD, []byte{1, 2, 3}, "cannot encode []uint8 value as FD"},
		{VRAT, uint32(0x00100010), "cannot encode uint32 value as AT"},
		{VRAT, []uint32{0x00100010}, "cannot encode []uint32 value as AT"},
		{VROB, uint32(7), "cannot encode uint32 value as OB"},
		{VRUN, uint32(7), "cannot encode uint32 value as UN"},
		{VROW, []uint16{1, 2}, "cannot encode []uint16 value as OW"},
	}
	for _, tc := range cases {
		err := errorFrom(t, tc.vr, tc.value)
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s with %T: error = %q, want %q", tc.vr, tc.value, err, tc.want)
		}
		if !strings.Contains(err.Error(), "0009,0010") {
			t.Errorf("%s with %T: error = %q, want it to name the tag", tc.vr, tc.value, err)
		}
	}
}

// A MultiValue[any] can hold a mix, and the per-item loop is where a bad element
// used to be skipped rather than reported.
func TestWriteRejectsUnencodableMultiValueItem(t *testing.T) {
	_, err := encodeOne(t, VRUS, NewMultiValue[any]([]any{1, "two"}))
	if err == nil {
		t.Fatal("US with a MultiValue[any] holding a string encoded without error")
	}
	if !strings.Contains(err.Error(), "cannot encode string value as US") {
		t.Errorf("error = %q, want it to name the offending item", err)
	}
}

// The guard must not turn a deliberate empty value into an error: a zero-length
// element is legal for these VRs and is how a cleared value is represented.
func TestWriteKeepsEmptyBinaryValuesEmpty(t *testing.T) {
	cases := []struct {
		vr    VR
		value any
	}{
		{VROB, []byte{}},
		{VROB, []byte(nil)},
		{VRUN, []byte{}},
		{VRAT, []Tag{}},
		{VRUS, []int{}},
		{VRFD, NewMultiValue[float64](nil)},
	}
	for _, tc := range cases {
		out, err := encodeOne(t, tc.vr, tc.value)
		if err != nil {
			t.Errorf("%s with %T%s: unexpected error %v", tc.vr, tc.value, "", err)
			continue
		}
		elem, ok := readBare(t, out).Get(MustTag(0x00090010))
		if !ok {
			t.Errorf("%s with %T: element missing", tc.vr, tc.value)
			continue
		}
		if len(elem.RawValue) != 0 {
			t.Errorf("%s with %T: wrote % x, want a zero-length value", tc.vr, tc.value, elem.RawValue)
		}
	}
}

// The refusal has to survive the path a caller actually uses, not just Encode.
func TestSaveAsRefusesOutOfRangeValue(t *testing.T) {
	ds := NewDataset()
	_ = ds.SetString(MustTag("PatientName"), "Doe^John")
	ds.Set(NewDataElement(MustTag(0x00280106), VRUS, 70000))

	path := filepath.Join(t.TempDir(), "out.dcm")
	err := samplePart10With(t, ds, ExplicitVRLittleEndian).SaveAs(path, nil)
	if err == nil {
		t.Fatal("SaveAs() with a US value of 70000 returned no error")
	}
	if !strings.Contains(err.Error(), "does not fit in US") {
		t.Errorf("SaveAs() error = %q, want it to explain the range", err)
	}
}

// AT and the float VRs take several Go forms; the type switches were rewritten
// to report a type they cannot handle, so the forms they do handle are pinned
// here rather than left to whichever test happens to build one.
func TestWriteAcceptsATAndFloatForms(t *testing.T) {
	atForms := map[string]any{
		"Tag":                   MustTag(0x00100010),
		"int":                   0x00100010,
		"[]Tag":                 []Tag{MustTag(0x00100010)},
		"[]int":                 []int{0x00100010},
		"MultiValue[Tag]":       NewMultiValue[Tag]([]Tag{MustTag(0x00100010)}),
		"MultiValue[int]":       NewMultiValue[int]([]int{0x00100010}),
		"MultiValue[interface]": NewMultiValue[any]([]any{MustTag(0x00100010)}),
	}
	for name, value := range atForms {
		out, err := encodeOne(t, VRAT, value)
		if err != nil {
			t.Errorf("AT with %s: unexpected error %v", name, err)
			continue
		}
		if !strings.HasSuffix(string(out), "\x04\x00\x10\x00\x10\x00") {
			t.Errorf("AT with %s: bytes = % x, want (0010,0010) as two 16-bit halves", name, out)
		}
	}

	oneAndAHalf := "\x04\x00\x00\x00\xc0\x3f"
	floatForms := []struct {
		name  string
		value any
		want  string
	}{
		{"float64", 1.5, oneAndAHalf},
		{"float32", float32(1.5), oneAndAHalf},
		{"MultiValue[float64]", NewMultiValue[float64]([]float64{1.5}), oneAndAHalf},
		{"MultiValue[interface]", NewMultiValue[any]([]any{float32(1.5)}), oneAndAHalf},
		// An int in a float VR is a whole number, not a fraction: 1 is 1.0f.
		{"int", 1, "\x04\x00\x00\x00\x80\x3f"},
	}
	for _, tc := range floatForms {
		out, err := encodeOne(t, VRFL, tc.value)
		if err != nil {
			t.Errorf("FL with %s: unexpected error %v", tc.name, err)
			continue
		}
		if !strings.HasSuffix(string(out), tc.want) {
			t.Errorf("FL with %s: bytes = % x, want it to end with % x", tc.name, out, tc.want)
		}
	}
}
