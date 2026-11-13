package godicom

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// This file builds a File-set: it creates the empty one, files instances under
// the directory records they need, and lays the files out under the File IDs
// the records imply. It is pydicom's FileSet.add(), FileSet.remove() and
// FileSet.write(), minus the staging directory pydicom writes each addition to
// before it knows where the instance belongs: a Go dataset is already in
// memory, so Add keeps it there and Save writes it once.

// FileSetWriteOptions controls how a File-set is written.
type FileSetWriteOptions struct {
	// ForceImplicitVR writes the DICOMDIR as Implicit VR Little Endian.
	// PS3.10 requires Explicit VR Little Endian, so this produces a file
	// strict readers reject; it exists for the readers that only accept it.
	ForceImplicitVR bool

	// UseExisting keeps the directory layout the File-set already has and
	// only rewrites the DICOMDIR. Without it every instance is filed under
	// the File ID its records imply, which is what a File-set built here
	// uses and what pydicom writes by default.
	UseExisting bool
}

// NewFileSet creates an empty File-set with a new Media Storage SOP Instance
// UID.
//
// The File-set has no root path until Save is given one, and no File-set ID:
// PS3.10 makes the ID optional, and pydicom leaves it out of a new File-set
// too. Set FileSet.ID before saving to write one.
func NewFileSet() (*FileSet, error) {
	u, err := uid.GenerateUID()
	if err != nil {
		return nil, fmt.Errorf("godicom: unable to create a File-set: %w", err)
	}

	ds := NewDataset()
	ds.Set(NewDataElement(tag.OffsetOfTheFirstDirectoryRecordOfTheRootDirectoryEntity, VRUL, uint32(0)))
	ds.Set(NewDataElement(tag.OffsetOfTheLastDirectoryRecordOfTheRootDirectoryEntity, VRUL, uint32(0)))
	ds.Set(NewDataElement(tag.FileSetConsistencyFlag, VRUS, uint16(0)))
	ds.Set(NewDataElement(tag.DirectoryRecordSequence, VRSQ, NewSequence(nil)))

	meta := NewFileMetaDataset()
	meta.Set(NewDataElement(tag.MediaStorageSOPClassUID, VRUI, uid.MediaStorageDirectoryStorage))
	meta.Set(NewDataElement(tag.MediaStorageSOPInstanceUID, VRUI, string(u)))
	meta.Set(NewDataElement(tag.TransferSyntaxUID, VRUI, uid.ExplicitVRLittleEndian))

	return &FileSet{
		Dataset: &FileDataset{Dataset: ds, FileMeta: meta},
		UID:     u,
		Tree:    &RecordNode{},
	}, nil
}

// Add stages an instance for addition to the File-set and returns it.
//
// The instance's directory records are built by DirectoryRecords, so an
// instance that is missing an element PS3.3 Section F.5 requires is refused
// here rather than when the File-set is written. An instance already in the
// File-set, matched by SOP Instance UID, is returned as it is instead of being
// added twice.
//
// The instance is kept in memory; Save writes it to the path its records
// imply.
func (fs *FileSet) Add(ds *FileDataset) (*FileInstance, error) {
	if fs == nil || fs.Dataset == nil {
		return nil, fmt.Errorf("godicom: unable to add an instance to a nil File-set")
	}
	if ds == nil || ds.Dataset == nil {
		return nil, fmt.Errorf("godicom: unable to add a nil instance to the File-set")
	}
	if fs.Tree == nil {
		fs.Tree = &RecordNode{}
	}

	if sop, ok := ds.GetString(tag.SOPInstanceUID); ok {
		if existing := fs.instanceBySOP(sop); existing != nil {
			return existing, nil
		}
	}

	records, err := DirectoryRecords(ds)
	if err != nil {
		return nil, err
	}

	nodes := make([]*RecordNode, len(records))
	for i, record := range records {
		nodes[i] = &RecordNode{Record: record}
	}
	for i := 1; i < len(nodes); i++ {
		nodes[i].Parent = nodes[i-1]
		nodes[i-1].Children = append(nodes[i-1].Children, nodes[i])
	}
	fs.Tree.insertRecordChain(nodes[0])

	leaf := nodes[len(nodes)-1]
	inst := &FileInstance{Node: leaf, fs: fs, ds: ds}
	leaf.Instance = inst
	fs.instances = append(fs.instances, inst)
	return inst, nil
}

