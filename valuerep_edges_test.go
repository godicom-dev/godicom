package godicom

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestDAStringFormatting(t *testing.T) {
	// Original wins when present, so a value keeps the spelling it was read with.
	if got := (DA{Original: "20240102"}).String(); got != "20240102" {
		t.Fatalf("got %q", got)
	}
	// Built from a time, it has to render the DICOM form.
	d := DA{Time: time.Date(2024, 2, 3, 0, 0, 0, 0, time.UTC)}
	if got := d.String(); got != "20240203" {
		t.Fatalf("got %q, want 20240203", got)
	}
	if got := (DA{}).String(); got != "" {
		t.Fatalf("zero DA rendered %q", got)
	}
}

func TestTMStringFormatting(t *testing.T) {
	if got := (TM{Original: "1200"}).String(); got != "1200" {
		t.Fatalf("got %q", got)
	}
	tm := TM{Time: time.Date(1, 1, 1, 13, 4, 5, 0, time.UTC)}
	if got := tm.String(); got != "130405" {
		t.Fatalf("got %q, want 130405", got)
	}
	if got := (TM{}).String(); got != "" {
		t.Fatalf("zero TM rendered %q", got)
	}
}

func TestDTStringFormatting(t *testing.T) {
	if got := (DT{Original: "20240102130405"}).String(); got != "20240102130405" {
		t.Fatalf("got %q", got)
	}
	dt := DT{Time: time.Date(2024, 1, 2, 13, 4, 5, 0, time.UTC)}
	if got := dt.String(); got != "20240102130405" {
		t.Fatalf("got %q", got)
	}
	if got := (DT{}).String(); got != "" {
		t.Fatalf("zero DT rendered %q", got)
	}
}

