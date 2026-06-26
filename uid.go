package godicom

import "github.com/godicom-dev/godicom/uid"

// UID represents a DICOM Unique Identifier.
type UID = uid.UID

const (
	ImplicitVRLittleEndian         UID = uid.ImplicitVRLittleEndian
	ExplicitVRLittleEndian         UID = uid.ExplicitVRLittleEndian
	DeflatedExplicitVRLittleEndian UID = uid.DeflatedExplicitVRLittleEndian
	ExplicitVRBigEndian            UID = uid.ExplicitVRBigEndian
	JPEGBaseline                   UID = uid.JPEGBaseline
	JPEGExtended                   UID = uid.JPEGExtended
	JPEGLossless                   UID = uid.JPEGLossless
	JPEGLosslessSV1                UID = uid.JPEGLosslessSV1
	JPEGLSLossless                 UID = uid.JPEGLSLossless
	JPEGLSLossy                    UID = uid.JPEGLSLossy
	JPEG2000Lossless               UID = uid.JPEG2000Lossless
	JPEG2000                       UID = uid.JPEG2000
	RLELossless                    UID = uid.RLELossless
	NativePixels                   UID = uid.NativePixels
	VerificationSOPClass           UID = uid.VerificationSOPClass
	PYDICOMImplementationUID       UID = uid.PYDICOMImplementationUID
)

// UIDInfo holds metadata about a UID.
type UIDInfo = uid.Info

// KnownUIDs maps UID strings to their info.
var KnownUIDs = uid.Known

// ValidateUID checks if the UID string conforms to DICOM rules.
func ValidateUID(s string) error {
	return uid.Validate(s)
}
