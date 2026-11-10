package godicom

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom/uid"
)

// These tests cover the pair of behaviours added for #98: a dataset read under a
// character set that does not decode keeps the bytes of the source as its text,
// so changing (0008,0005) and writing used to replace every non-ASCII run with
// U+FFFD. Dataset.ReinterpretCharacterSet is the repair, and the writer refuses
// the change instead of corrupting the value.

// charsetBareSequence encodes an undefined length SQ with explicit VR little
// endian items, so the reader has to recurse into items the same way it does for
// a real file.
func charsetBareSequence(t *testing.T, tag Tag, items ...[]byte) []byte {
	t.Helper()
	b := make([]byte, 0, 12)
	b = binary.LittleEndian.AppendUint16(b, uint16(tag.Group()))
	b = binary.LittleEndian.AppendUint16(b, uint16(tag.Element()))
	b = append(b, 'S', 'Q', 0, 0)
	b = binary.LittleEndian.AppendUint32(b, 0xFFFFFFFF)
	for _, item := range items {
		b = binary.LittleEndian.AppendUint16(b, 0xFFFE)
		b = binary.LittleEndian.AppendUint16(b, 0xE000)
		b = binary.LittleEndian.AppendUint32(b, uint32(len(item)))
		b = append(b, item...)
	}
	return append(b, 0xFE, 0xFF, 0xDD, 0xE0, 0, 0, 0, 0)
}

func charsetOf(t *testing.T, ds *Dataset) string {
	t.Helper()
	elem, ok := ds.Get(TagCharset)
	if !ok {
		t.Fatal("(0008,0005) missing")
	}
	return ParseCharacterSets(elem.Value)[0]
}

func textOf(t *testing.T, ds *Dataset, name string) string {
	t.Helper()
	elem, ok := ds.Get(MustTag(name))
	if !ok {
		t.Fatalf("%s missing", name)
	}
	switch v := elem.Value.(type) {
	case string:
		return v
	case PersonName:
		return v.String()
	default:
		t.Fatalf("%s type = %T", name, elem.Value)
		return ""
	}
}

func readBare(t *testing.T, data []byte) *FileDataset {
	t.Helper()
	fd, err := ReadBytes(data, &ReadOptions{Force: true})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return fd
}

// --- ReinterpretCharacterSet: the repair works -------------------------------

func TestReinterpretCharacterSetDecodesGB18030Name(t *testing.T) {
	gbk := gb18030Bytes(t, "王^小东")
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gbk))

	// Before the repair the value is the file's bytes, not the name.
	if got := textOf(t, fd.Dataset, "PatientName"); got == "王^小东" {
		t.Fatalf("value decoded before reinterpret: %q", got)
	}

	n, err := fd.ReinterpretCharacterSet("GB18030")
	if err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	if n != 1 {
		t.Fatalf("reinterpreted %d elements, want 1", n)
	}
	if got := textOf(t, fd.Dataset, "PatientName"); got != "王^小东" {
		t.Fatalf("PatientName = %q, want 王^小东", got)
	}
	if got := charsetOf(t, fd.Dataset); got != "GB18030" {
		t.Fatalf("(0008,0005) = %q, want GB18030", got)
	}

	// The write must be the original bytes: the values were decoded from them,
	// so nothing needs re-encoding.
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Contains(out, gbk) {
		t.Fatalf("encoded dataset lost the original bytes: %q", out)
	}
	if again := textOf(t, readBare(t, out).Dataset, "PatientName"); again != "王^小东" {
		t.Fatalf("re-read PatientName = %q", again)
	}
}

