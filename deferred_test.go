package godicom

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

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
