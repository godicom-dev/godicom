package godicom

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
)

// setSQ stores a sequence without going through the dictionary VR check. The
// functional group macros are all SQ in the standard, but building them this
// way keeps the fixtures independent of dictionary regeneration.
type elementSetter interface {
	Set(*DataElement)
}

func setSQ(t *testing.T, d elementSetter, tg Tag, items ...*Dataset) {
	t.Helper()
	d.Set(NewDataElement(tg, VRSQ, NewSequence(items)))
}

// enhancedDataset builds a two-frame enhanced instance: a 2x2 16-bit
// monochrome image with identical samples in both frames, so any difference in
// the display output can only come from the functional groups.
func enhancedDataset(t *testing.T) *FileDataset {
	t.Helper()
	fd := newLUTDataset()
	fd.Set(NewDataElement(tag.NumberOfFrames, VRIS, 2))
	pix := make([]byte, 16)
	for frame := 0; frame < 2; frame++ {
		for i, v := range []uint16{0, 1, 2, 3} {
			binary.LittleEndian.PutUint16(pix[(frame*4+i)*2:], v)
		}
	}
	fd.Set(NewDataElement(tag.PixelData, VROW, pix))
	return fd
}

// sharedGroup installs an empty Shared Functional Groups item and returns it so
// the test can attach macros to it.
func sharedGroup(t *testing.T, fd *FileDataset) *Dataset {
	t.Helper()
	item := NewDataset()
	setSQ(t, fd, tag.SharedFunctionalGroupsSequence, item)
	return item
}

func macro(t *testing.T, macroTag Tag, elem *Dataset) *Dataset {
	t.Helper()
	group := NewDataset()
	setSQ(t, group, macroTag, elem)
	return group
}

// dsElement is a one-line helper for a typed element inside a macro item.
func dsElement(tg Tag, value string) *DataElement {
	return NewDataElement(tg, VRDS, value)
}

func TestFunctionalGroupsForResolution(t *testing.T) {
	var nilFD *FileDataset
	if g, err := nilFD.functionalGroupsFor(0); err != nil || g.top != nil || g.shared != nil || g.perFrame != nil {
		t.Fatalf("nil dataset: g=%+v err=%v", g, err)
	}

	fd := NewDataset()
	empty := &FileDataset{Dataset: fd}
	g, err := empty.functionalGroupsFor(0)
	if err != nil || g.top != fd || g.shared != nil || g.perFrame != nil {
		t.Fatalf("bare dataset: g=%+v err=%v", g, err)
	}

	shared := NewDataset()
	p0, p1 := NewDataset(), NewDataset()
	setSQ(t, fd, tag.SharedFunctionalGroupsSequence, shared)
	setSQ(t, fd, tag.PerFrameFunctionalGroupsSequence, p0, p1)

	g, err = empty.functionalGroupsFor(1)
	if err != nil {
		t.Fatal(err)
	}
	if g.shared != shared || g.perFrame != p1 {
		t.Fatalf("frame 1: shared=%p perFrame=%p, want %p/%p", g.shared, g.perFrame, shared, p1)
	}
	g, err = empty.functionalGroupsFor(-1)
	if err != nil {
		t.Fatal(err)
	}
	if g.shared != shared || g.perFrame != nil {
		t.Fatal("negative frame index must not select a per-frame item")
	}
	if _, err := empty.functionalGroupsFor(2); err == nil ||
		!strings.Contains(err.Error(), "no item in Per-Frame Functional Groups Sequence") {
		t.Fatalf("out of range: %v", err)
	}
}