func TestWriteAfterReinterpretKeepsElementBytes(t *testing.T) {
	// Reinterpreting must not rewrite the values: the bytes are still the bytes
	// of the file, and every element except (0008,0005) has to come back as it
	// arrived, byte for byte.
	data := charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东"))
	fd := readBare(t, data)
	if _, err := fd.ReinterpretCharacterSet("GB18030"); err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Contains(out, gb18030Bytes(t, "王^小东")) {
		t.Fatalf("encoded dataset lost the original bytes: %q", out)
	}
	if len(out) != len(data) {
		t.Fatalf("length changed from %d to %d: an element was rewritten", len(data), len(out))
	}

	before := readBare(t, data).Dataset
	after := readBare(t, out).Dataset
	for _, elem := range after.Iter() {
		if elem.Tag == TagCharset {
			continue
		}
		orig, ok := before.Get(elem.Tag)
		if !ok {
			t.Fatalf("%s appeared after the write", elem.Tag)
		}
		if !bytes.Equal(orig.RawValue, elem.RawValue) {
			t.Fatalf("%s bytes = %q, want %q", elem.Tag, elem.RawValue, orig.RawValue)
		}
	}
}

func TestReinterpretCharacterSetDecodesShortString(t *testing.T) {
	gbk := gb18030Bytes(t, "北京协和医院")
	var data []byte
	data = append(data, charsetBareElement(t, TagCharset, VRCS, []byte("ISO_IR 6"))...)
	data = append(data, charsetBareElement(t, MustTag("InstitutionName"), VRSH, gbk)...)
	fd := readBare(t, data)

	n, err := fd.ReinterpretCharacterSet("GB18030")
	if err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	if n != 1 {
		t.Fatalf("reinterpreted %d elements, want 1", n)
	}
	if got := textOf(t, fd.Dataset, "InstitutionName"); got != "北京协和医院" {
		t.Fatalf("InstitutionName = %q", got)
	}
}

