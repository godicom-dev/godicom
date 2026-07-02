package godicom_test

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/pixels"
)

var testDataDir = filepath.Join("pydicom", "src", "pydicom", "data", "test_files")

func testFilePath(name string) string {
	return filepath.Join(testDataDir, name)
}

// rgbAt returns three 8-bit samples at (row, col) in interleaved RGB layout.
func rgbAt(raw []byte, columns, row, col int) (r, g, b byte) {
	i := (row*columns + col) * 3
	return raw[i], raw[i+1], raw[i+2]
}

func TestPixelBytes_native_CT_small(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 128*128*2 {
		t.Fatalf("len = %d, want %d", len(raw), 128*128*2)
	}
}

func TestPixelBytes_J2K_MR_small(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("MR_small_jp2klossless.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64*64*2 {
		t.Fatalf("len = %d, want %d", len(raw), 64*64*2)
	}
	// pydicom reference: arr[0,31:34] == (422, 319, 361) for int16 LE
	if got := int16(binary.LittleEndian.Uint16(raw[(0*64+31)*2:])); got != 422 {
		t.Fatalf("arr[0,31] = %d, want 422", got)
	}
	if got := int16(binary.LittleEndian.Uint16(raw[(31*64+0)*2:])); got != 366 {
		t.Fatalf("arr[31,0] = %d, want 366", got)
	}
}

func TestPixelBytes_RLE_MR_small(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("MR_small_RLE.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64*64*2 {
		t.Fatalf("len = %d, want %d", len(raw), 64*64*2)
	}
	if got := int16(binary.LittleEndian.Uint16(raw[(0*64+31)*2:])); got != 422 {
		t.Fatalf("arr[0,31] = %d, want 422", got)
	}
	if got := int16(binary.LittleEndian.Uint16(raw[(31*64+0)*2:])); got != 366 {
		t.Fatalf("arr[31,0] = %d, want 366", got)
	}
}

func TestPixelBytes_JPEGLS_lossless_MR_small(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("MR_small_jpeg_ls_lossless.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64*64*2 {
		t.Fatalf("len = %d, want %d", len(raw), 64*64*2)
	}
	if got := int16(binary.LittleEndian.Uint16(raw[(0*64+31)*2:])); got != 422 {
		t.Fatalf("arr[0,31] = %d, want 422", got)
	}
	if got := int16(binary.LittleEndian.Uint16(raw[(31*64+0)*2:])); got != 366 {
		t.Fatalf("arr[31,0] = %d, want 366", got)
	}
}

func TestPixelBytes_JPEGLS_nearLossless_08(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("JPEGLSNearLossless_08.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 45*10 {
		t.Fatalf("len = %d, want %d", len(raw), 45*10)
	}
	// pydicom pixels_reference JLSN_08_01_1_0_1F
	cases := []struct {
		row, col int
		want     byte
	}{
		{0, 0, 255},
		{5, 0, 125},
		{10, 0, 65},
		{15, 0, 30},
		{20, 0, 15},
		{25, 0, 5},
		{30, 0, 5},
		{35, 0, 0},
		{40, 0, 0},
	}
	for _, tc := range cases {
		got := raw[tc.row*10+tc.col]
		if got != tc.want {
			t.Fatalf("arr[%d,%d] = %d, want %d", tc.row, tc.col, got, tc.want)
		}
	}
}

func TestPixelBytes_JPEG_baseline_SC_rgb_lossy(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("SC_rgb_jpeg_lossy_gdcm.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 100*100*3 {
		t.Fatalf("len = %d, want %d", len(raw), 100*100*3)
	}
	// pydicom pixels_reference JPGB_08_08_3_1F_YBR_FULL (decoded sample values)
	r, g, b := rgbAt(raw, 100, 5, 50)
	if r != 76 || g != 85 || b != 255 {
		t.Fatalf("arr[5,50,:] = (%d,%d,%d), want (76,85,255)", r, g, b)
	}
	r, g, b = rgbAt(raw, 100, 95, 50)
	if r != 255 || g != 128 || b != 128 {
		t.Fatalf("arr[95,50,:] = (%d,%d,%d), want (255,128,128)", r, g, b)
	}
}

func TestPixelBytes_JPEG_losslessSV1_SC_rgb(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("SC_rgb_jpeg_gdcm.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 100*100*3 {
		t.Fatalf("len = %d, want %d", len(raw), 100*100*3)
	}
	// pydicom pixels_reference JPGS_08_08_3_1F_RGB
	r, g, b := rgbAt(raw, 100, 5, 50)
	if r != 255 || g != 0 || b != 0 {
		t.Fatalf("arr[5,50,:] = (%d,%d,%d), want (255,0,0)", r, g, b)
	}
	r, g, b = rgbAt(raw, 100, 95, 50)
	if r != 255 || g != 255 || b != 255 {
		t.Fatalf("arr[95,50,:] = (%d,%d,%d), want (255,255,255)", r, g, b)
	}
}

