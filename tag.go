package godicom

import (
	"fmt"
	"strconv"
	"strings"
)

// Tag represents a DICOM element (group, element) tag as a 32-bit integer.
type Tag uint32

const (
	ItemTag             Tag = 0xFFFEE000
	ItemDelimiterTag    Tag = 0xFFFEE00D
	SequenceDelimiterTag Tag = 0xFFFEE0DD
	TagPixelRep         Tag = 0x00280103
	TagCharset          Tag = 0x00080005
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
	return Tag((group << 16) | element)
}

func (t Tag) Group() int     { return int(t >> 16) }
func (t Tag) Element() int   { return int(t & 0xFFFF) }
func (t Tag) IsPrivate() bool { return t.Group()%2 == 1 }

func (t Tag) IsPrivateCreator() bool {
	return t.IsPrivate() && 0x0010 <= t.Element() && t.Element() < 0x0100
}

func (t Tag) PrivateCreator() Tag {
	return Tag((uint32(t) & 0xFFFF0000) | uint32(t.Element()>>8))
}

func (t Tag) String() string {
	return fmt.Sprintf("(%04X,%04X)", t.Group(), t.Element())
}

func (t Tag) JSONKey() string {
	return fmt.Sprintf("%04X%04X", t.Group(), t.Element())
}

func (t Tag) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
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

// LUT descriptor tags where first and third values are always US
var lutDescriptorTags = map[Tag]bool{
	MustTag(0x00281101): true,
	MustTag(0x00281102): true,
	MustTag(0x00281103): true,
	MustTag(0x00283002): true,
}
