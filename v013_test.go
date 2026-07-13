package godicom

import (
	"bytes"
	"path/filepath"
	"testing"
)

// pydicom.tests.test_filewriter.TestWriter.test_changed_character_set
func TestChangedCharacterSet(t *testing.T) {
	ds, err := ReadFile(requireCharsetFile(t, "chrFrenMulti.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(MustTag(0x00100010))
	if !ok {
		t.Fatal("PatientName missing")
	}
	if !bytes.Equal(elem.RawValue, []byte("Buc^J\xe9r\xf4me")) {
		t.Fatalf("original RawValue = %q", elem.RawValue)
	}

	ds.Set(NewDataElement(MustTag("SpecificCharacterSet"), VRCS, "ISO_IR 192"))

	outPath := filepath.Join(t.TempDir(), "changed_cs.dcm")
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	out, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	outElem, ok := out.Get(MustTag(0x00100010))
	if !ok {
		t.Fatal("output PatientName missing")
	}
	want := []byte("Buc^J\xc3\xa9r\xc3\xb4me")
	got := bytes.TrimRight(outElem.RawValue, " \x00")
	if !bytes.Equal(got, want) {
		t.Fatalf("PatientName RawValue = %q, want %q", got, want)
	}
	if outElem.Value.(PersonName).String() != "Buc^Jérôme" {
		t.Fatalf("PatientName = %q", outElem.Value.(PersonName).String())
	}
}

// pydicom.tests.test_filewriter.TestWritePN.test_no_encoding
func TestWritePNNoEncoding(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test")
	got := padToEven(VRPN, encodePNWithCharsets(elem, []string{DefaultCharacterSet}))
	if !bytes.Equal(got, []byte("Test")) {
		t.Fatalf("got = %q", got)
	}
}

// pydicom.tests.test_filewriter.TestWritePN.test_single_byte_multi_charset_groups
func TestWritePNMultiCharsetGroups(t *testing.T) {
	encodings := []string{"ISO 2022 IR 100", "ISO 2022 IR 126"}
	want := []byte("Dionysios=\x1b\x2d\x46\xc4\xe9\xef\xed\xf5\xf3\xe9\xef\xf2")

	elem := NewDataElement(MustTag(0x00100010), VRPN, want)
	got := encodePNWithCharsets(elem, encodings)
	if !bytes.Equal(got, want) {
		t.Fatalf("raw pass-through = %q, want %q", got, want)
	}

	elem = NewDataElement(MustTag(0x00100010), VRPN, PersonName{
		Alphabetic:  "Dionysios",
		Ideographic: "Διονυσιος",
	})
	got = encodePNWithCharsets(elem, encodings)
	if !bytes.Equal(got, want) {
		t.Fatalf("decoded encode = %q, want %q", got, want)
	}
}

// pydicom.tests.test_filewriter.TestWritePN.test_single_byte_multi_charset_values
func TestWritePNMultiCharsetValues(t *testing.T) {
	encodings := []string{"ISO 2022 IR 100", "ISO 2022 IR 144", "ISO 2022 IR 126"}
	want := []byte(
		"Buc^J\xe9r\xf4me\\\x1b\x2d\x46" +
			"\xc4\xe9\xef\xed\xf5\xf3\xe9\xef\xf2\\" +
			"\x1b\x2d\x4c" +
			"\xbb\xee\xda\x63\x65\xdc\xd1\x79\x70\xd3",
	)

	mv := NewMultiValue([]PersonName{
		{Alphabetic: "Buc^Jérôme"},
		{Alphabetic: "Διονυσιος"},
		{Alphabetic: "Люкceмбypг"},
	})
	elem := NewDataElement(MustTag(0x00100060), VRPN, mv)
	got := encodePNWithCharsets(elem, encodings)
	if !bytes.Equal(got, want) {
		t.Fatalf("got = %q, want %q", got, want)
	}
}

// pydicom.tests.test_filewriter.TestWriteText.test_single_byte_multi_charset_text
func TestWriteTextMultiCharset(t *testing.T) {
	encodings := []string{"ISO 2022 IR 100", "ISO 2022 IR 126"}
	elem := NewDataElement(MustTag(0x00081039), VRLO, "Dionysios is Διονυσιος")
	encoded := encodeStringWithCharsets(elem, encodings)
	got := DecodeBytesWithDelimiters(encoded, encodings, textVRDelims)
	if got != "Dionysios is Διονυσιος" {
		t.Fatalf("roundtrip = %q", got)
	}
}

// pydicom.tests.test_filewriter.TestWriteText.test_encode_mixed_charsets_text
func TestWriteTextMixedCharsets(t *testing.T) {
	encodings := []string{"ISO 2022 IR 100", "ISO 2022 IR 149", "ISO 2022 IR 87", "ISO 2022 IR 127"}
	decoded := "山田-قباني-吉洞-لنزار"
	elem := NewDataElement(MustTag(0x00081039), VRLO, decoded)
	encoded := encodeStringWithCharsets(elem, encodings)
	got := DecodeBytesWithDelimiters(encoded, encodings, textVRDelims)
	if got != decoded {
		t.Fatalf("roundtrip = %q, want %q", got, decoded)
	}
}

// pydicom.tests.test_filewriter.TestWriteText.test_single_byte_multi_charset_text_multivalue
func TestWriteTextMultiCharsetMultiValue(t *testing.T) {
	encodings := []string{"ISO 2022 IR 100", "ISO 2022 IR 144", "ISO 2022 IR 126"}
	decoded := []string{"Buc^Jérôme", "Διονυσιος", "Люкceмбypг"}
	elem := NewDataElement(MustTag(0x00081039), VRLO, NewMultiValue(decoded))
	encoded := encodeStringWithCharsets(elem, encodings)
	parts := bytes.Split(encoded, []byte{'\\'})
	if len(parts) != 3 {
		t.Fatalf("encoded parts = %d (%q)", len(parts), encoded)
	}
	for i, part := range parts {
		if DecodeBytesWithDelimiters(part, encodings, textVRDelims) != decoded[i] {
			t.Fatalf("[%d] = %q from %q", i, DecodeBytesWithDelimiters(part, encodings, textVRDelims), part)
		}
	}
}

func TestEncodeJapaneseISO2022(t *testing.T) {
	encodings := []string{"", "ISO 2022 IR 87"}
	pn := PersonName{
		Alphabetic:  "Yamada^Tarou",
		Ideographic: "山田^太郎",
		Phonetic:    "やまだ^たろう",
	}
	want := []byte("Yamada^Tarou=\x1b$B;3ED\x1b(B^\x1b$BB@O:\x1b(B=\x1b$B$d$^$@\x1b(B^\x1b$B$?$m$&\x1b(B")
	got := EncodePersonNameWithCharsets(pn, encodings)
	if !bytes.Equal(got, want) {
		t.Fatalf("got = %q, want %q", got, want)
	}

	kana := PersonName{Alphabetic: "やまだ^たろう"}
	wantKana := []byte("\x1b$B$d$^$@\x1b(B^\x1b$B$?$m$&\x1b(B")
	gotKana := EncodePersonNameWithCharsets(kana, encodings)
	if !bytes.Equal(gotKana, wantKana) {
		t.Fatalf("kana = %q, want %q", gotKana, wantKana)
	}
}

func TestEncodeStringRejectsNonASCIIDefault(t *testing.T) {
	_, err := EncodeString("Δ", DefaultCharacterSet)
	if err == nil {
		t.Fatal("expected error encoding non-ASCII with default charset")
	}
	b, err := EncodeString("Διονυσιος", "ISO_IR 126")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte{0xC4, 0xE9, 0xEF, 0xED, 0xF5, 0xF3, 0xE9, 0xEF, 0xF2}) {
		t.Fatalf("got = %q", b)
	}
}

func TestUnchangedCharsetKeepsRawBytes(t *testing.T) {
	ds, err := ReadFile(requireCharsetFile(t, "chrFren.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	elem, _ := ds.Get(MustTag(0x00100010))
	wantRaw := append([]byte(nil), elem.RawValue...)

	outPath := filepath.Join(t.TempDir(), "unchanged.dcm")
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}
	out, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	outElem, _ := out.Get(MustTag(0x00100010))
	if !bytes.Equal(outElem.RawValue, wantRaw) {
		t.Fatalf("RawValue changed without charset change: %q → %q", wantRaw, outElem.RawValue)
	}
}
