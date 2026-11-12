package godicom

// Ports of pydicom TestDataElementValidation (tests/test_dataelem.py). The
// Python class also checks what happens when a value of the wrong Python type
// reaches a DataElement; Go's type system makes that case inexpressible, so
// those parameters have no counterpart and are noted where they would have been.
//
// pydicom's own test method names for UI and UR are swapped (test_invalid_ui
// exercises UR and vice versa); the subtest names below follow what each case
// actually validates, not the upstream typo.

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateValueMaxLengthExceeded(t *testing.T) {
	// pydicom: test_maxvalue_exceeded
	cases := []struct {
		vr     VR
		length int
	}{
		{VRAE, 17},
		{VRCS, 17},
		{VRDS, 27},
		{VRLO, 66},
		{VRLT, 10250},
		{VRSH, 17},
		{VRST, 1025},
		{VRUI, 65},
	}

	for _, c := range cases {
		t.Run(string(c.vr)+"_"+itoaDecimal(c.length), func(t *testing.T) {
			err := ValidateValue(c.vr, strings.Repeat("1", c.length))
			if err == nil {
				t.Fatalf("ValidateValue(%s, %d chars) returned no error", c.vr, c.length)
			}
			var verr *ValueValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("error %v is not a *ValueValidationError", err)
			}
			if verr.VR != c.vr {
				t.Errorf("VR = %s, want %s", verr.VR, c.vr)
			}
		})
	}
}

func TestValidateValueValidLengthOnTheBoundary(t *testing.T) {
	// The limits are inclusive: a 16-character AE is legal, 17 is not.
	cases := []struct {
		vr     VR
		length int
	}{
		{VRAE, 16},
		{VRCS, 16},
		{VRDS, 16},
		{VRLO, 64},
		{VRSH, 16},
		{VRUI, 64},
	}

	for _, c := range cases {
		t.Run(string(c.vr)+"_"+itoaDecimal(c.length), func(t *testing.T) {
			value := strings.Repeat("1", c.length)
			if c.vr == VRAE || c.vr == VRCS || c.vr == VRLO || c.vr == VRSH {
				value = strings.Repeat("A", c.length)
			}
			if err := ValidateValue(c.vr, value); err != nil {
				t.Fatalf("ValidateValue(%s, %d chars) = %v, want nil", c.vr, c.length, err)
			}
		})
	}
}

// validateTable runs the same accept/reject matrix the pydicom class builds out
// of pytest.mark.parametrize, keeping one subtest per value so a failure names
// the value rather than just the VR.
func validateTable(t *testing.T, vr VR, valid, invalid []string) {
	t.Helper()

	for _, value := range valid {
		value := value
		t.Run("valid_"+labelFor(value), func(t *testing.T) {
			if err := ValidateValue(vr, value); err != nil {
				t.Fatalf("ValidateValue(%s, %q) = %v, want nil", vr, value, err)
			}
		})
	}
	for _, value := range invalid {
		value := value
		t.Run("invalid_"+labelFor(value), func(t *testing.T) {
			err := ValidateValue(vr, value)
			if err == nil {
				t.Fatalf("ValidateValue(%s, %q) returned no error", vr, value)
			}
			var verr *ValueValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("error %v is not a *ValueValidationError", err)
			}
			if verr.Value != value {
				t.Errorf("ValueValidationError.Value = %q, want %q", verr.Value, value)
			}
		})
	}
}

func TestValidateValueAE(t *testing.T) {
	// pydicom: test_invalid_ae / test_valid_ae
	validateTable(t, VRAE,
		[]string{"My AETitle", ""},
		[]string{"Руссский", "ctrl\tchar", "new\n", "Äneas"},
	)
}

func TestValidateValueAS(t *testing.T) {
	// pydicom: test_invalid_as / test_valid_as
	validateTable(t, VRAS,
		[]string{"012Y", "345M", "052W", "789D", ""},
		[]string{"12Y", "0012Y", "012B", "Y012", "012Y\n"},
	)
}

func TestValidateValueCS(t *testing.T) {
	// pydicom: test_invalid_cs / test_valid_cs
	validateTable(t, VRCS,
		[]string{"VALID_13579 ", "VALID_13579", ""},
		[]string{"abcd", "ABC+D", "ABCD-Z", "ÄÖÜ", "ABC\n"},
	)
}

func TestValidateValueDA(t *testing.T) {
	// pydicom: test_invalid_da / test_valid_da
	validateTable(t, VRDA,
		[]string{"19560303", "20101225-20201224 ", "-19560303", "19560303-", ""},
		[]string{"201012", "2010122505", "20102525", "-20101225-", "20101620",
			"20101040", "20101033", "20101225 20201224 "},
	)
}

func TestValidateValueDS(t *testing.T) {
	// pydicom: test_invalid_ds / test_valid_ds
	validateTable(t, VRDS,
		[]string{"12345", "+.1234 ", "-0345.76", "1956E3", "-1956e+3", "+195.6e-3", ""},
		[]string{"201012+", "20A0", "+-123.66", "-123.5 E4", "123F4 ", "- 195.6"},
	)
}

func TestValidateValueIS(t *testing.T) {
	// pydicom: test_invalid_is / test_valid_is
	validateTable(t, VRIS,
		[]string{" 12345 ", "+1234 ", "-034576", ""},
		[]string{"201012+", "20A0", "123.66", "-1235E4", "12 34"},
	)
}

