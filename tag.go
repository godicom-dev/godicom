package godicom

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/godicom-dev/godicom/tag"
)

// Tag represents a DICOM element (group, element) tag as a 32-bit integer.
type Tag = tag.Tag

const (
	ItemTag              Tag = tag.Item
	ItemDelimiterTag     Tag = tag.ItemDelimiter
	SequenceDelimiterTag Tag = tag.SequenceDelimiter
	TagPixelRep          Tag = tag.PixelRepresentation
	TagCharset           Tag = tag.SpecificCharacterSet
)

// ParseTag creates a Tag from various forms:
//   - ParseTag(0x00100010)
//   - ParseTag(0x0010, 0x0010)
//   - ParseTag("PatientName")
//   - ParseTag("00100010")
func ParseTag(arg interface{}, arg2 ...int) (Tag, error) {
	if t, ok := arg.(Tag); ok {
		return t, nil
	}

	if len(arg2) > 0 {
		return NewTag(toInt(arg), arg2[0]), nil
	}

	switch v := arg.(type) {
	case int:
		return Tag(v), nil
	case uint32:
		return Tag(v), nil
	case [2]int:
		return NewTag(v[0], v[1]), nil
	case [2]uint32:
		return NewTag(int(v[0]), int(v[1])), nil
	case string:
		if strings.HasPrefix(v, "(") && strings.Contains(v, ",") {
			v = strings.Trim(v, "()")
			parts := strings.Split(v, ",")
			if len(parts) == 2 {
				g, _ := strconv.ParseInt(strings.TrimSpace(parts[0]), 16, 32)
				e, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 16, 32)
				return NewTag(int(g), int(e)), nil
			}
		}
		if val, err := strconv.ParseInt(v, 16, 32); err == nil {
			return Tag(val), nil
		}
		if tag, ok := tagForKeyword(v); ok {
			return tag, nil
		}
		return 0, fmt.Errorf("godicom: unknown tag keyword %q", v)
	}
	return 0, fmt.Errorf("godicom: cannot create tag from %T(%v)", arg, arg)
}

// MustTag is like ParseTag but panics on error.
func MustTag(arg interface{}, arg2 ...int) Tag {
	t, err := ParseTag(arg, arg2...)
	if err != nil {
		panic(err)
	}
	return t
}

// NewTag creates a tag from group and element numbers.
func NewTag(group, element int) Tag {
	return tag.New(group, element)
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case uint32:
		return int(n)
	case uint16:
		return int(n)
	case int32:
		return int(n)
	}
	return 0
}