// AddFile reads the instance at path and adds it.
func (fs *FileSet) AddFile(path string) (*FileInstance, error) {
	ds, err := ReadFile(path, nil)
	if err != nil {
		return nil, err
	}
	return fs.Add(ds)
}

// AddCustom stages an instance under directory records the caller supplies.
//
// Add builds the records from the instance, which is what a conformant
// File-set needs; this takes them as they are, which is the only way to write
// a PRIVATE record: the standard gives private records no elements of their
// own that say where they belong, so no recorder could derive one. The records
// are the caller's, and leaf's ancestors have to be set: the chain leaf hangs
// from is inserted at the point where it meets the tree.
//
// The elements that link a record to a file are set here rather than by the
// caller -- Referenced File ID, and the Referenced SOP Class, SOP Instance and
// Transfer Syntax UIDs -- because they come from the instance.
func (fs *FileSet) AddCustom(ds *FileDataset, leaf *RecordNode) (*FileInstance, error) {
	if fs == nil || fs.Dataset == nil {
		return nil, fmt.Errorf("godicom: unable to add an instance to a nil File-set")
	}
	if ds == nil || ds.Dataset == nil {
		return nil, fmt.Errorf("godicom: unable to add a nil instance to the File-set")
	}
	if leaf == nil || leaf.Record == nil {
		return nil, fmt.Errorf("godicom: unable to add an instance without a directory record to file it under")
	}
	if len(leaf.Children) > 0 {
		return nil, fmt.Errorf("godicom: unable to add an instance under the %q record: it has records below it", leaf.Type())
	}
	// A File-set is eight levels deep at most, and the leaf is the eighth.
	if depth := leaf.depth(); depth > 7 {
		return nil, fmt.Errorf("godicom: unable to add an instance under the %q record: it is %d levels down, and a File-set is 8", leaf.Type(), depth+1)
	}
	top := leaf.top()
	if top.Parent != nil {
		return nil, fmt.Errorf("godicom: unable to add an instance under the %q record: its ancestors are already in a File-set", leaf.Type())
	}
	sop, ok := ds.GetString(tag.SOPInstanceUID)
	if !ok || sop == "" {
		return nil, fmt.Errorf("godicom: unable to add an instance without a SOP Instance UID")
	}
	if existing := fs.instanceBySOP(sop); existing != nil {
		return existing, nil
	}

	// pydicom normalises every record it wraps: the flag says the record is in
	// use, and the two offsets are set by the writer anyway.
	for n := leaf; n != nil && n.Record != nil; n = n.Parent {
		if err := checkCustomRecord(n.Record); err != nil {
			return nil, err
		}
		normaliseRecord(n.Record)
	}

	leaf.Record.Set(NewDataElement(tag.ReferencedFileID, VRCS, nil))
	leaf.Record.Set(NewDataElement(tag.ReferencedSOPInstanceUIDInFile, VRUI, sop))
	if class, ok := ds.GetString(tag.SOPClassUID); ok {
		leaf.Record.Set(NewDataElement(tag.ReferencedSOPClassUIDInFile, VRUI, class))
	}
	if ts, ok := ds.TransferSyntaxUID(); ok {
		leaf.Record.Set(NewDataElement(tag.ReferencedTransferSyntaxUIDInFile, VRUI, ts))
	}

	fs.Tree.insertRecordChain(top)
	inst := &FileInstance{Node: leaf, fs: fs, ds: ds}
	leaf.Instance = inst
	fs.instances = append(fs.instances, inst)
	return inst, nil
}

// instanceBySOP returns the instance with the given SOP Instance UID.
func (fs *FileSet) instanceBySOP(sop string) *FileInstance {
	if sop == "" {
		return nil
	}
	for _, inst := range fs.instances {
		if existing, ok := inst.SOPInstanceUID(); ok && existing == sop {
			return inst
		}
	}
	return nil
}

// checkCustomRecord reports why a record handed to AddCustom cannot join a
// File-set: it needs a Directory Record Type, and the element that makes it
// unique among its siblings. pydicom refuses both, and a File-set written with
// either missing is not one another reader can file anything into.
func checkCustomRecord(rec *Dataset) error {
	recordType, ok := rec.GetString(tag.DirectoryRecordType)
	if !ok || strings.TrimSpace(recordType) == "" {
		return fmt.Errorf("godicom: unable to add a directory record without a Directory Record Type")
	}
	if recordKey(rec) != "" {
		return nil
	}
	return fmt.Errorf("godicom: the %q directory record is missing its %s, which identifies it among its siblings", recordType, recordKeyName(recordType))
}

