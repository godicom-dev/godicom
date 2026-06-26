package godicom

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// convertValue converts raw bytes to a Go value based on VR.
func convertValue(raw *RawDataElement) (interface{}, error) {
	if raw.Value == nil {
		return emptyValueForVR(raw.VR), nil
	}

	switch raw.VR {
	case VRAE:
		return convertAEString(raw.Value)
	case VRAS:
		return convertString(raw.Value)
	case VRAT:
		return convertATValue(raw.Value, raw.IsLittleEndian)
	case VRCS:
		return convertString(raw.Value)
	case VRDA:
		return convertDAString(raw.Value)
	case VRDS:
		return convertDSString(raw.Value)
	case VRDT:
		return convertDTString(raw.Value)
	case VRFD:
		return convertFloats(raw.Value, raw.IsLittleEndian, 8)
	case VRFL:
		return convertFloats(raw.Value, raw.IsLittleEndian, 4)
	case VRIS:
		return convertISString(raw.Value)
	case VRLO, VRSH, VRST, VRUC, VRUT:
		return convertText(raw.Value)
	case VRLT:
		return convertText(raw.Value)
	case VROB, VROD, VROF, VROL, VROW, VROV, VRUN:
		return raw.Value, nil
	case VRPN:
		return convertPN(raw.Value)
	case VRSL:
		return convertInts(raw.Value, raw.IsLittleEndian, 4, true)
	case VRSS:
		return convertInts(raw.Value, raw.IsLittleEndian, 2, true)
	case VRSV:
		return convertInts(raw.Value, raw.IsLittleEndian, 8, true)
	case VRTM:
		return convertTMString(raw.Value)
	case VRUI:
		return convertUI(raw.Value)
	case VRUL:
		return convertInts(raw.Value, raw.IsLittleEndian, 4, false)
	case VRUR:
		return convertString(raw.Value)
	case VRUS:
		return convertInts(raw.Value, raw.IsLittleEndian, 2, false)
	case VRUV:
		return convertInts(raw.Value, raw.IsLittleEndian, 8, false)
	default:
		return string(raw.Value), nil
	}
}

func emptyValueForVR(vr VR) interface{} {
	if vr == VRSQ {
		return NewSequence(nil)
	}
	if vr == VRPN {
		return PersonName{}
	}
	if IsStringVR(vr) && vr != VRDS && vr != VRIS {
		return ""
	}
	return nil
}

func convertString(b []byte) (string, error) {
	return strings.TrimRight(string(b), " \x00"), nil
}

func convertText(b []byte) (string, error) {
	return strings.TrimRight(string(b), " \x00"), nil
}

func convertAEString(b []byte) (string, error) {
	s := string(b)
	parts := strings.Split(s, "\\")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return strings.Join(parts, "\\"), nil
}

func convertDAString(b []byte) (string, error) {
	return strings.TrimRight(string(b), " \x00"), nil
}

func convertDTString(b []byte) (string, error) {
	return strings.TrimRight(string(b), " \x00"), nil
}

func convertTMString(b []byte) (string, error) {
	return strings.TrimRight(string(b), " \x00"), nil
}

func convertDSString(b []byte) (interface{}, error) {
	s := strings.TrimRight(string(b), " \x00")
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, "\\")
	if len(parts) == 1 {
		return strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	}
	vals := make([]float64, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, err
		}
		vals[i] = v
	}
	return NewMultiValue(vals), nil
}

func convertISString(b []byte) (interface{}, error) {
	s := strings.TrimRight(string(b), " \x00")
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, "\\")
	if len(parts) == 1 {
		return strconv.Atoi(strings.TrimSpace(parts[0]))
	}
	vals := make([]int, len(parts))
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		vals[i] = v
	}
	return NewMultiValue(vals), nil
}

func convertUI(b []byte) (UID, error) {
	s := strings.TrimRight(string(b), " \x00")
	return UID(s), nil
}

func convertPN(b []byte) (PersonName, error) {
	s := strings.TrimRight(string(b), " \x00")
	return ParsePersonName(s), nil
}

func convertATValue(b []byte, le bool) (interface{}, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("godicom: AT value too short: %d bytes", len(b))
	}
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("godicom: AT value length must be multiple of 4, got %d", len(b))
	}
	if len(b) == 4 {
		return convertTag(b, le), nil
	}
	n := len(b) / 4
	tags := make([]Tag, n)
	for i := 0; i < n; i++ {
		tags[i] = convertTag(b[i*4:], le)
	}
	return NewMultiValue(tags), nil
}

func convertFloats(b []byte, le bool, size int) (interface{}, error) {
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}
	n := len(b) / size
	vals := make([]float64, n)
	for i := 0; i < n; i++ {
		if size == 4 {
			bits := order.Uint32(b[i*4:])
			vals[i] = float64(math.Float32frombits(bits))
		} else {
			bits := order.Uint64(b[i*8:])
			vals[i] = math.Float64frombits(bits)
		}
	}
	if n == 1 {
		return vals[0], nil
	}
	return NewMultiValue(vals), nil
}

func convertInts(b []byte, le bool, size int, signed bool) (interface{}, error) {
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}
	n := len(b) / size
	if n == 0 {
		return nil, nil
	}
	if signed {
		vals := make([]int64, n)
		for i := 0; i < n; i++ {
			switch size {
			case 2:
				vals[i] = int64(int16(order.Uint16(b[i*2:])))
			case 4:
				vals[i] = int64(int32(order.Uint32(b[i*4:])))
			case 8:
				vals[i] = int64(order.Uint64(b[i*8:]))
			}
		}
		if n == 1 {
			return vals[0], nil
		}
		return NewMultiValue(vals), nil
	}
	vals := make([]uint64, n)
	for i := 0; i < n; i++ {
		switch size {
		case 2:
			vals[i] = uint64(order.Uint16(b[i*2:]))
		case 4:
			vals[i] = uint64(order.Uint32(b[i*4:]))
		case 8:
			vals[i] = order.Uint64(b[i*8:])
		}
	}
	if n == 1 {
		return vals[0], nil
	}
	return NewMultiValue(vals), nil
}

// convertTag decodes a tag from 4 bytes (group, element as two uint16).
func convertTag(b []byte, le bool) Tag {
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}
	group := order.Uint16(b[0:2])
	elem := order.Uint16(b[2:4])
	return NewTag(int(group), int(elem))
}
