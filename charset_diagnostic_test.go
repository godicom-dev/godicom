package godicom

import (
	"bytes"
	"encoding/binary"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests cover #87: an unknown SpecificCharacterSet used to be rewritten
// to ISO_IR 6 with no warning, so a GB18030 Chinese patient name came back as
// mojibake and a PACS had no way to refuse the file. The bytes here are built
// by hand rather than through EncodeDataset, so the writer's own character-set
// encoding cannot hide a reader-side regression.

// charsetBareElement encodes one explicit VR little endian element using the
// short length form CS and PN use. Text VRs are padded with a space to an even
// length, exactly as PS3.5 requires.
func charsetBareElement(t *testing.T, tag Tag, vr VR, value []byte) []byte {
	t.Helper()
	v := append([]byte(nil), value...)
	if len(v)%2 != 0 {
		v = append(v, ' ')
	}
	b := make([]byte, 0, 8+len(v))
	b = binary.LittleEndian.AppendUint16(b, uint16(tag.Group()))
	b = binary.LittleEndian.AppendUint16(b, uint16(tag.Element()))
	b = append(b, byte(vr[0]), byte(vr[1]))
	b = binary.LittleEndian.AppendUint16(b, uint16(len(v)))
	return append(b, v...)
}

// charsetBareFile is a headerless dataset holding (0008,0005) and PatientName.
// An empty csValue omits (0008,0005) entirely, which is legal per PS3.5.
func charsetBareFile(t *testing.T, csValue string, pn []byte) []byte {
	t.Helper()
	var b []byte
	if csValue != "" {
		b = append(b, charsetBareElement(t, TagCharset, VRCS, []byte(csValue))...)
	}
	return append(b, charsetBareElement(t, MustTag("PatientName"), VRPN, pn)...)
}

func gb18030Bytes(t *testing.T, s string) []byte {
	t.Helper()
	b, err := EncodeString(s, "GB18030")
	if err != nil {
		t.Fatalf("encode %q as GB18030: %v", s, err)
	}
	return b
}

func patientNameOf(t *testing.T, fd *FileDataset) PersonName {
	t.Helper()
	elem, ok := fd.Dataset.Get(MustTag("PatientName"))
	if !ok {
		t.Fatal("PatientName missing")
	}
	pn, ok := elem.Value.(PersonName)
	if !ok {
		t.Fatalf("PatientName type = %T, want PersonName (value %v)", elem.Value, elem.Value)
	}
	return pn
}

// Layer 0: CS multi-value padding and spelling variants must resolve to a real
// encoding key instead of being treated as unknown.
func TestParseCharacterSetsCheckedNormalizes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{"absent", nil, "ISO_IR 6"},
		{"empty", "", "ISO_IR 6"},
		{"plain", "GB18030", "GB18030"},
		{"single value padding", "GB18030 ", "GB18030"},
		{"multi value padding", `GB18030 \ISO_IR 100`, "GB18030"},
		{"second value padding", `ISO_IR 100\GB18030 `, "ISO_IR 100"},
		{"null padding", "ISO_IR 192\x00", "ISO_IR 192"},
		{"lower case", "gb18030", "GB18030"},
		{"hyphenated", "GB-18030", "GB18030"},
		{"ISOIR typo", "ISOIR192", "ISO_IR 192"},
		{"utf8 spelling", "UTF-8", "ISO_IR 192"},
		{"case folded", "iso_ir_100", "ISO_IR 100"},
		{"multi value object", NewMultiValue([]string{"GB18030 ", "ISO_IR 100"}), "GB18030"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sets, unknown := parseCharacterSetsChecked(tt.value)
			if unknown != nil {
				t.Fatalf("unexpected unknown character sets: %v", unknown)
			}
			if len(sets) == 0 || sets[0] != tt.want {
				t.Fatalf("sets = %v, want first = %q", sets, tt.want)
			}
			// The exported helper must agree, since write paths still use it.
			if got := ParseCharacterSets(tt.value); len(got) == 0 || got[0] != tt.want {
				t.Errorf("ParseCharacterSets = %v, want first = %q", got, tt.want)
			}
		})
	}
}

// Unknown and private terms stay unknown. The error keeps the original
// spelling and the index in the multi-valued element, which is the only thing
// that lets an operator find the offending device setting.
func TestParseCharacterSetsCheckedReportsUnknown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		value     interface{}
		wantNames []string
		wantFirst string
	}{
		{"single", "UNSUPPORTED", []string{"UNSUPPORTED"}, "ISO_IR 6"},
		{"private term", "ACME_PRIVATE", []string{"ACME_PRIVATE"}, "ISO_IR 6"},
		{"unknown IR number", "ISO_IR 999", []string{"ISO_IR 999"}, "ISO_IR 6"},
		// A standalone first value keeps its own repertoire; only the bad
		// extension is dropped, which is the existing filter rule.
		{"second value unknown", `GB18030\ACME_PRIVATE`, []string{"ACME_PRIVATE"}, "GB18030"},
		{"both unknown", `FOO\BAR`, []string{"FOO", "BAR"}, "ISO_IR 6"},
		{"wrong value type", 42, []string{"<int>"}, "ISO_IR 6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, unknown := parseCharacterSetsChecked(tt.value)
			if unknown == nil {
				t.Fatal("expected an unknown character set error")
			}
			msg := unknown.Error()
			for _, name := range tt.wantNames {
				if !strings.Contains(msg, name) {
					t.Errorf("error %q does not contain original value %q", msg, name)
				}
			}
			if !strings.Contains(msg, "index") {
				t.Errorf("error %q does not name the value index", msg)
			}
			// The loose wrapper still returns the historical fallback.
			if got := ParseCharacterSets(tt.value); len(got) == 0 || got[0] != tt.wantFirst {
				t.Errorf("ParseCharacterSets = %v, want first = %q", got, tt.wantFirst)
			}
		})
	}
}

