package godicom

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// FileSet is a DICOM File-set read from a DICOMDIR.
//
// The directory records stay in the order their offsets link them: Patient,
// Study, Series, then the instance record, or whatever record types the file
// actually uses. Referenced File ID is resolved against the directory that
// contains the DICOMDIR.
//
// Writing a DICOMDIR, adding or removing instances, and folding orphaned
// records back in by re-reading the instances are not part of this reader.
// An orphan is a directory record the offset links never reach. By default
// those records are left out of the tree. FileSetOptions.RaiseOrphans reports
// them instead.
type FileSet struct {
	Dataset  *FileDataset
	RootPath string
	ID       string
	UID      UID
	Tree     *RecordNode

	instances []*FileInstance
}

// FileSetOptions controls how a DICOMDIR is loaded.
type FileSetOptions struct {
	// RaiseOrphans returns an error when the DICOMDIR contains a directory
	// record that is not linked from the root.
	RaiseOrphans bool
}

// RecordNode is one directory record in the File-set tree.
// The root node's Record is nil; its children are the top-level records.
type RecordNode struct {
	Record   *Dataset
	Offset   int64
	Parent   *RecordNode
	Children []*RecordNode
	Instance *FileInstance
}

// FileInstance is one SOP instance named by a directory record's Referenced File ID.
type FileInstance struct {
	Node *RecordNode
	fs   *FileSet
	rel  string
}

// OpenFileSet reads the DICOMDIR at path.
func OpenFileSet(path string, opts *FileSetOptions) (*FileSet, error) {
	ds, err := ReadFile(path, nil)
	if err != nil {
		return nil, err
	}
	return LoadFileSet(ds, opts)
}

// LoadFileSet reads a File-set from a DICOMDIR dataset.
// The dataset's Filename must be the path of that DICOMDIR file. The File-set
// root is the directory containing it.
func LoadFileSet(ds *FileDataset, opts *FileSetOptions) (*FileSet, error) {
	if ds == nil || ds.FileMeta == nil {
		return nil, fmt.Errorf("godicom: unable to load the File-set as the supplied dataset is not a 'Media Storage Directory' instance")
	}
	class, _ := ds.FileMeta.GetString(tag.MediaStorageSOPClassUID)
	if class != string(uid.MediaStorageDirectoryStorage) {
		return nil, fmt.Errorf("godicom: unable to load the File-set as the supplied dataset is not a 'Media Storage Directory' instance")
	}
	if ts, ok := ds.TransferSyntaxUID(); ok && ts != uid.ExplicitVRLittleEndian {
		logFileSet(ds, "The DICOMDIR dataset uses an invalid transfer syntax '"+ts.Name()+"' and will be updated to use 'Explicit VR Little Endian'")
	}
	if ds.Filename == "" {
		return nil, fmt.Errorf("godicom: unable to load the File-set as the DICOMDIR dataset must have a 'filename' attribute set to the path of the DICOMDIR file")
	}
	abs, err := filepath.Abs(ds.Filename)
	if err != nil {
		return nil, fmt.Errorf("godicom: unable to load the File-set as the 'filename' attribute for the DICOMDIR dataset is not a valid path: %s", ds.Filename)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("godicom: unable to load the File-set as the 'filename' attribute for the DICOMDIR dataset is not a valid path: %s", ds.Filename)
	}

	fs := &FileSet{
		Dataset:  ds,
		RootPath: filepath.Dir(abs),
		Tree:     &RecordNode{},
	}
	if id, ok := ds.GetString(tag.FileSetID); ok {
		fs.ID = id
	}
	if u, ok := ds.FileMeta.GetString(tag.MediaStorageSOPInstanceUID); ok {
		fs.UID = UID(u)
	}
	if err := fs.parseRecords(opts); err != nil {
		return nil, err
	}
	return fs, nil
}

// Len returns the number of instances whose referenced file is inside the File-set.
func (fs *FileSet) Len() int {
	if fs == nil {
		return 0
	}
	return len(fs.instances)
}

// Instances returns the instances in tree order.
func (fs *FileSet) Instances() []*FileInstance {
	if fs == nil {
		return nil
	}
	out := make([]*FileInstance, len(fs.instances))
	copy(out, fs.instances)
	return out
}

// Find returns instances whose directory records match every tag in query.
// An empty query returns every instance. SOP Instance UID, SOP Class UID and
// Transfer Syntax UID match the referenced-file elements, and a record is
// searched from the instance's own record up through its ancestors.
func (fs *FileSet) Find(query map[Tag]string) []*FileInstance {
	if fs == nil {
		return nil
	}
	if len(query) == 0 {
		return fs.Instances()
	}
	hasElements := false
	var matches []*FileInstance
	for _, inst := range fs.instances {
		if inst.hasAll(query) {
			hasElements = true
		}
		if inst.matches(query) {
			matches = append(matches, inst)
		}
	}
	if !hasElements {
		logFileSet(fs.Dataset, "None of the records in the DICOMDIR dataset contain all the query elements, consider using the 'load' parameter to expand the search to the corresponding SOP instances")
	}
	return matches
}

