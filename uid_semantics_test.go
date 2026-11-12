package godicom

import (
	"testing"

	"github.com/godicom-dev/godicom/uid"
)

// Ported from pydicom tests/test_uid.py::TestUID, TestUIDPrivate and
// TestGenerateUID. Equality, inequality and hash are Python protocol cases; in
// Go a UID is a string and those are the language's own semantics.

func TestUIDNameTypesAndKeyword(t *testing.T) {
	// pydicom: TestUID::test_name, test_name_with_equal_hash, test_type,
	// test_keyword, test_info
	tests := []struct {
		name        string
		uid         UID
		wantName    string
		wantType    string
		wantKeyword string
	}{
		{"transfer_syntax", ImplicitVRLittleEndian, "Implicit VR Little Endian", "Transfer Syntax", "ImplicitVRLittleEndian"},
		{"sop_class", CTImageStorage, "CT Image Storage", "SOP Class", "CTImageStorage"},
		{"verification", VerificationSOPClass, "Verification SOP Class", "SOP Class", "Verification"},
		{"unknown_uid_names_itself", UID("1.2.3.4"), "1.2.3.4", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.uid.Name(); got != tt.wantName {
				t.Fatalf("Name() = %q, want %q", got, tt.wantName)
			}
			if got := tt.uid.Type(); got != tt.wantType {
				t.Fatalf("Type() = %q, want %q", got, tt.wantType)
			}
			if got := tt.uid.Keyword(); got != tt.wantKeyword {
				t.Fatalf("Keyword() = %q, want %q", got, tt.wantKeyword)
			}
		})
	}
}

