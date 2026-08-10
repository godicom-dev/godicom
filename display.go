package godicom

import (
	"fmt"
	"strings"

	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
)

// DisplayFrame returns one frame as 8-bit color-by-pixel bytes suitable for JPEG/PNG encoders.
// It applies Modality LUT / rescale, VOI LUT / windowing, and Presentation LUT Shape when present.
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

	// 8-bit RGB (or processed color) without modality/VOI: decoded bytes are display-ready.
	if desc.BitsAllocated == 8 && desc.SamplesPerPixel >= 3 && !fd.needsDisplayLUT() {
		return fd.PixelBytes(pixels.WithRaw(false), pixels.WithFrameIndex(frameIndex))
	}

	samples, err := fd.PixelSamples(pixels.WithRaw(true), pixels.WithFrameIndex(frameIndex))
	if err != nil {
		return nil, err
	}
	mod, err := fd.ApplyModalityLUT(samples)
	if err != nil {
		return nil, err
	}
	dopts := applyDisplayOptions(opts)
	view, err := fd.ApplyVOILUT(mod, dopts.WindowIndex, dopts.PreferLUT)
	if err != nil {
		return nil, err
	}
	view, err = fd.ApplyPresentationLUTShape(view)
	if err != nil {
		return nil, err
	}
	pi, _ := fd.GetString(tag.PhotometricInterpretation)
	if strings.TrimSpace(pi) == "MONOCHROME1" {
		view = pixels.InvertValues(view)
	}
	return pixels.PackDisplayU8(view), nil
}

func (fd *FileDataset) needsDisplayLUT() bool {
	if _, ok := fd.GetFloat(tag.RescaleSlope); ok {
		return true
	}
	if _, ok := fd.GetFloat(tag.RescaleIntercept); ok {
		return true
	}
	if seq, ok := fd.GetSequence(tag.ModalityLUTSequence); ok && seq != nil && seq.Len() > 0 {
		return true
	}
	if seq, ok := fd.GetSequence(tag.VOILUTSequence); ok && seq != nil && seq.Len() > 0 {
		return true
	}
	if c, ok := fd.GetFloats(tag.WindowCenter); ok && len(c) > 0 {
		return true
	}
	if w, ok := fd.GetFloats(tag.WindowWidth); ok && len(w) > 0 {
		return true
	}
	shape, ok := fd.GetString(tag.PresentationLUTShape)
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