// Type returns the Directory Record Type, or an empty string for the root.
func (n *RecordNode) Type() string {
	if n == nil || n.Record == nil {
		return ""
	}
	s, _ := n.Record.GetString(tag.DirectoryRecordType)
	return strings.TrimSpace(s)
}

// Path returns the absolute path of the referenced file.
func (f *FileInstance) Path() string {
	if f == nil || f.fs == nil {
		return ""
	}
	return filepath.Join(f.fs.RootPath, f.rel)
}

// FileID returns the Referenced File ID relative to the File-set root.
func (f *FileInstance) FileID() string {
	if f == nil {
		return ""
	}
	return f.rel
}

// SOPInstanceUID returns the referenced SOP Instance UID.
func (f *FileInstance) SOPInstanceUID() (string, bool) {
	return f.GetString(tag.SOPInstanceUID)
}

// SOPClassUID returns the referenced SOP Class UID.
func (f *FileInstance) SOPClassUID() (string, bool) {
	return f.GetString(tag.SOPClassUID)
}

// TransferSyntaxUID returns the referenced Transfer Syntax UID.
func (f *FileInstance) TransferSyntaxUID() (string, bool) {
	return f.GetString(tag.TransferSyntaxUID)
}

// Get returns the element with tg from this instance's directory records,
// searching from the instance record up through its ancestors.
func (f *FileInstance) Get(tg Tag) (*DataElement, bool) {
	if f == nil || f.Node == nil {
		return nil, false
	}
	tg = referencedFileTag(tg)
	for n := f.Node; n != nil && n.Record != nil; n = n.Parent {
		if elem, ok := n.Record.Get(tg); ok && elem != nil {
			return elem, true
		}
	}
	return nil, false
}

// GetString returns the string value of the element Get would find.
func (f *FileInstance) GetString(tg Tag) (string, bool) {
	elem, ok := f.Get(tg)
	if !ok {
		return "", false
	}
	return elementString(elem), true
}

// Load reads the referenced SOP instance.
func (f *FileInstance) Load() (*FileDataset, error) {
	if f == nil || f.Path() == "" {
		return nil, fmt.Errorf("godicom: file instance has no path")
	}
	return ReadFile(f.Path(), nil)
}

func (fs *FileSet) parseRecords(opts *FileSetOptions) error {
	seq, ok := fs.Dataset.GetSequence(tag.DirectoryRecordSequence)
	if !ok || seq == nil || seq.Len() == 0 {
		return nil
	}
	byOffset := make(map[uint64]*RecordNode, seq.Len())
	var records []*RecordNode
	for _, item := range seq.Items() {
		if item == nil || item.ItemOffset == 0 {
			return fmt.Errorf("godicom: directory record is missing its file offset")
		}
		node := &RecordNode{Record: item, Offset: item.ItemOffset}
		byOffset[uint64(item.ItemOffset)] = node
		records = append(records, node)
	}
	first := directoryOffset(fs.Dataset.Dataset, tag.OffsetOfTheFirstDirectoryRecordOfTheRootDirectoryEntity)
	if first != 0 {
		if err := attachSiblings(fs.Tree, first, byOffset); err != nil {
			return err
		}
		if err := linkLower(fs.Tree, byOffset); err != nil {
			return err
		}
	}
	var orphans int
	for _, node := range records {
		if node.Parent == nil {
			orphans++
		}
	}
	if orphans > 0 && opts != nil && opts.RaiseOrphans {
		return fmt.Errorf("godicom: The DICOMDIR contains orphaned directory records")
	}
	return fs.collect(fs.Tree)
}

func linkLower(n *RecordNode, byOffset map[uint64]*RecordNode) error {
	if n.Record != nil {
		lower := directoryOffset(n.Record, tag.OffsetOfReferencedLowerLevelDirectoryEntity)
		if err := attachSiblings(n, lower, byOffset); err != nil {
			return err
		}
	}
	for _, child := range n.Children {
		if err := linkLower(child, byOffset); err != nil {
			return err
		}
	}
	return nil
}

func attachSiblings(parent *RecordNode, first uint64, byOffset map[uint64]*RecordNode) error {
	seen := map[uint64]bool{}
	for off := first; off != 0; {
		if seen[off] {
			return fmt.Errorf("godicom: directory record offset cycle at %d", off)
		}
		seen[off] = true
		node, ok := byOffset[off]
		if !ok {
			return fmt.Errorf("godicom: directory record offset %d does not match a record", off)
		}
		if node.Parent != nil {
			return fmt.Errorf("godicom: directory record at offset %d is linked more than once", off)
		}
		node.Parent = parent
		parent.Children = append(parent.Children, node)
		off = directoryOffset(node.Record, tag.OffsetOfTheNextDirectoryRecord)
	}
	return nil
}

