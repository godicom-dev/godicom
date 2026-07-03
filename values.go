package godicom

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// convertValue converts raw bytes to a Go value based on VR.
func convertValue(raw *RawDataElement) (interface{}, error) {
	return convertValueWithCharsets(raw, nil)
}

func convertValueWithCharsets(raw *RawDataElement, charsets []string) (interface{}, error) {
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
		return convertTextWithCharsets(raw.Value, charsets)
	case VRLT:
		return convertTextWithCharsets(raw.Value, charsets)
	case VROB, VROD, VROF, VROL, VROW, VROV, VRUN:
		return raw.Value, nil
	case VRPN:
		return convertPNWithCharsets(raw.Value, charsets)
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
	v, err := convertTextWithCharsets(b, nil)
	if err != nil {
		return "", err
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	return fmt.Sprintf("%v", v), nil
}

func convertTextWithCharsets(b []byte, charsets []string) (interface{}, error) {
	b = trimRightNullSpaceBytes(b)
	if len(charsets) == 0 || !needsCharsetDecode(b, charsets) {
		return strings.TrimRight(string(b), " \x00"), nil
	}
	if !bytes.Contains(b, []byte{'\\'}) {
		return DecodeBytesWithDelimiters(b, charsets, textVRDelims), nil
	}
	rawParts := bytes.Split(b, []byte{'\\'})
	vals := make([]string, len(rawParts))
	for i, part := range rawParts {
		vals[i] = DecodeBytesWithDelimiters(part, charsets, textVRDelims)
	}
	if len(vals) == 1 {
		return vals[0], nil
	}
	return NewMultiValue(vals), nil
}

func needsCharsetDecode(b []byte, charsets []string) bool {
	if bytes.Contains(b, []byte{esc}) {
		return true
	}
	normalized := ConvertCharacterSets(charsets)
	if len(normalized) > 1 {
		return true
	}
	if len(normalized) == 0 {
		return false
	}
	first := normalized[0]
	return first != DefaultCharacterSet && first != "ISO_IR 6" && first != "" && first != "ISO 2022 IR 6"
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

func convertDAString(b []byte) (interface{}, error) {
	s := strings.TrimRight(string(b), " \x00")
	if s == "" {
		return "", nil
	}
	parts := strings.Split(s, "\\")
	if len(parts) == 1 {
		return parseDAValue(parts[0])
	}
	vals := make([]DA, len(parts))
	for i, p := range parts {
		v, err := parseDAValue(p)
		if err != nil {
			return nil, err
		}
		if d, ok := v.(DA); ok {
			vals[i] = d
		}
	}
	return NewMultiValue(vals), nil
}

func convertDTString(b []byte) (interface{}, error) {
	s := strings.TrimRight(string(b), " \x00")
	if s == "" {
		return "", nil
	}
	parts := strings.Split(s, "\\")
	if len(parts) == 1 {
		return parseDTValue(parts[0])
	}
	vals := make([]DT, len(parts))
	for i, p := range parts {
		v, err := parseDTValue(p)
		if err != nil {
			return nil, err
		}
		if d, ok := v.(DT); ok {
			vals[i] = d
		}
	}
	return NewMultiValue(vals), nil
}

func convertTMString(b []byte) (interface{}, error) {
	s := strings.TrimRight(string(b), " \x00")
	if s == "" {
		return "", nil
	}
	parts := strings.Split(s, "\\")
	if len(parts) == 1 {
		return parseTMValue(parts[0])
	}
	vals := make([]TM, len(parts))
	for i, p := range parts {
		v, err := parseTMValue(p)
		if err != nil {
			return nil, err
		}
		if t, ok := v.(TM); ok {
			vals[i] = t
		}
	}
	return NewMultiValue(vals), nil
}

func convertDSString(b []byte) (interface{}, error) {
	s := strings.TrimRight(string(b), " \x00")
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, "\\")
	if len(parts) == 1 {
		return parseDSValue(parts[0])
	}
	vals := make([]DS, len(parts))
	for i, p := range parts {
		v, err := parseDSValue(p)
		if err != nil {
			return nil, err
		}
		if d, ok := v.(DS); ok {
			vals[i] = d
		}
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
		return parseISValue(parts[0])
	}
	vals := make([]IS, len(parts))
	for i, p := range parts {
		v, err := parseISValue(p)
		if err != nil {
			return nil, err
		}
		if is, ok := v.(IS); ok {
			vals[i] = is
		}
	}
	return NewMultiValue(vals), nil
}

func convertUI(b []byte) (UID, error) {
	s := strings.TrimRight(string(b), " \x00")
	return UID(s), nil
}

func convertPN(b []byte) (PersonName, error) {
	v, err := convertPNWithCharsets(b, nil)
	if err != nil {
		return PersonName{}, err
	}
	if pn, ok := v.(PersonName); ok {
		return pn, nil
	}
	return PersonName{}, fmt.Errorf("godicom: unexpected PN value type %T", v)
}

func convertPNWithCharsets(b []byte, charsets []string) (interface{}, error) {
	b = trimRightNullSpaceBytes(b)
	if len(charsets) == 0 || !needsCharsetDecode(b, charsets) {
		s := strings.TrimRight(string(b), " \x00")
		pn := ParsePersonName(s)
		pn.Original = s
		return pn, nil
	}
	if bytes.Contains(b, []byte{'\\'}) {
		rawParts := bytes.Split(b, []byte{'\\'})
		pns := make([]PersonName, len(rawParts))
		for i, part := range rawParts {
			pn, err := decodePersonNameBytes(part, charsets)
			if err != nil {
				return nil, err
			}
			pns[i] = pn
		}
		if len(pns) == 1 {
			return pns[0], nil
		}
		return NewMultiValue(pns), nil
	}
	return decodePersonNameBytes(b, charsets)
}

func decodePersonNameBytes(b []byte, charsets []string) (PersonName, error) {
	groups := bytes.Split(b, []byte{'='})
	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		parts = append(parts, DecodeBytesWithDelimiters(group, charsets, pnDelims))
	}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	pn := PersonName{}
	if len(parts) > 0 {
		pn.Alphabetic = parts[0]
	}
	if len(parts) > 1 {
		pn.Ideographic = parts[1]
	}
	if len(parts) > 2 {
		pn.Phonetic = parts[2]
	}
	pn.Original = personNameStringFromParts(parts)
	return pn, nil
}

func personNameStringFromParts(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + "=" + parts[1]
	default:
		return parts[0] + "=" + parts[1] + "=" + parts[2]
	}
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
	if len(b)%size != 0 {
		return nil, fmt.Errorf(
			"expected total bytes to be an even multiple of bytes per value",
		)
	}
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
