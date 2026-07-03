package godicom

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Dataset represents a DICOM Dataset - a collection of DataElements keyed by Tag.
type Dataset struct {
	elements                      map[Tag]*DataElement
	privateBlocks                 map[[2]interface{}]*PrivateBlock // key: (group, creator)
	originalEnc                   EncodingInfo
	parent                        *Sequence
	IsUndefinedLengthSequenceItem bool
	readCtx                       *readContext
}

// readContext holds the source used for deferred element loading.
type readContext struct {
	data     []byte
	filename string
	modTime  int64
}

// EncodingInfo describes the DICOM encoding used when reading/writing.
type EncodingInfo struct {
	IsImplicitVR   bool
	IsLittleEndian bool
}

// FileMetaDataset holds DICOM File Meta Information (group 0x0002).
type FileMetaDataset struct {
	*Dataset
}

// FileDataset extends Dataset with file-specific info.
type FileDataset struct {
	*Dataset
	Filename  string
	Preamble  []byte
	FileMeta  *FileMetaDataset
	Timestamp string // file modification time as Unix seconds (deferred read checks)
}

// PrivateBlock represents a private block in the dataset.
type PrivateBlock struct {
	Group          int
	PrivateCreator string
	dataset        *Dataset
	blockStart     int
}

func NewDataset() *Dataset {
	return &Dataset{
		elements:      make(map[Tag]*DataElement),
		privateBlocks: make(map[[2]interface{}]*PrivateBlock),
		originalEnc:   EncodingInfo{IsImplicitVR: false, IsLittleEndian: true},
	}
}

func NewFileMetaDataset() *FileMetaDataset {
	return &FileMetaDataset{Dataset: NewDataset()}
}

// --- Element access ---

func (d *Dataset) Get(tag Tag) (*DataElement, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return nil, false
	}
	e, ok := d.elements[tag]
	return e, ok
}

// LoadDeferred reads a deferred element's value from the source file/buffer.
// Mirrors pydicom deferred read triggered by Dataset.__getitem__.
func (d *Dataset) LoadDeferred(tag Tag) error {
	return d.loadDeferred(tag)
}

func (d *Dataset) loadDeferred(tag Tag) error {
	elem, ok := d.elements[tag]
	if !ok || !elem.Deferred {
		return nil
	}
	if d.readCtx == nil {
		return fmt.Errorf("godicom: deferred read requires source data")
	}
	return loadDeferredElement(d.readCtx, d, elem)
}

func (d *Dataset) Set(element *DataElement) {
	// Replacing an element clears any prior raw bytes; caller-owned elements
	// created via NewElement do not carry RawValue unless set explicitly.
	d.elements[element.Tag] = element
}

func (d *Dataset) Delete(tag Tag) {
	delete(d.elements, tag)
}

func (d *Dataset) Has(tag Tag) bool {
	_, ok := d.elements[tag]
	return ok
}

func (d *Dataset) Elements() map[Tag]*DataElement {
	elements := make(map[Tag]*DataElement, len(d.elements))
	for tag, elem := range d.elements {
		elements[tag] = elem
	}
	return elements
}

// SortedTags returns all tags in ascending order.
func (d *Dataset) SortedTags() []Tag {
	tags := make([]Tag, 0, len(d.elements))
	for t := range d.elements {
		tags = append(tags, t)
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i] < tags[j] })
	return tags
}

// Iter returns all elements sorted by tag.
func (d *Dataset) Iter() []*DataElement {
	tags := d.SortedTags()
	elems := make([]*DataElement, len(tags))
	for i, t := range tags {
		elems[i] = d.elements[t]
	}
	return elems
}

// --- Convenience getters ---

func (d *Dataset) GetString(tag Tag) (string, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return "", false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return "", false
	}
	switch v := e.Value.(type) {
	case string:
		return v, true
	case PersonName:
		return v.String(), true
	case DA:
		return v.String(), true
	case TM:
		return v.String(), true
	case DT:
		return v.String(), true
	case DS:
		return v.String(), true
	case IS:
		return v.String(), true
	case UID:
		return string(v), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

func (d *Dataset) GetInt(tag Tag) (int, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return 0, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return 0, false
	}
	switch v := e.Value.(type) {
	case int:
		return v, true
	case int16:
		return int(v), true
	case uint16:
		return int(v), true
	case int32:
		return int(v), true
	case uint32:
		return int(v), true
	case int64:
		return int(v), true
	case uint64:
		return int(v), true
	case IS:
		return int(v.Value), true
	case string:
		is, err := ParseIS(v)
		if err != nil {
			return 0, false
		}
		return int(is.Value), true
	}
	return 0, false
}

func (d *Dataset) GetDA(tag Tag) (DA, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return DA{}, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return DA{}, false
	}
	switch v := e.Value.(type) {
	case DA:
		return v, true
	case string:
		da, err := ParseDA(v)
		if err != nil {
			return DA{}, false
		}
		return da, true
	}
	return DA{}, false
}

func (d *Dataset) GetTM(tag Tag) (TM, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return TM{}, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return TM{}, false
	}
	switch v := e.Value.(type) {
	case TM:
		return v, true
	case string:
		tm, err := ParseTM(v)
		if err != nil {
			return TM{}, false
		}
		return tm, true
	}
	return TM{}, false
}

func (d *Dataset) GetDT(tag Tag) (DT, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return DT{}, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return DT{}, false
	}
	switch v := e.Value.(type) {
	case DT:
		return v, true
	case string:
		dt, err := ParseDT(v)
		if err != nil {
			return DT{}, false
		}
		return dt, true
	}
	return DT{}, false
}

