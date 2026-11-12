package godicom

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// Ported from pydicom tests/test_valuerep.py. The pickling cases have no Go
// counterpart (Go values are copied, not pickled) and Decimal has no equivalent
// type here -- DS is float64 -- so TestDSdecimal is absent by design.

func TestIsValidDSTable(t *testing.T) {
	// pydicom: TestIsValidDS::test_valid, test_invalid
	tests := []struct {
		value string
		valid bool
	}{
		{"1", true},
		{"1.0", true},
		{"-1.0", true},
		{"+1.0", true},
		{".5", true},
		{"5.", true},
		{"1e5", true},
		{"1.5E-3", true},
		{" 1.5 ", true},
		{"", false},
		{"1.0.0", false},
		{"1,5", false},
		{"abc", false},
		{"1.5x", false},
		{"1" + strings.Repeat("0", 16), false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := IsValidDS(tt.value); got != tt.valid {
				t.Fatalf("IsValidDS(%q) = %v, want %v", tt.value, got, tt.valid)
			}
		})
	}
}

func TestIsValidIS(t *testing.T) {
	// pydicom: TestIS::test_valid_value, test_invalid_value -- length and
	// character set only; the numeric range is ISInRange's job.
	tests := []struct {
		value string
		valid bool
	}{
		{"123", true},
		{"+123", true},
		{"-123", true},
		{" 123 ", true},
		{"", false},
		{"1.5", false},
		{"abc", false},
		{"1234" + strings.Repeat("0", 9), false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := IsValidIS(tt.value); got != tt.valid {
				t.Fatalf("IsValidIS(%q) = %v, want %v", tt.value, got, tt.valid)
			}
		})
	}
}

