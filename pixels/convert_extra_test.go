package pixels_test

import (
	"bytes"
	"testing"

	"github.com/godicom-dev/godicom/pixels"
)

func TestExpandYBR422SixteenBit(t *testing.T) {
	// 16-bit: y0=[1,2] y1=[3,4] cb=[5,6] cr=[7,8] as LE words -> two pixels
	// (y0,cb,cr) and (y1,cb,cr). Exercises the per-byte-plane loop, which is the
	// only path that differs from the 8-bit case.
	src := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	got, err := pixels.ExpandYBR422(src, 16)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{1, 2, 5, 6, 7, 8, 3, 4, 5, 6, 7, 8}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandYBR422Errors(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  []byte
		bits int
	}{
		{"no bytes per sample", []byte{1, 2, 3, 4}, 4},
		{"zero bits", []byte{1, 2, 3, 4}, 0},
		{"length not a multiple of four samples", []byte{1, 2, 3, 4, 5, 6}, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pixels.ExpandYBR422(tc.src, tc.bits); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	// An empty value is legal CS-style padding, not a malformed sub-sampled
	// frame, and expands to nothing.
	out, err := pixels.ExpandYBR422(nil, 8)
	if err != nil {
		t.Fatalf("empty input: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("empty input expanded to %v", out)
	}
}

func TestConvertColorSpaceIdentityCopies(t *testing.T) {
	src := []byte{1, 2, 3}
	out, err := pixels.ConvertColorSpace(src, "RGB", "RGB", 8)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, src) {
		t.Fatalf("got %v want %v", out, src)
	}
	out[0] = 99
	if src[0] != 1 {
		t.Fatal("ConvertColorSpace aliased the input instead of copying it")
	}
	// The identity path is checked before the bit-depth guard, so a non-8-bit
	// request still succeeds for an identity conversion.
	if _, err := pixels.ConvertColorSpace(src, "MONOCHROME2", "MONOCHROME2", 16); err != nil {
		t.Fatalf("identity at 16 bits: %v", err)
	}
}

func TestConvertColorSpaceGrayEndpoints(t *testing.T) {
	// cb=cr=128 is the achromatic axis, so Y maps straight through at both ends.
	src := []byte{0, 128, 128, 255, 128, 128}
	got, err := pixels.ConvertColorSpace(src, "YBR_FULL_422", "RGB", 8)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0, 0, 0, 255, 255, 255}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestConvertColorSpaceErrors(t *testing.T) {
	for _, tc := range []struct {
		name          string
		current       string
		desired       string
		bitsAllocated int
		src           []byte
	}{
		{"16 bit", "YBR_FULL", "RGB", 16, []byte{1, 2, 3}},
		{"unsupported pair", "RGB", "YBR_ICT", 8, []byte{1, 2, 3}},
		{"monochrome to rgb", "MONOCHROME2", "RGB", 8, []byte{1}},
		{"ybr length not divisible by three", "YBR_FULL", "RGB", 8, []byte{1, 2, 3, 4}},
		{"rgb length not divisible by three", "RGB", "YBR_FULL", 8, []byte{1, 2, 3, 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pixels.ConvertColorSpace(tc.src, tc.current, tc.desired, tc.bitsAllocated); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestPlanarConversionsSixteenBit(t *testing.T) {
	// 2 pixels, 2 samples, 2 bytes each: plane 0 is [1,2][3,4], plane 1 is
	// [5,6][7,8]. Color-by-pixel must interleave word by word, not byte by byte.
	src := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	cbyp, err := pixels.PlanarToColorByPixel(src, 1, 2, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{1, 2, 5, 6, 3, 4, 7, 8}
	if !bytes.Equal(cbyp, want) {
		t.Fatalf("planar->cbyp got %v want %v", cbyp, want)
	}
	back, err := pixels.ColorByPixelToPlanar(cbyp, 1, 2, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, src) {
		t.Fatalf("roundtrip got %v want %v", back, src)
	}
}

func TestPlanarConversionsErrors(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		rows, cols, samples, bytesPerSm int
		src                             []byte
	}{
		{"no samples", 1, 2, 0, 1, []byte{1, 2}},
		{"no rows", 0, 2, 3, 1, []byte{1, 2}},
		{"no columns", 1, 0, 3, 1, []byte{1, 2}},
		{"no bytes per sample", 1, 2, 3, 0, []byte{1, 2}},
		{"wrong length", 1, 2, 3, 1, []byte{1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pixels.PlanarToColorByPixel(tc.src, tc.rows, tc.cols, tc.samples, tc.bytesPerSm); err == nil {
				t.Fatal("PlanarToColorByPixel: expected error")
			}
			if _, err := pixels.ColorByPixelToPlanar(tc.src, tc.rows, tc.cols, tc.samples, tc.bytesPerSm); err == nil {
				t.Fatal("ColorByPixelToPlanar: expected error")
			}
		})
	}
}

func TestProcessFramePlanarToColorByPixel(t *testing.T) {
	desc := pixels.Descriptor{
		Rows:                      1,
		Columns:                   2,
		SamplesPerPixel:           3,
		BitsAllocated:             8,
		PhotometricInterpretation: "RGB",
		PlanarConfiguration:       1,
	}
	out, err := pixels.ProcessFrame([]byte{1, 2, 3, 4, 5, 6}, desc, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, []byte{1, 3, 5, 2, 4, 6}) {
		t.Fatalf("got %v", out)
	}
}

func TestProcessFrameYBR422ExpandsAndConverts(t *testing.T) {
	desc := pixels.Descriptor{
		Rows:                      2,
		Columns:                   2,
		SamplesPerPixel:           3,
		BitsAllocated:             8,
		PhotometricInterpretation: "YBR_FULL_422",
	}
	// Two YBR422 groups: pixel pair 0 at Y=255 (white), pair 1 at Y=0 (black).
	src := []byte{255, 255, 128, 128, 0, 0, 128, 128}

	raw, err := pixels.ProcessFrame(src, desc, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 12 {
		t.Fatalf("expanded length = %d, want 12", len(raw))
	}
	if !bytes.Equal(raw[:3], []byte{255, 128, 128}) {
		t.Fatalf("first expanded pixel = %v, want [255 128 128]", raw[:3])
	}

	rgb, err := pixels.ProcessFrame(src, desc, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rgb[:3], []byte{255, 255, 255}) || !bytes.Equal(rgb[6:9], []byte{0, 0, 0}) {
		t.Fatalf("rgb = %v, want white then black", rgb)
	}
}

func TestProcessFrameLeavesMonochromeAlone(t *testing.T) {
	desc := pixels.Descriptor{
		Rows:                      1,
		Columns:                   2,
		SamplesPerPixel:           1,
		BitsAllocated:             8,
		PhotometricInterpretation: "MONOCHROME2",
	}
	src := []byte{1, 2}
	out, err := pixels.ProcessFrame(src, desc, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, src) {
		t.Fatalf("got %v want %v", out, src)
	}
}

func TestProcessFramePropagatesExpandError(t *testing.T) {
	desc := pixels.Descriptor{
		Rows:                      1,
		Columns:                   2,
		SamplesPerPixel:           3,
		BitsAllocated:             8,
		PhotometricInterpretation: "YBR_FULL_422",
	}
	if _, err := pixels.ProcessFrame([]byte{1, 2, 3}, desc, true); err == nil {
		t.Fatal("expected YBR_FULL_422 length error")
	}
	desc.PhotometricInterpretation = "RGB"
	desc.PlanarConfiguration = 1
	if _, err := pixels.ProcessFrame([]byte{1, 2, 3}, desc, false); err == nil {
		t.Fatal("expected planar length error")
	}
}

func TestClipU8RoundsHalfUp(t *testing.T) {
	// 0.5 must go up, not to even, and out-of-range values must clamp. Both are
	// visible through the colour conversion, which is the only exported user.
	src := []byte{1, 128, 128}
	out, err := pixels.ConvertColorSpace(src, "YBR_FULL", "RGB", 8)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, []byte{1, 1, 1}) {
		t.Fatalf("got %v", out)
	}
	// The clamp has to stop at both ends rather than wrap around the byte:
	// Y=0 with a saturated Cb drives R and G negative, Y=255 with a saturated
	// Cr drives R past 255.
	low := []byte{0, 255, 128}
	out, err = pixels.ConvertColorSpace(low, "YBR_FULL", "RGB", 8)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 0 || out[1] != 0 {
		t.Fatalf("negative channels clamped to %v, want 0", out[:2])
	}
	high := []byte{255, 128, 255}
	out, err = pixels.ConvertColorSpace(high, "YBR_FULL", "RGB", 8)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 255 {
		t.Fatalf("overflowing channel clamped to %d, want 255", out[0])
	}
}
