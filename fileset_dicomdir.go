package godicom

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// This file writes a DICOMDIR. It is the second half of the File-set writer:
// the records themselves are built in fileset_write.go, and here they are laid
// out with the offsets that link them together, which is the part of
// pydicom's fileset.py that FileSet._write_dicomdir() does.
//
// A DICOMDIR is a linked list stored flat. Every record sits in the *Directory
// Record Sequence* and carries two offsets: the one to the next record at the
// same level, and the one to the first record one level down. Both are
// positions in the file, counted from its first byte, which means they can only
// be filled in once every record's length is known -- hence the two passes:
// encode all the records to learn the lengths, then write the offsets into the
// bytes that were produced.

// WriteDICOMDIROptions controls how a DICOMDIR is written.
type WriteDICOMDIROptions struct {
	// ForceImplicitVR writes the DICOMDIR as Implicit VR Little Endian.
	// PS3.10 requires Explicit VR Little Endian for a DICOMDIR, so this
	// produces a file that strict readers reject; it exists because some
	// older implementations only accept implicit VR.
	ForceImplicitVR bool
}

// countWriter keeps the position of the bytes it has passed on.
//
// The offsets in a DICOMDIR are positions in the file, and the writer needs
// them while it is producing the bytes rather than afterwards, so every write
// goes through this to be counted.
type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// encodedRecord is one directory record once encoded, together with where its
// two offset values sit inside those bytes.
type encodedRecord struct {
	node *RecordNode
	buf  []byte
	// nextAt and lowerAt are the positions of the values of *Offset of the
	// Next Directory Record* and *Offset of Referenced Lower Level Directory
	// Entity*, counted from the first byte of buf.
	nextAt  int
	lowerAt int
}

