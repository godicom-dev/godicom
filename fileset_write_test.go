package godicom

import (
	"iter"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// fileIDs pulls n File IDs out of a generated sequence.
func fileIDs(t *testing.T, seq iter.Seq[string], n int) []string {
	t.Helper()
	var got []string
	for id := range seq {
		got = append(got, id)
		if len(got) == n {
			break
		}
	}
	if len(got) != n {
		t.Fatalf("generated %d File IDs, want %d", len(got), n)
	}
	return got
}

// TestGenerateFilenameNumeric mirrors pydicom's
// TestGenerateFilename::test_numeric.
func TestGenerateFilenameNumeric(t *testing.T) {
	seq, err := GenerateFilename("", 0, false)
	if err != nil {
		t.Fatalf("GenerateFilename() error = %v", err)
	}
	want := []string{
		"00000000", "00000001", "00000002", "00000003", "00000004",
		"00000005", "00000006", "00000007", "00000008", "00000009",
		"00000010",
	}
	if got := fileIDs(t, seq, len(want)); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("File IDs = %v, want %v", got, want)
	}
}

// TestGenerateFilenameNumericPrefix mirrors test_numeric_prefix: the suffix
// shrinks as the prefix grows, so every ID stays eight characters.
func TestGenerateFilenameNumericPrefix(t *testing.T) {
	for n := 1; n <= 7; n++ {
		prefix := strings.Repeat("A", n)
		seq, err := GenerateFilename(prefix, 0, false)
		if err != nil {
			t.Fatalf("prefix %q: GenerateFilename() error = %v", prefix, err)
		}
		want := prefix + strings.Repeat("0", 8-n)
		if got := fileIDs(t, seq, 1)[0]; got != want {
			t.Errorf("prefix %q gave %q, want %q", prefix, got, want)
		}
	}
}

// TestGenerateFilenameNumericStart mirrors test_numeric_start.
func TestGenerateFilenameNumericStart(t *testing.T) {
	seq, err := GenerateFilename("", 10, false)
	if err != nil {
		t.Fatalf("GenerateFilename() error = %v", err)
	}
	want := []string{"00000010", "00000011", "00000012"}
	if got := fileIDs(t, seq, len(want)); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("File IDs = %v, want %v", got, want)
	}
}

// TestGenerateFilenameAlphanumeric mirrors test_alphanumeric. J is not in the
// character set, so Z follows Y at index 34 and the next ID carries into the
// second position.
func TestGenerateFilenameAlphanumeric(t *testing.T) {
	seq, err := GenerateFilename("", 0, true)
	if err != nil {
		t.Fatalf("GenerateFilename() error = %v", err)
	}
	all := fileIDs(t, seq, 36)
	want := []string{
		"00000000", "00000001", "00000002", "00000003", "00000004",
		"00000005", "00000006", "00000007", "00000008", "00000009",
	}
	if strings.Join(all[:10], ",") != strings.Join(want, ",") {
		t.Errorf("first ten = %v, want %v", all[:10], want)
	}
	if all[10] != "0000000A" {
		t.Errorf("index 10 = %q, want 0000000A", all[10])
	}
	if all[34] != "0000000Z" {
		t.Errorf("index 34 = %q, want 0000000Z", all[34])
	}
	if all[35] != "00000010" {
		t.Errorf("index 35 = %q, want 00000010", all[35])
	}
	for _, id := range all {
		if strings.ContainsRune(id, 'J') {
			t.Fatalf("File ID %q contains J, which is not in the character set", id)
		}
	}
}

// TestGenerateFilenameAlphanumericPrefix mirrors test_alphanumeric_prefix.
func TestGenerateFilenameAlphanumericPrefix(t *testing.T) {
	for n := 1; n <= 7; n++ {
		prefix := strings.Repeat("A", n)
		seq, err := GenerateFilename(prefix, 0, true)
		if err != nil {
			t.Fatalf("prefix %q: GenerateFilename() error = %v", prefix, err)
		}
		got := fileIDs(t, seq, 11)
		if want := prefix + strings.Repeat("0", 8-n); got[0] != want {
			t.Errorf("prefix %q index 0 = %q, want %q", prefix, got[0], want)
		}
		if want := prefix + strings.Repeat("0", 7-n) + "A"; got[10] != want {
			t.Errorf("prefix %q index 10 = %q, want %q", prefix, got[10], want)
		}
		if len(got[0]) != 8 {
			t.Errorf("prefix %q gave a File ID of %d characters, want 8", prefix, len(got[0]))
		}
	}
}

// TestGenerateFilenameAlphanumericStart mirrors test_alphanumeric_start.
func TestGenerateFilenameAlphanumericStart(t *testing.T) {
	seq, err := GenerateFilename("", 10, true)
	if err != nil {
		t.Fatalf("GenerateFilename() error = %v", err)
	}
	want := []string{"0000000A", "0000000B", "0000000C"}
	if got := fileIDs(t, seq, len(want)); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("File IDs = %v, want %v", got, want)
	}
}