func TestUIDIsRetired(t *testing.T) {
	// pydicom: TestUID::test_is_retired
	tests := []struct {
		name string
		uid  UID
		want bool
	}{
		{"explicit_vr_big_endian_is_retired", ExplicitVRBigEndian, true},
		{"implicit_vr_little_endian_is_current", ImplicitVRLittleEndian, false},
		{"ct_image_storage_is_current", CTImageStorage, false},
		{"unknown_uid_is_not_retired", UID("1.2.3.4"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.uid.IsRetired(); got != tt.want {
				t.Fatalf("IsRetired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUIDIsPrivate(t *testing.T) {
	// pydicom: TestUIDPrivate::test_is_private, TestUID::test_is_private
	tests := []struct {
		name string
		uid  UID
		want bool
	}{
		{"dicom_root_is_public", ImplicitVRLittleEndian, false},
		{"sop_class_is_public", CTImageStorage, false},
		{"dicom_root_without_dot_is_private", UID("1.2.840.100085.1"), true},
		{"foreign_root_is_private", UID("1.2.826.0.1.3680043.8.498.1"), true},
		{"short_uid_is_private", UID("1.2.3"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.uid.IsPrivate(); got != tt.want {
				t.Fatalf("IsPrivate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUIDTransferSyntaxFlags(t *testing.T) {
	// pydicom: TestUID::test_is_transfer_syntax, test_is_implicit_vr,
	// test_is_little_endian, test_is_deflated, test_is_compressed,
	// test_is_encapsulated (plus the same cases in TestUIDPrivate)
	tests := []struct {
		name         string
		uid          UID
		isTS         bool
		isImplicitVR bool
		isLittle     bool
		isDeflated   bool
		isCompressed bool
	}{
		{"implicit_vr_little_endian", ImplicitVRLittleEndian, true, true, true, false, false},
		{"explicit_vr_little_endian", ExplicitVRLittleEndian, true, false, true, false, false},
		{"explicit_vr_big_endian", ExplicitVRBigEndian, true, false, false, false, false},
		{"deflated", DeflatedExplicitVRLittleEndian, true, false, true, true, false},
		{"jpeg_baseline", JPEGBaseline, true, false, true, false, true},
		{"rle", RLELossless, true, false, true, false, true},
		{"jpeg2000", JPEG2000, true, false, true, false, true},
		{"sop_class_has_no_encoding", CTImageStorage, false, false, false, false, false},
		{"private_uid_has_no_encoding", UID("1.2.3.4"), false, false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.uid.IsTransferSyntax(); got != tt.isTS {
				t.Fatalf("IsTransferSyntax() = %v, want %v", got, tt.isTS)
			}
			if got := tt.uid.IsImplicitVR(); got != tt.isImplicitVR {
				t.Fatalf("IsImplicitVR() = %v, want %v", got, tt.isImplicitVR)
			}
			if got := tt.uid.IsLittleEndian(); got != tt.isLittle {
				t.Fatalf("IsLittleEndian() = %v, want %v", got, tt.isLittle)
			}
			if got := tt.uid.IsDeflated(); got != tt.isDeflated {
				t.Fatalf("IsDeflated() = %v, want %v", got, tt.isDeflated)
			}
			if got := tt.uid.IsCompressed(); got != tt.isCompressed {
				t.Fatalf("IsCompressed() = %v, want %v", got, tt.isCompressed)
			}
			if got := tt.uid.IsEncapsulated(); got != tt.isCompressed {
				t.Fatalf("IsEncapsulated() = %v, want it to match IsCompressed() = %v", got, tt.isCompressed)
			}
		})
	}
}

func TestUIDValidate(t *testing.T) {
	// pydicom: TestUID::test_is_valid (the rule set lives in validate_value)
	long := make64()
	tests := []struct {
		name    string
		uid     string
		wantErr bool
	}{
		{"valid_simple", "1.2.3", false},
		{"valid_transfer_syntax", "1.2.840.10008.1.2.1", false},
		{"valid_single_component", "1", false},
		{"valid_zero_component", "1.0.3", false},
		{"valid_64_chars", long, false},
		{"empty", "", true},
		{"65_chars", long + "1", true},
		{"trailing_dot", "1.2.3.", true},
		{"leading_zero_in_component", "1.02.3", true},
		{"empty_component", "1..3", true},
		{"non_numeric", "1.2.A", true},
		{"negative", "1.2.-1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUID(tt.uid)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateUID(%q) error = %v, wantErr %v", tt.uid, err, tt.wantErr)
			}
			if got := UID(tt.uid).IsValid(); got != !tt.wantErr {
				t.Fatalf("IsValid(%q) = %v, want %v", tt.uid, got, !tt.wantErr)
			}
		})
	}
}

func make64() string {
	// Exactly 64 characters: the longest UID DICOM allows.
	s := "1"
	for len(s) < 63 {
		s += ".1"
	}
	return s + "0"
}

func TestUIDLookupAndKeywordLookup(t *testing.T) {
	// pydicom: TestUID::test_info, and the uid dictionary round trip
	info, ok := uid.Lookup(uid.CTImageStorage)
	if !ok {
		t.Fatal("Lookup(CTImageStorage) not found")
	}
	if info.Name != "CT Image Storage" || info.Keyword != "CTImageStorage" || info.Type != "SOP Class" {
		t.Fatalf("Lookup(CTImageStorage) = %+v", info)
	}

	got, ok := LookupUID("CTImageStorage")
	if !ok {
		t.Fatal("LookupUID(CTImageStorage) not found")
	}
	if got != CTImageStorage {
		t.Fatalf("LookupUID(CTImageStorage) = %s, want %s", got, CTImageStorage)
	}

	if _, ok := LookupUID("NoSuchKeyword"); ok {
		t.Fatal("LookupUID(NoSuchKeyword) found, want not found")
	}
}

func TestGenerateUIDShape(t *testing.T) {
	// pydicom: TestGenerateUID::test_generate_uid, test_entropy_src,
	// test_entropy_src_custom, test_none
	first, err := uid.GenerateUID()
	if err != nil {
		t.Fatalf("GenerateUID() error = %v", err)
	}
	if !first.IsValid() {
		t.Fatalf("GenerateUID() = %q, not a valid UID", first)
	}
	if !first.IsPrivate() {
		t.Fatalf("GenerateUID() = %q, want a private (non-DICOM-root) UID", first)
	}

	second, err := uid.GenerateUID()
	if err != nil {
		t.Fatalf("GenerateUID() error = %v", err)
	}
	if first == second {
		t.Fatalf("two GenerateUID() calls returned the same UID %q", first)
	}
}

func TestGenerateUIDWithEntropyIsDeterministic(t *testing.T) {
	// pydicom: TestGenerateUID::test_entropy_src_custom -- the same entropy has to
	// produce the same UID, different entropy a different one.
	gen := func(entropy string) UID {
		t.Helper()
		got, err := uid.GenerateUID(uid.WithEntropy(entropy))
		if err != nil {
			t.Fatalf("GenerateUID(WithEntropy(%q)) error = %v", entropy, err)
		}
		return got
	}

	a := gen("patient-42")
	b := gen("patient-42")
	c := gen("patient-43")

	if a != b {
		t.Fatalf("same entropy produced %q and %q", a, b)
	}
	if a == c {
		t.Fatalf("different entropy produced the same UID %q", a)
	}
	if !a.IsValid() {
		t.Fatalf("entropy UID %q is not valid", a)
	}
}
