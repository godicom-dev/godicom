package godicom

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

func TestISEqual(t *testing.T) {
	if !(IS{Value: 3, Original: "3"}).Equal(IS{Value: 3, Original: "3.0"}) {
		t.Fatal("IS equality should compare the numeric value, not the spelling")
	}
	if (IS{Value: 3}).Equal(IS{Value: 4}) {
		t.Fatal("different values compared equal")
	}
}

func TestDTEqual(t *testing.T) {
	if !(DT{}).Equal(DT{}) {
		t.Fatal("two absent DTs should be equal")
	}
	a := DT{Time: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)}
	if !a.Equal(a) {
		t.Fatal("a DT should equal itself")
	}
	b := DT{Time: a.Time.Add(time.Second)}
	if a.Equal(b) {
		t.Fatal("different instants compared equal")
	}
}

func TestSetDSSetISAndGetters(t *testing.T) {
	ds := NewDataset()
	if err := ds.SetDS(tag.PatientWeight, DS{Value: 70.5, Original: "70.5"}); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetIS(tag.NumberOfFrames, IS{Value: 3, Original: "3"}); err != nil {
		t.Fatal(err)
	}
	weight, ok := ds.GetDS(tag.PatientWeight)
	if !ok || weight.Value != 70.5 {
		t.Fatalf("GetDS = %v %v", weight, ok)
	}
	frames, ok := ds.GetIS(tag.NumberOfFrames)
	if !ok || frames.Value != 3 || frames.String() != "3" {
		t.Fatalf("GetIS = %+v %v", frames, ok)
	}
	if _, ok := ds.GetIS(tag.PatientWeight); ok {
		t.Fatal("GetIS on a DS element should not report a value")
	}
	if _, ok := ds.GetDS(tag.NumberOfFrames); ok {
		t.Fatal("GetDS on an IS element should not report a value")
	}
}

