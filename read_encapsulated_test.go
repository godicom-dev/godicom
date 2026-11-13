package godicom

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/godicom-dev/godicom/tag"
)

// viewOf hands readOrSkipEncapsulated the atView it reads through.
func viewOf(t *testing.T, data []byte) atView {
	t.Helper()
	return atView{ra: bytes.NewReader(data), size: int64(len(data))}
}

// tagBytesLE encodes a tag the way a little endian stream does: the group as
// a 16-bit little endian number, then the element the same way. A 32-bit
// conversion of the tag would put the bytes the other way round.
func tagBytesLE(t Tag) []byte {
	b := binary.LittleEndian.AppendUint16(nil, uint16(t.Group()))
	return binary.LittleEndian.AppendUint16(b, uint16(t.Element()))
}

// encapsItem encodes one item of an encapsulated pixel data stream: the Item
// tag, a four-byte length, then the payload.
func encapsItem(payload []byte) []byte {
	b := append(tagBytesLE(ItemTag), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(payload)))
	return append(b, payload...)
}

// encapsUndefinedItem encodes the item a non-conformant implementation writes
// when it does not know a frame's length in advance: the Item tag with the
// undefined length, then the payload with nothing to say where it ends.
func encapsUndefinedItem(payload []byte) []byte {
	b := append(tagBytesLE(ItemTag), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(b[4:], 0xFFFFFFFF)
	return append(b, payload...)
}

// encapsStreamDelimiter is the Sequence Delimiter Item that ends a value of
// undefined length: the tag, then the four zero bytes of its length.
func encapsStreamDelimiter(littleEndian bool) []byte {
	b := make([]byte, 4)
	if littleEndian {
		copy(b, tagBytesLE(SequenceDelimiterTag))
	} else {
		binary.BigEndian.PutUint16(b[0:], uint16(SequenceDelimiterTag.Group()))
		binary.BigEndian.PutUint16(b[2:], uint16(SequenceDelimiterTag.Element()))
	}
	return binary.LittleEndian.AppendUint32(b, 0)
}

// TestReadOrSkipEncapsulatedReadsAnItemStream is the conformant case: every
// frame announces its length, the walk never has to guess, and the value ends
// at the Sequence Delimiter whether it was kept or skipped.
func TestReadOrSkipEncapsulatedReadsAnItemStream(t *testing.T) {
	items := append(encapsItem([]byte("AAAA")), encapsItem([]byte("BB"))...)
	data := append(append([]byte{}, items...), encapsStreamDelimiter(true)...)
	v := viewOf(t, data)

	end, value, err := readOrSkipEncapsulated(v, 0, true, true)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if end != int64(len(data)) {
		t.Fatalf("end = %d, want %d", end, len(data))
	}
	if !bytes.Equal(value, items) {
		t.Fatalf("value = %q, want the two items %q", value, items)
	}

	end, value, err = readOrSkipEncapsulated(v, 0, true, false)
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	if end != int64(len(data)) || value != nil {
		t.Fatalf("skip = (%d, %v), want (%d, nil)", end, value, len(data))
	}
}

// TestReadOrSkipEncapsulatedDegradesOnAnUndefinedLengthItem is the
// non-conformant case the walk cannot continue past: an item that itself has
// the undefined length. The value is then whatever precedes the Sequence
// Delimiter, item headers and all, which is what pydicom's fallback returns.
func TestReadOrSkipEncapsulatedDegradesOnAnUndefinedLengthItem(t *testing.T) {
	first := encapsItem([]byte("AAAA"))
	degenerate := encapsUndefinedItem([]byte("PAYLOAD"))
	data := concatBytes(first, degenerate, encapsStreamDelimiter(true))
	v := viewOf(t, data)

	end, value, err := readOrSkipEncapsulated(v, 0, true, true)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if end != int64(len(data)) {
		t.Fatalf("end = %d, want %d", end, len(data))
	}
	if want := data[:len(data)-len(encapsStreamDelimiter(true))]; !bytes.Equal(value, want) {
		t.Fatalf("value = %q, want everything before the delimiter %q", value, want)
	}

	end, value, err = readOrSkipEncapsulated(v, 0, true, false)
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	if end != int64(len(data)) || value != nil {
		t.Fatalf("skip = (%d, %v), want (%d, nil)", end, value, len(data))
	}
}

// TestReadOrSkipEncapsulatedDegradedScanStopsAtTheFirstDelimiter pins what the
// degradation costs: the byte-wise scan it falls back to looks for the
// delimiter's bytes and cannot tell a real one from a frame that happens to
// contain them, so it stops at the first. pydicom's fallback scans the same
// way -- the structured attempt both make has already failed by then.
func TestReadOrSkipEncapsulatedDegradedScanStopsAtTheFirstDelimiter(t *testing.T) {
	coincidence := append(tagBytesLE(SequenceDelimiterTag), 0x01, 0x02, 0x03, 0x04)
	data := concatBytes(
		encapsUndefinedItem([]byte("frame data")),
		coincidence,
		[]byte("more frame data"),
		encapsStreamDelimiter(true),
	)
	v := viewOf(t, data)

	end, value, err := readOrSkipEncapsulated(v, 0, true, true)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if end != int64(len(data)-len("more frame data")-len(encapsStreamDelimiter(true))) {
		t.Fatalf("end = %d, want the scan to stop inside the payload", end)
	}
	if want := data[:end-8]; !bytes.Equal(value, want) {
		t.Fatalf("value = %q, want %q", value, want)
	}
}

// TestReadOrSkipEncapsulatedWithoutADelimiter covers the value that just ends:
// the walk runs out of bytes, and everything there is was the value.
func TestReadOrSkipEncapsulatedWithoutADelimiter(t *testing.T) {
	data := encapsItem([]byte("AAAA"))
	v := viewOf(t, data)

	end, value, err := readOrSkipEncapsulated(v, 0, true, true)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if end != int64(len(data)) || !bytes.Equal(value, data) {
		t.Fatalf("read = (%d, %q), want (%d, %q)", end, value, len(data), data)
	}
}

// TestReadOrSkipEncapsulatedDegradedWithoutADelimiter is the same, degraded:
// the byte-wise scan runs to the end of the view and takes everything with it.
func TestReadOrSkipEncapsulatedDegradedWithoutADelimiter(t *testing.T) {
	data := concatBytes(encapsItem([]byte("AAAA")), encapsUndefinedItem([]byte("PAYLOAD")))
	v := viewOf(t, data)

	end, value, err := readOrSkipEncapsulated(v, 0, true, true)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if end != int64(len(data)) || !bytes.Equal(value, data) {
		t.Fatalf("read = (%d, %q), want (%d, %q)", end, value, len(data), data)
	}
}

// TestReadOrSkipEncapsulatedBigEndianDegrades the same way in a big endian
// transfer syntax: the tag bytes the scan looks for are the other way round.
func TestReadOrSkipEncapsulatedBigEndianDegrades(t *testing.T) {
	first := encapsItemBigEndian([]byte("AAAA"))
	degenerate := encapsUndefinedItemBigEndian([]byte("PAYLOAD"))
	delimiter := encapsStreamDelimiter(false)
	data := concatBytes(first, degenerate, delimiter)
	v := viewOf(t, data)

	end, value, err := readOrSkipEncapsulated(v, 0, false, true)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if end != int64(len(data)) {
		t.Fatalf("end = %d, want %d", end, len(data))
	}
	if want := data[:len(data)-len(delimiter)]; !bytes.Equal(value, want) {
		t.Fatalf("value = %q, want %q", value, want)
	}
}

// encapsItemBigEndian and encapsUndefinedItemBigEndian are the two item
// encodings of a big endian stream.
func encapsItemBigEndian(payload []byte) []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(ItemTag))
	b = binary.BigEndian.AppendUint32(b, uint32(len(payload)))
	return append(b, payload...)
}

