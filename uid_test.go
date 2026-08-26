package godicom

import (
	"testing"

	"github.com/godicom-dev/godicom/uid"
)

func TestUIDKnown(t *testing.T) {
	if ImplicitVRLittleEndian.Name() != "Implicit VR Little Endian" {
		t.Errorf("got %s", ImplicitVRLittleEndian.Name())
	}
	if !ImplicitVRLittleEndian.IsImplicitVR() {
		t.Error("should be implicit VR")
	}
	if !ExplicitVRLittleEndian.IsLittleEndian() {
		t.Error("should be little endian")
	}
	if ExplicitVRBigEndian.IsLittleEndian() {
		t.Error("big endian should not be little endian")
	}
}

func TestUIDTransferSyntax(t *testing.T) {
	if !ImplicitVRLittleEndian.IsTransferSyntax() {
		t.Error("should be transfer syntax")
	}
	if VerificationSOPClass.IsTransferSyntax() {
		t.Error("should not be transfer syntax")
	}
}

func TestUIDCompressed(t *testing.T) {
	if !JPEGBaseline.IsCompressed() {
		t.Error("JPEG baseline should be compressed")
	}
	if ImplicitVRLittleEndian.IsCompressed() {
		t.Error("implicit VR LE should not be compressed")
	}
}

func TestLookupUID(t *testing.T) {
	got, ok := LookupUID("CTImageStorage")
	if !ok || got != CTImageStorage {
		t.Fatalf("LookupUID(CTImageStorage) = %q, %t", got, ok)
	}
	if _, ok := LookupUID("NotARealKeyword"); ok {
		t.Fatal("LookupUID should fail for an unknown keyword")
	}

	// UIDInfo is an alias for uid.Info. Nothing in the root package returns one
	// now that KnownUIDs is gone, so this is what keeps the alias honest: what
	// uid.Lookup hands back is still assignable to it.
	var info UIDInfo
	info, ok = uid.Lookup(ExplicitVRLittleEndian)
	if !ok || !info.IsTransferSyntax || !info.IsLittleEndian || info.IsImplicitVR {
		t.Fatalf("uid.Lookup(ExplicitVRLittleEndian) = %+v, %t", info, ok)
	}
}

func TestValidateUID(t *testing.T) {
	if err := ValidateUID("1.2.840.10008.1.2"); err != nil {
		t.Errorf("valid UID rejected: %v", err)
	}
	if err := ValidateUID(""); err == nil {
		t.Error("empty UID should be rejected")
	}
	if err := ValidateUID("1.2.3.04.5"); err == nil {
		t.Error("leading zero should be rejected")
	}
	if err := ValidateUID("1.2.3.a.5"); err == nil {
		t.Error("non-numeric should be rejected")
	}
}
