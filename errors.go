package godicom

import "fmt"

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
