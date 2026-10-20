package godicom_test

import (
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/tag"
)

// Cases follow pydicom.tests.pixels.test_processing.TestApplyColorLUT and
// TestExpandSegmentedLUT's palette-file checks. test_palette_unavailable_raises
// renames an installed pydicom data file; the palettes here are embedded, so
// that case has no equivalent.

func TestApplyColorLUTNeitherDatasetNorPalette(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OBXXXX1A.dcm")
	arr := mustSamples(t, ds)
	_, err := godicom.ApplyColorLUT(arr, nil, "")
	if err == nil || !strings.Contains(err.Error(), "either a dataset or a palette is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyColorLUTUnknownPalette(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OBXXXX1A.dcm")
	arr := mustSamples(t, ds)
	_, err := godicom.ApplyColorLUT(arr, nil, "TEST")
	if err == nil || !strings.Contains(err.Error(), "unknown palette 'TEST'") {
		t.Fatalf("name: %v", err)
	}
	_, err = godicom.ApplyColorLUT(arr, nil, "1.2.840.10008.1.1")
	if err == nil || !strings.Contains(err.Error(), "unknown palette '1.2.840.10008.1.1'") {
		t.Fatalf("uid: %v", err)
	}
}

func TestApplyColorLUTSupplementalRaises(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "eCT_Supplemental.dcm")
	arr := mustSamples(t, ds)
	_, err := godicom.ApplyColorLUT(arr, ds, "")
	if err == nil || !strings.Contains(err.Error(), "Supplemental Palette Color Lookup Table Module is not currently supported") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyColorLUTInvalidBitDepth(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OBXXXX1A.dcm")
	desc := mustDescriptor(t, ds)
	desc[2] = 15
	ds.Set(godicom.NewDataElement(tag.RedPaletteColorLookupTableDescriptor, godicom.VRUS, desc))
	_, err := godicom.ApplyColorLUT(mustSamples(t, ds), ds, "")
	if err == nil || !strings.Contains(err.Error(), "must be 8 or 16-bit, not 15-bit") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyColorLUTInvalidLUTBitDepth(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OBXXXX1A.dcm")
	for _, tg := range []tag.Tag{
		tag.RedPaletteColorLookupTableData,
		tag.GreenPaletteColorLookupTableData,
		tag.BluePaletteColorLookupTableData,
	} {
		raw := mustBytes(t, ds, tg)
		ds.Set(godicom.NewDataElement(tg, godicom.VROW, raw[:len(raw)-2]))
	}
	_, err := godicom.ApplyColorLUT(mustSamples(t, ds), ds, "")
	if err == nil || !strings.Contains(err.Error(), "bit depth of the LUT data '15.9'") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyColorLUTUnequalLength(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OBXXXX1A.dcm")
	raw := mustBytes(t, ds, tag.BluePaletteColorLookupTableData)
	ds.Set(godicom.NewDataElement(tag.BluePaletteColorLookupTableData, godicom.VROW, raw[:len(raw)-2]))
	_, err := godicom.ApplyColorLUT(mustSamples(t, ds), ds, "")
	if err == nil || !strings.Contains(err.Error(), "LUT data must be the same length") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyColorLUTNoPalette(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OBXXXX1A.dcm")
	ds.Delete(tag.RedPaletteColorLookupTableData)
	_, err := godicom.ApplyColorLUT(mustSamples(t, ds), ds, "")
	if err == nil || !strings.Contains(err.Error(), "No suitable Palette Color Lookup Table Module found") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyColorLUTUint08_16(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OT-PAL-8-face.dcm")
	ensureImplicitLE(t, ds)
	if got, _ := ds.GetInt(tag.BitsStored); got != 8 {
		t.Fatalf("BitsStored = %d, want 8", got)
	}
	desc := mustDescriptor(t, ds)
	if desc[2] != 16 {
		t.Fatalf("LUT depth = %d, want 16", desc[2])
	}
	arr := mustSamples(t, ds)
	orig := append([]float64(nil), arr...)
	rgb, err := godicom.ApplyColorLUT(arr, ds, "")
	if err != nil {
		t.Fatal(err)
	}
	rows, cols := 480, 640
	if rgb.Channels != 3 || len(rgb.Samples) != rows*cols*3 {
		t.Fatalf("shape channels=%d len=%d", rgb.Channels, len(rgb.Samples))
	}
	assertRGB(t, rgb, cols, 0, 0, 0, 0, 0)
	assertRGB(t, rgb, cols, 0, 4, 9216, 9216, 9216)
	assertRGB(t, rgb, cols, 0, 9, 18688, 18688, 18688)
	assertRGB(t, rgb, cols, 0, 638, 27904, 33536, 0)
	assertRGB(t, rgb, cols, 479, 639, 18688, 24320, 0)
	if !floatEqual(arr, orig) {
		t.Fatal("ApplyColorLUT modified the input")
	}
}

func TestApplyColorLUTUint08_16TwoFrame(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OBXXXX1A_2frame.dcm")
	desc := mustDescriptor(t, ds)
	if desc[2] != 16 {
		t.Fatalf("LUT depth = %d, want 16", desc[2])
	}
	arr := mustSamples(t, ds)
	orig := append([]float64(nil), arr...)
	rgb, err := godicom.ApplyColorLUT(arr, ds, "")
	if err != nil {
		t.Fatal(err)
	}
	rows, cols := 600, 800
	if rgb.Channels != 3 || len(rgb.Samples) != 2*rows*cols*3 {
		t.Fatalf("shape channels=%d len=%d", rgb.Channels, len(rgb.Samples))
	}
	assertRGBFrame(t, rgb, rows, cols, 0, 0, 0, 9472, 15872, 24064)
	assertRGBFrame(t, rgb, rows, cols, 0, 12, 12, 34816, 43520, 54016)
	assertRGBFrame(t, rgb, rows, cols, 0, 17, 110, 65280, 65280, 65280)
	assertRGBFrame(t, rgb, rows, cols, 0, 77, 103, 0, 0, 0)
	assertRGBFrame(t, rgb, rows, cols, 0, 478, 793, 23040, 52480, 65280)
	ref := [][3]uint16{
		{26112, 26112, 26112},
		{54528, 54528, 54528},
		{54528, 54528, 54528},
		{16640, 16640, 16640},
		{49152, 45056, 22016},
		{34816, 43520, 54016},
		{5632, 9984, 14848},
		{62464, 2816, 2816},
		{3072, 5632, 8192},
		{3072, 5632, 8192},
	}
	for i, want := range ref {
		assertRGBFrame(t, rgb, rows, cols, 1, 143+i, 355, want[0], want[1], want[2])
	}
	if !floatEqual(arr, orig) {
		t.Fatal("ApplyColorLUT modified the input")
	}
}

func TestApplyColorLUTSegmentedLittle(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "gdcm-US-ALOKA-16.dcm")
	arr := mustSamples(t, ds)
	checkALOKA(t, ds, arr)

	ds.FileMeta = nil
	checkALOKA(t, ds, arr)
	// pydicom then clears Dataset._read_little and expects an endianness error.
	// ReadFile always records originalEnc, so that third case has no equivalent.
}

func TestApplyColorLUTSegmentedBig(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "gdcm-US-ALOKA-16_big.dcm")
	checkALOKA(t, ds, mustSamples(t, ds))
}

func TestApplyColorLUTEightBitEntriesInSixteenAllocated(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OT-PAL-8-face.dcm")
	ensureImplicitLE(t, ds)
	ds.Set(godicom.NewDataElement(tag.RedPaletteColorLookupTableDescriptor, godicom.VRUS, []int{200, 0, 8}))
	lut := make([]byte, 400)
	for i := 0; i < 200; i++ {
		lut[i*2] = byte(i)
		lut[i*2+1] = 0
	}
	for _, tg := range []tag.Tag{
		tag.RedPaletteColorLookupTableData,
		tag.GreenPaletteColorLookupTableData,
		tag.BluePaletteColorLookupTableData,
	} {
		cp := append([]byte(nil), lut...)
		ds.Set(godicom.NewDataElement(tg, godicom.VROW, cp))
	}
	arr := mustSamples(t, ds)
	min, max := arr[0], arr[0]
	for _, v := range arr {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	if min != 56 || max != 149 {
		t.Fatalf("pixel range %v..%v, want 56..149", min, max)
	}
	out, err := godicom.ApplyColorLUT(arr, ds, "")
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range arr {
		for c := 0; c < 3; c++ {
			if got := out.At(i, c); got != uint16(v) {
				t.Fatalf("pixel %d channel %d = %d, want %v", i, c, got, v)
			}
		}
	}
}

func TestApplyColorLUTAlpha(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OBXXXX1A.dcm")
	alpha := make([]byte, 512)
	for i := 0; i < 256; i++ {
		alpha[i*2] = 0x00
		alpha[i*2+1] = 0x80
	}
	ds.Set(godicom.NewDataElement(tag.AlphaPaletteColorLookupTableData, godicom.VROW, alpha))
	rgba, err := godicom.ApplyColorLUT(mustSamples(t, ds), ds, "")
	if err != nil {
		t.Fatal(err)
	}
	rows, cols := 600, 800
	if rgba.Channels != 4 || len(rgba.Samples) != rows*cols*4 {
		t.Fatalf("shape channels=%d len=%d", rgba.Channels, len(rgba.Samples))
	}
	if rgba.At(0, 3) != 32768 {
		t.Fatalf("alpha[0,0] = %d, want 32768", rgba.At(0, 3))
	}
	for i := 0; i < rows*cols; i++ {
		if rgba.At(i, 3) != 32768 {
			t.Fatalf("alpha pixel %d = %d", i, rgba.At(i, 3))
		}
	}
}

func TestApplyColorLUTWellKnownPET(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OBXXXX1A.dcm")
	arr := mustSamples(t, ds)
	rgb, err := godicom.ApplyColorLUT(arr, nil, "PET")
	if err != nil {
		t.Fatal(err)
	}
	cols := 800
	ref := [][3]uint16{
		{249, 122, 12},
		{255, 130, 4},
		{255, 136, 16},
		{255, 134, 12},
		{253, 126, 4},
		{239, 112, 32},
		{211, 84, 88},
		{197, 70, 116},
		{177, 50, 156},
		{168, 40, 176},
		{173, 46, 164},
		{185, 58, 140},
		{207, 80, 96},
		{209, 82, 92},
		{189, 62, 132},
		{173, 46, 164},
		{168, 40, 176},
		{162, 34, 188},
		{162, 34, 188},
		{154, 26, 204},
	}
	for i, want := range ref {
		assertRGB(t, rgb, cols, 68+i, 364, want[0], want[1], want[2])
	}
	byUID, err := godicom.ApplyColorLUT(arr, nil, "1.2.840.10008.1.5.2")
	if err != nil {
		t.Fatal(err)
	}
	if len(byUID.Samples) != len(rgb.Samples) {
		t.Fatalf("uid len %d, name len %d", len(byUID.Samples), len(rgb.Samples))
	}
	for i := range rgb.Samples {
		if byUID.Samples[i] != rgb.Samples[i] {
			t.Fatalf("uid and name differ at %d", i)
		}
	}
}

func TestApplyColorLUTFirstMapPositive(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OT-PAL-8-face.dcm")
	ensureImplicitLE(t, ds)
	desc := mustDescriptor(t, ds)
	desc[1] = 10
	ds.Set(godicom.NewDataElement(tag.RedPaletteColorLookupTableDescriptor, godicom.VRUS, desc))
	arr := mustSamples(t, ds)
	rgb, err := godicom.ApplyColorLUT(arr, ds, "")
	if err != nil {
		t.Fatal(err)
	}
	assertWhere(t, arr, rgb, 88, 33280, 56320, 65280)
	assertWhere(t, arr, rgb, 149, 50944, 16384, 27904)
}

func TestApplyColorLUTFirstMapNegative(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OT-PAL-8-face.dcm")
	ensureImplicitLE(t, ds)
	desc := mustDescriptor(t, ds)
	desc[1] = -10
	ds.Set(godicom.NewDataElement(tag.RedPaletteColorLookupTableDescriptor, godicom.VRSS, desc))
	arr := mustSamples(t, ds)
	rgb, err := godicom.ApplyColorLUT(arr, ds, "")
	if err != nil {
		t.Fatal(err)
	}
	assertWhere(t, arr, rgb, 60, 33280, 61952, 65280)
	assertWhere(t, arr, rgb, 130, 60160, 25600, 37376)
}

func TestApplyColorLUTNoModule(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "CT_small.dcm")
	if _, ok := ds.Get(tag.RedPaletteColorLookupTableDescriptor); ok {
		t.Fatal("CT_small unexpectedly has a palette descriptor")
	}
	_, err := godicom.ApplyColorLUT(mustSamples(t, ds), ds, "")
	if err == nil || !strings.Contains(err.Error(), "No suitable Palette Color Lookup Table Module found") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyColorLUTSegmentedWellKnownPalettes(t *testing.T) {
	t.Parallel()
	samples := make([]float64, 256)
	for i := range samples {
		samples[i] = float64(i)
	}
	spring, err := godicom.ApplyColorLUT(samples, nil, "SPRING")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		if spring.At(i, 0) != 255 || spring.At(i, 1) != uint16(i) || spring.At(i, 2) != uint16(255-i) {
			t.Fatalf("SPRING[%d] = %d %d %d", i, spring.At(i, 0), spring.At(i, 1), spring.At(i, 2))
		}
	}
	fall, err := godicom.ApplyColorLUT(samples, nil, "FALL")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		if fall.At(i, 0) != 255 || fall.At(i, 1) != uint16(255-i) || fall.At(i, 2) != 0 {
			t.Fatalf("FALL[%d] = %d %d %d", i, fall.At(i, 0), fall.At(i, 1), fall.At(i, 2))
		}
	}
	winter, err := godicom.ApplyColorLUT(samples, nil, "WINTER")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 128; i++ {
		if winter.At(i, 0) != 0 {
			t.Fatalf("WINTER red[%d] = %d, want 0", i, winter.At(i, 0))
		}
	}
	for i := 0; i < 256; i++ {
		if winter.At(i, 1) != uint16(i) {
			t.Fatalf("WINTER green[%d] = %d, want %d", i, winter.At(i, 1), i)
		}
	}
	wantBlueHead := []uint16{255, 255, 254, 254, 253}
	wantBlueTail := []uint16{130, 129, 129, 128, 128}
	wantRedTail := []uint16{123, 124, 125, 126, 127}
	for i, want := range wantBlueHead {
		if winter.At(i, 2) != want {
			t.Fatalf("WINTER blue[%d] = %d, want %d", i, winter.At(i, 2), want)
		}
	}
	for i, want := range wantBlueTail {
		if winter.At(251+i, 2) != want {
			t.Fatalf("WINTER blue[%d] = %d, want %d", 251+i, winter.At(251+i, 2), want)
		}
	}
	for i, want := range wantRedTail {
		if winter.At(251+i, 0) != want {
			t.Fatalf("WINTER red[%d] = %d, want %d", 251+i, winter.At(251+i, 0), want)
		}
	}

	summer, err := godicom.ApplyColorLUT(samples, nil, "SUMMER")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		if summer.At(i, 0) != 0 {
			t.Fatalf("SUMMER red[%d] = %d, want 0", i, summer.At(i, 0))
		}
	}
	wantGreenHead := []uint16{255, 255, 254, 254, 253}
	wantGreenTail := []uint16{130, 129, 129, 128, 128}
	for i, want := range wantGreenHead {
		if summer.At(i, 1) != want {
			t.Fatalf("SUMMER green[%d] = %d, want %d", i, summer.At(i, 1), want)
		}
	}
	for i, want := range wantGreenTail {
		if summer.At(251+i, 1) != want {
			t.Fatalf("SUMMER green[%d] = %d, want %d", 251+i, summer.At(251+i, 1), want)
		}
	}
	for i := 0; i < 128; i++ {
		if summer.At(i, 2) != 0 {
			t.Fatalf("SUMMER blue[%d] = %d, want 0", i, summer.At(i, 2))
		}
	}
	wantSummerBlueTail := []uint16{246, 248, 250, 252, 254}
	for i, want := range wantSummerBlueTail {
		if summer.At(251+i, 2) != want {
			t.Fatalf("SUMMER blue[%d] = %d, want %d", 251+i, summer.At(251+i, 2), want)
		}
	}
}

