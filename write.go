package godicom

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// WriteOptions controls DICOM file writing behavior.
type WriteOptions struct {
	ImplicitVR        *bool
	LittleEndian      *bool
	EnforceFileFormat bool
}

type writeSource struct {
	dataset  *Dataset
	fileMeta *FileMetaDataset
	preamble []byte
}

// writeFile writes a Dataset to a DICOM file.
func writeFile(filename string, source writeSource, opts *WriteOptions) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if opts == nil {
		opts = &WriteOptions{}
	}

	if source.dataset == nil {
		return fmt.Errorf("godicom: missing dataset")
	}

	// Determine encoding
	isImplicit := false
	isLittleEndian := true

	if opts.ImplicitVR != nil {
		isImplicit = *opts.ImplicitVR
	} else {
		isImplicit = source.dataset.originalEnc.IsImplicitVR
	}

	if opts.LittleEndian != nil {
		isLittleEndian = *opts.LittleEndian
	} else {
		isLittleEndian = source.dataset.originalEnc.IsLittleEndian
	}

	preamble := source.preamble
	if len(preamble) == 0 {
		preamble = make([]byte, 128)
	}
	if len(preamble) != 128 {
		return fmt.Errorf("godicom: preamble must be 128 bytes, got %d", len(preamble))
	}
	// Write preamble (128 bytes + "DICM")
	if _, err := f.Write(preamble); err != nil {
		return err
	}
	if _, err := f.Write([]byte("DICM")); err != nil {
		return err
	}

	// Write File Meta Information (always Explicit VR Little Endian)
	fp := newDicomWriter(f)
	fp.SetByteOrder(true)

	if err := writeFileMetaInfo(fp, source.fileMeta); err != nil {
		return fmt.Errorf("godicom: error writing file meta: %w", err)
	}

	// Write dataset
	fp.SetByteOrder(isLittleEndian)

	if err := writeDataset(fp, source.dataset, isImplicit, isLittleEndian); err != nil {
		return fmt.Errorf("godicom: error writing dataset: %w", err)
	}

	return nil
}

func writeFileMetaInfo(fp *dicomIO, fileMeta *FileMetaDataset) error {
	if fileMeta == nil {
		return nil
	}
	// File Meta is always Explicit VR Little Endian
	for _, elem := range fileMeta.Iter() {
		if elem.Tag.Group() != 0x0002 {
			continue
		}
		if err := writeElement(fp, elem, false, true); err != nil {
			return err
		}
	}

	return nil
}

func writeDataset(fp *dicomIO, ds *Dataset, isImplicit, isLittleEndian bool) error {
	for _, elem := range ds.Iter() {
		if elem.Tag.Group() == 0x0002 {
			continue // Already written as file meta
		}
		if err := writeElement(fp, elem, isImplicit, isLittleEndian); err != nil {
			return err
		}
	}
	return nil
}

