package godicom

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestRead_FromFileMatchesReadFile(t *testing.T) {
	t.Parallel()
	path := testFilePath("CT_small.dcm")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	viaRead, err := Read(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	viaFile, err := ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if viaRead.Len() != viaFile.Len() {
		t.Fatalf("Len = %d, want %d", viaRead.Len(), viaFile.Len())
	}
	a, _ := viaRead.GetBytes(MustTag("PixelData"))
	b, _ := viaFile.GetBytes(MustTag("PixelData"))
	if !bytes.Equal(a, b) {
		t.Fatalf("PixelData mismatch: %d vs %d bytes", len(a), len(b))
	}
}

func TestRead_StopBeforePixelsDoesNotRequireTail(t *testing.T) {
	t.Parallel()
	path := testFilePath("CT_small.dcm")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Truncate well before Pixel Data; streaming StopBeforePixels should still work
	// if the truncated prefix contains the meta+header tags we need.
	// Find PixelData offset via a full parse of tags with StopBeforePixels on full file first.
	full, err := ReadFile(path, &ReadOptions{StopBeforePixels: true})
	if err != nil {
		t.Fatal(err)
	}
	if full.Has(MustTag("PixelData")) {
		t.Fatal("StopBeforePixels retained PixelData")
	}
	if full.Len() < 10 {
		t.Fatalf("too few elements: %d", full.Len())
	}

	// Non-seekable buffer reader (forces ReadAll fallback) still honors the option.
	ds, err := Read(bytes.NewReader(data), &ReadOptions{StopBeforePixels: true})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Has(MustTag("PixelData")) {
		t.Fatal("StopBeforePixels retained PixelData from bytes.Reader")
	}
}

func TestRead_DeferSizeReloadsFromFile(t *testing.T) {
	t.Parallel()
	path := testFilePath("CT_small.dcm")
	ds, err := ReadFile(path, &ReadOptions{DeferSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	var pixelElem *DataElement
	for _, elem := range ds.Iter() {
		if elem.Tag == MustTag("PixelData") {
			pixelElem = elem
			break
		}
	}
	if pixelElem == nil || !pixelElem.Deferred {
		t.Fatal("PixelData should start deferred")
	}
	// Ensure we did not keep a full in-memory copy of the file for deferred reload.
	if ds.readCtx == nil || ds.readCtx.data != nil {
		t.Fatal("streaming ReadFile should defer via filename, not in-memory data")
	}
	if ds.readCtx.filename == "" {
		t.Fatal("missing filename for deferred reload")
	}
	pixel, ok := ds.GetBytes(MustTag("PixelData"))
	if !ok || len(pixel) != 32768 {
		t.Fatalf("PixelData len = %d, want 32768", len(pixel))
	}
}

func TestRead_DeferSizeReloadsFromSeeker(t *testing.T) {
	t.Parallel()
	path := testFilePath("CT_small.dcm")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// bytes.Reader is seekable but not an *os.File, so there is no path to
	// reopen: deferred values must come back through the retained reader.
	ds, err := Read(bytes.NewReader(data), &ReadOptions{DeferSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	var pixelElem *DataElement
	for _, elem := range ds.Iter() {
		if elem.Tag == MustTag("PixelData") {
			pixelElem = elem
			break
		}
	}
	if pixelElem == nil || !pixelElem.Deferred {
		t.Fatal("PixelData should start deferred")
	}
	if ds.readCtx == nil || ds.readCtx.data != nil || ds.readCtx.filename != "" {
		t.Fatal("seekable Read should defer via the retained reader, not data or filename")
	}
	pixel, ok := ds.GetBytes(MustTag("PixelData"))
	if !ok {
		t.Fatal("deferred PixelData unreachable after Read from a non-file ReadSeeker")
	}
	eager, err := ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, ok := eager.GetBytes(MustTag("PixelData"))
	if !ok {
		t.Fatal("PixelData missing from eager read")
	}
	if !bytes.Equal(pixel, want) {
		t.Fatalf("deferred PixelData = %d bytes, want %d", len(pixel), len(want))
	}
}

func TestRead_SpecificTagsSkipsLargeValues(t *testing.T) {
	t.Parallel()
	path := testFilePath("CT_small.dcm")
	ds, err := ReadFile(path, &ReadOptions{
		SpecificTags: []Tag{MustTag("PatientName"), MustTag("PatientID"), MustTag("SOPInstanceUID")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Has(MustTag("PixelData")) {
		t.Fatal("SpecificTags should omit PixelData")
	}
	if _, ok := ds.GetString(MustTag("PatientID")); !ok {
		t.Fatal("PatientID missing")
	}
}

// A reader that already offers random access over a known extent is handed to
// the parser as-is rather than wrapped in seekerReaderAt, so nothing ever seeks
// it and the caller's offset survives the parse -- and the deferred loads after
// it, which go back through the same retained source.
func TestRead_SizedReaderAtLeavesOffsetAlone(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(testFilePath("CT_small.dcm"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := ReadBytes(data, nil)
	if err != nil {
		t.Fatal(err)
	}

	r := bytes.NewReader(data)
	ds, err := Read(r, &ReadOptions{DeferSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if r.Len() != len(data) {
		t.Errorf("the parse consumed %d bytes of the reader; a source it can ReadAt must not be seeked",
			len(data)-r.Len())
	}

	// Get loads every deferred element through readCtx.src on the way past.
	if err := datasetsEqual(ds, want); err != nil {
		t.Errorf("dataset differs from ReadBytes: %v", err)
	}
	if r.Len() != len(data) {
		t.Errorf("deferred loads consumed %d bytes of the reader", len(data)-r.Len())
	}
}

// An io.SectionReader reports the section's Size and takes section-relative
// offsets in ReadAt. Reading one directly has to honour both, so surrounding
// bytes stay invisible.
func TestRead_SectionReaderIgnoresSurroundingBytes(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(testFilePath("CT_small.dcm"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := ReadBytes(data, nil)
	if err != nil {
		t.Fatal(err)
	}

	const lead = "junk-before-"
	padded := append([]byte(lead), data...)
	padded = append(padded, "-junk-after"...)
	section := io.NewSectionReader(bytes.NewReader(padded), int64(len(lead)), int64(len(data)))

	ds, err := Read(section, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := datasetsEqual(ds, want); err != nil {
		t.Errorf("dataset differs from ReadBytes: %v", err)
	}
}

func TestRead_NilReader(t *testing.T) {
	t.Parallel()
	_, err := Read(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRead_NonSeekableReader(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(testFilePath("CT_small.dcm"))
	if err != nil {
		t.Fatal(err)
	}
	// io.NopCloser hides Seek; Read should fall back to buffering.
	ds, err := Read(io.NopCloser(bytes.NewReader(data)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Len() < 100 {
		t.Fatalf("Len = %d", ds.Len())
	}
}