// TestGenerateFilenameLongPrefix mirrors test_long_prefix_raises.
func TestGenerateFilenameLongPrefix(t *testing.T) {
	_, err := GenerateFilename(strings.Repeat("A", 8), 0, false)
	if err == nil {
		t.Fatal("expected an error for a prefix of 8 characters")
	}
	if !strings.Contains(err.Error(), "less than 8 characters") {
		t.Errorf("error = %v, want it to name the prefix limit", err)
	}
}

// TestGenerateFilenameNegativeStart pins the one place this differs from
// pydicom: a negative index there produces a blank suffix, which is not a File
// ID at all. Here it starts at zero.
func TestGenerateFilenameNegativeStart(t *testing.T) {
	seq, err := GenerateFilename("", -5, false)
	if err != nil {
		t.Fatalf("GenerateFilename() error = %v", err)
	}
	if got := fileIDs(t, seq, 1)[0]; got != "00000000" {
		t.Errorf("first File ID = %q, want 00000000", got)
	}
}

// TestIsConformantFileID covers the PS3.10 Section 8.5 rules one at a time.
func TestIsConformantFileID(t *testing.T) {
	cases := []struct {
		name    string
		parts   []string
		want    bool
		because string
	}{
		{"eight components", []string{"A", "B", "C", "D", "E", "F", "G", "H"}, true, "eight is the limit"},
		{"nine components", []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"}, false, "more than eight components"},
		{"eight characters", []string{"ABCDEFGH"}, true, "eight is the limit"},
		{"nine characters", []string{"ABCDEFGHI"}, false, "more than eight characters in a component"},
		{"digits and underscore", []string{"PT000001", "SE_1"}, true, "all allowed characters"},
		{"lowercase", []string{"pt000001"}, false, "lower case is not in the character set"},
		{"hyphen", []string{"PT-1"}, false, "hyphen is not in the character set"},
		{"non-ASCII", []string{"PT00Ä1"}, false, "characters must be ASCII"},
		{"empty component", []string{""}, true, "the pattern matches the empty string"},
		{"no components", nil, true, "the pattern matches the empty string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsConformantFileID(tc.parts); got != tc.want {
				t.Errorf("IsConformantFileID(%v) = %v, want %v: %s", tc.parts, got, tc.want, tc.because)
			}
		})
	}
}

// TestRecordTypeTablesResolve checks the three static tables against each
// other. A record type with no recorder, or a SOP Class keyword that is not in
// the UID dictionary, would panic at init instead of failing here, so this test
// is what turns that into a readable failure.
func TestRecordTypeTablesResolve(t *testing.T) {
	tables := map[string]map[string]string{
		"single-level": singleLevelRecordTypeKeywords,
		"four-level":   fourLevelRecordTypeKeywords,
	}
	for name, table := range tables {
		for keyword, recordType := range table {
			if _, ok := uid.LookupKeyword(keyword); !ok {
				t.Errorf("%s table: SOP Class keyword %q is not in the UID dictionary", name, keyword)
			}
			if _, ok := directoryRecorders[recordType]; !ok {
				t.Errorf("%s table: record type %q has no recorder", name, recordType)
			}
			if _, ok := recordTypePrefixes[recordType]; !ok {
				t.Errorf("%s table: record type %q has no File ID prefix", name, recordType)
			}
		}
	}
}

// TestEveryRecorderHasAPrefix catches a record type that can be produced but
// not named.
func TestEveryRecorderHasAPrefix(t *testing.T) {
	for recordType := range directoryRecorders {
		if _, ok := recordTypePrefixes[recordType]; !ok {
			t.Errorf("record type %q has a recorder but no File ID prefix", recordType)
		}
	}
}

