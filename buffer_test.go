package godicom

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestPathFromPathlike(t *testing.T) {
	got, err := pathFromPathlike("test.dcm")
	if err != nil {
		t.Fatal(err)
	}
	if got != "test.dcm" {
		t.Fatalf("pathFromPathlike = %q, want test.dcm", got)
	}
}

func TestPathFromPathlikeEmpty(t *testing.T) {
	if _, err := pathFromPathlike(""); err == nil {
		t.Fatal("pathFromPathlike empty error = nil, want error")
	}
}

func TestCheckBufferCurrentBehavior(t *testing.T) {
	if err := CheckBuffer(nil); err != nil {
		t.Fatalf("CheckBuffer(nil) = %v, want nil", err)
	}
}

func TestBufferLengthBytes(t *testing.T) {
	got, err := BufferLength([]byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if got != 3 {
		t.Fatalf("BufferLength = %d, want 3", got)
	}
}

func TestBufferLengthFileRestoresPosition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "buffer.bin")
	if err := os.WriteFile(path, []byte{1, 2, 3, 4}, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if _, err := file.Seek(2, 0); err != nil {
		t.Fatal(err)
	}
	got, err := BufferLength(file)
	if err != nil {
		t.Fatal(err)
	}
	if got != 4 {
		t.Fatalf("BufferLength = %d, want 4", got)
	}
	pos, err := file.Seek(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 2 {
		t.Fatalf("file position = %d, want 2", pos)
	}
}

func TestBufferLengthUnsupported(t *testing.T) {
	got, err := BufferLength("not a buffer")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("BufferLength unsupported = %d, want 0", got)
	}
}

func TestBufferEquality(t *testing.T) {
	tests := []struct {
		name     string
		a        interface{}
		b        interface{}
		expected bool
	}{
		{
			name:     "equal bytes",
			a:        []byte{1, 2, 3},
			b:        []byte{1, 2, 3},
			expected: true,
		},
		{
			name:     "different length",
			a:        []byte{1, 2},
			b:        []byte{1, 2, 3},
			expected: false,
		},
		{
			name:     "different content",
			a:        []byte{1, 2, 3},
			b:        []byte{1, 2, 4},
			expected: false,
		},
		{
			name:     "unsupported",
			a:        []byte{1},
			b:        "not bytes",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BufferEquality(tt.a, tt.b); got != tt.expected {
				t.Fatalf("BufferEquality = %t, want %t", got, tt.expected)
			}
		})
	}
}

func TestUnackTag(t *testing.T) {
	if got := unackTag([]byte{0x20, 0x00, 0x10, 0x00}, true); got != MustTag(0x00100020) {
		t.Fatalf("unackTag little = %s, want (0010,0020)", got)
	}
	if got := unackTag([]byte{0x00, 0x10, 0x00, 0x20}, false); got != MustTag(0x00100020) {
		t.Fatalf("unackTag big = %s, want (0010,0020)", got)
	}
}

func TestReadUndefinedLengthValueImplicit(t *testing.T) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint32(MustTag(0x00100010)))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(2))
	buf.Write([]byte("AB"))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(SequenceDelimiterTag))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))

	fp := newDicomReader(bytes.NewReader(buf.Bytes()))
	fp.SetByteOrder(true)
	value, err := readUndefinedLengthValue(fp, SequenceDelimiterTag, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if value == nil {
		t.Fatal("readUndefinedLengthValue returned nil, want empty slice")
	}
}
