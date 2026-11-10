package godicom

import (
	"errors"
	"fmt"
)

// ErrCharsetChangeLosesText is returned when a dataset is written after
// (0008,0005) was changed while text values still hold the undecoded bytes of
// the source file. Those bytes were never text, so re-encoding them replaces
// every non-ASCII run with U+FFFD; ReinterpretCharacterSet is the way to decode
// them under the declared set instead of re-encoding them.
var ErrCharsetChangeLosesText = errors.New("godicom: character set change would re-encode text that was never decoded")

// InvalidDICOMError is returned when data is not valid DICOM.
type InvalidDICOMError struct {
	Message string
}

func (e *InvalidDICOMError) Error() string {
	return fmt.Sprintf("godicom: invalid DICOM: %s", e.Message)
}

// BytesLengthError is returned when a value has an unexpected byte length.
type BytesLengthError struct {
	Expected int
	Actual   int
	VR       VR
}

func (e *BytesLengthError) Error() string {
	return fmt.Sprintf("godicom: expected %d bytes for VR %s, got %d", e.Expected, e.VR, e.Actual)
}

// TagError wraps an error with tag context.
type TagError struct {
	Tag Tag
	Err error
}

func (e *TagError) Error() string {
	return fmt.Sprintf("godicom: tag %s: %v", e.Tag, e.Err)
}

func (e *TagError) Unwrap() error { return e.Err }
