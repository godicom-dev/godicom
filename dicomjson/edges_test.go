package dicomjson

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/godicom-dev/godicom"
)

func TestPersonNameComponentRoundtrip(t *testing.T) {
	cases := []struct {
		tag  godicom.Tag
		pn   godicom.PersonName
		want string
	}{
		{godicom.MustTag("PatientName"), godicom.ParsePersonName("Yamada^Tarou=山田^太郎=やまだ^たろう"), "Yamada^Tarou=山田^太郎=やまだ^たろう"},
		{godicom.MustTag(0x0009, 0x1001), godicom.ParsePersonName("Yamada^Tarou"), "Yamada^Tarou"},
		{godicom.MustTag(0x0009, 0x1002), godicom.ParsePersonName("Yamada^Tarou=="), "Yamada^Tarou"},
		{godicom.MustTag(0x0009, 0x1003), godicom.ParsePersonName("=山田^太郎=やまだ^たろう"), "=山田^太郎=やまだ^たろう"},
		{godicom.MustTag(0x0009, 0x1004), godicom.ParsePersonName("Yamada^Tarou==やまだ^たろう"), "Yamada^Tarou==やまだ^たろう"},
		{godicom.MustTag(0x0009, 0x1005), godicom.ParsePersonName("==やまだ^たろう"), "==やまだ^たろう"},
		{godicom.MustTag(0x0009, 0x1006), godicom.ParsePersonName("=山田^太郎"), "=山田^太郎"},
		{godicom.MustTag(0x0009, 0x1007), godicom.ParsePersonName("Yamada^Tarou=山田^太郎"), "Yamada^Tarou=山田^太郎"},
	}

	ds := godicom.NewDataset()
	for _, tc := range cases {
		ds.Set(godicom.NewDataElement(tc.tag, godicom.VRPN, tc.pn))
	}
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1008), godicom.VRPN, godicom.PersonName{}))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1009), godicom.VRPN, godicom.ParsePersonName("")))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1010), godicom.VRPN, godicom.NewMultiValue([]godicom.PersonName{})))

	model, err := DatasetToMap(ds)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"00091008", "00091009", "00091010"} {
		if len(model[key].Value) != 0 {
			t.Fatalf("empty PN %s should omit Value", key)
		}
	}

	data, err := MarshalDataset(ds)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		got, ok := parsed.GetString(tc.tag)
		if !ok || got != tc.want {
			t.Fatalf("%s = %q, want %q", tc.tag, got, tc.want)
		}
		elem, _ := parsed.Get(tc.tag)
		if _, ok := elem.Value.(godicom.PersonName); !ok {
			t.Fatalf("%s value type = %T, want PersonName", tc.tag, elem.Value)
		}
	}
}

func TestPersonNameFromJSONExplicitEmptyComponents(t *testing.T) {
	data := []byte(`{
		"00100010":{"vr":"PN","Value":[{"Alphabetic":"Yamada^Tarou","Ideographic":"山田^太郎","Phonetic":"やまだ^たろう"}]},
		"00091001":{"vr":"PN","Value":[{"Alphabetic":"Yamada^Tarou"}]},
		"00091002":{"vr":"PN","Value":[{"Alphabetic":"Yamada^Tarou","Ideographic":"","Phonetic":""}]},
		"00091003":{"vr":"PN","Value":[{"Ideographic":"山田^太郎","Phonetic":"やまだ^たろう"}]},
		"00091004":{"vr":"PN","Value":[{"Alphabetic":"Yamada^Tarou","Phonetic":"やまだ^たろう"}]},
		"00091005":{"vr":"PN","Value":[{"Phonetic":"やまだ^たろう"}]},
		"00091006":{"vr":"PN","Value":[{"Ideographic":"山田^太郎"}]},
		"00091007":{"vr":"PN","Value":[{"Alphabetic":"Yamada^Tarou","Ideographic":"山田^太郎"}]}
	}`)
	parsed, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}
	want := map[godicom.Tag]string{
		godicom.MustTag("PatientName"):  "Yamada^Tarou=山田^太郎=やまだ^たろう",
		godicom.MustTag(0x0009, 0x1001): "Yamada^Tarou",
		godicom.MustTag(0x0009, 0x1002): "Yamada^Tarou",
		godicom.MustTag(0x0009, 0x1003): "=山田^太郎=やまだ^たろう",
		godicom.MustTag(0x0009, 0x1004): "Yamada^Tarou==やまだ^たろう",
		godicom.MustTag(0x0009, 0x1005): "==やまだ^たろう",
		godicom.MustTag(0x0009, 0x1006): "=山田^太郎",
		godicom.MustTag(0x0009, 0x1007): "Yamada^Tarou=山田^太郎",
	}
	for tag, expected := range want {
		got, ok := parsed.GetString(tag)
		if !ok || got != expected {
			t.Fatalf("%s = %q, want %q", tag, got, expected)
		}
	}
}

