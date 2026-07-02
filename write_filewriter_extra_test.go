package godicom

import (
	"bytes"
	"testing"
	"time"
)

func TestWriteElementEmptyAT(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteDataElement.test_empty_AT
	elem := NewDataElement(MustTag(0x00280009), VRAT, NewMultiValue([]Tag{}))
	got := encodeElementImplicitLittle(elem)
	want := []byte{0x28, 0x00, 0x09, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("got = % X, want % X", got, want)
	}
}

func TestWriteElementEmptyLO(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_empty_LO
	elem := NewDataElement(MustTag(0x00080070), VRLO, nil)
	got := encodeElementImplicitLittle(elem)
	want := []byte{0x08, 0x00, 0x70, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("got = % X, want % X", got, want)
	}
}

func TestWriteElementDA(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_DA
	elem := NewDataElement(MustTag(0x00080022), VRDA, "20000101")
	want := []byte{0x08, 0x00, 0x22, 0x00, 0x08, 0x00, 0x00, 0x00, '2', '0', '0', '0', '0', '1', '0', '1'}
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("string: got = % X, want % X", got, want)
	}

	da, err := ParseDA("20000101")
	if err != nil {
		t.Fatal(err)
	}
	elem = NewDataElement(MustTag(0x00080022), VRDA, da)
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("DA type: got = % X, want % X", got, want)
	}
}

func TestWriteElementMultiDA(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_multi_DA
	elem := NewDataElement(MustTag(0x0014407E), VRDA, []string{"20100101", "20101231"})
	want := []byte{
		0x14, 0x00, 0x7e, 0x40,
		0x12, 0x00, 0x00, 0x00,
		'2', '0', '1', '0', '0', '1', '0', '1', '\\',
		'2', '0', '1', '0', '1', '2', '3', '1', ' ',
	}
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("[]string: got = % X, want % X", got, want)
	}

	da1, _ := ParseDA("20100101")
	da2, _ := ParseDA("20101231")
	elem = NewDataElement(MustTag(0x0014407E), VRDA, []DA{da1, da2})
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("[]DA: got = % X, want % X", got, want)
	}
}

func TestWriteElementTM(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_TM
	elem := NewDataElement(MustTag(0x00080030), VRTM, "010203")
	want := []byte{0x08, 0x00, 0x30, 0x00, 0x06, 0x00, 0x00, 0x00, '0', '1', '0', '2', '0', '3'}
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("string: got = % X, want % X", got, want)
	}

	elem = NewDataElement(MustTag(0x00080030), VRTM, []byte("010203"))
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("[]byte: got = % X, want % X", got, want)
	}

	tm, err := ParseTM("010203")
	if err != nil {
		t.Fatal(err)
	}
	elem = NewDataElement(MustTag(0x00080030), VRTM, tm)
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("TM type: got = % X, want % X", got, want)
	}
}

func TestWriteElementMultiTM(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_multi_TM
	elem := NewDataElement(MustTag(0x0014407C), VRTM, []string{"082500", "092655"})
	want := []byte{
		0x14, 0x00, 0x7c, 0x40,
		0x0e, 0x00, 0x00, 0x00,
		'0', '8', '2', '5', '0', '0', '\\', '0', '9', '2', '6', '5', '5', ' ',
	}
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("[]string: got = % X, want % X", got, want)
	}

	tm1, _ := ParseTM("082500")
	tm2, _ := ParseTM("092655")
	elem = NewDataElement(MustTag(0x0014407C), VRTM, []TM{tm1, tm2})
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("[]TM: got = % X, want % X", got, want)
	}
}

func TestWriteElementDT(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_DT
	elem := NewDataElement(MustTag(0x0008002A), VRDT, "20170101120000")
	want := []byte{
		0x08, 0x00, 0x2a, 0x00,
		0x0e, 0x00, 0x00, 0x00,
		'2', '0', '1', '7', '0', '1', '0', '1', '1', '2', '0', '0', '0', '0',
	}
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("string: got = % X, want % X", got, want)
	}

	elem = NewDataElement(MustTag(0x0008002A), VRDT, []byte("20170101120000"))
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("[]byte: got = % X, want % X", got, want)
	}

	dt, err := ParseDT("20170101120000")
	if err != nil {
		t.Fatal(err)
	}
	elem = NewDataElement(MustTag(0x0008002A), VRDT, dt)
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("DT type: got = % X, want % X", got, want)
	}
}

func TestWriteElementMultiDT(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteDataElement.test_write_multi_DT
	elem := NewDataElement(MustTag(0x0040A13A), VRDT, []string{"20120820120804", "20130901111111"})
	want := []byte{
		0x40, 0x00, 0x3a, 0xa1,
		0x1e, 0x00, 0x00, 0x00,
		'2', '0', '1', '2', '0', '8', '2', '0', '1', '2', '0', '8', '0', '4', '\\',
		'2', '0', '1', '3', '0', '9', '0', '1', '1', '1', '1', '1', '1', '1', ' ',
	}
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("[]string: got = % X, want % X", got, want)
	}

	elem = NewDataElement(MustTag(0x0040A13A), VRDT, "20120820120804\\20130901111111")
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("backslash string: got = % X, want % X", got, want)
	}

	dt1, _ := ParseDT("20120820120804")
	dt2, _ := ParseDT("20130901111111")
	elem = NewDataElement(MustTag(0x0040A13A), VRDT, []DT{dt1, dt2})
	if got := encodeElementImplicitLittle(elem); !bytes.Equal(got, want) {
		t.Fatalf("[]DT: got = % X, want % X", got, want)
	}
}

