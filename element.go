package godicom

import (
	"fmt"
)

// Element holds a single DICOM data element.
type Element struct {
	Tag               Tag
	VR                VR
	Value             interface{}
	RawValue          []byte // original value bytes from read; written verbatim when set
	ValueTell         int64  // value offset in source; used for deferred reads
	ValueLength       uint32 // byte length when deferred
	Deferred          bool   // value not yet loaded from source
	IsImplicitVR      bool   // encoding at read time; for deferred load
	IsLittleEndian    bool
	readCharsets      []string // charset active when element was read (deferred decode)
	FileTell          int64
	IsUndefinedLength bool
	PrivateCreator    string
}

// DataElement is an alias for Element.
type DataElement = Element

func NewElement(tag Tag, vr VR, value interface{}) *Element {
	return &Element{
		Tag:   tag,
		VR:    vr,
		Value: value,
	}
}

// NewDataElement creates a DICOM data element.
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
	case string:
		if len(v) == 0 {
			return 0
		}
		return 1
	case []byte:
		if len(v) == 0 {
			return 0
		}
		return 1
	case PersonName:
		if v.Alphabetic == "" && v.Ideographic == "" && v.Phonetic == "" {
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
	case *MultiValue[PersonName]:
		return v.Len()
	case *MultiValue[uint16]:
		return v.Len()
	case *MultiValue[uint32]:
		return v.Len()
	case *MultiValue[Tag]:
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

func (e *Element) IsRaw() bool {
	return len(e.RawValue) > 0
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
