package pixels

import (
	"fmt"
	"math"
)

// ExpandYBR422 expands horizontally subsampled YBR_FULL_422 data to YBR_FULL.
// Mirrors pydicom.pixels.utils.expand_ybr422.
func ExpandYBR422(src []byte, bitsAllocated int) ([]byte, error) {
	nBytes := bitsAllocated / 8
	if nBytes < 1 {
		return nil, fmt.Errorf("pixels: BitsAllocated %d too small for YBR_FULL_422 expand", bitsAllocated)
	}
	if len(src)%(nBytes*4) != 0 {
		return nil, fmt.Errorf("pixels: YBR_FULL_422 length %d not divisible by %d", len(src), nBytes*4)
	}
	length := len(src) / 2 * 3
	dst := make([]byte, length)
	stepSrc := nBytes * 4
	stepDst := nBytes * 6
	for ii := 0; ii < nBytes; ii++ {
		for srcOff, dstOff := 0, 0; srcOff < len(src); srcOff, dstOff = srcOff+stepSrc, dstOff+stepDst {
			y0 := src[0*nBytes+ii+srcOff]
			y1 := src[1*nBytes+ii+srcOff]
			cb := src[2*nBytes+ii+srcOff]
			cr := src[3*nBytes+ii+srcOff]
			dst[0*nBytes+ii+dstOff] = y0
			dst[1*nBytes+ii+dstOff] = cb
			dst[2*nBytes+ii+dstOff] = cr
			dst[3*nBytes+ii+dstOff] = y1
			dst[4*nBytes+ii+dstOff] = cb
			dst[5*nBytes+ii+dstOff] = cr
		}
	}
	return dst, nil
}

// ConvertColorSpace converts interleaved multi-sample pixel bytes between
// photometric interpretations. Currently supports YBR_FULL ↔ RGB (8-bit).
// Input layout must be color-by-pixel (Planar Configuration 0).
// Mirrors pydicom.pixels.processing.convert_color_space for the YBR_FULL path.
func ConvertColorSpace(src []byte, current, desired string, bitsAllocated int) ([]byte, error) {
	if current == desired {
		out := make([]byte, len(src))
		copy(out, src)
		return out, nil
	}
	if bitsAllocated != 8 {
		return nil, fmt.Errorf("pixels: ConvertColorSpace currently supports BitsAllocated=8, got %d", bitsAllocated)
	}
	switch {
	case (current == "YBR_FULL" || current == "YBR_FULL_422") && desired == "RGB":
		return convertYBRFullToRGB8(src)
	case current == "RGB" && (desired == "YBR_FULL" || desired == "YBR_FULL_422"):
		return convertRGBToYBRFull8(src)
	default:
		return nil, fmt.Errorf("pixels: unsupported color conversion %s → %s", current, desired)
	}
}

// PlanarToColorByPixel converts color-by-plane (PlanarConfiguration=1) samples
// to color-by-pixel (PlanarConfiguration=0) for a single frame.
func PlanarToColorByPixel(src []byte, rows, columns, samples, bytesPerSample int) ([]byte, error) {
	if samples < 1 || rows < 1 || columns < 1 || bytesPerSample < 1 {
		return nil, fmt.Errorf("pixels: invalid planar conversion dimensions")
	}
	pixels := rows * columns
	planeSize := pixels * bytesPerSample
	want := planeSize * samples
	if len(src) != want {
		return nil, fmt.Errorf("pixels: planar input length %d, want %d", len(src), want)
	}
	dst := make([]byte, want)
	for p := 0; p < pixels; p++ {
		for s := 0; s < samples; s++ {
			srcOff := s*planeSize + p*bytesPerSample
			dstOff := (p*samples + s) * bytesPerSample
			copy(dst[dstOff:dstOff+bytesPerSample], src[srcOff:srcOff+bytesPerSample])
		}
	}
	return dst, nil
}

// ColorByPixelToPlanar converts color-by-pixel to color-by-plane for a single frame.
func ColorByPixelToPlanar(src []byte, rows, columns, samples, bytesPerSample int) ([]byte, error) {
	if samples < 1 || rows < 1 || columns < 1 || bytesPerSample < 1 {
		return nil, fmt.Errorf("pixels: invalid planar conversion dimensions")
	}
	pixels := rows * columns
	planeSize := pixels * bytesPerSample
	want := planeSize * samples
	if len(src) != want {
		return nil, fmt.Errorf("pixels: interleaved input length %d, want %d", len(src), want)
	}
	dst := make([]byte, want)
	for p := 0; p < pixels; p++ {
		for s := 0; s < samples; s++ {
			srcOff := (p*samples + s) * bytesPerSample
			dstOff := s*planeSize + p*bytesPerSample
			copy(dst[dstOff:dstOff+bytesPerSample], src[srcOff:srcOff+bytesPerSample])
		}
	}
	return dst, nil
}