func TestReinterpretCharacterSetDecodesMultiValue(t *testing.T) {
	gbk := gb18030Bytes(t, "北京\\上海")
	var data []byte
	data = append(data, charsetBareElement(t, TagCharset, VRCS, []byte("ISO_IR 6"))...)
	data = append(data, charsetBareElement(t, MustTag("InstitutionName"), VRLO, gbk)...)
	fd := readBare(t, data)

	if _, err := fd.ReinterpretCharacterSet("GB18030"); err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	elem, ok := fd.Dataset.Get(MustTag("InstitutionName"))
	if !ok {
		t.Fatal("InstitutionName missing")
	}
	values, ok := elem.Value.(*MultiValue[string])
	if !ok {
		t.Fatalf("InstitutionName type = %T, want *MultiValue[string]", elem.Value)
	}
	want := []string{"北京", "上海"}
	got := values.Values()
	if len(got) != len(want) {
		t.Fatalf("values = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("value %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReinterpretCharacterSetDecodesInsideSequence(t *testing.T) {
	item := charsetBareElement(t, MustTag("PatientName"), VRPN, gb18030Bytes(t, "王^小东"))
	var data []byte
	data = append(data, charsetBareElement(t, TagCharset, VRCS, []byte("ISO_IR 6"))...)
	data = append(data, charsetBareSequence(t, MustTag("ReferencedSeriesSequence"), item)...)
	fd := readBare(t, data)

	n, err := fd.ReinterpretCharacterSet("GB18030")
	if err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	if n != 1 {
		t.Fatalf("reinterpreted %d elements, want the one inside the item", n)
	}
	seqElem, ok := fd.Dataset.Get(MustTag("ReferencedSeriesSequence"))
	if !ok {
		t.Fatal("sequence missing")
	}
	seq := seqElem.Value.(*Sequence)
	if len(seq.Items()) != 1 {
		t.Fatalf("items = %d", len(seq.Items()))
	}
	if got := textOf(t, seq.Items()[0], "PatientName"); got != "王^小东" {
		t.Fatalf("item PatientName = %q", got)
	}
}

func TestReinterpretCharacterSetSkipsASCIIOnlyElements(t *testing.T) {
	var data []byte
	data = append(data, charsetBareElement(t, TagCharset, VRCS, []byte("ISO_IR 6"))...)
	data = append(data, charsetBareElement(t, MustTag("PatientName"), VRPN, []byte("Doe^John"))...)
	data = append(data, charsetBareElement(t, MustTag("InstitutionName"), VRSH, gb18030Bytes(t, "北京"))...)
	fd := readBare(t, data)

	n, err := fd.ReinterpretCharacterSet("GB18030")
	if err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	if n != 1 {
		t.Fatalf("reinterpreted %d elements, want 1 (the ASCII name needs no decoding)", n)
	}
	if got := textOf(t, fd.Dataset, "PatientName"); got != "Doe^John" {
		t.Fatalf("PatientName = %q", got)
	}
}

func TestReinterpretCharacterSetLeavesReplacedValuesAlone(t *testing.T) {
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")))
	if err := fd.SetString(MustTag("PatientName"), "Wang^XiaoDong"); err != nil {
		t.Fatalf("set name: %v", err)
	}
	n, err := fd.ReinterpretCharacterSet("GB18030")
	if err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	if n != 0 {
		t.Fatalf("reinterpreted %d elements, want 0: the only value was replaced", n)
	}
	if got := textOf(t, fd.Dataset, "PatientName"); got != "Wang^XiaoDong" {
		t.Fatalf("PatientName = %q, replaced value was reinterpreted", got)
	}
}

func TestReinterpretCharacterSetAfterReplacedValueWritesNewName(t *testing.T) {
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")))
	if err := fd.SetString(MustTag("PatientName"), "Wang^XiaoDong"); err != nil {
		t.Fatalf("set name: %v", err)
	}
	if _, err := fd.ReinterpretCharacterSet("GB18030"); err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := textOf(t, readBare(t, out).Dataset, "PatientName"); got != "Wang^XiaoDong" {
		t.Fatalf("re-read PatientName = %q", got)
	}
}

// --- ReinterpretCharacterSet: what it refuses -------------------------------

func TestReinterpretCharacterSetRejectsUnusableTerms(t *testing.T) {
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")))
	before := textOf(t, fd.Dataset, "PatientName")
	for _, term := range []string{"", "   ", "ACME_PRIVATE", "ISO_IR 6", "ISO_IR 2022 WHATEVER"} {
		if _, err := fd.ReinterpretCharacterSet(term); err == nil {
			t.Fatalf("ReinterpretCharacterSet(%q) succeeded", term)
		}
		if got := charsetOf(t, fd.Dataset); got != "ISO_IR 6" {
			t.Fatalf("(0008,0005) changed to %q after rejecting %q", got, term)
		}
		if got := textOf(t, fd.Dataset, "PatientName"); got != before {
			t.Fatalf("value changed after rejecting %q", term)
		}
	}
}

func TestReinterpretCharacterSetRecoversMisdeclaredLatin1(t *testing.T) {
	// The other shape of wrong declaration: a single byte set in front of GBK
	// bytes. The read produced valid text, so nothing complained, but the name is
	// mojibake and the bytes are still there.
	fd := readBare(t, charsetBareFile(t, "ISO_IR 100", gb18030Bytes(t, "王^小东")))
	if got := textOf(t, fd.Dataset, "PatientName"); got == "王^小东" {
		t.Fatalf("value already correct before reinterpret: %q", got)
	}
	n, err := fd.ReinterpretCharacterSet("GB18030")
	if err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	if n != 1 {
		t.Fatalf("reinterpreted %d elements, want 1", n)
	}
	if got := textOf(t, fd.Dataset, "PatientName"); got != "王^小东" {
		t.Fatalf("PatientName = %q", got)
	}
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Contains(out, gb18030Bytes(t, "王^小东")) {
		t.Fatalf("encoded dataset lost the original bytes")
	}
	if got := textOf(t, readBare(t, out).Dataset, "PatientName"); got != "王^小东" {
		t.Fatalf("re-read PatientName = %q", got)
	}
}

func TestReinterpretCharacterSetRejectsDatasetNotReadFromSource(t *testing.T) {
	ds := NewDataset()
	if err := ds.SetString(TagCharset, "ISO_IR 6"); err != nil {
		t.Fatal(err)
	}
	if err := ds.SetString(MustTag("PatientName"), "王^小东"); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.ReinterpretCharacterSet("GB18030"); err == nil {
		t.Fatal("a dataset that was never read has no bytes to reinterpret")
	}
}

func TestReinterpretCharacterSetIsIdempotent(t *testing.T) {
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")))
	first, err := fd.ReinterpretCharacterSet("GB18030")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := fd.ReinterpretCharacterSet("GB18030")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Fatalf("counts differ: %d then %d", first, second)
	}
	if got := textOf(t, fd.Dataset, "PatientName"); got != "王^小东" {
		t.Fatalf("PatientName = %q after the second call", got)
	}
}

func TestReinterpretCharacterSetIsAtomic(t *testing.T) {
	// One decodable element and one whose bytes GB18030 cannot represent: the
	// failure must leave both untouched, not half-applied.
	var data []byte
	data = append(data, charsetBareElement(t, TagCharset, VRCS, []byte("ISO_IR 6"))...)
	data = append(data, charsetBareElement(t, MustTag("PatientName"), VRPN, gb18030Bytes(t, "王^小东"))...)
	data = append(data, charsetBareElement(t, MustTag("InstitutionName"), VRSH, []byte{0xFF, 0xFE, 0x41, 0x42})...)
	fd := readBare(t, data)
	beforeName := textOf(t, fd.Dataset, "PatientName")
	beforeInst := textOf(t, fd.Dataset, "InstitutionName")

	_, err := fd.ReinterpretCharacterSet("GB18030")
	if err == nil {
		t.Fatal("reinterpreting undecodable bytes must fail")
	}
	var tagErr *TagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("error = %v, want a *TagError naming the element", err)
	}
	if tagErr.Tag != MustTag("InstitutionName") {
		t.Fatalf("TagError tag = %s, want InstitutionName", tagErr.Tag)
	}
	if got := textOf(t, fd.Dataset, "PatientName"); got != beforeName {
		t.Fatalf("PatientName = %q, the successful element was applied anyway", got)
	}
	if got := textOf(t, fd.Dataset, "InstitutionName"); got != beforeInst {
		t.Fatalf("InstitutionName = %q, changed despite the failure", got)
	}
	if got := charsetOf(t, fd.Dataset); got != "ISO_IR 6" {
		t.Fatalf("(0008,0005) = %q after a failed reinterpret", got)
	}
}

// --- Writer guard -----------------------------------------------------------

func TestWriteRefusesCharsetChangeOnUndecodedText(t *testing.T) {
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")))
	if err := fd.SetString(TagCharset, "GB18030"); err != nil {
		t.Fatalf("set charset: %v", err)
	}

	_, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if !errors.Is(err, ErrCharsetChangeLosesText) {
		t.Fatalf("EncodeDataset error = %v, want ErrCharsetChangeLosesText", err)
	}
	if !strings.Contains(err.Error(), "PatientName") {
		t.Fatalf("error = %v, want it to name the element that would be corrupted", err)
	}
	if !strings.Contains(err.Error(), "ReinterpretCharacterSet") {
		t.Fatalf("error = %v, want it to name the repair", err)
	}
}

func TestWriteRefusesCharsetChangeOnUndecodedTextThroughSaveAs(t *testing.T) {
	// Round trip through a real Part 10 file first, so SaveAs has a File Meta to
	// write and the guard is exercised on the path a caller actually uses.
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")))
	if err := fd.SetString(MustTag("SOPClassUID"), string(uid.CTImageStorage)); err != nil {
		t.Fatal(err)
	}
	if err := fd.SetString(MustTag("SOPInstanceUID"), "1.2.826.0.1.3680043.10.1337.42"); err != nil {
		t.Fatal(err)
	}
	fd.FileMeta = NewFileMetaDataset()
	part10, err := EncodeFile(fd, &WriteOptions{EnforceFileFormat: true})
	if err != nil {
		t.Fatalf("encode part 10: %v", err)
	}

	path := filepath.Join(t.TempDir(), "corrupt.dcm")
	again := readBare(t, part10)
	again.Filename = path
	if err := again.SetString(TagCharset, "GB18030"); err != nil {
		t.Fatalf("set charset: %v", err)
	}
	err = again.SaveAs(path, &WriteOptions{EnforceFileFormat: true})
	if !errors.Is(err, ErrCharsetChangeLosesText) {
		t.Fatalf("SaveAs error = %v, want ErrCharsetChangeLosesText", err)
	}
}

func TestWriteRefusesCharsetChangeInsideSequenceItem(t *testing.T) {
	item := charsetBareElement(t, MustTag("PatientName"), VRPN, gb18030Bytes(t, "王^小东"))
	var data []byte
	data = append(data, charsetBareElement(t, TagCharset, VRCS, []byte("ISO_IR 6"))...)
	data = append(data, charsetBareSequence(t, MustTag("ReferencedSeriesSequence"), item)...)
	fd := readBare(t, data)
	if err := fd.SetString(TagCharset, "GB18030"); err != nil {
		t.Fatalf("set charset: %v", err)
	}
	if _, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian); !errors.Is(err, ErrCharsetChangeLosesText) {
		t.Fatalf("error = %v, want the guard to reach sequence items", err)
	}
}

