package godicom

import (
	"bytes"
	"compress/flate"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// The writer accepts several storage shapes for the same VR, and the ones a
// value can arrive in from a real file -- a multi-valued PN, a multi-valued IS,
// an element that still holds raw bytes with an undefined length, a sequence
// whose items were written with undefined lengths -- go through branches that
// the fixture round trips do not reach. These tests build the shapes directly
// and check the bytes that come out.

func TestEncodeMultiValuedPersonNameJoinsWithBackslash(t *testing.T) {
	tag := MustTag("ReferringPhysicianName")
	ds := NewDataset()
	ds.Set(NewDataElement(tag, VRPN, NewMultiValue([]PersonName{
		ParsePersonName("Doe^John"),
		ParsePersonName("Roe^Jane"),
	})))

	b, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("EncodeDataset() error = %v", err)
	}
	if want := []byte("Doe^John\\Roe^Jane"); !bytes.Contains(b, want) {
		t.Fatalf("encoded = % x, want it to contain % x", b, want)
	}
}

func TestEncodeMultiValuedPersonNameUnderGB18030(t *testing.T) {
	// A declared character set has to reach every value of a multi-valued PN,
	// not just the first: encoding the join as one UTF-8 string would put UTF-8
	// bytes in a GB18030 dataset, which the round trip below reports as mojibake.
	tag := MustTag("ReferringPhysicianName")
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("SpecificCharacterSet"), VRCS, "GB18030"))
	ds.Set(NewDataElement(tag, VRPN, NewMultiValue([]PersonName{
		ParsePersonName("张^三"),
		ParsePersonName("李^四"),
	})))

	b, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("EncodeDataset() error = %v", err)
	}
	if bytes.Contains(b, []byte("张")) {
		t.Fatal("value was written as UTF-8 in a GB18030 dataset")
	}
	joined := append(EncodeBytesWithCharsets("张^三", []string{"GB18030"}), '\\')
	joined = append(joined, EncodeBytesWithCharsets("李^四", []string{"GB18030"})...)
	if !bytes.Contains(b, joined) {
		t.Fatalf("encoded = % x, want it to contain % x", b, joined)
	}
}

func TestEncodeMultiValuedIS(t *testing.T) {
	tag := MustTag("EchoNumbers")
	ds := NewDataset()
	ds.Set(NewDataElement(tag, VRIS, NewMultiValue([]IS{
		{Value: 1, Original: "1"},
		{Value: 2, Original: "2"},
		{Value: 3, Original: "3"},
	})))

	b, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("EncodeDataset() error = %v", err)
	}
	if want := []byte("1\\2\\3"); !bytes.Contains(b, want) {
		t.Fatalf("encoded = % x, want it to contain % x", b, want)
	}
}

func TestEncodeUntypedMultiValue(t *testing.T) {
	// A value that came from a generic source arrives as []interface{} holding
	// float64. DS has to format each one as a decimal string, not fall back to
	// fmt's default verb, which would write 1.5e+00 for a value the standard
	// spells without an exponent.
	tag := MustTag("PatientSize")
	ds := NewDataset()
	ds.Set(NewDataElement(tag, VRDS, NewMultiValue[interface{}]([]interface{}{
		float64(1.5),
		float64(2.25),
	})))

	b, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("EncodeDataset() error = %v", err)
	}
	if want := []byte("1.5\\2.25"); !bytes.Contains(b, want) {
		t.Fatalf("encoded = % x, want it to contain % x", b, want)
	}
}

// undefinedLengthTail returns the (FFFE,E0DD) zero-length tag that closes an
// undefined-length value.
func undefinedLengthTail() []byte {
	out := make([]byte, 8)
	out[0], out[1] = 0xFE, 0xFF
	out[2], out[3] = 0xDD, 0xE0
	return out
}