func TestGetFloatsAcrossValueTypes(t *testing.T) {
	ds := NewDataset()
	// The renderer converts whatever the parser stored; each spelling has to
	// reach the same float.
	ds.Set(NewDataElement(tag.PatientWeight, VRDS, DS{Value: 70.5, Original: "70.5"}))
	if v, ok := ds.GetFloat(tag.PatientWeight); !ok || v != 70.5 {
		t.Fatalf("GetFloat(DS) = %v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.PatientWeight, VRDS, "70.5"))
	if v, ok := ds.GetFloat(tag.PatientWeight); !ok || v != 70.5 {
		t.Fatalf("GetFloat(string) = %v %v", v, ok)
	}
	ds.Set(NewDataElement(tag.PatientWeight, VRDS, int64(70)))
	if _, ok := ds.GetFloat(tag.PatientWeight); ok {
		t.Fatal("a bare int64 is not a DS spelling GetFloats converts")
	}
	ds.Set(NewDataElement(tag.PatientWeight, VRDS, "not a number"))
	if _, ok := ds.GetFloat(tag.PatientWeight); ok {
		t.Fatal("invalid DS string should not convert")
	}

	ds.Set(NewDataElement(tag.WindowCenter, VRDS, NewMultiValue([]interface{}{
		"1.5", 2, int64(3), DS{Value: 4.5},
	})))
	out, ok := ds.GetFloats(tag.WindowCenter)
	if !ok || len(out) != 4 || out[0] != 1.5 || out[3] != 4.5 {
		t.Fatalf("GetFloats = %v %v", out, ok)
	}
	ds.Set(NewDataElement(tag.WindowCenter, VRDS, NewMultiValue([]interface{}{"bad"})))
	if _, ok := ds.GetFloats(tag.WindowCenter); ok {
		t.Fatal("a bad element in the list should fail the whole conversion")
	}
}

func TestFileDatasetEncodeFile(t *testing.T) {
	fd := &FileDataset{Dataset: NewDataset(), FileMeta: NewFileMetaDataset()}
	fd.Set(NewDataElement(tag.PatientName, VRPN, "CITIZEN^Jan"))
	fd.FileMeta.Set(NewDataElement(tag.TransferSyntaxUID, VRUI, string(uid.ExplicitVRLittleEndian)))
	fd.FileMeta.Set(NewDataElement(tag.MediaStorageSOPClassUID, VRUI, string(uid.CTImageStorage)))
	fd.FileMeta.Set(NewDataElement(tag.MediaStorageSOPInstanceUID, VRUI, "1.2.3"))
	b, err := fd.EncodeFile(&WriteOptions{EnforceFileFormat: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 132 || string(b[128:132]) != "DICM" {
		t.Fatalf("encoded output is not a Part 10 file: %d bytes", len(b))
	}
	back, err := ReadBytes(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := back.GetString(tag.PatientName); !ok || name != "CITIZEN^Jan" {
		t.Fatalf("roundtrip name = %q", name)
	}
}

func TestFileInstanceFileIDAndLoad(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "SUB"), 0o755); err != nil {
		t.Fatal(err)
	}
	fd := &FileDataset{Dataset: NewDataset(), FileMeta: NewFileMetaDataset()}
	fd.Set(NewDataElement(tag.PatientName, VRPN, "CITIZEN^Jan"))
	fd.FileMeta.Set(NewDataElement(tag.TransferSyntaxUID, VRUI, string(uid.ExplicitVRLittleEndian)))
	fd.FileMeta.Set(NewDataElement(tag.MediaStorageSOPClassUID, VRUI, string(uid.CTImageStorage)))
	fd.FileMeta.Set(NewDataElement(tag.MediaStorageSOPInstanceUID, VRUI, "1.2.3"))
	path := filepath.Join(dir, "SUB", "IMG1")
	if err := fd.SaveAs(path, &WriteOptions{EnforceFileFormat: true}); err != nil {
		t.Fatal(err)
	}

	inst := &FileInstance{fs: &FileSet{RootPath: dir}, rel: filepath.Join("SUB", "IMG1")}
	if got := inst.FileID(); got != filepath.Join("SUB", "IMG1") {
		t.Fatalf("FileID = %q", got)
	}
	if got := inst.Path(); got != path {
		t.Fatalf("Path = %q, want %q", got, path)
	}
	loaded, err := inst.Load()
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := loaded.GetString(tag.PatientName); !ok || name != "CITIZEN^Jan" {
		t.Fatalf("loaded name = %q", name)
	}

	if got := (&FileInstance{}).FileID(); got != "" {
		t.Fatalf("empty instance FileID = %q", got)
	}
	if _, err := (&FileInstance{}).Load(); err == nil {
		t.Fatal("an instance without a path should refuse to load")
	}
}

func TestDicomIOByteOrderAccessors(t *testing.T) {
	d := NewDicomReader(bytes.NewReader([]byte{1, 2, 3, 4}))
	// A fresh reader carries no byte order until the caller picks one; the read
	// path always does, which is why the zero value is nil rather than a guess.
	if d.ByteOrder() != nil {
		t.Fatal("a fresh reader should not assume a byte order")
	}
	d.SetByteOrder(true)
	if !d.IsLittleEndian() {
		t.Fatal("IsLittleEndian false after SetByteOrder(true)")
	}
	if d.ByteOrder().Uint16([]byte{1, 0}) != 1 {
		t.Fatal("SetByteOrder(true) is not little endian")
	}
	d.SetByteOrder(false)
	if d.IsLittleEndian() {
		t.Fatal("IsLittleEndian still true after SetByteOrder(false)")
	}
	if d.ByteOrder().Uint16([]byte{1, 0}) != 256 {
		t.Fatal("ByteOrder did not switch to big endian")
	}
}

func TestSeekerReaderAt(t *testing.T) {
	s := seekerReaderAt{rs: bytes.NewReader([]byte{0, 1, 2, 3, 4})}
	buf := make([]byte, 2)
	if n, err := s.ReadAt(buf, 1); err != nil || n != 2 {
		t.Fatalf("ReadAt = %d %v", n, err)
	}
	if buf[0] != 1 || buf[1] != 2 {
		t.Fatalf("read %v, want [1 2]", buf)
	}
	// The adapter seeks before every read, so the position does not accumulate.
	if _, err := s.ReadAt(buf, 3); err != nil {
		t.Fatal(err)
	}
	if buf[0] != 3 {
		t.Fatalf("second read %v, want [3 4]", buf)
	}
	if _, err := s.ReadAt(make([]byte, 4), 3); err == nil {
		t.Fatal("expected short-read error past the end")
	}
}

func TestScanUntilDelimiter(t *testing.T) {
	delim := []byte{0xFE, 0xFF, 0xDD, 0xE0}
	data := append([]byte{0xAA, 0xBB}, delim...)
	data = append(data, 0xCC)

	// The delimiter is an item-style tag with an 8-byte header, so the next
	// element starts eight bytes after the tag does.
	end := scanUntilDelimiter(data, 0, SequenceDelimiterTag, true)
	if end != 10 {
		t.Fatalf("end = %d, want 10 (past the delimiter header)", end)
	}
	value, endPos := readBytesUntilDelimiter(data, 0, SequenceDelimiterTag, true)
	if endPos != 10 || !bytes.Equal(value, []byte{0xAA, 0xBB}) {
		t.Fatalf("read %v end %d", value, endPos)
	}
	if len(value) > 0 && &value[0] == &data[0] {
		t.Fatal("readBytesUntilDelimiter should return a copy")
	}

	// A tail too short to hold a tag cannot contain a delimiter, so the scan
	// stops at the offset and the value comes back empty.
	if got := scanUntilDelimiter(data[:2], 0, SequenceDelimiterTag, true); got != 0 {
		t.Fatalf("scan over a 2-byte tail = %d, want 0", got)
	}
	value, endPos = readBytesUntilDelimiter(data[:2], 0, SequenceDelimiterTag, true)
	if endPos != 0 || len(value) != 0 {
		t.Fatalf("read over a 2-byte tail %v end %d", value, endPos)
	}
	// Five bytes can hold one tag position, so the scan stops where the next
	// tag could no longer start.
	five := []byte{1, 2, 3, 4, 5}
	if got := scanUntilDelimiter(five, 0, SequenceDelimiterTag, true); got != 2 {
		t.Fatalf("scan over five bytes = %d, want 2", got)
	}

	// The same tag in big-endian order: group FFFE, element E0DD -> FF FE E0 DD.
	be := append([]byte{0x01}, 0xFF, 0xFE, 0xE0, 0xDD)
	if got := scanUntilDelimiter(be, 0, SequenceDelimiterTag, false); got != 9 {
		t.Fatalf("big-endian scan = %d, want 9", got)
	}
	if got := scanUntilDelimiter(be, 0, SequenceDelimiterTag, true); got != 2 {
		t.Fatalf("big-endian bytes read as little endian = %d, want no match", got)
	}
}
