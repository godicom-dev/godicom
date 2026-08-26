// Package uid provides DICOM UID constants and lookup.
package uid

import (
	"fmt"
	"strings"
)

// UID represents a DICOM Unique Identifier.
type UID string

// Native encoding transfer syntaxes are uncompressed (not encapsulated).
var nativeEncoding = map[UID]struct{}{
	ImplicitVRLittleEndian:         {},
	ExplicitVRLittleEndian:         {},
	ExplicitVRBigEndian:            {},
	DeflatedExplicitVRLittleEndian: {},
}

// Non-dictionary UIDs used by godicom.
const (
	NativePixels             UID = ImplicitVRLittleEndian
	PYDICOMImplementationUID UID = "1.2.40.0.13.1.1.1"
	GodicomImplementationUID UID = "1.2.826.0.1.3680043.8.498.1"
)

// Backward-compatible aliases for earlier godicom constant names.
const (
	JPEGBaseline         = JPEGBaseline8Bit
	JPEGExtended         = JPEGExtended12Bit
	JPEGLSLossy          = JPEGLSNearLossless
	VerificationSOPClass = Verification
)

// Info is what Lookup returns: what the PS3.6 UID registry records about a UID,
// plus the encoding facts that follow from it.
//
// The four flags are only meaningful for a Transfer Syntax and are false for
// everything else, so IsTransferSyntax is the one to check first -- a SOP Class
// is not big-endian, it simply has no endianness at all.
type Info struct {
	UID              UID
	Name             string
	Type             string
	ExtraInfo        string
	Retired          bool
	Keyword          string
	IsTransferSyntax bool
	IsCompressed     bool
	IsImplicitVR     bool
	IsLittleEndian   bool
}

func (u UID) entry() (dictEntry, bool) {
	e, ok := dictionary[string(u)]
	return e, ok
}

// Lookup returns what the UID registry records about u, and whether it holds an
// entry for it at all. A UID it does not know -- a private one, or a typo -- is
// reported as absent rather than answered with a zero Info, which would read like
// a registered UID that happens to be no transfer syntax:
//
//	info, ok := uid.Lookup(ts)
//	if !ok || !info.IsTransferSyntax {
//		// ts is not something to encode with
//	}
//
// The Info is a copy, so the registry cannot be reached through it. That is the
// point: this table used to be an exported map, which let any caller anywhere in
// the process redefine what a transfer syntax means for every other caller.
func Lookup(u UID) (Info, bool) {
	entry, ok := u.entry()
	if !ok {
		return Info{}, false
	}
	info := Info{
		UID:       u,
		Name:      entry.Name,
		Type:      entry.Type,
		ExtraInfo: entry.ExtraInfo,
		Retired:   entry.Retired,
		Keyword:   entry.Keyword,
	}
	if entry.Type == "Transfer Syntax" {
		info.IsTransferSyntax = true
		info.IsCompressed = u.isCompressedTransferSyntax()
		info.IsImplicitVR = u == ImplicitVRLittleEndian
		info.IsLittleEndian = u != ExplicitVRBigEndian
	}
	return info, true
}

// LookupKeyword returns the UID that a PS3.6 keyword names, which is the reverse
// of Info.Keyword and the way to reach a UID whose name you have but whose value
// you do not:
//
//	u, ok := uid.LookupKeyword("CTImageStorage") // 1.2.840.10008.5.1.4.1.1.2
//
// The generated constants are the better answer when the keyword is known at
// compile time -- uid.CTImageStorage is checked by the compiler and this is not.
// This is for a keyword that arrives at run time, from a configuration file or a
// command line.
func LookupKeyword(keyword string) (UID, bool) {
	u, ok := keywordToUID[keyword]
	return u, ok
}

func (u UID) Name() string {
	if e, ok := u.entry(); ok {
		return e.Name
	}
	return string(u)
}

func (u UID) Type() string {
	if e, ok := u.entry(); ok {
		return e.Type
	}
	return ""
}

func (u UID) ExtraInfo() string {
	if e, ok := u.entry(); ok {
		return e.ExtraInfo
	}
	return ""
}

func (u UID) Keyword() string {
	if e, ok := u.entry(); ok {
		return e.Keyword
	}
	return ""
}

func (u UID) IsRetired() bool {
	if e, ok := u.entry(); ok {
		return e.Retired
	}
	return false
}

func (u UID) IsPrivate() bool {
	return !strings.HasPrefix(string(u), "1.2.840.10008.")
}

func (u UID) IsTransferSyntax() bool {
	if e, ok := u.entry(); ok {
		return e.Type == "Transfer Syntax"
	}
	return false
}

func (u UID) isCompressedTransferSyntax() bool {
	if !u.IsTransferSyntax() {
		return false
	}
	_, native := nativeEncoding[u]
	return !native
}

func (u UID) IsCompressed() bool {
	return u.isCompressedTransferSyntax()
}

func (u UID) IsEncapsulated() bool {
	return u.IsCompressed()
}

func (u UID) IsDeflated() bool {
	return u.IsTransferSyntax() && u == DeflatedExplicitVRLittleEndian
}

func (u UID) IsImplicitVR() bool {
	if !u.IsTransferSyntax() {
		return false
	}
	return u == ImplicitVRLittleEndian
}

func (u UID) IsLittleEndian() bool {
	if !u.IsTransferSyntax() {
		return false
	}
	return u != ExplicitVRBigEndian
}

func (u UID) IsValid() bool {
	return Validate(string(u)) == nil
}

// Validate checks if the UID string conforms to DICOM rules.
func Validate(s string) error {
	if len(s) == 0 || len(s) > 64 {
		return fmt.Errorf("uid: length must be 1-64 characters, got %d", len(s))
	}
	parts := strings.Split(s, ".")
	for _, p := range parts {
		if len(p) == 0 {
			return fmt.Errorf("uid: %q has empty component", s)
		}
		if p[0] == '0' && len(p) > 1 {
			return fmt.Errorf("uid: %q has leading zero in component %q", s, p)
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return fmt.Errorf("uid: %q has non-numeric character %q", s, c)
			}
		}
	}
	return nil
}