func TestEncodeEncapsulatedPixelDataKeepsFragments(t *testing.T) {
	// Encapsulated pixel data is opaque to the writer: the fragments are copied
	// back out with an undefined length and a sequence delimiter, not decoded and
	// re-encoded. A file that is read and written with the same transfer syntax
	// has to come back byte for byte.
	src := filepath.Join("testdata", "dcm", "emri_small_jpeg_2k_lossless.dcm")
	if _, err := os.Stat(src); err != nil {
		t.Skip("fixture not fetched; run scripts/fetch-testdata.sh")
	}
	fd, err := ReadFile(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	fragments, ok := fd.GetBytes(MustTag("PixelData"))
	if !ok || len(fragments) == 0 {
		t.Fatal("fixture has no encapsulated pixel data")
	}

	b, err := EncodeDataset(fd.Dataset, JPEG2000Lossless)
	if err != nil {
		t.Fatalf("EncodeDataset() error = %v", err)
	}
	if !bytes.Contains(b, fragments) {
		t.Fatal("raw fragments were not preserved")
	}
	if !bytes.HasSuffix(b, undefinedLengthTail()) {
		t.Fatalf("encoded tail = % x, want the sequence delimiter", b[len(b)-8:])
	}
}

func TestWriteUndefinedLengthElementExplicit(t *testing.T) {
	// Same shape in explicit VR: OB needs the two reserved bytes before the
	// 32-bit length, and that is what the undefined-length branch writes.
	ds := NewDataset()
	elem := NewDataElement(MustTag("PixelData"), VROB, nil)
	elem.RawValue = []byte{0x05, 0x06}
	elem.IsUndefinedLength = true
	ds.Set(elem)

	b, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("EncodeDataset() error = %v", err)
	}
	want := []byte{
		0xE0, 0x7F, 0x10, 0x00,
		'O', 'B', 0x00, 0x00,
		0xFF, 0xFF, 0xFF, 0xFF,
		0x05, 0x06,
	}
	want = append(want, undefinedLengthTail()...)
	if !bytes.Equal(b, want) {
		t.Fatalf("encoded = % x, want % x", b, want)
	}
}

func TestWriteUndefinedLengthSequence(t *testing.T) {
	// A sequence read with an undefined length, and an item inside it that was
	// also read with one, both have to come back out the same way: the item
	// length is 0xFFFFFFFF and an Item Delimiter closes it.
	groupTag := MustTag("ReferencedStudySequence")
	valueTag := MustTag("CodeValue")

	item := NewDataset()
	item.Set(NewDataElement(valueTag, VRSH, "ABC"))
	item.IsUndefinedLengthSequenceItem = true
	seq := NewSequence([]*Dataset{item})
	seq.IsUndefinedLength = true

	ds := NewDataset()
	ds.Set(NewDataElement(groupTag, VRSQ, seq))

	b, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("EncodeDataset() error = %v", err)
	}

	want := []byte{
		0x08, 0x00, 0x10, 0x11,
		'S', 'Q', 0x00, 0x00,
		0xFF, 0xFF, 0xFF, 0xFF,
		// Item with an undefined length.
		0xFE, 0xFF, 0x00, 0xE0,
		0xFF, 0xFF, 0xFF, 0xFF,
		// The item's one element: (0008,0100) SH "ABC".
		0x08, 0x00, 0x00, 0x01,
		'S', 'H', 0x04, 0x00,
		'A', 'B', 'C', ' ',
		// Item Delimiter, then the sequence's own delimiter.
		0xFE, 0xFF, 0x0D, 0xE0,
		0x00, 0x00, 0x00, 0x00,
	}
	want = append(want, undefinedLengthTail()...)
	if !bytes.Equal(b, want) {
		t.Fatalf("encoded = % x, want % x", b, want)
	}
}

func TestEncodeDatasetDeflated(t *testing.T) {
	// A deflated dataset is the explicit-VR little-endian encoding run through
	// flate, padded to an even length. Checked by inflating and comparing with
	// the encoding it was built from, rather than by matching the compressor's
	// output byte for byte.
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag("PatientID"), VRLO, "DEFLATE"))
	ds.Set(NewDataElement(MustTag("PatientName"), VRPN, ParsePersonName("Doe^John")))

	plain, err := EncodeDataset(ds, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("EncodeDataset() error = %v", err)
	}
	deflated, err := EncodeDataset(ds, DeflatedExplicitVRLittleEndian)
	if err != nil {
		t.Fatalf("EncodeDataset() deflated error = %v", err)
	}
	if len(deflated)%2 != 0 {
		t.Fatalf("deflated payload length = %d, want even", len(deflated))
	}

	r := flate.NewReader(bytes.NewReader(deflated))
	inflated, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("inflate error = %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("inflate close error = %v", err)
	}
	if !bytes.Equal(inflated, plain) {
		t.Fatalf("inflated = % x, want % x", inflated, plain)
	}
}
