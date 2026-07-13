package godicom

import (
	"strings"
	"testing"
)

// pydicom.tests.test_dataset.TestDataset.test_formatted_lines
func TestDatasetFormattedLines(t *testing.T) {
	ds := NewDataset()
	if lines := ds.FormattedLines(nil); len(lines) != 0 {
		t.Fatalf("empty FormattedLines = %v", lines)
	}

	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "CITIZEN^Jan"))
	item := NewDataset()
	item.Set(NewDataElement(MustTag("PatientID"), VRLO, "JAN^Citizen"))
	ds.Set(NewDataElement(MustTag("BeamSequence"), VRSQ, NewSequence([]*Dataset{item})))

	lines := ds.FormattedLines(&FormatLinesOptions{
		ElementFormat:         "%(tag)s",
		SequenceElementFormat: "%(name)s %(tag)s",
	})
	want := []string{
		"(0010,0010)",
		"Beam Sequence (300A,00B0)",
		"(0010,0020)",
	}
	if len(lines) != len(want) {
		t.Fatalf("FormattedLines = %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("FormattedLines[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

// pydicom.tests.test_dataset.TestDataset.test_formatted_lines_known_uid
func TestDatasetStringKnownUID(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("TransferSyntaxUID"), VRUI, "1.2.840.10008.1.2"))
	got := ds.String()
	if !strings.Contains(got, "Implicit VR Little Endian") {
		t.Fatalf("String = %q, want known UID name", got)
	}
}

// pydicom.tests.test_dataset.TestDataset.test_top
func TestDatasetTop(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "CITIZEN^Jan"))
	item := NewDataset()
	item.Set(NewDataElement(MustTag("PatientID"), VRLO, "JAN^Citizen"))
	ds.Set(NewDataElement(MustTag("BeamSequence"), VRSQ, NewSequence([]*Dataset{item})))

	top := ds.Top()
	if !strings.Contains(top, "Patient's Name") {
		t.Fatalf("Top missing Patient's Name:\n%s", top)
	}
	if strings.Contains(top, "Patient ID") {
		t.Fatalf("Top should not include nested Patient ID:\n%s", top)
	}
	if !strings.Contains(top, "Beam Sequence") || !strings.Contains(top, "1 item(s) ----") {
		t.Fatalf("Top missing sequence summary:\n%s", top)
	}
}

func TestDatasetStringNestedSequence(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "CITIZEN^Jan"))
	item := NewDataset()
	item.Set(NewDataElement(MustTag("PatientID"), VRLO, "JAN^Citizen"))
	ds.Set(NewDataElement(MustTag("BeamSequence"), VRSQ, NewSequence([]*Dataset{item})))

	got := ds.String()
	if !strings.Contains(got, "Patient's Name") {
		t.Fatalf("String missing Patient's Name:\n%s", got)
	}
	if !strings.Contains(got, "Patient ID") {
		t.Fatalf("String missing nested Patient ID:\n%s", got)
	}
	if !strings.Contains(got, "---------") {
		t.Fatalf("String missing item delimiter:\n%s", got)
	}
}

func TestDatasetFormattedLinesDefault(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, "CITIZEN^Jan"))
	lines := ds.FormattedLines(nil)
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if !strings.Contains(lines[0], "(0010,0010)") || !strings.Contains(lines[0], "PN:") {
		t.Fatalf("line = %q", lines[0])
	}
}

func TestElementReprValueUIString(t *testing.T) {
	elem := NewDataElement(MustTag("TransferSyntaxUID"), VRUI, "1.2.840.10008.1.2")
	if got := elem.ReprValue(); got != "Implicit VR Little Endian" {
		t.Fatalf("ReprValue = %q", got)
	}
}