// recordKeyName returns the element that recordKey reads for a record type.
func recordKeyName(recordType string) string {
	switch recordType {
	case "PATIENT":
		return "Patient ID"
	case "STUDY":
		return "Study Instance UID or Referenced SOP Instance UID in File"
	case "SERIES":
		return "Series Instance UID"
	case "PRIVATE":
		return "Private Record UID"
	default:
		return "Referenced SOP Instance UID in File"
	}
}

// normaliseRecord sets the elements every directory record carries and that
// the writer would otherwise have to invent.
func normaliseRecord(rec *Dataset) {
	rec.Set(NewDataElement(tag.RecordInUseFlag, VRUS, uint16(0xFFFF)))
	if _, ok := rec.Get(tag.OffsetOfTheNextDirectoryRecord); !ok {
		rec.Set(NewDataElement(tag.OffsetOfTheNextDirectoryRecord, VRUL, uint32(0)))
	}
	if _, ok := rec.Get(tag.OffsetOfReferencedLowerLevelDirectoryEntity); !ok {
		rec.Set(NewDataElement(tag.OffsetOfReferencedLowerLevelDirectoryEntity, VRUL, uint32(0)))
	}
}

// Remove takes an instance out of the File-set.
//
// Only the instance record is removed; an ancestor that has no other children
// goes with it. The file the instance pointed at is left on disk, so a
// mistaken removal can be undone by adding it back; pydicom deletes it
// instead.
func (fs *FileSet) Remove(inst *FileInstance) error {
	if inst == nil || inst.Node == nil {
		return fmt.Errorf("godicom: unable to remove a nil instance")
	}
	if len(inst.Node.Children) > 0 {
		return fmt.Errorf("godicom: unable to remove the %q record: it still has records below it", inst.Node.Type())
	}

	kept := fs.instances[:0]
	found := false
	for _, existing := range fs.instances {
		if existing == inst {
			found = true
			continue
		}
		kept = append(kept, existing)
	}
	fs.instances = kept
	if !found {
		return fmt.Errorf("godicom: unable to remove the instance: it is not in this File-set")
	}

	inst.Node.Instance = nil
	inst.Node.unlink()
	return nil
}

