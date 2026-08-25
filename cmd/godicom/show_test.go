package main

import (
	"bytes"
	"flag"
	"path/filepath"
	"regexp"
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

// -top narrows the -t search and does nothing on its own, because the unfiltered
// path lists the top level and reports a sequence by item count without ever
// descending into it.
//
// The two filtered cases differ only in topLevel and expect opposite results,
// which is the point: this test used to pass a nil filter with topLevel set, and
// so passed just as well with it unset. Nothing was checking the flag, which is
// how it came to be documented as "only show top-level elements" -- true of the
// unfiltered output whether or not the flag is given, and not what it does.
func TestWriteShowTopLevel(t *testing.T) {
	ds, err := godicom.ReadFile(cliTestFile("rtplan.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ds.Filename = "rtplan.dcm"

	// TreatmentMachineName appears only inside BeamSequence, so it is in the output
	// exactly when the search recurses.
	filterTags, err := parseShowTags([]string{"TreatmentMachineName"})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		opts       showOptions
		filter     map[godicom.Tag]struct{}
		wantNested bool
	}{
		{"filtered, recursive", showOptions{noMeta: true}, filterTags, true},
		{"filtered, top only", showOptions{noMeta: true, topLevel: true}, filterTags, false},
		{"unfiltered", showOptions{noMeta: true}, nil, false},
		{"unfiltered, top only", showOptions{noMeta: true, topLevel: true}, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeShow(&buf, ds, tt.opts, tt.filter); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if got := strings.Contains(out, "Treatment Machine Name"); got != tt.wantNested {
				t.Errorf("nested TreatmentMachineName present = %t, want %t:\n%s", got, tt.wantNested, out)
			}
			if tt.filter == nil && !strings.Contains(out, "Beam Sequence") {
				t.Errorf("unfiltered output should list the top-level BeamSequence:\n%s", out)
			}
		})
	}
}

// usageFlag matches a flag entry in printUsage's output: an indented line whose
// first token is a dash name. The command lines above it start with a word, so
// they cannot match.
var usageFlag = regexp.MustCompile(`(?m)^\s+-(\S+)`)

// printUsage listed only -debug for a long time while show grew four more flags,
// because nothing tied the text to the flag set. This checks both directions: an
// undocumented flag and a documented flag that no longer exists are the same kind
// of bug, and either one fails here.
func TestPrintUsageMatchesShowFlags(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)
	usage := buf.String()

	fs := newShowFlagSet(&showOptions{}, new(bool))

	fs.VisitAll(func(f *flag.Flag) {
		// \b so that -t does not count itself as documented by the -tag line.
		documented := regexp.MustCompile(`(?m)^\s+-` + regexp.QuoteMeta(f.Name) + `\b`)
		if !documented.MatchString(usage) {
			t.Errorf("-%s is registered but printUsage does not document it:\n%s", f.Name, usage)
		}
	})

	for _, m := range usageFlag.FindAllStringSubmatch(usage, -1) {
		if fs.Lookup(m[1]) == nil {
			t.Errorf("printUsage documents -%s but no such flag is registered", m[1])
		}
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
