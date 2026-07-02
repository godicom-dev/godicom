package godicom

import (
	"testing"
	"time"
)

func TestParseDA(t *testing.T) {
	da, err := ParseDA("19111213")
	if err != nil {
		t.Fatal(err)
	}
	if da.Time.Year() != 1911 || int(da.Time.Month()) != 12 || da.Time.Day() != 13 {
		t.Fatalf("date = %v", da.Time)
	}
	if da.String() != "19111213" {
		t.Fatalf("String() = %q", da.String())
	}

	da, err = ParseDA("1001.02.03")
	if err != nil {
		t.Fatal(err)
	}
	if da.String() != "1001.02.03" {
		t.Fatalf("String() = %q", da.String())
	}

	if _, err := ParseDA("123456"); err == nil {
		t.Fatal("expected error for invalid DA")
	}
}

func TestParseTM(t *testing.T) {
	tm, err := ParseTM("115500")
	if err != nil {
		t.Fatal(err)
	}
	if tm.Time.Hour() != 11 || tm.Time.Minute() != 55 || tm.Time.Second() != 0 {
		t.Fatalf("time = %v", tm.Time)
	}
	if tm.String() != "115500" {
		t.Fatalf("String() = %q", tm.String())
	}

	tm, err = ParseTM("115500.123456")
	if err != nil {
		t.Fatal(err)
	}
	if tm.Time.Nanosecond() != 123456000 {
		t.Fatalf("nsec = %d", tm.Time.Nanosecond())
	}

	tm, err = ParseTM("235960")
	if err != nil {
		t.Fatal(err)
	}
	if tm.Time.Second() != 59 {
		t.Fatalf("second = %d, want 59 for leap second", tm.Time.Second())
	}
}

func TestParseDT(t *testing.T) {
	dt, err := ParseDT("20200101115500.000000")
	if err != nil {
		t.Fatal(err)
	}
	if dt.Time.Year() != 2020 || int(dt.Time.Month()) != 1 || dt.Time.Day() != 1 {
		t.Fatalf("date = %v", dt.Time)
	}
	if dt.Time.Hour() != 11 || dt.Time.Minute() != 55 {
		t.Fatalf("time = %v", dt.Time)
	}
	if dt.String() != "20200101115500.000000" {
		t.Fatalf("String() = %q", dt.String())
	}

	dt, err = ParseDT("2020")
	if err != nil {
		t.Fatal(err)
	}
	if dt.Time.Year() != 2020 || int(dt.Time.Month()) != 1 || dt.Time.Day() != 1 {
		t.Fatalf("partial date = %v", dt.Time)
	}
}

func TestConvertDAFromBytes(t *testing.T) {
	v, err := convertDAString([]byte("20000101"))
	if err != nil {
		t.Fatal(err)
	}
	da, ok := v.(DA)
	if !ok {
		t.Fatalf("type = %T", v)
	}
	if da.String() != "20000101" {
		t.Fatalf("got %q", da.String())
	}
}

func TestGetDAFromDataset(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientBirthDate"), VRDA, "20000101"))

	da, ok := ds.GetDA(MustTag("PatientBirthDate"))
	if !ok {
		t.Fatal("GetDA failed")
	}
	if da.String() != "20000101" {
		t.Fatalf("got %q", da.String())
	}
	s, ok := ds.GetString(MustTag("PatientBirthDate"))
	if !ok || s != "20000101" {
		t.Fatalf("GetString = %q, %v", s, ok)
	}
}

func TestDARoundtripWrite(t *testing.T) {
	ds := NewDataset()
	da, _ := ParseDA("20000101")
	ds.Set(NewDataElement(MustTag("PatientBirthDate"), VRDA, da))

	out := encodeElementExplicitLittle(NewDataElement(MustTag("PatientBirthDate"), VRDA, da))
	if !bytesHasSuffix(out, []byte("20000101")) {
		t.Fatalf("encoded = % X", out)
	}
}

func bytesHasSuffix(b, suffix []byte) bool {
	if len(b) < len(suffix) {
		return false
	}
	for i := range suffix {
		if b[len(b)-len(suffix)+i] != suffix[i] {
			return false
		}
	}
	return true
}

func TestDAFromTime(t *testing.T) {
	da := DA{Time: time.Date(1001, 2, 3, 0, 0, 0, 0, time.UTC)}
	if da.String() != "10010203" {
		t.Fatalf("String() = %q", da.String())
	}
}
