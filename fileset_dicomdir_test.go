package godicom_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/tag"
)

// writeTempDICOMDIR encodes fs and writes the result to a temporary file,
// returning its path.
func writeTempDICOMDIR(t *testing.T, fs *godicom.FileSet, opts *godicom.WriteDICOMDIROptions) string {
	t.Helper()
	data, err := fs.EncodeDICOMDIR(opts)
	if err != nil {
		t.Fatalf("EncodeDICOMDIR: %v", err)
	}
	path := filepath.Join(t.TempDir(), "DICOMDIR")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write DICOMDIR: %v", err)
	}
	return path
}

// TestEncodeDICOMDIROffsetsLinkTheRecords checks the written DICOMDIR against
// itself: every offset in it has to be the position of the item tag it names,
// and the root pointers have to be the first and last item. The offsets are
// recovered by reading the file back, which is a different code path from the
// one that wrote them.
func TestEncodeDICOMDIROffsetsLinkTheRecords(t *testing.T) {
	fs := mustFileSet(t, "DICOMDIR")
	path := writeTempDICOMDIR(t, fs, nil)

	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	seq, ok := ds.GetSequence(tag.DirectoryRecordSequence)
	if !ok || seq.Len() == 0 {
		t.Fatal("the written DICOMDIR has no directory records")
	}

	items := seq.Items()
	positions := make(map[uint64]bool, len(items))
	for _, item := range items {
		positions[uint64(item.ItemOffset)] = true
	}

	rootFirst, _ := ds.GetInt(tag.OffsetOfTheFirstDirectoryRecordOfTheRootDirectoryEntity)
	rootLast, _ := ds.GetInt(tag.OffsetOfTheLastDirectoryRecordOfTheRootDirectoryEntity)
	if int64(rootFirst) != items[0].ItemOffset {
		t.Fatalf("first root record offset = %d, want %d", rootFirst, items[0].ItemOffset)
	}
	// *Offset of the Last Directory Record of the Root Directory Entity* is the
	// last record at the top level, not the last item: the items are stored
	// depth first, so the file ends inside the last branch.
	if !positions[uint64(rootLast)] {
		t.Fatalf("last root record offset = %d, which is not an item tag", rootLast)
	}
	for _, item := range items {
		if item.ItemOffset == int64(rootLast) {
			if next, _ := item.GetInt(tag.OffsetOfTheNextDirectoryRecord); next != 0 {
				t.Fatalf("the last root record points at %d", next)
			}
		}
	}

	for i, item := range items {
		next, _ := item.GetInt(tag.OffsetOfTheNextDirectoryRecord)
		lower, _ := item.GetInt(tag.OffsetOfReferencedLowerLevelDirectoryEntity)
		if next != 0 && !positions[uint64(next)] {
			t.Fatalf("record %d (%s) points at %d, which is not an item tag", i, itemType(item), next)
		}
		if lower != 0 && !positions[uint64(lower)] {
			t.Fatalf("record %d (%s) points down at %d, which is not an item tag", i, itemType(item), lower)
		}
	}
	// Following the links from the root has to reach every item exactly once:
	// a record the links never reach is an orphan, which a reader is entitled
	// to drop.
	byOffset := map[int64]*godicom.Dataset{}
	for _, item := range items {
		byOffset[item.ItemOffset] = item
	}
	seen := map[int64]bool{}
	var walkSiblings func(offset int64)
	walkSiblings = func(offset int64) {
		for offset != 0 {
			if seen[offset] {
				t.Fatalf("the records link back to %d", offset)
			}
			seen[offset] = true
			item, ok := byOffset[offset]
			if !ok {
				t.Fatalf("record offset %d is not an item", offset)
			}
			lower, _ := item.GetInt(tag.OffsetOfReferencedLowerLevelDirectoryEntity)
			walkSiblings(int64(lower))
			next, _ := item.GetInt(tag.OffsetOfTheNextDirectoryRecord)
			offset = int64(next)
		}
	}
	walkSiblings(int64(rootFirst))
	if len(seen) != len(items) {
		t.Fatalf("the root links reach %d records, but the sequence holds %d", len(seen), len(items))
	}
}

// TestWriteDICOMDIRRoundTrip replaces the DICOMDIR of a copy of each test
// File-set and reads the copy back, so the tree and the instances have to come
// out the same as they went in. The fixtures are the ones pydicom's own
// File-set tests write back: one reordered, one without record offsets, one
// big endian and one implicit VR, all of which have to come back out as
// conformant explicit VR little endian.
func TestWriteDICOMDIRRoundTrip(t *testing.T) {
	fixture := filepath.Join("pydicom", "src", "pydicom", "data", "test_files", "dicomdirtests")
	for _, name := range []string{"DICOMDIR", "DICOMDIR-reordered", "DICOMDIR-nooffset", "DICOMDIR-bigEnd", "DICOMDIR-implicit"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "fileset")
			copyDir(t, fixture, root)

			fs, err := godicom.OpenFileSet(filepath.Join(root, name), nil)
			if err != nil {
				t.Fatalf("OpenFileSet: %v", err)
			}
			before := sopList(t, fs)
			beforeTree := treeShape(fs.Tree)
			if len(before) != 31 {
				t.Fatalf("the fixture yielded %d instances, want 31", len(before))
			}

			if err := fs.WriteDICOMDIR(filepath.Join(root, "DICOMDIR"), nil); err != nil {
				t.Fatalf("WriteDICOMDIR: %v", err)
			}

			after, err := godicom.OpenFileSet(filepath.Join(root, "DICOMDIR"), nil)
			if err != nil {
				t.Fatalf("OpenFileSet after write: %v", err)
			}
			if got := sopList(t, after); strings.Join(got, ",") != strings.Join(before, ",") {
				t.Fatalf("instances changed:\n got %v\nwant %v", got, before)
			}
			if got := treeShape(after.Tree); got != beforeTree {
				t.Fatalf("tree changed:\n got %s\nwant %s", got, beforeTree)
			}
			if after.Len() != 31 {
				t.Fatalf("instances = %d, want 31", after.Len())
			}
		})
	}
}

