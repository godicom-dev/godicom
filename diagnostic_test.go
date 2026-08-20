package godicom

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// diagRecorder collects the diagnostics a read reports and, when reject is set,
// fails the read on the first one.
type diagRecorder struct {
	got    []Diagnostic
	reject bool
}

func (r *diagRecorder) hook(d Diagnostic) error {
	r.got = append(r.got, d)
	if r.reject {
		return d
	}
	return nil
}

func (r *diagRecorder) first(t *testing.T) Diagnostic {
	t.Helper()
	if len(r.got) == 0 {
		t.Fatal("no diagnostic reported")
	}
	return r.got[0]
}

// encodeBare returns ds as a headerless explicit VR little endian dataset, so a
// test can truncate it at a byte offset it computed itself. ReadOptions.Force
// reads it back without a preamble or File Meta.
func encodeBare(t *testing.T, ds *Dataset) []byte {
	t.Helper()
	encoded, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func bareDataset(t *testing.T) *Dataset {
	t.Helper()
	ds := NewDataset()
	if err := ds.SetString(MustTag("PatientID"), "ABCD1234"); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetString(MustTag("PatientName"), "Doe^Jane"); err != nil {
		t.Fatal(err)
	}
	return ds
}

// A value shorter than its length field is the common shape of a truncated
// transfer. The default is still to keep what was parsed, but the anomaly is no
// longer invisible.
func TestReadBytes_TruncatedValueReportsDiagnostic(t *testing.T) {
	t.Parallel()
	full := encodeBare(t, bareDataset(t))
	// Elements are written in tag order, so PatientName (0010,0010) comes first
	// and the cut lands in PatientID's 8-byte value.
	cut := len(full) - 3
	rec := &diagRecorder{}

	fd, err := ReadBytes(full[:cut], &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err != nil {
		t.Fatalf("tolerant read must succeed: %v", err)
	}
	if name, ok := fd.GetString(MustTag("PatientName")); !ok || name != "Doe^Jane" {
		t.Errorf("PatientName = %q, %v; elements before the truncation must survive", name, ok)
	}
	if fd.Has(MustTag("PatientID")) {
		t.Error("the truncated element must not be stored")
	}

	d := rec.first(t)
	if d.Kind != DiagnosticTruncatedValue {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticTruncatedValue)
	}
	if d.Tag != MustTag("PatientID") {
		t.Errorf("Tag = %s, want PatientID", d.Tag)
	}
	if d.VR != VRLO {
		t.Errorf("VR = %s, want LO", d.VR)
	}
	if d.Offset != int64(len(full)-8) {
		t.Errorf("Offset = %d, want %d (value start)", d.Offset, len(full)-8)
	}
	if d.Need != 8 || d.Have != 5 {
		t.Errorf("Need/Have = %d/%d, want 8/5", d.Need, d.Have)
	}
	if len(d.Path) != 0 {
		t.Errorf("Path = %v, want empty for a top-level element", d.Path)
	}
}

// Returning the Diagnostic from the hook is the whole strict mode: no policy
// enum, and the caller gets the anomaly itself as the error.
func TestReadBytes_HookErrorFailsTheRead(t *testing.T) {
	t.Parallel()
	full := encodeBare(t, bareDataset(t))
	rec := &diagRecorder{reject: true}

	fd, err := ReadBytes(full[:len(full)-3], &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err == nil {
		t.Fatal("strict read must fail")
	}
	if fd != nil {
		t.Error("a failed read must not return a dataset")
	}
	var d Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("error %v does not carry a Diagnostic", err)
	}
	if d.Kind != DiagnosticTruncatedValue {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticTruncatedValue)
	}
	if d.Tag != MustTag("PatientID") {
		t.Errorf("Tag = %s, want PatientID", d.Tag)
	}
}

