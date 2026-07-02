package dicomjson

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
)

func buildPydicomRoundtripDataset() *godicom.Dataset {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0008, 0x0005), godicom.VRCS, "ISO_IR 100"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x0010), godicom.VRLO, "Creator 1.0"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1001), godicom.VRSH, "Version1"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1002), godicom.VROB, []byte("BinaryContent")))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1003), godicom.VROW, []byte{0x01, 0x02, 0x30, 0x40, 0x50, 0x60}))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1004), godicom.VROF, []byte{0, 1, 2, 3, 4, 5, 6, 7}))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1005), godicom.VROD, []byte{
		0, 1, 2, 3, 4, 5, 6, 7, 1, 1, 2, 3, 4, 5, 6, 7,
	}))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1006), godicom.VROL, []byte{
		0, 1, 2, 3, 4, 5, 6, 7, 1, 1, 2, 3,
	}))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1007), godicom.VRUI, "1.2.3.4.5.6"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1008), godicom.VRDA, "20200101"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1009), godicom.VRTM, "115500"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100A), godicom.VRDT, "20200101115500.000000"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100B), godicom.VRUL, uint32(3000000000)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100C), godicom.VRSL, int32(-2000000000)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100D), godicom.VRUS, uint16(40000)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100E), godicom.VRSS, int16(-22222)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x100F), godicom.VRFL, float32(3.14)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1010), godicom.VRFD, 3.14159265))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1011), godicom.VRCS, "TEST MODE"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1012), godicom.VRPN, godicom.ParsePersonName("CITIZEN^1")))
	ds.Set(godicom.NewDataElement(
		godicom.MustTag(0x0009, 0x1013),
		godicom.VRPN,
		godicom.ParsePersonName("Yamada^Tarou=山田^太郎=やまだ^たろう"),
	))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1014), godicom.VRIS, "42"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1015), godicom.VRDS, "3.14159265"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1016), godicom.VRAE, []byte("CONQUESTSRV1")))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1017), godicom.VRAS, "055Y"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1018), godicom.VRLT, strings.Repeat("Калинка,", 50)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1019), godicom.VRUC, "LONG CODE VALUE"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x101A), godicom.VRUN, []byte{0x01, 0x02, 0x30, 0x40, 0x50, 0x60}))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x101B), godicom.VRUR, "https://example.com"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x101C), godicom.VRAT, godicom.NewMultiValue([]godicom.Tag{
		godicom.MustTag("PatientName"),
		godicom.MustTag("PatientID"),
	})))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x101D), godicom.VRST, strings.Repeat("علي بابا", 100)))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x101E), godicom.VRSH, "Διονυσιος"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x0011), godicom.VRLO, "Creator 2.0"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1101), godicom.VRSH, "Version2"))
	ds.Set(godicom.NewDataElement(godicom.MustTag(0x0009, 0x1102), godicom.VRUS, uint16(2)))
	return ds
}

func TestPydicomRoundtripDataset(t *testing.T) {
	original := buildPydicomRoundtripDataset()

	model, err := DatasetToMap(original)
	if err != nil {
		t.Fatal(err)
	}

	assertJSONValue(t, model, "00080005", []interface{}{"ISO_IR 100"})
	assertJSONValue(t, model, "00091007", []interface{}{"1.2.3.4.5.6"})
	assertJSONValue(t, model, "0009100A", []interface{}{"20200101115500.000000"})
	assertJSONValue(t, model, "0009100B", []interface{}{float64(3000000000)})
	assertJSONValue(t, model, "0009100C", []interface{}{float64(-2000000000)})
	assertJSONValue(t, model, "0009100D", []interface{}{float64(40000)})
	assertJSONValue(t, model, "0009100F", []interface{}{3.14})
	assertJSONValue(t, model, "00091010", []interface{}{3.14159265})
	assertJSONValue(t, model, "00091018", []interface{}{strings.Repeat("Калинка,", 50)})

	data, err := MarshalDataset(original)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := assertDatasetsEqual(t, original, parsed); err != nil {
		t.Fatal(err)
	}

	parsed2, err := ParseDataset(data)
	if err != nil {
		t.Fatal(err)
	}
	model2, err := DatasetToMap(parsed2)
	if err != nil {
		t.Fatal(err)
	}
	if !jsonMapsEqual(model, model2) {
		t.Fatal("json model changed on second marshal path")
	}
}

func TestCTSmallJSONFullRoundtrip(t *testing.T) {
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
	if err := assertDatasetsEqual(t, ds.Dataset, parsed); err != nil {
		t.Fatal(err)
	}
}

func assertJSONValue(t *testing.T, model map[string]Element, key string, want []interface{}) {
	t.Helper()
	elem, ok := model[key]
	if !ok {
		t.Fatalf("missing key %s", key)
	}
	var got []interface{}
	if err := json.Unmarshal(wrapArray(elem.Value), &got); err != nil {
		t.Fatalf("key %s: %v", key, err)
	}
	if len(got) != len(want) {
		t.Fatalf("key %s: got %v want %v", key, got, want)
	}
	for i := range want {
		if !jsonValuesEqual(got[i], want[i]) {
			t.Fatalf("key %s[%d]: got %v (%T) want %v (%T)", key, i, got[i], got[i], want[i], want[i])
		}
	}
}