// EncodeDICOMDIR returns the File-set's DICOMDIR as bytes.
//
// The records are written in the order the tree links them, and their offset
// elements are updated in place to match the layout, exactly as pydicom's
// FileSet._write_dicomdir() does. Records that the offset links never reach --
// orphans the reader dropped -- are left out.
//
// The File-set must have a *Media Storage SOP Instance UID*; one read from a
// DICOMDIR has it, and a File-set built from scratch is given one when it is
// created.
func (fs *FileSet) EncodeDICOMDIR(opts *WriteDICOMDIROptions) ([]byte, error) {
	if fs == nil || fs.Dataset == nil || fs.Dataset.Dataset == nil {
		return nil, fmt.Errorf("godicom: unable to write a DICOMDIR for a nil File-set")
	}

	forceImplicit := opts != nil && opts.ForceImplicitVR
	cc := codecContext{EncodingInfo: EncodingInfo{IsLittleEndian: true, IsImplicitVR: forceImplicit}}

	fileMeta, err := fs.dicomdirFileMeta(forceImplicit)
	if err != nil {
		return nil, err
	}

	ds := fs.Dataset.Dataset
	// The File-set ID is a field on FileSet rather than part of the dataset,
	// so it is copied in here: assigning to it and saving has to be enough.
	if fs.ID != "" {
		if len(fs.ID) > 16 {
			return nil, fmt.Errorf("godicom: the File-set ID must be at most 16 characters, got %d", len(fs.ID))
		}
		if current, ok := ds.GetString(tag.FileSetID); !ok || current != fs.ID {
			ds.Set(NewDataElement(tag.FileSetID, VRCS, fs.ID))
		}
	}
	if err := CorrectAmbiguousVR(ds, true, nil); err != nil {
		return nil, err
	}
	firstElem, err := ensureOffsetElement(ds, tag.OffsetOfTheFirstDirectoryRecordOfTheRootDirectoryEntity)
	if err != nil {
		return nil, err
	}
	lastElem, err := ensureOffsetElement(ds, tag.OffsetOfTheLastDirectoryRecordOfTheRootDirectoryEntity)
	if err != nil {
		return nil, err
	}
	firstElem.Value = uint32(0)
	firstElem.RawValue = nil
	lastElem.Value = uint32(0)
	lastElem.RawValue = nil

	var out bytes.Buffer
	// Every byte goes through the counter, including the preamble: the offsets
	// are positions in the file, and the preamble is part of it.
	cw := &countWriter{w: &out}
	_, _ = cw.Write(make([]byte, 128))
	_, _ = cw.Write([]byte("DICM"))

	metaFP := newDicomWriter(cw)
	metaFP.SetByteOrder(true)
	if err := writeFileMetaInfo(metaFP, fileMeta, true); err != nil {
		return nil, fmt.Errorf("godicom: unable to write the DICOMDIR's file meta information: %w", err)
	}

	fp := newDicomWriter(cw)
	fp.SetByteOrder(true)
	st := newWriteState()

	// Everything up to the *Directory Record Sequence* goes out first, in tag
	// order, which is the order the offsets below are counted in.
	var firstAt, lastAt = int64(-1), int64(-1)
	for _, elem := range ds.Iter() {
		if elem.Tag == tag.DirectoryRecordSequence {
			continue
		}
		if elem.Tag.Group() == 0x0002 {
			continue
		}
		// Retired group lengths are not written (PS3.5 Section 7.2).
		if elem.Tag.Element() == 0 && elem.Tag.Group() > 6 {
			continue
		}
		switch elem.Tag {
		case tag.OffsetOfTheFirstDirectoryRecordOfTheRootDirectoryEntity:
			firstAt = cw.n + 8
		case tag.OffsetOfTheLastDirectoryRecordOfTheRootDirectoryEntity:
			lastAt = cw.n + 8
		}
		if err := writeElementState(fp, elem, cc, false, st); err != nil {
			return nil, fmt.Errorf("godicom: unable to write the DICOMDIR's %s element: %w", elem.Tag, err)
		}
		if elem.Tag == TagCharset {
			cc = cc.withCharsets(ParseCharacterSets(elem.Value))
		}
	}
	if firstAt < 0 || lastAt < 0 {
		return nil, fmt.Errorf("godicom: unable to write the DICOMDIR as its first and last record offset elements are missing")
	}

	// Pass one: encode every record and work out where it will land. The
	// sequence element's own header comes before the first item, so the first
	// item starts at the current position plus that header's length.
	var seqHeaderLen int64 = 12
	if forceImplicit {
		seqHeaderLen = 8
	}
	nodes := preorderNodes(fs.Tree)
	records := make([]encodedRecord, 0, len(nodes))
	offset := cw.n + seqHeaderLen
	var contentLen int64
	for _, node := range nodes {
		buf, nextAt, lowerAt, err := encodeDirectoryRecord(node.Record, cc)
		if err != nil {
			return nil, err
		}
		// Both offsets are 32-bit in the standard. Refusing here is the only
		// way a File-set too large for one DICOMDIR is reported: the offset
		// would otherwise wrap and point at an unrelated record.
		if offset > int64(^uint32(0)) {
			return nil, fmt.Errorf("godicom: the DICOMDIR is too large: the record at %d bytes does not fit a 32-bit offset", offset)
		}
		node.Offset = offset
		records = append(records, encodedRecord{node: node, buf: buf, nextAt: nextAt, lowerAt: lowerAt})
		contentLen += 8 + int64(len(buf))
		offset += 8 + int64(len(buf))
	}

	// Pass two: fill in the links and write the sequence.
	writeSequenceHeader(cw, contentLen, forceImplicit)
	for i := range records {
		rec := &records[i]
		next := int64(0)
		if sibling := rec.node.nextSibling(); sibling != nil {
			next = sibling.Offset
		}
		lower := int64(0)
		if len(rec.node.Children) > 0 {
			lower = rec.node.Children[0].Offset
		}
		if rec.nextAt+4 > len(rec.buf) || rec.lowerAt+4 > len(rec.buf) {
			return nil, fmt.Errorf("godicom: unable to lay out the DICOMDIR: the offset values of the %q record are not four bytes long", rec.node.Type())
		}
		binary.LittleEndian.PutUint32(rec.buf[rec.nextAt:], uint32(next))
		binary.LittleEndian.PutUint32(rec.buf[rec.lowerAt:], uint32(lower))
		// Keep the record the caller can see in step with the bytes: reading
		// back what was just written would otherwise hand out the old links.
		setOffsetValue(rec.node.Record, tag.OffsetOfTheNextDirectoryRecord, next)
		setOffsetValue(rec.node.Record, tag.OffsetOfReferencedLowerLevelDirectoryEntity, lower)

		if err := writeItem(cw, rec.buf); err != nil {
			return nil, err
		}
	}

	// Everything after the sequence.
	for _, elem := range ds.Iter() {
		if elem.Tag <= tag.DirectoryRecordSequence {
			continue
		}
		if elem.Tag.Group() == 0x0002 {
			continue
		}
		if elem.Tag.Element() == 0 && elem.Tag.Group() > 6 {
			continue
		}
		if err := writeElementState(fp, elem, cc, false, st); err != nil {
			return nil, fmt.Errorf("godicom: unable to write the DICOMDIR's %s element: %w", elem.Tag, err)
		}
	}

	// The two pointers into the root level are the only values that could not
	// be known before the records were placed.
	var first, last int64
	if children := fs.Tree.Children; len(children) > 0 {
		first = children[0].Offset
		last = children[len(children)-1].Offset
	}
	data := out.Bytes()
	if int64(len(data)) < firstAt+4 || int64(len(data)) < lastAt+4 {
		return nil, fmt.Errorf("godicom: unable to lay out the DICOMDIR: the root record offsets landed past the end of the file")
	}
	binary.LittleEndian.PutUint32(data[firstAt:], uint32(first))
	binary.LittleEndian.PutUint32(data[lastAt:], uint32(last))

	// Leave the in-memory File-set describing the file that was just produced.
	firstElem.Value = uint32(first)
	lastElem.Value = uint32(last)
	ds.Set(NewDataElement(tag.DirectoryRecordSequence, VRSQ, recordSequence(nodes)))

	return data, nil
}