// ProcessFrame applies optional post-decode transforms for display-oriented output:
// YBR_FULL_422 expand, PlanarConfiguration→0, and YBR→RGB when asRGB is true.
func ProcessFrame(frame []byte, desc Descriptor, asRGB bool) ([]byte, error) {
	out := frame
	var err error

	pi := desc.PhotometricInterpretation
	if pi == "YBR_FULL_422" {
		out, err = ExpandYBR422(out, desc.BitsAllocated)
		if err != nil {
			return nil, err
		}
		pi = "YBR_FULL"
	}

	if desc.SamplesPerPixel > 1 && desc.PlanarConfiguration == 1 {
		out, err = PlanarToColorByPixel(out, desc.Rows, desc.Columns, desc.SamplesPerPixel, desc.BytesPerSample())
		if err != nil {
			return nil, err
		}
	}

	if asRGB && (pi == "YBR_FULL" || pi == "YBR_FULL_422") {
		out, err = ConvertColorSpace(out, "YBR_FULL", "RGB", desc.BitsAllocated)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ITU T.871 / DICOM C.7.6.3.1.2 matrices (float32 constants as in pydicom).
var (
	ybrFullToRGB = [3][3]float64{
		{1.000, 1.000, 1.000},
		{0.000, -0.114 * 1.772 / 0.587, 1.772},
		{1.402, -0.299 * 1.402 / 0.587, 0.000},
	}
	rgbToYBRFull = [3][3]float64{
		{+0.299, -0.299 / 1.772, +0.701 / 1.402},
		{+0.587, -0.587 / 1.772, -0.587 / 1.402},
		{+0.114, +0.886 / 1.772, -0.114 / 1.402},
	}
)

func convertYBRFullToRGB8(src []byte) ([]byte, error) {
	if len(src)%3 != 0 {
		return nil, fmt.Errorf("pixels: YBR_FULL length %d not divisible by 3", len(src))
	}
	dst := make([]byte, len(src))
	const mid = 128.0
	for i := 0; i < len(src); i += 3 {
		y := float64(src[i])
		cb := float64(src[i+1]) - mid
		cr := float64(src[i+2]) - mid
		r := y*ybrFullToRGB[0][0] + cb*ybrFullToRGB[1][0] + cr*ybrFullToRGB[2][0]
		g := y*ybrFullToRGB[0][1] + cb*ybrFullToRGB[1][1] + cr*ybrFullToRGB[2][1]
		b := y*ybrFullToRGB[0][2] + cb*ybrFullToRGB[1][2] + cr*ybrFullToRGB[2][2]
		dst[i] = clipU8(r)
		dst[i+1] = clipU8(g)
		dst[i+2] = clipU8(b)
	}
	return dst, nil
}

func convertRGBToYBRFull8(src []byte) ([]byte, error) {
	if len(src)%3 != 0 {
		return nil, fmt.Errorf("pixels: RGB length %d not divisible by 3", len(src))
	}
	dst := make([]byte, len(src))
	const mid = 128.0
	for i := 0; i < len(src); i += 3 {
		r := float64(src[i])
		g := float64(src[i+1])
		b := float64(src[i+2])
		y := r*rgbToYBRFull[0][0] + g*rgbToYBRFull[1][0] + b*rgbToYBRFull[2][0]
		cb := r*rgbToYBRFull[0][1] + g*rgbToYBRFull[1][1] + b*rgbToYBRFull[2][1] + mid
		cr := r*rgbToYBRFull[0][2] + g*rgbToYBRFull[1][2] + b*rgbToYBRFull[2][2] + mid
		dst[i] = clipU8(y)
		dst[i+1] = clipU8(cb)
		dst[i+2] = clipU8(cr)
	}
	return dst, nil
}

func clipU8(v float64) byte {
	// Round half up via floor(v+0.5), matching pydicom.
	v = math.Floor(v + 0.5)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}
