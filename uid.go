package godicom

import (
	"fmt"
	"strings"
)

// UID represents a DICOM Unique Identifier.
type UID string

const (
	// Transfer Syntax UIDs
	ImplicitVRLittleEndian            UID = "1.2.840.10008.1.2"
	ExplicitVRLittleEndian            UID = "1.2.840.10008.1.2.1"
	DeflatedExplicitVRLittleEndian    UID = "1.2.840.10008.1.2.1.99"
	ExplicitVRBigEndian               UID = "1.2.840.10008.1.2.2"
	JPEGBaseline                      UID = "1.2.840.10008.1.2.4.50"
	JPEGExtended                      UID = "1.2.840.10008.1.2.4.51"
	JPEGLossless                      UID = "1.2.840.10008.1.2.4.57"
	JPEGLosslessSV1                   UID = "1.2.840.10008.1.2.4.70"
	JPEGLSLossless                    UID = "1.2.840.10008.1.2.4.80"
	JPEGLSLossy                       UID = "1.2.840.10008.1.2.4.81"
	JPEG2000Lossless                  UID = "1.2.840.10008.1.2.4.90"
	JPEG2000                          UID = "1.2.840.10008.1.2.4.91"
	RLELossless                       UID = "1.2.840.10008.1.2.5"
	NativePixels                      UID = "1.2.840.10008.1.2"
	// Standard SOP Classes
	VerificationSOPClass              UID = "1.2.840.10008.1.1"
	// Implementation UID
	PYDICOMImplementationUID          UID = "1.2.40.0.13.1.1.1"
)

// UIDInfo holds metadata about a UID.
type UIDInfo struct {
	UID             UID
	Name            string
	Type            string
	IsTransferSyntax bool
	IsCompressed    bool
	IsImplicitVR    bool
	IsLittleEndian  bool
}

// KnownUIDs maps UID strings to their info.
var KnownUIDs = map[UID]UIDInfo{
	ImplicitVRLittleEndian:         {UID: ImplicitVRLittleEndian, Name: "Implicit VR Little Endian", Type: "Transfer Syntax", IsTransferSyntax: true, IsImplicitVR: true, IsLittleEndian: true},
	ExplicitVRLittleEndian:         {UID: ExplicitVRLittleEndian, Name: "Explicit VR Little Endian", Type: "Transfer Syntax", IsTransferSyntax: true, IsLittleEndian: true},
	DeflatedExplicitVRLittleEndian: {UID: DeflatedExplicitVRLittleEndian, Name: "Deflated Explicit VR Little Endian", Type: "Transfer Syntax", IsTransferSyntax: true, IsLittleEndian: true},
	ExplicitVRBigEndian:            {UID: ExplicitVRBigEndian, Name: "Explicit VR Big Endian", Type: "Transfer Syntax", IsTransferSyntax: true},
	JPEGBaseline:                   {UID: JPEGBaseline, Name: "JPEG Baseline (Process 1)", Type: "Transfer Syntax", IsTransferSyntax: true, IsCompressed: true},
	JPEGExtended:                   {UID: JPEGExtended, Name: "JPEG Extended (Process 2 & 4)", Type: "Transfer Syntax", IsTransferSyntax: true, IsCompressed: true},
	JPEGLossless:                   {UID: JPEGLossless, Name: "JPEG Lossless (Process 14)", Type: "Transfer Syntax", IsTransferSyntax: true, IsCompressed: true},
	JPEGLosslessSV1:                {UID: JPEGLosslessSV1, Name: "JPEG Lossless (SV1)", Type: "Transfer Syntax", IsTransferSyntax: true, IsCompressed: true},
	JPEGLSLossless:                 {UID: JPEGLSLossless, Name: "JPEG-LS Lossless", Type: "Transfer Syntax", IsTransferSyntax: true, IsCompressed: true},
	JPEGLSLossy:                    {UID: JPEGLSLossy, Name: "JPEG-LS Lossy (Near-Lossless)", Type: "Transfer Syntax", IsTransferSyntax: true, IsCompressed: true},
	JPEG2000Lossless:               {UID: JPEG2000Lossless, Name: "JPEG 2000 Lossless", Type: "Transfer Syntax", IsTransferSyntax: true, IsCompressed: true},
	JPEG2000:                       {UID: JPEG2000, Name: "JPEG 2000", Type: "Transfer Syntax", IsTransferSyntax: true, IsCompressed: true},
	RLELossless:                    {UID: RLELossless, Name: "RLE Lossless", Type: "Transfer Syntax", IsTransferSyntax: true, IsCompressed: true},
	VerificationSOPClass:           {UID: VerificationSOPClass, Name: "Verification SOP Class", Type: "SOP Class"},
}

func (u UID) Name() string {
	if info, ok := KnownUIDs[u]; ok {
		return info.Name
	}
	return string(u)
}

func (u UID) IsTransferSyntax() bool {
	if info, ok := KnownUIDs[u]; ok {
		return info.IsTransferSyntax
	}
	return false
}

func (u UID) IsCompressed() bool {
	if info, ok := KnownUIDs[u]; ok {
		return info.IsCompressed
	}
	return false
}

func (u UID) IsImplicitVR() bool {
	if info, ok := KnownUIDs[u]; ok {
		return info.IsImplicitVR
	}
	return u == ImplicitVRLittleEndian
}

func (u UID) IsLittleEndian() bool {
	if info, ok := KnownUIDs[u]; ok {
		return info.IsLittleEndian
	}
	return true
}

// ValidateUID checks if the UID string conforms to DICOM rules.
func ValidateUID(s string) error {
	if len(s) == 0 || len(s) > 64 {
		return fmt.Errorf("godicom: UID length must be 1-64 characters, got %d", len(s))
	}
	parts := strings.Split(s, ".")
	for _, p := range parts {
		if len(p) == 0 {
			return fmt.Errorf("godicom: UID %q has empty component", s)
		}
		if p[0] == '0' && len(p) > 1 {
			return fmt.Errorf("godicom: UID %q has leading zero in component %q", s, p)
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return fmt.Errorf("godicom: UID %q has non-numeric character %q", s, c)
			}
		}
	}
	return nil
}