func writeElement(fp *dicomIO, elem *DataElement, isImplicit, isLittleEndian bool) error {
	fp.SetByteOrder(isLittleEndian)

	// Write tag
	if err := fp.WriteTag(elem.Tag); err != nil {
		return err
	}

	// Get encoded value
	encoded := encodeValue(elem, isLittleEndian)

	if isImplicit {
		// Implicit VR: tag + 4-byte length + value
		length := uint32(len(encoded))
		if elem.IsUndefinedLength {
			length = 0xFFFFFFFF
		}
		if err := fp.WriteUint32(length); err != nil {
			return err
		}
	} else {
		// Explicit VR: tag + VR + [2 reserved] + length (2 or 4) + value
		vr := string(elem.VR)
		if _, err := fp.Write([]byte(vr)); err != nil {
			return err
		}

		length := uint32(len(encoded))
		if elem.IsUndefinedLength {
			length = 0xFFFFFFFF
		}

		if ExplicitVRLength16[elem.VR] {
			if length > 0xFFFF {
				return fmt.Errorf("godicom: value too long for VR %s with 16-bit length: %d", elem.VR, length)
			}
			if err := fp.WriteUint16(uint16(length)); err != nil {
				return err
			}
		} else {
			// 2 reserved bytes + 4-byte length
			if _, err := fp.Write([]byte{0, 0}); err != nil {
				return err
			}
			if err := fp.WriteUint32(length); err != nil {
				return err
			}
		}
	}

	// Write value
	if len(encoded) > 0 {
		if _, err := fp.Write(encoded); err != nil {
			return err
		}
	}

	// Handle sequences with undefined length
	if elem.VR == VRSQ && elem.IsUndefinedLength {
		if seq, ok := elem.Value.(*Sequence); ok {
			for _, item := range seq.Items() {
				// Write Item tag
				if err := fp.WriteTag(ItemTag); err != nil {
					return err
				}
				if err := fp.WriteUint32(0xFFFFFFFF); err != nil {
					return err
				}
				// Write item contents
				if err := writeDataset(fp, item, isImplicit, isLittleEndian); err != nil {
					return err
				}
				// Write ItemDelimiter
				if err := fp.WriteTag(ItemDelimiterTag); err != nil {
					return err
				}
				if err := fp.WriteUint32(0); err != nil {
					return err
				}
			}
			// Write SequenceDelimiter
			if err := fp.WriteTag(SequenceDelimiterTag); err != nil {
				return err
			}
			if err := fp.WriteUint32(0); err != nil {
				return err
			}
		}
	}

	return nil
}

func encodeValue(elem *DataElement, le bool) []byte {
	if elem.Value == nil {
		return nil
	}

	switch elem.VR {
	case VRAE, VRAS, VRCS, VRDA, VRDT, VRLO, VRLT, VRSH, VRST, VRTM, VRUC, VRUR, VRUT:
		return encodeString(elem)
	case VRDS:
		return encodeNumberString(elem)
	case VRIS:
		return encodeNumberString(elem)
	case VRUI:
		return encodeString(elem)
	case VRPN:
		return encodePN(elem)
	case VRFD:
		return encodeFloats(elem, le, 8)
	case VRFL:
		return encodeFloats(elem, le, 4)
	case VRUL:
		return encodeInts(elem, le, 4, false)
	case VRUS:
		return encodeInts(elem, le, 2, false)
	case VRUV:
		return encodeInts(elem, le, 8, false)
	case VRSL:
		return encodeInts(elem, le, 4, true)
	case VRSS:
		return encodeInts(elem, le, 2, true)
	case VRSV:
		return encodeInts(elem, le, 8, true)
	case VRAT:
		return encodeAT(elem, le)
	case VROB, VROD, VROF, VROL, VROW, VROV, VRUN:
		return encodeBytes(elem)
	case VRSQ:
		return nil // Handled separately
	default:
		return encodeString(elem)
	}
}

func encodeString(elem *DataElement) []byte {
	if elem.Value == nil {
		return nil
	}
	switch v := elem.Value.(type) {
	case string:
		return []byte(v)
	case UID:
		return []byte(string(v))
	case PersonName:
		return []byte(v.String())
	case fmt.Stringer:
		return []byte(v.String())
	}
	return []byte(fmt.Sprintf("%v", elem.Value))
}

func encodeNumberString(elem *DataElement) []byte {
	if elem.Value == nil {
		return nil
	}
	switch v := elem.Value.(type) {
	case string:
		return []byte(v)
	case int:
		return []byte(fmt.Sprintf("%d", v))
	case float64:
		return []byte(fmt.Sprintf("%g", v))
	case *MultiValue[int]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += fmt.Sprintf("%d", val)
		}
		return []byte(s)
	case *MultiValue[float64]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += fmt.Sprintf("%g", val)
		}
		return []byte(s)
	case *MultiValue[interface{}]:
		s := ""
		for i, val := range v.Values() {
			if i > 0 {
				s += "\\"
			}
			s += fmt.Sprintf("%v", val)
		}
		return []byte(s)
	}
	return []byte(fmt.Sprintf("%v", elem.Value))
}