func (d *Dataset) GetPN(tag Tag) (PersonName, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return PersonName{}, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return PersonName{}, false
	}
	switch v := e.Value.(type) {
	case PersonName:
		return v, true
	case string:
		return ParsePersonName(v), true
	}
	return PersonName{}, false
}

func (d *Dataset) GetDS(tag Tag) (DS, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return DS{}, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return DS{}, false
	}
	switch v := e.Value.(type) {
	case DS:
		return v, true
	case float64:
		return DS{Value: v, Original: strconv.FormatFloat(v, 'g', -1, 64)}, true
	case string:
		ds, err := ParseDS(v)
		if err != nil {
			return DS{}, false
		}
		return ds, true
	}
	return DS{}, false
}

func (d *Dataset) GetIS(tag Tag) (IS, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return IS{}, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return IS{}, false
	}
	switch v := e.Value.(type) {
	case IS:
		return v, true
	case int:
		return IS{Value: int64(v), Original: strconv.FormatInt(int64(v), 10)}, true
	case int64:
		return IS{Value: v, Original: strconv.FormatInt(v, 10)}, true
	case string:
		is, err := ParseIS(v)
		if err != nil {
			return IS{}, false
		}
		return is, true
	}
	return IS{}, false
}

func (d *Dataset) GetFloat(tag Tag) (float64, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return 0, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return 0, false
	}
	switch v := e.Value.(type) {
	case float64:
		return v, true
	case DS:
		return v.Value, true
	case string:
		ds, err := ParseDS(v)
		if err != nil {
			return 0, false
		}
		return ds.Value, true
	}
	return 0, false
}

func (d *Dataset) GetBytes(tag Tag) ([]byte, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return nil, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return nil, false
	}
	b, ok := e.Value.([]byte)
	return b, ok
}

func (d *Dataset) GetSequence(tag Tag) (*Sequence, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return nil, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return nil, false
	}
	s, ok := e.Value.(*Sequence)
	return s, ok
}

func (d *Dataset) StringValue(tag Tag) (string, bool) {
	return d.GetString(tag)
}

func (d *Dataset) IntValue(tag Tag) (int, bool) {
	return d.GetInt(tag)
}

func (d *Dataset) FloatValue(tag Tag) (float64, bool) {
	return d.GetFloat(tag)
}

func (d *Dataset) BytesValue(tag Tag) ([]byte, bool) {
	return d.GetBytes(tag)
}

func (d *Dataset) SequenceValue(tag Tag) (*Sequence, bool) {
	return d.GetSequence(tag)
}

func (d *Dataset) GetDataElement(tag Tag) *DataElement {
	if err := d.loadDeferred(tag); err != nil {
		return nil
	}
	return d.elements[tag]
}

// --- Private blocks ---

func (d *Dataset) PrivateBlock(group int, creator string) *PrivateBlock {
	key := [2]interface{}{group, creator}
	if pb, ok := d.privateBlocks[key]; ok {
		return pb
	}
	// Find the private creator element
	for _, e := range d.elements {
		if e.Tag.Group() == group && e.Tag.Element() >= 0x0010 && e.Tag.Element() < 0x0100 {
			if s, ok := e.Value.(string); ok && s == creator {
				pb := &PrivateBlock{
					Group:          group,
					PrivateCreator: creator,
					dataset:        d,
					blockStart:     e.Tag.Element() << 8,
				}
				d.privateBlocks[key] = pb
				return pb
			}
		}
	}
	return nil
}

func (pb *PrivateBlock) GetTag(offset int) Tag {
	return NewTag(pb.Group, pb.blockStart+offset)
}

func (pb *PrivateBlock) Get(offset int) (*DataElement, bool) {
	return pb.dataset.Get(pb.GetTag(offset))
}

func (pb *PrivateBlock) Set(offset int, vr VR, value interface{}) {
	tag := pb.GetTag(offset)
	pb.dataset.Set(NewDataElement(tag, vr, value))
}

// --- String ---

func (d *Dataset) String() string {
	var b strings.Builder
	for _, elem := range d.Iter() {
		b.WriteString(elem.String())
		b.WriteString("\n")
	}
	return b.String()
}

// WalkFunc is called for each element in a Dataset during Walk.
type WalkFunc func(ds *Dataset, elem *Element)

// Walk visits each element in tag order, optionally recursing into sequences.
// Mirrors pydicom Dataset.walk.
func (d *Dataset) Walk(fn WalkFunc, recursive bool) {
	d.walk(fn, recursive)
}

func (d *Dataset) walk(fn WalkFunc, recursive bool) {
	for _, tag := range d.SortedTags() {
		elem, ok := d.Get(tag)
		if !ok {
			continue
		}
		fn(d, elem)
		if !recursive || elem.VR != VRSQ {
			continue
		}
		seq, ok := elem.Value.(*Sequence)
		if !ok {
			continue
		}
		for _, item := range seq.Items() {
			item.walk(fn, recursive)
		}
	}
}

// Clone returns a deep copy of the dataset, including sequence items.
func (d *Dataset) Clone() *Dataset {
	return cloneDataset(d)
}

// --- Save ---

func (d *Dataset) SaveAs(filename string, opts *WriteOptions) error {
	return WriteFile(filename, d, opts)
}

func (fd *FileDataset) SaveAs(filename string, opts *WriteOptions) error {
	return writeFile(filename, writeSource{
		dataset:  fd.Dataset,
		fileMeta: fd.FileMeta,
		preamble: fd.Preamble,
	}, opts)
}

// --- Element count ---

func (d *Dataset) Len() int { return len(d.elements) }