func TestValidateValueTM(t *testing.T) {
	// pydicom: test_invalid_tm / test_valid_tm
	validateTable(t, VRTM,
		[]string{"23", "1234", "010159", "225959.3", "000000.345", "222222.222222",
			"-1234", "123456-", "123460-1330", "005960", ""},
		[]string{"234", "1", "01015", "225959.", "0000.345", "222222.2222222",
			"-1234-", "+123456", "-123456-1330", "006000", "005961", "0000aa",
			"0000.00", "123461-1330", "123400-1360"},
	)
}

func TestValidateValueDT(t *testing.T) {
	// pydicom: test_invalid_dt / test_valid_dt
	validateTable(t, VRDT,
		[]string{"1984", "200112", "20200101", "1877123112", "200006012020",
			"19190420015960", "20300202022222.222222", "1984+0600", "1877123112-0030",
			"20300202022222.2-1200", "20000101-", "-2020010100", "1929-1997", ""},
		[]string{"19", "198", "20011", "20200101.222", "187712311", "20001301",
			"19190432010159", "203002020222.2222222", "203002020270.2", "1984+2000",
			"+1877123112-0030", "19190430010161", "19190430016000"},
	)
}

func TestValidateValueUI(t *testing.T) {
	// pydicom: test_valid_ui / test_invalid_ur (the upstream names are swapped)
	validateTable(t, VRUI,
		[]string{"1234.567890.333", "0.0.0", strings.Repeat("1234.", 12) + "1234", ""},
		[]string{".123.456", "00.1.2", "123..456", "123.45.", "12a.45", "123.04"},
	)
}

func TestValidateValueUR(t *testing.T) {
	// pydicom: test_valid_ur / test_invalid_ui (the upstream names are swapped)
	validateTable(t, VRUR,
		[]string{"https://www.a.b/sdf_g?a=1&b=5", "/a#b(c)[d]@!", "'url'", ""},
		[]string{"Руссский", "ctrl\tchar", "\"url\"", "a<b", "{abc}"},
	)
}

func TestValidateValuePN(t *testing.T) {
	// pydicom: test_valid_pn / test_invalid_pn
	validateTable(t, VRPN,
		[]string{"John^Doe", "Yamada^Tarou=山田^太郎", ""},
		[]string{
			"Jim=John=Jimmy=Jonny",      // four component groups, the standard allows three
			strings.Repeat("Jimmy", 13), // 65 characters in one group
		},
	)
}

func TestValidateValueNonASCIILengthCountsCharacters(t *testing.T) {
	// pydicom: test_write_valid_length_non_ascii_text /
	// test_write_valid_non_ascii_pn -- the limit is on characters, not bytes.
	pn := strings.Repeat("洪^吉洞", 10) // 40 characters, 80 UTF-8 bytes
	if err := ValidateValue(VRPN, pn); err != nil {
		t.Fatalf("ValidateValue(PN, %d bytes / %d characters) = %v, want nil",
			len(pn), len([]rune(pn)), err)
	}

	sh := strings.Repeat("洪", 15) // 15 characters, 45 bytes
	if err := ValidateValue(VRSH, sh); err != nil {
		t.Fatalf("ValidateValue(SH, 15 Chinese characters) = %v, want nil", err)
	}

	shOver := strings.Repeat("洪", 17)
	if err := ValidateValue(VRSH, shOver); err == nil {
		t.Fatal("ValidateValue(SH, 17 Chinese characters) returned no error")
	}
}

func TestValidateValueIgnoresBinaryAndNumericVRs(t *testing.T) {
	// Binary and numeric VRs have no string form to validate; asking about one
	// is a category error, not a finding. The encoder is what guarantees those.
	for _, vr := range []VR{VROB, VROW, VRUN, VRUS, VRSS, VRUL, VRSL, VRFD, VRFL, VRSQ, VRAT} {
		if err := ValidateValue(vr, "anything at all"); err != nil {
			t.Errorf("ValidateValue(%s, ...) = %v, want nil", vr, err)
		}
	}
}

func TestValidateValueEmptyIsAlwaysValid(t *testing.T) {
	// PS3.5 permits a zero-length value in every string VR, and "no value" is
	// the commonest correct answer in a real dataset.
	for _, vr := range []VR{VRAE, VRAS, VRCS, VRDA, VRDS, VRDT, VRIS, VRLO, VRLT,
		VRPN, VRSH, VRST, VRTM, VRUI, VRUR} {
		if err := ValidateValue(vr, ""); err != nil {
			t.Errorf("ValidateValue(%s, \"\") = %v, want nil", vr, err)
		}
	}
}

// labelFor turns a value into a subtest name: Go replaces spaces with
// underscores anyway, and control characters would make the name unreadable.
func labelFor(value string) string {
	switch value {
	case "":
		return "empty"
	}
	out := make([]rune, 0, len(value))
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			out = append(out, '~')
			continue
		}
		out = append(out, r)
	}
	return strings.ReplaceAll(string(out), " ", "_")
}

// itoaDecimal renders a small non-negative int without pulling strconv into the
// assertions that only need it for a subtest name.
func itoaDecimal(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