func jsonValuesEqual(a, b interface{}) bool {
	af, aok := a.(float64)
	bf, bok := b.(float64)
	if aok && bok {
		if af == bf {
			return true
		}
		// float32 roundtrip via JSON may differ slightly
		diff := af - bf
		if diff < 0 {
			diff = -diff
		}
		return diff < 1e-5
	}
	return a == b
}

func jsonMapsEqual(a, b map[string]Element) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || va.VR != vb.VR {
			return false
		}
		ja, _ := json.Marshal(va)
		jb, _ := json.Marshal(vb)
		if string(ja) != string(jb) {
			return false
		}
	}
	return true
}

func assertDatasetsEqual(t *testing.T, want, got *godicom.Dataset) error {
	t.Helper()
	wantTags := want.SortedTags()
	gotTags := got.SortedTags()
	if len(wantTags) != len(gotTags) {
		return &datasetDiff{msg: "tag count", want: len(wantTags), got: len(gotTags)}
	}
	for i, tag := range wantTags {
		if tag != gotTags[i] {
			return &datasetDiff{msg: "tag order", tag: tag, gotTag: gotTags[i]}
		}
		we, _ := want.Get(tag)
		ge, _ := got.Get(tag)
		if !elementsJSONEqual(we, ge) {
			t.Errorf("tag %s: want %#v got %#v", tag, we.Value, ge.Value)
			return &datasetDiff{msg: "value mismatch", tag: tag}
		}
	}
	return nil
}

type datasetDiff struct {
	msg    string
	tag    godicom.Tag
	gotTag godicom.Tag
	want   int
	got    int
}

func (e *datasetDiff) Error() string {
	if e.tag != 0 {
		return e.msg + " at " + e.tag.String()
	}
	return e.msg
}

func elementsJSONEqual(a, b *godicom.DataElement) bool {
	if a.Tag != b.Tag || a.VR != b.VR {
		return false
	}
	return valuesJSONEqual(a.VR, a.Value, b.Value)
}

func valuesJSONEqual(vr godicom.VR, a, b interface{}) bool {
	if vr == godicom.VRSQ {
		sa, oka := a.(*godicom.Sequence)
		sb, okb := b.(*godicom.Sequence)
		if !oka || !okb || sa.Len() != sb.Len() {
			return false
		}
		for i := 0; i < sa.Len(); i++ {
			if err := assertDatasetsEqualSilent(sa.Get(i), sb.Get(i)); err != nil {
				return false
			}
		}
		return true
	}

	av := normalizeJSONValue(vr, a)
	bv := normalizeJSONValue(vr, b)
	return jsonValueDeepEqual(av, bv)
}

func assertDatasetsEqualSilent(want, got *godicom.Dataset) error {
	wantTags := want.SortedTags()
	gotTags := got.SortedTags()
	if len(wantTags) != len(gotTags) {
		return &datasetDiff{msg: "tag count"}
	}
	for i, tag := range wantTags {
		if tag != gotTags[i] {
			return &datasetDiff{msg: "tag order"}
		}
		we, _ := want.Get(tag)
		ge, _ := got.Get(tag)
		if !elementsJSONEqual(we, ge) {
			return &datasetDiff{msg: "value mismatch", tag: tag}
		}
	}
	return nil
}

func normalizeJSONValue(vr godicom.VR, value interface{}) interface{} {
	if value == nil {
		return nil
	}
	switch vr {
	case godicom.VRPN:
		if pn, ok := value.(godicom.PersonName); ok {
			return pn.String()
		}
	case godicom.VRDA:
		if da, ok := value.(godicom.DA); ok {
			return da.String()
		}
	case godicom.VRTM:
		if tm, ok := value.(godicom.TM); ok {
			return tm.String()
		}
	case godicom.VRDT:
		if dt, ok := value.(godicom.DT); ok {
			return dt.String()
		}
	case godicom.VRAT:
		return tagsToStrings(value)
	case godicom.VRIS, godicom.VRDS:
		if f, ok := toFloat64(value); ok {
			return f
		}
	}
	if b, ok := value.([]byte); ok {
		return string(b)
	}
	return value
}

func tagsToStrings(value interface{}) []string {
	switch v := value.(type) {
	case godicom.Tag:
		return []string{v.JSONKey()}
	case *godicom.MultiValue[godicom.Tag]:
		out := make([]string, 0, v.Len())
		for _, tag := range v.Values() {
			out = append(out, tag.JSONKey())
		}
		return out
	case *godicom.MultiValue[interface{}]:
		out := make([]string, 0, v.Len())
		for _, item := range v.Values() {
			if tag, ok := item.(godicom.Tag); ok {
				out = append(out, tag.JSONKey())
			}
		}
		return out
	}
	return nil
}

func toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case int16:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case string:
		if v == "" {
			return 0, false
		}
		f, err := json.Number(v).Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func jsonValueDeepEqual(a, b interface{}) bool {
	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)
	if aok && bok {
		diff := af - bf
		if diff < 0 {
			diff = -diff
		}
		return diff < 1e-5
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
