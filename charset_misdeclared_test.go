package godicom

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests cover #94: a file that declares ISO_IR 6 -- or declares nothing at
// all -- and ships GBK bytes anyway used to read as mojibake with no diagnostic,
// and turning that into a default diagnostic would reject files that have been
// archived unremarked for twenty years. So it is opt-in, once per dataset, and
// this file pins both halves: silent by default, reported when asked.
//
// Detection is not the repair; Dataset.ReinterpretCharacterSet is, and #99's
// tests cover it. What is asserted here is that asking does not change what the
// values hold.

// misdeclaredFile is a headerless dataset that declares nothing (cs empty) or a
// wrong character set, then carries the GB18030 bytes of 王^小东.
func misdeclaredFile(t *testing.T, cs string) []byte {
	t.Helper()
	return charsetBareFile(t, cs, gb18030Bytes(t, "王^小东"))
}

// mustEncodeString spells s in the given character set, for the cases where the
// bytes have to be right rather than merely present.
func mustEncodeString(t *testing.T, s, charset string) []byte {
	t.Helper()
	b, err := EncodeString(s, charset)
	if err != nil {
		t.Fatalf("encode %q as %s: %v", s, charset, err)
	}
	return b
}

// The default: not reported, so nothing existing changes. This is the behaviour
// the issue was filed about, kept deliberately.
func TestMisdeclaredCharsetIsSilentByDefault(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		cs   string
	}{
		{"ISO_IR 6", "ISO_IR 6"},
		{"absent", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := &diagRecorder{}
			fd, err := ReadBytes(misdeclaredFile(t, tt.cs), &ReadOptions{Force: true, OnDiagnostic: rec.hook})
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if len(rec.got) != 0 {
				t.Fatalf("diagnostics = %v, want none without the option", rec.got)
			}
			if got := patientNameOf(t, fd).String(); got == "王^小东" {
				t.Error("the library must not guess the encoding")
			}
		})
	}
}

// Opted in: one diagnostic per dataset, naming the first element that proves the
// declaration wrong.
func TestMisdeclaredCharsetIsReportedWhenAsked(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		cs   string
	}{
		{"ISO_IR 6", "ISO_IR 6"},
		{"absent", ""},
		{"ISO 2022 IR 6", "ISO 2022 IR 6"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := &diagRecorder{}
			fd, err := ReadBytes(misdeclaredFile(t, tt.cs), &ReadOptions{
				Force:                             true,
				ReportNonASCIIUnderDefaultCharset: true,
				OnDiagnostic:                      rec.hook,
			})
			if err != nil {
				t.Fatalf("a hook returning nil must not fail the read: %v", err)
			}
			if len(rec.got) != 1 {
				t.Fatalf("diagnostics = %v, want one", rec.got)
			}
			d := rec.got[0]
			if d.Kind != DiagnosticMisdeclaredCharacterSet {
				t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticMisdeclaredCharacterSet)
			}
			if d.Tag != MustTag("PatientName") || d.VR != VRPN {
				t.Errorf("Tag/VR = %s/%s, want PatientName/PN", d.Tag, d.VR)
			}
			if d.Offset <= 0 {
				t.Errorf("Offset = %d, want the value position", d.Offset)
			}
			// Six of the seven bytes of 王^小东 are above 0x7F; the caret is not.
			if d.Err == nil || !strings.Contains(d.Err.Error(), "6 byte(s) above 0x7F") {
				t.Errorf("Err = %v, want the high byte count", d.Err)
			}
			if d.Err != nil && !strings.Contains(d.Err.Error(), "ISO_IR 6") {
				t.Errorf("Err = %v, want it to name the declaration in effect", d.Err)
			}
			// Reporting must not touch the value: it is still the raw bytes, and
			// the caller decides what to do about them.
			if got := patientNameOf(t, fd).String(); got == "王^小东" {
				t.Error("reporting must not silently re-decode the value")
			}
		})
	}
}

// Correctly declared files are not affected, which is the difference between a
// useful diagnostic and one every Chinese study trips.
func TestMisdeclaredCharsetNotReportedWhenDeclarationIsRight(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		cs   string
		pn   []byte
	}{
		{"GB18030", "GB18030", gb18030Bytes(t, "王^小东")},
		{"ISO_IR 192", "ISO_IR 192", []byte("王^小东")},
		{"ISO_IR 13", "ISO_IR 13", mustEncodeString(t, "ﾜﾝ^ｼｬｵﾄﾝ", "ISO_IR 13")},
		{"ASCII only", "", []byte("Doe^Jane")},
		{"ASCII with ISO_IR 6", "ISO_IR 6", []byte("Doe^Jane")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := &diagRecorder{}
			if _, err := ReadBytes(charsetBareFile(t, tt.cs, tt.pn), &ReadOptions{
				Force:                             true,
				ReportNonASCIIUnderDefaultCharset: true,
				OnDiagnostic:                      rec.hook,
			}); err != nil {
				t.Fatalf("read: %v", err)
			}
			if len(rec.got) != 0 {
				t.Fatalf("diagnostics = %v, want none", rec.got)
			}
		})
	}
}

