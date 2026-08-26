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
	if got := private.ExtraInfo(); got != "" {
		t.Fatalf("private ExtraInfo = %q, want empty", got)
	}
	if private.IsRetired() {
		t.Fatal("an unregistered UID cannot be retired")
	}
}

func TestLookup(t *testing.T) {
	info, ok := Lookup(ExplicitVRLittleEndian)
	if !ok {
		t.Fatal("Lookup(ExplicitVRLittleEndian) = _, false")
	}
	if info.UID != ExplicitVRLittleEndian {
		t.Errorf("Info.UID = %q, want %q", info.UID, ExplicitVRLittleEndian)
	}
	if info.Name != "Explicit VR Little Endian" {
		t.Errorf("Info.Name = %q", info.Name)
	}
	if info.Keyword != "ExplicitVRLittleEndian" || info.Type != "Transfer Syntax" {
		t.Errorf("Info.Keyword = %q, Info.Type = %q", info.Keyword, info.Type)
	}
	if !info.IsTransferSyntax || info.IsCompressed || info.IsImplicitVR || !info.IsLittleEndian {
		t.Errorf("Info flags = %+v", info)
	}

	// The default transfer syntax, which is the only implicit-VR one there is.
	info, ok = Lookup(ImplicitVRLittleEndian)
	if !ok {
		t.Fatal("Lookup(ImplicitVRLittleEndian) = _, false")
	}
	if !info.IsTransferSyntax || info.IsCompressed || !info.IsImplicitVR || !info.IsLittleEndian {
		t.Errorf("ImplicitVRLittleEndian flags = %+v", info)
	}
	if info.ExtraInfo != "Default Transfer Syntax for DICOM" {
		t.Errorf("Info.ExtraInfo = %q", info.ExtraInfo)
	}

	// A compressed one, to pin the flag that is derived rather than stored.
	info, ok = Lookup(JPEG2000Lossless)
	if !ok {
		t.Fatal("Lookup(JPEG2000Lossless) = _, false")
	}
	if !info.IsTransferSyntax || !info.IsCompressed || info.IsImplicitVR || !info.IsLittleEndian {
		t.Errorf("JPEG2000Lossless flags = %+v", info)
	}

	// A SOP Class has no encoding at all, so every encoding flag stays false
	// rather than claiming big-endian explicit VR.
	info, ok = Lookup(CTImageStorage)
	if !ok {
		t.Fatal("Lookup(CTImageStorage) = _, false")
	}
	if info.IsTransferSyntax || info.IsCompressed || info.IsImplicitVR || info.IsLittleEndian {
		t.Errorf("CTImageStorage flags = %+v", info)
	}

	// An unregistered UID is reported absent rather than answered with a zero
	// Info, which would be indistinguishable from a registered non-transfer-syntax.
	if info, ok := Lookup(UID("9.9.999.90009.1.2")); ok {
		t.Errorf("Lookup(private UID) = %+v, true", info)
	}
}

// Every registered UID resolves through Lookup. This is the invariant the exported
// Known map used to state as len(Known) == len(Dictionary), and it matters more now
// that the Info is built on demand: a UID in the table whose Info came out wrong
// would have been a wrong entry in that map instead.
func TestLookupCoversEveryEntry(t *testing.T) {
	for value, entry := range dictionary {
		info, ok := Lookup(UID(value))
		if !ok {
			t.Fatalf("Lookup(%q) = _, false", value)
		}
		if info.UID != UID(value) || info.Name != entry.Name || info.Type != entry.Type ||
			info.ExtraInfo != entry.ExtraInfo || info.Retired != entry.Retired ||
			info.Keyword != entry.Keyword {
			t.Fatalf("Lookup(%q) = %+v, want it to carry %+v", value, info, entry)
		}
		if want := entry.Type == "Transfer Syntax"; info.IsTransferSyntax != want {
			t.Fatalf("Lookup(%q).IsTransferSyntax = %t, want %t", value, info.IsTransferSyntax, want)
		}
	}
}

// Lookup hands out a copy. That is what makes the registry read-only rather than
// read-only by convention: the table is unexported now, and the Info a caller gets
// must not be a window back into it.
func TestLookupReturnsCopy(t *testing.T) {
	info, ok := Lookup(ExplicitVRLittleEndian)
	if !ok {
		t.Fatal("Lookup(ExplicitVRLittleEndian) = _, false")
	}
	info.Name = "rewritten"
	info.IsImplicitVR = true

	again, _ := Lookup(ExplicitVRLittleEndian)
	if again.Name != "Explicit VR Little Endian" || again.IsImplicitVR {
		t.Fatalf("the registry was reachable through the returned Info: %+v", again)
	}
	if ExplicitVRLittleEndian.Name() != "Explicit VR Little Endian" {
		t.Fatalf("UID.Name() = %q", ExplicitVRLittleEndian.Name())
	}
}

