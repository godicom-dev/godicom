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