func TestReadBytes_TruncatedHeaderReportsDiagnostic(t *testing.T) {
	t.Parallel()
	full := encodeBare(t, bareDataset(t))
	// Keep PatientName whole, then only 6 of PatientID's 8 header bytes.
	cut := len(full) - 8 - 2
	rec := &diagRecorder{}

	fd, err := ReadBytes(full[:cut], &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err != nil {
		t.Fatalf("tolerant read must succeed: %v", err)
	}
	if !fd.Has(MustTag("PatientName")) {
		t.Error("PatientName must survive")
	}

	d := rec.first(t)
	if d.Kind != DiagnosticTruncatedHeader {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticTruncatedHeader)
	}
	if d.Tag != MustTag("PatientID") {
		t.Errorf("Tag = %s, want PatientID", d.Tag)
	}
	if d.Need != 8 || d.Have != 6 {
		t.Errorf("Need/Have = %d/%d, want 8/6", d.Need, d.Have)
	}
}

// An anomaly inside a sequence has to name the sequence it came from: the
// element tag alone does not say where in the dataset to look.
func TestReadBytes_DiagnosticCarriesSequencePath(t *testing.T) {
	t.Parallel()
	item := NewDataset()
	if err := item.SetString(MustTag("ReferencedSOPInstanceUID"), "1.2.840.10008.1.2.1"); err != nil {
		t.Fatal(err)
	}
	ds := NewDataset()
	if err := ds.SetSequence(MustTag("ReferencedImageSequence"), NewSequence([]*Dataset{item})); err != nil {
		t.Fatal(err)
	}
	full := encodeBare(t, ds)
	rec := &diagRecorder{}

	if _, err := ReadBytes(full[:len(full)-4], &ReadOptions{Force: true, OnDiagnostic: rec.hook}); err != nil {
		t.Fatalf("tolerant read must succeed: %v", err)
	}

	d := rec.first(t)
	if len(d.Path) != 1 || d.Path[0] != MustTag("ReferencedImageSequence") {
		t.Fatalf("Path = %v, want [ReferencedImageSequence]", d.Path)
	}
	if d.Tag != MustTag("ReferencedSOPInstanceUID") {
		t.Errorf("Tag = %s, want ReferencedSOPInstanceUID", d.Tag)
	}
	if d.Kind != DiagnosticTruncatedValue {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticTruncatedValue)
	}
}

// A hook error raised inside a sequence has to unwind through the sequence
// readers, which return no error of their own.
func TestReadBytes_HookErrorInsideSequenceFailsTheRead(t *testing.T) {
	t.Parallel()
	item := NewDataset()
	if err := item.SetString(MustTag("ReferencedSOPInstanceUID"), "1.2.840.10008.1.2.1"); err != nil {
		t.Fatal(err)
	}
	ds := NewDataset()
	if err := ds.SetSequence(MustTag("ReferencedImageSequence"), NewSequence([]*Dataset{item})); err != nil {
		t.Fatal(err)
	}
	full := encodeBare(t, ds)
	rec := &diagRecorder{reject: true}

	_, err := ReadBytes(full[:len(full)-4], &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err == nil {
		t.Fatal("strict read must fail")
	}
	var d Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("error %v does not carry a Diagnostic", err)
	}
	if len(d.Path) != 1 || d.Path[0] != MustTag("ReferencedImageSequence") {
		t.Errorf("Path = %v, want [ReferencedImageSequence]", d.Path)
	}
	if len(rec.got) != 1 {
		t.Errorf("reported %d diagnostics, want 1: the read must stop at the first", len(rec.got))
	}
}

// truncatedItemBytes returns an explicit VR little endian buffer holding a
// defined-length sequence whose declared length overruns the buffer, cut so the
// last thing present is an Item tag with no room for its length. That is the one
// anomaly raised by the item loop itself rather than by the element reader.
func truncatedItemBytes() []byte {
	return []byte{
		// (0008,1140) SQ, reserved, length 0x20 -- 32 bytes that are not there
		0x08, 0x00, 0x40, 0x11, 'S', 'Q', 0x00, 0x00, 0x20, 0x00, 0x00, 0x00,
		// (FFFE,E000) Item, and then the file ends
		0xFE, 0xFF, 0x00, 0xE0,
	}
}

