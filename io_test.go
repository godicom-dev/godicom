package godicom

import (
	"bytes"
	"io"
	"testing"
)

func TestDicomIOReadTag(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		isLittleEndian bool
		expected       Tag
	}{
		{
			name:           "little endian",
			data:           []byte{0x10, 0x00, 0x20, 0x00},
			isLittleEndian: true,
			expected:       MustTag(0x00200010),
		},
		{
			name:           "big endian",
			data:           []byte{0x00, 0x10, 0x00, 0x20},
			isLittleEndian: false,
			expected:       MustTag(0x00100020),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := NewDicomReader(bytes.NewReader(tt.data))
			fp.SetByteOrder(tt.isLittleEndian)
			got, err := fp.ReadTag()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.expected {
				t.Fatalf("ReadTag = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestDicomIOReadUint16(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		isLittleEndian bool
		expected       uint16
	}{
		{
			name:           "little endian",
			data:           []byte{0xFF, 0x00},
			isLittleEndian: true,
			expected:       0x00FF,
		},
		{
			name:           "big endian",
			data:           []byte{0x00, 0xFF},
			isLittleEndian: false,
			expected:       0x00FF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := NewDicomReader(bytes.NewReader(tt.data))
			fp.SetByteOrder(tt.isLittleEndian)
			got, err := fp.ReadUint16()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.expected {
				t.Fatalf("ReadUint16 = %04X, want %04X", got, tt.expected)
			}
		})
	}
}

func TestDicomIOReadUint32(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		isLittleEndian bool
		expected       uint32
	}{
		{
			name:           "little endian",
			data:           []byte{0xFE, 0xFF, 0xFF, 0xFF},
			isLittleEndian: true,
			expected:       0xFFFFFFFE,
		},
		{
			name:           "big endian",
			data:           []byte{0xFF, 0xFF, 0xFF, 0xFE},
			isLittleEndian: false,
			expected:       0xFFFFFFFE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := NewDicomReader(bytes.NewReader(tt.data))
			fp.SetByteOrder(tt.isLittleEndian)
			got, err := fp.ReadUint32()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.expected {
				t.Fatalf("ReadUint32 = %08X, want %08X", got, tt.expected)
			}
		})
	}
}

func TestDicomIOWritePrimitiveValues(t *testing.T) {
	var buf bytes.Buffer
	fp := NewDicomWriter(&buf)
	fp.SetByteOrder(true)

	if err := fp.WriteUint16(0x00FF); err != nil {
		t.Fatal(err)
	}
	if err := fp.WriteUint32(0xFFFFFFFE); err != nil {
		t.Fatal(err)
	}
	if err := fp.WriteTag(MustTag(0x00100020)); err != nil {
		t.Fatal(err)
	}

	expected := []byte{0xFF, 0x00, 0xFE, 0xFF, 0xFF, 0xFF, 0x20, 0x00, 0x10, 0x00}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Fatalf("written bytes = % X, want % X", buf.Bytes(), expected)
	}
}

func TestDicomIOSeekTellAndRead(t *testing.T) {
	fp := NewDicomReader(bytes.NewReader([]byte{1, 2, 3, 4}))
	pos, err := fp.Seek(2, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 2 {
		t.Fatalf("Seek pos = %d, want 2", pos)
	}
	if got := fp.Tell(); got != 2 {
		t.Fatalf("Tell = %d, want 2", got)
	}
	buf := make([]byte, 2)
	if _, err := fp.Read(buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, []byte{3, 4}) {
		t.Fatalf("Read = %v, want [3 4]", buf)
	}
}

func TestDicomBytesIO(t *testing.T) {
	fp := NewDicomBytesIO([]byte{1, 2, 3, 4})
	if got := fp.Len(); got != 4 {
		t.Fatalf("Len = %d, want 4", got)
	}
	buf := make([]byte, 2)
	if _, err := fp.Read(buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, []byte{1, 2}) {
		t.Fatalf("Read = %v, want [1 2]", buf)
	}
	if _, err := fp.Seek(1, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := fp.ReadAt(buf, 2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, []byte{3, 4}) {
		t.Fatalf("ReadAt = %v, want [3 4]", buf)
	}
}

func TestDicomBytesIOBytesCurrentBehavior(t *testing.T) {
	fp := NewDicomBytesIO([]byte{1, 2, 3})
	if got := fp.Bytes(); got != nil {
		t.Fatalf("Bytes = %v, want nil", got)
	}
}
