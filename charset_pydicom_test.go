package godicom

// The character set suite already ports pydicom's tests/test_charset.py at
// commit fb1f24e4f0: encodedNames/TestSingleByteCodeExtensions come from
// ENCODED_NAMES, TestMultiCharset* come from the code-extension tests, and
// TestReadCharsetFilesPatientName comes from FILE_PATIENT_NAMES.
//
// What that port did not carry over is pydicom's term table itself. This file
// adds it, because a table gap is invisible to every test that only exercises
// the encodings someone happened to pick, while it is very visible to the site
// whose legal file declares one of the missing terms.
//
// Where godicom deliberately differs, the difference is pinned with a comment
// saying what pydicom returns and why godicom does not. A pinned difference is
// a decision; an unpinned one is a bug waiting to happen.

import (
	"strings"
	"testing"
)

// pydicomDefinedTerms is pydicom's python_encoding table without its "" default
// entry (pydicom/charset.py, "Map DICOM Specific Character Set to python
// equivalent"). Every one of these is a defined term a real file can legally
// carry, so a term missing from dicomEncodings means a legal file is reported
// as an unknown character set and then decoded as ISO_IR 6.
//
// Three were missing before this table existed: ISO_IR 13, ISO 2022 GBK and
// ISO 2022 58. A Japanese CR declaring ISO_IR 13 therefore raised a false
// unknown_character_set diagnostic and came back as mojibake.
var pydicomDefinedTerms = []string{
	"ISO_IR 6",
	"ISO_IR 13",
	"ISO_IR 100",
	"ISO_IR 101",
	"ISO_IR 109",
	"ISO_IR 110",
	"ISO_IR 126",
	"ISO_IR 127",
	"ISO_IR 138",
	"ISO_IR 144",
	"ISO_IR 148",
	"ISO_IR 166",
	"ISO 2022 IR 6",
	"ISO 2022 IR 13",
	"ISO 2022 IR 87",
	"ISO 2022 IR 100",
	"ISO 2022 IR 101",
	"ISO 2022 IR 109",
	"ISO 2022 IR 110",
	"ISO 2022 IR 126",
	"ISO 2022 IR 127",
	"ISO 2022 IR 138",
	"ISO 2022 IR 144",
	"ISO 2022 IR 148",
	"ISO 2022 IR 149",
	"ISO 2022 IR 159",
	"ISO 2022 IR 166",
	"ISO 2022 IR 58",
	"ISO_IR 192",
	"GB18030",
	"ISO 2022 GBK",
	"ISO 2022 58",
	"GBK",
}

// Port of pydicom's TestCharset.test_encodings, widened: every term in the
// table must resolve to a codec that works in both directions. pydicom only
// checks that each Python codec can encode "Hello World"; the resolution half
// is what actually catches a term that is present but maps to nothing usable,
// and the ASCII round trip catches a term mapped to the wrong codec.
func TestCharacterSetTableMatchesPydicom(t *testing.T) {
	t.Parallel()
	for _, term := range pydicomDefinedTerms {
		t.Run(term, func(t *testing.T) {
			name, ok := resolveCharacterSetNameOK(term)
			if !ok {
				t.Fatalf("defined term %q does not resolve; the reader would report it as unknown and decode as ISO_IR 6", term)
			}
			enc, ok := dicomEncodings[name]
			if !ok || enc == nil {
				t.Fatalf("%q resolved to %q, which has no codec", term, name)
			}
			// Header values, dates and UIDs travel in every dataset, so a codec
			// that cannot round-trip plain ASCII is the wrong codec.
			const ascii = "Hello World"
			encoded, err := enc.NewEncoder().String(ascii)
			if err != nil {
				t.Fatalf("codec for %q cannot encode ASCII: %v", term, err)
			}
			decoded, err := enc.NewDecoder().String(encoded)
			if err != nil {
				t.Fatalf("codec for %q cannot decode its own output: %v", term, err)
			}
			if decoded != ascii {
				t.Fatalf("codec for %q round-tripped %q to %q", term, ascii, decoded)
			}
		})
	}
}

// TestPydicomTermTableIsWellFormed guards the copied table itself: the list is
// hand-copied from pydicom, so it needs one check that the copy is clean. The
// count assertion is deliberate friction -- adding or removing a term in
// pydicom should force a conscious edit here.
func TestPydicomTermTableIsWellFormed(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool, len(pydicomDefinedTerms))
	for _, term := range pydicomDefinedTerms {
		if term == "" {
			t.Fatal("empty term in pydicomDefinedTerms")
		}
		if strings.TrimSpace(term) != term {
			t.Fatalf("term %q has leading or trailing whitespace", term)
		}
		if seen[term] {
			t.Fatalf("duplicate term %q", term)
		}
		seen[term] = true
	}
	if len(pydicomDefinedTerms) != 33 {
		t.Fatalf("pydicomDefinedTerms has %d entries, pydicom fb1f24e has 33; update the table and this count together", len(pydicomDefinedTerms))
	}
}

