package godicom

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Dataset represents a DICOM Dataset - a collection of DataElements keyed by Tag.
type Dataset struct {
	elements                      map[Tag]*DataElement
	privateBlocks                 map[[2]interface{}]*PrivateBlock // key: (group, creator)
	originalEnc                   EncodingInfo
	originalCharsets              []string      // SpecificCharacterSet at read time; nil if unset/new
	writeEnc                      *EncodingInfo // nil = same as originalEnc for IsOriginalEncoding
	parent                        *Sequence
	IsUndefinedLengthSequenceItem bool
	readCtx                       *readContext
}

// readContext holds the source used for deferred element loading.
type readContext struct {
	data     []byte // in-memory source (ReadBytes / non-seekable Read)
	filename string // reopen path for streaming ReadFile
	modTime  int64
	size     int64 // file size when filename is used without data
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
	if !ok {
		return nil, false
	}
	if IsAmbiguousVR(e.VR) && e.IsRaw() {
		_ = correctAmbiguousVRElement(e, d, d.originalEnc.IsLittleEndian, d.ambiguousVRAncestors())
	}
	return e, true
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
	if element.VR == VRSQ {
		if seq, ok := element.Value.(*Sequence); ok && seq != nil {
			seq.owner = d
			for _, item := range seq.Items() {
				if item != nil {
					item.parent = seq
				}
			}
		}
	}
	// Replacing an element clears any prior raw bytes; caller-owned elements
	// created via NewElement do not carry RawValue unless set explicitly.
	d.elements[element.Tag] = element
}

func (d *Dataset) ambiguousVRAncestors() []*Dataset {
	ancestors := []*Dataset{d}
	cur := d
	for cur.parent != nil {
		owner := cur.parent.owner
		if owner == nil {
			break
		}
		ancestors = append(ancestors, owner)
		cur = owner
	}
	return ancestors
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
	vals, ok := d.GetFloats(tag)
	if !ok || len(vals) == 0 {
		return 0, false
	}
	return vals[0], true
}

// GetFloats returns all floating values for a DS/FD/FL multi-valued element.
func (d *Dataset) GetFloats(tag Tag) ([]float64, bool) {
	if err := d.loadDeferred(tag); err != nil {
		return nil, false
	}
	e, ok := d.elements[tag]
	if !ok || e.Value == nil {
		return nil, false
	}
	switch v := e.Value.(type) {
	case float64:
		return []float64{v}, true
	case DS:
		return []float64{v.Value}, true
	case string:
		ds, err := ParseDS(v)
		if err != nil {
			return nil, false
		}
		return []float64{ds.Value}, true
	case *MultiValue[float64]:
		out := make([]float64, v.Len())
		copy(out, v.Values())
		return out, true
	case *MultiValue[DS]:
		out := make([]float64, v.Len())
		for i, ds := range v.Values() {
			out[i] = ds.Value
		}
		return out, true
	case *MultiValue[string]:
		out := make([]float64, 0, v.Len())
		for _, s := range v.Values() {
			ds, err := ParseDS(s)
			if err != nil {
				return nil, false
			}
			out = append(out, ds.Value)
		}
		return out, true
	case *MultiValue[interface{}]:
		out := make([]float64, 0, v.Len())
		for _, item := range v.Values() {
			f, ok := floatFromValue(item)
			if !ok {
				return nil, false
			}
			out = append(out, f)
		}
		return out, true
	}
	return nil, false
}

func floatFromValue(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case DS:
		return x.Value, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		ds, err := ParseDS(x)
		if err != nil {
			return 0, false
		}
		return ds.Value, true
	default:
		return 0, false
	}
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

const (
	// DefaultElementFormat matches pydicom Dataset.default_element_format.
	DefaultElementFormat = "%(tag)s %(name)-35.35s %(VR)s: %(repval)s"
	// DefaultSequenceElementFormat matches pydicom Dataset.default_sequence_element_format.
	DefaultSequenceElementFormat = "%(tag)s %(name)-35.35s %(VR)s: %(repval)s"

	datasetIndentChars = "   "
)

// FormatLinesOptions controls Dataset.FormattedLines output.
type FormatLinesOptions struct {
	ElementFormat         string
	SequenceElementFormat string
}

func (d *Dataset) String() string {
	return d.prettyString(0, false)
}

