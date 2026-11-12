package dicomjson

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
)

// An UN value holds an implicit-VR little-endian dataset, so a sequence inside
// it can carry either a written-out length or 0xFFFFFFFF and a Sequence
// Delimiter. The undefined forms are what a converter emits for an implicit-VR
// file, and they are the ones that need the delimiter to be found
// structurally: the four bytes of (FFFE,E0DD) are also legal inside a value,
// and searching the raw bytes for them stops at the wrong one.

// itemPrefix encodes the (FFFE,E000) item header that opens an item of the
// given length.
func itemPrefix(length int) []byte {
	out := make([]byte, 8)
	putTag(out, godicom.ItemTag)
	binary.LittleEndian.PutUint32(out[4:8], uint32(length))
	return out
}

// undefinedLengthSQElement encodes an SQ element that runs until a Sequence
// Delimiter, holding the given encoded items.
func undefinedLengthSQElement(tag godicom.Tag, items ...[]byte) []byte {
	out := undefinedLengthPrefix(tag)
	for _, it := range items {
		out = append(out, it...)
	}
	return append(out, zeroLengthTag(godicom.SequenceDelimiterTag)...)
}

// definedLengthItem wraps encoded elements in an item with a written-out length.
func definedLengthItem(elements ...[]byte) []byte {
	var content []byte
	for _, e := range elements {
		content = append(content, e...)
	}
	return append(itemPrefix(len(content)), content...)
}

// undefinedLengthItem wraps encoded elements in an item that ends at an Item
// Delimiter.
func undefinedLengthItem(elements ...[]byte) []byte {
	out := itemPrefix(0)
	binary.LittleEndian.PutUint32(out[4:8], 0xFFFFFFFF)
	for _, e := range elements {
		out = append(out, e...)
	}
	return append(out, zeroLengthTag(godicom.ItemDelimiterTag)...)
}

func TestParseInlineBinaryUNSequenceElementUndefinedLength(t *testing.T) {
	// A sequence element inside an item uses an undefined length, and the item
	// continues with another element after the sequence's delimiter. The
	// trailing value is what proves the parser resumed at the right offset.
	content := godicom.MustTag("ContentSequence")
	codeValue := godicom.MustTag("CodeValue")

	inner := undefinedLengthSQElement(content,
		definedLengthItem(implicitElement(codeValue, []byte("IN"))),
	)
	value := append(definedLengthItem(inner, implicitElement(codeValue, []byte("TAIL"))),
		zeroLengthTag(godicom.SequenceDelimiterTag)...)

	ds, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", value))
	if err != nil {
		t.Fatalf("ParseDataset() error = %v", err)
	}
	seq, ok := ds.GetSequence(godicom.MustTag("ReferencedSeriesSequence"))
	if !ok || seq.Len() != 1 {
		t.Fatalf("outer sequence = %v, ok = %v", seq, ok)
	}

	innerSeq, ok := seq.Get(0).GetSequence(content)
	if !ok || innerSeq.Len() != 1 {
		t.Fatalf("inner sequence = %v, ok = %v", innerSeq, ok)
	}
	if got, _ := innerSeq.Get(0).GetString(codeValue); got != "IN" {
		t.Fatalf("inner value = %q, want IN", got)
	}
	if got, ok := seq.Get(0).GetString(codeValue); !ok || got != "TAIL" {
		t.Fatalf("trailing value = %q, ok = %v, want TAIL", got, ok)
	}
}

func TestParseInlineBinaryUNSequenceDelimiterBytesInsideValue(t *testing.T) {
	// The private value holds FE FF DD E0, which is (FFFE,E0DD) little endian.
	// A byte-wise search for the delimiter stops inside this value and leaves
	// the parser reading element headers out of a value; finding the delimiter
	// structurally is the only way through.
	content := godicom.MustTag("ContentSequence")
	codeValue := godicom.MustTag("CodeValue")
	private := godicom.MustTag("00091001")

	marker := []byte{0xAA, 0xAA, 0xFE, 0xFF, 0xDD, 0xE0, 0xBB, 0xBB}
	inner := undefinedLengthSQElement(content,
		definedLengthItem(
			implicitElement(private, marker),
			implicitElement(codeValue, []byte("IN")),
		),
	)
	value := append(definedLengthItem(inner, implicitElement(codeValue, []byte("TAIL"))),
		zeroLengthTag(godicom.SequenceDelimiterTag)...)

	ds, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", value))
	if err != nil {
		t.Fatalf("ParseDataset() error = %v", err)
	}
	seq, ok := ds.GetSequence(godicom.MustTag("ReferencedSeriesSequence"))
	if !ok || seq.Len() != 1 {
		t.Fatalf("outer sequence = %v, ok = %v", seq, ok)
	}
	innerSeq, ok := seq.Get(0).GetSequence(content)
	if !ok || innerSeq.Len() != 1 {
		t.Fatalf("inner sequence = %v, ok = %v", innerSeq, ok)
	}
	got, ok := innerSeq.Get(0).GetBytes(private)
	if !ok || string(got) != string(marker) {
		t.Fatalf("private value = %x, ok = %v, want %x", got, ok, marker)
	}
	if v, _ := innerSeq.Get(0).GetString(codeValue); v != "IN" {
		t.Fatalf("inner value = %q, want IN", v)
	}
}