// Save writes the File-set to root: every instance at the File ID its records
// imply, then the DICOMDIR.
//
// root may be empty for a File-set that already has one, which is written in
// place; a File-set created by NewFileSet needs the path given. A File-set that
// already has a root cannot be moved to another one -- CopyTo copies it
// somewhere else and leaves it where it is.
//
// Every instance the File-set holds is written from the dataset Add was given,
// so an instance that was never saved does not have to exist on disk. An
// instance read from a File-set is moved to the path its records imply, unless
// UseExisting is set, in which case the layout is left alone and only the
// DICOMDIR changes.
func (fs *FileSet) Save(root string, opts *FileSetWriteOptions) error {
	if fs == nil || fs.Dataset == nil {
		return fmt.Errorf("godicom: unable to save a nil File-set")
	}
	useExisting := opts != nil && opts.UseExisting
	var forceImplicit bool
	if opts != nil {
		forceImplicit = opts.ForceImplicitVR
	}

	if useExisting && fs.hasStagedAddition() {
		return fmt.Errorf("godicom: unable to keep the existing layout: instances staged for addition need one")
	}

	switch {
	case fs.RootPath == "" && root == "":
		return fmt.Errorf("godicom: unable to save a new File-set without a root directory")
	case fs.RootPath == "":
		abs, err := filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("godicom: the File-set root %q is not a valid path: %w", root, err)
		}
		fs.RootPath = abs
	case root != "":
		abs, err := filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("godicom: the File-set root %q is not a valid path: %w", root, err)
		}
		if !samePath(abs, fs.RootPath) {
			return fmt.Errorf("godicom: cannot change the root of an existing File-set from %s to %s", fs.RootPath, abs)
		}
	}

	if err := os.MkdirAll(fs.RootPath, 0o755); err != nil {
		return err
	}

	// Six numeric digits hold a million siblings; past that the File IDs have
	// to use the alphanumeric set, which is the threshold pydicom switches at.
	if len(fs.instances) > 1_000_000 {
		fs.alphanumericIDs = true
	}

	// Where each instance goes. UseExisting keeps what the DICOMDIR said.
	destinations := make([]string, len(fs.instances))
	for i, inst := range fs.instances {
		if useExisting {
			destinations[i] = inst.rel
			continue
		}
		rel, err := fs.fileIDFor(inst.Node)
		if err != nil {
			return err
		}
		destinations[i] = rel
	}

	// Check every destination before anything moves: a half-written layout
	// with instances pointing at files that were not written yet is worse than
	// a refusal.
	for i, inst := range fs.instances {
		dest := filepath.Join(fs.RootPath, destinations[i])
		if inst.ds != nil {
			// An addition: the destination must be free, or already ours
			// when the same instance is saved twice.
			if _, err := os.Lstat(dest); err == nil {
				return fmt.Errorf("godicom: unable to add the instance: %s already exists", dest)
			}
			continue
		}
		if filepath.Clean(fs.RootPath+string(filepath.Separator)+inst.rel) == filepath.Clean(dest) {
			continue
		}
		if _, err := os.Lstat(dest); err == nil {
			return fmt.Errorf("godicom: unable to move the instance to %s: it already exists", dest)
		}
	}

	// Write the additions and move the instances whose File ID changed.
	moved := make([]string, len(fs.instances))
	for i, inst := range fs.instances {
		dest := filepath.Join(fs.RootPath, destinations[i])
		switch {
		case inst.ds != nil:
			data, err := EncodeFile(inst.ds, &WriteOptions{EnforceFileFormat: true})
			if err != nil {
				return fmt.Errorf("godicom: unable to encode the instance to add: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			if err := writeFileBytes(dest, data); err != nil {
				return err
			}
			moved[i] = dest
		case destinations[i] != inst.rel:
			src := filepath.Join(fs.RootPath, inst.rel)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			if err := os.Rename(src, dest); err != nil {
				return fmt.Errorf("godicom: unable to move %s to %s: %w", src, dest, err)
			}
			moved[i] = dest
		}
	}

	// The records have to name the files that were just written before the
	// DICOMDIR is encoded.
	for i, inst := range fs.instances {
		if moved[i] == "" {
			continue
		}
		inst.rel = destinations[i]
		parts := strings.Split(filepath.ToSlash(inst.rel), "/")
		if !IsConformantFileID(parts) {
			return fmt.Errorf("godicom: the File ID %q is not conformant to PS3.10 Section 8.2", inst.rel)
		}
		inst.Node.Record.Set(NewDataElement(tag.ReferencedFileID, VRCS, parts))
	}

	dicomdirPath := filepath.Join(fs.RootPath, "DICOMDIR")
	if err := fs.WriteDICOMDIR(dicomdirPath, &WriteDICOMDIROptions{ForceImplicitVR: forceImplicit}); err != nil {
		return err
	}

	// Reload so the File-set describes the file rather than what was intended.
	reloaded, err := OpenFileSet(dicomdirPath, nil)
	if err != nil {
		return err
	}
	fs.Dataset = reloaded.Dataset
	fs.Tree = reloaded.Tree
	fs.instances = reloaded.instances
	fs.ID = reloaded.ID
	fs.UID = reloaded.UID
	for _, inst := range fs.instances {
		inst.fs = fs
	}
	return nil
}

