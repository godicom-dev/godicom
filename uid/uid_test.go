package uid

import "testing"

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
	}{
		{
			name:             "implicit little endian",
			uid:              ImplicitVRLittleEndian,
			isTransferSyntax: true,
			isCompressed:     false,
			isImplicitVR:     true,
			isLittleEndian:   true,
		},
		{
			name:             "explicit little endian",
			uid:              ExplicitVRLittleEndian,
			isTransferSyntax: true,
			isCompressed:     false,
			isImplicitVR:     false,
			isLittleEndian:   true,
		},
		{
			name:             "explicit big endian",
			uid:              ExplicitVRBigEndian,
			isTransferSyntax: true,
			isCompressed:     false,
			isImplicitVR:     false,
			isLittleEndian:   false,
		},
		{
			name:             "jpeg baseline",
			uid:              JPEGBaseline,
			isTransferSyntax: true,
			isCompressed:     true,
			isImplicitVR:     false,
			isLittleEndian:   true,
		},
		{
			name:             "verification sop class",
			uid:              VerificationSOPClass,
			isTransferSyntax: false,
			isCompressed:     false,
			isImplicitVR:     false,
			isLittleEndian:   false,
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
			if got := tt.uid.IsImplicitVR(); got != tt.isImplicitVR {
				t.Fatalf("IsImplicitVR = %t, want %t", got, tt.isImplicitVR)
			}
			if got := tt.uid.IsLittleEndian(); got != tt.isLittleEndian {
				t.Fatalf("IsLittleEndian = %t, want %t", got, tt.isLittleEndian)
			}
		})
	}
}

func TestKnownUIDs(t *testing.T) {
	if len(Known) == 0 {
		t.Fatal("Known is empty")
	}
	info, ok := Known[ExplicitVRLittleEndian]
	if !ok {
		t.Fatal("Known missing ExplicitVRLittleEndian")
	}
	if info.UID != ExplicitVRLittleEndian {
		t.Fatalf("Info.UID = %q, want %q", info.UID, ExplicitVRLittleEndian)
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