// The item loop reports the anomaly and, by default, keeps the empty sequence.
func TestReadBytes_TruncatedItemReportsDiagnostic(t *testing.T) {
	t.Parallel()
	rec := &diagRecorder{}

	fd, err := ReadBytes(truncatedItemBytes(), &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err != nil {
		t.Fatalf("tolerant read must succeed: %v", err)
	}

	d := rec.first(t)
	if d.Kind != DiagnosticTruncatedItem {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticTruncatedItem)
	}
	if d.Tag != ItemTag {
		t.Errorf("Tag = %s, want %s", d.Tag, ItemTag)
	}
	if d.Offset != 12 {
		t.Errorf("Offset = %d, want 12", d.Offset)
	}
	if d.Need != 8 || d.Have != 4 {
		t.Errorf("Need/Have = %d/%d, want 8/4", d.Need, d.Have)
	}
	if !fd.Has(MustTag("ReferencedImageSequence")) {
		t.Error("the sequence element itself must still be kept")
	}
}

// The same anomaly, raised two frames below the element loop, has to reach the
// caller as an error now that the sequence readers return one.
func TestReadBytes_TruncatedItemHookErrorFailsTheRead(t *testing.T) {
	t.Parallel()
	rec := &diagRecorder{reject: true}

	_, err := ReadBytes(truncatedItemBytes(), &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err == nil {
		t.Fatal("strict read must fail")
	}
	var d Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("error %v does not carry a Diagnostic", err)
	}
	if d.Kind != DiagnosticTruncatedItem {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticTruncatedItem)
	}
	if len(d.Path) != 1 || d.Path[0] != MustTag("ReferencedImageSequence") {
		t.Errorf("Path = %v, want [ReferencedImageSequence]", d.Path)
	}
}

// ReadFile takes the streaming parser, a separate code path from ReadBytes;
// both have to report the same anomaly.
func TestReadFile_TruncatedValueReportsDiagnostic(t *testing.T) {
	t.Parallel()
	full := encodeBare(t, bareDataset(t))
	path := filepath.Join(t.TempDir(), "truncated.dcm")
	if err := os.WriteFile(path, full[:len(full)-3], 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &diagRecorder{}

	fd, err := ReadFile(path, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
	if err != nil {
		t.Fatalf("tolerant read must succeed: %v", err)
	}
	if !fd.Has(MustTag("PatientName")) {
		t.Error("PatientName must survive")
	}

	d := rec.first(t)
	if d.Kind != DiagnosticTruncatedValue {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticTruncatedValue)
	}
	if d.Tag != MustTag("PatientID") {
		t.Errorf("Tag = %s, want PatientID", d.Tag)
	}
	if d.Offset != int64(len(full)-8) {
		t.Errorf("Offset = %d, want %d", d.Offset, len(full)-8)
	}

	rec2 := &diagRecorder{reject: true}
	if _, err := ReadFile(path, &ReadOptions{Force: true, OnDiagnostic: rec2.hook}); err == nil {
		t.Fatal("strict read must fail")
	}
}

// Dataset.Get turns a failed deferred load into a plain "absent", so the tag
// stays listed while every read of it returns nothing. The hook is what makes
// that visible.
func TestDeferredValueUnreadable_ReportsDiagnostic(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(testFilePath("CT_small.dcm"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "deferred.dcm")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &diagRecorder{}
	fd, err := ReadFile(path, &ReadOptions{DeferSize: 1024, OnDiagnostic: rec.hook})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.got) != 0 {
		t.Fatalf("clean read reported %v", rec.got)
	}
	// The value is only read when it is asked for, and by then the file is gone.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if _, ok := fd.Get(MustTag("PixelData")); ok {
		t.Fatal("PixelData must not load once the source is gone")
	}
	if !fd.Has(MustTag("PixelData")) {
		t.Error("the tag stays listed even though Get reports it absent")
	}

	d := rec.first(t)
	if d.Kind != DiagnosticDeferredValueUnreadable {
		t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticDeferredValueUnreadable)
	}
	if d.Tag != MustTag("PixelData") {
		t.Errorf("Tag = %s, want PixelData", d.Tag)
	}
	if d.Err == nil {
		t.Error("Err must carry the underlying cause")
	}
	// Diagnostic unwraps, so a caller can ask why the load failed.
	if !errors.Is(d, os.ErrNotExist) {
		t.Errorf("Diagnostic does not unwrap to os.ErrNotExist: %v", d)
	}
}