func TestATMarshalEmptyAndSingle(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1001), godicom.VRAT, godicom.NewMultiValue([]godicom.Tag{
		godicom.MustTag("PatientName"),
		godicom.MustTag("PatientID"),
	})))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1002), godicom.VRAT, godicom.MustTag(0x0028, 0x0002)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1003), godicom.VRAT, godicom.Tag(0x00280002)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1004), godicom.VRAT, godicom.NewMultiValue([]godicom.Tag{
		godicom.Tag(0x00280002),
		godicom.MustTag("PatientName"),
	})))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1005), godicom.VRAT, nil))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1006), godicom.VRAT, ""))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1007), godicom.VRAT, godicom.NewMultiValue([]godicom.Tag{})))

	model, err := DatasetToMap(ds)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONValue(t, model, "00091001", []interface{}{"00100010", "00100020"})
	assertJSONValue(t, model, "00091002", []interface{}{"00280002"})
	assertJSONValue(t, model, "00091003", []interface{}{"00280002"})
	assertJSONValue(t, model, "00091004", []interface{}{"00280002", "00100010"})
	for _, key := range []string{"00091005", "00091006", "00091007"} {
		if len(model[key].Value) != 0 {
			t.Fatalf("%s should omit Value", key)
		}
	}

	parsed, err := ParseDataset([]byte(`{"00091001":{"vr":"AT","Value":["000910AF"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := parsed.Get(godicom.MustTag(0x0009, 0x1001))
	if !ok {
		t.Fatal("missing element")
	}
	tag, ok := elem.Value.(godicom.Tag)
	if !ok || tag != godicom.Tag(0x000910AF) {
		t.Fatalf("value = %#v", elem.Value)
	}
}

func TestATInvalidValueIgnored(t *testing.T) {
	parsed, err := ParseDataset([]byte(`{
		"00091001":{"vr":"AT","Value":["000910AG"]},
		"00091002":{"vr":"AT","Value":["00100010"]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := parsed.Get(godicom.MustTag(0x0009, 0x1001))
	if !ok || elem.Value != nil {
		t.Fatalf("invalid AT should be nil, got %#v", elem.Value)
	}
	valid, ok := parsed.Get(godicom.MustTag(0x0009, 0x1002))
	if !ok {
		t.Fatal("missing valid AT")
	}
	if v, ok := valid.Value.(godicom.Tag); !ok || v != godicom.MustTag("PatientName") {
		t.Fatalf("valid AT = %#v", valid.Value)
	}
}

func TestNumericMarshalAndEmptyUS(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100B), godicom.VRUL, uint32(3000000000)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100C), godicom.VRSL, int32(-2000000000)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100D), godicom.VRUS, uint16(40000)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100E), godicom.VRSS, int16(-22222)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100F), godicom.VRFL, float32(3.14)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1010), godicom.VRFD, 3.14159265))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1014), godicom.VRIS, "42"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1015), godicom.VRDS, "3.14159265"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1102), godicom.VRUS, uint16(2)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1103), godicom.VRUS, nil))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1104), godicom.VRUS, uint16(0)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1105), godicom.VRUS, godicom.NewMultiValue([]uint16{})))

	model, err := DatasetToMap(ds)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONValue(t, model, "0009100B", []interface{}{float64(3000000000)})
	assertJSONValue(t, model, "0009100C", []interface{}{float64(-2000000000)})
	assertJSONValue(t, model, "0009100D", []interface{}{float64(40000)})
	assertJSONValue(t, model, "0009100E", []interface{}{float64(-22222)})
	assertJSONValue(t, model, "0009100F", []interface{}{3.14})
	assertJSONValue(t, model, "00091010", []interface{}{3.14159265})
	assertJSONValue(t, model, "00091014", []interface{}{float64(42)})
	assertJSONValue(t, model, "00091015", []interface{}{3.14159265})
	assertJSONValue(t, model, "00091102", []interface{}{float64(2)})
	assertJSONValue(t, model, "00091104", []interface{}{float64(0)})
	for _, key := range []string{"00091103", "00091105"} {
		if len(model[key].Value) != 0 {
			t.Fatalf("%s should omit Value", key)
		}
	}

	var raw map[string]map[string]interface{}
	data, err := MarshalDataset(ds)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"0009100B", "0009100C", "0009100D", "0009100E", "00091014", "00091102"} {
		v := raw[key]["Value"].([]interface{})[0]
		if _, ok := v.(float64); !ok {
			t.Fatalf("%s value type = %T, want number in JSON", key, v)
		}
	}
	for _, key := range []string{"0009100F", "00091010", "00091015"} {
		v := raw[key]["Value"].([]interface{})[0]
		if _, ok := v.(float64); !ok {
			t.Fatalf("%s value type = %T, want float64", key, v)
		}
	}
}

