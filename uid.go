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
	CTImageStorage                 UID = uid.CTImageStorage
	PYDICOMImplementationUID       UID = uid.PYDICOMImplementationUID
	GodicomImplementationUID       UID = uid.GodicomImplementationUID
)

// UIDInfo holds metadata about a UID.
type UIDInfo = uid.Info

// UIDDictionary maps UID values to their metadata.
var UIDDictionary = uid.Dictionary

// KnownUIDs maps UID strings to their info.
var KnownUIDs = uid.Known

// LookupUID returns the UID for a dictionary keyword.
func LookupUID(keyword string) (UID, bool) {
	return uid.Lookup(keyword)
}

// ValidateUID checks if the UID string conforms to DICOM rules.
func ValidateUID(s string) error {
	return uid.Validate(s)
}

// RootUID is the default prefix used by GenerateUID.
const RootUID = uid.RootUID

// GenerateOption configures GenerateUID.
type GenerateOption = uid.GenerateOption

// WithPrefix sets the UID prefix for GenerateUID.
func WithPrefix(prefix string) GenerateOption { return uid.WithPrefix(prefix) }

// WithUUIDPrefix generates a 2.25.<uuid4-as-int> UID.
func WithUUIDPrefix() GenerateOption { return uid.WithUUIDPrefix() }

// WithEntropy makes GenerateUID deterministic from the given sources.
func WithEntropy(srcs ...string) GenerateOption { return uid.WithEntropy(srcs...) }

// GenerateUID returns a DICOM UID of at most 64 characters.
// See [uid.GenerateUID] for behaviour aligned with common DICOM UID generators.
func GenerateUID(opts ...GenerateOption) (UID, error) {
	return uid.GenerateUID(opts...)
}

// MustGenerateUID is like GenerateUID but panics on error.
func MustGenerateUID(opts ...GenerateOption) UID {
	return uid.MustGenerateUID(opts...)
}
