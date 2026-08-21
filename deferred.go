package godicom

import (
	"fmt"
	"io"
	"os"
)

func dataElementOffsetToValue(isImplicit bool, vr VR) int64 {
	if isImplicit {
		return 8
	}
	if ExplicitVRLength32[vr] {
		return 12
	}
	return 8
}

func shouldDeferElement(tag Tag, length int, deferSize uint32) bool {
	if deferSize == 0 {
		return false
	}
	// Never defer Specific Character Set (needed immediately for decoding).
	if tag == TagCharset {
		return false
	}
	return uint32(length) > deferSize
}

func markElementDeferred(
	elem *Element,
	valueTell int64,
	length int,
	cc codecContext,
) {
	elem.Deferred = true
	elem.ValueTell = valueTell
	elem.ValueLength = uint32(length)
	elem.IsImplicitVR = cc.IsImplicitVR
	elem.IsLittleEndian = cc.IsLittleEndian
	elem.readCharsets = append([]string(nil), cc.Charsets...)
	elem.Value = nil
	elem.RawValue = nil
}

// valueReaderAt returns the random-access source used to reload a deferred
// value, plus a cleanup func when this call opened the source itself (nil
// otherwise). Reopening filename wins over the retained parse source so a
// deferred load never depends on a reader the caller may have moved on from.
func (rc *readContext) valueReaderAt() (io.ReaderAt, func(), error) {
	if rc.filename != "" {
		f, err := os.Open(rc.filename)
		if err != nil {
			return nil, nil, fmt.Errorf("godicom: deferred read: %w", err)
		}
		return f, func() { _ = f.Close() }, nil
	}
	if rc.src != nil {
		return rc.src, nil, nil
	}
	return nil, nil, fmt.Errorf("godicom: deferred read requires source data")
}

func loadDeferredElement(ctx *readContext, ds *Dataset, elem *Element) error {
	if !elem.Deferred {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("godicom: deferred read requires source data")
	}
	if ctx.filename != "" {
		if _, err := os.Stat(ctx.filename); err != nil {
			return fmt.Errorf("godicom: deferred read -- original file %q is missing: %w", ctx.filename, err)
		}
	}

	// The encoding a deferred element is reloaded under is the one it was first
	// read under, which markElementDeferred stored on the element itself -- the
	// enclosing parse is long over by now.
	cc := codecContext{
		EncodingInfo: EncodingInfo{
			IsImplicitVR:   elem.IsImplicitVR,
			IsLittleEndian: elem.IsLittleEndian,
		},
		Charsets: elem.readCharsets,
	}

	elementStart := elem.ValueTell - dataElementOffsetToValue(elem.IsImplicitVR, elem.VR)

	data := ctx.data
	if data == nil {
		src, cleanup, err := ctx.valueReaderAt()
		if err != nil {
			return err
		}
		if cleanup != nil {
			defer cleanup()
		}
		total := dataElementOffsetToValue(elem.IsImplicitVR, elem.VR) + int64(elem.ValueLength)
		owned := make([]byte, total)
		if _, err := io.ReadFull(io.NewSectionReader(src, elementStart, total), owned); err != nil {
			return fmt.Errorf("godicom: deferred read: %w", err)
		}
		data = owned
		elementStart = 0
	}

	// The private creator lives in the dataset, not necessarily in data: a
	// deferred load may have re-read only the element's own bytes.
	raw, err := readRawDataElementAt(data, elementStart, cc.EncodingInfo, datasetCreator(ds))
	if err != nil {
		return err
	}
	if raw.Tag != elem.Tag {
		return fmt.Errorf("godicom: deferred read tag %s does not match original %s", raw.Tag, elem.Tag)
	}
	if raw.VR != elem.VR {
		return fmt.Errorf("godicom: deferred read VR %s does not match original %s", raw.VR, elem.VR)
	}

	assignElementBytes(elem, raw.Value, raw.VR, cc)
	elem.Deferred = false
	elem.ValueLength = 0
	elem.ValueTell = 0

	return nil
}

// readRawDataElementAt reads a single defined-length element at tag position pos.
// Mirrors pydicom.filereader.data_element_generator for one element with
// defer_size=None. creator resolves private creators so an implicit VR private
// element resolves to the same VR it did on the original read.
func readRawDataElementAt(
	data []byte,
	pos int64,
	enc EncodingInfo,
	creator creatorFunc,
) (*RawDataElement, error) {
	tag := readTagBytes(data, pos, enc.IsLittleEndian)
	h, need, ok := decodeElementHeader(data, pos, tag, enc, creator)
	if !ok {
		return nil, fmt.Errorf("godicom: unexpected EOF reading deferred element header for %s: need %d bytes, have %d",
			tag, need, int64(len(data))-pos)
	}

	valueTell := pos + int64(h.Size)

	if h.Length == 0xFFFFFFFF {
		return nil, fmt.Errorf("godicom: deferred read does not support undefined length element %s", tag)
	}
	if valueTell+int64(h.Length) > int64(len(data)) {
		return nil, fmt.Errorf("godicom: unexpected EOF reading deferred value for %s", tag)
	}

	var value []byte
	if h.Length > 0 {
		value = append([]byte(nil), data[valueTell:valueTell+int64(h.Length)]...)
	}

	return &RawDataElement{
		Tag:            tag,
		VR:             h.VR,
		Length:         uint32(h.Length),
		Value:          value,
		ValueTell:      valueTell,
		IsImplicitVR:   enc.IsImplicitVR,
		IsLittleEndian: enc.IsLittleEndian,
		IsRaw:          true,
	}, nil
}
