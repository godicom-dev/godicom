package godicom

import (
	"bytes"
	"testing"
)

func TestEncodeFileReadBytesRoundtrip(t *testing.T) {
	src, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := EncodeFile(src, &WriteOptions{EnforceFileFormat: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) < 132 || string(encoded[128:132]) != "DICM" {
		t.Fatalf("missing DICM prefix, len=%d", len(encoded))
	}

	got, err := ReadBytes(encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Len() != src.Len() {
		t.Fatalf("element count: got %d want %d", got.Len(), src.Len())
	}
	sop, _ := src.GetString(MustTag("SOPInstanceUID"))
	gotSOP, ok := got.GetString(MustTag("SOPInstanceUID"))
	if !ok || gotSOP != sop {
		t.Fatalf("SOPInstanceUID=%q want %q", gotSOP, sop)
	}

	var buf bytes.Buffer
	if err := src.Write(&buf, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), encoded) {
		t.Fatalf("Write vs EncodeFile mismatch: %d vs %d bytes", buf.Len(), len(encoded))
	}
}