// WriteDICOMDIR writes the File-set's DICOMDIR to path.
//
// Only the DICOMDIR is written; the instances it points at are left where they
// are. Use Write to replace the DICOMDIR of the File-set this one was read
// from.
func (fs *FileSet) WriteDICOMDIR(path string, opts *WriteDICOMDIROptions) error {
	data, err := fs.EncodeDICOMDIR(opts)
	if err != nil {
		return err
	}
	return writeFileBytes(path, data)
}

// Write replaces the DICOMDIR of the File-set with one built from its current
// tree.
//
// The file written is <root>/DICOMDIR, the name PS3.10 Section 8.3 fixes for
// it.
func (fs *FileSet) Write(opts *WriteDICOMDIROptions) error {
	if fs.RootPath == "" {
		return fmt.Errorf("godicom: unable to write the DICOMDIR as the File-set has no root path")
	}
	return fs.WriteDICOMDIR(filepath.Join(fs.RootPath, "DICOMDIR"), opts)
}

// dicomdirFileMeta returns the File Meta Information a DICOMDIR is written
// with: the File-set's own, with the syntax and identity PS3.10 requires.
func (fs *FileSet) dicomdirFileMeta(forceImplicit bool) (*FileMetaDataset, error) {
	meta := NewFileMetaDataset()
	if fs.Dataset != nil && fs.Dataset.FileMeta != nil {
		meta = cloneFileMeta(fs.Dataset.FileMeta)
	}
	ts := uid.ExplicitVRLittleEndian
	if forceImplicit {
		ts = uid.ImplicitVRLittleEndian
	}
	meta.Set(NewDataElement(tag.TransferSyntaxUID, VRUI, ts))
	meta.Set(NewDataElement(tag.MediaStorageSOPClassUID, VRUI, uid.MediaStorageDirectoryStorage))
	if fs.UID == "" {
		return nil, fmt.Errorf("godicom: unable to write the DICOMDIR as the File-set has no Media Storage SOP Instance UID")
	}
	meta.Set(NewDataElement(tag.MediaStorageSOPInstanceUID, VRUI, string(fs.UID)))
	// Adds the File Meta Information Version and the implementation elements
	// PS3.10 Section 7.1 makes mandatory, which a File-set read from a
	// conformant DICOMDIR already has and a new one does not.
	if err := ValidateFileMeta(meta, true); err != nil {
		return nil, err
	}
	return meta, nil
}

// ensureOffsetElement returns the offset element tg, adding it when the
// dataset does not have one.
func ensureOffsetElement(ds *Dataset, tg Tag) (*DataElement, error) {
	if elem, ok := ds.Get(tg); ok && elem != nil {
		return elem, nil
	}
	elem := NewDataElement(tg, VRUL, uint32(0))
	ds.Set(elem)
	return elem, nil
}

// encodeDirectoryRecord encodes one directory record and reports where its two
// offset values sit inside the bytes.
func encodeDirectoryRecord(rec *Dataset, cc codecContext) ([]byte, int, int, error) {
	if rec == nil {
		return nil, 0, 0, fmt.Errorf("godicom: unable to write a DICOMDIR record that is nil")
	}
	for _, tg := range []Tag{tag.OffsetOfTheNextDirectoryRecord, tag.OffsetOfReferencedLowerLevelDirectoryEntity} {
		if _, ok := rec.Get(tg); !ok {
			rec.Set(NewDataElement(tg, VRUL, uint32(0)))
		}
	}

	var buf bytes.Buffer
	cw := &countWriter{w: &buf}
	fp := newDicomWriter(cw)
	fp.SetByteOrder(true)
	st := newWriteState()

	var nextAt, lowerAt = -1, -1
	for _, elem := range rec.Iter() {
		if elem.Tag.Group() == 0x0002 {
			continue
		}
		if elem.Tag.Element() == 0 && elem.Tag.Group() > 6 {
			continue
		}
		start := cw.n
		switch elem.Tag {
		case tag.OffsetOfTheNextDirectoryRecord:
			nextAt = int(start) + 8
		case tag.OffsetOfReferencedLowerLevelDirectoryEntity:
			lowerAt = int(start) + 8
		}
		if err := writeElementState(fp, elem, cc, false, st); err != nil {
			return nil, 0, 0, fmt.Errorf("godicom: unable to write the %s element of a DICOMDIR record: %w", elem.Tag, err)
		}
		if elem.Tag == TagCharset {
			cc = cc.withCharsets(ParseCharacterSets(elem.Value))
		}
		// An offset is a 32-bit value; a record that spells one any other way
		// cannot be patched in place, so refuse before the wrong four bytes are
		// overwritten.
		if (elem.Tag == tag.OffsetOfTheNextDirectoryRecord || elem.Tag == tag.OffsetOfReferencedLowerLevelDirectoryEntity) && cw.n-start != 12 {
			return nil, 0, 0, fmt.Errorf("godicom: the DICOMDIR record's %s element must be a single 32-bit value", elem.Tag)
		}
	}
	if nextAt < 0 || lowerAt < 0 {
		return nil, 0, 0, fmt.Errorf("godicom: the DICOMDIR record's offset elements were not written")
	}
	return buf.Bytes(), nextAt, lowerAt, nil
}

