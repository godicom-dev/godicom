package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
)

var cliTestDataDir = filepath.Join("..", "..", "pydicom", "src", "pydicom", "data", "test_files")

func cliTestFile(name string) string {
	return filepath.Join(cliTestDataDir, name)
}

func TestParseShowTags(t *testing.T) {
	tags, err := parseShowTags([]string{"PatientName", "00100020"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("got %d tags, want 2", len(tags))
	}
	if !hasTag(tags, godicom.MustTag("PatientName")) {
		t.Fatal("missing PatientName")
	}
	if !hasTag(tags, godicom.MustTag(0x00100020)) {
		t.Fatal("missing PatientID tag")
	}
}

func TestWriteShowTagFilter(t *testing.T) {
	ds, err := godicom.ReadFile(cliTestFile("MR_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.Filename = "MR_small.dcm"

	filterTags, err := parseShowTags([]string{"PatientName", "Rows"})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writeShow(&buf, ds, showOptions{noMeta: true}, filterTags); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Patient's Name") {
		t.Fatalf("output missing PatientName:\n%s", out)
	}
	if !strings.Contains(out, "Rows") {
		t.Fatalf("output missing Rows:\n%s", out)
	}
	if strings.Contains(out, "Columns") {
		t.Fatalf("output should not include Columns:\n%s", out)
	}
	if !strings.Contains(out, "Matching elements:") {
		t.Fatalf("output missing match count:\n%s", out)
	}
}

func TestWriteShowTopLevel(t *testing.T) {
	ds, err := godicom.ReadFile(cliTestFile("rtplan.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.Filename = "rtplan.dcm"

	var buf bytes.Buffer
	if err := writeShow(&buf, ds, showOptions{noMeta: true, topLevel: true}, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Beam Sequence") {
		t.Fatalf("output missing BeamSequence:\n%s", out)
	}
	if strings.Contains(out, "Treatment Machine Name") {
		t.Fatalf("nested TreatmentMachineName should not appear in top mode:\n%s", out)
	}
}

func TestWriteShowNestedTagFilter(t *testing.T) {
	ds, err := godicom.ReadFile(cliTestFile("rtplan.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.Filename = "rtplan.dcm"

	filterTags, err := parseShowTags([]string{"TreatmentMachineName"})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writeShow(&buf, ds, showOptions{noMeta: true}, filterTags); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Treatment Machine Name") {
		t.Fatalf("output missing nested tag:\n%s", out)
	}
}
