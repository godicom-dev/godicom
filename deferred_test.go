package godicom

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// A Deflated transfer syntax is inflated before the dataset is parsed, so every
// offset the parse records -- including a deferred element's ValueTell -- is
// relative to the inflated bytes. The in-memory reader left its deferred source
// pointing at the still-compressed buffer, so a deferred load read whatever byte
// happened to live at that offset before decompression: loudly, as a tag or VR
// mismatch, but unreadable either way.
func TestDeferredReadDeflated(t *testing.T) {
	t.Parallel()
	path := testFilePath("image_dfl.dcm")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	eager, err := ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Small enough that a short text element is deferred too, not just Pixel Data.
	const deferSize = 64

	readers := map[string]func() (*FileDataset, error){
		// ReadBytes inflates in place, so the deferred source has to follow.
		"ReadBytes": func() (*FileDataset, error) {
			return ReadBytes(data, &ReadOptions{DeferSize: deferSize})
		},
		// A non-seekable reader buffers and lands in that same code path.
		"Read": func() (*FileDataset, error) {
			return Read(io.NopCloser(bytes.NewReader(data)), &ReadOptions{DeferSize: deferSize})
		},
		// The streaming reader inflates into a fresh parse, and has to agree.
		"ReadFile": func() (*FileDataset, error) {
			return ReadFile(path, &ReadOptions{DeferSize: deferSize})
		},
	}

	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fd, err := read()
			if err != nil {
				t.Fatal(err)
			}

			var deferred []Tag
			for _, tag := range fd.SortedTags() {
				if fd.Dataset.elements[tag].Deferred {
					deferred = append(deferred, tag)
				}
			}
			if len(deferred) == 0 {
				t.Fatal("nothing was deferred; the test no longer covers the reload path")
			}

			for _, tag := range deferred {
				if err := fd.LoadDeferred(tag); err != nil {
					t.Fatalf("LoadDeferred(%s): %v", tag, err)
				}
				got, ok := fd.Get(tag)
				if !ok {
					t.Fatalf("%s is absent after a reload that reported success", tag)
				}
				want, ok := eager.Get(tag)
				if !ok {
					t.Fatalf("%s is missing from the eager read", tag)
				}
				if err := elementsEqual(got, want); err != nil {
					t.Errorf("%s: %v", tag, err)
				}
			}
		})
	}
}

func TestDeferredReadValuesIdentical(t *testing.T) {
	// pydicom.tests.test_filereader.TestDeferredRead.test_values_identical
	path := testFilePath("CT_small.dcm")

	normal, err := ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	deferred, err := ReadFile(path, &ReadOptions{DeferSize: 2000})
	if err != nil {
		t.Fatal(err)
	}

	pixelNormal, ok := normal.GetBytes(MustTag("PixelData"))
	if !ok {
		t.Fatal("PixelData missing in normal read")
	}
	pixelDeferred, ok := deferred.GetBytes(MustTag("PixelData"))
	if !ok {
		t.Fatal("PixelData missing in deferred read")
	}
	if !bytes.Equal(pixelNormal, pixelDeferred) {
		t.Fatalf("deferred PixelData length %d != normal %d", len(pixelDeferred), len(pixelNormal))
	}
	if len(pixelDeferred) != 32768 {
		t.Fatalf("PixelData length = %d, want 32768", len(pixelDeferred))
	}

	elem, ok := deferred.Get(MustTag("PixelData"))
	if ok && elem.Deferred {
		t.Fatal("PixelData should be loaded after GetBytes")
	}
}

func TestDeferredReadBuffer(t *testing.T) {
	// pydicom.tests.test_filereader.TestDeferredRead.test_buffer_deferred
	path := testFilePath("CT_small.dcm")
	ds, err := ReadFile(path, &ReadOptions{DeferSize: 1024})
	if err != nil {
		t.Fatal(err)
	}

	pixel, ok := ds.GetBytes(MustTag("PixelData"))
	if !ok {
		t.Fatal("PixelData missing")
	}
	if len(pixel) != 32768 {
		t.Fatalf("PixelData length = %d, want 32768", len(pixel))
	}

	block := ds.PrivateBlock(0x43, "GEMS_PARM_01")
	if block == nil {
		t.Fatal("private block missing")
	}
	priv, ok := block.Get(0x29)
	if !ok {
		t.Fatal("private element 0x29 missing")
	}
	val, ok := priv.Value.([]byte)
	if !ok {
		t.Fatalf("private element value type %T, want []byte", priv.Value)
	}
	if len(val) != 2068 {
		t.Fatalf("private element length = %d, want 2068", len(val))
	}
}

func TestDeferredReadFileMissing(t *testing.T) {
	// pydicom.tests.test_filereader.TestDeferredRead.test_file_exists
	src := testFilePath("CT_small.dcm")
	tmp := filepath.Join(t.TempDir(), "deferred.dcm")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatal(err)
	}

	ds, err := ReadFile(tmp, &ReadOptions{DeferSize: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(tmp); err != nil {
		t.Fatal(err)
	}

	if err := ds.LoadDeferred(MustTag("PixelData")); err == nil {
		t.Fatal("expected error loading deferred element from missing file")
	}
}

func TestDeferredReadNeverDefersCharset(t *testing.T) {
	// pydicom.tests.test_filereader.test_long_specific_char_set (defer_size < charset length)
	path := testFilePath("CT_small.dcm")
	ds, err := ReadFile(path, &ReadOptions{DeferSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, elem := range ds.Iter() {
		if elem.Tag != TagCharset {
			continue
		}
		if elem.Deferred {
			t.Fatal("SpecificCharacterSet must never be deferred")
		}
		return
	}
	t.Skip("CT_small.dcm has no SpecificCharacterSet element")
}