func TestWriteAllowsCharsetChangeWhenTextIsASCII(t *testing.T) {
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", []byte("Doe^John")))
	if err := fd.SetString(TagCharset, "ISO_IR 192"); err != nil {
		t.Fatalf("set charset: %v", err)
	}
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("ASCII text is identical in every set, write must succeed: %v", err)
	}
	if got := textOf(t, readBare(t, out).Dataset, "PatientName"); got != "Doe^John" {
		t.Fatalf("re-read PatientName = %q", got)
	}
}

func TestWriteAllowsCharsetChangeAfterReinterpret(t *testing.T) {
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")))
	if _, err := fd.ReinterpretCharacterSet("GB18030"); err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("write after reinterpret: %v", err)
	}
	if got := textOf(t, readBare(t, out).Dataset, "PatientName"); got != "王^小东" {
		t.Fatalf("re-read PatientName = %q", got)
	}
}

func TestWriteAllowsCharsetChangeWhenValueWasReplaced(t *testing.T) {
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")))
	if err := fd.SetString(MustTag("PatientName"), "Wang^XiaoDong"); err != nil {
		t.Fatalf("set name: %v", err)
	}
	if err := fd.SetString(TagCharset, "GB18030"); err != nil {
		t.Fatalf("set charset: %v", err)
	}
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("replaced value is text, write must succeed: %v", err)
	}
	if got := textOf(t, readBare(t, out).Dataset, "PatientName"); got != "Wang^XiaoDong" {
		t.Fatalf("re-read PatientName = %q", got)
	}
}

