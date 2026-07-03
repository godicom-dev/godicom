package godicom

import (
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
	for _, file := range []string{"chrFren.dcm", "chrRuss.dcm", "chrGreek.dcm", "chrX1.dcm"} {
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
