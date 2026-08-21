package godicom

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeElementHeader(t *testing.T) {
	t.Parallel()
	patientName := MustTag("PatientName")
	pixelData := MustTag(0x7FE00010)

	cases := []struct {
		name       string
		data       []byte
		tag        Tag
		implicitVR bool
		bigEndian  bool
		wantVR     VR
		wantLength int
		wantSize   int
		wantNeed   int64
		wantOK     bool
	}{
		{
			name:       "implicit takes the VR from the dictionary",
			data:       []byte{0x10, 0x00, 0x10, 0x00, 8, 0, 0, 0},
			tag:        patientName,
			implicitVR: true,
			wantVR:     VRPN,
			wantLength: 8,
			wantSize:   8,
			wantOK:     true,
		},
		{
			name:       "explicit VR with a 16-bit length",
			data:       []byte{0x10, 0x00, 0x10, 0x00, 'P', 'N', 8, 0},
			tag:        patientName,
			wantVR:     VRPN,
			wantLength: 8,
			wantSize:   8,
			wantOK:     true,
		},
		{
			name:       "explicit VR with a 32-bit length skips two reserved bytes",
			data:       []byte{0xE0, 0x7F, 0x10, 0x00, 'O', 'B', 0, 0, 0x00, 0x10, 0, 0},
			tag:        pixelData,
			wantVR:     VROB,
			wantLength: 0x1000,
			wantSize:   12,
			wantOK:     true,
		},
		{
			// pydicom issues 1067 and 1035: VR bytes that are not two uppercase
			// letters mean this element is really implicit VR.
			name:       "non-alphabetic VR bytes fall back to implicit",
			data:       []byte{0x10, 0x00, 0x10, 0x00, 8, 0, 0, 0},
			tag:        patientName,
			wantVR:     VRPN,
			wantLength: 8,
			wantSize:   8,
			wantOK:     true,
		},
		{
			name:       "big endian reads the length the other way round",
			data:       []byte{0x00, 0x10, 0x00, 0x10, 'P', 'N', 0, 8},
			tag:        patientName,
			bigEndian:  true,
			wantVR:     VRPN,
			wantLength: 8,
			wantSize:   8,
			wantOK:     true,
		},
		{
			name:     "a header shorter than 8 bytes needs 8",
			data:     []byte{0x10, 0x00, 0x10, 0x00, 'P', 'N'},
			tag:      patientName,
			wantNeed: 8,
		},
		{
			// The VR is readable, so the decoder knows a 32-bit length was coming
			// and reports 12 rather than guessing.
			name:     "a 32-bit length cut short needs 12",
			data:     []byte{0xE0, 0x7F, 0x10, 0x00, 'O', 'B', 0, 0, 0x00, 0x10},
			tag:      pixelData,
			wantNeed: 12,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, need, ok := decodeElementHeader(tc.data, 0, tc.tag, EncodingInfo{
				IsImplicitVR:   tc.implicitVR,
				IsLittleEndian: !tc.bigEndian,
			}, nil)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if need != tc.wantNeed && !tc.wantOK {
				t.Errorf("need = %d, want %d", need, tc.wantNeed)
			}
			if !tc.wantOK {
				return
			}
			if h.VR != tc.wantVR {
				t.Errorf("VR = %s, want %s", h.VR, tc.wantVR)
			}
			if h.Length != tc.wantLength {
				t.Errorf("Length = %d, want %d", h.Length, tc.wantLength)
			}
			if h.Size != tc.wantSize {
				t.Errorf("Size = %d, want %d", h.Size, tc.wantSize)
			}
		})
	}
}

// A private tag has no VR on the wire in implicit VR, and the dictionary can
// only resolve it through the private creator.
func TestDecodeElementHeader_PrivateVRNeedsTheCreator(t *testing.T) {
	t.Parallel()
	tag := MustTag(0x00191015)
	data := []byte{0x19, 0x00, 0x15, 0x10, 4, 0, 0, 0}

	implicitLE := EncodingInfo{IsImplicitVR: true, IsLittleEndian: true}

	h, _, ok := decodeElementHeader(data, 0, tag, implicitLE, nil)
	if !ok {
		t.Fatal("decode failed")
	}
	if h.VR != VRUN {
		t.Errorf("without a creator VR = %s, want UN", h.VR)
	}

	h, _, ok = decodeElementHeader(data, 0, tag, implicitLE, func(Tag) string { return "Canon Inc." })
	if !ok {
		t.Fatal("decode failed")
	}
	if h.VR != VRDS {
		t.Errorf("with the creator VR = %s, want DS", h.VR)
	}
}

// privateCreatorDataset builds an implicit VR dataset holding a private element
// whose VR is only knowable through its creator.
func privateCreatorDataset(t *testing.T) ([]byte, Tag) {
	t.Helper()
	tag := MustTag(0x00191015)

	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00190010), VRLO, "Canon Inc."))
	ds.Set(NewDataElement(tag, VRDS, strings.TrimSuffix(strings.Repeat("1.5\\", 40), "\\")))
	data, err := EncodeDataset(ds, ImplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	return data, tag
}