func encodePN(elem *DataElement) []byte {
	if elem.Value == nil {
		return nil
	}
	switch v := elem.Value.(type) {
	case PersonName:
		return []byte(v.String())
	case string:
		return []byte(v)
	}
	return []byte(fmt.Sprintf("%v", elem.Value))
}

func encodeFloats(elem *DataElement, le bool, size int) []byte {
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}

	var floats []float64
	switch v := elem.Value.(type) {
	case float64:
		floats = []float64{v}
	case float32:
		floats = []float64{float64(v)}
	case int:
		floats = []float64{float64(v)}
	case *MultiValue[float64]:
		floats = v.Values()
	case *MultiValue[interface{}]:
		for _, item := range v.Values() {
			switch x := item.(type) {
			case float64:
				floats = append(floats, x)
			case float32:
				floats = append(floats, float64(x))
			case int:
				floats = append(floats, float64(x))
			}
		}
	default:
		return nil
	}

	buf := make([]byte, len(floats)*size)
	for i, f := range floats {
		if size == 4 {
			order.PutUint32(buf[i*4:], math.Float32bits(float32(f)))
		} else {
			order.PutUint64(buf[i*8:], math.Float64bits(f))
		}
	}
	return buf
}

func encodeInts(elem *DataElement, le bool, size int, signed bool) []byte {
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}

	var ints []uint64
	switch v := elem.Value.(type) {
	case int:
		ints = []uint64{uint64(v)}
	case uint16:
		ints = []uint64{uint64(v)}
	case uint32:
		ints = []uint64{uint64(v)}
	case int32:
		ints = []uint64{uint64(v)}
	case int64:
		ints = []uint64{uint64(v)}
	case uint64:
		ints = []uint64{v}
	case *MultiValue[int]:
		for _, x := range v.Values() {
			ints = append(ints, uint64(x))
		}
	case *MultiValue[int64]:
		for _, x := range v.Values() {
			ints = append(ints, uint64(x))
		}
	case *MultiValue[uint64]:
		ints = v.Values()
	case *MultiValue[interface{}]:
		for _, item := range v.Values() {
			switch x := item.(type) {
			case int:
				ints = append(ints, uint64(x))
			case int64:
				ints = append(ints, uint64(x))
			case uint64:
				ints = append(ints, x)
			}
		}
	default:
		return nil
	}

	buf := make([]byte, len(ints)*size)
	for i, v := range ints {
		switch size {
		case 2:
			order.PutUint16(buf[i*2:], uint16(v))
		case 4:
			order.PutUint32(buf[i*4:], uint32(v))
		case 8:
			order.PutUint64(buf[i*8:], v)
		}
	}
	return buf
}

func encodeAT(elem *DataElement, le bool) []byte {
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}

	var tags []Tag
	switch v := elem.Value.(type) {
	case Tag:
		tags = []Tag{v}
	case int:
		tags = []Tag{Tag(v)}
	case *MultiValue[Tag]:
		tags = v.Values()
	case *MultiValue[interface{}]:
		for _, item := range v.Values() {
			switch x := item.(type) {
			case Tag:
				tags = append(tags, x)
			case int:
				tags = append(tags, Tag(x))
			}
		}
	default:
		return nil
	}

	buf := make([]byte, len(tags)*4)
	for i, t := range tags {
		order.PutUint32(buf[i*4:], uint32(t))
	}
	return buf
}

func encodeBytes(elem *DataElement) []byte {
	if elem.Value == nil {
		return nil
	}
	switch v := elem.Value.(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	}
	return nil
}

// WriteFile writes a Dataset to a DICOM file.
func WriteFile(filename string, ds *Dataset, opts *WriteOptions) error {
	return writeFile(filename, writeSource{dataset: ds}, opts)
}

// DcmWrite writes a Dataset to a DICOM file.
//
// Deprecated: use WriteFile.
func DcmWrite(filename string, ds *Dataset, opts *WriteOptions) error {
	return WriteFile(filename, ds, opts)
}

// Ensure binary is used
var _ = binary.BigEndian
var _ = io.Discard
