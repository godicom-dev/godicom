package godicom

import (
	"bytes"
	"testing"
)

func TestDecodeString(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		encoding string
		expected string
	}{
		{
			name:     "default ascii",
			input:    []byte("Doe^John"),
			encoding: "ISO_IR 6",
			expected: "Doe^John",
		},
		{
			name:     "latin one",
			input:    []byte{0xC4, 0xD6, 0xDC},
			encoding: "ISO_IR 100",
			expected: "ÄÖÜ",
		},
		{
			name:     "greek",
			input:    []byte{0xC4, 0xE9, 0xEF, 0xED, 0xF5, 0xF3, 0xE9, 0xEF, 0xF2},
			encoding: "ISO_IR 126",
			expected: "Διονυσιος",
		},
		{
			name:     "unknown falls back to raw bytes",
			input:    []byte("ABC"),
			encoding: "UNSUPPORTED",
			expected: "ABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeString(tt.input, tt.encoding)
			if err != nil {
				t.Fatalf("DecodeString error = %v", err)
			}
			if got != tt.expected {
				t.Fatalf("DecodeString = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestEncodeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		encoding string
		expected []byte
	}{
		{
			name:     "default ascii",
			input:    "Doe^John",
			encoding: "ISO_IR 6",
			expected: []byte("Doe^John"),
		},
		{
			name:     "latin one",
			input:    "ÄÖÜ",
			encoding: "ISO_IR 100",
			expected: []byte{0xC4, 0xD6, 0xDC},
		},
		{
			name:     "greek",
			input:    "Διονυσιος",
			encoding: "ISO_IR 126",
			expected: []byte{0xC4, 0xE9, 0xEF, 0xED, 0xF5, 0xF3, 0xE9, 0xEF, 0xF2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeString(tt.input, tt.encoding)
			if err != nil {
				t.Fatalf("EncodeString error = %v", err)
			}
			if string(got) != string(tt.expected) {
				t.Fatalf("EncodeString = % X, want % X", got, tt.expected)
			}
		})
	}
}

func TestDecodeBytes(t *testing.T) {
	got := DecodeBytes([]byte{0xC4, 0xD6, 0xDC}, []string{"ISO_IR 100"})
	if got != "ÄÖÜ" {
		t.Fatalf("DecodeBytes = %q, want ÄÖÜ", got)
	}
}

func TestDecodeBytesFallsBackToRawString(t *testing.T) {
	got := DecodeBytes([]byte("ABC"), []string{})
	if got != "ABC" {
		t.Fatalf("DecodeBytes = %q, want ABC", got)
	}
}

func TestConvertCharacterSets(t *testing.T) {
	got := ConvertCharacterSets([]string{"", "ISO 2022 IR 144"})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0] != DefaultCharacterSet {
		t.Fatalf("first = %q, want default", got[0])
	}
	if got[1] != "ISO 2022 IR 144" {
		t.Fatalf("second = %q", got[1])
	}

	standalone := ConvertCharacterSets([]string{"ISO_IR 192", "ISO 2022 IR 100"})
	if len(standalone) != 1 || standalone[0] != "ISO_IR 192" {
		t.Fatalf("standalone = %v", standalone)
	}
}

func TestParseCharacterSets(t *testing.T) {
	got := ParseCharacterSets("ISO 2022 IR 100\\ISO 2022 IR 126")
	want := []string{"ISO 2022 IR 100", "ISO 2022 IR 126"}
	if len(got) != len(want) {
		t.Fatalf("ParseCharacterSets = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseCharacterSets[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

var encodedNames = []struct {
	encoding string
	decoded  string
	raw      []byte
}{
	{
		"ISO 2022 IR 13",
		"ﾔﾏﾀﾞ^ﾀﾛｳ",
		[]byte{0x1b, 0x29, 0x49, 0xd4, 0xcf, 0xc0, 0xde, 0x5e, 0x1b, 0x29, 0x49, 0xc0, 0xdb, 0xb3},
	},
	{
		"ISO 2022 IR 100",
		"Buc^Jérôme",
		[]byte{0x1b, 0x2d, 0x41, 0x42, 0x75, 0x63, 0x5e, 0x1b, 0x2d, 0x41, 0x4a, 0xe9, 0x72, 0xf4, 0x6d, 0x65},
	},
	{"ISO 2022 IR 101", "Wałęsa", []byte{0x1b, 0x2d, 0x42, 0x57, 0x61, 0xb3, 0xea, 0x73, 0x61}},
	{
		"ISO 2022 IR 109",
		"antaŭnomo",
		[]byte{0x1b, 0x2d, 0x43, 0x61, 0x6e, 0x74, 0x61, 0xfd, 0x6e, 0x6f, 0x6d, 0x6f},
	},
	{"ISO 2022 IR 110", "vārds", []byte{0x1b, 0x2d, 0x44, 0x76, 0xe0, 0x72, 0x64, 0x73}},
	{
		"ISO 2022 IR 127",
		"قباني^لنزار",
		[]byte{0x1b, 0x2d, 0x47, 0xe2, 0xc8, 0xc7, 0xe6, 0xea, 0x5e, 0x1b, 0x2d, 0x47, 0xe4, 0xe6, 0xd2, 0xc7, 0xd1},
	},
	{
		"ISO 2022 IR 126",
		"Διονυσιος",
		[]byte{0x1b, 0x2d, 0x46, 0xc4, 0xe9, 0xef, 0xed, 0xf5, 0xf3, 0xe9, 0xef, 0xf2},
	},
	{
		"ISO 2022 IR 138",
		"שרון^דבורה",
		[]byte{0x1b, 0x2d, 0x48, 0xf9, 0xf8, 0xe5, 0xef, 0x5e, 0x1b, 0x2d, 0x48, 0xe3, 0xe1, 0xe5, 0xf8, 0xe4},
	},
	{
		"ISO 2022 IR 144",
		"Люкceмбypг",
		[]byte{0x1b, 0x2d, 0x4c, 0xbb, 0xee, 0xda, 0x63, 0x65, 0xdc, 0xd1, 0x79, 0x70, 0xd3},
	},
	{
		"ISO 2022 IR 148",
		"Çavuşoğlu",
		[]byte{0x1b, 0x2d, 0x4d, 0xc7, 0x61, 0x76, 0x75, 0xfe, 0x6f, 0xf0, 0x6c, 0x75},
	},
	{
		"ISO 2022 IR 166",
		"นามสกุล",
		[]byte{0x1b, 0x2d, 0x54, 0xb9, 0xd2, 0xc1, 0xca, 0xa1, 0xd8, 0xc5},
	},
}

func TestSingleByteCodeExtensions(t *testing.T) {
	for _, tt := range encodedNames {
		t.Run(tt.encoding, func(t *testing.T) {
			raw := append([]byte("ASCII+"), tt.raw...)
			got := DecodeBytesWithDelimiters(raw, []string{"", tt.encoding}, pnDelims)
			want := "ASCII+" + tt.decoded
			if got != want {
				t.Fatalf("DecodeBytesWithDelimiters = %q, want %q", got, want)
			}
		})
	}
}

func TestMultiCharsetDefaultValue(t *testing.T) {
	raw := []byte("Buc^J\xe9r\xf4me")
	got, err := convertPNWithCharsets(raw, []string{"ISO 2022 IR 100", "ISO 2022 IR 144"})
	if err != nil {
		t.Fatal(err)
	}
	pn, ok := got.(PersonName)
	if !ok {
		t.Fatalf("type = %T", got)
	}
	if pn.String() != "Buc^Jérôme" {
		t.Fatalf("PN = %q, want Buc^Jérôme", pn.String())
	}
}

func TestMultiCharsetPersonNameGroups(t *testing.T) {
	raw := []byte("Dionysios=\x1b\x2d\x46\xc4\xe9\xef\xed\xf5\xf3\xe9\xef\xf2")
	got, err := convertPNWithCharsets(raw, []string{"ISO 2022 IR 100", "ISO 2022 IR 126"})
	if err != nil {
		t.Fatal(err)
	}
	pn := got.(PersonName)
	if pn.String() != "Dionysios=Διονυσιος" {
		t.Fatalf("PN = %q", pn.String())
	}
}

func TestMultiCharsetText(t *testing.T) {
	raw := []byte("Dionysios is \x1b\x2d\x46\xc4\xe9\xef\xed\xf5\xf3\xe9\xef\xf2")
	got, err := convertTextWithCharsets(raw, []string{"ISO 2022 IR 100", "ISO 2022 IR 126"})
	if err != nil {
		t.Fatal(err)
	}
	if got.(string) != "Dionysios is Διονυσιος" {
		t.Fatalf("text = %q", got)
	}
}

func TestMultiCharsetMultiValuePersonName(t *testing.T) {
	raw := []byte(
		"Buc^J\xe9r\xf4me\\\x1b\x2d\x46" +
			"\xc4\xe9\xef\xed\xf5\xf3\xe9\xef\xf2\\" +
			"\x1b\x2d\x4c" +
			"\xbb\xee\xda\x63\x65\xdc\xd1\x79\x70\xd3",
	)
	got, err := convertPNWithCharsets(raw, []string{"ISO 2022 IR 100", "ISO 2022 IR 144", "ISO 2022 IR 126"})
	if err != nil {
		t.Fatal(err)
	}
	mv := got.(*MultiValue[PersonName])
	want := []string{"Buc^Jérôme", "Διονυσιος", "Люкceмбypг"}
	if mv.Len() != len(want) {
		t.Fatalf("len = %d", mv.Len())
	}
	for i, expected := range want {
		if mv.Get(i).String() != expected {
			t.Fatalf("[%d] = %q, want %q", i, mv.Get(i).String(), expected)
		}
	}
}

func TestMultiCharsetMultiValueText(t *testing.T) {
	raw := []byte(
		"Buc^J\xe9r\xf4me\\\x1b\x2d\x46" +
			"\xc4\xe9\xef\xed\xf5\xf3\xe9\xef\xf2\\" +
			"\x1b\x2d\x4c" +
			"\xbb\xee\xda\x63\x65\xdc\xd1\x79\x70\xd3",
	)
	got, err := convertTextWithCharsets(raw, []string{"ISO 2022 IR 100", "ISO 2022 IR 144", "ISO 2022 IR 126"})
	if err != nil {
		t.Fatal(err)
	}
	mv := got.(*MultiValue[string])
	want := []string{"Buc^Jérôme", "Διονυσιος", "Люкceмбypг"}
	if mv.Len() != len(want) {
		t.Fatalf("len = %d", mv.Len())
	}
	for i, expected := range want {
		if mv.Get(i) != expected {
			t.Fatalf("[%d] = %q, want %q", i, mv.Get(i), expected)
		}
	}
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	for _, tt := range encodedNames {
		t.Run(tt.encoding, func(t *testing.T) {
			decoded := DecodeBytesWithDelimiters(tt.raw, []string{"", tt.encoding}, pnDelims)
			encoded := EncodeBytesWithCharsets(decoded, []string{"", tt.encoding})
			got := DecodeBytesWithDelimiters(encoded, []string{"", tt.encoding}, pnDelims)
			if got != decoded {
				t.Fatalf("roundtrip = %q, want %q", got, decoded)
			}
		})
	}
}

func TestEncodePersonNameRoundtrip(t *testing.T) {
	raw := []byte("Dionysios=\x1b\x2d\x46\xc4\xe9\xef\xed\xf5\xf3\xe9\xef\xf2")
	charsets := []string{"ISO 2022 IR 100", "ISO 2022 IR 126"}
	pn, err := decodePersonNameBytes(raw, charsets)
	if err != nil {
		t.Fatal(err)
	}
	encoded := EncodePersonNameWithCharsets(pn, charsets)
	got, err := decodePersonNameBytes(encoded, charsets)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != pn.String() {
		t.Fatalf("roundtrip PN = %q, want %q", got.String(), pn.String())
	}
}

func TestWriteCharsetRoundtrip(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(TagCharset, VRCS, "ISO_IR 100"))
	ds.Set(NewDataElement(MustTag(0x00100010), VRPN, PersonName{Alphabetic: "Buc^Jérôme"}))

	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)
	if err := writeDataset(fp, ds, false, true, nil, false); err != nil {
		t.Fatal(err)
	}

	readDS := NewDataset()
	ctx := &readContext{data: buf.Bytes()}
	if _, err := readDatasetElements(buf.Bytes(), 0, int64(buf.Len()), readDS, false, true, []string{DefaultCharacterSet}, nil, ctx); err != nil {
		t.Fatal(err)
	}
	elem, ok := readDS.Get(MustTag(0x00100010))
	if !ok {
		t.Fatal("PatientName missing")
	}
	pn := elem.Value.(PersonName)
	if pn.String() != "Buc^Jérôme" {
		t.Fatalf("PatientName = %q", pn.String())
	}
}

func TestSequenceCharsetInheritance(t *testing.T) {
	// Parent default ASCII; SQ item declares ISO_IR 100 for its PatientName.
	item := NewDataset()
	item.Set(NewDataElement(TagCharset, VRCS, "ISO_IR 100"))
	item.Set(NewDataElement(MustTag(0x00100010), VRPN, PersonName{Alphabetic: "Buc^Jérôme"}))

	seq := NewSequence([]*Dataset{item})
	parent := NewDataset()
	parent.Set(NewDataElement(MustTag(0x00100060), VRPN, PersonName{Alphabetic: "ASCII^Name"}))
	parent.Set(NewDataElement(MustTag(0x00400020), VRSQ, seq))

	var buf bytes.Buffer
	fp := newDicomWriter(&buf)
	fp.SetByteOrder(true)
	if err := writeDataset(fp, parent, false, true, nil, false); err != nil {
		t.Fatal(err)
	}

	readDS := NewDataset()
	ctx := &readContext{data: buf.Bytes()}
	if _, err := readDatasetElements(buf.Bytes(), 0, int64(buf.Len()), readDS, false, true, []string{DefaultCharacterSet}, nil, ctx); err != nil {
		t.Fatal(err)
	}

	sqElem, ok := readDS.Get(MustTag(0x00400020))
	if !ok {
		t.Fatal("sequence missing")
	}
	sq := sqElem.Value.(*Sequence)
	if sq.Len() != 1 {
		t.Fatalf("sequence len = %d", sq.Len())
	}
	itemPN, ok := sq.Items()[0].Get(MustTag(0x00100010))
	if !ok {
		t.Fatal("item PatientName missing")
	}
	if itemPN.Value.(PersonName).String() != "Buc^Jérôme" {
		t.Fatalf("item PN = %q", itemPN.Value.(PersonName).String())
	}
	parentPN, ok := readDS.Get(MustTag(0x00100060))
	if !ok {
		t.Fatal("parent PN missing")
	}
	if parentPN.Value.(PersonName).String() != "ASCII^Name" {
		t.Fatalf("parent PN = %q", parentPN.Value.(PersonName).String())
	}
}

func TestReadWithSpecificCharacterSet(t *testing.T) {
	// Explicit VR little endian dataset with SpecificCharacterSet + PatientName.
	data := []byte{
		0x08, 0x00, 0x05, 0x00, 'C', 'S', 0x0a, 0x00,
		'I', 'S', 'O', '_', 'I', 'R', ' ', '1', '0', '0',
		0x10, 0x00, 0x10, 0x00, 'P', 'N', 0x0c, 0x00,
		0x42, 0x75, 0x63, 0x5e, 0x4a, 0xe9, 0x72, 0xf4, 0x6d, 0x65, 0x20, 0x20,
	}
	ds := NewDataset()
	ctx := &readContext{data: data}
	_, err := readDatasetElements(data, 0, int64(len(data)), ds, false, true, []string{DefaultCharacterSet}, nil, ctx)
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(MustTag(0x00100010))
	if !ok {
		t.Fatal("PatientName missing")
	}
	pn, ok := elem.Value.(PersonName)
	if !ok {
		t.Fatalf("value type = %T", elem.Value)
	}
	if pn.String() != "Buc^Jérôme" {
		t.Fatalf("PatientName = %q", pn.String())
	}
}
