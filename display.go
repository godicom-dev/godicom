package godicom

import (
	"fmt"
	"strings"

	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
)

// DisplayFrame returns one frame as 8-bit color-by-pixel bytes suitable for JPEG/PNG encoders.
// It applies Modality LUT / rescale, VOI LUT / windowing, and Presentation LUT Shape when present.
// A PALETTE COLOR image is expanded through its color lookup table instead of that grayscale pipeline.
//
// An enhanced multi-frame instance keeps those parameters in its functional
// groups, so the rescale, window and palette are resolved for frameIndex rather
// than once for the whole dataset. The legacy ApplyModalityLUT / ApplyVOILUT /
// ApplyColorLUT entry points deliberately keep their dataset-wide behaviour.
func (fd *FileDataset) DisplayFrame(frameIndex int, opts ...pixels.DisplayOption) ([]byte, error) {
	if fd == nil {
		return nil, fmt.Errorf("godicom: nil dataset")
	}
	if frameIndex < 0 {
		return nil, fmt.Errorf("godicom: frame index %d out of range", frameIndex)
	}
	desc, err := pixels.DescriptorFromFile(fd)
	if err != nil {
		return nil, err
	}
	if desc.NumberOfFrames > 0 && frameIndex >= desc.NumberOfFrames {
		return nil, fmt.Errorf("godicom: frame index %d out of range (have %d)", frameIndex, desc.NumberOfFrames)
	}
	g, err := fd.functionalGroupsFor(frameIndex)
	if err != nil {
		return nil, err
	}

	// 8-bit RGB (or processed color) without modality/VOI: decoded bytes are display-ready.
	if desc.BitsAllocated == 8 && desc.SamplesPerPixel >= 3 && !fd.needsDisplayLUT(g) {
		return fd.PixelBytes(pixels.WithRaw(false), pixels.WithFrameIndex(frameIndex))
	}

	pi, _ := fd.GetString(tag.PhotometricInterpretation)
	if strings.EqualFold(strings.TrimSpace(pi), "PALETTE COLOR") {
		samples, err := fd.PixelSamples(pixels.WithRaw(true), pixels.WithFrameIndex(frameIndex))
		if err != nil {
			return nil, err
		}
		pal, err := ApplyColorLUTForFrame(samples, fd, frameIndex, "")
		if err != nil {
			return nil, err
		}
		return pal.PackDisplayU8(), nil
	}

	samples, err := fd.PixelSamples(pixels.WithRaw(true), pixels.WithFrameIndex(frameIndex))
	if err != nil {
		return nil, err
	}
	mod, err := fd.applyModalityLUT(g, samples)
	if err != nil {
		return nil, err
	}
	dopts := applyDisplayOptions(opts)
	view, err := fd.applyVOILUT(g, mod, dopts.WindowIndex, dopts.PreferLUT)
	if err != nil {
		return nil, err
	}
	view, err = fd.applyPresentationLUTShape(g, view)
	if err != nil {
		return nil, err
	}
	pi, _ = fd.GetString(tag.PhotometricInterpretation)
	if strings.TrimSpace(pi) == "MONOCHROME1" {
		view = pixels.InvertValues(view)
	}
	return pixels.PackDisplayU8(view), nil
}

// needsDisplayLUT reports whether the short-circuit for display-ready 8-bit
// color is safe. The attributes may be in a functional group macro, so the
// same resolvers the display pipeline uses decide this, not just the top level.
func (fd *FileDataset) needsDisplayLUT(g *functionalGroups) bool {
	for _, probe := range []struct{ macro, elem tag.Tag }{
		{tag.PixelValueTransformationSequence, tag.RescaleSlope},
		{tag.PixelValueTransformationSequence, tag.RescaleIntercept},
		{tag.FrameVOILUTSequence, tag.WindowCenter},
		{tag.FrameVOILUTSequence, tag.WindowWidth},
	} {
		if _, ok := g.float(probe.macro, probe.elem); ok {
			return true
		}
		if _, ok := g.floats(probe.macro, probe.elem); ok {
			return true
		}
	}
	for _, item := range g.sources(0) {
		for _, seqTag := range []tag.Tag{tag.ModalityLUTSequence, tag.VOILUTSequence} {
			if seq, ok := item.GetSequence(seqTag); ok && seq != nil && seq.Len() > 0 {
				return true
			}
		}
	}
	for _, item := range g.macros(tag.PixelValueTransformationSequence) {
		if seq, ok := item.GetSequence(tag.ModalityLUTSequence); ok && seq != nil && seq.Len() > 0 {
			return true
		}
	}
	for _, item := range g.macros(tag.FrameVOILUTSequence) {
		if seq, ok := item.GetSequence(tag.VOILUTSequence); ok && seq != nil && seq.Len() > 0 {
			return true
		}
	}
	shape, ok := g.str(tag.FrameVOILUTSequence, tag.PresentationLUTShape)
	return ok && strings.TrimSpace(shape) != "" && strings.ToUpper(strings.TrimSpace(shape)) != "IDENTITY"
}

func applyDisplayOptions(opts []pixels.DisplayOption) pixels.DisplayOptions {
	out := pixels.DisplayOptions{PreferLUT: true}
	for _, fn := range opts {
		if fn != nil {
			fn(&out)
		}
	}
	return out
}
