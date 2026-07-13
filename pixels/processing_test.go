package pixels_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/pixels"
)

func TestConvertYBRFullToRGB(t *testing.T) {
	// pydicom convert_color_space reference values
	src := []byte{76, 85, 255, 255, 128, 128}
	got, err := pixels.ConvertColorSpace(src, "YBR_FULL", "RGB", 8)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{254, 0, 0, 255, 255, 255}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestConvertRGBToYBRFullRoundtripSpot(t *testing.T) {
	rgb := []byte{255, 0, 0, 255, 255, 255}
	ybr, err := pixels.ConvertColorSpace(rgb, "RGB", "YBR_FULL", 8)
	if err != nil {
		t.Fatal(err)
	}
	back, err := pixels.ConvertColorSpace(ybr, "YBR_FULL", "RGB", 8)
	if err != nil {
		t.Fatal(err)
	}
	// Roundtrip may differ by ±1 due to integer rounding.
	for i := range rgb {
		d := int(back[i]) - int(rgb[i])
		if d < -1 || d > 1 {
			t.Fatalf("roundtrip[%d] = %d, want ~%d (got ybr %v)", i, back[i], rgb[i], ybr)
		}
	}
}

func TestExpandYBR422(t *testing.T) {
	// Two pixels: Y0 Y1 Cb Cr → Y0 Cb Cr Y1 Cb Cr
	src := []byte{10, 20, 30, 40}
	got, err := pixels.ExpandYBR422(src, 8)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{10, 30, 40, 20, 30, 40}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPlanarToColorByPixel(t *testing.T) {
	// 2x1 RGB planar: R=[1,2] G=[3,4] B=[5,6]
	src := []byte{1, 2, 3, 4, 5, 6}
	got, err := pixels.PlanarToColorByPixel(src, 1, 2, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{1, 3, 5, 2, 4, 6}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	back, err := pixels.ColorByPixelToPlanar(got, 1, 2, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, src) {
		t.Fatalf("roundtrip planar = %v", back)
	}
}

func TestPixelBytesYBRConvertedToRGB(t *testing.T) {
	path := filepath.Join("..", "pydicom", "src", "pydicom", "data", "test_files", "SC_rgb_jpeg_lossy_gdcm.dcm")
	ds, err := godicom.ReadFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ds.PixelBytes(pixels.WithRaw(true))
	if err != nil {
		t.Fatal(err)
	}
	if raw[(5*100+50)*3] != 76 {
		t.Fatalf("raw Y not as expected: %v", raw[(5*100+50)*3:(5*100+50)*3+3])
	}
	rgb, err := ds.PixelBytes() // default Raw=false → convert to RGB
	if err != nil {
		t.Fatal(err)
	}
	r, g, b := rgb[(5*100+50)*3], rgb[(5*100+50)*3+1], rgb[(5*100+50)*3+2]
	if r != 254 || g != 0 || b != 0 {
		t.Fatalf("arr[5,50,:] = (%d,%d,%d), want (254,0,0)", r, g, b)
	}
	r, g, b = rgb[(95*100+50)*3], rgb[(95*100+50)*3+1], rgb[(95*100+50)*3+2]
	if r != 255 || g != 255 || b != 255 {
		t.Fatalf("arr[95,50,:] = (%d,%d,%d), want (255,255,255)", r, g, b)
	}
}
