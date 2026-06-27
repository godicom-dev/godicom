package uid

import (
	"strings"
	"testing"
)

func TestUIDName(t *testing.T) {
	if got := ImplicitVRLittleEndian.Name(); got != "Implicit VR Little Endian" {
		t.Fatalf("Name = %q, want Implicit VR Little Endian", got)
	}

	unknown := UID("1.2.3.4")
	if got := unknown.Name(); got != "1.2.3.4" {
		t.Fatalf("unknown Name = %q, want raw UID", got)
	}
}

func TestUIDTransferSyntaxProperties(t *testing.T) {
	tests := []struct {
		name             string
		uid              UID
		isTransferSyntax bool
		isCompressed     bool
		isImplicitVR     bool
		isLittleEndian   bool
		isDeflated       bool
	}{
		{
			name:             "implicit little endian",
			uid:              ImplicitVRLittleEndian,
			isTransferSyntax: true,
			isCompressed:     false,
			isImplicitVR:     true,
			isLittleEndian:   true,
			isDeflated:       false,
		},
		{
			name:             "explicit little endian",
			uid:              ExplicitVRLittleEndian,
			isTransferSyntax: true,
			isCompressed:     false,
			isImplicitVR:     false,
			isLittleEndian:   true,
			isDeflated:       false,
		},
		{
			name:             "deflated explicit little endian",
			uid:              DeflatedExplicitVRLittleEndian,
			isTransferSyntax: true,
			isCompressed:     false,
			isImplicitVR:     false,
			isLittleEndian:   true,
			isDeflated:       true,
		},
		{
			name:             "explicit big endian",
			uid:              ExplicitVRBigEndian,
			isTransferSyntax: true,
			isCompressed:     false,
			isImplicitVR:     false,
			isLittleEndian:   false,
			isDeflated:       false,
		},
		{
			name:             "jpeg baseline",
			uid:              JPEGBaseline8Bit,
			isTransferSyntax: true,
			isCompressed:     true,
			isImplicitVR:     false,
			isLittleEndian:   true,
			isDeflated:       false,
		},
		{
			name:             "verification sop class",
			uid:              VerificationSOPClass,
			isTransferSyntax: false,
			isCompressed:     false,
			isImplicitVR:     false,
			isLittleEndian:   false,
			isDeflated:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.uid.IsTransferSyntax(); got != tt.isTransferSyntax {
				t.Fatalf("IsTransferSyntax = %t, want %t", got, tt.isTransferSyntax)
			}
			if got := tt.uid.IsCompressed(); got != tt.isCompressed {
				t.Fatalf("IsCompressed = %t, want %t", got, tt.isCompressed)
			}
			if got := tt.uid.IsEncapsulated(); got != tt.isCompressed {
				t.Fatalf("IsEncapsulated = %t, want %t", got, tt.isCompressed)
			}
			if got := tt.uid.IsImplicitVR(); got != tt.isImplicitVR {
				t.Fatalf("IsImplicitVR = %t, want %t", got, tt.isImplicitVR)
			}
			if got := tt.uid.IsLittleEndian(); got != tt.isLittleEndian {
				t.Fatalf("IsLittleEndian = %t, want %t", got, tt.isLittleEndian)
			}
			if got := tt.uid.IsDeflated(); got != tt.isDeflated {
				t.Fatalf("IsDeflated = %t, want %t", got, tt.isDeflated)
			}
		})
	}
}

func TestUIDDictionaryMetadata(t *testing.T) {
	if got := CTImageStorage.Name(); got != "CT Image Storage" {
		t.Fatalf("CTImageStorage.Name() = %q", got)
	}
	if got := CTImageStorage.Type(); got != "SOP Class" {
		t.Fatalf("CTImageStorage.Type() = %q", got)
	}
	if got := CTImageStorage.Keyword(); got != "CTImageStorage" {
		t.Fatalf("CTImageStorage.Keyword() = %q", got)
	}
	if got := ImplicitVRLittleEndian.ExtraInfo(); got != "Default Transfer Syntax for DICOM" {
		t.Fatalf("ImplicitVRLittleEndian.ExtraInfo() = %q", got)
	}
	if !ExplicitVRBigEndian.IsRetired() {
		t.Fatal("ExplicitVRBigEndian should be retired")
	}
}