func TestISInRangeBoundaries(t *testing.T) {
	// pydicom: TestIS::test_overflow / test_underflow
	tests := []struct {
		name string
		v    int64
		want bool
	}{
		{"zero", 0, true},
		{"max", 1<<31 - 1, true},
		{"min", -1 << 31, true},
		{"above_max", 1 << 31, false},
		{"below_min", -1<<31 - 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ISInRange(tt.v); got != tt.want {
				t.Fatalf("ISInRange(%d) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestParseDSTable(t *testing.T) {
	// pydicom: TestDS::test_float_values, TestBadValueRead
	tests := []struct {
		name     string
		in       string
		wantVal  float64
		wantOrig string
		wantErr  bool
	}{
		{"integer", "42", 42, "42", false},
		{"decimal", "1.5", 1.5, "1.5", false},
		{"negative", "-2.25", -2.25, "-2.25", false},
		{"padded_is_preserved", "1.50 ", 1.5, "1.50", false},
		{"empty_is_zero", "", 0, "", false},
		{"padding_only_is_zero", "   ", 0, "", false},
		{"not_a_number", "abc", 0, "", true},
		{"comma_decimal", "1,5", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDS(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDS(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Value != tt.wantVal {
				t.Fatalf("Value = %v, want %v", got.Value, tt.wantVal)
			}
			if got.Original != tt.wantOrig {
				t.Fatalf("Original = %q, want %q", got.Original, tt.wantOrig)
			}
		})
	}
}

func TestDSStringKeepsTheOriginalSpelling(t *testing.T) {
	// pydicom: TestDSfloat::test_str, test_str_value -- the string form is what
	// was read, not a re-formatted float.
	tests := []struct {
		in   string
		want string
	}{
		{"1.50", "1.50"},
		{"1.5", "1.5"},
		{"2.0  ", "2.0"},
		{"  2.0", "  2.0"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseDS(tt.in)
			if err != nil {
				t.Fatalf("ParseDS(%q) error = %v", tt.in, err)
			}
			if got.String() != tt.want {
				t.Fatalf("String() = %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestDSIsZero(t *testing.T) {
	// pydicom: TestDSfloat::test_new_empty
	var zero DS
	if !zero.IsZero() {
		t.Fatal("zero DS should report IsZero")
	}
	parsed, _ := ParseDS("0")
	if parsed.IsZero() {
		t.Fatal("DS parsed from \"0\" should not report IsZero: it has an Original")
	}
}

func TestFormatNumberAsDSTable(t *testing.T) {
	// pydicom: TestTruncateFloatForDS::test_auto_format, test_invalid,
	// test_wrong_type
	tests := []struct {
		name    string
		in      float64
		want    string
		wantErr bool
	}{
		{"short_value_is_unchanged", 1.5, "1.5", false},
		{"integer_valued_float", 3, "3.0", false},
		{"negative", -2.25, "-2.25", false},
		{"zero", 0, "0.0", false},
		{"nan_is_refused", math.NaN(), "", true},
		{"positive_infinity_is_refused", math.Inf(1), "", true},
		{"negative_infinity_is_refused", math.Inf(-1), "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNumberAsDS(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FormatNumberAsDS(%v) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Fatalf("FormatNumberAsDS(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatNumberAsDSNeverExceedsSixteenChars(t *testing.T) {
	// pydicom: TestTruncateFloatForDS::test_powers_of_pi -- the serialised form of
	// any float has to fit the VR's 16-byte field.
	values := []float64{
		3.141592653589793,
		3141592653589793,
		0.000000123456789,
		-3.141592653589793,
		1e-20,
		1e20,
	}
	for _, v := range values {
		t.Run(strconv.FormatFloat(v, 'g', -1, 64), func(t *testing.T) {
			got, err := FormatNumberAsDS(v)
			if err != nil {
				t.Fatalf("FormatNumberAsDS(%v) error = %v", v, err)
			}
			if len(got) > 16 {
				t.Fatalf("FormatNumberAsDS(%v) = %q, %d chars > 16", v, got, len(got))
			}
			if !IsValidDS(got) {
				t.Fatalf("FormatNumberAsDS(%v) = %q, which IsValidDS rejects", v, got)
			}
		})
	}
}

func TestParseISTable(t *testing.T) {
	// pydicom: TestIS::test_valid_value, test_invalid_value
	tests := []struct {
		in      string
		wantVal int64
		wantErr bool
	}{
		{"123", 123, false},
		{"+123", 123, false},
		{"-456", -456, false},
		{"0", 0, false},
		{" 42 ", 42, false},
		{"1.5", 0, true},
		{"abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseIS(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseIS(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Value != tt.wantVal {
				t.Fatalf("Value = %d, want %d", got.Value, tt.wantVal)
			}
		})
	}
}

func TestISStringKeepsTheOriginalSpelling(t *testing.T) {
	// pydicom: TestIS::test_str
	got, err := ParseIS("+0123")
	if err != nil {
		t.Fatalf("ParseIS(+0123) error = %v", err)
	}
	if got.String() != "+0123" {
		t.Fatalf("String() = %q, want +0123", got.String())
	}
	if got.Value != 123 {
		t.Fatalf("Value = %d, want 123", got.Value)
	}
}

func TestParseDATable(t *testing.T) {
	// pydicom: TestDA::test_new_str_conversion, test_date_behavior
	tests := []struct {
		name      string
		in        string
		wantYear  int
		wantMonth int
		wantDay   int
		wantErr   bool
	}{
		{"compact", "20240131", 2024, 1, 31, false},
		{"dotted", "2024.01.31", 2024, 1, 31, false},
		{"empty_is_zero", "", 0, 0, 0, false},
		{"dashed_is_invalid", "2024-01-31", 0, 0, 0, true},
		{"short_is_invalid", "202401", 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDA(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDA(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.IsZero() {
				if tt.wantYear != 0 {
					t.Fatal("parsed date reports IsZero")
				}
				return
			}
			if got.Time.Year() != tt.wantYear || int(got.Time.Month()) != tt.wantMonth || got.Time.Day() != tt.wantDay {
				t.Fatalf("ParseDA(%q) = %v", tt.in, got.Time)
			}
			if got.String() != strings.TrimRight(tt.in, " \x00") {
				t.Fatalf("String() = %q, want the original spelling", got.String())
			}
		})
	}
}

func TestParseTMTable(t *testing.T) {
	// pydicom: TestTM::test_new_str_conversion, test_time_behavior
	tests := []struct {
		name       string
		in         string
		wantHour   int
		wantMinute int
		wantSecond int
		wantErr    bool
	}{
		{"hour_only", "12", 12, 0, 0, false},
		{"hour_minute", "1230", 12, 30, 0, false},
		{"full", "123045", 12, 30, 45, false},
		{"with_fraction", "123045.123456", 12, 30, 45, false},
		{"leap_second_is_clamped", "123060", 12, 30, 59, false},
		{"empty_is_zero", "", 0, 0, 0, false},
		{"out_of_range", "240000", 0, 0, 0, true},
		{"bad_minute", "1275", 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTM(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseTM(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.IsZero() {
				return
			}
			if got.Time.Hour() != tt.wantHour || got.Time.Minute() != tt.wantMinute || got.Time.Second() != tt.wantSecond {
				t.Fatalf("ParseTM(%q) = %v", tt.in, got.Time)
			}
		})
	}
}

func TestParseDTTable(t *testing.T) {
	// pydicom: TestDT::test_new_str_conversion, test_datetime_behavior
	tests := []struct {
		name     string
		in       string
		wantYear int
		wantErr  bool
	}{
		{"year_only", "2024", 2024, false},
		{"date_only", "20240131", 2024, false},
		{"full", "20240131123045", 2024, false},
		{"with_fraction", "20240131123045.123456", 2024, false},
		{"with_timezone", "20240131123045+0800", 2024, false},
		{"empty_is_zero", "", 0, false},
		{"too_long", strings.Repeat("1", 27), 0, true},
		{"garbage", "abcdefgh", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDT(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDT(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.IsZero() {
				if tt.wantYear != 0 {
					t.Fatal("parsed datetime reports IsZero")
				}
				return
			}
			if got.Time.Year() != tt.wantYear {
				t.Fatalf("ParseDT(%q) year = %d, want %d", tt.in, got.Time.Year(), tt.wantYear)
			}
		})
	}
}

func TestDSAndISEquality(t *testing.T) {
	// pydicom: TestDSfloat::test_comparison_operators, TestIS::test_comparison --
	// Equal is about the numeric value; the original spelling is only what String
	// returns.
	a, _ := ParseDS("1.50")
	b, _ := ParseDS("1.5")
	if !a.Equal(b) {
		t.Fatal("DS Equal() = false for 1.50 and 1.5, want true (same value)")
	}
	c, _ := ParseDS("1.5")
	d, _ := ParseDS("2.5")
	if c.Equal(d) {
		t.Fatal("DS Equal() = true for 1.5 and 2.5")
	}

	var zeroDS1, zeroDS2 DS
	if !zeroDS1.Equal(zeroDS2) {
		t.Fatal("DS Equal() = false for two zero values")
	}

	e, _ := ParseIS("123")
	f, _ := ParseIS("+123")
	if !e.Equal(f) {
		t.Fatal("IS Equal() = false for 123 and +123, want true (same value)")
	}
	g, _ := ParseIS("124")
	if e.Equal(g) {
		t.Fatal("IS Equal() = true for 123 and 124")
	}
}
