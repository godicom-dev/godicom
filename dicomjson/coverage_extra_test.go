package dicomjson

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
)

// putTag writes a tag the way an implicit-VR little-endian dataset stores one:
// group then element, each little endian.
func putTag(dst []byte, tag godicom.Tag) {
	v := uint32(tag)
	binary.LittleEndian.PutUint16(dst[0:2], uint16(v>>16))
	binary.LittleEndian.PutUint16(dst[2:4], uint16(v&0xFFFF))
}

// implicitElement encodes one implicit-VR little-endian element, which is the
// shape a UN value is expected to hold when it carries a dataset.
func implicitElement(tag godicom.Tag, value []byte) []byte {
	out := make([]byte, 8, 8+len(value))
	putTag(out, tag)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(value)))
	return append(out, value...)
}

func undefinedLengthPrefix(tag godicom.Tag) []byte {
	out := make([]byte, 8)
	putTag(out, tag)
	binary.LittleEndian.PutUint32(out[4:8], 0xFFFFFFFF)
	return out
}

func zeroLengthTag(tag godicom.Tag) []byte {
	out := make([]byte, 8)
	putTag(out, tag)
	return out
}

func inlineBinaryJSON(t *testing.T, tag string, vr string, value []byte) []byte {
	t.Helper()
	doc := `{"` + tag + `":{"vr":"` + vr + `","InlineBinary":"` + base64.StdEncoding.EncodeToString(value) + `"}}`
	return []byte(doc)
}