func encapsUndefinedItemBigEndian(payload []byte) []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(ItemTag))
	b = binary.BigEndian.AppendUint32(b, 0xFFFFFFFF)
	return append(b, payload...)
}

// concatBytes joins byte slices.
func concatBytes(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

// encapsShort builds an explicit VR little endian element whose length fits
// 16 bits.
func encapsShort(t Tag, vr VR, value []byte) []byte {
	out := binary.LittleEndian.AppendUint16(nil, uint16(t.Group()))
	out = binary.LittleEndian.AppendUint16(out, uint16(t.Element()))
	out = append(out, byte(vr[0]), byte(vr[1]))
	out = binary.LittleEndian.AppendUint16(out, uint16(len(value)))
	return append(out, value...)
}

// encapsLong is the same for the VRs whose length is 32 bits, the length
// chosen by the caller -- 0xFFFFFFFF for an undefined one.
func encapsLong(t Tag, vr VR, length uint32, value []byte) []byte {
	out := binary.LittleEndian.AppendUint16(nil, uint16(t.Group()))
	out = binary.LittleEndian.AppendUint16(out, uint16(t.Element()))
	out = append(out, byte(vr[0]), byte(vr[1]), 0, 0)
	out = binary.LittleEndian.AppendUint32(out, length)
	return append(out, value...)
}

// leUint32 is n as four little endian bytes.
func leUint32(n int) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(n))
	return b
}