// The core regression: a legal CS padding in a multi-valued (0008,0005) used
// to degrade the whole dataset to ISO_IR 6. Chinese names must still decode.
func TestReadGB18030ChineseNameWithCSPadding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cs   string
		pn   string
	}{
		{"plain", "GB18030", "王^小东"},
		{"single padding", "GB18030 ", "王^小东"},
		{"multi padding", `GB18030 \ISO_IR 100`, "王^小东"},
		{"four byte character", "GB18030", "王^𠮷"},
		{"three PN groups", "GB18030", "Wang^XiaoDong=王^小东=ワン^シャオトン"},
		{"lowercase alias", "gb18030", "王^小东"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := charsetBareFile(t, tt.cs, gb18030Bytes(t, tt.pn))
			rec := &diagRecorder{}
			fd, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if len(rec.got) != 0 {
				t.Fatalf("reported %d diagnostics on a valid file: %v", len(rec.got), rec.got)
			}
			if got := patientNameOf(t, fd).String(); got != tt.pn {
				t.Fatalf("PatientName = %q, want %q", got, tt.pn)
			}
		})
	}
}

// Layer 1: unknown (0008,0005) is reported once, with the tag, VR, a real
// offset and the unresolved value in Err. The read still succeeds by default.
func TestReadUnknownCharacterSetReportsDiagnostic(t *testing.T) {
	t.Parallel()
	data := charsetBareFile(t, "ACME_PRIVATE", gb18030Bytes(t, "王^小东"))
	rec := &diagRecorder{}
	fd, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err != nil {
		t.Fatalf("a hook returning nil must not fail the read: %v", err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("reported %d diagnostics, want 1: %v", len(rec.got), rec.got)
	}
	d := rec.got[0]
	if d.Kind != DiagnosticUnknownCharacterSet {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticUnknownCharacterSet)
	}
	if d.Tag != TagCharset {
		t.Errorf("Tag = %s, want %s", d.Tag, TagCharset)
	}
	if d.VR != VRCS {
		t.Errorf("VR = %s, want CS", d.VR)
	}
	if d.Offset <= 0 {
		t.Errorf("Offset = %d, want the (0008,0005) value position", d.Offset)
	}
	if d.Err == nil || !strings.Contains(d.Err.Error(), "ACME_PRIVATE") {
		t.Errorf("Err = %v, want the original unresolved value", d.Err)
	}
	// The parse is unchanged: the ISO_IR 6 fallback is still applied, so the
	// name is not silently "fixed" by the library either.
	if got := patientNameOf(t, fd).String(); got == "王^小东" {
		t.Error("the library must not guess GB18030 for an unknown character set")
	}
}

// A strict caller turns the diagnostic into a hard failure. This is what a PACS
// archiving endpoint needs: refuse the study and quarantine the original bytes.
func TestReadUnknownCharacterSetStrictHookFailsRead(t *testing.T) {
	t.Parallel()
	data := charsetBareFile(t, "ACME_PRIVATE", gb18030Bytes(t, "王^小东"))
	rec := &diagRecorder{reject: true}
	_, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err == nil {
		t.Fatal("a hook returning the diagnostic must fail the read")
	}
	var d Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("read error does not unwrap to a Diagnostic: %v", err)
	}
	if d.Kind != DiagnosticUnknownCharacterSet {
		t.Errorf("failed with %+v, want an unknown character set", d)
	}
	if !strings.Contains(err.Error(), "ACME_PRIVATE") {
		t.Errorf("read error %q does not name the offending value", err)
	}
}

// Without a hook the anomaly is still logged at warn level, matching the
// VR-mismatch diagnostic's behaviour.
func TestUnknownCharacterSetIsLoggedWithoutHook(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	data := charsetBareFile(t, "ACME_PRIVATE", gb18030Bytes(t, "王^小东"))
	if _, err := ReadBytes(data, &ReadOptions{Force: true, Logger: l}); err != nil {
		t.Fatalf("no hook must not fail the read: %v", err)
	}
	for _, want := range []string{"parse diagnostic", "kind=unknown_character_set", "vr=CS"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log is missing %q:\n%s", want, buf.String())
		}
	}
}