// writeSequenceHeader writes the *Directory Record Sequence* element header,
// leaving the items to the caller.
func writeSequenceHeader(w io.Writer, contentLen int64, forceImplicit bool) {
	var hdr [12]byte
	binary.LittleEndian.PutUint16(hdr[0:], uint16(tag.DirectoryRecordSequence.Group()))
	binary.LittleEndian.PutUint16(hdr[2:], uint16(tag.DirectoryRecordSequence.Element()))
	if forceImplicit {
		binary.LittleEndian.PutUint32(hdr[4:], uint32(contentLen))
		_, _ = w.Write(hdr[:8])
		return
	}
	copy(hdr[4:], "SQ")
	binary.LittleEndian.PutUint16(hdr[6:], 0)
	binary.LittleEndian.PutUint32(hdr[8:], uint32(contentLen))
	_, _ = w.Write(hdr[:])
}

// writeItem writes one sequence item around the encoded record.
func writeItem(w io.Writer, content []byte) error {
	if int64(len(content)) > int64(^uint32(0)) {
		return fmt.Errorf("godicom: a DICOMDIR record of %d bytes does not fit a sequence item", len(content))
	}
	var hdr [8]byte
	binary.LittleEndian.PutUint16(hdr[0:], uint16(ItemTag.Group()))
	binary.LittleEndian.PutUint16(hdr[2:], uint16(ItemTag.Element()))
	binary.LittleEndian.PutUint32(hdr[4:], uint32(len(content)))
	_, _ = w.Write(hdr[:])
	_, _ = w.Write(content)
	return nil
}

// preorderNodes returns every record node below root, parents before children
// and siblings in order, which is the order a DICOMDIR stores them in.
func preorderNodes(root *RecordNode) []*RecordNode {
	if root == nil {
		return nil
	}
	var out []*RecordNode
	var walk func(n *RecordNode)
	walk = func(n *RecordNode) {
		for _, child := range n.Children {
			if child.Record == nil {
				continue
			}
			out = append(out, child)
			walk(child)
		}
	}
	walk(root)
	return out
}

// nextSibling returns the node after n among its parent's children, or nil.
func (n *RecordNode) nextSibling() *RecordNode {
	if n == nil || n.Parent == nil {
		return nil
	}
	for i, child := range n.Parent.Children {
		if child == n && i+1 < len(n.Parent.Children) {
			return n.Parent.Children[i+1]
		}
	}
	return nil
}

// setOffsetValue sets a record's offset element to offset, keeping the element
// a single 32-bit value.
func setOffsetValue(rec *Dataset, tg Tag, offset int64) {
	if rec == nil {
		return
	}
	rec.Set(NewDataElement(tg, VRUL, uint32(offset)))
}

// recordSequence returns a sequence holding nodes' records in order.
func recordSequence(nodes []*RecordNode) *Sequence {
	items := make([]*Dataset, 0, len(nodes))
	for _, node := range nodes {
		items = append(items, node.Record)
	}
	return NewSequence(items)
}

// writeFileBytes writes data to filename through a temporary file beside it,
// so a failure leaves whatever was there before untouched. It is the same
// sequence writeFile uses, minus the DICOM encoding.
func writeFileBytes(filename string, data []byte) error {
	perm, replaceable := replaceableMode(filename)
	if !replaceable {
		// A symlink, device or pipe: write through it rather than replacing
		// the name with a regular file.
		return os.WriteFile(filename, data, 0o666)
	}

	f, err := createSibling(filename, perm)
	if err != nil {
		return err
	}
	name := f.Name()
	committed := false
	defer func() {
		if !committed {
			_ = f.Close()
			_ = os.Remove(name)
		}
	}()

	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filename); err != nil {
		return err
	}
	committed = true
	return nil
}
