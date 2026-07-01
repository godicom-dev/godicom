package godicom_test

import (
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
