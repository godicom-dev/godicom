package godicom

import (
	"errors"
	"testing"
)

func TestInvalidDICOMError(t *testing.T) {
	err := &InvalidDICOMError{Message: "missing DICM prefix"}
	expected := "godicom: invalid DICOM: missing DICM prefix"
	if err.Error() != expected {
		t.Fatalf("Error = %q, want %q", err.Error(), expected)
	}
}

func TestBytesLengthError(t *testing.T) {
	err := &BytesLengthError{
		Expected: 4,
		Actual:   2,
		VR:       VRUL,
	}
	expected := "godicom: expected 4 bytes for VR UL, got 2"
	if err.Error() != expected {
		t.Fatalf("Error = %q, want %q", err.Error(), expected)
	}
}

func TestTagError(t *testing.T) {
	inner := errors.New("invalid value")
	err := &TagError{
		Tag: MustTag(0x00100010),
		Err: inner,
	}
	expected := "godicom: tag (0010,0010): invalid value"
	if err.Error() != expected {
		t.Fatalf("Error = %q, want %q", err.Error(), expected)
	}
	if !errors.Is(err, inner) {
		t.Fatal("errors.Is(TagError, inner) = false")
	}
}
