package godicom

import (
	"fmt"
	"strings"

	"github.com/godicom-dev/godicom/tag"
)

// An enhanced multi-frame instance (PS3.3 C.7.6.16) keeps, in functional group
// macros, the attributes a single-frame instance carries at the top level: one
// shared item for every frame in Shared Functional Groups Sequence (5200,9229),
// and one item per frame in Per-Frame Functional Groups Sequence (5200,9230).
// When both carry the same attribute the per-frame item wins; a top-level value
// applies only when neither has one.
//
// pydicom reads neither sequence -- its pixel handlers look at the top level --
// so there is nothing to mirror here. The precedence above is what PS3.3
// C.7.6.16 states, and the fallback to the top level is what keeps classic
// single-frame and classic multi-frame files working through the same code.
type functionalGroups struct {
	top      *Dataset // the dataset itself, nil when the FileDataset is empty
	shared   *Dataset // item 0 of (5200,9229), nil when absent
	perFrame *Dataset // item frameIndex of (5200,9230), nil when absent
}

// functionalGroupsFor resolves the functional group items that apply to
// frameIndex. A negative frameIndex means "no particular frame": only the shared
// item and the top level are consulted, which is all a caller without a frame
// index can honestly use.
//
// A Per-Frame Functional Groups Sequence shorter than the frame count is a
// broken instance, not a reason to fall back on the shared item: silently
// reusing shared values would put a wrong rescale or window into a thumbnail
// and report nothing.
func (fd *FileDataset) functionalGroupsFor(frameIndex int) (*functionalGroups, error) {
	g := &functionalGroups{}
	if fd == nil || fd.Dataset == nil {
		return g, nil
	}
	g.top = fd.Dataset
	if seq, ok := fd.GetSequence(tag.SharedFunctionalGroupsSequence); ok && seq != nil && seq.Len() > 0 {
		g.shared = seq.Get(0)
	}
	if frameIndex < 0 {
		return g, nil
	}
	seq, ok := fd.GetSequence(tag.PerFrameFunctionalGroupsSequence)
	if !ok || seq == nil || seq.Len() == 0 {
		return g, nil
	}
	if frameIndex >= seq.Len() {
		return g, fmt.Errorf(
			"godicom: frame %d has no item in Per-Frame Functional Groups Sequence (%d items)",
			frameIndex, seq.Len(),
		)
	}
	g.perFrame = seq.Get(frameIndex)
	return g, nil
}

// FunctionalGroupFrameCount reports the number of items in the Per-Frame
// Functional Groups Sequence. It is the fallback for an enhanced instance that
// omits NumberOfFrames (0028,0008); see pixels.FrameCountSource.
func (fd *FileDataset) FunctionalGroupFrameCount() int {
	if fd == nil || fd.Dataset == nil {
		return 0
	}
	if seq, ok := fd.GetSequence(tag.PerFrameFunctionalGroupsSequence); ok && seq != nil {
		return seq.Len()
	}
	return 0
}

// sources returns the datasets to consult for one attribute, most specific
// first. For each functional group (per-frame before shared) it adds the macro
// item named by macroTag and then the group item itself; the dataset top level
// comes last. A zero macroTag means "no macro, top level only".
//
// Looking through every source rather than stopping at the first macro that
// happens to exist matters for real files: a per-frame Frame VOI LUT item may
// carry only Window Center while the shared item carries Window Width, and
// dropping the shared half would leave the frame with no window at all.
func (g *functionalGroups) sources(macroTag tag.Tag) []*Dataset {
	out := make([]*Dataset, 0, 5)
	for _, group := range []*Dataset{g.perFrame, g.shared} {
		if group == nil {
			continue
		}
		if macroTag != 0 {
			if seq, ok := group.GetSequence(macroTag); ok && seq != nil && seq.Len() > 0 {
				if item := seq.Get(0); item != nil {
					out = append(out, item)
				}
			}
		}
		out = append(out, group)
	}
	if g.top != nil {
		out = append(out, g.top)
	}
	return out
}