// Top returns a string representation of only top-level elements.
// Mirrors pydicom Dataset.top.
func (d *Dataset) Top() string {
	return d.prettyString(0, true)
}

// FormattedLines returns formatted lines for every element, recursing into sequences.
// Mirrors pydicom Dataset.formatted_lines.
func (d *Dataset) FormattedLines(opts *FormatLinesOptions) []string {
	elemFmt := DefaultElementFormat
	seqFmt := DefaultSequenceElementFormat
	if opts != nil {
		if opts.ElementFormat != "" {
			elemFmt = opts.ElementFormat
		}
		if opts.SequenceElementFormat != "" {
			seqFmt = opts.SequenceElementFormat
		}
	}
	var out []string
	for _, elem := range d.IterAll() {
		if elem.VR == VRSQ {
			out = append(out, formatElementLine(elem, seqFmt))
		} else {
			out = append(out, formatElementLine(elem, elemFmt))
		}
	}
	return out
}

func (d *Dataset) prettyString(indent int, topLevelOnly bool) string {
	var lines []string
	indentStr := strings.Repeat(datasetIndentChars, indent)
	nextIndentStr := strings.Repeat(datasetIndentChars, indent+1)

	for _, elem := range d.Iter() {
		if elem.VR == VRSQ {
			n := 0
			seq, ok := elem.Value.(*Sequence)
			if ok {
				n = seq.Len()
			}
			lines = append(lines, fmt.Sprintf("%s%s  %s  %d item(s) ---- ", indentStr, elem.Tag, elem.Name(), n))
			if topLevelOnly || !ok {
				continue
			}
			for _, item := range seq.Items() {
				if item == nil {
					lines = append(lines, nextIndentStr+"---------")
					continue
				}
				nested := item.prettyString(indent+1, false)
				if nested != "" {
					lines = append(lines, nested)
				}
				lines = append(lines, nextIndentStr+"---------")
			}
			continue
		}
		lines = append(lines, indentStr+elem.String())
	}
	return strings.Join(lines, "\n")
}

func formatElementLine(elem *Element, format string) string {
	tag := elem.Tag.String()
	name := elem.Name()
	vr := string(elem.VR)
	repval := elem.ReprValue()

	out := format
	out = strings.ReplaceAll(out, "%(tag)s", tag)
	out = strings.ReplaceAll(out, "%(VR)s", vr)
	out = strings.ReplaceAll(out, "%(repval)s", repval)
	if strings.Contains(out, "%(name)-35.35s") {
		out = strings.ReplaceAll(out, "%(name)-35.35s", padTruncateRunes(name, 35))
	}
	out = strings.ReplaceAll(out, "%(name)s", name)
	return out
}