func TestLookupKeyword(t *testing.T) {
	u, ok := LookupKeyword("CTImageStorage")
	if !ok || u != CTImageStorage {
		t.Fatalf("LookupKeyword(CTImageStorage) = %q, %t", u, ok)
	}
	_, ok = LookupKeyword("NotARealKeyword")
	if ok {
		t.Fatal("LookupKeyword should fail for unknown keyword")
	}

	// The two directions agree: the keyword Lookup reports resolves back to the
	// UID it was read from.
	info, _ := Lookup(JPEG2000Lossless)
	back, ok := LookupKeyword(info.Keyword)
	if !ok || back != JPEG2000Lossless {
		t.Fatalf("LookupKeyword(%q) = %q, %t; want %q", info.Keyword, back, ok, JPEG2000Lossless)
	}
}

func TestStorageSOPClassUIDs(t *testing.T) {
	if CTImageStorage != UID("1.2.840.10008.5.1.4.1.1.2") {
		t.Fatalf("CTImageStorage = %q", CTImageStorage)
	}
}

// These six are registered in PS3.6 Table A-1 but absent from pydicom's
// _uid_dict.py, so generate_uid_dict.py supplies them from its STANDARD_ADDITIONS
// table instead of from the parse. That makes them the only entries in the
// dictionary that a pydicom submodule bump could drop without any other test
// noticing: the count would fall by six and every remaining assertion would still
// hold. This is the test that would fail.
//
// It checks all three generated maps, because they are emitted by separate loops:
// the constant, the dictionary (via Name/Type/IsRetired), and the keyword table
// (via LookupKeyword). A UID present in one and missing from another is a real
// failure mode.
func TestUIDsAheadOfPydicom(t *testing.T) {
	tests := []struct {
		uid     UID
		value   string
		name    string
		keyword string
	}{
		{CTImageStorageForProcessing, "1.2.840.10008.5.1.4.1.1.2.3",
			"CT Image Storage - For Processing", "CTImageStorageForProcessing"},
		{EnhancedCTImageStorageForProcessing, "1.2.840.10008.5.1.4.1.1.2.4",
			"Enhanced CT Image Storage - For Processing", "EnhancedCTImageStorageForProcessing"},
		{LegacyConvertedEnhancedCTImageStorageForProcessing, "1.2.840.10008.5.1.4.1.1.2.5",
			"Legacy Converted Enhanced CT Image Storage - For Processing", "LegacyConvertedEnhancedCTImageStorageForProcessing"},
		{WaveformPresentationStateStorage, "1.2.840.10008.5.1.4.1.1.9.100.1",
			"Waveform Presentation State Storage", "WaveformPresentationStateStorage"},
		{WaveformAcquisitionPresentationStateStorage, "1.2.840.10008.5.1.4.1.1.9.100.2",
			"Waveform Acquisition Presentation State Storage", "WaveformAcquisitionPresentationStateStorage"},
		{UltrasoundWaveformStorage, "1.2.840.10008.5.1.4.1.1.601.5",
			"Ultrasound Waveform Storage", "UltrasoundWaveformStorage"},
	}

	for _, tt := range tests {
		t.Run(tt.keyword, func(t *testing.T) {
			if string(tt.uid) != tt.value {
				t.Errorf("constant = %q, want %q", tt.uid, tt.value)
			}
			if got := tt.uid.Name(); got != tt.name {
				t.Errorf("Name() = %q, want %q", got, tt.name)
			}
			if got := tt.uid.Type(); got != "SOP Class" {
				t.Errorf("Type() = %q, want SOP Class", got)
			}
			if got := tt.uid.Keyword(); got != tt.keyword {
				t.Errorf("Keyword() = %q, want %q", got, tt.keyword)
			}
			if tt.uid.IsRetired() {
				t.Error("IsRetired() = true, want false")
			}
			if got, ok := LookupKeyword(tt.keyword); !ok || got != tt.uid {
				t.Errorf("LookupKeyword(%q) = %q, %t; want %q, true", tt.keyword, got, ok, tt.uid)
			}
		})
	}
}

func TestDictionarySize(t *testing.T) {
	if len(dictionary) < 400 {
		t.Fatalf("dictionary has only %d entries", len(dictionary))
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