func TestWriteEmptySequence(t *testing.T) {
	// pydicom.tests.test_filewriter.TestWriteFile.test_write_empty_sequence
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.2")))
	ds.Set(NewDataElement(MustTag("PerformedProcedureCodeSequence"), VRSQ, NewSequence(nil)))

	outPath := t.TempDir() + "/empty_seq.dcm"
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	seq, ok := reread.GetSequence(MustTag("PerformedProcedureCodeSequence"))
	if !ok {
		t.Fatal("PerformedProcedureCodeSequence missing")
	}
	if !seq.IsEmpty() {
		t.Fatalf("sequence len = %d, want 0", seq.Len())
	}
}

func TestCorrectAmbiguousVREmptyValue(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_empty_value
	ds := NewDataset()
	elem := NewDataElement(MustTag(0x00280106), VRUsSS, nil)
	ds.Set(elem)

	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	got, ok := ds.Get(MustTag(0x00280106))
	if !ok || got.VR != VRUS {
		t.Fatalf("VR = %s, want US", got.VR)
	}
	if got.Value != nil {
		t.Fatalf("value = %#v, want nil", got.Value)
	}

	ds.Set(NewDataElement(MustTag(0x00283002), VRUS, []int{1, 1, 1}))
	lut := NewDataElement(MustTag(0x00283006), VRUsSS, nil)
	ds.Set(lut)
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	got, ok = ds.Get(MustTag(0x00283006))
	if !ok || got.VR != VRUS {
		t.Fatalf("LUTData VR = %s, want US", got.VR)
	}
}

func TestCorrectAmbiguousVREmptyLUTData(t *testing.T) {
	// pydicom.tests.test_filewriter.TestCorrectAmbiguousVR.test_empty_lut_data
	item := NewDataset()
	item.Set(NewDataElement(MustTag(0x00283002), VRUsSS, nil))
	item.Set(NewDataElement(MustTag(0x00283006), VRUsOw, nil))
	seq := NewSequence([]*Dataset{item})
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("ModalityLUTSequence"), VRSQ, seq))

	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		t.Fatal(err)
	}
	lutItem := seq.Get(0)
	lutData, ok := lutItem.Get(MustTag(0x00283006))
	if !ok || lutData.VR != VROW {
		t.Fatalf("LUTData VR = %s, want OW", lutData.VR)
	}

	ds.Set(NewDataElement(MustTag("SOPClassUID"), VRUI, CTImageStorage))
	ds.Set(NewDataElement(MustTag("SOPInstanceUID"), VRUI, UID("1.2.826.0.1.3680043.8.498.1")))

	outPath := t.TempDir() + "/lut.dcm"
	if err := ds.SaveAs(outPath, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}
	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	rseq, ok := reread.GetSequence(MustTag("ModalityLUTSequence"))
	if !ok || rseq.Len() != 1 {
		t.Fatal("ModalityLUTSequence missing")
	}
	ritem := rseq.Get(0)
	desc, ok := ritem.Get(MustTag(0x00283002))
	if !ok || desc.VR != VRUS {
		t.Fatalf("LUTDescriptor VR = %s, want US", desc.VR)
	}
	data, ok := ritem.Get(MustTag(0x00283006))
	if !ok || data.VR != VROW {
		t.Fatalf("LUTData VR = %s, want OW", data.VR)
	}
}

func TestWriteDateTimeRoundtrip(t *testing.T) {
	// pydicom.tests.test_filewriter.TestScratchWriteDateTime.test_multivalue_DA
	ds, err := ReadFile(testFilePath("MR_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}

	da1, _ := ParseDA("19610804")
	da2, _ := ParseDA("19631122")
	ds.Set(NewDataElement(MustTag("CalibrationDate"), VRDA, NewMultiValue([]DA{da1, da2})))
	ds.Set(NewDataElement(MustTag("DateOfLastCalibration"), VRDA, da1))

	dt1, _ := ParseDT("19610804")
	dt2, _ := ParseDT("19631122123000-0600")
	ds.Set(NewDataElement(MustTag("ReferencedDateTime"), VRDT, NewMultiValue([]DT{dt1, dt2})))

	tm1, _ := ParseTM("012345")
	tm2, _ := ParseTM("111111")
	ds.Set(NewDataElement(MustTag("CalibrationTime"), VRTM, NewMultiValue([]TM{tm1, tm2})))
	tm3, _ := ParseTM("111111.1")
	ds.Set(NewDataElement(MustTag("TimeOfLastCalibration"), VRTM, tm3))

	outPath := t.TempDir() + "/datetime.dcm"
	if err := ds.SaveAs(outPath, nil); err != nil {
		t.Fatal(err)
	}
	reread, err := ReadFile(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	calElem, ok := reread.Get(MustTag("CalibrationDate"))
	if !ok {
		t.Fatal("CalibrationDate missing")
	}
	mv, ok := calElem.Value.(*MultiValue[DA])
	if !ok || mv.Len() != 2 {
		t.Fatalf("CalibrationDate = %#v, want *MultiValue[DA] len 2", calElem.Value)
	}
	dates := mv.Values()
	if dates[0].Time != time.Date(1961, 8, 4, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("CalibrationDate[0] = %v", dates[0].Time)
	}
	if dates[1].Time != time.Date(1963, 11, 22, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("CalibrationDate[1] = %v", dates[1].Time)
	}

	lastCal, ok := reread.GetDA(MustTag("DateOfLastCalibration"))
	if !ok || lastCal.Time != time.Date(1961, 8, 4, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("DateOfLastCalibration = %v, %t", lastCal, ok)
	}
}