func TestFunctionalGroupsSourcesPrecedence(t *testing.T) {
	fd := enhancedDataset(t)
	fd.Set(dsElement(tag.RescaleSlope, "100"))
	fd.Set(dsElement(tag.RescaleIntercept, "900"))

	sharedMacro := NewDataset()
	sharedMacro.Set(dsElement(tag.RescaleSlope, "2"))
	sharedMacro.Set(dsElement(tag.RescaleIntercept, "10"))
	shared := sharedGroup(t, fd)
	setSQ(t, shared, tag.PixelValueTransformationSequence, sharedMacro)

	// The per-frame macro carries only half the transformation: the missing
	// half must still come from the shared item, not from the top level.
	perFrameMacro := NewDataset()
	perFrameMacro.Set(dsElement(tag.RescaleSlope, "3"))
	perFrame := macro(t, tag.PixelValueTransformationSequence, perFrameMacro)
	setSQ(t, fd, tag.PerFrameFunctionalGroupsSequence, perFrame, NewDataset())

	g, err := fd.functionalGroupsFor(0)
	if err != nil {
		t.Fatal(err)
	}
	slope, ok := g.float(tag.PixelValueTransformationSequence, tag.RescaleSlope)
	if !ok || slope != 3 {
		t.Fatalf("slope = %v, %v; want 3 from the per-frame macro", slope, ok)
	}
	intercept, ok := g.float(tag.PixelValueTransformationSequence, tag.RescaleIntercept)
	if !ok || intercept != 10 {
		t.Fatalf("intercept = %v, %v; want 10 from the shared macro", intercept, ok)
	}

	g, err = fd.functionalGroupsFor(-1)
	if err != nil {
		t.Fatal(err)
	}
	slope, _ = g.float(tag.PixelValueTransformationSequence, tag.RescaleSlope)
	if slope != 2 {
		t.Fatalf("shared-only slope = %v, want 2", slope)
	}
}