func (fs *FileSet) collect(n *RecordNode) error {
	if n.Record != nil && len(n.Children) == 0 && !hasReferencedFileID(n.Record) {
		n.unlink()
		return nil
	}
	parts, ok := referencedFileID(n.Record)
	if ok && len(parts) > 0 {
		rel, err := fileIDRelative(parts)
		if err != nil {
			return err
		}
		full := filepath.Join(fs.RootPath, rel)
		if _, err := os.Stat(full); err != nil {
			logFileSet(fs.Dataset, fmt.Sprintf("The referenced SOP Instance for the directory record at offset %d does not exist: %s", n.Offset, full))
		} else {
			inst := &FileInstance{Node: n, fs: fs, rel: rel}
			n.Instance = inst
			fs.instances = append(fs.instances, inst)
		}
	}
	kids := append([]*RecordNode(nil), n.Children...)
	for _, child := range kids {
		if child.Parent != n {
			continue
		}
		if err := fs.collect(child); err != nil {
			return err
		}
	}
	return nil
}

func (n *RecordNode) unlink() {
	parent := n.Parent
	if parent == nil {
		return
	}
	kept := parent.Children[:0]
	for _, child := range parent.Children {
		if child != n {
			kept = append(kept, child)
		}
	}
	parent.Children = kept
	n.Parent = nil
	if len(parent.Children) == 0 && parent.Record != nil {
		parent.unlink()
	}
}

func (f *FileInstance) hasAll(query map[Tag]string) bool {
	for tg := range query {
		if _, ok := f.Get(tg); !ok {
			return false
		}
	}
	return true
}

func (f *FileInstance) matches(query map[Tag]string) bool {
	for tg, want := range query {
		got, ok := f.GetString(tg)
		if !ok || got != want {
			return false
		}
	}
	return true
}

func referencedFileTag(tg Tag) Tag {
	switch tg {
	case tag.SOPInstanceUID:
		return tag.ReferencedSOPInstanceUIDInFile
	case tag.SOPClassUID:
		return tag.ReferencedSOPClassUIDInFile
	case tag.TransferSyntaxUID:
		return tag.ReferencedTransferSyntaxUIDInFile
	default:
		return tg
	}
}

func hasReferencedFileID(ds *Dataset) bool {
	if ds == nil {
		return false
	}
	_, ok := ds.Get(tag.ReferencedFileID)
	return ok
}

func referencedFileID(ds *Dataset) ([]string, bool) {
	if ds == nil {
		return nil, false
	}
	elem, ok := ds.Get(tag.ReferencedFileID)
	if !ok || elem == nil || elem.Value == nil {
		return nil, false
	}
	var raw []string
	switch v := elem.Value.(type) {
	case string:
		if strings.TrimRight(v, " \x00") == "" {
			return nil, true
		}
		raw = strings.Split(v, `\`)
	case []string:
		raw = v
	case *MultiValue[string]:
		raw = v.Values()
	default:
		s := strings.TrimRight(fmt.Sprint(v), " \x00")
		if s == "" {
			return nil, true
		}
		raw = strings.Split(s, `\`)
	}
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimRight(part, " \x00")
		if part != "" {
			out = append(out, part)
		}
	}
	return out, true
}

func fileIDRelative(parts []string) (string, error) {
	for _, part := range parts {
		if part == "" || filepath.IsAbs(part) {
			return "", fileIDEscape(parts)
		}
	}
	rel := filepath.Clean(filepath.Join(parts...))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fileIDEscape(parts)
	}
	return rel, nil
}

func fileIDEscape(parts []string) error {
	return fmt.Errorf("godicom: ReferencedFileID ('%s') must be inside the DICOMDIR root path", strings.Join(parts, string(filepath.Separator)))
}

func directoryOffset(ds *Dataset, tg Tag) uint64 {
	if ds == nil {
		return 0
	}
	elem, ok := ds.Get(tg)
	if !ok || elem == nil || elem.Value == nil {
		return 0
	}
	switch v := elem.Value.(type) {
	case uint64:
		return v
	case uint32:
		return uint64(v)
	case int:
		if v <= 0 {
			return 0
		}
		return uint64(v)
	case int64:
		if v <= 0 {
			return 0
		}
		return uint64(v)
	default:
		return 0
	}
}

func elementString(elem *DataElement) string {
	if elem == nil || elem.Value == nil {
		return ""
	}
	switch v := elem.Value.(type) {
	case string:
		return v
	case UID:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func logFileSet(ds *FileDataset, msg string) {
	ctx := context.Background()
	if ds != nil && ds.Dataset != nil && ds.readCtx != nil {
		ctx = ds.readCtx.logCtx()
	}
	l := LoggerFromContext(ctx)
	if l.Enabled(ctx, slog.LevelWarn) {
		l.WarnContext(ctx, msg)
	}
}
