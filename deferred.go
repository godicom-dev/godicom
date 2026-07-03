package godicom

import (
	"fmt"
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
	isImplicit, isLittleEndian bool,
	charsets []string,
) {
	elem.Deferred = true
	elem.ValueTell = valueTell
	elem.ValueLength = uint32(length)
	elem.IsImplicitVR = isImplicit
	elem.IsLittleEndian = isLittleEndian
	elem.readCharsets = append([]string(nil), charsets...)
	elem.Value = nil
	elem.RawValue = nil
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

	elementStart := elem.ValueTell - dataElementOffsetToValue(elem.IsImplicitVR, elem.VR)
	raw, err := readRawDataElementAt(ctx.data, elementStart, elem.IsImplicitVR, elem.IsLittleEndian)
	if err != nil {
		return err
	}
	if raw.Tag != elem.Tag {
		return fmt.Errorf("godicom: deferred read tag %s does not match original %s", raw.Tag, elem.Tag)
	}
	if raw.VR != elem.VR {
		return fmt.Errorf("godicom: deferred read VR %s does not match original %s", raw.VR, elem.VR)
	}

	assignElementBytes(elem, raw.Value, raw.VR, raw.IsImplicitVR, raw.IsLittleEndian, elem.readCharsets)
	elem.Deferred = false
	elem.ValueLength = 0
	elem.ValueTell = 0
	_ = ds

	return nil
}

// readRawDataElementAt reads a single defined-length element at tag position pos.
// Mirrors pydicom.filereader.data_element_generator for one element with defer_size=None.
func readRawDataElementAt(
	data []byte,
	pos int64,
	isImplicit, isLittleEndian bool,
) (*RawDataElement, error) {
	if pos+8 > int64(len(data)) {
		return nil, fmt.Errorf("godicom: unexpected EOF reading deferred element")
	}

	var vr VR
	var length int
	var hdrSize int

	if isImplicit {
		if isLittleEndian {
			length = int(binaryLittleEndianUint32(data[pos+4 : pos+8]))
		} else {
			length = int(binaryBigEndianUint32(data[pos+4 : pos+8]))
		}
		hdrSize = 8
		vr = LookupVR(readTagBytes(data, pos, isLittleEndian))
	} else {
		vrBytes := data[pos+4 : pos+6]
		vr = VR(string(vrBytes))
		if vrBytes[0] < 0x41 || vrBytes[0] > 0x5A || vrBytes[1] < 0x41 || vrBytes[1] > 0x5A {
			if isLittleEndian {
				length = int(binaryLittleEndianUint32(data[pos+4 : pos+8]))
			} else {
				length = int(binaryBigEndianUint32(data[pos+4 : pos+8]))
			}
			hdrSize = 8
			vr = LookupVR(readTagBytes(data, pos, isLittleEndian))
		} else if ExplicitVRLength16[vr] {
			if isLittleEndian {
				length = int(uint16(data[pos+6]) | uint16(data[pos+7])<<8)
			} else {
				length = int(uint16(data[pos+7]) | uint16(data[pos+6])<<8)
			}
			hdrSize = 8
		} else {
			if pos+12 > int64(len(data)) {
				return nil, fmt.Errorf("godicom: unexpected EOF reading deferred element header")
			}
			if isLittleEndian {
				length = int(binaryLittleEndianUint32(data[pos+8 : pos+12]))
			} else {
				length = int(binaryBigEndianUint32(data[pos+8 : pos+12]))
			}
			hdrSize = 12
		}
	}

	tag := readTagBytes(data, pos, isLittleEndian)
	valueTell := pos + int64(hdrSize)

	if length == 0xFFFFFFFF {
		return nil, fmt.Errorf("godicom: deferred read does not support undefined length element %s", tag)
	}
	if valueTell+int64(length) > int64(len(data)) {
		return nil, fmt.Errorf("godicom: unexpected EOF reading deferred value for %s", tag)
	}

	var value []byte
	if length > 0 {
		value = append([]byte(nil), data[valueTell:valueTell+int64(length)]...)
	}

	return &RawDataElement{
		Tag:            tag,
		VR:             vr,
		Length:         uint32(length),
		Value:          value,
		ValueTell:      valueTell,
		IsImplicitVR:   isImplicit,
		IsLittleEndian: isLittleEndian,
		IsRaw:          true,
	}, nil
}

func binaryLittleEndianUint32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func binaryBigEndianUint32(b []byte) uint32 {
	return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
}
