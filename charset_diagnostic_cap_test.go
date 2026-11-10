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

// These tests cover #95: invalid_character_data used to be reported once per
// text element, so a study with a few hundred undecodable values produced a few
// hundred diagnostics and a few hundred identical warn lines. The reader now
// reports the first handful and collapses the rest into one summary carrying
// the count.
//
// The bytes are built by hand rather than through EncodeDataset, for the same
// reason as charset_diagnostic_test.go: the writer must not be able to hide a
// reader-side regression.

// charsetBadElements returns n explicit VR LO elements, each carrying bytes
// GB18030 cannot decode, under distinct private tags so an assertion can say
// which element a diagnostic came from.
func charsetBadElements(t *testing.T, n int) []byte {
	t.Helper()
	var b []byte
	for i := 0; i < n; i++ {
		tag := Tag(uint32(0x00090010 + i))
		b = append(b, charsetBareElement(t, tag, VRLO, []byte{0xFF, 0xFE})...)
	}
	return b
}

// charsetFileWithBadElements is a headerless dataset declaring GB18030 and then
// carrying n undecodable LO values.
func charsetFileWithBadElements(t *testing.T, n int) []byte {
	t.Helper()
	b := charsetBareElement(t, TagCharset, VRCS, []byte("GB18030"))
	return append(b, charsetBadElements(t, n)...)
}

// charsetUndefinedLengthSequence wraps payload in an explicit VR SQ with
// undefined length and a single undefined-length item, which is how a real file
// most often spells it. Hand-built because the writer refuses to encode a value
// it cannot represent, which is the whole point of the payload.
func charsetUndefinedLengthSequence(seqTag Tag, payload []byte) []byte {
	var b []byte
	b = binary.LittleEndian.AppendUint16(b, uint16(seqTag.Group()))
	b = binary.LittleEndian.AppendUint16(b, uint16(seqTag.Element()))
	b = append(b, 'S', 'Q', 0x00, 0x00)
	b = binary.LittleEndian.AppendUint32(b, 0xFFFFFFFF)
	b = binary.LittleEndian.AppendUint16(b, uint16(ItemTag.Group()))
	b = binary.LittleEndian.AppendUint16(b, uint16(ItemTag.Element()))
	b = binary.LittleEndian.AppendUint32(b, 0xFFFFFFFF)
	b = append(b, payload...)
	b = binary.LittleEndian.AppendUint16(b, uint16(ItemDelimiterTag.Group()))
	b = binary.LittleEndian.AppendUint16(b, uint16(ItemDelimiterTag.Element()))
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = binary.LittleEndian.AppendUint16(b, uint16(SequenceDelimiterTag.Group()))
	b = binary.LittleEndian.AppendUint16(b, uint16(SequenceDelimiterTag.Element()))
	b = binary.LittleEndian.AppendUint32(b, 0)
	return b
}

// charsetDefinedLengthSequence wraps payload in an explicit VR SQ whose length is
// declared, which is the form the streaming reader parses out of a copied buffer
// and therefore the one where a diagnostic offset needs shifting back.
func charsetDefinedLengthSequence(seqTag Tag, payload []byte) []byte {
	var b []byte
	b = binary.LittleEndian.AppendUint16(b, uint16(seqTag.Group()))
	b = binary.LittleEndian.AppendUint16(b, uint16(seqTag.Element()))
	b = append(b, 'S', 'Q', 0x00, 0x00)
	b = binary.LittleEndian.AppendUint32(b, uint32(8+len(payload)))
	b = binary.LittleEndian.AppendUint16(b, uint16(ItemTag.Group()))
	b = binary.LittleEndian.AppendUint16(b, uint16(ItemTag.Element()))
	b = binary.LittleEndian.AppendUint32(b, uint32(len(payload)))
	return append(b, payload...)
}

// countKind returns how many of got carry kind.
func countKind(got []Diagnostic, kind DiagnosticKind) int {
	n := 0
	for _, d := range got {
		if d.Kind == kind {
			n++
		}
	}
	return n
}

// summary returns the one diagnostic that stands for several elements, or nil.
func summaryOf(got []Diagnostic) *Diagnostic {
	for i := range got {
		if got[i].Kind == DiagnosticInvalidCharacterData && got[i].Count > 0 {
			return &got[i]
		}
	}
	return nil
}