// The deferred reload used to resolve the VR without the private creator, so a
// deferred private element came back as UN, mismatched the VR recorded on the
// first pass, and Get reported the tag as absent for the rest of the dataset's
// life. Reloading it must produce what reading it eagerly produces, in both
// readers.
func TestDeferredPrivateElement_ReloadsWithCreatorVR(t *testing.T) {
	t.Parallel()
	data, tag := privateCreatorDataset(t)

	path := filepath.Join(t.TempDir(), "private.dcm")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	eager, err := ReadBytes(data, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	want, ok := eager.Dataset.elements[tag]
	if !ok {
		t.Fatal("the private element was not parsed by an eager read")
	}
	if want.VR != VRDS {
		t.Fatalf("eager VR = %s, want DS; the private dictionary no longer backs this test", want.VR)
	}

	readers := map[string]func(*diagRecorder) (*FileDataset, error){
		"ReadBytes": func(rec *diagRecorder) (*FileDataset, error) {
			return ReadBytes(data, &ReadOptions{Force: true, DeferSize: 32, OnDiagnostic: rec.hook})
		},
		"ReadFile": func(rec *diagRecorder) (*FileDataset, error) {
			return ReadFile(path, &ReadOptions{Force: true, DeferSize: 32, OnDiagnostic: rec.hook})
		},
	}

	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := &diagRecorder{}
			fd, err := read(rec)
			if err != nil {
				t.Fatal(err)
			}
			elem, ok := fd.Dataset.elements[tag]
			if !ok {
				t.Fatal("the private element was not parsed")
			}
			if !elem.Deferred {
				t.Fatalf("element is not deferred (length %d); the test no longer covers the reload path", elem.ValueLength)
			}
			if elem.VR != VRDS {
				t.Fatalf("VR on the first pass = %s, want DS", elem.VR)
			}

			if err := fd.LoadDeferred(tag); err != nil {
				t.Fatalf("LoadDeferred: %v", err)
			}
			if len(rec.got) != 0 {
				t.Errorf("a successful reload reported %v", rec.got)
			}
			if _, ok := fd.Get(tag); !ok {
				t.Fatal("Get reports the reloaded element as absent")
			}
			if elem.VR != want.VR {
				t.Errorf("VR after reload = %s, want %s", elem.VR, want.VR)
			}
			if !reflect.DeepEqual(elem.Value, want.Value) {
				t.Errorf("reloaded value does not match an eager read:\n got %v\nwant %v", elem.Value, want.Value)
			}
		})
	}
}

// The whole point of one decoder is that the readers can no longer disagree
// about a malformed file. Each case below is cut so the last header is
// incomplete; both readers must keep the same elements and report the same
// diagnostic.
func TestReadersAgreeOnTruncatedHeaders(t *testing.T) {
	t.Parallel()

	// Explicit VR little endian, no File Meta:
	//   0: (0010,0010) PN len 8 "Doe^Jane"
	//  16: (7FE0,0010) OB, 12-byte header -- cut inside the 32-bit length
	full := []byte{
		0x10, 0x00, 0x10, 0x00, 'P', 'N', 8, 0x00,
		'D', 'o', 'e', '^', 'J', 'a', 'n', 'e',
		0xE0, 0x7F, 0x10, 0x00, 'O', 'B', 0x00, 0x00, 0x00, 0x10,
	}

	cases := []struct {
		name     string
		data     []byte
		wantNeed int64
		wantHave int64
	}{
		{name: "cut inside a 32-bit length", data: full, wantNeed: 12, wantHave: 10},
		{name: "cut inside the VR", data: full[:22], wantNeed: 8, wantHave: 6},
		{name: "cut just past the tag", data: full[:20], wantNeed: 8, wantHave: 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "cut.dcm")
			if err := os.WriteFile(path, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}

			type result struct {
				diag Diagnostic
				name string
			}
			var results []result
			for _, r := range []struct {
				name string
				read func(*diagRecorder) (*FileDataset, error)
			}{
				{"ReadBytes", func(rec *diagRecorder) (*FileDataset, error) {
					return ReadBytes(tc.data, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
				}},
				{"ReadFile", func(rec *diagRecorder) (*FileDataset, error) {
					return ReadFile(path, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
				}},
			} {
				rec := &diagRecorder{}
				fd, err := r.read(rec)
				if err != nil {
					t.Fatalf("%s: tolerant read must succeed: %v", r.name, err)
				}
				// Everything before the damage survives in both readers.
				if got, ok := fd.GetString(MustTag("PatientName")); !ok || got != "Doe^Jane" {
					t.Errorf("%s: PatientName = %q, %v", r.name, got, ok)
				}
				if fd.Has(MustTag(0x7FE00010)) {
					t.Errorf("%s: the truncated element must not be stored", r.name)
				}
				results = append(results, result{diag: rec.first(t), name: r.name})
			}

			for _, r := range results {
				if r.diag.Kind != DiagnosticTruncatedHeader {
					t.Errorf("%s: Kind = %q, want %q", r.name, r.diag.Kind, DiagnosticTruncatedHeader)
				}
				if r.diag.Offset != 16 {
					t.Errorf("%s: Offset = %d, want 16", r.name, r.diag.Offset)
				}
				if r.diag.Need != tc.wantNeed || r.diag.Have != tc.wantHave {
					t.Errorf("%s: Need/Have = %d/%d, want %d/%d",
						r.name, r.diag.Need, r.diag.Have, tc.wantNeed, tc.wantHave)
				}
			}
		})
	}
}
