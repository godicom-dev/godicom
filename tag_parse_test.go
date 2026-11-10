package godicom

import "testing"

func TestParseTagAcceptedForms(t *testing.T) {
	want := MustTag("PatientName")
	for _, tc := range []struct {
		name string
		arg  interface{}
		rest []int
	}{
		{"tag passthrough", want, nil},
		{"hex integer", 0x00100010, nil},
		{"uint32", uint32(0x00100010), nil},
		{"group and element variant", 0x0010, []int{0x0010}},
		{"pair of ints", [2]int{0x0010, 0x0010}, nil},
		{"pair of uint32", [2]uint32{0x0010, 0x0010}, nil},
		{"keyword", "PatientName", nil},
		{"hex string", "00100010", nil},
		{"parenthesised", "(0010,0010)", nil},
		{"parenthesised with spaces", "( 0010 , 0010 )", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTag(tc.arg, tc.rest...)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}

	// The two-argument form takes the group as either int or uint32.
	for _, g := range []interface{}{0x0010, uint32(0x0010), uint16(0x0010), int32(0x0010)} {
		if got, err := ParseTag(g, 0x0010); err != nil || got != want {
			t.Fatalf("group %#v -> %s %v", g, got, err)
		}
	}
	// Anything toInt does not know becomes group 0 rather than an error, which is
	// the same choice the single-argument form makes for a missing tag.
	if got, err := ParseTag("nonsense", 0x0010); err != nil || uint32(got) != 0x0010 {
		t.Fatalf("unknown group -> %#v %v", got, err)
	}
}

func TestParseTagRejections(t *testing.T) {
	for _, tc := range []struct {
		name string
		arg  interface{}
	}{
		{"unknown keyword", "NotAKeyword"},
		{"overlong hex", "100000000"},
		{"float", 1.5},
		{"nil", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseTag(tc.arg); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestToIntTypes(t *testing.T) {
	for _, tc := range []struct {
		in   interface{}
		want int
	}{
		{0x10, 0x10},
		{uint32(0x10), 0x10},
		{uint16(0x10), 0x10},
		{int32(0x10), 0x10},
		{"16", 0},
	} {
		if got := toInt(tc.in); got != tc.want {
			t.Fatalf("toInt(%#v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
