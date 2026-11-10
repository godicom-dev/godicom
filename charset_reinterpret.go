package godicom

import (
	"fmt"
	"strings"
)

// reinterpretEdit is one element value decoded under the new character set. The
// edits are collected before any of them is applied, so a decode failure leaves
// the dataset exactly as it was.
type reinterpretEdit struct {
	elem  *DataElement
	value interface{}
}

// ReinterpretCharacterSet decodes the text values of a dataset that was read
// from bytes using charset instead of the set its (0008,0005) named, and updates
// (0008,0005) to match.
//
// It is the repair for a file whose declaration is wrong, which is the common
// case for older Chinese and Japanese equipment: the bytes are still the bytes
// of the file, so decoding them under the set the caller declares recovers the
// original text. Two shapes of wrong declaration end up here:
//
//   - ISO_IR 6 or no (0008,0005) at all, with national bytes behind it. The read
//     kept those bytes undecoded, and writing after a plain (0008,0005) change
//     is refused with ErrCharsetChangeLosesText, because re-encoding bytes that
//     were never text replaces every non-ASCII run with U+FFFD.
//   - a single byte set such as ISO_IR 100, with GBK bytes behind it. The read
//     produced mojibake that is valid text, so nothing complains, and decoding
//     the same bytes again is the only way back -- pydicom cannot do this at all,
//     because it dropped the bytes when it decoded Latin-1.
//
// Only elements that still carry the bytes read from the source are touched. A
// value the caller has replaced since the read has no stored bytes and is left
// alone, which also means the method composes with edits: replace the values you
// have, reinterpret the rest.
//
// charset must be a term the decoder implements; an unknown or non-decoding one
// is rejected rather than silently falling back to ISO_IR 6. The returned count
// is the number of elements whose bytes held non-ASCII data and were therefore
// decoded again. Calling it twice with the same charset is a no-op that returns
// the same count.
func (d *Dataset) ReinterpretCharacterSet(charset string) (int, error) {
	if d == nil {
		return 0, fmt.Errorf("godicom: missing dataset")
	}
	charset = strings.TrimSpace(charset)
	if charset == "" {
		return 0, fmt.Errorf("godicom: ReinterpretCharacterSet needs a character set term, got an empty string")
	}
	canonical, ok := resolveCharacterSetNameOK(charset)
	if !ok {
		return 0, fmt.Errorf("godicom: unknown character set %q", charset)
	}
	if !charsetDecodesText([]string{canonical}) {
		return 0, fmt.Errorf(
			"godicom: %q does not decode text, so there is nothing to reinterpret; use ISO_IR 6 only to declare the default",
			charset,
		)
	}
	if d.originalCharsets == nil {
		return 0, fmt.Errorf(
			"godicom: dataset was not read from a source, so it holds no undecoded bytes; set %s and write instead",
			TagCharset,
		)
	}

	edits, err := collectReinterpretedValues(d, []string{canonical})
	if err != nil {
		return 0, err
	}
	for _, edit := range edits {
		edit.elem.Value = edit.value
	}
	if err := d.SetString(TagCharset, canonical); err != nil {
		return 0, err
	}
	// The text now matches the declaration, so a later write must treat this as
	// the encoding the values were decoded with: the bytes of an untouched
	// element are then written back verbatim, and a replaced one is encoded into
	// the new set. Leaving the old value here would make the writer re-encode
	// every element, which is the corruption this method exists to avoid.
	d.originalCharsets = []string{canonical}
	return len(edits), nil
}

// charsetDecodesText reports whether charsets decodes text at all. A dataset
// read under a set that does not is the only one whose values can still be
// undecoded bytes, and the only one where a (0008,0005) change is destructive.
func charsetDecodesText(charsets []string) bool {
	// 0x80 is the smallest byte whose meaning differs between the default set
	// (ISO_IR 6) and every set that decodes text, so this asks the same question
	// needsCharsetDecode asks of a value, without a value.
	return needsCharsetDecode([]byte{0x80}, charsets)
}

func hasNonASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return true
		}
	}
	return false
}

// tagName is the tag with its keyword, so an error names the element a caller
// has to look at rather than only its number.
func tagName(tag Tag) string {
	if keyword, ok := keywordForTag(tag); ok {
		return fmt.Sprintf("%s %s", tag, keyword)
	}
	return tag.String()
}

// collectReinterpretedValues decodes every text and PN element that still holds
// the bytes read from the source, walking sequence items as well. Elements with
// ASCII-only bytes are skipped: every character set godicom implements keeps
// ASCII, so re-decoding them could not change anything. A deferred value is
// loaded first, because the bytes it stands for are exactly what has to be
// decoded.
func collectReinterpretedValues(ds *Dataset, charsets []string) ([]reinterpretEdit, error) {
	if ds == nil {
		return nil, nil
	}
	var edits []reinterpretEdit
	for _, elem := range ds.Iter() {
		if elem == nil {
			continue
		}
		if elem.VR == VRSQ {
			seq, ok := elem.Value.(*Sequence)
			if !ok {
				continue
			}
			for _, item := range seq.Items() {
				nested, err := collectReinterpretedValues(item, charsets)
				if err != nil {
					return nil, err
				}
				edits = append(edits, nested...)
			}
			continue
		}
		if !vrUsesCharacterSet(elem.VR) {
			continue
		}
		if elem.Deferred {
			if err := ds.loadDeferred(elem.Tag); err != nil {
				return nil, err
			}
		}
		if len(elem.RawValue) == 0 || !hasNonASCII(elem.RawValue) {
			continue
		}
		raw := &RawDataElement{
			Tag:            elem.Tag,
			VR:             elem.VR,
			Length:         uint32(len(elem.RawValue)),
			Value:          elem.RawValue,
			IsImplicitVR:   elem.IsImplicitVR,
			IsLittleEndian: elem.IsLittleEndian,
			IsRaw:          true,
		}
		value, err := convertValueWithCharsets(raw, charsets)
		if err != nil {
			return nil, &TagError{Tag: elem.Tag, Err: fmt.Errorf(
				"godicom: %s cannot be reinterpreted as %q: %w", tagName(elem.Tag), strings.Join(charsets, `\`), err,
			)}
		}
		edits = append(edits, reinterpretEdit{elem: elem, value: value})
	}
	return edits, nil
}

// firstUndecodedTextTag returns the first text or PN element that still holds
// the bytes read from the source and contains a byte above 0x7F -- the bytes a
// character set change would re-encode as if they were text. A deferred element
// is loaded so its bytes can be inspected; the writer would otherwise re-encode
// it without anyone noticing.
func firstUndecodedTextTag(ds *Dataset) (Tag, bool, error) {
	if ds == nil {
		return 0, false, nil
	}
	for _, elem := range ds.Iter() {
		if elem == nil {
			continue
		}
		if elem.VR == VRSQ {
			seq, ok := elem.Value.(*Sequence)
			if !ok {
				continue
			}
			for _, item := range seq.Items() {
				if tag, found, err := firstUndecodedTextTag(item); found || err != nil {
					return tag, found, err
				}
			}
			continue
		}
		if !vrUsesCharacterSet(elem.VR) {
			continue
		}
		if elem.Deferred {
			if err := ds.loadDeferred(elem.Tag); err != nil {
				return 0, false, err
			}
		}
		if len(elem.RawValue) > 0 && hasNonASCII(elem.RawValue) {
			return elem.Tag, true, nil
		}
	}
	return 0, false, nil
}