func TestDeferredValueUnreadable_HookErrorSurfacesFromLoadDeferred(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(testFilePath("CT_small.dcm"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "deferred.dcm")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("rejected")
	fd, err := ReadFile(path, &ReadOptions{
		DeferSize:    1024,
		OnDiagnostic: func(Diagnostic) error { return sentinel },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := fd.LoadDeferred(MustTag("PixelData")); !errors.Is(err, sentinel) {
		t.Fatalf("LoadDeferred error = %v, want %v", err, sentinel)
	}
}

// No hook is the pre-existing behaviour: truncation is tolerated silently.
func TestReadBytes_NoHookKeepsTolerantBehaviour(t *testing.T) {
	t.Parallel()
	full := encodeBare(t, bareDataset(t))
	fd, err := ReadBytes(full[:len(full)-3], &ReadOptions{Force: true})
	if err != nil {
		t.Fatalf("read must succeed: %v", err)
	}
	if !fd.Has(MustTag("PatientName")) {
		t.Error("PatientName must survive")
	}
	if fd.Has(MustTag("PatientID")) {
		t.Error("the truncated element must not be stored")
	}
}

// The streaming reader copies a defined-length sequence out to a chunk and
// parses that, so its offsets have to be shifted back into file coordinates.
// Both readers must name the same byte.
func TestTruncationInsideSequence_OffsetIsFileRelative(t *testing.T) {
	t.Parallel()
	// Explicit VR little endian, hand-built so the offsets are exact:
	//   0: (0008,1140) SQ, 32-bit length 18
	//  12: item header, length 10
	//  20: (0010,0020) LO, length 8 -- but only 2 value bytes fit in the item
	//  28: "AB"
	data := []byte{
		0x08, 0x00, 0x40, 0x11, 'S', 'Q', 0x00, 0x00, 18, 0x00, 0x00, 0x00,
		0xFE, 0xFF, 0x00, 0xE0, 10, 0x00, 0x00, 0x00,
		0x10, 0x00, 0x20, 0x00, 'L', 'O', 8, 0x00,
		'A', 'B',
	}
	const wantOffset = 28

	path := filepath.Join(t.TempDir(), "seq.dcm")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	readers := map[string]func(*diagRecorder) error{
		"ReadBytes": func(rec *diagRecorder) error {
			_, err := ReadBytes(data, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
			return err
		},
		"ReadFile": func(rec *diagRecorder) error {
			_, err := ReadFile(path, &ReadOptions{Force: true, OnDiagnostic: rec.hook})
			return err
		},
	}
	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := &diagRecorder{}
			if err := read(rec); err != nil {
				t.Fatalf("tolerant read must succeed: %v", err)
			}
			d := rec.first(t)
			if d.Kind != DiagnosticTruncatedValue {
				t.Errorf("Kind = %q, want %q", d.Kind, DiagnosticTruncatedValue)
			}
			if d.Tag != MustTag("PatientID") {
				t.Errorf("Tag = %s, want PatientID", d.Tag)
			}
			if d.Offset != wantOffset {
				t.Errorf("Offset = %d, want %d (file coordinates)", d.Offset, wantOffset)
			}
			if d.Need != 8 || d.Have != 2 {
				t.Errorf("Need/Have = %d/%d, want 8/2", d.Need, d.Have)
			}
			if len(d.Path) != 1 || d.Path[0] != MustTag("ReferencedImageSequence") {
				t.Errorf("Path = %v, want [ReferencedImageSequence]", d.Path)
			}
		})
	}
}

func TestDiagnostic_ErrorMessage(t *testing.T) {
	t.Parallel()
	d := Diagnostic{
		Kind:   DiagnosticTruncatedValue,
		Tag:    MustTag("PatientName"),
		VR:     VRPN,
		Offset: 0x1234,
		Path:   []Tag{MustTag("ReferencedImageSequence")},
		Need:   16,
		Have:   4,
	}
	want := "godicom: truncated_value at (0010,0010) PN (offset 4660/00001234) " +
		"in (0008,1140): need 16 bytes, have 4"
	if got := d.Error(); got != want {
		t.Fatalf("Error() =\n%s\nwant\n%s", got, want)
	}
}