// A single byte set in front of GBK bytes is a different failure: the value
// decodes to valid but wrong text under the declared repertoire, so there is
// nothing for this diagnostic to see. It stays out of scope, and
// ReinterpretCharacterSet is what repairs it.
func TestSingleByteDeclarationIsNotMisdeclaration(t *testing.T) {
	t.Parallel()
	rec := &diagRecorder{}
	fd, err := ReadBytes(charsetBareFile(t, "ISO_IR 100", gb18030Bytes(t, "王^小东")), &ReadOptions{
		Force:                             true,
		ReportNonASCIIUnderDefaultCharset: true,
		OnDiagnostic:                      rec.hook,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rec.got) != 0 {
		t.Fatalf("diagnostics = %v, want none: ISO_IR 100 is a real repertoire", rec.got)
	}
	if got := patientNameOf(t, fd).String(); got != "Íõ^Ð¡¶«" {
		t.Logf("Latin-1 view of the name is %q", got)
	}
}

// One diagnostic per dataset, however many elements carry the bytes: the wrong
// declaration is one fact about the file. Without this the option would reintroduce
// the flood #95 removed.
func TestMisdeclaredCharsetIsReportedOncePerDataset(t *testing.T) {
	t.Parallel()
	var data []byte
	for i := 0; i < 30; i++ {
		data = append(data, charsetBareElement(t, Tag(uint32(0x00090010+i)), VRLO, gb18030Bytes(t, "王"))...)
	}
	rec := &diagRecorder{}
	if _, err := ReadBytes(data, &ReadOptions{
		Force:                             true,
		ReportNonASCIIUnderDefaultCharset: true,
		OnDiagnostic:                      rec.hook,
	}); err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("diagnostics = %d, want one: %v", len(rec.got), rec.got)
	}
	// The first offending element, not the last.
	if rec.got[0].Tag != Tag(0x00090010) {
		t.Errorf("Tag = %s, want the first offending element (0009,0010)", rec.got[0].Tag)
	}
}

// The option is meant for exactly the archiving endpoint that fails closed, so
// the hook must be able to refuse the file.
func TestMisdeclaredCharsetStrictHookFailsRead(t *testing.T) {
	t.Parallel()
	rec := &diagRecorder{reject: true}
	_, err := ReadBytes(misdeclaredFile(t, "ISO_IR 6"), &ReadOptions{
		Force:                             true,
		ReportNonASCIIUnderDefaultCharset: true,
		OnDiagnostic:                      rec.hook,
	})
	if err == nil {
		t.Fatal("a rejecting hook must fail the read")
	}
	var d Diagnostic
	if !errors.As(err, &d) || d.Kind != DiagnosticMisdeclaredCharacterSet {
		t.Fatalf("read error = %v, want a misdeclared_character_set diagnostic", err)
	}
}

// Without a hook the anomaly is still logged, matching every other diagnostic.
func TestMisdeclaredCharsetIsLoggedWithoutHook(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if _, err := ReadBytes(misdeclaredFile(t, "ISO_IR 6"), &ReadOptions{
		Force:                             true,
		Logger:                            l,
		ReportNonASCIIUnderDefaultCharset: true,
	}); err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"parse diagnostic", "kind=misdeclared_character_set", "tag=(0010,0010)"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log is missing %q:\n%s", want, buf.String())
		}
	}
}

// All three reader entry points, because the streaming reader decides where a
// value's bytes are decoded.
func TestMisdeclaredCharsetAcrossReadPaths(t *testing.T) {
	t.Parallel()
	data := misdeclaredFile(t, "ISO_IR 6")
	path := filepath.Join(t.TempDir(), "misdeclared.dcm")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		read func(*diagRecorder) error
	}{
		{"ReadBytes", func(rec *diagRecorder) error {
			_, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook, ReportNonASCIIUnderDefaultCharset: true})
			return err
		}},
		{"Read/seekable", func(rec *diagRecorder) error {
			_, err := Read(bytes.NewReader(data), &ReadOptions{Force: true, OnDiagnostic: rec.hook, ReportNonASCIIUnderDefaultCharset: true})
			return err
		}},
		{"ReadFile", func(rec *diagRecorder) error {
			_, err := ReadFile(path, &ReadOptions{Force: true, OnDiagnostic: rec.hook, ReportNonASCIIUnderDefaultCharset: true})
			return err
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := &diagRecorder{}
			if err := tt.read(rec); err != nil {
				t.Fatalf("read: %v", err)
			}
			if len(rec.got) != 1 || rec.got[0].Kind != DiagnosticMisdeclaredCharacterSet {
				t.Fatalf("diagnostics = %v, want one misdeclared_character_set", rec.got)
			}
		})
	}
}

// Reporting and repairing are separate: with the option on, the bytes on disk
// still round-trip unchanged, and the documented repair still produces the name.
func TestMisdeclaredCharsetReportDoesNotChangeBytes(t *testing.T) {
	t.Parallel()
	data := misdeclaredFile(t, "ISO_IR 6")
	withOption, err := ReadBytes(data, &ReadOptions{
		Force:                             true,
		ReportNonASCIIUnderDefaultCharset: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := EncodeDataset(withOption.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !bytes.Equal(out, data) {
		t.Errorf("an unmodified write changed the bytes:\n got % x\nwant % x", out, data)
	}

	rec := &diagRecorder{}
	fd, err := ReadBytes(data, &ReadOptions{
		Force:                             true,
		ReportNonASCIIUnderDefaultCharset: true,
		OnDiagnostic:                      rec.hook,
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := fd.ReinterpretCharacterSet("GB18030")
	if err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	if n != 1 {
		t.Errorf("reinterpreted %d values, want 1", n)
	}
	if got := patientNameOf(t, fd).String(); got != "王^小东" {
		t.Errorf("PatientName = %q after the repair, want 王^小东", got)
	}
}
