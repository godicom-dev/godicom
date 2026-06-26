package tag

import "testing"

func TestNew(t *testing.T) {
	tag := New(0x0010, 0x0020)
	if tag != PatientID {
		t.Fatalf("New(0x0010, 0x0020) = %s, want %s", tag, PatientID)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Tag
	}{
		{
			name:     "hex keyword value",
			input:    "00100010",
			expected: PatientName,
		},
		{
			name:     "tuple string",
			input:    "(0010,0020)",
			expected: PatientID,
		},
		{
			name:     "keyword",
			input:    "PixelData",
			expected: PixelData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.input, err)
			}
			if got != tt.expected {
				t.Fatalf("Parse(%q) = %s, want %s", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unknown keyword",
			input: "NotADICOMKeyword",
		},
		{
			name:  "invalid group",
			input: "(GGGG,0010)",
		},
		{
			name:  "invalid element",
			input: "(0010,GGGG)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.input); err == nil {
				t.Fatalf("Parse(%q) error = nil, want error", tt.input)
			}
		})
	}
}

func TestMustParse(t *testing.T) {
	if got := MustParse("PatientName"); got != PatientName {
		t.Fatalf("MustParse(PatientName) = %s, want %s", got, PatientName)
	}
}

func TestMustParsePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustParse did not panic")
		}
	}()
	_ = MustParse("NotADICOMKeyword")
}

func TestFromKeyword(t *testing.T) {
	got, err := FromKeyword("SpecificCharacterSet")
	if err != nil {
		t.Fatal(err)
	}
	if got != SpecificCharacterSet {
		t.Fatalf("FromKeyword(SpecificCharacterSet) = %s, want %s", got, SpecificCharacterSet)
	}
}

func TestByKeywordAndKeyword(t *testing.T) {
	got, ok := ByKeyword("PatientName")
	if !ok {
		t.Fatal("ByKeyword(PatientName) ok = false")
	}
	if got != PatientName {
		t.Fatalf("ByKeyword(PatientName) = %s, want %s", got, PatientName)
	}

	keyword, ok := Keyword(PatientName)
	if !ok {
		t.Fatal("Keyword(PatientName) ok = false")
	}
	if keyword != "PatientName" {
		t.Fatalf("Keyword(PatientName) = %q, want PatientName", keyword)
	}
}

func TestGeneratedKeywordCoverage(t *testing.T) {
	if len(keywordToTag) < 5000 {
		t.Fatalf("len(keywordToTag) = %d, want at least 5000", len(keywordToTag))
	}
	if len(tagToKeyword) < 5000 {
		t.Fatalf("len(tagToKeyword) = %d, want at least 5000", len(tagToKeyword))
	}

	for keyword, tag := range keywordToTag {
		got, ok := tagToKeyword[tag]
		if !ok {
			t.Fatalf("tagToKeyword missing tag %s for keyword %q", tag, keyword)
		}
		if got != keyword {
			t.Fatalf("tagToKeyword[%s] = %q, want %q", tag, got, keyword)
		}
	}
}

func TestProperties(t *testing.T) {
	tag := New(0x0010, 0x0020)
	if tag.Group() != 0x0010 {
		t.Fatalf("Group = %04X, want 0010", tag.Group())
	}
	if tag.Element() != 0x0020 {
		t.Fatalf("Element = %04X, want 0020", tag.Element())
	}
	if tag.IsPrivate() {
		t.Fatal("PatientID IsPrivate = true, want false")
	}
}

func TestPrivate(t *testing.T) {
	privateCreator := New(0x0009, 0x0010)
	if !privateCreator.IsPrivate() {
		t.Fatal("private creator IsPrivate = false")
	}
	if !privateCreator.IsPrivateCreator() {
		t.Fatal("private creator IsPrivateCreator = false")
	}

	privateData := New(0x0009, 0x1001)
	if privateData.PrivateCreator() != privateCreator {
		t.Fatalf("PrivateCreator = %s, want %s", privateData.PrivateCreator(), privateCreator)
	}
}

func TestStringJSONKeyAndMarshalText(t *testing.T) {
	tag := PatientName
	if tag.String() != "(0010,0010)" {
		t.Fatalf("String = %q, want (0010,0010)", tag.String())
	}
	if tag.JSONKey() != "00100010" {
		t.Fatalf("JSONKey = %q, want 00100010", tag.JSONKey())
	}
	text, err := tag.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != "(0010,0010)" {
		t.Fatalf("MarshalText = %q, want (0010,0010)", text)
	}
}