// macros returns only the macro items named by macroTag, per-frame first. LUT
// sequences live inside these items, not in the group item itself.
func (g *functionalGroups) macros(macroTag tag.Tag) []*Dataset {
	out := make([]*Dataset, 0, 2)
	for _, group := range []*Dataset{g.perFrame, g.shared} {
		if group == nil {
			continue
		}
		if seq, ok := group.GetSequence(macroTag); ok && seq != nil && seq.Len() > 0 {
			if item := seq.Get(0); item != nil {
				out = append(out, item)
			}
		}
	}
	return out
}

// float returns elem as found in the first source that carries it.
func (g *functionalGroups) float(macroTag, elem tag.Tag) (float64, bool) {
	for _, d := range g.sources(macroTag) {
		if v, ok := d.GetFloat(elem); ok {
			return v, true
		}
	}
	return 0, false
}

// floats returns the multi-valued elem from the first source that carries it.
func (g *functionalGroups) floats(macroTag, elem tag.Tag) ([]float64, bool) {
	for _, d := range g.sources(macroTag) {
		if v, ok := d.GetFloats(elem); ok && len(v) > 0 {
			return v, true
		}
	}
	return nil, false
}

// str returns a string elem from the first source that carries it.
func (g *functionalGroups) str(macroTag, elem tag.Tag) (string, bool) {
	for _, d := range g.sources(macroTag) {
		if v, ok := d.GetString(elem); ok && strings.TrimSpace(v) != "" {
			return v, true
		}
	}
	return "", false
}

// paletteUID returns the (0028,1199) that names the palette the frame uses,
// from the per-frame item, the shared item or the top level.
func (g *functionalGroups) paletteUID() (string, bool) {
	return g.str(0, tag.PaletteColorLookupTableUID)
}

// enhancedPalette returns the Enhanced Palette Color Lookup Table Sequence,
// looked up in the frame's functional groups before the top level.
func (g *functionalGroups) enhancedPalette() (*Sequence, bool) {
	for _, d := range g.sources(0) {
		if seq, ok := d.GetSequence(tag.EnhancedPaletteColorLookupTableSequence); ok && seq != nil && seq.Len() > 0 {
			return seq, true
		}
	}
	return nil, false
}

// PixelMeasures holds the physical measures one frame declares, which an
// enhanced instance keeps in its Pixel Measures macro and a classic instance may
// keep at the top level.
type PixelMeasures struct {
	// (0028,0030): the first value is the row spacing, the second the column
	// spacing. mm.
	RowSpacing      float64
	ColumnSpacing   float64
	HasPixelSpacing bool

	SliceThickness    float64 // (0018,0050), mm
	HasSliceThickness bool

	SpacingBetweenSlices    float64 // (0018,0088), mm
	HasSpacingBetweenSlices bool
}

// PixelMeasures returns the physical measures that apply to one frame.
//
// It exists because the measures are per-frame in an enhanced instance, so they
// cannot live on pixels.Descriptor, which is resolved once per dataset. A
// negative frameIndex asks for the shared measures alone.
func (fd *FileDataset) PixelMeasures(frameIndex int) (PixelMeasures, error) {
	g, err := fd.functionalGroupsFor(frameIndex)
	if err != nil {
		return PixelMeasures{}, err
	}
	var m PixelMeasures
	if v, ok := g.floats(tag.PixelMeasuresSequence, tag.PixelSpacing); ok && len(v) >= 2 {
		m.RowSpacing, m.ColumnSpacing, m.HasPixelSpacing = v[0], v[1], true
	}
	if v, ok := g.float(tag.PixelMeasuresSequence, tag.SliceThickness); ok {
		m.SliceThickness, m.HasSliceThickness = v, true
	}
	if v, ok := g.float(tag.PixelMeasuresSequence, tag.SpacingBetweenSlices); ok {
		m.SpacingBetweenSlices, m.HasSpacingBetweenSlices = v, true
	}
	return m, nil
}