func TestParseInlineBinaryUNSequenceItemDelimiterBytesInsideValue(t *testing.T) {
	// The same hazard one level down: an item with an undefined length whose
	// value contains FE FF 0D E0, the Item Delimiter in little endian.
	content := godicom.MustTag("ContentSequence")
	codeValue := godicom.MustTag("CodeValue")
	private := godicom.MustTag("00091001")

	marker := []byte{0xAA, 0xAA, 0xFE, 0xFF, 0x0D, 0xE0, 0xBB, 0xBB}
	inner := undefinedLengthSQElement(content,
		undefinedLengthItem(
			implicitElement(private, marker),
			implicitElement(codeValue, []byte("IN")),
		),
	)
	value := append(definedLengthItem(inner), zeroLengthTag(godicom.SequenceDelimiterTag)...)

	ds, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", value))
	if err != nil {
		t.Fatalf("ParseDataset() error = %v", err)
	}
	seq, ok := ds.GetSequence(godicom.MustTag("ReferencedSeriesSequence"))
	if !ok || seq.Len() != 1 {
		t.Fatalf("outer sequence = %v, ok = %v", seq, ok)
	}
	innerSeq, ok := seq.Get(0).GetSequence(content)
	if !ok || innerSeq.Len() != 1 {
		t.Fatalf("inner sequence = %v, ok = %v", innerSeq, ok)
	}
	got, ok := innerSeq.Get(0).GetBytes(private)
	if !ok || string(got) != string(marker) {
		t.Fatalf("private value = %x, ok = %v, want %x", got, ok, marker)
	}
	if v, _ := innerSeq.Get(0).GetString(codeValue); v != "IN" {
		t.Fatalf("inner value = %q, want IN", v)
	}
}

func TestParseInlineBinaryUNSequenceNestedUndefinedLength(t *testing.T) {
	// An undefined-length item holding an undefined-length sequence: each stops
	// at its own delimiter, and neither delimiter ends the other. This shape used
	// to be refused, because the item scan searched the raw bytes for the next
	// Item Delimiter and found the one belonging to the item inside the sequence.
	content := godicom.MustTag("ContentSequence")
	codeValue := godicom.MustTag("CodeValue")

	innerSQ := undefinedLengthSQElement(content,
		undefinedLengthItem(implicitElement(codeValue, []byte("IN"))),
	)
	outerSQ := undefinedLengthSQElement(content, undefinedLengthItem(innerSQ))
	value := append(definedLengthItem(outerSQ), zeroLengthTag(godicom.SequenceDelimiterTag)...)

	ds, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", value))
	if err != nil {
		t.Fatalf("ParseDataset() error = %v", err)
	}
	seq, ok := ds.GetSequence(godicom.MustTag("ReferencedSeriesSequence"))
	if !ok || seq.Len() != 1 {
		t.Fatalf("outer sequence = %v, ok = %v", seq, ok)
	}
	innerSeq, ok := seq.Get(0).GetSequence(content)
	if !ok || innerSeq.Len() != 1 {
		t.Fatalf("inner sequence = %v, ok = %v", innerSeq, ok)
	}
	deepest, ok := innerSeq.Get(0).GetSequence(content)
	if !ok || deepest.Len() != 1 {
		t.Fatalf("nested sequence = %v, ok = %v", deepest, ok)
	}
	if got, _ := deepest.Get(0).GetString(codeValue); got != "IN" {
		t.Fatalf("value = %q, want IN", got)
	}
}

func TestParseInlineBinaryUNSequenceUndefinedLengthErrors(t *testing.T) {
	t.Run("sequence_without_delimiter", func(t *testing.T) {
		content := godicom.MustTag("ContentSequence")
		codeValue := godicom.MustTag("CodeValue")

		// The sequence element claims an undefined length and then simply ends.
		inner := undefinedLengthPrefix(content)
		inner = append(inner, definedLengthItem(implicitElement(codeValue, []byte("IN")))...)
		value := append(definedLengthItem(inner), zeroLengthTag(godicom.SequenceDelimiterTag)...)

		_, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", value))
		if err == nil || !strings.Contains(err.Error(), "unterminated undefined length sequence") {
			t.Fatalf("error = %v, want unterminated undefined length sequence", err)
		}
	})

	t.Run("item_without_delimiter", func(t *testing.T) {
		content := godicom.MustTag("ContentSequence")
		codeValue := godicom.MustTag("CodeValue")

		item := itemPrefix(0)
		binary.LittleEndian.PutUint32(item[4:8], 0xFFFFFFFF)
		item = append(item, implicitElement(codeValue, []byte("IN"))...)

		value := append(definedLengthItem(undefinedLengthSQElement(content, item)),
			zeroLengthTag(godicom.SequenceDelimiterTag)...)

		_, err := ParseDataset(inlineBinaryJSON(t, "00081115", "UN", value))
		if err == nil || !strings.Contains(err.Error(), "unterminated undefined length item") {
			t.Fatalf("error = %v, want unterminated undefined length item", err)
		}
	})
}
