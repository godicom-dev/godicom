package encaps_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/encaps"
	"github.com/godicom-dev/godicom/tag"
)

func dicomTestFile(name string) string {
	candidates := []string{
		filepath.Join("..", "testdata", "dcm", name),
		filepath.Join("..", "pydicom", "src", "pydicom", "data", "test_files", name),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return candidates[1]
}

func readPixelData(t *testing.T, filename string) (*godicom.FileDataset, []byte, int) {
	t.Helper()
	ds, err := godicom.ReadFile(dicomTestFile(filename), nil)
	if err != nil {
		t.Fatal(err)
	}
	pixelData, ok := ds.GetBytes(tag.PixelData)
	if !ok {
		t.Fatal("PixelData missing")
	}
	nf, ok := ds.GetInt(tag.NumberOfFrames)
	if !ok || nf <= 0 {
		nf = 1
	}
	return ds, pixelData, nf
}

func TestGenerateFrames_file_JP2K_10frame_noBOT(t *testing.T) {
	// pydicom.tests.test_encaps.TestGenerateFrames.test_empty_bot_single_fragment_per_frame
	_, pixelData, nf := readPixelData(t, "emri_small_jpeg_2k_lossless.dcm")
	if nf != 10 {
		t.Fatalf("NumberOfFrames = %d, want 10", nf)
	}
	frames, err := encaps.GenerateFrames(pixelData, encaps.FramesOptions{
		NumberOfFrames: nf,
		LittleEndian:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 10 {
		t.Fatalf("frames = %d, want 10", len(frames))
	}
	for i, frame := range frames {
		if len(frame) == 0 {
			t.Fatalf("frame %d is empty", i)
		}
	}
}

func TestGetFrame_file_JP2K_frame8(t *testing.T) {
	// pydicom.tests.test_encaps.TestGenerateFrames.test_mmap — frame 9 (index 8)
	_, pixelData, nf := readPixelData(t, "emri_small_jpeg_2k_lossless.dcm")
	opts := encaps.FramesOptions{NumberOfFrames: nf, LittleEndian: true}
	frame, err := encaps.GetFrame(pixelData, 8, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) != 3754 {
		t.Fatalf("frame 8 len = %d, want 3754", len(frame))
	}
	wantTail := []byte{0x56, 0xf7, 0xff, 0x4e, 0x60, 0xe3, 0xda, 0x0f, 0xff, 0xd9}
	if !bytes.Equal(frame[len(frame)-10:], wantTail) {
		t.Fatalf("frame 8 tail = % X, want % X", frame[len(frame)-10:], wantTail)
	}
}

func TestGenerateFrames_file_JPEG2000_singleFrame(t *testing.T) {
	// pydicom test_encaps uses JPEG2000.dcm for single-frame encapsulated pixel data
	_, pixelData, nf := readPixelData(t, "JPEG2000.dcm")
	frames, err := encaps.GenerateFrames(pixelData, encaps.FramesOptions{
		NumberOfFrames: nf,
		LittleEndian:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	if len(frames[0]) == 0 {
		t.Fatal("single frame is empty")
	}
}

func TestCountFragments_file_JPEG2000(t *testing.T) {
	_, pixelData, _ := readPixelData(t, "JPEG2000.dcm")
	_, rest, err := encaps.ParseBasicOffsets(pixelData, true)
	if err != nil {
		t.Fatal(err)
	}
	n, err := encaps.CountFragments(rest, true)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("fragments = %d, want at least 1", n)
	}
}

func TestGenerateFrames_file_JPEGLS_10frame(t *testing.T) {
	_, pixelData, nf := readPixelData(t, "emri_small_jpeg_ls_lossless.dcm")
	if nf != 10 {
		t.Fatalf("NumberOfFrames = %d, want 10", nf)
	}
	frames, err := encaps.GenerateFrames(pixelData, encaps.FramesOptions{
		NumberOfFrames: nf,
		LittleEndian:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 10 {
		t.Fatalf("frames = %d, want 10", len(frames))
	}
}

func TestGetFrame_matchesGenerateFrames_JP2K(t *testing.T) {
	_, pixelData, nf := readPixelData(t, "emri_small_jpeg_2k_lossless.dcm")
	opts := encaps.FramesOptions{NumberOfFrames: nf, LittleEndian: true}
	all, err := encaps.GenerateFrames(pixelData, opts)
	if err != nil {
		t.Fatal(err)
	}
	for i := range all {
		one, err := encaps.GetFrame(pixelData, i, opts)
		if err != nil {
			t.Fatalf("GetFrame(%d): %v", i, err)
		}
		if !bytes.Equal(one, all[i]) {
			t.Fatalf("GetFrame(%d) differs from GenerateFrames[%d]", i, i)
		}
	}
}