func TestDisplayFramePaletteColor(t *testing.T) {
	t.Parallel()
	ds := mustPalette(t, "OBXXXX1A.dcm")
	frame, err := ds.DisplayFrame(0)
	if err != nil {
		t.Fatal(err)
	}
	// apply_color_lut reports [9472, 15872, 24064] at (0, 0). Display keeps the high byte.
	if frame[0] != 37 || frame[1] != 62 || frame[2] != 94 {
		t.Fatalf("display (0,0) = %d %d %d, want 37 62 94", frame[0], frame[1], frame[2])
	}
}

func BenchmarkApplyColorLUT(b *testing.B) {
	ds, err := godicom.ReadFile(testFilePath("OT-PAL-8-face.dcm"), &godicom.ReadOptions{Force: true})
	if err != nil {
		b.Skip(err)
	}
	// The fixture has no Transfer Syntax UID. pydicom's test assigns Implicit VR Little Endian before decoding.
	if _, ok := ds.TransferSyntaxUID(); !ok {
		if ds.FileMeta == nil {
			ds.FileMeta = godicom.NewFileMetaDataset()
		}
		if err := ds.FileMeta.SetString(tag.TransferSyntaxUID, "1.2.840.10008.1.2"); err != nil {
			b.Fatal(err)
		}
	}
	arr, err := ds.PixelSamples()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := godicom.ApplyColorLUT(arr, ds, ""); err != nil {
			b.Fatal(err)
		}
	}
}