// A missing or empty (0008,0005) is legal PS3.5 and must never be reported.
func TestAbsentOrEmptyCharacterSetRaisesNoDiagnostic(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		cs   []byte
	}{
		{"absent", nil},
		{"empty", []byte{}},
		{"padding only", []byte{' ', 0x00}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var data []byte
			if tt.cs != nil {
				data = append(data, charsetBareElement(t, TagCharset, VRCS, tt.cs)...)
			}
			data = append(data, charsetBareElement(t, MustTag("PatientName"), VRPN, []byte("Doe^Jane"))...)
			rec := &diagRecorder{}
			if _, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook}); err != nil {
				t.Fatalf("read: %v", err)
			}
			if len(rec.got) != 0 {
				t.Fatalf("reported %d diagnostics on a legal default repertoire: %v", len(rec.got), rec.got)
			}
		})
	}
}

// Layer 2: an ISO_IR 192 value carrying invalid UTF-8 is no longer swallowed.
// The undecoded bytes stay in RawValue so a strict caller loses nothing.
func TestInvalidCharacterDataReportsDiagnostic(t *testing.T) {
	t.Parallel()
	raw := []byte{0xC4, 0xE3, 0xFF}
	data := charsetBareFile(t, "ISO_IR 192", raw)
	rec := &diagRecorder{}
	fd, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err != nil {
		t.Fatalf("a hook returning nil must not fail the read: %v", err)
	}
	if len(rec.got) != 1 || rec.got[0].Kind != DiagnosticInvalidCharacterData {
		t.Fatalf("diagnostics = %v, want one invalid_character_data", rec.got)
	}
	d := rec.got[0]
	if d.Tag != MustTag("PatientName") || d.VR != VRPN {
		t.Errorf("Tag/VR = %s/%s, want PatientName/PN", d.Tag, d.VR)
	}
	elem, ok := fd.Dataset.Get(MustTag("PatientName"))
	if !ok {
		t.Fatal("PatientName missing")
	}
	if !bytes.Equal(elem.RawValue[:len(raw)], raw) {
		t.Errorf("RawValue = % x, want the original undecoded bytes % x", elem.RawValue, raw)
	}

	// The same file is refused when the caller is strict.
	rec2 := &diagRecorder{reject: true}
	if _, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec2.hook}); err == nil {
		t.Fatal("strict hook must fail the read on invalid character data")
	}
}

// All three reader entry points must report the anomaly: ReadBytes (buffered),
// Read over a seekable source, and ReadFile.
func TestUnknownCharacterSetAcrossReadPaths(t *testing.T) {
	t.Parallel()
	data := charsetBareFile(t, "ACME_PRIVATE", gb18030Bytes(t, "王^小东"))
	path := filepath.Join(t.TempDir(), "unknown.dcm")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		read func(*diagRecorder) error
	}{
		{"ReadBytes", func(rec *diagRecorder) error {
			_, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
			return err
		}},
		{"Read/seekable", func(rec *diagRecorder) error {
			_, err := Read(bytes.NewReader(data), &ReadOptions{Force: true, OnDiagnostic: rec.hook})
			return err
		}},
		{"ReadFile", func(rec *diagRecorder) error {
			_, err := ReadFile(path, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
			return err
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := &diagRecorder{}
			if err := tt.read(rec); err != nil {
				t.Fatalf("read: %v", err)
			}
			if len(rec.got) != 1 || rec.got[0].Kind != DiagnosticUnknownCharacterSet {
				t.Fatalf("diagnostics = %v, want one unknown_character_set", rec.got)
			}
		})
	}
}

// A (0008,0005) inside a sequence item is reported with the item's Path, so a
// diagnostic says which item of many carries the bad declaration.
func TestUnknownCharacterSetInNestedSequence(t *testing.T) {
	t.Parallel()
	item := NewDataset()
	item.Set(NewDataElement(TagCharset, VRCS, "ACME_PRIVATE"))
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("ReferencedStudySequence"), VRSQ, NewSequence([]*Dataset{item})))
	data, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}

	rec := &diagRecorder{}
	if _, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook}); err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rec.got) != 1 || rec.got[0].Kind != DiagnosticUnknownCharacterSet {
		t.Fatalf("diagnostics = %v, want one unknown_character_set", rec.got)
	}
	if len(rec.got[0].Path) == 0 {
		t.Errorf("Path is empty; the sequence item is not identified: %+v", rec.got[0])
	}
}

// Reporting must not change the bytes. A read followed by an unmodified write
// still goes through the raw pass-through path, which is the safety net that
// keeps an undisplayable name recoverable from the original file.
func TestUnknownCharacterSetRawRoundtripIsByteIdentical(t *testing.T) {
	t.Parallel()
	data := charsetBareFile(t, "ACME_PRIVATE", gb18030Bytes(t, "王^小东"))
	fd, err := ReadBytes(data, &ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, data) {
		t.Errorf("unmodified write changed the bytes:\n got % x\nwant % x", out, data)
	}
}