func TestParseDatasetsArrayAndSingle(t *testing.T) {
	array := []byte(`[
		{"00100010":{"vr":"PN","Value":[{"Alphabetic":"CITIZEN^Jan"}]}},
		{"00100010":{"vr":"PN","Value":[{"Alphabetic":"CITIZEN^Feb"}]}}
	]`)
	datasets, err := ParseDatasets(array)
	if err != nil {
		t.Fatal(err)
	}
	if len(datasets) != 2 {
		t.Fatalf("got %d datasets", len(datasets))
	}
	if name, ok := datasets[1].GetString(godicom.MustTag("PatientName")); !ok || name != "CITIZEN^Feb" {
		t.Fatalf("second dataset name = %q", name)
	}

	// A single object is accepted, because QIDO-RS sometimes returns one.
	single, err := ParseDatasets([]byte(`{"00100010":{"vr":"PN","Value":[{"Alphabetic":"X"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(single) != 1 {
		t.Fatalf("single object gave %d datasets", len(single))
	}

	if _, err := ParseDatasets([]byte("   ")); err == nil {
		t.Fatal("expected empty-input error")
	}
	// "zzzz" is not a hex key; "bad" would be, since B, A and D are hex digits.
	if _, err := ParseDatasets([]byte(`[{"zzzz":{}}]`)); err == nil {
		t.Fatal("expected parse error")
	} else if !strings.Contains(err.Error(), "dataset[0]") {
		t.Fatalf("error should name the failing index: %v", err)
	}
	if _, err := ParseDatasets([]byte(`["not an object"]`)); err == nil {
		t.Fatal("expected error for a non-object element")
	}
}

func TestMarshalDatasets(t *testing.T) {
	first := godicom.NewDataset()
	first.Set(godicom.NewDataElement(godicom.MustTag("PatientID"), godicom.VRLO, "A"))
	second := godicom.NewDataset()
	second.Set(godicom.NewDataElement(godicom.MustTag("PatientID"), godicom.VRLO, "B"))

	out, err := MarshalDatasets([]*godicom.Dataset{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "[") {
		t.Fatalf("expected a JSON array, got %s", out)
	}
	back, err := ParseDatasets(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 {
		t.Fatalf("got %d datasets", len(back))
	}
	if id, _ := back[1].GetString(godicom.MustTag("PatientID")); id != "B" {
		t.Fatalf("second id = %q", id)
	}

	if _, err := MarshalDatasets(nil); err != nil {
		t.Fatalf("empty slice: %v", err)
	}
}

func TestParseInlineBinaryUNResolvesKnownVR(t *testing.T) {
	// An inline binary declared UN but belonging to a tag whose VR is known has
	// to be decoded as that VR, not handed back as opaque bytes.
	rows := make([]byte, 2)
	binary.LittleEndian.PutUint16(rows, 512)
	ds, err := ParseDataset(inlineBinaryJSON(t, "00280010", "UN", rows))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := ds.GetInt(godicom.MustTag("Rows")); !ok || v != 512 {
		t.Fatalf("Rows = %d %v", v, ok)
	}

	// (0010,9431) Examined Body Thickness is FL, so the UN value is four bytes.
	fl := make([]byte, 4)
	binary.LittleEndian.PutUint32(fl, 0x3FC00000) // 1.5
	ds, err = ParseDataset(inlineBinaryJSON(t, "00109431", "UN", fl))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := ds.GetFloat(godicom.MustTag("ExaminedBodyThickness")); !ok || v != 1.5 {
		t.Fatalf("ExaminedBodyThickness = %v %v", v, ok)
	}
}

func TestParseInlineBinaryUNUnknownTagStaysBytes(t *testing.T) {
	// A private data element has no dictionary VR, so there is nothing to decode
	// it as. (gggg,0010) would not do: that block is always a Private Creator and
	// resolves to LO whatever the dictionary says.
	raw := []byte{1, 2, 3, 4}
	ds, err := ParseDataset(inlineBinaryJSON(t, "00091001", "UN", raw))
	if err != nil {
		t.Fatal(err)
	}
	elem, ok := ds.Get(godicom.MustTag("00091001"))
	if !ok {
		t.Fatal("element missing")
	}
	got, ok := elem.Value.([]byte)
	if !ok || len(got) != 4 {
		t.Fatalf("value = %T %v", elem.Value, elem.Value)
	}
}

func TestParseInlineBinaryUNSequenceWithUndefinedLengths(t *testing.T) {
	// Both the sequence and its item carry undefined lengths, which is what a
	// converted implicit-VR dataset looks like before it is written out.
	value := undefinedLengthPrefix(godicom.ItemTag)
	value = append(value, implicitElement(godicom.MustTag("CodeValue"), []byte("ABC"))...)
	value = append(value, zeroLengthTag(godicom.ItemDelimiterTag)...)
	value = append(value, zeroLengthTag(godicom.SequenceDelimiterTag)...)

	ds, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", value))
	if err != nil {
		t.Fatal(err)
	}
	seq, ok := ds.GetSequence(godicom.MustTag("ReferencedSeriesSequence"))
	if !ok || seq.Len() != 1 {
		t.Fatalf("sequence = %v %v", seq, ok)
	}
	if v, ok := seq.Get(0).GetString(godicom.MustTag("CodeValue")); !ok || v != "ABC" {
		t.Fatalf("item value = %q", v)
	}
}

func TestParseInlineBinaryUNSequenceDefinedLength(t *testing.T) {
	item := implicitElement(godicom.MustTag("CodeValue"), []byte("XYZ"))
	value := make([]byte, 8)
	putTag(value, godicom.ItemTag)
	binary.LittleEndian.PutUint32(value[4:8], uint32(len(item)))
	value = append(value, item...)
	value = append(value, zeroLengthTag(godicom.SequenceDelimiterTag)...)

	ds, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", value))
	if err != nil {
		t.Fatal(err)
	}
	seq, ok := ds.GetSequence(godicom.MustTag("ReferencedSeriesSequence"))
	if !ok || seq.Len() != 1 {
		t.Fatalf("sequence = %v %v", seq, ok)
	}
	if v, _ := seq.Get(0).GetString(godicom.MustTag("CodeValue")); v != "XYZ" {
		t.Fatalf("item value = %q", v)
	}
}

func TestParseInlineBinaryUNSequenceNestedDefinedLength(t *testing.T) {
	// A sequence nested inside an item parses as long as both lengths are known.
	// Nested *undefined* lengths are not supported: the item scan looks for the
	// next Item Delimiter in the raw bytes and cannot tell the inner one from the
	// outer, which is why that shape is left to the binary reader.
	codeElement := implicitElement(godicom.MustTag("CodeValue"), []byte("IN"))
	innerItem := make([]byte, 8)
	putTag(innerItem, godicom.ItemTag)
	binary.LittleEndian.PutUint32(innerItem[4:8], uint32(len(codeElement)))
	innerItem = append(innerItem, codeElement...)

	inner := make([]byte, 8)
	putTag(inner, godicom.MustTag("ReferencedSeriesSequence"))
	binary.LittleEndian.PutUint32(inner[4:8], uint32(len(innerItem)))
	inner = append(inner, innerItem...)

	value := make([]byte, 8)
	putTag(value, godicom.ItemTag)
	binary.LittleEndian.PutUint32(value[4:8], uint32(len(inner)))
	value = append(value, inner...)
	value = append(value, zeroLengthTag(godicom.SequenceDelimiterTag)...)

	ds, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", value))
	if err != nil {
		t.Fatal(err)
	}
	outer, ok := ds.GetSequence(godicom.MustTag("ReferencedSeriesSequence"))
	if !ok || outer.Len() != 1 {
		t.Fatalf("outer sequence = %v %v", outer, ok)
	}
	nested, ok := outer.Get(0).GetSequence(godicom.MustTag("ReferencedSeriesSequence"))
	if !ok || nested.Len() != 1 {
		t.Fatalf("inner sequence = %v %v", nested, ok)
	}
	if v, _ := nested.Get(0).GetString(godicom.MustTag("CodeValue")); v != "IN" {
		t.Fatalf("nested value = %q", v)
	}
}

func TestParseInlineBinaryUNSequenceErrors(t *testing.T) {
	// Item tag expected.
	bad := implicitElement(godicom.MustTag("CodeValue"), []byte("x"))
	if _, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", bad)); err == nil {
		t.Fatal("expected item-tag error")
	}

	// Item length past the end of the value.
	over := make([]byte, 8)
	binary.LittleEndian.PutUint32(over[0:4], uint32(godicom.ItemTag))
	binary.LittleEndian.PutUint32(over[4:8], 4096)
	if _, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", over)); err == nil {
		t.Fatal("expected oversized item error")
	}

	// Unterminated undefined-length item.
	if _, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", undefinedLengthPrefix(godicom.ItemTag))); err == nil {
		t.Fatal("expected unterminated item error")
	}

	// Element length past the end of the item.
	huge := make([]byte, 8)
	binary.LittleEndian.PutUint32(huge[0:4], uint32(godicom.MustTag("CodeValue")))
	binary.LittleEndian.PutUint32(huge[4:8], 4096)
	if _, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", huge)); err == nil {
		t.Fatal("expected oversized element error")
	}

	// Undefined length on a non-SQ element cannot be skipped.
	undefinedText := undefinedLengthPrefix(godicom.MustTag("CodeValue"))
	if _, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", undefinedText)); err == nil {
		t.Fatal("expected unsupported undefined length error")
	}
}

func TestParseBinaryIntsAndFloats(t *testing.T) {
	us := make([]byte, 4)
	binary.LittleEndian.PutUint16(us[0:2], 1)
	binary.LittleEndian.PutUint16(us[2:4], 2)
	v, err := parseBinaryInts(us, godicom.VRUS)
	if err != nil {
		t.Fatal(err)
	}
	mv, ok := v.(*godicom.MultiValue[interface{}])
	if !ok || mv.Len() != 2 || mv.Get(1) != int64(2) {
		t.Fatalf("US multi = %T %v", v, v)
	}

	ss := make([]byte, 2)
	binary.LittleEndian.PutUint16(ss, uint16(0xFFFE))
	if v, err = parseBinaryInts(ss, godicom.VRSS); err != nil || v != int64(-2) {
		t.Fatalf("SS = %v %v", v, err)
	}
	ul := make([]byte, 4)
	binary.LittleEndian.PutUint32(ul, 70000)
	if v, err = parseBinaryInts(ul, godicom.VRUL); err != nil || v != int64(70000) {
		t.Fatalf("UL = %v %v", v, err)
	}
	sl := make([]byte, 4)
	binary.LittleEndian.PutUint32(sl, uint32(0xFFFFFFFE))
	if v, err = parseBinaryInts(sl, godicom.VRSL); err != nil || v != int64(-2) {
		t.Fatalf("SL = %v %v", v, err)
	}
	if _, err := parseBinaryInts([]byte{1}, godicom.VRUS); err == nil {
		t.Fatal("expected odd US length error")
	}
	if _, err := parseBinaryInts([]byte{1, 2}, godicom.VRDS); err == nil {
		t.Fatal("expected unsupported integer VR error")
	}

	fl := make([]byte, 4)
	binary.LittleEndian.PutUint32(fl, 0x3FC00000)
	if v, err = parseBinaryFloats(fl, godicom.VRFL); err != nil || v != float64(1.5) {
		t.Fatalf("FL = %v %v", v, err)
	}
	fd := make([]byte, 8)
	binary.LittleEndian.PutUint64(fd, 0x4004000000000000) // 2.5
	if v, err = parseBinaryFloats(fd, godicom.VRFD); err != nil || v != 2.5 {
		t.Fatalf("FD = %v %v", v, err)
	}
	if _, err := parseBinaryFloats([]byte{1}, godicom.VRFL); err == nil {
		t.Fatal("expected invalid float length error")
	}
	if _, err := parseBinaryFloats([]byte{1, 2, 3, 4}, godicom.VRUS); err == nil {
		t.Fatal("expected unsupported float VR error")
	}
}

func TestParseJSONNumericStrings(t *testing.T) {
	// The JSON model writes numbers as numbers, but string spellings are legal
	// and appear in the wild, including the empty string for an empty value.
	ds, err := ParseDataset([]byte(`{"00280010":{"vr":"US","Value":["512"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := ds.GetInt(godicom.MustTag("Rows")); !ok || v != 512 {
		t.Fatalf("Rows = %d %v", v, ok)
	}

	ds, err = ParseDataset([]byte(`{"00281052":{"vr":"FL","Value":["1.5"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := ds.GetFloat(godicom.MustTag("RescaleIntercept")); !ok || v != 1.5 {
		t.Fatalf("RescaleIntercept = %v %v", v, ok)
	}

	if _, err := ParseDataset([]byte(`{"00280010":{"vr":"US","Value":["not a number"]}}`)); err == nil {
		t.Fatal("expected integer parse error")
	}

	// Empty spellings parse to an element with no value rather than failing.
	ds, err = ParseDataset([]byte(`{"00280010":{"vr":"US","Value":[""]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ds.GetInt(godicom.MustTag("Rows")); ok {
		t.Fatal("empty integer string should not produce a value")
	}
}

func TestParseElementValueKeyErrors(t *testing.T) {
	if _, err := ParseDataset([]byte(`{"zzzz":{"vr":"LO","Value":["x"]}}`)); err == nil {
		t.Fatal("expected invalid tag key error")
	}
	if _, err := ParseDataset([]byte(`{"00100010":{"vr":"PN","Value":[],"InlineBinary":"AA=="}}`)); err == nil {
		t.Fatal("expected multiple value keys error")
	}
	if _, err := ParseDataset([]byte(`{"7FE00010":{"vr":"OB","InlineBinary":"!!!"}}`)); err == nil {
		t.Fatal("expected base64 error")
	}
	if _, err := ParseDataset([]byte(`{"7FE00010":{"vr":"OB","InlineBinary":123}}`)); err == nil {
		t.Fatal("expected InlineBinary type error")
	}
	if _, err := ParseDataset([]byte(`{"00100010":{"vr":"PN","Value":"not a list"}}`)); err == nil {
		t.Fatal("expected Value-not-a-list error")
	}
	if _, err := ParseDataset([]byte(`{"00081115":{"vr":"SQ","Value":["not an object"]}}`)); err == nil {
		t.Fatal("expected SQ item error")
	}
	if _, err := ParseDataset([]byte(`{"00200052":{"vr":"AT","Value":[42]}}`)); err == nil {
		t.Fatal("expected AT type error")
	}
}

func TestMarshalValueVariants(t *testing.T) {
	ds := godicom.NewDataset()
	set := func(tg string, vr godicom.VR, v interface{}) {
		ds.Set(godicom.NewDataElement(godicom.MustTag(tg), vr, v))
	}
	set("00080020", godicom.VRDA, godicom.DA{})
	set("00080030", godicom.VRTM, "120000")
	set("00080018", godicom.VRUI, []byte("1.2.3"))
	set("00280010", godicom.VRUS, uint16(512))
	set("00280011", godicom.VRUS, godicom.NewMultiValue([]uint16{1, 2}))
	set("00281050", godicom.VRDS, "")
	set("00281051", godicom.VRDS, "1.5")
	set("00200052", godicom.VRAT, godicom.MustTag("PatientName"))
	set("00080008", godicom.VRCS, godicom.NewMultiValue([]interface{}{"ORIGINAL", "PRIMARY"}))

	out, err := MarshalDatasetString(ds)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"00280010":{"vr":"US","Value":[512]}`, `"00281051":{"vr":"DS","Value":[1.5]}`, `"00200052":{"vr":"AT","Value":["00100010"]}`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s in %s", want, out)
		}
	}
	// An empty DS string is an empty value, not the string "".
	if !strings.Contains(out, `"00281050":{"vr":"DS"}`) {
		t.Fatalf("empty DS marshaled as %s", out)
	}
}

func TestMarshalBinaryValueUsesRawValue(t *testing.T) {
	// A deferred element keeps its bytes in RawValue with no decoded Value; the
	// JSON model still has to emit them.
	elem := godicom.NewDataElement(godicom.MustTag("PixelData"), godicom.VROB, nil)
	elem.RawValue = []byte{1, 2, 3}
	ds := godicom.NewDataset()
	ds.Set(elem)
	out, err := MarshalDatasetString(ds)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, base64.StdEncoding.EncodeToString([]byte{1, 2, 3})) {
		t.Fatalf("got %s", out)
	}
}

func TestMarshalEmptyBinaryOmitsValue(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag("PixelData"), godicom.VROB, []byte{}))
	out, err := MarshalDatasetString(ds)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "InlineBinary") || strings.Contains(out, "Value") {
		t.Fatalf("empty binary should carry no value: %s", out)
	}
}

func TestMarshalSequenceShapes(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag("ReferencedSeriesSequence"), godicom.VRSQ, godicom.NewSequence(nil)))
	out, err := MarshalDatasetString(ds)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"00081115":{"vr":"SQ"}`) {
		t.Fatalf("empty sequence marshaled as %s", out)
	}

	item := godicom.NewDataset()
	item.Set(godicom.NewDataElement(godicom.MustTag("SeriesInstanceUID"), godicom.VRUI, "1.2.3"))
	ds.Set(godicom.NewDataElement(godicom.MustTag("ReferencedSeriesSequence"), godicom.VRSQ,
		godicom.NewSequence([]*godicom.Dataset{item})))
	out, err = MarshalDatasetString(ds)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1.2.3") {
		t.Fatalf("sequence item lost: %s", out)
	}
}

func TestMarshalSuppressesUnmarshalableElement(t *testing.T) {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.MustTag("PatientID"), godicom.VRLO, "KEEP"))
	ds.Set(godicom.NewDataElement(godicom.MustTag("ReferencedSOPClassUID"), godicom.VRAT, "not a tag"))
	if _, err := MarshalDataset(ds); err == nil {
		t.Fatal("expected AT error without the suppress option")
	}
	out, err := MarshalDatasetString(ds, WithSuppressInvalidTags())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "KEEP") || strings.Contains(out, "not a tag") {
		t.Fatalf("got %s", out)
	}
}