func checkALOKA(t *testing.T, ds *godicom.FileDataset, arr []float64) {
	t.Helper()
	if got, _ := ds.GetInt(tag.BitsStored); got != 16 {
		t.Fatalf("BitsStored = %d, want 16", got)
	}
	desc := mustDescriptor(t, ds)
	if desc[2] != 16 {
		t.Fatalf("LUT depth = %d, want 16", desc[2])
	}
	rgb, err := godicom.ApplyColorLUT(arr, ds, "")
	if err != nil {
		t.Fatal(err)
	}
	cols := 640
	assertRGB(t, rgb, cols, 0, 0, 10280, 11565, 16705)
	assertRGB(t, rgb, cols, 0, 320, 10280, 11565, 16705)
	assertRGB(t, rgb, cols, 0, 639, 10280, 11565, 16705)
	assertRGB(t, rgb, cols, 240, 0, 0, 0, 0)
	assertRGB(t, rgb, cols, 240, 320, 257, 257, 257)
	assertRGB(t, rgb, cols, 240, 639, 2313, 2313, 2313)
	assertRGB(t, rgb, cols, 479, 0, 10280, 11565, 16705)
	assertRGB(t, rgb, cols, 479, 320, 10280, 11565, 16705)
	assertRGB(t, rgb, cols, 479, 639, 10280, 11565, 16705)
}

