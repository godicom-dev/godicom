package dicomjson

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/godicom-dev/godicom"
)

func parseUnknownInlineBinary(tag godicom.Tag, value []byte) (*godicom.DataElement, bool, error) {
	vr := godicom.LookupVR(tag)
	if vr == "" || vr == godicom.VRUN {
		return nil, false, nil
	}
	parsed, err := parseKnownVRValue(tag, vr, value)
	if err != nil {
		return nil, false, err
	}
	return godicom.NewDataElement(tag, vr, parsed), true, nil
}

func parseKnownVRValue(tag godicom.Tag, vr godicom.VR, value []byte) (interface{}, error) {
	if vr == godicom.VRSQ {
		return parseImplicitLittleEndianSequence(value)
	}
	if isBinaryVR(vr) {
		return value, nil
	}
	if isIntegerVR(vr) && !godicom.IsStringVR(vr) {
		return parseBinaryInts(value, vr)
	}
	if isFloatVR(vr) && !godicom.IsStringVR(vr) {
		return parseBinaryFloats(value, vr)
	}

	text := strings.TrimRight(string(value), " \x00")
	if vr == godicom.VRPN {
		return godicom.ParsePersonName(text), nil
	}
	if vr == godicom.VRIS {
		if text == "" {
			return nil, nil
		}
		return strconv.Atoi(text)
	}
	if vr == godicom.VRDS {
		if text == "" {
			return nil, nil
		}
		return strconv.ParseFloat(text, 64)
	}
	return text, nil
}

func parseImplicitLittleEndianSequence(data []byte) (*godicom.Sequence, error) {
	items := make([]*godicom.Dataset, 0)
	pos := 0
	for pos+8 <= len(data) {
		tag := readLittleEndianTag(data[pos : pos+4])
		if tag == godicom.SequenceDelimiterTag {
			break
		}
		if tag != godicom.ItemTag {
			return nil, fmt.Errorf("dicomjson: expected item tag in UN SQ, got %s", tag)
		}
		length := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		pos += 8
		if length == 0xFFFFFFFF {
			item, newPos, err := parseUndefinedLengthItem(data, pos)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
			pos = newPos
			continue
		}
		end := pos + int(length)
		if end > len(data) {
			return nil, fmt.Errorf("dicomjson: UN SQ item length exceeds value")
		}
		item, err := parseImplicitLittleEndianDataset(data[pos:end])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		pos = end
	}
	return godicom.NewSequence(items), nil
}

func parseUndefinedLengthItem(data []byte, pos int) (*godicom.Dataset, int, error) {
	start := pos
	for pos+8 <= len(data) {
		tag := readLittleEndianTag(data[pos : pos+4])
		if tag == godicom.ItemDelimiterTag {
			item, err := parseImplicitLittleEndianDataset(data[start:pos])
			if err != nil {
				return nil, 0, err
			}
			return item, pos + 8, nil
		}
		pos++
	}
	return nil, 0, fmt.Errorf("dicomjson: unterminated undefined length item")
}

func parseImplicitLittleEndianDataset(data []byte) (*godicom.Dataset, error) {
	ds := godicom.NewDataset()
	pos := 0
	for pos+8 <= len(data) {
		tag := readLittleEndianTag(data[pos : pos+4])
		if tag == godicom.ItemDelimiterTag || tag == godicom.SequenceDelimiterTag {
			break
		}
		length := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		pos += 8
		vr := godicom.LookupVR(tag)
		if vr == "" {
			vr = godicom.VRUN
		}
		if length == 0xFFFFFFFF {
			if vr != godicom.VRSQ {
				return nil, fmt.Errorf("dicomjson: unsupported undefined length VR %s for %s", vr, tag)
			}
			seq, newPos, err := parseUndefinedLengthSequence(data, pos)
			if err != nil {
				return nil, err
			}
			ds.Set(godicom.NewDataElement(tag, vr, seq))
			pos = newPos
			continue
		}
		end := pos + int(length)
		if end > len(data) {
			return nil, fmt.Errorf("dicomjson: element %s length exceeds dataset", tag)
		}
		value, err := parseKnownVRValue(tag, vr, data[pos:end])
		if err != nil {
			return nil, err
		}
		ds.Set(godicom.NewDataElement(tag, vr, value))
		pos = end
	}
	return ds, nil
}

func parseUndefinedLengthSequence(data []byte, pos int) (*godicom.Sequence, int, error) {
	seq, err := parseImplicitLittleEndianSequence(data[pos:])
	if err != nil {
		return nil, 0, err
	}
	for pos+8 <= len(data) {
		if readLittleEndianTag(data[pos:pos+4]) == godicom.SequenceDelimiterTag {
			return seq, pos + 8, nil
		}
		pos++
	}
	return nil, 0, fmt.Errorf("dicomjson: unterminated undefined length sequence")
}

func parseBinaryInts(value []byte, vr godicom.VR) (interface{}, error) {
	size := 2
	if vr == godicom.VRUL || vr == godicom.VRSL {
		size = 4
	}
	if len(value)%size != 0 {
		return nil, fmt.Errorf("dicomjson: invalid integer value length %d for VR %s", len(value), vr)
	}
	count := len(value) / size
	items := make([]interface{}, 0, count)
	for i := 0; i < count; i++ {
		chunk := value[i*size:]
		switch vr {
		case godicom.VRUS:
			items = append(items, int64(binary.LittleEndian.Uint16(chunk)))
		case godicom.VRSS:
			items = append(items, int64(int16(binary.LittleEndian.Uint16(chunk))))
		case godicom.VRUL:
			items = append(items, int64(binary.LittleEndian.Uint32(chunk)))
		case godicom.VRSL:
			items = append(items, int64(int32(binary.LittleEndian.Uint32(chunk))))
		default:
			return nil, fmt.Errorf("dicomjson: unsupported integer VR %s", vr)
		}
	}
	if len(items) == 1 {
		return items[0], nil
	}
	return godicom.NewMultiValue(items), nil
}

func parseBinaryFloats(value []byte, vr godicom.VR) (interface{}, error) {
	size := 4
	if vr == godicom.VRFD {
		size = 8
	}
	if len(value)%size != 0 {
		return nil, fmt.Errorf("dicomjson: invalid float value length %d for VR %s", len(value), vr)
	}
	count := len(value) / size
	items := make([]interface{}, 0, count)
	for i := 0; i < count; i++ {
		chunk := value[i*size:]
		switch vr {
		case godicom.VRFL:
			bits := binary.LittleEndian.Uint32(chunk)
			items = append(items, float64(math.Float32frombits(bits)))
		case godicom.VRFD:
			bits := binary.LittleEndian.Uint64(chunk)
			items = append(items, math.Float64frombits(bits))
		default:
			return nil, fmt.Errorf("dicomjson: unsupported float VR %s", vr)
		}
	}
	if len(items) == 1 {
		return items[0], nil
	}
	return godicom.NewMultiValue(items), nil
}

func readLittleEndianTag(data []byte) godicom.Tag {
	group := binary.LittleEndian.Uint16(data[0:2])
	elem := binary.LittleEndian.Uint16(data[2:4])
	return godicom.NewTag(int(group), int(elem))
}
