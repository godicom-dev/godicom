package dicomjson

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
)

func TestPersonNameComponentsToJSON(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(
		godicom.MustTag("PatientName"),
		godicom.VRPN,
		godicom.ParsePersonName("Yamada^Tarou=山田^太郎=やまだ^たろう"),
	))
	ds.Set(godicom.NewDataElement(
		godicom.MustTag(0x0009, 0x1001),
		godicom.VRPN,
		godicom.ParsePersonName("Yamada^Tarou"),
	))
	ds.Set(godicom.NewDataElement(
		godicom.MustTag(0x0009, 0x1003),
		godicom.VRPN,
		godicom.ParsePersonName("=山田^太郎=やまだ^たろう"),
	))

	model, err := DatasetToMap(ds)
	if err != nil {
		t.Fatal(err)
	}

	var patientName []map[string]string
	if err := json.Unmarshal(model["00100010"].Value[0], &patientName); err == nil && len(patientName) > 0 {
		t.Fatalf("unexpected nested PN shape: %#v", patientName)
	}
	var comps map[string]string
	if err := json.Unmarshal(model["00100010"].Value[0], &comps); err != nil {
		t.Fatal(err)
	}
	if comps["Alphabetic"] != "Yamada^Tarou" || comps["Ideographic"] != "山田^太郎" || comps["Phonetic"] != "やまだ^たろう" {
		t.Fatalf("PN components = %#v", comps)
	}

	comps = nil
	if err := json.Unmarshal(model["00091001"].Value[0], &comps); err != nil {
		t.Fatal(err)
	}
	if comps["Alphabetic"] != "Yamada^Tarou" {
		t.Fatalf("PN alphabetic components = %#v", comps)
	}
	if _, ok := comps["Ideographic"]; ok {
		t.Fatalf("PN alphabetic should omit Ideographic: %#v", comps)
	}
	if _, ok := comps["Phonetic"]; ok {
		t.Fatalf("PN alphabetic should omit Phonetic: %#v", comps)
	}

	comps = nil
	if err := json.Unmarshal(model["00091003"].Value[0], &comps); err != nil {
		t.Fatal(err)
	}
	if _, ok := comps["Alphabetic"]; ok {
		t.Fatalf("PN missing alphabetic should omit Alphabetic: %#v", comps)
	}
	if comps["Ideographic"] != "山田^太郎" || comps["Phonetic"] != "やまだ^たろう" {
		t.Fatalf("PN missing alphabetic components = %#v", comps)
	}
}

func TestPersonNameComponentsFromJSON(t *testing.T) {
	data := []byte(`{
		"00100010":{"vr":"PN","Value":[{"Alphabetic":"Yamada^Tarou","Ideographic":"山田^太郎","Phonetic":"やまだ^たろう"}]},
		"00091001":{"vr":"PN","Value":[{"Alphabetic":"Yamada^Tarou"}]},
		"00091003":{"vr":"PN","Value":[{"Ideographic":"山田^太郎","Phonetic":"やまだ^たろう"}]}
	}`)

	ds, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}
	pn, ok := ds.GetString(godicom.MustTag("PatientName"))
	if !ok || pn != "Yamada^Tarou=山田^太郎=やまだ^たろう" {
		t.Fatalf("PatientName = %q, %t", pn, ok)
	}
	pn, ok = ds.GetString(godicom.MustTag(0x0009, 0x1001))
	if !ok || pn != "Yamada^Tarou" {
		t.Fatalf("00091001 = %q, %t", pn, ok)
	}
	pn, ok = ds.GetString(godicom.MustTag(0x0009, 0x1003))
	if !ok || pn != "=山田^太郎=やまだ^たろう" {
		t.Fatalf("00091003 = %q, %t", pn, ok)
	}
}