// CopyTo copies the File-set to root and returns the File-set that lives
// there.
//
// Every instance is copied to the File ID its records imply, so the copy is
// laid out the way a File-set built here is rather than the way the original
// happened to be, and then the DICOMDIR is written. The original is left as it
// was, down to the Referenced File ID of every record, which is why a copy can
// be taken of a File-set that is only partly written.
//
// The destination has to be somewhere else and empty: a DICOMDIR that already
// exists there, or a file where an instance would go, is a refusal rather than
// an overwrite.
func (fs *FileSet) CopyTo(root string, opts *WriteDICOMDIROptions) (*FileSet, error) {
	if fs == nil || fs.Dataset == nil {
		return nil, fmt.Errorf("godicom: unable to copy a nil File-set")
	}
	if root == "" {
		return nil, fmt.Errorf("godicom: unable to copy the File-set without a destination directory")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("godicom: the File-set destination %q is not a valid path: %w", root, err)
	}
	if fs.RootPath != "" && samePath(abs, fs.RootPath) {
		return nil, fmt.Errorf("godicom: unable to copy the File-set: %s is already its root", abs)
	}
	if len(fs.instances) > maxFileSetInstances {
		return nil, fmt.Errorf("godicom: unable to copy a File-set with %d instances: at most %d fit in the File ID character set", len(fs.instances), maxFileSetInstances)
	}
	if len(fs.instances) > 1_000_000 {
		fs.alphanumericIDs = true
	}

	dicomdirPath := filepath.Join(abs, "DICOMDIR")
	if _, err := os.Lstat(dicomdirPath); err == nil {
		return nil, fmt.Errorf("godicom: unable to copy the File-set: %s already exists", dicomdirPath)
	}

	// Where each instance goes, worked out before anything is written so a
	// refusal leaves the destination untouched.
	dests := make([]string, len(fs.instances))
	for i, inst := range fs.instances {
		rel, err := fs.fileIDFor(inst.Node)
		if err != nil {
			return nil, err
		}
		dests[i] = rel
		if _, err := os.Lstat(filepath.Join(abs, rel)); err == nil {
			return nil, fmt.Errorf("godicom: unable to copy the File-set: %s already exists", filepath.Join(abs, rel))
		}
	}

	// The records have to name the copies while the DICOMDIR is encoded; the
	// originals go back afterwards, so copying does not move the File-set it
	// copied.
	type hadFileID struct {
		elem *DataElement
		ok   bool
	}
	before := make([]hadFileID, len(fs.instances))
	for i, inst := range fs.instances {
		elem, ok := inst.Node.Record.Get(tag.ReferencedFileID)
		before[i] = hadFileID{elem, ok}
	}
	restore := func() {
		for i, inst := range fs.instances {
			if before[i].ok {
				inst.Node.Record.Set(before[i].elem)
				continue
			}
			inst.Node.Record.Delete(tag.ReferencedFileID)
		}
	}

	for i, inst := range fs.instances {
		dest := filepath.Join(abs, dests[i])
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			restore()
			return nil, err
		}
		if inst.ds != nil {
			data, err := EncodeFile(inst.ds, &WriteOptions{EnforceFileFormat: true})
			if err != nil {
				restore()
				return nil, fmt.Errorf("godicom: unable to encode the instance to copy: %w", err)
			}
			if err := writeFileBytes(dest, data); err != nil {
				restore()
				return nil, err
			}
		} else if err := copyInstanceFile(inst.Path(), dest); err != nil {
			restore()
			return nil, fmt.Errorf("godicom: unable to copy %s: %w", inst.Path(), err)
		}
		parts := strings.Split(filepath.ToSlash(dests[i]), "/")
		if !IsConformantFileID(parts) {
			restore()
			return nil, fmt.Errorf("godicom: the File ID %q is not conformant to PS3.10 Section 8.2", dests[i])
		}
		inst.Node.Record.Set(NewDataElement(tag.ReferencedFileID, VRCS, parts))
	}

	err = fs.WriteDICOMDIR(dicomdirPath, opts)
	restore()
	if err != nil {
		return nil, err
	}
	return OpenFileSet(dicomdirPath, nil)
}

// copyInstanceFile copies src to dst, which must not exist.
func copyInstanceFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// insertRecordChain files the chain starting at top in the tree.
//
// A File-set holds one record per patient, study and series, so the chain
// shares the records it already has: the walk descends while the next record
// in the chain matches a child of the tree, and the rest of the chain is
// attached where it stops. This is pydicom's RecordNode.add().
func (n *RecordNode) insertRecordChain(top *RecordNode) {
	current := n
	node := top
	for {
		match := current.childWithKey(recordKey(node.Record))
		if match == nil || len(node.Children) == 0 {
			break
		}
		current = match
		node = node.Children[0]
	}
	node.Parent = current
	current.Children = append(current.Children, node)
}

// childWithKey returns the child whose record has key, or nil.
func (n *RecordNode) childWithKey(key string) *RecordNode {
	if key == "" {
		return nil
	}
	for _, child := range n.Children {
		if recordKey(child.Record) == key {
			return child
		}
	}
	return nil
}

