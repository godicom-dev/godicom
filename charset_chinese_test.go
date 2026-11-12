package godicom

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The fixtures behind these tests are written by pydicom, not by godicom -- see
// scripts/gen-charset-fixtures.py. A file godicom both writes and reads only
// proves the two halves of godicom agree with each other; these bytes come from
// the reference implementation, so a decode assertion here says godicom reads
// what pydicom wrote.
//
// Issue #88: the 17 chr*.dcm fixtures inherited from pydicom cover Arabic,
// French, German, Greek, Hebrew, Japanese, Korean and Russian. Chinese was one
// file, chrX2.dcm, carrying a two-byte name with no phonetic component and
// nothing outside the BMP. The three paths a mainland PACS hits most often --
// a four-byte character, all three component groups, and a multi-valued PN --
// had no fixture at all.

var gb18030ChineseNames = []struct {
	file  string
	want  string
	alpha string
	ideo  string
	phon  string
}{
	{
		// U+20000 is outside the BMP and costs four bytes in GB18030; U+9F98
		// (龘) costs three, so one name exercises both widths.
		file:  "chrGB18030Rare.dcm",
		want:  "\U00020000^\u5d0e=\u9f98^\u5d0e",
		alpha: "\U00020000^\u5d0e",
		ideo:  "\u9f98^\u5d0e",
	},
	{
		// alphabetic=ideographic=phonetic, every group non-empty. A reader that
		// splits on '=' and then drops empty groups passes the two-group case
		// and gets this one wrong.
		file:  "chrGB18030Three.dcm",
		want:  "Wang^XiaoDong=\u738b^\u5c0f\u4e1c=Wang^XiaoDong",
		alpha: "Wang^XiaoDong",
		ideo:  "\u738b^\u5c0f\u4e1c",
		phon:  "Wang^XiaoDong",
	},
}

func TestReadGB18030ChinesePatientName(t *testing.T) {
	t.Parallel()
	for _, tt := range gb18030ChineseNames {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			ds, err := ReadFile(requireCharsetFile(t, tt.file), nil)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			sets, ok := ds.GetString(TagCharset)
			if !ok || sets != "GB18030" {
				t.Fatalf("SpecificCharacterSet = %q, %v", sets, ok)
			}
			elem, ok := ds.Get(MustTag("PatientName"))
			if !ok {
				t.Fatal("PatientName missing")
			}
			pn, ok := elem.Value.(PersonName)
			if !ok {
				t.Fatalf("PatientName type = %T", elem.Value)
			}
			if got := pn.String(); got != tt.want {
				t.Fatalf("PatientName = %q, want %q", got, tt.want)
			}
			if pn.Alphabetic != tt.alpha {
				t.Errorf("Alphabetic = %q, want %q", pn.Alphabetic, tt.alpha)
			}
			if pn.Ideographic != tt.ideo {
				t.Errorf("Ideographic = %q, want %q", pn.Ideographic, tt.ideo)
			}
			if pn.Phonetic != tt.phon {
				t.Errorf("Phonetic = %q, want %q", pn.Phonetic, tt.phon)
			}
		})
	}
}

func TestReadGB18030PhoneticComponent(t *testing.T) {
	t.Parallel()
	ds, err := ReadFile(requireCharsetFile(t, "chrGB18030Three.dcm"), nil)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	elem, ok := ds.Get(MustTag("PatientName"))
	if !ok {
		t.Fatal("PatientName missing")
	}
	pn := elem.Value.(PersonName)
	if pn.Phonetic != "Wang^XiaoDong" {
		t.Fatalf("Phonetic = %q, want %q", pn.Phonetic, "Wang^XiaoDong")
	}
}

func TestReadGB18030MultiValuedPatientName(t *testing.T) {
	t.Parallel()
	ds, err := ReadFile(requireCharsetFile(t, "chrGB18030Multi.dcm"), nil)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	elem, ok := ds.Get(MustTag("PatientName"))
	if !ok {
		t.Fatal("PatientName missing")
	}
	mv, ok := elem.Value.(*MultiValue[PersonName])
	if !ok {
		t.Fatalf("PatientName type = %T, want *MultiValue[PersonName]", elem.Value)
	}
	if mv.Len() != 2 {
		t.Fatalf("values = %d, want 2", mv.Len())
	}
	// The '\' separates values; the '^' inside each value separates the name
	// components and must not split them into extra values.
	if got := mv.Get(0).String(); got != "\u738b^\u5c0f\u4e1c" {
		t.Errorf("value 0 = %q", got)
	}
	if got := mv.Get(1).String(); got != "\u674e^\u660e" {
		t.Errorf("value 1 = %q", got)
	}
}