func TestWriteAllowsDecodedCharsetChange(t *testing.T) {
	// Read as Latin-1: the value is Unicode already, so re-encoding into UTF-8 is
	// the documented behaviour (pydicom test_changed_character_set) and the guard
	// must not fire.
	fd := readBare(t, charsetBareFile(t, "ISO_IR 100", []byte("Buc^J\xe9r\xf4me")))
	if err := fd.SetString(TagCharset, "ISO_IR 192"); err != nil {
		t.Fatalf("set charset: %v", err)
	}
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !bytes.Contains(out, []byte("Buc^J\xc3\xa9r\xc3\xb4me")) {
		t.Fatalf("value was not re-encoded as UTF-8: %q", out)
	}
	if got := textOf(t, readBare(t, out).Dataset, "PatientName"); got != "Buc^Jérôme" {
		t.Fatalf("re-read PatientName = %q", got)
	}
}

func TestWriteGuardIgnoresBinaryValues(t *testing.T) {
	var data []byte
	data = append(data, charsetBareElement(t, TagCharset, VRCS, []byte("ISO_IR 6"))...)
	data = append(data, charsetBareElement(t, MustTag("PatientName"), VRPN, []byte("Doe^John"))...)
	data = append(data, charsetBareElement(t, MustTag("BitsAllocated"), VRUS, []byte{0xFF, 0xFE})...)
	fd := readBare(t, data)
	if err := fd.SetString(TagCharset, "ISO_IR 192"); err != nil {
		t.Fatalf("set charset: %v", err)
	}
	if _, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian); err != nil {
		t.Fatalf("a binary VR cannot be corrupted by a charset change: %v", err)
	}
}

