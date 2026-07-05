package godicom

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func charsetFilePath(name string) string {
	return filepath.Join("testdata", "charset", name)
}

func requireCharsetFile(t *testing.T, name string) string {
	t.Helper()
	path := charsetFilePath(name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("charset file %s not found (run scripts/fetch-testdata.sh): %v", name, err)
	}
	return path
}

// pydicom.tests.test_charset.FILE_PATIENT_NAMES
var charsetPatientNames = []struct {
	file string
	want string
}{
	{"chrArab.dcm", "قباني^لنزار"},
	{"chrFren.dcm", "Buc^Jérôme"},
	{"chrFrenMulti.dcm", "Buc^Jérôme"},
	{"chrGerm.dcm", "Äneas^Rüdiger"},
	{"chrGreek.dcm", "Διονυσιος"},
	{"chrH31.dcm", "Yamada^Tarou=山田^太郎=やまだ^たろう"},
	{"chrH32.dcm", "ﾔﾏﾀﾞ^ﾀﾛｳ=山田^太郎=やまだ^たろう"},
	{"chrHbrw.dcm", "שרון^דבורה"},
	{"chrI2.dcm", "Hong^Gildong=洪^吉洞=홍^길동"},
	{"chrJapMulti.dcm", "やまだ^たろう"},
	{"chrJapMultiExplicitIR6.dcm", "やまだ^たろう"},
	{"chrKoreanMulti.dcm", "김희중"},
	{"chrRuss.dcm", "Люкceмбypг"},
	{"chrX1.dcm", "Wang^XiaoDong=王^小東"},
	{"chrX2.dcm", "Wang^XiaoDong=王^小东"},
}

var charsetBytesIdenticalFiles = []string{
	"chrArab.dcm",
	"chrFren.dcm",
	"chrFrenMulti.dcm",
	"chrGerm.dcm",
	"chrGreek.dcm",
	"chrH31.dcm",
	"chrH32.dcm",
	"chrHbrw.dcm",
	"chrI2.dcm",
	"chrRuss.dcm",
	"chrX1.dcm",
	"chrX2.dcm",
}

func TestReadCharsetFilesPatientName(t *testing.T) {
	for _, tt := range charsetPatientNames {
		t.Run(tt.file, func(t *testing.T) {
			ds, err := ReadFile(requireCharsetFile(t, tt.file), nil)
			if err != nil {
				t.Fatal(err)
			}
			elem, ok := ds.Get(MustTag(0x00100010))
			if !ok {
				t.Fatal("PatientName missing")
			}
			pn, ok := elem.Value.(PersonName)
			if !ok {
				t.Fatalf("PatientName type = %T", elem.Value)
			}
			if pn.String() != tt.want {
				t.Fatalf("PatientName = %q, want %q", pn.String(), tt.want)
			}
		})
	}
}

func TestCharsetFileWriteRoundtrip(t *testing.T) {
	for _, tt := range charsetPatientNames {
		t.Run(tt.file, func(t *testing.T) {
			src := requireCharsetFile(t, tt.file)
			ds, err := ReadFile(src, nil)
			if err != nil {
				t.Fatal(err)
			}
			orig, ok := ds.Get(MustTag(0x00100010))
			if !ok {
				t.Fatal("PatientName missing")
			}
			wantPN := orig.Value.(PersonName).String()

			outPath := filepath.Join(t.TempDir(), tt.file)
			if err := ds.SaveAs(outPath, nil); err != nil {
				t.Fatal(err)
			}
			round, err := ReadFile(outPath, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := round.Get(MustTag(0x00100010))
			if !ok {
				t.Fatal("roundtrip PatientName missing")
			}
			if got.Value.(PersonName).String() != wantPN {
				t.Fatalf("roundtrip PN = %q, want %q", got.Value.(PersonName).String(), wantPN)
			}
		})
	}
}

func TestCharsetFileBytesIdentical(t *testing.T) {
	for _, file := range charsetBytesIdenticalFiles {
		t.Run(file, func(t *testing.T) {
			src := requireCharsetFile(t, file)
			orig, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			ds, err := ReadFile(src, nil)
			if err != nil {
				t.Fatal(err)
			}
			outPath := filepath.Join(t.TempDir(), file)
			if err := ds.SaveAs(outPath, nil); err != nil {
				t.Fatal(err)
			}
			written, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(orig, written) {
				t.Fatalf("byte roundtrip differs: orig=%d written=%d", len(orig), len(written))
			}
		})
	}
}

func TestReadCharsetFrenMultiValues(t *testing.T) {
	ds, err := ReadFile(requireCharsetFile(t, "chrFrenMulti.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	other, ok := ds.Get(MustTag(0x00101000))
	if !ok {
		t.Fatal("(0010,1000) missing")
	}
	mv, ok := other.Value.(*MultiValue[string])
	if !ok {
		t.Fatalf("(0010,1000) = %T, want MultiValue[string]", other.Value)
	}
	if mv.Len() != 2 {
		t.Fatalf("(0010,1000) len = %d, want 2", mv.Len())
	}
	vals := mv.Values()
	if vals[0] != "eggs" || vals[1] != "spam" {
		t.Fatalf("(0010,1000) = %q, want [eggs spam]", vals)
	}
	pnElem, ok := ds.Get(MustTag(0x00101001))
	if !ok {
		t.Fatal("(0010,1001) missing")
	}
	pnMV, ok := pnElem.Value.(*MultiValue[PersonName])
	if !ok {
		t.Fatalf("(0010,1001) = %T, want MultiValue[PersonName]", pnElem.Value)
	}
	if pnMV.Len() != 2 {
		t.Fatalf("(0010,1001) len = %d, want 2", pnMV.Len())
	}
	for i, pn := range pnMV.Values() {
		if pn.String() != "Buc^Jérôme" {
			t.Fatalf("PN[%d] = %q, want Buc^Jérôme", i, pn.String())
		}
	}
}

func TestReadCharsetSequenceEncoding(t *testing.T) {
	const want = "ﾔﾏﾀﾞ^ﾀﾛｳ=山田^太郎=やまだ^たろう"
	for _, file := range []string{"chrSQEncoding.dcm", "chrSQEncoding1.dcm"} {
		t.Run(file, func(t *testing.T) {
			ds, err := ReadFile(requireCharsetFile(t, file), nil)
			if err != nil {
				t.Fatal(err)
			}
			seqElem, ok := ds.Get(MustTag(0x00321064))
			if !ok {
				t.Fatal("scheduled procedure step sequence missing")
			}
			seq, ok := seqElem.Value.(*Sequence)
			if !ok || seq.Len() == 0 {
				t.Fatalf("sequence type = %T len = %d", seqElem.Value, seq.Len())
			}
			itemPN, ok := seq.Items()[0].Get(MustTag(0x00100010))
			if !ok {
				t.Fatal("sequence item PatientName missing")
			}
			if itemPN.Value.(PersonName).String() != want {
				t.Fatalf("sequence PN = %q, want %q", itemPN.Value.(PersonName).String(), want)
			}
		})
	}
}