func TestEnhancedCTFixtureUsesSharedFunctionalGroups(t *testing.T) {
	ds, err := ReadFile("testdata/dcm/eCT_Supplemental.dcm", nil)
	if err != nil {
		t.Fatal(err)
	}
	// The whole point of #96: none of these are at the top level.
	for _, tg := range []Tag{
		tag.RescaleSlope, tag.RescaleIntercept,
		tag.WindowCenter, tag.WindowWidth, tag.PixelSpacing, tag.SliceThickness,
	} {
		if _, ok := ds.GetFloat(tg); ok {
			t.Fatalf("(%04X,%04X) unexpectedly present at the top level", tg.Group(), tg.Element())
		}
	}

	g, err := ds.functionalGroupsFor(0)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := g.float(tag.PixelValueTransformationSequence, tag.RescaleSlope); !ok || v != 1 {
		t.Fatalf("RescaleSlope = %v, %v; want 1", v, ok)
	}
	if v, ok := g.float(tag.PixelValueTransformationSequence, tag.RescaleIntercept); !ok || v != -1024 {
		t.Fatalf("RescaleIntercept = %v, %v; want -1024", v, ok)
	}
	if v, ok := g.floats(tag.FrameVOILUTSequence, tag.WindowCenter); !ok || len(v) != 1 || v[0] != 49 {
		t.Fatalf("WindowCenter = %v, %v; want [49]", v, ok)
	}
	if v, ok := g.floats(tag.FrameVOILUTSequence, tag.WindowWidth); !ok || len(v) != 1 || v[0] != 102 {
		t.Fatalf("WindowWidth = %v, %v; want [102]", v, ok)
	}

	out, err := ds.ApplyModalityLUTForFrame([]float64{0, 100}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != -1024 || out[1] != -924 {
		t.Fatalf("ApplyModalityLUTForFrame = %v, want [-1024 -924]", out)
	}
	if _, err := ds.DisplayFrame(0); err != nil {
		t.Fatalf("DisplayFrame: %v", err)
	}
}

func TestPixelMeasuresFromEnhancedFile(t *testing.T) {
	ds, err := ReadFile("testdata/dcm/eCT_Supplemental.dcm", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, frame := range []int{-1, 0, 1} {
		m, err := ds.PixelMeasures(frame)
		if err != nil {
			t.Fatalf("frame %d: %v", frame, err)
		}
		if !m.HasPixelSpacing || m.RowSpacing != 0.388672 || m.ColumnSpacing != 0.388672 {
			t.Fatalf("frame %d spacing = %+v", frame, m)
		}
		if !m.HasSliceThickness || m.SliceThickness != 10 {
			t.Fatalf("frame %d thickness = %+v", frame, m)
		}
		if m.HasSpacingBetweenSlices {
			t.Fatalf("frame %d unexpectedly has SpacingBetweenSlices: %+v", frame, m)
		}
	}
	if _, err := ds.PixelMeasures(2); err == nil {
		t.Fatal("frame 2 has no per-frame item, want an error")
	}
}

func TestPixelMeasuresPrecedenceAndFallback(t *testing.T) {
	fd := enhancedDataset(t)
	fd.Set(NewDataElement(tag.PixelSpacing, VRDS, NewMultiValue([]string{"1", "1"})))
	fd.Set(dsElement(tag.SliceThickness, "1"))

	sharedMacro := NewDataset()
	sharedMacro.Set(NewDataElement(tag.PixelSpacing, VRDS, NewMultiValue([]string{"2", "3"})))
	sharedMacro.Set(dsElement(tag.SliceThickness, "4"))
	sharedMacro.Set(dsElement(tag.SpacingBetweenSlices, "5"))
	shared := sharedGroup(t, fd)
	setSQ(t, shared, tag.PixelMeasuresSequence, sharedMacro)

	frame0Macro := NewDataset()
	frame0Macro.Set(NewDataElement(tag.PixelSpacing, VRDS, NewMultiValue([]string{"6", "7"})))
	setSQ(t, fd, tag.PerFrameFunctionalGroupsSequence,
		macro(t, tag.PixelMeasuresSequence, frame0Macro), NewDataset())

	m, err := fd.PixelMeasures(0)
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasPixelSpacing || m.RowSpacing != 6 || m.ColumnSpacing != 7 {
		t.Fatalf("frame 0 spacing = %+v, want 6/7", m)
	}
	if !m.HasSliceThickness || m.SliceThickness != 4 || !m.HasSpacingBetweenSlices || m.SpacingBetweenSlices != 5 {
		t.Fatalf("frame 0 must inherit shared measures: %+v", m)
	}

	for _, frame := range []int{-1, 1} {
		m, err = fd.PixelMeasures(frame)
		if err != nil {
			t.Fatal(err)
		}
		if !m.HasPixelSpacing || m.RowSpacing != 2 || m.ColumnSpacing != 3 {
			t.Fatalf("frame %d spacing = %+v, want 2/3", frame, m)
		}
	}

	fd.Delete(tag.SharedFunctionalGroupsSequence)
	fd.Delete(tag.PerFrameFunctionalGroupsSequence)
	m, err = fd.PixelMeasures(0)
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasPixelSpacing || m.RowSpacing != 1 || m.ColumnSpacing != 1 || m.SliceThickness != 1 {
		t.Fatalf("top-level fallback = %+v", m)
	}
}

func TestFrameCountFromPerFrameFunctionalGroups(t *testing.T) {
	fd := enhancedDataset(t)
	fd.Delete(tag.NumberOfFrames)
	setSQ(t, fd, tag.PerFrameFunctionalGroupsSequence, NewDataset(), NewDataset(), NewDataset())
	desc, err := pixels.DescriptorFromFile(fd)
	if err != nil {
		t.Fatal(err)
	}
	if desc.NumberOfFrames != 3 {
		t.Fatalf("NumberOfFrames = %d, want 3 from the functional groups", desc.NumberOfFrames)
	}

	// An explicit NumberOfFrames wins; the standard requires the two agree but
	// we do not silently second-guess a value the file states outright.
	fd.Set(NewDataElement(tag.NumberOfFrames, VRIS, 2))
	desc, err = pixels.DescriptorFromFile(fd)
	if err != nil {
		t.Fatal(err)
	}
	if desc.NumberOfFrames != 2 {
		t.Fatalf("NumberOfFrames = %d, want 2 from the element", desc.NumberOfFrames)
	}

	classic := enhancedDataset(t)
	classic.Delete(tag.NumberOfFrames)
	desc, err = pixels.DescriptorFromFile(classic)
	if err != nil {
		t.Fatal(err)
	}
	if desc.NumberOfFrames != 1 {
		t.Fatalf("classic fallback = %d, want 1", desc.NumberOfFrames)
	}
}

func TestApplyModalityLUTForFramePrecedence(t *testing.T) {
	fd := enhancedDataset(t)
	fd.Set(dsElement(tag.RescaleSlope, "100"))
	fd.Set(dsElement(tag.RescaleIntercept, "900"))

	sharedMacro := NewDataset()
	sharedMacro.Set(dsElement(tag.RescaleSlope, "2"))
	sharedMacro.Set(dsElement(tag.RescaleIntercept, "10"))
	shared := sharedGroup(t, fd)
	setSQ(t, shared, tag.PixelValueTransformationSequence, sharedMacro)

	frame0Macro := NewDataset()
	frame0Macro.Set(dsElement(tag.RescaleSlope, "3"))
	frame0Macro.Set(dsElement(tag.RescaleIntercept, "1"))
	setSQ(t, fd, tag.PerFrameFunctionalGroupsSequence,
		macro(t, tag.PixelValueTransformationSequence, frame0Macro), NewDataset())

	in := []float64{0, 10}
	assert := func(label string, got []float64, err error, want []float64) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
		if in[0] != 0 || in[1] != 10 {
			t.Fatalf("%s mutated its input: %v", label, in)
		}
	}
	out, err := fd.ApplyModalityLUTForFrame(in, 0)
	assert("frame 0", out, err, []float64{1, 31})
	out, err = fd.ApplyModalityLUTForFrame(in, 1)
	assert("frame 1 (shared)", out, err, []float64{10, 30})
	out, err = fd.ApplyModalityLUT(in)
	assert("no frame index (shared)", out, err, []float64{10, 30})

	if _, err := fd.ApplyModalityLUTForFrame(in, -1); err == nil {
		t.Fatal("negative frame index must be rejected")
	}
	if _, err := fd.ApplyModalityLUTForFrame(in, 2); err == nil {
		t.Fatal("frame 2 has no per-frame item, want an error")
	}
}

func TestApplyVOILUTForFrameSelectsFrameLUT(t *testing.T) {
	fd := enhancedDataset(t)
	sharedMacro := NewDataset()
	setSQ(t, sharedMacro, tag.VOILUTSequence, lutItem([]int{4, 0, 16}, []byte{9, 0, 9, 0, 9, 0, 9, 0}))
	shared := sharedGroup(t, fd)
	setSQ(t, shared, tag.FrameVOILUTSequence, sharedMacro)

	frame0Macro := NewDataset()
	setSQ(t, frame0Macro, tag.VOILUTSequence, lutItem([]int{4, 0, 16}, []byte{10, 0, 20, 0, 30, 0, 40, 0}))
	frame1Macro := NewDataset()
	setSQ(t, frame1Macro, tag.VOILUTSequence, lutItem([]int{4, 0, 16}, []byte{40, 0, 30, 0, 20, 0, 10, 0}))
	setSQ(t, fd, tag.PerFrameFunctionalGroupsSequence,
		macro(t, tag.FrameVOILUTSequence, frame0Macro),
		macro(t, tag.FrameVOILUTSequence, frame1Macro))

	in := []float64{0, 1, 2, 3}
	want := func(a, b, c, d float64) []float64 { return []float64{a, b, c, d} }
	check := func(label string, frame int, expected []float64) {
		t.Helper()
		out, err := fd.ApplyVOILUTForFrame(in, frame)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		for i := range expected {
			if out[i] != expected[i] {
				t.Fatalf("%s = %v, want %v", label, out, expected)
			}
		}
	}
	check("frame 0", 0, want(10, 20, 30, 40))
	check("frame 1", 1, want(40, 30, 20, 10))
	out, err := fd.ApplyVOILUT(in, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range out {
		if out[i] != 9 {
			t.Fatalf("no frame index = %v, want the shared LUT", out)
		}
	}

	// The DisplayOptions are forwarded, not dropped: with a shared window and
	// PreferLUT off, windowing wins over the shared LUT.
	sharedMacro.Set(dsElement(tag.WindowCenter, "1.5"))
	sharedMacro.Set(dsElement(tag.WindowWidth, "3"))
	windowed, err := fd.ApplyVOILUTForFrame(in, 1, pixels.WithPreferVOILUT(false))
	if err != nil {
		t.Fatal(err)
	}
	if windowed[0] == 9 && windowed[1] == 9 && windowed[2] == 9 && windowed[3] == 9 {
		t.Fatal("PreferLUT=false was ignored, the shared LUT still won")
	}
	if windowed[0] != 0 || windowed[3] != 65535 {
		t.Fatalf("windowed frame 1 = %v, want [0 ... 65535]", windowed)
	}
}

func TestDisplayFrameResolvesFunctionalGroupsPerFrame(t *testing.T) {
	fd := enhancedDataset(t)
	shared := sharedGroup(t, fd)
	_ = shared

	frame0Macro := NewDataset()
	setSQ(t, frame0Macro, tag.VOILUTSequence, lutItem([]int{4, 0, 16}, []byte{10, 0, 20, 0, 30, 0, 40, 0}))
	frame1Macro := NewDataset()
	setSQ(t, frame1Macro, tag.VOILUTSequence, lutItem([]int{4, 0, 16}, []byte{40, 0, 30, 0, 20, 0, 10, 0}))
	setSQ(t, fd, tag.PerFrameFunctionalGroupsSequence,
		macro(t, tag.FrameVOILUTSequence, frame0Macro),
		macro(t, tag.FrameVOILUTSequence, frame1Macro))

	first, err := fd.DisplayFrame(0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fd.DisplayFrame(1)
	if err != nil {
		t.Fatal(err)
	}
	// Both frames hold the same samples {0,1,2,3}. The only thing that can make
	// these differ is the per-frame VOI LUT, so equality is the regression.
	if bytes.Equal(first, second) {
		t.Fatalf("both frames rendered identically (%v): frame 0 reused for every frame", first)
	}
	want0 := []byte{10, 20, 30, 40}
	want1 := []byte{40, 30, 20, 10}
	if !bytes.Equal(first, want0) {
		t.Fatalf("frame 0 = %v, want %v", first, want0)
	}
	if !bytes.Equal(second, want1) {
		t.Fatalf("frame 1 = %v, want %v", second, want1)
	}
}

func TestDisplayFrameEnhancedCTDiffersWithoutFunctionalGroups(t *testing.T) {
	withFG, err := ReadFile("testdata/dcm/eCT_Supplemental.dcm", nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err := withFG.DisplayFrame(0)
	if err != nil {
		t.Fatalf("with functional groups: %v", err)
	}
	withoutFG, err := ReadFile("testdata/dcm/eCT_Supplemental.dcm", nil)
	if err != nil {
		t.Fatal(err)
	}
	withoutFG.Delete(tag.SharedFunctionalGroupsSequence)
	withoutFG.Delete(tag.PerFrameFunctionalGroupsSequence)
	b, err := withoutFG.DisplayFrame(0)
	if err != nil {
		t.Fatalf("without functional groups: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("functional groups had no effect on DisplayFrame")
	}
}

func TestApplyPresentationLUTShapeForFrame(t *testing.T) {
	fd := enhancedDataset(t)
	shared := sharedGroup(t, fd)
	shared.Set(NewDataElement(tag.PresentationLUTShape, VRCS, "IDENTITY"))

	inverse := NewDataset()
	inverse.Set(NewDataElement(tag.PresentationLUTShape, VRCS, "INVERSE"))
	setSQ(t, fd, tag.PerFrameFunctionalGroupsSequence,
		macro(t, tag.FrameVOILUTSequence, inverse), NewDataset())

	in := []float64{0, 10, 30}
	out, err := fd.ApplyPresentationLUTShapeForFrame(in, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 30 || out[1] != 20 || out[2] != 0 {
		t.Fatalf("frame 0 = %v, want [30 20 0]", out)
	}
	out, err = fd.ApplyPresentationLUTShapeForFrame(in, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 0 || out[1] != 10 || out[2] != 30 {
		t.Fatalf("frame 1 = %v, want the shared IDENTITY", out)
	}
	out, err = fd.ApplyPresentationLUTShape(in)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 0 || out[2] != 30 {
		t.Fatalf("no frame index = %v, want the shared IDENTITY", out)
	}
	if _, err := fd.ApplyPresentationLUTShapeForFrame(in, -1); err == nil {
		t.Fatal("negative frame index must be rejected")
	}
}

func TestNeedsDisplayLUTSeesFunctionalGroups(t *testing.T) {
	fd := enhancedDataset(t)
	fd.Set(NewDataElement(tag.BitsAllocated, VRUS, 8))
	fd.Set(NewDataElement(tag.BitsStored, VRUS, 8))
	fd.Set(NewDataElement(tag.SamplesPerPixel, VRUS, 3))
	fd.Set(NewDataElement(tag.PhotometricInterpretation, VRCS, "RGB"))

	g, err := fd.functionalGroupsFor(0)
	if err != nil {
		t.Fatal(err)
	}
	if fd.needsDisplayLUT(g) {
		t.Fatal("a plain 8-bit RGB frame must keep the display-ready short-circuit")
	}

	sharedMacro := NewDataset()
	sharedMacro.Set(dsElement(tag.WindowCenter, "128"))
	sharedMacro.Set(dsElement(tag.WindowWidth, "256"))
	shared := sharedGroup(t, fd)
	setSQ(t, shared, tag.FrameVOILUTSequence, sharedMacro)
	g, err = fd.functionalGroupsFor(0)
	if err != nil {
		t.Fatal(err)
	}
	if !fd.needsDisplayLUT(g) {
		t.Fatal("a shared window must disable the display-ready short-circuit")
	}

	fd.Delete(tag.SharedFunctionalGroupsSequence)
	fd.Set(dsElement(tag.RescaleSlope, "2"))
	g, err = fd.functionalGroupsFor(0)
	if err != nil {
		t.Fatal(err)
	}
	if !fd.needsDisplayLUT(g) {
		t.Fatal("a top-level rescale must still disable the short-circuit")
	}
}

func TestApplyColorLUTForFrameSelectsEnhancedPalette(t *testing.T) {
	fd := newLUTDataset()
	fd.Set(NewDataElement(tag.Rows, VRUS, 1))
	fd.Set(NewDataElement(tag.Columns, VRUS, 2))
	fd.Set(NewDataElement(tag.BitsAllocated, VRUS, 8))
	fd.Set(NewDataElement(tag.BitsStored, VRUS, 8))
	fd.Set(NewDataElement(tag.NumberOfFrames, VRIS, 2))
	fd.Set(NewDataElement(tag.PhotometricInterpretation, VRCS, "PALETTE COLOR"))
	fd.Set(NewDataElement(tag.PixelData, VROW, []byte{0, 1, 0, 1}))

	palette := func(uid string, red, green, blue []byte) *Dataset {
		item := NewDataset()
		item.Set(NewDataElement(tag.PaletteColorLookupTableUID, VRUI, uid))
		for _, tg := range []Tag{
			tag.RedPaletteColorLookupTableDescriptor,
			tag.GreenPaletteColorLookupTableDescriptor,
			tag.BluePaletteColorLookupTableDescriptor,
		} {
			item.Set(NewDataElement(tg, VRUS, []int{2, 0, 8}))
		}
		item.Set(NewDataElement(tag.RedPaletteColorLookupTableData, VROW, red))
		item.Set(NewDataElement(tag.GreenPaletteColorLookupTableData, VROW, green))
		item.Set(NewDataElement(tag.BluePaletteColorLookupTableData, VROW, blue))
		return item
	}
	setSQ(t, fd, tag.EnhancedPaletteColorLookupTableSequence,
		palette("1.2.3", []byte{10, 20}, []byte{30, 40}, []byte{50, 60}),
		palette("1.2.4", []byte{60, 50}, []byte{40, 30}, []byte{20, 10}),
	)

	frameUID := func(uid string) *Dataset {
		item := NewDataset()
		item.Set(NewDataElement(tag.PaletteColorLookupTableUID, VRUI, uid))
		return item
	}
	setSQ(t, fd, tag.PerFrameFunctionalGroupsSequence, frameUID("1.2.3"), frameUID("1.2.4"))

	in := []float64{0, 1}
	check := func(frame int, want [][3]uint16) {
		t.Helper()
		out, err := ApplyColorLUTForFrame(in, fd, frame, "")
		if err != nil {
			t.Fatalf("frame %d: %v", frame, err)
		}
		if out.Channels != 3 {
			t.Fatalf("frame %d channels = %d", frame, out.Channels)
		}
		for i := range want {
			for c := 0; c < 3; c++ {
				if got := out.At(i, c); got != want[i][c] {
					t.Fatalf("frame %d pixel %d channel %d = %d, want %d", frame, i, c, got, want[i][c])
				}
			}
		}
	}
	check(0, [][3]uint16{{10, 30, 50}, {20, 40, 60}})
	check(1, [][3]uint16{{60, 40, 20}, {50, 30, 10}})

	// An explicit well-known palette keeps overriding the dataset, and the
	// classic ApplyColorLUT entry point still resolves the shared/top level.
	if _, err := ApplyColorLUT(in, fd, "HOT_IRON"); err != nil {
		t.Fatalf("explicit palette: %v", err)
	}
	if _, err := ApplyColorLUTForFrame(in, fd, 99, ""); err == nil {
		t.Fatal("an out-of-range frame must be rejected")
	}

	fd.Delete(tag.PerFrameFunctionalGroupsSequence)
	fd.Set(NewDataElement(tag.PaletteColorLookupTableUID, VRUI, "1.2.4"))
	out, err := ApplyColorLUTForFrame(in, fd, 0, "")
	if err != nil {
		t.Fatalf("top-level UID: %v", err)
	}
	if out.At(0, 0) != 60 {
		t.Fatalf("top-level UID selected the wrong palette: %d", out.At(0, 0))
	}

	fd.Set(NewDataElement(tag.PaletteColorLookupTableUID, VRUI, "9.9.9"))
	if _, err := ApplyColorLUTForFrame(in, fd, 0, ""); err == nil ||
		!strings.Contains(err.Error(), "no Enhanced Palette Color Lookup Table matches UID") {
		t.Fatalf("unmatched UID: %v", err)
	}
	fd.Delete(tag.PaletteColorLookupTableUID)
	if _, err := ApplyColorLUTForFrame(in, fd, 0, ""); err == nil ||
		!strings.Contains(err.Error(), "does not name a Palette Color Lookup Table UID") {
		t.Fatalf("missing UID: %v", err)
	}

	// A table carried inline in the per-frame item is the most specific source:
	// it wins over both the UID lookup and the classic top-level module. There
	// is no top-level palette here at all, so this path is the only one that
	// can answer.
	inline := palette("1.2.3", []byte{1, 2}, []byte{3, 4}, []byte{5, 6})
	setSQ(t, fd, tag.PerFrameFunctionalGroupsSequence, inline, NewDataset())
	out, err = ApplyColorLUTForFrame(in, fd, 0, "")
	if err != nil {
		t.Fatalf("inline palette: %v", err)
	}
	if out.At(0, 0) != 1 || out.At(1, 2) != 6 {
		t.Fatalf("inline palette RGB = %v", out.Samples)
	}
}