// instanceWith builds a File Dataset holding the elements build sets, plus the
// SOP Class, SOP Instance UID and transfer syntax a record refers to.
func instanceWith(t *testing.T, sopClassKeyword string, build func(*Dataset)) *FileDataset {
	t.Helper()
	ds := NewDataset()
	if build != nil {
		build(ds)
	}
	sopClass, ok := uid.LookupKeyword(sopClassKeyword)
	if !ok {
		t.Fatalf("unknown SOP Class keyword %q", sopClassKeyword)
	}
	if err := ds.SetString(tag.SOPClassUID, string(sopClass)); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetString(tag.SOPInstanceUID, "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	meta := NewFileMetaDataset()
	if err := meta.SetString(tag.TransferSyntaxUID, string(ExplicitVRLittleEndian)); err != nil {
		t.Fatal(err)
	}
	return &FileDataset{Dataset: ds, FileMeta: meta}
}

// setStrings sets a list of keywords on ds, failing the test on error.
func setStrings(t *testing.T, ds *Dataset, values map[string]string) {
	t.Helper()
	for keyword, value := range values {
		if err := ds.SetString(MustTag(keyword), value); err != nil {
			t.Fatalf("%s: %v", keyword, err)
		}
	}
}

// ctInstance is the four-level case every File-set has: a patient, a study, a
// series and an image.
func ctInstance(t *testing.T) *FileDataset {
	t.Helper()
	return instanceWith(t, "CTImageStorage", func(ds *Dataset) {
		setStrings(t, ds, map[string]string{
			"PatientName":       "Doe^John",
			"PatientID":         "ID-1",
			"StudyDate":         "20260101",
			"StudyTime":         "120000",
			"StudyID":           "STUDY-1",
			"StudyDescription":  "Chest",
			"StudyInstanceUID":  "1.2.3.5",
			"Modality":          "CT",
			"SeriesInstanceUID": "1.2.3.6",
			"SeriesNumber":      "2",
			"InstanceNumber":    "3",
		})
	})
}

func recordTypesOf(records []*Dataset) []string {
	types := make([]string, len(records))
	for i, record := range records {
		types[i], _ = record.GetString(tag.DirectoryRecordType)
	}
	return types
}

// TestDirectoryRecordsFourLevel mirrors what pydicom's _recordify yields for a
// dataset with a patient, study, series and image in it.
func TestDirectoryRecordsFourLevel(t *testing.T) {
	records, err := DirectoryRecords(ctInstance(t))
	if err != nil {
		t.Fatalf("DirectoryRecords() error = %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("got %d records, want 4", len(records))
	}
	if got, want := strings.Join(recordTypesOf(records), ","), "PATIENT,STUDY,SERIES,IMAGE"; got != want {
		t.Fatalf("record types = %s, want %s", got, want)
	}

	values := []struct {
		index   int
		keyword string
		want    string
	}{
		{0, "PatientName", "Doe^John"},
		{0, "PatientID", "ID-1"},
		{1, "StudyDate", "20260101"},
		{1, "StudyTime", "120000"},
		{1, "StudyID", "STUDY-1"},
		{1, "StudyDescription", "Chest"},
		{2, "Modality", "CT"},
		{2, "SeriesNumber", "2"},
		{3, "InstanceNumber", "3"},
	}
	for _, v := range values {
		got, ok := records[v.index].GetString(MustTag(v.keyword))
		if !ok || got != v.want {
			t.Errorf("%s record %s = %q (present %v), want %q", recordTypesOf(records)[v.index], v.keyword, got, ok, v.want)
		}
	}

	for i, record := range records {
		flag, ok := record.GetInt(tag.RecordInUseFlag)
		if !ok || flag != 0xFFFF {
			t.Errorf("record %d RecordInUseFlag = %d (present %v), want 65535", i, flag, ok)
		}
		for _, offset := range []Tag{
			tag.OffsetOfTheNextDirectoryRecord,
			tag.OffsetOfReferencedLowerLevelDirectoryEntity,
		} {
			if value, ok := record.GetInt(offset); !ok || value != 0 {
				t.Errorf("record %d %s = %d (present %v), want 0", i, offset, value, ok)
			}
		}
	}

	// Only the leaf names a file; the records above it name their children.
	for i, record := range records[:3] {
		if _, ok := record.Get(tag.ReferencedSOPInstanceUIDInFile); ok {
			t.Errorf("record %d (%s) names a SOP Instance, which only the leaf does", i, recordTypesOf(records)[i])
		}
	}
	leaf := records[3]
	if _, ok := leaf.Get(tag.ReferencedFileID); !ok {
		t.Error("the leaf has no Referenced File ID element for the writer to fill in")
	} else if elem, _ := leaf.Get(tag.ReferencedFileID); elem.VM() != 0 {
		t.Errorf("the leaf's Referenced File ID is not empty: %v", elem.Value)
	}
	if got, _ := leaf.GetString(tag.ReferencedSOPInstanceUIDInFile); got != "1.2.3.4" {
		t.Errorf("Referenced SOP Instance UID = %q, want 1.2.3.4", got)
	}
	if got, _ := leaf.GetString(tag.ReferencedTransferSyntaxUIDInFile); got != string(ExplicitVRLittleEndian) {
		t.Errorf("Referenced Transfer Syntax UID = %q, want %s", got, ExplicitVRLittleEndian)
	}
	if got, _ := leaf.GetString(tag.ReferencedSOPClassUIDInFile); !strings.Contains(got, "1.2.840.10008.5.1.4.1.1.2") {
		t.Errorf("Referenced SOP Class UID = %q, want the instance's SOP Class", got)
	}
}

// TestDirectoryRecordsCarryTheCharacterSet checks that a record decoded under a
// declared character set decodes the same way the instance did.
func TestDirectoryRecordsCarryTheCharacterSet(t *testing.T) {
	inst := instanceWith(t, "CTImageStorage", func(ds *Dataset) {
		setStrings(t, ds, map[string]string{
			"SpecificCharacterSet": "ISO_IR 192",
			"PatientName":          "Wang^XiaoDong",
			"PatientID":            "ID-1",
			"StudyDate":            "20260101",
			"StudyTime":            "120000",
			"StudyID":              "STUDY-1",
			"Modality":             "CT",
			"SeriesInstanceUID":    "1.2.3.6",
			"SeriesNumber":         "2",
			"InstanceNumber":       "3",
		})
	})
	records, err := DirectoryRecords(inst)
	if err != nil {
		t.Fatalf("DirectoryRecords() error = %v", err)
	}
	for i, record := range records {
		got, ok := record.GetString(tag.SpecificCharacterSet)
		if !ok || got != "ISO_IR 192" {
			t.Errorf("record %d SpecificCharacterSet = %q (present %v), want ISO_IR 192", i, got, ok)
		}
	}
}

// TestDirectoryRecordsOptionalElementStaysPresent pins pydicom's behaviour for
// the optional record elements: a missing Study Description is written as an
// empty element, not left out. The two are not the same file, and a reader that
// expects the element would see a different dataset.
func TestDirectoryRecordsOptionalElementStaysPresent(t *testing.T) {
	inst := instanceWith(t, "CTImageStorage", func(ds *Dataset) {
		setStrings(t, ds, map[string]string{
			"PatientID":         "ID-1",
			"StudyDate":         "20260101",
			"StudyTime":         "120000",
			"StudyID":           "STUDY-1",
			"Modality":          "CT",
			"SeriesInstanceUID": "1.2.3.6",
			"SeriesNumber":      "2",
			"InstanceNumber":    "3",
		})
	})
	records, err := DirectoryRecords(inst)
	if err != nil {
		t.Fatalf("DirectoryRecords() error = %v", err)
	}

	for _, tc := range []struct {
		index   int
		keyword string
	}{
		{0, "PatientName"},
		{1, "StudyDescription"},
		{1, "AccessionNumber"},
	} {
		record := records[tc.index]
		elem, ok := record.Get(MustTag(tc.keyword))
		if !ok {
			t.Errorf("record %d has no %s element, want an empty one", tc.index, tc.keyword)
			continue
		}
		if elem.VM() != 0 {
			t.Errorf("record %d %s = %v, want an empty value", tc.index, tc.keyword, elem.Value)
		}
	}
}

// TestDirectoryRecordsSingleLevel covers the SOP Classes that are filed
// directly under the root instead of under PATIENT/STUDY/SERIES.
func TestDirectoryRecordsSingleLevel(t *testing.T) {
	inst := instanceWith(t, "ColorPaletteStorage", func(ds *Dataset) {
		setStrings(t, ds, map[string]string{
			"ContentLabel":       "PALETTE",
			"ContentDescription": "A palette",
		})
	})
	records, err := DirectoryRecords(inst)
	if err != nil {
		t.Fatalf("DirectoryRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if got := recordTypesOf(records)[0]; got != "PALETTE" {
		t.Errorf("record type = %q, want PALETTE", got)
	}
	if got, _ := records[0].GetString(tag.ContentLabel); got != "PALETTE" {
		t.Errorf("ContentLabel = %q, want PALETTE", got)
	}
	if _, ok := records[0].Get(tag.ReferencedFileID); !ok {
		t.Error("the record has no Referenced File ID element")
	}
}

// TestDirectoryRecordsLeafType covers the checks that come before the SOP Class
// table, in the order pydicom applies them.
func TestDirectoryRecordsLeafType(t *testing.T) {
	cases := []struct {
		name    string
		keyword string
		build   func(*Dataset)
		want    string
	}{
		{
			name:    "radiotherapy modality",
			keyword: "RTImageStorage",
			build: func(ds *Dataset) {
				setStrings(t, ds, map[string]string{"Modality": "RTRAD", "InstanceNumber": "1"})
			},
			want: "RADIOTHERAPY",
		},
		{
			name:    "plan modality",
			keyword: "RTImageStorage",
			build: func(ds *Dataset) {
				setStrings(t, ds, map[string]string{"Modality": "PLAN", "InstanceNumber": "1"})
			},
			want: "PLAN",
		},
		{
			name:    "encapsulated document",
			keyword: "CTImageStorage",
			build: func(ds *Dataset) {
				setStrings(t, ds, map[string]string{
					"Modality":                       "OT",
					"InstanceNumber":                 "1",
					"MIMETypeOfEncapsulatedDocument": "application/pdf",
				})
				ds.Set(NewDataElement(tag.EncapsulatedDocument, VROB, []byte("%PDF-1.7")))
			},
			want: "ENCAP DOC",
		},
		{
			name:    "rt plan label",
			keyword: "CTImageStorage",
			build: func(ds *Dataset) {
				setStrings(t, ds, map[string]string{
					"Modality":       "RT",
					"InstanceNumber": "1",
					"RTPlanLabel":    "PLAN-A",
				})
			},
			want: "RT PLAN",
		},
		{
			name:    "rt dose sop class",
			keyword: "RTDoseStorage",
			build: func(ds *Dataset) {
				setStrings(t, ds, map[string]string{
					"Modality":          "RTDOSE",
					"InstanceNumber":    "1",
					"DoseSummationType": "PLAN",
				})
			},
			want: "RT DOSE",
		},
		{
			name:    "plain image falls through",
			keyword: "CTImageStorage",
			build: func(ds *Dataset) {
				setStrings(t, ds, map[string]string{"Modality": "CT", "InstanceNumber": "1"})
			},
			want: "IMAGE",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inst := instanceWith(t, tc.keyword, func(ds *Dataset) {
				setStrings(t, ds, map[string]string{
					"PatientID":         "ID-1",
					"StudyDate":         "20260101",
					"StudyTime":         "120000",
					"StudyID":           "STUDY-1",
					"SeriesInstanceUID": "1.2.3.6",
					"SeriesNumber":      "2",
				})
				tc.build(ds)
			})
			records, err := DirectoryRecords(inst)
			if err != nil {
				t.Fatalf("DirectoryRecords() error = %v", err)
			}
			if len(records) != 4 {
				t.Fatalf("got %d records, want 4", len(records))
			}
			if got := recordTypesOf(records)[3]; got != tc.want {
				t.Errorf("leaf record type = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDirectoryRecordsRefusesAMissingType1Element checks the message a caller
// sees when the instance cannot be filed. pydicom raises ValueError naming the
// record type, the tag and the element; the same three details are here.
func TestDirectoryRecordsRefusesAMissingType1Element(t *testing.T) {
	cases := []struct {
		name string
		drop string
		want string
	}{
		{"missing Patient ID", "PatientID", "(0010,0020) 'Patient ID' element is missing"},
		{"missing Study Date", "StudyDate", "(0008,0020) 'Study Date' element is missing"},
		{"missing Modality", "Modality", "(0008,0060) 'Modality' element is missing"},
		{"missing Instance Number", "InstanceNumber", "(0020,0013) 'Instance Number' element is missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inst := ctInstance(t)
			inst.Dataset.Delete(MustTag(tc.drop))
			_, err := DirectoryRecords(inst)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), "unable to use the default") {
				t.Errorf("error = %v, want it to name the record creator", err)
			}
		})
	}
}

// TestDirectoryRecordsRefusesAnEmptyType1Element is the other half of
// pydicom's _check_dataset: an element that is there but has no value would
// produce a record that points at a patient with no ID.
func TestDirectoryRecordsRefusesAnEmptyType1Element(t *testing.T) {
	inst := ctInstance(t)
	inst.Dataset.Set(NewDataElement(tag.PatientID, VRLO, nil))

	_, err := DirectoryRecords(inst)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("error = %v, want it to say the element cannot be empty", err)
	}
}

// The fixture the parity comparison below runs against. It is the same instance
// pydicom's own test suite files in TestFileSet.
const ctSmallFixture = "pydicom/src/pydicom/data/test_files/CT_small.dcm"

// TestDirectoryRecordsMatchPyDicomOnAnInstance locks the whole record set for a
// real instance against what pydicom 3.1.0.dev0's FileSet._recordify() produces for
// the same file. Both sides were dumped as sorted (tag, value) lines and
// compared: 40 lines, identical. The element counts below are what makes this
// more than a spot check -- a record that gained or lost an attribute would
// still pass a value-by-value comparison.
func TestDirectoryRecordsMatchPyDicomOnAnInstance(t *testing.T) {
	ds, err := ReadFile(ctSmallFixture, nil)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", ctSmallFixture, err)
	}
	records, err := DirectoryRecords(ds)
	if err != nil {
		t.Fatalf("DirectoryRecords() error = %v", err)
	}

	if got, want := strings.Join(recordTypesOf(records), ","), "PATIENT,STUDY,SERIES,IMAGE"; got != want {
		t.Fatalf("record types = %s, want %s", got, want)
	}

	// Element counts per record, from the pydicom dump.
	for i, want := range []int{7, 11, 8, 10} {
		if got := len(records[i].Elements()); got != want {
			t.Errorf("%s record has %d elements, want %d", recordTypesOf(records)[i], got, want)
		}
	}

	// Values copied straight from the instance.
	for _, tc := range []struct {
		index   int
		keyword string
		want    string
	}{
		{0, "PatientName", "CompressedSamples^CT1"},
		{0, "PatientID", "1CT1"},
		{1, "StudyDate", "20040119"},
		{1, "StudyTime", "072730"},
		{1, "StudyDescription", "e+1"},
		{1, "StudyInstanceUID", "1.3.6.1.4.1.5962.1.2.1.20040119072730.12322"},
		{1, "StudyID", "1CT1"},
		{2, "Modality", "CT"},
		{2, "SeriesInstanceUID", "1.3.6.1.4.1.5962.1.3.1.1.20040119072730.12322"},
		{2, "SeriesNumber", "1"},
		{3, "InstanceNumber", "1"},
		{3, "ReferencedSOPClassUIDInFile", "1.2.840.10008.5.1.4.1.1.2"},
		{3, "ReferencedSOPInstanceUIDInFile", "1.3.6.1.4.1.5962.1.1.1.1.1.20040119072730.12322"},
		{3, "ReferencedTransferSyntaxUIDInFile", "1.2.840.10008.1.2.1"},
	} {
		got, ok := records[tc.index].GetString(MustTag(tc.keyword))
		if !ok || got != tc.want {
			t.Errorf("%s record %s = %q (present %v), want %q", recordTypesOf(records)[tc.index], tc.keyword, got, ok, tc.want)
		}
	}

	// Accession Number is absent from this instance and still present in the
	// record, empty, which is what pydicom writes.
	if elem, ok := records[1].Get(tag.AccessionNumber); !ok || elem.VM() != 0 {
		t.Errorf("STUDY AccessionNumber = %v (present %v), want an empty element", elem, ok)
	}

	// The character set this instance declares is on every record.
	for i, record := range records {
		if got, _ := record.GetString(tag.SpecificCharacterSet); got != "ISO_IR 100" {
			t.Errorf("record %d SpecificCharacterSet = %q, want ISO_IR 100", i, got)
		}
	}
}

// TestDirectoryRecordsRefusesAnInstancePyDicomRefuses is the other half of the
// comparison: rtdose.dcm in pydicom's test data has an empty Instance Number,
// and pydicom refuses to file it --
//
//	Unable to use the default 'RT DOSE' record creator: The instance's
//	(0020,0013) 'Instance Number' element cannot be empty
//
// The same instance is refused here, for the same reason, naming the same tag.
func TestDirectoryRecordsRefusesAnInstancePyDicomRefuses(t *testing.T) {
	ds, err := ReadFile("pydicom/src/pydicom/data/test_files/rtdose.dcm", nil)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	_, err = DirectoryRecords(ds)
	if err == nil {
		t.Fatal("expected the instance to be refused")
	}
	for _, want := range []string{
		`"RT DOSE"`,
		"(0020,0013)",
		"Instance Number",
		"cannot be empty",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to contain %q", err, want)
		}
	}
}

// recorderCase is one directory recorder under test: the attributes a valid
// instance needs at that level, and the elements the record has to end up with.
type recorderCase struct {
	recordType string
	strings    map[string]string
	ints       map[string]int
	multi      map[string][]string
	sequences  []string
	want       []string
}

// TestEveryRecorderProducesItsRecord walks all of directoryRecorders. A typo in
// one of the keyword literals inside a recorder turns into a missing element
// here instead of a record with a hole in it, and every branch of every recorder
// -- including the "copy this optional element only when the instance has it"
// ones -- is reached.
func TestEveryRecorderProducesItsRecord(t *testing.T) {
	codeSequence := func() []string { return []string{"ConceptNameCodeSequence"} }
	cases := []recorderCase{
		{
			recordType: "PATIENT",
			strings:    map[string]string{"PatientName": "Doe^John", "PatientID": "ID-1"},
			want:       []string{"PatientName", "PatientID"},
		},
		{
			recordType: "STUDY",
			strings: map[string]string{
				"StudyDate": "20260101", "StudyTime": "120000", "StudyID": "S1",
				"StudyDescription": "Chest", "StudyInstanceUID": "1.2.3", "AccessionNumber": "A1",
			},
			want: []string{"StudyDate", "StudyTime", "StudyID", "StudyDescription", "StudyInstanceUID", "AccessionNumber"},
		},
		{
			recordType: "SERIES",
			strings:    map[string]string{"Modality": "CT", "SeriesInstanceUID": "1.2.4", "SeriesNumber": "2"},
			want:       []string{"Modality", "SeriesInstanceUID", "SeriesNumber"},
		},
		{
			recordType: "IMAGE",
			strings:    map[string]string{"InstanceNumber": "3"},
			want:       []string{"InstanceNumber"},
		},
		{
			recordType: "RT DOSE",
			strings:    map[string]string{"InstanceNumber": "3", "DoseSummationType": "PLAN"},
			want:       []string{"InstanceNumber", "DoseSummationType"},
		},
		{
			recordType: "RT STRUCTURE SET",
			strings: map[string]string{
				"InstanceNumber": "3", "StructureSetLabel": "SS1",
				"StructureSetDate": "20260101", "StructureSetTime": "120000",
			},
			want: []string{"InstanceNumber", "StructureSetLabel", "StructureSetDate", "StructureSetTime"},
		},
		{
			recordType: "RT PLAN",
			strings: map[string]string{
				"InstanceNumber": "3", "RTPlanLabel": "PL1",
				"RTPlanDate": "20260101", "RTPlanTime": "120000",
			},
			want: []string{"InstanceNumber", "RTPlanLabel", "RTPlanDate", "RTPlanTime"},
		},
		{
			recordType: "RT TREAT RECORD",
			strings: map[string]string{
				"InstanceNumber": "3", "TreatmentDate": "20260101", "TreatmentTime": "120000",
			},
			want: []string{"InstanceNumber", "TreatmentDate", "TreatmentTime"},
		},
		{
			recordType: "PRESENTATION",
			strings: map[string]string{
				"PresentationCreationDate": "20260101",
				"PresentationCreationTime": "120000",
				"InstanceNumber":           "3",
				"ContentLabel":             "PR1",
				"ContentDescription":       "A presentation",
				"ContentCreatorName":       "Doe^John",
			},
			sequences: []string{"ReferencedSeriesSequence", "BlendingSequence"},
			want: []string{
				"PresentationCreationDate", "PresentationCreationTime", "InstanceNumber",
				"ContentLabel", "ContentDescription", "ContentCreatorName",
				"ReferencedSeriesSequence", "BlendingSequence",
			},
		},
		{
			recordType: "SR DOCUMENT",
			strings: map[string]string{
				"InstanceNumber": "3", "CompletionFlag": "COMPLETE", "VerificationFlag": "UNVERIFIED",
				"ContentDate": "20260101", "ContentTime": "120000", "VerificationDateTime": "20260101120000",
			},
			sequences: []string{"ConceptNameCodeSequence", "ContentSequence"},
			want: []string{
				"InstanceNumber", "CompletionFlag", "VerificationFlag", "ContentDate",
				"ContentTime", "VerificationDateTime", "ConceptNameCodeSequence", "ContentSequence",
			},
		},
		{
			recordType: "KEY OBJECT DOC",
			strings:    map[string]string{"InstanceNumber": "3", "ContentDate": "20260101", "ContentTime": "120000"},
			sequences:  append(codeSequence(), "ContentSequence"),
			want: []string{
				"ContentDate", "ContentTime", "InstanceNumber",
				"ConceptNameCodeSequence", "ContentSequence",
			},
		},
		{
			recordType: "SPECTROSCOPY",
			strings: map[string]string{
				"ContentDate": "20260101", "ContentTime": "120000", "InstanceNumber": "3",
			},
			multi:     map[string][]string{"ImageType": {"ORIGINAL", "PRIMARY"}},
			ints:      map[string]int{"NumberOfFrames": 1, "Rows": 2, "Columns": 3, "DataPointRows": 4, "DataPointColumns": 5},
			sequences: []string{"ReferencedImageEvidenceSequence"},
			want: []string{
				"ImageType", "ContentDate", "ContentTime", "InstanceNumber",
				"ReferencedImageEvidenceSequence", "NumberOfFrames", "Rows", "Columns",
				"DataPointRows", "DataPointColumns",
			},
		},
		{
			recordType: "HANGING PROTOCOL",
			strings: map[string]string{
				"HangingProtocolCreator": "Doe^John", "HangingProtocolCreationDateTime": "20260101120000",
			},
			ints:      map[string]int{"NumberOfPriorsReferenced": 0},
			sequences: []string{"HangingProtocolDefinitionSequence", "HangingProtocolUserIdentificationCodeSequence"},
			want: []string{
				"HangingProtocolCreator", "HangingProtocolCreationDateTime",
				"HangingProtocolDefinitionSequence", "NumberOfPriorsReferenced",
				"HangingProtocolUserIdentificationCodeSequence",
			},
		},
		{
			recordType: "ENCAP DOC",
			strings: map[string]string{
				"InstanceNumber": "3", "MIMETypeOfEncapsulatedDocument": "application/pdf",
				"ContentDate": "20260101", "ContentTime": "120000",
				"DocumentTitle": "Report", "HL7InstanceIdentifier": "1.2.3",
			},
			sequences: codeSequence(),
			want: []string{
				"ContentDate", "ContentTime", "InstanceNumber", "DocumentTitle",
				"HL7InstanceIdentifier", "ConceptNameCodeSequence", "MIMETypeOfEncapsulatedDocument",
			},
		},
		{
			recordType: "PALETTE",
			strings:    map[string]string{"ContentLabel": "PA1", "ContentDescription": "A palette"},
			want:       []string{"ContentLabel", "ContentDescription"},
		},
		{
			recordType: "IMPLANT",
			strings: map[string]string{
				"Manufacturer": "ACME", "ImplantName": "Screw", "ImplantPartNumber": "P1",
			},
			multi: map[string][]string{"ImplantSize": {"4", "12"}},
			want:  []string{"Manufacturer", "ImplantName", "ImplantSize", "ImplantPartNumber"},
		},
		{
			recordType: "IMPLANT ASSY",
			strings:    map[string]string{"ImplantAssemblyTemplateName": "Assy", "Manufacturer": "ACME"},
			sequences:  []string{"ProcedureTypeCodeSequence"},
			want:       []string{"ImplantAssemblyTemplateName", "Manufacturer", "ProcedureTypeCodeSequence"},
		},
		{
			recordType: "IMPLANT GROUP",
			strings:    map[string]string{"ImplantTemplateGroupName": "Grp", "ImplantTemplateGroupIssuer": "ACME"},
			want:       []string{"ImplantTemplateGroupName", "ImplantTemplateGroupIssuer"},
		},
		{
			recordType: "SURFACE SCAN",
			strings:    map[string]string{"ContentDate": "20260101", "ContentTime": "120000"},
			want:       []string{"ContentDate", "ContentTime"},
		},
		{
			recordType: "ASSESSMENT",
			strings: map[string]string{
				"InstanceNumber": "3", "InstanceCreationDate": "20260101", "InstanceCreationTime": "120000",
			},
			want: []string{"InstanceNumber", "InstanceCreationDate", "InstanceCreationTime"},
		},
		{
			recordType: "RADIOTHERAPY",
			strings: map[string]string{
				"InstanceNumber": "3", "UserContentLabel": "L", "UserContentLongLabel": "Long",
				"ContentDescription": "desc", "ContentCreatorName": "Doe^John",
			},
			want: []string{
				"InstanceNumber", "UserContentLabel", "UserContentLongLabel",
				"ContentDescription", "ContentCreatorName",
			},
		},
		{
			recordType: "WAVEFORM",
			strings:    map[string]string{"InstanceNumber": "3", "ContentDate": "20260101", "ContentTime": "120000"},
			want:       []string{"InstanceNumber", "ContentDate", "ContentTime"},
		},
		{
			recordType: "REGISTRATION",
			strings: map[string]string{
				"InstanceNumber": "3", "ContentDate": "20260101", "ContentTime": "120000", "ContentLabel": "RG1",
			},
			want: []string{"InstanceNumber", "ContentDate", "ContentTime", "ContentLabel"},
		},
		// The following share a recorder with one of the cases above. They are
		// listed separately because the record type is part of what the table
		// decides: a name wired to the wrong recorder would still produce a
		// valid record, just not the one PS3.3 asks for.
		{
			recordType: "RAW DATA",
			strings:    map[string]string{"InstanceNumber": "3", "ContentDate": "20260101", "ContentTime": "120000"},
			want:       []string{"InstanceNumber", "ContentDate", "ContentTime"},
		},
		{
			recordType: "FIDUCIAL",
			strings:    map[string]string{"InstanceNumber": "3", "ContentDate": "20260101", "ContentTime": "120000", "ContentLabel": "L1"},
			sequences:  codeSequence(),
			want:       []string{"InstanceNumber", "ContentDate", "ContentTime", "ContentLabel"},
		},
		{
			recordType: "MEASUREMENT",
			strings:    map[string]string{"InstanceNumber": "3", "ContentDate": "20260101", "ContentTime": "120000", "ContentLabel": "L1"},
			sequences:  codeSequence(),
			want:       []string{"InstanceNumber", "ContentDate", "ContentTime", "ContentLabel"},
		},
		{
			recordType: "SURFACE",
			strings:    map[string]string{"InstanceNumber": "3", "ContentDate": "20260101", "ContentTime": "120000", "ContentLabel": "L1"},
			sequences:  codeSequence(),
			want:       []string{"InstanceNumber", "ContentDate", "ContentTime", "ContentLabel"},
		},
		{
			recordType: "TRACT",
			strings:    map[string]string{"InstanceNumber": "3", "ContentDate": "20260101", "ContentTime": "120000", "ContentLabel": "L1"},
			sequences:  codeSequence(),
			want:       []string{"InstanceNumber", "ContentDate", "ContentTime", "ContentLabel"},
		},
		{
			recordType: "VALUE MAP",
			strings:    map[string]string{"InstanceNumber": "3", "ContentDate": "20260101", "ContentTime": "120000", "ContentLabel": "L1"},
			sequences:  codeSequence(),
			want:       []string{"InstanceNumber", "ContentDate", "ContentTime", "ContentLabel"},
		},
		{
			recordType: "PLAN",
			want:       nil,
		},
		{
			recordType: "STEREOMETRIC",
			want:       nil,
		},
	}

	covered := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.recordType, func(t *testing.T) {
			ds := NewDataset()
			setStrings(t, ds, tc.strings)
			for keyword, value := range tc.ints {
				if err := ds.SetInt(MustTag(keyword), value); err != nil {
					t.Fatalf("%s: %v", keyword, err)
				}
			}
			for keyword, values := range tc.multi {
				if err := ds.SetStrings(MustTag(keyword), values...); err != nil {
					t.Fatalf("%s: %v", keyword, err)
				}
			}
			for _, keyword := range tc.sequences {
				tg := MustTag(keyword)
				ds.Set(NewDataElement(tg, VRSQ, NewSequence([]*Dataset{NewDataset()})))
			}

			record, err := newDirectoryRecord(tc.recordType, ds)
			if err != nil {
				t.Fatalf("newDirectoryRecord(%q) error = %v", tc.recordType, err)
			}
			covered[tc.recordType] = true

			if got, _ := record.GetString(tag.DirectoryRecordType); got != tc.recordType {
				t.Errorf("DirectoryRecordType = %q, want %q", got, tc.recordType)
			}
			for _, keyword := range tc.want {
				if _, ok := record.Get(MustTag(keyword)); !ok {
					t.Errorf("record has no %s element", keyword)
				}
			}
		})
	}

	for recordType := range directoryRecorders {
		if !covered[recordType] {
			t.Errorf("record type %q has no case in this test", recordType)
		}
	}
}

// TestRecorderRefusesAnEmptySequenceValue covers the "present but empty" arm of
// checkRecordDataset for the sequence-valued Type 1 elements, which read as
// missing through the value rather than through the tag.
func TestRecorderRefusesAnEmptySequenceValue(t *testing.T) {
	ds := NewDataset()
	setStrings(t, ds, map[string]string{
		"InstanceNumber": "3", "ContentDate": "20260101", "ContentTime": "120000",
	})
	ds.Set(NewDataElement(tag.ConceptNameCodeSequence, VRSQ, NewSequence(nil)))

	_, err := newDirectoryRecord("KEY OBJECT DOC", ds)
	if err == nil {
		t.Fatal("expected the empty sequence to be refused")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("error = %v, want it to say the element cannot be empty", err)
	}
}