func TestUIDPrivate(t *testing.T) {
	private := UID("9.9.999.90009.1.2")
	if !private.IsPrivate() {
		t.Fatal("expected private UID")
	}
	if private.IsTransferSyntax() {
		t.Fatal("private UID without registration should not be transfer syntax")
	}
	if got := private.Name(); got != "9.9.999.90009.1.2" {
		t.Fatalf("private Name = %q", got)
	}
	if got := private.Type(); got != "" {
		t.Fatalf("private Type = %q, want empty", got)
	}
	if got := private.Keyword(); got != "" {
		t.Fatalf("private Keyword = %q, want empty", got)
	}
}

func TestLookup(t *testing.T) {
	u, ok := Lookup("CTImageStorage")
	if !ok || u != CTImageStorage {
		t.Fatalf("Lookup(CTImageStorage) = %q, %t", u, ok)
	}
	_, ok = Lookup("NotARealKeyword")
	if ok {
		t.Fatal("Lookup should fail for unknown keyword")
	}
}

func TestStorageSOPClassUIDs(t *testing.T) {
	if CTImageStorage != UID("1.2.840.10008.5.1.4.1.1.2") {
		t.Fatalf("CTImageStorage = %q", CTImageStorage)
	}
}

func TestKnownUIDs(t *testing.T) {
	if len(Known) != len(Dictionary) {
		t.Fatalf("len(Known) = %d, want %d", len(Known), len(Dictionary))
	}
	info, ok := Known[ExplicitVRLittleEndian]
	if !ok {
		t.Fatal("Known missing ExplicitVRLittleEndian")
	}
	if info.UID != ExplicitVRLittleEndian {
		t.Fatalf("Info.UID = %q, want %q", info.UID, ExplicitVRLittleEndian)
	}
	if !info.IsTransferSyntax {
		t.Fatal("ExplicitVRLittleEndian should be transfer syntax in Known")
	}
}

func TestDictionarySize(t *testing.T) {
	if len(Dictionary) < 400 {
		t.Fatalf("Dictionary has only %d entries", len(Dictionary))
	}
}

func TestBackwardCompatAliases(t *testing.T) {
	if JPEGBaseline != JPEGBaseline8Bit {
		t.Fatal("JPEGBaseline alias mismatch")
	}
	if JPEGExtended != JPEGExtended12Bit {
		t.Fatal("JPEGExtended alias mismatch")
	}
	if JPEGLSLossy != JPEGLSNearLossless {
		t.Fatal("JPEGLSLossy alias mismatch")
	}
	if VerificationSOPClass != Verification {
		t.Fatal("VerificationSOPClass alias mismatch")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid",
			input:   "1.2.840.10008.1.2",
			wantErr: false,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "empty component",
			input:   "1..2",
			wantErr: true,
		},
		{
			name:    "leading zero",
			input:   "1.02.3",
			wantErr: true,
		},
		{
			name:    "non numeric",
			input:   "1.2.a",
			wantErr: true,
		},
		{
			name:    "too long",
			input:   "1.12345678901234567890123456789012345678901234567890123456789012345",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.input)
			if tt.wantErr && err == nil {
				t.Fatal("Validate error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate error = %v, want nil", err)
			}
		})
	}
}

func TestIsValid(t *testing.T) {
	for _, s := range []string{"1", "0.1", "1.0.23", strings.Repeat("1", 64), "1." + strings.Repeat("2", 62)} {
		if !UID(s).IsValid() {
			t.Fatalf("IsValid false for %q", s)
		}
	}

	for _, s := range []string{"", ".", "1.", "1.01", "1.a"} {
		if UID(s).IsValid() {
			t.Fatalf("IsValid true for invalid %q", s)
		}
	}
}