func ensureImplicitLE(t *testing.T, ds *godicom.FileDataset) {
	t.Helper()
	if _, ok := ds.TransferSyntaxUID(); ok {
		return
	}
	if ds.FileMeta == nil {
		ds.FileMeta = godicom.NewFileMetaDataset()
	}
	if err := ds.FileMeta.SetString(tag.TransferSyntaxUID, "1.2.840.10008.1.2"); err != nil {
		t.Fatal(err)
	}
}

func mustPalette(t *testing.T, name string) *godicom.FileDataset {
	t.Helper()
	path := testFilePath(name)
	if !fileExists(path) {
		t.Skipf("palette fixture %s not found (run scripts/fetch-testdata.sh)", name)
	}
	ds, err := godicom.ReadFile(path, &godicom.ReadOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func mustSamples(t *testing.T, ds *godicom.FileDataset) []float64 {
	t.Helper()
	arr, err := ds.PixelSamples()
	if err != nil {
		t.Fatal(err)
	}
	return arr
}

func mustDescriptor(t *testing.T, ds *godicom.FileDataset) []int {
	t.Helper()
	elem, ok := ds.Get(tag.RedPaletteColorLookupTableDescriptor)
	if !ok || elem == nil {
		t.Fatal("missing Red Palette Color Lookup Table Descriptor")
	}
	switch v := elem.Value.(type) {
	case []int:
		return append([]int(nil), v...)
	case *godicom.MultiValue[int]:
		return append([]int(nil), v.Values()...)
	case *godicom.MultiValue[uint64]:
		out := make([]int, v.Len())
		for i, n := range v.Values() {
			out[i] = int(n)
		}
		return out
	case *godicom.MultiValue[int64]:
		out := make([]int, v.Len())
		for i, n := range v.Values() {
			out[i] = int(n)
		}
		return out
	default:
		t.Fatalf("descriptor type %T", elem.Value)
	}
	return nil
}

func mustBytes(t *testing.T, ds *godicom.FileDataset, tg tag.Tag) []byte {
	t.Helper()
	elem, ok := ds.Get(tg)
	if !ok || elem == nil {
		t.Fatalf("missing %s", tg)
	}
	b, ok := elem.Value.([]byte)
	if !ok {
		t.Fatalf("%s type %T", tg, elem.Value)
	}
	return b
}

func assertRGB(t *testing.T, p godicomPalette, cols, row, col int, r, g, b uint16) {
	t.Helper()
	assertRGBFrame(t, p, 0, cols, 0, row, col, r, g, b)
}

type godicomPalette = interface {
	At(pixel, channel int) uint16
}

func assertRGBFrame(t *testing.T, p godicomPalette, rows, cols, frame, row, col int, r, g, b uint16) {
	t.Helper()
	i := row*cols + col
	if rows > 0 {
		i = (frame*rows+row)*cols + col
	}
	if p.At(i, 0) != r || p.At(i, 1) != g || p.At(i, 2) != b {
		t.Fatalf("pixel frame %d (%d,%d) = %d %d %d, want %d %d %d", frame, row, col, p.At(i, 0), p.At(i, 1), p.At(i, 2), r, g, b)
	}
}

func assertWhere(t *testing.T, arr []float64, p godicomPalette, value float64, r, g, b uint16) {
	t.Helper()
	n := 0
	for i, v := range arr {
		if v != value {
			continue
		}
		n++
		if p.At(i, 0) != r || p.At(i, 1) != g || p.At(i, 2) != b {
			t.Fatalf("pixel %d value %v = %d %d %d, want %d %d %d", i, value, p.At(i, 0), p.At(i, 1), p.At(i, 2), r, g, b)
		}
	}
	if n == 0 {
		t.Fatalf("no pixel with value %v", value)
	}
}

func floatEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
