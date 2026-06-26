package godicom

import "testing"

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