func padTruncateRunes(s string, width int) string {
	runes := []rune(s)
	if len(runes) > width {
		runes = runes[:width]
	}
	padded := string(runes)
	if n := width - len([]rune(padded)); n > 0 {
		padded += strings.Repeat(" ", n)
	}
	return padded
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

// IterAll returns all elements in tag order, recursing into sequences.
// Mirrors pydicom Dataset.iterall.
func (d *Dataset) IterAll() []*DataElement {
	var out []*DataElement
	d.Walk(func(_ *Dataset, elem *Element) {
		out = append(out, elem)
	}, true)
	return out
}

// Clear removes all elements from the dataset.
// Mirrors pydicom Dataset.clear.
func (d *Dataset) Clear() {
	d.elements = make(map[Tag]*DataElement)
	d.privateBlocks = make(map[[2]interface{}]*PrivateBlock)
}

// Pop removes and returns the element for tag.
// Mirrors pydicom Dataset.pop for tag keys.
func (d *Dataset) Pop(tag Tag) (*DataElement, bool) {
	elem, ok := d.Get(tag)
	if !ok {
		return nil, false
	}
	d.Delete(tag)
	return elem, true
}

// Update copies elements from other into d (overwriting matching tags).
// Mirrors pydicom Dataset.update for Dataset sources.
func (d *Dataset) Update(other *Dataset) {
	if other == nil {
		return
	}
	for _, elem := range other.Iter() {
		d.Set(cloneElement(elem))
	}
}

// GroupDataset returns a new dataset containing only elements of the given group.
// Mirrors pydicom Dataset.group_dataset.
func (d *Dataset) GroupDataset(group int) *Dataset {
	out := NewDataset()
	for _, elem := range d.Iter() {
		if int(elem.Tag.Group()) == group {
			out.Set(cloneElement(elem))
		}
	}
	return out
}

// RemovePrivateTags deletes all private elements, including nested sequences.
// Mirrors pydicom Dataset.remove_private_tags.
func (d *Dataset) RemovePrivateTags() {
	d.Walk(func(ds *Dataset, elem *Element) {
		if elem.IsPrivate() {
			ds.Delete(elem.Tag)
		}
	}, true)
}

// ElementByKeyword returns the element for a DICOM keyword, if present.
// Mirrors pydicom Dataset.data_element.
func (d *Dataset) ElementByKeyword(keyword string) (*DataElement, bool) {
	tag, err := TagFromKeyword(keyword)
	if err != nil {
		return nil, false
	}
	return d.Get(tag)
}

// Equal reports whether d and other contain the same tags, VRs, and values.
// Mirrors pydicom Dataset.__eq__ for Dataset values.
func (d *Dataset) Equal(other *Dataset) bool {
	if d == other {
		return true
	}
	if d == nil || other == nil {
		return false
	}
	if d.Len() != other.Len() {
		return false
	}
	for _, tag := range d.SortedTags() {
		a, okA := d.Get(tag)
		b, okB := other.Get(tag)
		if !okA || !okB || !a.Equal(b) {
			return false
		}
	}
	return true
}

// SetOriginalEncoding records the encoding used when the dataset was decoded.
// Mirrors pydicom Dataset.set_original_encoding.
func (d *Dataset) SetOriginalEncoding(isImplicit, isLittleEndian bool, charsets []string) {
	d.originalEnc = EncodingInfo{IsImplicitVR: isImplicit, IsLittleEndian: isLittleEndian}
	if charsets == nil {
		d.originalCharsets = []string{DefaultCharacterSet}
	} else {
		d.originalCharsets = ConvertCharacterSets(charsets)
	}
	enc := d.originalEnc
	d.writeEnc = &enc
}

// SetWriteEncoding sets the VR/endianness that would be used for writing.
// Used with IsOriginalEncoding; nil write encoding means "same as original".
func (d *Dataset) SetWriteEncoding(isImplicit, isLittleEndian bool) {
	d.writeEnc = &EncodingInfo{IsImplicitVR: isImplicit, IsLittleEndian: isLittleEndian}
}

// IsOriginalEncoding reports whether the current write encoding and
// SpecificCharacterSet match those captured when the dataset was read.
// Mirrors pydicom Dataset.is_original_encoding.
func (d *Dataset) IsOriginalEncoding() bool {
	if d == nil || d.originalCharsets == nil {
		return false
	}
	if charsetChanged(d) {
		return false
	}
	if d.writeEnc == nil {
		return true
	}
	return d.writeEnc.IsImplicitVR == d.originalEnc.IsImplicitVR &&
		d.writeEnc.IsLittleEndian == d.originalEnc.IsLittleEndian
}

// Clone returns a deep copy of the dataset, including sequence items.
func (d *Dataset) Clone() *Dataset {
	return cloneDataset(d)
}

// --- Save / Encode ---

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

// EncodeFile returns Part 10 DICOM file bytes for fd.
func (fd *FileDataset) EncodeFile(opts *WriteOptions) ([]byte, error) {
	return EncodeFile(fd, opts)
}

// Write encodes fd as a Part 10 DICOM file to w.
func (fd *FileDataset) Write(w io.Writer, opts *WriteOptions) error {
	return Write(w, fd, opts)
}

// Encode returns the dataset bytes (no preamble / File Meta) for transferSyntaxUID.
func (d *Dataset) Encode(transferSyntaxUID string) ([]byte, error) {
	return EncodeDataset(d, transferSyntaxUID)
}

// EncodeEncoding returns the dataset bytes using explicit VR/endian flags.
func (d *Dataset) EncodeEncoding(isImplicitVR, isLittleEndian bool) ([]byte, error) {
	return EncodeDatasetEncoding(d, isImplicitVR, isLittleEndian)
}

// --- Element count ---

func (d *Dataset) Len() int { return len(d.elements) }
