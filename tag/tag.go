package tag

import (
	"fmt"
	"strconv"
	"strings"
)

// Tag represents a DICOM element tag as a 32-bit integer.
type Tag uint32

const (
	ItemDelimiter     Tag = 0xFFFEE00D
	SequenceDelimiter Tag = 0xFFFEE0DD
)

// New creates a tag from group and element numbers.
func New(group, element int) Tag {
	return Tag((group << 16) | element)
}

// Parse creates a tag from a hex string, tuple string, keyword, or integer string.
func Parse(s string) (Tag, error) {
	if strings.HasPrefix(s, "(") && strings.Contains(s, ",") {
		s = strings.Trim(s, "()")
		parts := strings.Split(s, ",")
		if len(parts) == 2 {
			group, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 16, 32)
			if err != nil {
				return 0, fmt.Errorf("tag: invalid group %q: %w", parts[0], err)
			}
			element, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 16, 32)
			if err != nil {
				return 0, fmt.Errorf("tag: invalid element %q: %w", parts[1], err)
			}
			return New(int(group), int(element)), nil
		}
	}

	if val, err := strconv.ParseInt(s, 16, 32); err == nil {
		return Tag(val), nil
	}

	if tag, ok := ByKeyword(s); ok {
		return tag, nil
	}

	return 0, fmt.Errorf("tag: unknown keyword %q", s)
}

// MustParse is like Parse but panics on error.
func MustParse(s string) Tag {
	tag, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return tag
}

// FromKeyword returns the tag for a DICOM keyword.
func FromKeyword(keyword string) (Tag, error) {
	tag, ok := ByKeyword(keyword)
	if !ok {
		return 0, fmt.Errorf("tag: unknown keyword %q", keyword)
	}
	return tag, nil
}

func (t Tag) Group() int {
	return int(t >> 16)
}

func (t Tag) Element() int {
	return int(t & 0xFFFF)
}

func (t Tag) IsPrivate() bool {
	return t.Group()%2 == 1
}

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