func TestATToAndFromJSON(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1001), godicom.VRAT, godicom.NewMultiValue([]godicom.Tag{
		godicom.MustTag("PatientName"),
		godicom.MustTag("PatientID"),
	})))

	model, err := DatasetToMap(ds)
	if err != nil {
		t.Fatal(err)
	}
	var values []string
	if err := json.Unmarshal(wrapArray(model["00091001"].Value), &values); err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != "00100010" || values[1] != "00100020" {
		t.Fatalf("AT JSON = %#v", values)
	}

	parsed, err := ParseDataset([]byte(`{"00091001":{"vr":"AT","Value":["00100010","00100020"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := parsed.Get(godicom.MustTag(0x0009, 0x1001))
	if !ok {
		t.Fatal("AT element missing")
	}
	mv, ok := elem.Value.(*godicom.MultiValue[interface{}])
	if !ok || mv.Len() != 2 {
		t.Fatalf("AT value type = %T", elem.Value)
	}
	if mv.Get(0) != godicom.MustTag("PatientName") || mv.Get(1) != godicom.MustTag("PatientID") {
		t.Fatalf("AT values = %#v", mv.Values())
	}
}

func TestInlineBinaryAndBulkDataURI(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1002), godicom.VROB, []byte("BinaryContent")))

	model, err := DatasetToMap(ds)
	if err != nil {
		t.Fatal(err)
	}
	if got := model["00091002"].InlineBinary; got != "QmluYXJ5Q29udGVudA==" {
		t.Fatalf("InlineBinary = %q", got)
	}

	parsed, err := ParseDataset([]byte(`{"00091002":{"vr":"OB","InlineBinary":["QmluYXJ5Q29udGVudA=="]}}`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := parsed.GetBytes(godicom.MustTag(0x0009, 0x1002))
	if !ok || !bytes.Equal(got, []byte("BinaryContent")) {
		t.Fatalf("InlineBinary parsed = %q, %t", got, ok)
	}

	parsed, err = ParseDataset(
		[]byte(`{"00091002":{"vr":"OB","BulkDataURI":"https://example.com/bulk"}}`),
		WithBulkDataURIReader(func(tag godicom.Tag, vr godicom.VR, uri string) ([]byte, error) {
			if tag != godicom.MustTag(0x0009, 0x1002) || vr != godicom.VROB || uri != "https://example.com/bulk" {
				t.Fatalf("bulk callback args = %s %s %q", tag, vr, uri)
			}
			return []byte("xyzzy"), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	got, ok = parsed.GetBytes(godicom.MustTag(0x0009, 0x1002))
	if !ok || !bytes.Equal(got, []byte("xyzzy")) {
		t.Fatalf("BulkDataURI parsed = %q, %t", got, ok)
	}
}

func TestSequenceAndNumericRoundtrip(t *testing.T) {
	item := godicom.NewDataset()
	item.Set(godicom.NewDataElement(godicom.MustTag("PatientPosition"), godicom.VRCS, "HFS"))
	item.Set(godicom.NewDataElement(godicom.MustTag("PatientSetupNumber"), godicom.VRIS, 1))

	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(
		godicom.MustTag(0x003A, 0x0200),
		godicom.VRSQ,
		godicom.NewSequence([]*godicom.Dataset{item}),
	))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100B), godicom.VRUL, uint32(3000000000)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100F), godicom.VRFL, 3.14))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1014), godicom.VRIS, "42"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1015), godicom.VRDS, "3.14159265"))

	data, err := MarshalDataset(ds)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}

	seq, ok := parsed.GetSequence(godicom.MustTag(0x003A, 0x0200))
	if !ok || seq.Len() != 1 {
		t.Fatalf("sequence missing: %v %t", seq, ok)
	}
	position, ok := seq.Get(0).GetString(godicom.MustTag("PatientPosition"))
	if !ok || position != "HFS" {
		t.Fatalf("PatientPosition = %q, %t", position, ok)
	}
	setupNumber, ok := seq.Get(0).GetInt(godicom.MustTag("PatientSetupNumber"))
	if !ok || setupNumber != 1 {
		t.Fatalf("PatientSetupNumber = %d, %t", setupNumber, ok)
	}
	if value, ok := parsed.GetInt(godicom.MustTag(0x0009, 0x1014)); !ok || value != 42 {
		t.Fatalf("IS value = %d, %t", value, ok)
	}
	if value, ok := parsed.GetFloat(godicom.MustTag(0x0009, 0x1015)); !ok || value != 3.14159265 {
		t.Fatalf("DS value = %g, %t", value, ok)
	}
}

func TestDecodeDataset(t *testing.T) {
	ds, err := DecodeDataset(bytes.NewReader([]byte(`{"00100010":{"vr":"PN","Value":[{"Alphabetic":"Jane^Doe"}]}}`)))
	if err != nil {
		t.Fatal(err)
	}
	name, ok := ds.GetString(godicom.MustTag("PatientName"))
	if !ok || name != "Jane^Doe" {
		t.Fatalf("PatientName = %q, %t", name, ok)
	}
}

func TestPydicomPersonNameJSONFile(t *testing.T) {
	data, err := os.ReadFile(testDataFile("test_PN.json"))
	if err != nil {
		t.Fatal(err)
	}
	ds, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}
	name, ok := ds.GetString(godicom.MustTag("PatientName"))
	if !ok || name != "Prostate^Volunteer" {
		t.Fatalf("PatientName = %q, %t", name, ok)
	}
	seq, ok := ds.GetSequence(godicom.MustTag(0x0400, 0x0561))
	if !ok || seq.Len() != 1 {
		t.Fatalf("outer sequence missing: %v %t", seq, ok)
	}
	inner, ok := seq.Get(0).GetSequence(godicom.MustTag(0x0400, 0x0550))
	if !ok || inner.Len() != 1 {
		t.Fatalf("inner sequence missing: %v %t", inner, ok)
	}
	innerName, ok := inner.Get(0).GetString(godicom.MustTag("PatientName"))
	if !ok || innerName != "" {
		t.Fatalf("inner PatientName = %q, %t", innerName, ok)
	}
}

func TestDICOMFileJSONRoundtripValues(t *testing.T) {
	ds, err := godicom.ReadFile(testDataFile("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalDataset(ds.Dataset)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := parsed.GetInt(godicom.MustTag("Rows"))
	if !ok || rows != 128 {
		t.Fatalf("Rows = %d, %t", rows, ok)
	}
	cols, ok := parsed.GetInt(godicom.MustTag("Columns"))
	if !ok || cols != 128 {
		t.Fatalf("Columns = %d, %t", cols, ok)
	}
	pixel, ok := parsed.GetBytes(godicom.MustTag("PixelData"))
	if !ok || len(pixel) != 32768 {
		t.Fatalf("PixelData length = %d, %t", len(pixel), ok)
	}
}

func TestBulkDataURIBoundaries(t *testing.T) {
	parsed, err := ParseDataset([]byte(`{"00091002":{"vr":"OB","BulkDataURI":"https://example.com/bulk"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if elem, ok := parsed.Get(godicom.MustTag(0x0009, 0x1002)); !ok || elem.Value != nil {
		t.Fatalf("BulkDataURI without reader = %#v, %t", elem, ok)
	}

	for _, data := range [][]byte{
		[]byte(`{"00091002":{"vr":"OB","BulkDataURI":42}}`),
		[]byte(`{"00091002":{"vr":"OB","BulkDataURI":[42]}}`),
	} {
		if _, err := ParseDataset(data); err == nil {
			t.Fatalf("expected BulkDataURI type error for %s", data)
		}
	}

	for _, data := range [][]byte{
		[]byte(`{"00091002":{"vr":"OB","InlineBinary":42}}`),
		[]byte(`{"00091002":{"vr":"OB","InlineBinary":[42]}}`),
	} {
		if _, err := ParseDataset(data); err == nil {
			t.Fatalf("expected InlineBinary type error for %s", data)
		}
	}
}

func TestBulkDataURIWithinSequence(t *testing.T) {
	parsed, err := ParseDataset(
		[]byte(`{"003A0200":{"vr":"SQ","Value":[{"54001010":{"vr":"OW","BulkDataURI":"https://example.com/waveform"}}]}}`),
		WithBulkDataURIReader(func(tag godicom.Tag, vr godicom.VR, uri string) ([]byte, error) {
			if tag != godicom.MustTag(0x5400, 0x1010) || vr != godicom.VROW || uri != "https://example.com/waveform" {
				t.Fatalf("bulk callback args = %s %s %q", tag, vr, uri)
			}
			return []byte("xyzzy"), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	seq, ok := parsed.GetSequence(godicom.MustTag(0x003A, 0x0200))
	if !ok || seq.Len() != 1 {
		t.Fatalf("waveform sequence missing: %v %t", seq, ok)
	}
	got, ok := seq.Get(0).GetBytes(godicom.MustTag(0x5400, 0x1010))
	if !ok || !bytes.Equal(got, []byte("xyzzy")) {
		t.Fatalf("nested BulkDataURI = %q, %t", got, ok)
	}
}

func TestBulkDataURIBuilderThreshold(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1002), godicom.VROB, []byte("BinaryContent")))

	model, err := DatasetToMap(
		ds,
		WithBulkDataThreshold(4),
		WithBulkDataURIBuilder(func(tag godicom.Tag, vr godicom.VR, value []byte) (string, error) {
			if tag != godicom.MustTag(0x0009, 0x1002) || vr != godicom.VROB || !bytes.Equal(value, []byte("BinaryContent")) {
				t.Fatalf("bulk builder args = %s %s %q", tag, vr, value)
			}
			return "https://example.com/bulk", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := model["00091002"].BulkDataURI; got != "https://example.com/bulk" {
		t.Fatalf("BulkDataURI = %q", got)
	}
	if got := model["00091002"].InlineBinary; got != "" {
		t.Fatalf("InlineBinary should be omitted, got %q", got)
	}
}

func TestEmptyJSONValues(t *testing.T) {
	parsed, err := ParseDataset([]byte(`{
		"00091000":{"vr":"CS","Value":[""]},
		"00091001":{"vr":"CS","Value":[null]},
		"00091002":{"vr":"LO","Value":[""]},
		"00091003":{"vr":"LO","Value":[null]},
		"00091006":{"vr":"UI","Value":[""]},
		"00091007":{"vr":"UI","Value":[null]},
		"00091008":{"vr":"DA","Value":[""]},
		"00091009":{"vr":"DA","Value":[null]},
		"00091020":{"vr":"DS","Value":[""]},
		"00091021":{"vr":"DS","Value":[null]},
		"00091022":{"vr":"US","Value":[""]},
		"00091023":{"vr":"US","Value":[null]},
		"00091024":{"vr":"FL","Value":[""]},
		"00091025":{"vr":"FL","Value":[null]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []godicom.Tag{
		godicom.MustTag(0x0009, 0x1000),
		godicom.MustTag(0x0009, 0x1001),
		godicom.MustTag(0x0009, 0x1002),
		godicom.MustTag(0x0009, 0x1003),
		godicom.MustTag(0x0009, 0x1006),
		godicom.MustTag(0x0009, 0x1007),
		godicom.MustTag(0x0009, 0x1008),
		godicom.MustTag(0x0009, 0x1009),
	} {
		value, ok := parsed.GetString(tag)
		if !ok || value != "" {
			t.Fatalf("string empty value for %s = %q, %t", tag, value, ok)
		}
	}
	for _, tag := range []godicom.Tag{
		godicom.MustTag(0x0009, 0x1020),
		godicom.MustTag(0x0009, 0x1021),
		godicom.MustTag(0x0009, 0x1022),
		godicom.MustTag(0x0009, 0x1023),
		godicom.MustTag(0x0009, 0x1024),
		godicom.MustTag(0x0009, 0x1025),
	} {
		elem, ok := parsed.Get(tag)
		if !ok || elem.Value != nil {
			t.Fatalf("numeric empty value for %s = %#v, %t", tag, elem.Value, ok)
		}
	}
}

func TestSuppressInvalidTags(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag("PatientName"), godicom.VRPN, godicom.ParsePersonName("Jane^Doe")))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1001), godicom.VRAT, "not a tag"))

	if _, err := DatasetToMap(ds); err == nil {
		t.Fatal("expected AT marshal error")
	}
	model, err := DatasetToMap(ds, WithSuppressInvalidTags())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := model["00091001"]; ok {
		t.Fatal("invalid AT tag should be suppressed")
	}
	if _, ok := model["00100010"]; !ok {
		t.Fatal("valid PatientName should remain")
	}
}

func TestPydicomTest1JSONFixture(t *testing.T) {
	data, err := os.ReadFile(testDataFile("test1.json"))
	if err != nil {
		t.Fatal(err)
	}
	ds, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}
	matrixElem, ok := ds.Get(godicom.MustTag(0x0018, 0x1310))
	if !ok {
		t.Fatal("AcquisitionMatrix missing")
	}
	matrix, ok := matrixElem.Value.(*godicom.MultiValue[interface{}])
	if !ok || matrix.Len() != 5 || matrix.Get(0) != int64(128) || matrix.Get(4) != nil {
		t.Fatalf("AcquisitionMatrix = %#v", matrixElem.Value)
	}
	seq, ok := ds.GetSequence(godicom.MustTag(0x0012, 0x0064))
	if !ok || seq.Len() == 0 {
		t.Fatalf("DeidentificationMethodCodeSequence missing: %v %t", seq, ok)
	}
	pixel, ok := ds.Get(godicom.MustTag("PixelData"))
	if !ok || pixel.Value != nil {
		t.Fatalf("PixelData from empty BulkDataURI = %#v, %t", pixel, ok)
	}
}

func TestInlineBinaryUNSequence(t *testing.T) {
	data := []byte(`{"300A0180":{"vr":"UN","InlineBinary":"/v8A4B4AAAAYAABRBAAAAEhGUyAKMIIBAgAAADEgCjCyAQAAAAA="}}`)
	parsed, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}
	seq, ok := parsed.GetSequence(godicom.MustTag("PatientSetupSequence"))
	if !ok || seq.Len() != 1 {
		t.Fatalf("PatientSetupSequence missing: %v %t", seq, ok)
	}
	position, ok := seq.Get(0).GetString(godicom.MustTag("PatientPosition"))
	if !ok || position != "HFS" {
		t.Fatalf("PatientPosition = %q, %t", position, ok)
	}
	setupNumber, ok := seq.Get(0).GetInt(godicom.MustTag("PatientSetupNumber"))
	if !ok || setupNumber != 1 {
		t.Fatalf("PatientSetupNumber = %d, %t", setupNumber, ok)
	}
	desc, ok := seq.Get(0).GetString(godicom.MustTag("SetupTechniqueDescription"))
	if !ok || desc != "" {
		t.Fatalf("SetupTechniqueDescription = %q, %t", desc, ok)
	}
}

func TestInlineBinaryUNKnownVRPadding(t *testing.T) {
	parsed, err := ParseDataset([]byte(`{"00185100":{"vr":"UN","InlineBinary":"SEZTIA=="}}`))
	if err != nil {
		t.Fatal(err)
	}
	position, ok := parsed.GetString(godicom.MustTag("PatientPosition"))
	if !ok || position != "HFS" {
		t.Fatalf("PatientPosition = %q, %t", position, ok)
	}
	elem, ok := parsed.Get(godicom.MustTag("PatientPosition"))
	if !ok || elem.VR != godicom.VRCS {
		t.Fatalf("PatientPosition VR = %s, %t", elem.VR, ok)
	}
}

func TestMultiValueAndEmptyPersonName(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(
		godicom.MustTag(0x0009, 0x1001),
		godicom.VRPN,
		godicom.NewMultiValue([]interface{}{
			godicom.ParsePersonName("Buc^Jérôme"),
			godicom.ParsePersonName("Διονυσιος"),
			godicom.ParsePersonName("Люкceмбypг"),
		}),
	))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1002), godicom.VRPN, godicom.PersonName{}))

	model, err := DatasetToMap(ds)
	if err != nil {
		t.Fatal(err)
	}
	var names []map[string]string
	if err := json.Unmarshal(wrapArray(model["00091001"].Value), &names); err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 || names[0]["Alphabetic"] != "Buc^Jérôme" || names[2]["Alphabetic"] != "Люкceмбypг" {
		t.Fatalf("multi-value PN = %#v", names)
	}
	if len(model["00091002"].Value) != 0 {
		t.Fatal("empty PN should omit Value")
	}
}

func TestMarshalDatasetStringSortOrder(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag("PatientSex"), godicom.VRCS, "F"))
	ds.Set(godicom.NewDataElement(godicom.MustTag("PatientBirthDate"), godicom.VRDA, "20000101"))
	ds.Set(godicom.NewDataElement(godicom.MustTag("PatientID"), godicom.VRLO, "0017"))
	ds.Set(godicom.NewDataElement(godicom.MustTag("PatientName"), godicom.VRPN, godicom.ParsePersonName("Jane^Doe")))

	jsonText, err := MarshalDatasetString(ds)
	if err != nil {
		t.Fatal(err)
	}
	nameIndex := strings.Index(jsonText, "00100010")
	idIndex := strings.Index(jsonText, "00100020")
	birthDateIndex := strings.Index(jsonText, "00100030")
	sexIndex := strings.Index(jsonText, "00100040")
	if !(nameIndex < idIndex && idIndex < birthDateIndex && birthDateIndex < sexIndex) {
		t.Fatalf("JSON keys not sorted: %s", jsonText)
	}
}

func TestInvalidTagAndDuplicateValueKeys(t *testing.T) {
	if _, err := ParseDataset([]byte(`{"000910AG":{"vr":"AT","Value":["00091000"]}}`)); err == nil {
		t.Fatal("expected invalid JSON tag error")
	}
	if _, err := ParseDataset([]byte(`{"00091002":{"vr":"OB","Value":[],"InlineBinary":"AA=="}}`)); err == nil {
		t.Fatal("expected duplicate value key error")
	}
}

func testDataFile(name string) string {
	return filepath.Join("..", "pydicom", "src", "pydicom", "data", "test_files", name)
}

func wrapArray(values []json.RawMessage) []byte {
	data, _ := json.Marshal(values)
	return data
}
