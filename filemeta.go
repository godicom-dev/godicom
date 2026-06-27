package godicom

import (
	"fmt"
)

// ValidateFileMeta validates File Meta Information elements.
// Mirrors pydicom.dataset.validate_file_meta.
func ValidateFileMeta(fileMeta *FileMetaDataset, enforceStandard bool) error {
	if fileMeta == nil {
		return nil
	}

	for _, elem := range fileMeta.Iter() {
		if elem.Tag.Group() != 0x0002 {
			return fmt.Errorf(
				"godicom: only File Meta Information group (0002,eeee) elements may be present in file meta",
			)
		}
	}

	if !enforceStandard {
		return nil
	}

	versionTag := MustTag("FileMetaInformationVersion")
	if elem, ok := fileMeta.Get(versionTag); !ok || elem.IsEmpty() {
		fileMeta.Set(NewDataElement(versionTag, VROB, []byte{0x00, 0x01}))
	}

	implClassTag := MustTag("ImplementationClassUID")
	if elem, ok := fileMeta.Get(implClassTag); !ok || elem.IsEmpty() {
		fileMeta.Set(NewDataElement(implClassTag, VRUI, GodicomImplementationUID))
	}

	implVersionTag := MustTag("ImplementationVersionName")
	if _, ok := fileMeta.Get(implVersionTag); !ok {
		fileMeta.Set(NewDataElement(implVersionTag, VRSH, "godicom"))
	}

	required := []struct {
		tag Tag
		kw  string
	}{
		{MustTag("MediaStorageSOPClassUID"), "MediaStorageSOPClassUID"},
		{MustTag("MediaStorageSOPInstanceUID"), "MediaStorageSOPInstanceUID"},
		{MustTag("TransferSyntaxUID"), "TransferSyntaxUID"},
	}
	var missing []string
	for _, req := range required {
		elem, ok := fileMeta.Get(req.tag)
		if !ok || elem.IsEmpty() {
			missing = append(missing, fmt.Sprintf("%s %s", req.tag, req.kw))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"godicom: required File Meta Information elements are missing or empty: %v",
			missing,
		)
	}

	return nil
}

func cloneFileMeta(fileMeta *FileMetaDataset) *FileMetaDataset {
	if fileMeta == nil {
		return NewFileMetaDataset()
	}
	clone := NewFileMetaDataset()
	for _, elem := range fileMeta.Iter() {
		clone.Set(cloneElement(elem))
	}
	return clone
}

func cloneElement(elem *Element) *Element {
	copied := &Element{
		Tag:               elem.Tag,
		VR:                elem.VR,
		Value:             elem.Value,
		FileTell:          elem.FileTell,
		ValueTell:         elem.ValueTell,
		ValueLength:       elem.ValueLength,
		Deferred:          elem.Deferred,
		IsImplicitVR:      elem.IsImplicitVR,
		IsLittleEndian:    elem.IsLittleEndian,
		IsUndefinedLength: elem.IsUndefinedLength,
		PrivateCreator:    elem.PrivateCreator,
	}
	if elem.RawValue != nil {
		copied.RawValue = append([]byte(nil), elem.RawValue...)
	}
	return copied
}

func cloneDataset(ds *Dataset) *Dataset {
	if ds == nil {
		return NewDataset()
	}
	clone := NewDataset()
	clone.originalEnc = ds.originalEnc
	clone.IsUndefinedLengthSequenceItem = ds.IsUndefinedLengthSequenceItem
	for _, elem := range ds.Iter() {
		clone.Set(cloneElement(elem))
	}
	return clone
}