// TestEncodeDICOMDIRMatchesPyDicom pins the bytes.
//
// Both digests are of the File-set in pydicom's own DICOMDIR test fixture, and
// were confirmed against pydicom 3.0.0: loading that fixture and calling
// FileSet._write_dicomdir() produces a file identical to ours, byte for byte,
// in explicit VR and with force_implicit=True alike. The same holds for the
// reordered, no-offset, big endian and implicit VR fixtures.
//
// A digest rather than the 11 kB of bytes keeps the test readable; a change
// here means the layout moved, and the comparison against pydicom has to be
// made again before the new value is accepted.
func TestEncodeDICOMDIRMatchesPyDicom(t *testing.T) {
	fs := mustFileSet(t, "DICOMDIR")

	cases := []struct {
		name string
		opts *godicom.WriteDICOMDIROptions
		want string
		size int
	}{
		{
			name: "explicit",
			want: "b9bf631bb20f9276118bafab094291bae3721bccd25bf4bef18474aae5d60498",
			size: 11116,
		},
		{
			name: "implicit",
			opts: &godicom.WriteDICOMDIROptions{ForceImplicitVR: true},
			want: "0023bc2242ccd127fdddc399f907109643660b6c4c2d71fd5f5613ce65ad2a94",
			size: 11110,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := fs.EncodeDICOMDIR(tc.opts)
			if err != nil {
				t.Fatalf("EncodeDICOMDIR: %v", err)
			}
			if len(data) != tc.size {
				t.Fatalf("encoded %d bytes, want %d", len(data), tc.size)
			}
			got := fmt.Sprintf("%x", sha256.Sum256(data))
			if got != tc.want {
				t.Fatalf("DICOMDIR digest = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestWriteDICOMDIRImplicit covers the non-conformant syntax the option exists
// for: the records still link, and the file declares implicit VR.
func TestWriteDICOMDIRImplicit(t *testing.T) {
	fs := mustFileSet(t, "DICOMDIR")
	data, err := fs.EncodeDICOMDIR(&godicom.WriteDICOMDIROptions{ForceImplicitVR: true})
	if err != nil {
		t.Fatalf("EncodeDICOMDIR: %v", err)
	}

	path := filepath.Join(t.TempDir(), "DICOMDIR")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	ts, ok := ds.FileMeta.GetString(tag.TransferSyntaxUID)
	if !ok || ts != "1.2.840.10008.1.2" {
		t.Fatalf("Transfer Syntax = %q, want implicit VR little endian", ts)
	}
	seq, _ := ds.GetSequence(tag.DirectoryRecordSequence)
	if seq == nil || seq.Len() == 0 {
		t.Fatal("no directory records")
	}
	first, _ := ds.GetInt(tag.OffsetOfTheFirstDirectoryRecordOfTheRootDirectoryEntity)
	if int64(first) != seq.Get(0).ItemOffset {
		t.Fatalf("first root record offset = %d, want %d", first, seq.Get(0).ItemOffset)
	}
}

// TestEncodeDICOMDIRIsStable writes the same File-set twice and expects the
// same bytes: nothing in the layout may depend on iteration order.
func TestEncodeDICOMDIRIsStable(t *testing.T) {
	fs := mustFileSet(t, "DICOMDIR")
	first, err := fs.EncodeDICOMDIR(nil)
	if err != nil {
		t.Fatalf("EncodeDICOMDIR: %v", err)
	}
	second, err := fs.EncodeDICOMDIR(nil)
	if err != nil {
		t.Fatalf("EncodeDICOMDIR: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("encoding the same File-set twice produced different bytes")
	}
}

// TestWriteDICOMDIRRefusesANilFileSet covers the nil receiver.
func TestEncodeDICOMDIRRefusesANilFileSet(t *testing.T) {
	var fs *godicom.FileSet
	_, err := fs.EncodeDICOMDIR(nil)
	if err == nil || !strings.Contains(err.Error(), "nil File-set") {
		t.Fatalf("err = %v", err)
	}
}

func itemType(ds *godicom.Dataset) string {
	s, _ := ds.GetString(tag.DirectoryRecordType)
	return s
}

// treeShape renders the record tree as a string so two File-sets can be
// compared without writing a recursive comparison.
func treeShape(n *godicom.RecordNode) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(node *godicom.RecordNode, depth int)
	walk = func(node *godicom.RecordNode, depth int) {
		for _, child := range node.Children {
			fileID, _ := child.Record.GetString(tag.ReferencedFileID)
			b.WriteString(strings.Repeat(" ", depth))
			b.WriteString(child.Type())
			b.WriteString(" ")
			b.WriteString(strings.ReplaceAll(fileID, "\\", "/"))
			b.WriteString("\n")
			walk(child, depth+1)
		}
	}
	walk(n, 0)
	return b.String()
}

// copyDir copies a directory tree, which the round-trip test needs so it can
// rewrite a DICOMDIR without touching the fixture.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}
}