// encapsFile assembles a minimal Part 10 file around an OB Pixel Data of
// undefined length: explicit VR little endian, the meta elements a reader
// needs, a Patient Name to show that the elements before it are read, and
// whatever the caller appends after it.
func encapsFile(pixelBody []byte, after ...[]byte) []byte {
	meta := concatBytes(
		encapsShort(tag.MediaStorageSOPClassUID, VRUI, []byte("1.2.840.10008.5.1.4.1.1.7")),
		encapsShort(tag.MediaStorageSOPInstanceUID, VRUI, []byte("1.2.3.4")),
		encapsShort(tag.TransferSyntaxUID, VRUI, []byte("1.2.840.10008.1.2.1")),
		encapsShort(tag.ImplementationClassUID, VRUI, []byte("1.2")),
	)
	dataset := concatBytes(append([][]byte{
		encapsShort(tag.PatientName, VRPN, []byte("TEST^A")),
		append(encapsLong(tag.PixelData, VROB, 0xFFFFFFFF, nil), pixelBody...),
	}, after...)...)

	return concatBytes(
		make([]byte, 128),
		[]byte("DICM"),
		encapsShort(tag.FileMetaInformationGroupLength, VRUL, leUint32(len(meta))),
		meta,
		dataset,
	)
}

// pixelDataOf reads the file and returns the raw Pixel Data bytes.
func pixelDataOf(t *testing.T, file []byte) []byte {
	t.Helper()
	ds, err := ReadBytes(file, nil)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	elem, ok := ds.Get(tag.PixelData)
	if !ok {
		t.Fatal("the file has no Pixel Data")
	}
	if elem.RawValue == nil {
		t.Fatal("Pixel Data has no value")
	}
	return elem.RawValue
}

// TestReadDegradedPixelDataEndToEnd runs the degradation through the whole
// reader: the same value the unit test sees, and the elements before the
// Pixel Data still read.
func TestReadDegradedPixelDataEndToEnd(t *testing.T) {
	body := concatBytes(encapsItem([]byte("AAAA")), encapsUndefinedItem([]byte("PAYLOAD")))
	file := encapsFile(body, encapsStreamDelimiter(true))

	raw := pixelDataOf(t, file)
	want := concatBytes(encapsItem([]byte("AAAA")), encapsUndefinedItem([]byte("PAYLOAD")))
	if !bytes.Equal(raw, want) {
		t.Fatalf("Pixel Data = %x, want %x", raw, want)
	}

	ds, err := ReadBytes(file, nil)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := ds.GetString(tag.PatientName); !ok || name != "TEST^A" {
		t.Fatalf("Patient Name = %q", name)
	}
}

// TestReadTruncatedPixelDataEndToEnd covers the value that runs out without a
// delimiter. The reader keeps what is there as the value and stops at the end
// of the view rather than handing the caller the last bytes of the value to
// parse as an element header -- which is what it used to do, three bytes at a
// time. pydicom warns, drops the element and stops reading; this keeps the
// value instead of losing it, and stops at the same place.
func TestReadTruncatedPixelDataEndToEnd(t *testing.T) {
	body := concatBytes(encapsItem([]byte("AAAA")), encapsUndefinedItem([]byte("PAYLOAD")))
	file := encapsFile(body)

	raw := pixelDataOf(t, file)
	if !bytes.Equal(raw, body) {
		t.Fatalf("Pixel Data = %x, want the whole body %x", raw, body)
	}

	// And the same when something follows: the view ends inside the value, so
	// the value is everything the view holds after the header.
	file = concatBytes(encapsFile(body), encapsShort(tag.Modality, VRCS, []byte("CR")))
	raw = pixelDataOf(t, file)
	want := concatBytes(body, encapsShort(tag.Modality, VRCS, []byte("CR")))
	if !bytes.Equal(raw, want) {
		t.Fatalf("Pixel Data = %x, want %x", raw, want)
	}
}