func TestWriteWithoutCharsetChangeKeepsUndecodedBytes(t *testing.T) {
	data := charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东"))
	fd := readBare(t, data)
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !bytes.Equal(out, data) {
		t.Fatalf("unchanged dataset is not byte identical:\n got %q\nwant %q", out, data)
	}
}

func TestWriteRefusesCharsetChangeOnDeferredText(t *testing.T) {
	// With DeferSize the value is still on disk when the charset changes. The
	// writer loads and re-encodes it, so the guard has to look at it too.
	fd, err := ReadBytes(charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")), &ReadOptions{
		Force:     true,
		DeferSize: 1,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := fd.SetString(TagCharset, "GB18030"); err != nil {
		t.Fatalf("set charset: %v", err)
	}
	if _, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian); !errors.Is(err, ErrCharsetChangeLosesText) {
		t.Fatalf("error = %v, want the guard to cover deferred values", err)
	}
}

func TestReinterpretCharacterSetDecodesDeferredValue(t *testing.T) {
	fd, err := ReadBytes(charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")), &ReadOptions{
		Force:     true,
		DeferSize: 1,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	n, err := fd.ReinterpretCharacterSet("GB18030")
	if err != nil {
		t.Fatalf("reinterpret: %v", err)
	}
	if n != 1 {
		t.Fatalf("reinterpreted %d elements, want 1", n)
	}
	if got := textOf(t, fd.Dataset, "PatientName"); got != "王^小东" {
		t.Fatalf("PatientName = %q", got)
	}
	out, err := EncodeDataset(fd.Dataset, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := textOf(t, readBare(t, out).Dataset, "PatientName"); got != "王^小东" {
		t.Fatalf("re-read PatientName = %q", got)
	}
}

func TestWriteRefusalLeavesTheOutputStreamUntouched(t *testing.T) {
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")))
	if err := fd.SetString(TagCharset, "GB18030"); err != nil {
		t.Fatalf("set charset: %v", err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, fd, &WriteOptions{EnforceFileFormat: true}); !errors.Is(err, ErrCharsetChangeLosesText) {
		t.Fatalf("Write error = %v, want ErrCharsetChangeLosesText", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %d bytes before refusing: %q", buf.Len(), buf.Bytes())
	}
}

func TestWriteRefusalDoesNotTruncateAnExistingFile(t *testing.T) {
	fd := readBare(t, charsetBareFile(t, "ISO_IR 6", gb18030Bytes(t, "王^小东")))
	if err := fd.SetString(TagCharset, "GB18030"); err != nil {
		t.Fatalf("set charset: %v", err)
	}
	path := filepath.Join(t.TempDir(), "existing.dcm")
	const sentinel = "do not lose this"
	if err := os.WriteFile(path, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, fd.Dataset, &WriteOptions{EnforceFileFormat: true}); !errors.Is(err, ErrCharsetChangeLosesText) {
		t.Fatalf("WriteFile error = %v, want ErrCharsetChangeLosesText", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Fatalf("existing file = %q, want it untouched", got)
	}
}