// A GB18030 file read and written back unchanged has to leave the bytes alone:
// re-encoding what already decoded correctly is how a round trip starts
// producing different files from the same input.
func TestGB18030ChineseWriteRoundtrip(t *testing.T) {
	t.Parallel()
	files := []string{
		"chrGB18030Rare.dcm",
		"chrGB18030Three.dcm",
		"chrGB18030Multi.dcm",
	}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := requireCharsetFile(t, name)
			ds, err := ReadFile(src, nil)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			outPath := filepath.Join(t.TempDir(), name)
			if err := ds.SaveAs(outPath, nil); err != nil {
				t.Fatalf("save: %v", err)
			}
			original, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			written, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(original, written) {
				t.Fatalf("byte round trip differs: original=%d written=%d",
					len(original), len(written))
			}
		})
	}
}

// Changing the declaration from GB18030 to UTF-8 is only allowed once the
// values are known to have been decoded with the old one, so the writer can
// re-encode them instead of copying bytes that mean something else now. This is
// pydicom's test_changed_character_set with a Chinese value instead of a
// French one.
func TestGB18030ChineseRewrittenAsUTF8(t *testing.T) {
	t.Parallel()
	ds, err := ReadFile(requireCharsetFile(t, "chrGB18030Three.dcm"), nil)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := ds.ReinterpretCharacterSet("GB18030"); err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	if err := ds.SetString(TagCharset, "ISO_IR 192"); err != nil {
		t.Fatalf("set charset: %v", err)
	}
	out, err := EncodeDataset(ds.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	utf8Name := []byte("\u738b^\u5c0f\u4e1c")                      // UTF-8
	gb18030Name := []byte{0xcd, 0xf5, '^', 0xd0, 0xa1, 0xb6, 0xab} // GB18030
	if !bytes.Contains(out, utf8Name) {
		t.Fatalf("value was not re-encoded as UTF-8: %x", out)
	}
	if bytes.Contains(out, gb18030Name) {
		t.Fatalf("GB18030 bytes survived a UTF-8 rewrite: %x", out)
	}

	round := readBare(t, out)
	if got := textOf(t, round.Dataset, "PatientName"); got != "Wang^XiaoDong=\u738b^\u5c0f\u4e1c=Wang^XiaoDong" {
		t.Fatalf("re-read PatientName = %q", got)
	}
}

// Undecodable bytes keep the historical fallback -- the raw bytes become the
// element value, RawValue still holds them -- but the reader must say so. A
// silent U+FFFD is how a Chinese name turns into mojibake inside an archive
// that reports itself healthy.
func TestGB18030InvalidBytesReported(t *testing.T) {
	t.Parallel()
	var got []Diagnostic
	ds, err := ReadFile(requireCharsetFile(t, "chrGB18030Invalid.dcm"), &ReadOptions{
		OnDiagnostic: func(d Diagnostic) error {
			got = append(got, d)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("a nil hook must not fail the read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %v", len(got), got)
	}
	if got[0].Kind != DiagnosticInvalidCharacterData {
		t.Errorf("kind = %s, want %s", got[0].Kind, DiagnosticInvalidCharacterData)
	}
	if got[0].Tag != MustTag("PatientName") {
		t.Errorf("tag = %s, want (0010,0010)", got[0].Tag)
	}

	// 0x81 0x30 0x81 is a truncated four-byte sequence, 0xFF starts none; the
	// trailing 0x20 is the PN pad byte the writer added before it was replaced.
	want := []byte{0x81, 0x30, 0x81, 0xff, 0x81, 0x30, 0x81, 0x20}
	elem, ok := ds.Get(MustTag("PatientName"))
	if !ok {
		t.Fatal("PatientName missing")
	}
	raw, ok := elem.Value.([]byte)
	if !ok {
		t.Fatalf("undecodable value type = %T, want the raw bytes", elem.Value)
	}
	if !bytes.Equal(raw, want) {
		t.Errorf("value = %x, want %x", raw, want)
	}
	if !bytes.Equal(elem.RawValue, want) {
		t.Errorf("RawValue = %x, want %x", elem.RawValue, want)
	}
}