// Regression tests for the three terms the table used to be missing. A resolved
// name is not the same as a working decode, so each one decodes a real payload
// rather than only checking that the key exists.
func TestRestoredDefinedTermsDecode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		charset string
		raw     string
		want    string
	}{
		{
			name:    "ISO_IR 13 is the standalone Japanese term, not the ISO 2022 form",
			charset: "ISO_IR 13",
			raw:     "\xd4\xcf\xc0\xde",
			want:    "ﾔﾏﾀﾞ",
		},
		{"ISO 2022 GBK", "ISO 2022 GBK", "\xd6\xd0\xce\xc4", "中文"},
		{"ISO 2022 58", "ISO 2022 58", "\xd6\xd0\xce\xc4", "中文"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertPNWithCharsets([]byte(tt.raw), []string{tt.charset})
			if err != nil {
				t.Fatalf("decode with %s: %v", tt.charset, err)
			}
			pn, ok := got.(PersonName)
			if !ok {
				t.Fatalf("value type = %T, want PersonName", got)
			}
			if pn.String() != tt.want {
				t.Fatalf("%s decoded %q, want %q", tt.charset, pn.String(), tt.want)
			}
		})
	}
}

// Port of test_missing_escape_for_single_byte_code_extensions.
//
// Pinned difference: pydicom returns "ASCII+ﾔﾏﾀﾞ^ÀÛ³", because its default
// encoding is Latin-1, so the run after the component delimiter comes back as
// U+00C0 U+00DB U+00B3. godicom maps ISO_IR 6 to encoding.Nop, so the same
// bytes stay undecoded and the Go string holds 0xC0 0xDB 0xB3, which is not
// valid UTF-8.
//
// Changing that means changing what ISO_IR 6 means for every high-byte value
// godicom has ever read, which is an API decision, not a bug fix -- so it is
// left visible here instead of being quietly changed.
func TestMissingEscapeFallsBackToFirstEncoding(t *testing.T) {
	t.Parallel()
	raw := []byte("ASCII+\x1b\x29\x49\xd4\xcf\xc0\xde\x5e\xc0\xdb\xb3")
	got := DecodeBytesWithDelimiters(raw, []string{"", "ISO 2022 IR 13"}, pnDelims)
	want := "ASCII+ﾔﾏﾀﾞ^\xc0\xdb\xb3" // pydicom: "ASCII+ﾔﾏﾀﾞ^ÀÛ³"
	if got != want {
		t.Fatalf("decoded %q, want %q", got, want)
	}
}

// Port of test_convert_encodings_warnings, behaviour half only: a stand-alone
// character set cannot be used as a code extension, so it is dropped from the
// extension position instead of breaking the first encoding. pydicom also warns
// here; godicom filters silently, which is the existing behaviour for this path
// and is not what the unknown-character-set diagnostic is for.
func TestStandaloneCharsetInExtensionPositionIsDropped(t *testing.T) {
	t.Parallel()
	got := ConvertCharacterSets([]string{"ISO_IR 126", "GBK", "ISO 2022 IR 144", "ISO_IR 192"})
	want := []string{"ISO_IR 126", "ISO 2022 IR 144"}
	if len(got) != len(want) {
		t.Fatalf("ConvertCharacterSets = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ConvertCharacterSets = %v, want %v", got, want)
		}
	}
	if got := ConvertCharacterSets([]string{"GB18030", "ISO 2022 IR 100"}); len(got) != 1 || got[0] != "GB18030" {
		t.Fatalf("GB18030 first: ConvertCharacterSets = %v, want [GB18030]", got)
	}
}

// Port of test_bad_charset and test_empty_charset: an absent, empty or
// empty-valued (0008,0005) all mean ISO_IR 6, and none of them is an unknown
// character set.
func TestEmptyOrAbsentCharsetIsISOIR6(t *testing.T) {
	t.Parallel()
	for _, value := range []interface{}{nil, "", NewMultiValue([]string{""})} {
		got := ParseCharacterSets(value)
		if len(got) != 1 || got[0] != "ISO_IR 6" {
			t.Errorf("ParseCharacterSets(%#v) = %v, want [ISO_IR 6]", value, got)
		}
	}
}
