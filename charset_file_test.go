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

var charsetPatientNames = []struct {
	file string
	want string
}{
	{"chrRuss.dcm", "Люкceмбypг"},
	{"chrFren.dcm", "Buc^Jérôme"},
	{"chrGreek.dcm", "Διονυσιος"},
	{"chrX1.dcm", "Wang^XiaoDong=王^小東"},
	{"chrH31.dcm", "Yamada^Tarou=山田^太郎=やまだ^たろう"},
	{"chrFrenMulti.dcm", "Buc^Jérôme"},
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
	for _, file := range []string{
		"chrFren.dcm", "chrRuss.dcm", "chrGreek.dcm", "chrX1.dcm",
		"chrH31.dcm", "chrFrenMulti.dcm",
	} {
		t.Run(file, func(t *testing.T) {
			src := requireCharsetFile(t, file)
			ds, err := ReadFile(src, nil)
			if err != nil {
				t.Fatal(err)
			}
			orig, ok := ds.Get(MustTag(0x00100010))
			if !ok {
				t.Fatal("PatientName missing")
			}
			wantPN := orig.Value.(PersonName).String()

			outPath := filepath.Join(t.TempDir(), file)
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

// pydicom.tests.test_filewriter.TestWriteFile.test_unicode / testMultiPN
func TestCharsetFileBytesIdentical(t *testing.T) {
	for _, file := range []string{"chrH31.dcm", "chrFrenMulti.dcm"} {
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
	ds, err := ReadFile(requireCharsetFile(t, "chrSQEncoding.dcm"), nil)
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
	want := "ﾔﾏﾀﾞ^ﾀﾛｳ=山田^太郎=やまだ^たろう"
	if itemPN.Value.(PersonName).String() != want {
		t.Fatalf("sequence PN = %q, want %q", itemPN.Value.(PersonName).String(), want)
	}
}