func TestNestedSequenceRoundtripWithoutPixelData(t *testing.T) {
	data, err := os.ReadFile(testDataFile("test1.json"))
	if err != nil {
		t.Fatal(err)
	}
	ds, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}
	ds.Delete(godicom.MustTag("PixelData"))

	wantMatrix, ok := ds.Get(godicom.MustTag(0x0018, 0x1310))
	if !ok {
		t.Fatal("AcquisitionMatrix missing")
	}
	mv, ok := wantMatrix.Value.(*godicom.MultiValue[interface{}])
	if !ok || mv.Len() != 5 {
		t.Fatalf("AcquisitionMatrix = %#v", wantMatrix.Value)
	}

	jsonData, err := MarshalDataset(ds)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDataset(jsonData)
	if err != nil {
		t.Fatal(err)
	}
	if err := assertDatasetsEqual(t, ds, parsed); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyPersonNameMarshal(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag("PatientName"), godicom.VRPN, godicom.ParsePersonName("")))
	model, err := DatasetToMap(ds)
	if err != nil {
		t.Fatal(err)
	}
	if len(model["00100010"].Value) != 0 {
		t.Fatal("empty PN should omit Value")
	}
}

func TestEmptyJSONIncludesLT(t *testing.T) {
	parsed, err := ParseDataset([]byte(`{
		"00091004":{"vr":"LT","Value":[""]},
		"00091005":{"vr":"LT","Value":[null]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []godicom.Tag{
		godicom.MustTag(0x0009, 0x1004),
		godicom.MustTag(0x0009, 0x1005),
	} {
		got, ok := parsed.GetString(tag)
		if !ok || got != "" {
			t.Fatalf("%s = %q, want empty string", tag, got)
		}
	}
}

func TestInlineBinaryRoundtrip(t *testing.T) {
	original := godicom.NewDataset()
	original.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1002), godicom.VROB, []byte("BinaryContent")))

	model, err := DatasetToMap(original)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDataset(mustJSON(t, model))
	if err != nil {
		t.Fatal(err)
	}
	if err := assertDatasetsEqual(t, original, parsed); err != nil {
		t.Fatal(err)
	}

	model["00091002"] = Element{VR: "OB", InlineBinary: "QmluYXJ5Q29udGVudA=="}
	parsed, err = ParseDataset(mustJSON(t, model))
	if err != nil {
		t.Fatal(err)
	}
	if err := assertDatasetsEqual(t, original, parsed); err != nil {
		t.Fatal(err)
	}
}

func TestBulkDataURIReaderArity(t *testing.T) {
	jsonData := []byte(`{"00091002":{"vr":"OB","BulkDataURI":"https://a.dummy.url"}}`)
	parsed, err := ParseDataset(jsonData, WithBulkDataURIReader(func(tag godicom.Tag, vr godicom.VR, uri string) ([]byte, error) {
		if uri != "https://a.dummy.url" {
			t.Fatalf("uri = %q", uri)
		}
		return []byte("xyzzy"), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := parsed.GetBytes(godicom.MustTag(0x0009, 0x1002))
	if !ok || !bytes.Equal(got, []byte("xyzzy")) {
		t.Fatalf("value = %q, %t", got, ok)
	}
}

func mustJSON(t *testing.T, model map[string]Element) []byte {
	t.Helper()
	data, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