func TestParseTMPrecisionAndLeapSecond(t *testing.T) {
	// A fractional part of fewer than six digits is microseconds, not
	// nanoseconds: "5" means .5 s, and the value must round-trip as written.
	for _, tc := range []struct {
		in   string
		nsec int
	}{
		{"120000.5", 500000000},
		{"120000.123456", 123456000},
		{"120000.000001", 1000},
	} {
		tm, err := ParseTM(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if tm.Time.Nanosecond() != tc.nsec {
			t.Fatalf("%s: nsec = %d, want %d", tc.in, tm.Time.Nanosecond(), tc.nsec)
		}
	}

	// 60 is a leap second and is clamped rather than rejected.
	tm, err := ParseTM("235960")
	if err != nil {
		t.Fatal(err)
	}
	if tm.Time.Second() != 59 {
		t.Fatalf("leap second = %d, want 59", tm.Time.Second())
	}

	// "12" is legal: TM may carry only the hour. Anything past that has to be
	// complete, and the hour cannot exceed 23.
	for _, bad := range []string{"24", "12:00", "126000", "12345"} {
		if _, err := ParseTM(bad); err == nil {
			t.Fatalf("%q should not parse", bad)
		}
	}
}

func TestParseDTPartialFormsAndTimezone(t *testing.T) {
	// DT allows the value to stop at any component, and the missing ones default.
	for _, tc := range []struct {
		in                   string
		year, month, day     int
		hour, minute, second int
	}{
		{"2024", 2024, 1, 1, 0, 0, 0},
		{"202402", 2024, 2, 1, 0, 0, 0},
		{"20240203", 2024, 2, 3, 0, 0, 0},
		{"2024020313", 2024, 2, 3, 13, 0, 0},
		{"202402031304", 2024, 2, 3, 13, 4, 0},
		{"20240203130405", 2024, 2, 3, 13, 4, 5},
	} {
		dt, err := ParseDT(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		got := dt.Time
		if got.Year() != tc.year || int(got.Month()) != tc.month || got.Day() != tc.day ||
			got.Hour() != tc.hour || got.Minute() != tc.minute || got.Second() != tc.second {
			t.Fatalf("%s parsed as %s", tc.in, got)
		}
	}

	dt, err := ParseDT("20240203130405.123456+0530")
	if err != nil {
		t.Fatal(err)
	}
	_, offset := dt.Time.Zone()
	if offset != 5*3600+30*60 {
		t.Fatalf("offset = %d, want 19800", offset)
	}
	if dt.Time.Nanosecond() != 123456000 {
		t.Fatalf("nsec = %d", dt.Time.Nanosecond())
	}
	if dt.String() != "20240203130405.123456+0530" {
		t.Fatalf("round trip = %q", dt.String())
	}

	neg, err := ParseDT("20240203130405-0800")
	if err != nil {
		t.Fatal(err)
	}
	if _, offset := neg.Time.Zone(); offset != -8*3600 {
		t.Fatalf("negative offset = %d", offset)
	}

	if _, err := ParseDT("202402031304056000001"); err == nil {
		t.Fatal("expected length error")
	}
	if _, err := ParseDT("notadate"); err == nil {
		t.Fatal("expected format error")
	}
}

func TestParseValueWrappers(t *testing.T) {
	// An absent value is the empty string, not a zero-valued struct, because the
	// writer has to be able to tell "no value" from "1900-01-01".
	if v, err := parseDAValue(""); err != nil || v != "" {
		t.Fatalf("empty DA = %v %v", v, err)
	}
	if v, err := parseTMValue(""); err != nil || v != "" {
		t.Fatalf("empty TM = %v %v", v, err)
	}
	if v, err := parseDTValue(""); err != nil || v != "" {
		t.Fatalf("empty DT = %v %v", v, err)
	}
	if v, err := parseDSValue(""); err != nil || v != nil {
		t.Fatalf("empty DS = %v %v", v, err)
	}

	if v, err := parseDAValue("20240102"); err != nil {
		t.Fatal(err)
	} else if da, ok := v.(DA); !ok || da.Original != "20240102" {
		t.Fatalf("DA = %T %v", v, v)
	}
	if v, err := parseTMValue("120000"); err != nil {
		t.Fatal(err)
	} else if _, ok := v.(TM); !ok {
		t.Fatalf("TM = %T", v)
	}
	if v, err := parseDTValue("20240102120000"); err != nil {
		t.Fatal(err)
	} else if _, ok := v.(DT); !ok {
		t.Fatalf("DT = %T", v)
	}

	if _, err := parseDAValue("2024"); err == nil {
		t.Fatal("expected DA error")
	}
	if _, err := parseTMValue("99"); err == nil {
		t.Fatal("expected TM error")
	}
	if _, err := parseDTValue("x"); err == nil {
		t.Fatal("expected DT error")
	}
}

func TestDSStringAndZero(t *testing.T) {
	if got := (DS{Value: 1.5, Original: "1.50"}).String(); got != "1.50" {
		t.Fatalf("got %q", got)
	}
	if got := (DS{Value: 1.5}).String(); got != "1.5" {
		t.Fatalf("got %q", got)
	}
	if !(DS{}).IsZero() {
		t.Fatal("zero DS should report zero")
	}
	if (DS{Value: 0, Original: "0"}).IsZero() {
		t.Fatal("an explicit zero keeps its spelling and is not the absent value")
	}
}

func TestISStringAndZero(t *testing.T) {
	if got := (IS{Value: 7, Original: "007"}).String(); got != "007" {
		t.Fatalf("got %q", got)
	}
	if got := (IS{Value: 7}).String(); got != "7" {
		t.Fatalf("got %q", got)
	}
	if !(IS{}).IsZero() {
		t.Fatal("zero IS should report zero")
	}
}

func TestIsValidDSAndIS(t *testing.T) {
	for _, ok := range []string{"1", "-1", "+1", "1.", ".5", "1.5", "1e3", "1.5e-3", " 1 ", "0"} {
		if !IsValidDS(ok) {
			t.Fatalf("%q should be a valid DS", ok)
		}
	}
	for _, bad := range []string{"", "1,5", "abc", "1.2.3", "--1", "e3", strings.Repeat("1", 17)} {
		if IsValidDS(bad) {
			t.Fatalf("%q should not be a valid DS", bad)
		}
	}

	for _, ok := range []string{"1", "-1", "+1", " 42 "} {
		if !IsValidIS(ok) {
			t.Fatalf("%q should be a valid IS", ok)
		}
	}
	for _, bad := range []string{"", "1.5", "abc", strings.Repeat("1", 13)} {
		if IsValidIS(bad) {
			t.Fatalf("%q should not be a valid IS", bad)
		}
	}
}

func TestISInRange(t *testing.T) {
	for _, v := range []int64{0, -2147483648, 2147483647} {
		if !ISInRange(v) {
			t.Fatalf("%d should be in range", v)
		}
	}
	for _, v := range []int64{-2147483649, 2147483648} {
		if ISInRange(v) {
			t.Fatalf("%d should be out of range", v)
		}
	}
}

func TestFormatNumberAsDSEdgeCases(t *testing.T) {
	// Values at the ends of the exponent range switch to scientific notation
	// instead of printing a long run of zeros or digits, and every result has to
	// fit the 16-character DS field and be re-readable.
	for _, val := range []float64{1e20, -1e20, 1e-9, -1e-9, 123456789012345.0, 1.0 / 3.0, 70.5} {
		got, err := FormatNumberAsDS(val)
		if err != nil {
			t.Fatalf("%v: %v", val, err)
		}
		if len(got) > maxDSLength {
			t.Fatalf("%v -> %q is %d characters", val, got, len(got))
		}
		if !IsValidDS(got) {
			t.Fatalf("%v produced an invalid DS %q", val, got)
		}
	}

	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := FormatNumberAsDS(bad); err == nil {
			t.Fatalf("%v should not be representable as DS", bad)
		}
	}
}

func TestDSFromFloatRejectsNonFinite(t *testing.T) {
	if _, err := DSFromFloat(math.Inf(1)); err == nil {
		t.Fatal("expected error for infinity")
	}
	if _, err := DSFromFloat(math.NaN()); err == nil {
		t.Fatal("expected error for NaN")
	}
}
