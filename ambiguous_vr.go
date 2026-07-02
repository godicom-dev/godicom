package godicom

import (
	"fmt"
)

// Ambiguous VR correction follows pydicom.filewriter.correct_ambiguous_vr.

var ambiguousUsSsTags = map[Tag]bool{
	MustTag(0x00189810): true,
	MustTag(0x00221452): true,
	MustTag(0x00280104): true,
	MustTag(0x00280105): true,
	MustTag(0x00280106): true,
	MustTag(0x00280107): true,
	MustTag(0x00280108): true,
	MustTag(0x00280109): true,
	MustTag(0x00280110): true,
	MustTag(0x00280111): true,
	MustTag(0x00280120): true,
	MustTag(0x00280121): true,
	MustTag(0x00281101): true,
	MustTag(0x00281102): true,
	MustTag(0x00281103): true,
	MustTag(0x00283002): true,
	MustTag(0x00409211): true,
	MustTag(0x00409216): true,
	MustTag(0x00603004): true,
	MustTag(0x00603006): true,
}

var ambiguousObOwTags = map[Tag]bool{
	MustTag(0x54000110): true,
	MustTag(0x54000112): true,
	MustTag(0x5400100A): true,
	MustTag(0x54001010): true,
}

func isOverlayDataTag(tag Tag) bool {
	group := tag.Group()
	return group >= 0x6000 && group <= 0x601E && group%2 == 0 && tag.Element() == 0x3000
}

func pixelRepresentationFromAncestors(ancestors []*Dataset) (int, bool) {
	for _, ds := range ancestors {
		if v, ok := ds.GetInt(MustTag(0x00280103)); ok {
			return v, true
		}
	}
	return 0, false
}

func elementRawBytes(elem *Element) []byte {
	if elem.RawValue != nil {
		return elem.RawValue
	}
	if b, ok := elem.Value.([]byte); ok {
		return b
	}
	return nil
}

func valueIsInt(v interface{}) bool {
	switch v.(type) {
	case int, int16, int32, int64, uint16, uint32, uint64:
		return true
	case *MultiValue[int]:
		return true
	case *MultiValue[int64]:
		return true
	default:
		return false
	}
}

func correctAmbiguousVRElement(elem *Element, ds *Dataset, isLittleEndian bool, ancestors []*Dataset) error {
	if len(ancestors) == 0 {
		ancestors = []*Dataset{ds}
	}

	switch {
	case elem.Tag == MustTag(0x7FE00010):
		if elem.IsUndefinedLength {
			elem.VR = VROB
			return nil
		}
		if ds.originalEnc.IsImplicitVR {
			elem.VR = VROW
			return nil
		}
		bits, ok := ds.GetInt(MustTag(0x00280100))
		if !ok {
			return fmt.Errorf("failed to resolve ambiguous VR for tag %s: missing 'BitsAllocated'", elem.Tag)
		}
		if bits > 8 {
			elem.VR = VROW
		} else {
			elem.VR = VROB
		}

	case ambiguousUsSsTags[elem.Tag]:
		pixelRep, hasRep := pixelRepresentationFromAncestors(ancestors)
		if !hasRep {
			if ds.Has(MustTag(0x7FE00010)) {
				return fmt.Errorf("failed to resolve ambiguous VR for tag %s: missing 'PixelRepresentation'", elem.Tag)
			}
			if repElem, ok := ds.Get(MustTag(0x00280103)); ok {
				if v, ok := repElem.Value.(int); ok && v == 0 {
					pixelRep = 0
				} else {
					pixelRep = 1
				}
			} else {
				pixelRep = 0
			}
		}

		if pixelRep == 0 {
			elem.VR = VRUS
		} else {
			elem.VR = VRSS
		}

		if elem.VM() == 0 {
			return nil
		}
		if !valueIsInt(elem.Value) {
			raw := elementRawBytes(elem)
			if raw == nil {
				return nil
			}
			converted, err := convertInts(raw, isLittleEndian, 2, pixelRep != 0)
			if err != nil {
				return fmt.Errorf("failed to resolve ambiguous VR for tag %s: %w", elem.Tag, err)
			}
			elem.Value = converted
			elem.RawValue = nil
		}

	case ambiguousObOwTags[elem.Tag]:
		if ds.originalEnc.IsImplicitVR {
			elem.VR = VROW
			return nil
		}
		bits, ok := ds.GetInt(MustTag(0x003A021A))
		if !ok {
			return fmt.Errorf("failed to resolve ambiguous VR for tag %s: missing 'WaveformBitsAllocated'", elem.Tag)
		}
		if bits > 8 {
			elem.VR = VROW
		} else {
			elem.VR = VROB
		}

	case elem.Tag == MustTag(0x00283006):
		lutDescriptor, ok := ds.Get(MustTag(0x00283002))
		if !ok {
			return fmt.Errorf("failed to resolve ambiguous VR for tag %s: missing 'LUTDescriptor'", elem.Tag)
		}
		first := lutDescriptorFirstValue(lutDescriptor)
		if first == 1 {
			elem.VR = VRUS
			if elem.VM() == 0 {
				return nil
			}
			if !valueIsInt(elem.Value) {
				raw := elementRawBytes(elem)
				if raw == nil {
					return nil
				}
				converted, err := convertInts(raw, isLittleEndian, 2, false)
				if err != nil {
					return fmt.Errorf("failed to resolve ambiguous VR for tag %s: %w", elem.Tag, err)
				}
				elem.Value = converted
				elem.RawValue = nil
			}
		} else {
			elem.VR = VROW
		}

	case isOverlayDataTag(elem.Tag):
		elem.VR = VROW
	}

	return nil
}

func lutDescriptorFirstValue(elem *Element) int {
	switch v := elem.Value.(type) {
	case int:
		return v
	case []int:
		if len(v) > 0 {
			return v[0]
		}
	case *MultiValue[int]:
		vals := v.Values()
		if len(vals) > 0 {
			return vals[0]
		}
	case *MultiValue[interface{}]:
		vals := v.Values()
		if len(vals) > 0 {
			if i, ok := vals[0].(int); ok {
				return i
			}
		}
	case []byte:
		if len(v) >= 2 {
			return int(uint16(v[0]) | uint16(v[1])<<8)
		}
	}
	return 0
}

// CorrectAmbiguousVR walks ds correcting ambiguous VR elements when possible.
// Mirrors pydicom.filewriter.correct_ambiguous_vr.
func CorrectAmbiguousVR(ds *Dataset, isLittleEndian bool, ancestors []*Dataset) error {
	if ancestors == nil {
		ancestors = []*Dataset{ds}
	}

	for _, tag := range ds.SortedTags() {
		elem, ok := ds.Get(tag)
		if !ok {
			continue
		}
		if elem.VR == VRSQ {
			seq, ok := elem.Value.(*Sequence)
			if !ok || seq == nil {
				continue
			}
			for _, item := range seq.Items() {
				childAncestors := append([]*Dataset{item}, ancestors...)
				if err := CorrectAmbiguousVR(item, isLittleEndian, childAncestors); err != nil {
					return err
				}
			}
			continue
		}
		if IsAmbiguousVR(elem.VR) {
			if err := correctAmbiguousVRElement(elem, ds, isLittleEndian, ancestors); err != nil {
				return err
			}
		}
	}
	return nil
}