// The regression: 300 bad elements used to be 300 diagnostics. Now the caller
// gets the default cap of eight and one summary that says how many more there
// were, so a report stays readable and a hook does not allocate 300 structs it
// will discard.
func TestInvalidCharacterDataIsCappedWithSummary(t *testing.T) {
	t.Parallel()
	const bad = 300
	rec := &diagRecorder{}
	fd, err := ReadBytes(charsetFileWithBadElements(t, bad), &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err != nil {
		t.Fatalf("a hook returning nil must not fail the read: %v", err)
	}
	if len(rec.got) != DefaultMaxInvalidCharacterDataDiagnostics+1 {
		t.Fatalf("diagnostics = %d, want %d individual plus one summary",
			len(rec.got), DefaultMaxInvalidCharacterDataDiagnostics)
	}
	if got := countKind(rec.got, DiagnosticInvalidCharacterData); got != len(rec.got) {
		t.Errorf("%d of %d diagnostics are not invalid_character_data", len(rec.got)-got, len(rec.got))
	}
	s := summaryOf(rec.got)
	if s == nil {
		t.Fatal("no summary diagnostic; the cap swallowed the rest silently")
	}
	if want := bad - DefaultMaxInvalidCharacterDataDiagnostics; s.Count != want {
		t.Errorf("summary Count = %d, want %d", s.Count, want)
	}
	// The summary points at the first element it stands for, not at the first
	// element of the file, so a caller can go straight to it.
	wantTag := Tag(uint32(0x00090010 + DefaultMaxInvalidCharacterDataDiagnostics))
	if s.Tag != wantTag {
		t.Errorf("summary Tag = %s, want %s (the first element it covers)", s.Tag, wantTag)
	}
	if s.Err == nil || !strings.Contains(s.Err.Error(), "MaxInvalidCharacterDataDiagnostics") {
		t.Errorf("summary Err = %v, want it to name the option that lifts the cap", s.Err)
	}
	// The data is untouched: every element is still there, undecoded bytes and
	// all. The cap is about reporting, not about parsing.
	if fd.Dataset.Len() < DefaultMaxInvalidCharacterDataDiagnostics {
		t.Errorf("dataset has %d elements, want at least %d", fd.Dataset.Len(), bad)
	}
	if got := fd.Dataset.Len(); got != 1+bad { // (0008,0005) plus the bad ones
		t.Errorf("dataset has %d elements, want %d", got, 1+bad)
	}
}

// The boundary either side of the cap, because an off-by-one here is invisible
// in the 300-element case: a dataset that produces exactly the cap must not grow
// a summary, and one more must.
func TestInvalidCharacterDataCapBoundary(t *testing.T) {
	t.Parallel()
	cap := DefaultMaxInvalidCharacterDataDiagnostics
	for _, tt := range []struct {
		name      string
		bad       int
		wantCount int
		wantSum   int
	}{
		{"one", 1, 1, 0},
		{"just under the cap", cap - 1, cap - 1, 0},
		{"exactly the cap", cap, cap, 0},
		{"one over the cap", cap + 1, cap, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := &diagRecorder{}
			if _, err := ReadBytes(charsetFileWithBadElements(t, tt.bad), &ReadOptions{Force: true, OnDiagnostic: rec.hook}); err != nil {
				t.Fatalf("read: %v", err)
			}
			if got := len(rec.got); got != tt.wantCount+tt.wantSum {
				t.Fatalf("diagnostics = %d, want %d individual plus %d summary", got, tt.wantCount, tt.wantSum)
			}
			s := summaryOf(rec.got)
			if tt.wantSum == 0 {
				if s != nil {
					t.Fatalf("summary raised for %d elements with a cap of %d: %+v", tt.bad, cap, *s)
				}
				return
			}
			if s == nil {
				t.Fatal("no summary")
			}
			if s.Count != 1 {
				t.Errorf("summary Count = %d, want 1", s.Count)
			}
		})
	}
}

