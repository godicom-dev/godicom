package godicom

import (
	"fmt"
	"strings"
)

// Element holds a single DICOM data element.
type Element struct {
	Tag               Tag
	VR                VR
	Value             interface{}
	FileTell          int64
	IsUndefinedLength bool
	PrivateCreator    string
}

// DataElement holds a single DICOM data element.
//
// Deprecated: use Element.
type DataElement = Element

func NewElement(tag Tag, vr VR, value interface{}) *Element {
	return &Element{
		Tag:   tag,
		VR:    vr,
		Value: value,
	}
}

// NewDataElement creates a DICOM data element.
//
// Deprecated: use NewElement.
func NewDataElement(tag Tag, vr VR, value interface{}) *DataElement {
	return NewElement(tag, vr, value)
}

func (e *Element) String() string {
	name := e.Name()
	val := e.ReprValue()
	return fmt.Sprintf("%s %-35s %s: %s", e.Tag, name, e.VR, val)
}

func (e *Element) ReprValue() string {
	if e.Value == nil {
		return ""
	}
	switch v := e.Value.(type) {
	case UID:
		return v.Name()
	case *Sequence:
		return fmt.Sprintf("Sequence of %d items", v.Len())
	case []byte:
		if len(v) > 16 {
			return fmt.Sprintf("Array of %d bytes", len(v))
		}
		return fmt.Sprintf("%v", v)
	case *MultiValue[interface{}]:
		if v.Len() > 16 {
			return fmt.Sprintf("Array of %d elements", v.Len())
		}
		return fmt.Sprintf("%v", v.Values())
	default:
		s := fmt.Sprintf("%v", v)
		if len(s) > 64 {
			return s[:61] + "..."
		}
		return s
	}
}

func (e *Element) Name() string {
	if e.Tag.IsPrivate() {
		if e.PrivateCreator != "" {
			if name, ok := privateDictLookup(e.Tag, e.PrivateCreator); ok {
				return "[" + name + "]"
			}
		}
		if e.Tag.Element()>>8 == 0 {
			return "Private Creator"
		}
		return "Private tag data"
	}
	if name, ok := dictionaryDescription(e.Tag); ok {
		return name
	}
	if e.Tag.Element() == 0 {
		return "Group Length"
	}
	return ""
}

func (e *Element) Keyword() string {
	if kw, ok := keywordForTag(e.Tag); ok {
		return kw
	}
	return ""
}

func (e *Element) VM() int {
	if e.VR == VRSQ {
		if seq, ok := e.Value.(*Sequence); ok {
			return seq.Len()
		}
		return 0
	}
	if e.Value == nil {
		return 0
	}
	switch v := e.Value.(type) {
	case string, []byte:
		if len(v.(string)) == 0 {
			return 0
		}
		return 1
	case *MultiValue[interface{}]:
		return v.Len()
	case *MultiValue[string]:
		return v.Len()
	case *MultiValue[int]:
		return v.Len()
	case *MultiValue[float64]:
		return v.Len()
	case *Sequence:
		return v.Len()
	default:
		return 1
	}
}

func (e *Element) IsEmpty() bool {
	return e.VM() == 0
}

func (e *Element) IsPrivate() bool {
	return e.Tag.IsPrivate()
}

func (e *Element) Equal(other *Element) bool {
	if e.Tag != other.Tag || e.VR != other.VR {
		return false
	}
	return fmt.Sprintf("%v", e.Value) == fmt.Sprintf("%v", other.Value)
}

// RawDataElement holds raw (undecoded) element data from a file.
type RawDataElement struct {
	Tag            Tag
	VR             VR
	Length         uint32
	Value          []byte
	ValueTell      int64
	IsImplicitVR   bool
	IsLittleEndian bool
	IsRaw          bool
}

// PersonName holds a DICOM Person Name (PN) value.
type PersonName struct {
	Alphabetic  string
	Ideographic string
	Phonetic    string
}

func ParsePersonName(s string) PersonName {
	parts := strings.Split(s, "=")
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
	return pn
}

func (pn PersonName) String() string {
	parts := make([]string, 0, 3)
	if pn.Alphabetic != "" {
		parts = append(parts, pn.Alphabetic)
	}
	if pn.Ideographic != "" {
		parts = append(parts, pn.Ideographic)
	}
	if pn.Phonetic != "" {
		parts = append(parts, pn.Phonetic)
	}
	return strings.Join(parts, "=")
}