// recordKey returns what makes a record unique among its siblings, as PS3.3
// Annex F.5 defines it: the Patient ID for a PATIENT record, the Study
// Instance UID for a STUDY, the Series Instance UID for a SERIES, and the
// referenced instance's SOP Instance UID for everything else.
//
// An empty key means the record cannot be matched, so it is always added as a
// new branch rather than merged into another one.
func recordKey(rec *Dataset) string {
	if rec == nil {
		return ""
	}
	recordType, _ := rec.GetString(tag.DirectoryRecordType)
	switch recordType {
	case "PATIENT":
		key, _ := rec.GetString(tag.PatientID)
		return key
	case "STUDY":
		if key, ok := rec.GetString(tag.StudyInstanceUID); ok && key != "" {
			return key
		}
		key, _ := rec.GetString(tag.ReferencedSOPInstanceUIDInFile)
		return key
	case "SERIES":
		key, _ := rec.GetString(tag.SeriesInstanceUID)
		return key
	case "PRIVATE":
		key, _ := rec.GetString(tag.PrivateRecordUID)
		return key
	default:
		key, _ := rec.GetString(tag.ReferencedSOPInstanceUIDInFile)
		return key
	}
}

// fileIDFor returns the File ID the tree puts node at, as a path relative to
// the File-set root.
func (fs *FileSet) fileIDFor(node *RecordNode) (string, error) {
	var parts []string
	for n := node; n != nil && n.Record != nil; n = n.Parent {
		component, err := fs.fileIDComponent(n)
		if err != nil {
			return "", err
		}
		parts = append(parts, component)
	}
	// Walked from the leaf up, so the components come out reversed.
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	if !IsConformantFileID(parts) {
		return "", fmt.Errorf("godicom: the File ID %q is not conformant to PS3.10 Section 8.2", strings.Join(parts, "/"))
	}
	return filepath.Join(parts...), nil
}

// fileIDComponent returns the single File ID component for one record: the two
// characters its record type gives it, then its index among its siblings in
// the File ID character set.
func (fs *FileSet) fileIDComponent(node *RecordNode) (string, error) {
	recordType := node.Type()
	prefix, ok := recordTypePrefixes[recordType]
	if !ok {
		return "", fmt.Errorf("godicom: no File ID prefix for the %q record type", recordType)
	}
	if recordType == "PRIVATE" {
		// Private records are only distinguished by their level.
		prefix += strconv.Itoa(node.depth())
	}

	chars := fileIDNumeric
	if fs.alphanumeric() {
		chars = fileIDAlphanumeric
	}
	base := int64(len(chars))
	width := 8 - len(prefix)
	if width < 0 {
		width = 0
	}

	index := int64(node.index())
	var digits []byte
	if index == 0 && width > 0 {
		digits = []byte{'0'}
	}
	for index > 0 {
		digits = append(digits, chars[index%base])
		index /= base
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	if len(digits) > width {
		return "", fmt.Errorf("godicom: the %q record has more siblings than a File ID component can hold", recordType)
	}
	return prefix + strings.Repeat("0", width-len(digits)) + string(digits), nil
}

// index returns the node's position among its parent's children.
func (n *RecordNode) index() int {
	if n == nil || n.Parent == nil {
		return 0
	}
	for i, child := range n.Parent.Children {
		if child == n {
			return i
		}
	}
	return 0
}

// maxFileSetInstances is how many instances fit in the File ID character set
// pydicom switches to above a million: six components of 35 characters.
const maxFileSetInstances = 35 * 35 * 35 * 35 * 35 * 35

// top returns the furthest ancestor of n, or n itself when it has none.
func (n *RecordNode) top() *RecordNode {
	for n != nil && n.Parent != nil && n.Parent.Record != nil {
		n = n.Parent
	}
	return n
}

// depth returns how many records are above n, with a top-level record at 0.
func (n *RecordNode) depth() int {
	depth := 0
	for parent := n.Parent; parent != nil && parent.Record != nil; parent = parent.Parent {
		depth++
	}
	return depth
}

// alphanumeric reports whether File IDs should use the alphanumeric set. Above
// a million instances the numeric set runs out of components, which is the
// threshold pydicom switches at.
func (fs *FileSet) alphanumeric() bool { return fs.alphanumericIDs }

// hasStagedAddition reports whether any instance was added and not yet saved.
func (fs *FileSet) hasStagedAddition() bool {
	for _, inst := range fs.instances {
		if inst.ds != nil {
			return true
		}
	}
	return false
}

// samePath reports whether a and b name the same directory, comparing the
// cleaned absolute paths so a trailing separator does not make a File-set look
// like a different one.
func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