func TestPixelFrames_singleIndex(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("MR_small_jp2klossless.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := ds.PixelFrames(pixels.WithRaw(true), pixels.WithFrameIndex(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || len(frames[0]) != 64*64*2 {
		t.Fatalf("unexpected frames: %d lengths", len(frames))
	}
}

const (
	emriRows    = 64
	emriColumns = 64
	emriFrames  = 10
	emriBPS     = 2 // uint16
)

func emriFrameBytes() int {
	return emriRows * emriColumns * emriBPS
}

// u16At returns a little-endian uint16 sample at (row, col) in one emri frame.
func u16At(frame []byte, row, col int) uint16 {
	i := (row*emriColumns + col) * emriBPS
	return binary.LittleEndian.Uint16(frame[i:])
}

func assertEmriFramePixels(t *testing.T, frame []byte, frameIndex int) {
	t.Helper()
	if len(frame) != emriFrameBytes() {
		t.Fatalf("frame %d len = %d, want %d", frameIndex, len(frame), emriFrameBytes())
	}
	type spot struct {
		row, col int
		want     uint16
	}
	var checks []spot
	switch frameIndex {
	case 0:
		checks = []spot{{0, 31, 206}, {0, 32, 197}, {0, 33, 159}, {31, 0, 49}, {31, 1, 78}, {31, 2, 128}}
	case 4:
		checks = []spot{{0, 31, 67}, {0, 32, 82}, {0, 33, 44}, {31, 0, 37}, {31, 1, 41}, {31, 2, 17}}
	case 9:
		checks = []spot{{0, 31, 72}, {0, 32, 86}, {0, 33, 69}, {31, 0, 25}, {31, 1, 4}, {31, 2, 9}}
	default:
		return
	}
	for _, c := range checks {
		if got := u16At(frame, c.row, c.col); got != c.want {
			t.Fatalf("frame %d [%d,%d] = %d, want %d", frameIndex, c.row, c.col, got, c.want)
		}
	}
}

func TestPixelBytes_native_emri_small_10frame(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("emri_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := ds.PixelFrames(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != emriFrames {
		t.Fatalf("frames = %d, want %d", len(frames), emriFrames)
	}
	for i, frame := range frames {
		assertEmriFramePixels(t, frame, i)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != emriFrameBytes()*emriFrames {
		t.Fatalf("len = %d, want %d", len(raw), emriFrameBytes()*emriFrames)
	}
}

func TestPixelBytes_RLE_emri_small_10frame(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("emri_small_RLE.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := ds.PixelFrames(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != emriFrames {
		t.Fatalf("frames = %d, want %d", len(frames), emriFrames)
	}
	for i, frame := range frames {
		assertEmriFramePixels(t, frame, i)
	}
}

func TestPixelBytes_JPEGLS_emri_small_10frame(t *testing.T) {
	native, err := godicom.ReadFile(testFilePath("emri_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	nativeFrames, err := native.PixelFrames(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	ds, err := godicom.ReadFile(testFilePath("emri_small_jpeg_ls_lossless.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := ds.PixelFrames(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != len(nativeFrames) {
		t.Fatalf("frames = %d, want %d", len(frames), len(nativeFrames))
	}
	for i := range frames {
		if !bytes.Equal(frames[i], nativeFrames[i]) {
			t.Fatalf("frame %d differs from native reference", i)
		}
	}
}

func TestPixelBytes_J2K_emri_small_10frame(t *testing.T) {
	native, err := godicom.ReadFile(testFilePath("emri_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	nativeFrames, err := native.PixelFrames(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	ds, err := godicom.ReadFile(testFilePath("emri_small_jpeg_2k_lossless.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := ds.PixelFrames(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != len(nativeFrames) {
		t.Fatalf("frames = %d, want %d", len(frames), len(nativeFrames))
	}
	for i := range frames {
		if !bytes.Equal(frames[i], nativeFrames[i]) {
			t.Fatalf("frame %d differs from native reference", i)
		}
	}
}

func TestPixelFrames_emri_indexed(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("emri_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, idx := range []int{0, 4, 9} {
		frames, err := ds.PixelFrames(pixels.WithRaw(true), pixels.WithFrameIndex(idx))
		if err != nil {
			t.Fatalf("frame %d: %v", idx, err)
		}
		if len(frames) != 1 {
			t.Fatalf("frame %d: got %d frames, want 1", idx, len(frames))
		}
		assertEmriFramePixels(t, frames[0], idx)
	}
}

func TestPixelBytes_JPEG_extended_JPGExtended(t *testing.T) {
	ds, err := godicom.ReadFile(testFilePath("JPGExtended.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1024*256*2 {
		t.Fatalf("len = %d, want %d", len(raw), 1024*256*2)
	}
	// pydicom pixels_reference JPGE_16_12_1_0_1F_M2
	if got := int16(binary.LittleEndian.Uint16(raw[(420*256+140)*2:])); got != 244 {
		t.Fatalf("arr[420,140] = %d, want 244", got)
	}
	if got := int16(binary.LittleEndian.Uint16(raw[(230*256+120)*2:])); got != 95 {
		t.Fatalf("arr[230,120] = %d, want 95", got)
	}
}