// A caller who really wants every element says so with a negative cap, which is
// the escape hatch the summary message names.
func TestInvalidCharacterDataCapCanBeLifted(t *testing.T) {
	t.Parallel()
	const bad = 40
	rec := &diagRecorder{}
	_, err := ReadBytes(charsetFileWithBadElements(t, bad), &ReadOptions{
		Force:                              true,
		MaxInvalidCharacterDataDiagnostics: -1,
		OnDiagnostic:                       rec.hook,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rec.got) != bad {
		t.Fatalf("diagnostics = %d, want %d", len(rec.got), bad)
	}
	if s := summaryOf(rec.got); s != nil {
		t.Errorf("summary raised with the cap lifted: %+v", *s)
	}
	// Every element must be named, so a per-element report can be built from the
	// diagnostics alone.
	seen := make(map[Tag]bool, bad)
	for _, d := range rec.got {
		if d.Kind != DiagnosticInvalidCharacterData {
			t.Fatalf("unexpected kind %q", d.Kind)
		}
		seen[d.Tag] = true
		if d.Err == nil {
			t.Errorf("%s: no cause attached", d.Tag)
		}
	}
	if len(seen) != bad {
		t.Errorf("%d distinct tags reported, want %d", len(seen), bad)
	}
}

// A positive cap other than the default is honoured, so the number is a knob and
// not a constant with a name.
func TestInvalidCharacterDataCapIsConfigurable(t *testing.T) {
	t.Parallel()
	rec := &diagRecorder{}
	_, err := ReadBytes(charsetFileWithBadElements(t, 50), &ReadOptions{
		Force:                              true,
		MaxInvalidCharacterDataDiagnostics: 2,
		OnDiagnostic:                       rec.hook,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rec.got) != 3 {
		t.Fatalf("diagnostics = %d, want 2 individual plus one summary: %v", len(rec.got), rec.got)
	}
	if s := summaryOf(rec.got); s == nil || s.Count != 48 {
		t.Fatalf("summary = %+v, want Count 48", s)
	}
}

// The log is the other half of #95: a hook can be told to ignore diagnostics,
// but the default warn line was the flood. One line per reported element plus
// one for the summary, not 300.
func TestInvalidCharacterDataLogIsCapped(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if _, err := ReadBytes(charsetFileWithBadElements(t, 300), &ReadOptions{Force: true, Logger: l}); err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Count(buf.String(), "parse diagnostic")
	if want := DefaultMaxInvalidCharacterDataDiagnostics + 1; lines != want {
		t.Fatalf("logged %d parse diagnostics, want %d:\n%s", lines, want, buf.String())
	}
	if !strings.Contains(buf.String(), "count=292") {
		t.Errorf("summary log line does not carry the count:\n%s", buf.String())
	}
}

// The summary has to name the element it stands for even when that element is
// inside a sequence item, which is where an archive's worst data tends to live.
func TestInvalidCharacterDataSummaryKeepsSequencePath(t *testing.T) {
	t.Parallel()
	body := charsetUndefinedLengthSequence(MustTag("ReferencedStudySequence"), charsetBadElements(t, 20))
	data := append(charsetBareElement(t, TagCharset, VRCS, []byte("GB18030")), body...)

	rec := &diagRecorder{}
	if _, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook}); err != nil {
		t.Fatalf("read: %v", err)
	}
	s := summaryOf(rec.got)
	if s == nil {
		t.Fatalf("no summary: %v", rec.got)
	}
	if len(s.Path) != 1 {
		t.Fatalf("summary Path = %v, want one step naming the item", s.Path)
	}
	if s.Path[0].Tag != MustTag("ReferencedStudySequence") || s.Path[0].Item != 0 {
		t.Errorf("summary Path[0] = %+v, want (0040,0100)[0]", s.Path[0])
	}
	if s.Offset <= 0 {
		t.Errorf("summary Offset = %d, want the element's value position", s.Offset)
	}
	// The individual ones carry the same path, so the two are comparable.
	for _, d := range rec.got {
		if d.Count == 0 && len(d.Path) != 1 {
			t.Errorf("%s reported without a path", d.Tag)
		}
	}
}

// A defined-length sequence is parsed out of a copied buffer by the streaming
// reader, so its diagnostics have to be shifted back into source coordinates.
// Comparing the two readers is the cheapest way to say that without arithmetic:
// same file, same diagnostics, same offsets.
func TestInvalidCharacterDataOffsetsAgreeAcrossReaders(t *testing.T) {
	t.Parallel()
	body := charsetDefinedLengthSequence(MustTag("ReferencedStudySequence"), charsetBadElements(t, 20))
	data := append(charsetBareElement(t, TagCharset, VRCS, []byte("GB18030")), body...)

	eager := &diagRecorder{}
	if _, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: eager.hook}); err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	stream := &diagRecorder{}
	if _, err := Read(bytes.NewReader(data), &ReadOptions{Force: true, OnDiagnostic: stream.hook}); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(eager.got) != len(stream.got) {
		t.Fatalf("ReadBytes reported %d diagnostics, Read reported %d", len(eager.got), len(stream.got))
	}
	for i := range eager.got {
		a, b := eager.got[i], stream.got[i]
		if a.Tag != b.Tag || a.Offset != b.Offset || a.Count != b.Count {
			t.Errorf("diagnostic %d differs: ReadBytes %s@%d count=%d, Read %s@%d count=%d",
				i, a.Tag, a.Offset, a.Count, b.Tag, b.Offset, b.Count)
		}
	}
	// Both must be past the sequence header (16 bytes of charset element plus 12
	// of sequence header and 8 of item header), which rules out an offset that
	// was left relative to the copied chunk.
	if s := summaryOf(stream.got); s == nil || s.Offset <= 36 {
		t.Errorf("streaming summary = %+v, want an offset inside the sequence", s)
	}
}

// A strict hook still fails fast: the cap only applies to what is reported after
// the hook has been handed the first one.
func TestInvalidCharacterDataStrictHookStillFails(t *testing.T) {
	t.Parallel()
	rec := &diagRecorder{reject: true}
	_, err := ReadBytes(charsetFileWithBadElements(t, 300), &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err == nil {
		t.Fatal("a hook that rejects must fail the read")
	}
	if len(rec.got) != 1 {
		t.Errorf("hook saw %d diagnostics before failing, want 1", len(rec.got))
	}
	var d Diagnostic
	if !errors.As(err, &d) || d.Kind != DiagnosticInvalidCharacterData {
		t.Fatalf("read error = %v, want an invalid_character_data diagnostic", err)
	}
}

// The summary is reported at the end of the parse, so a hook that rejects it
// fails the read rather than the next one -- and the caller learns the count.
func TestInvalidCharacterDataSummaryRejectsTheRead(t *testing.T) {
	t.Parallel()
	rec := &diagRecorder{}
	rejecting := func(d Diagnostic) error {
		rec.got = append(rec.got, d)
		if d.Count > 0 {
			return d
		}
		return nil
	}
	_, err := ReadBytes(charsetFileWithBadElements(t, 100), &ReadOptions{Force: true, OnDiagnostic: rejecting})
	if err == nil {
		t.Fatal("rejecting the summary must fail the read")
	}
	if s := summaryOf(rec.got); s == nil {
		t.Fatalf("no summary was offered: %v", rec.got)
	}
	var d Diagnostic
	if !errors.As(err, &d) || d.Count != 100-DefaultMaxInvalidCharacterDataDiagnostics {
		t.Fatalf("read error = %+v, want the summary diagnostic", err)
	}
}

// All three reader entry points must cap, because the streaming reader has its
// own element loop and its own place to flush.
func TestInvalidCharacterDataCapAcrossReadPaths(t *testing.T) {
	t.Parallel()
	data := charsetFileWithBadElements(t, 60)
	path := filepath.Join(t.TempDir(), "flood.dcm")
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
			if len(rec.got) != DefaultMaxInvalidCharacterDataDiagnostics+1 {
				t.Fatalf("diagnostics = %d, want %d: %v", len(rec.got), DefaultMaxInvalidCharacterDataDiagnostics+1, rec.got)
			}
			s := summaryOf(rec.got)
			if s == nil {
				t.Fatal("no summary")
			}
			if want := 60 - DefaultMaxInvalidCharacterDataDiagnostics; s.Count != want {
				t.Errorf("summary Count = %d, want %d", s.Count, want)
			}
		})
	}
}

// A file whose text all decodes must not gain a summary from the counting that
// implements the cap.
func TestNoSummaryWithoutSuppression(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"valid GB18030", charsetBareFile(t, "GB18030", gb18030Bytes(t, "王^小东"))},
		{"ASCII", charsetBareFile(t, "", []byte("Doe^Jane"))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := &diagRecorder{}
			if _, err := ReadBytes(tt.data, &ReadOptions{Force: true, OnDiagnostic: rec.hook}); err != nil {
				t.Fatalf("read: %v", err)
			}
			if len(rec.got) != 0 {
				t.Fatalf("diagnostics = %v, want none", rec.got)
			}
		})
	}
}

// Diagnostic.Count has to be readable both ways: in Error and in the log, since
// an operator reads the log and a caller reads the struct.
func TestDiagnosticSummaryRendersCount(t *testing.T) {
	t.Parallel()
	d := Diagnostic{
		Kind:   DiagnosticInvalidCharacterData,
		Tag:    MustTag("PatientName"),
		VR:     VRPN,
		Offset: 200,
		Count:  292,
		Err:    errInvalidCharacterDataSuppressed,
	}
	msg := d.Error()
	for _, want := range []string{"invalid_character_data", "(0010,0010)", "292 elements"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, missing %q", msg, want)
		}
	}
	// Count is zero on every per-element diagnostic and must print nothing then.
	plain := Diagnostic{Kind: DiagnosticInvalidCharacterData, Tag: MustTag("PatientName")}
	if strings.Contains(plain.Error(), "elements") {
		t.Errorf("Error() = %q, want no count for a single element", plain.Error())
	}
}
