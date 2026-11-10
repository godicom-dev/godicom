package godicom

import (
	"encoding/binary"
	"fmt"

	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
)

// PixelSamples returns decoded pixel samples as float64 (one per sample).
// Decoding options match PixelBytes; photometric YBR→RGB still applies when Raw=false.
func (fd *FileDataset) PixelSamples(opts ...pixels.DecodeOption) ([]float64, error) {
	raw, err := fd.PixelBytes(opts...)
	if err != nil {
		return nil, err
	}
	desc, err := pixels.DescriptorFromFile(fd)
	if err != nil {
		return nil, err
	}
	return pixels.UnpackSamples(raw, desc.BitsAllocated, desc.PixelRepresentation, fd.pixelLittleEndian())
}

// ApplyModalityLUT applies Modality LUT Sequence or Rescale Slope/Intercept.
// Mirrors pydicom.pixels.processing.apply_modality_lut.
//
// It has no frame index, so for an enhanced multi-frame instance it sees only
// the shared functional group and the top level. That is correct when the
// instance keeps the transformation in the shared item, which is the usual
// case, and it is what ApplyModalityLUTForFrame extends when it does not.
func (fd *FileDataset) ApplyModalityLUT(arr []float64) ([]float64, error) {
	return fd.applyModalityLUTForFrame(arr, -1)
}

// ApplyModalityLUTForFrame is ApplyModalityLUT for one frame of an enhanced
// multi-frame instance, whose per-frame item may carry its own transformation.
func (fd *FileDataset) ApplyModalityLUTForFrame(arr []float64, frameIndex int) ([]float64, error) {
	if frameIndex < 0 {
		return nil, fmt.Errorf("godicom: frame index %d out of range", frameIndex)
	}
	return fd.applyModalityLUTForFrame(arr, frameIndex)
}

func (fd *FileDataset) applyModalityLUTForFrame(arr []float64, frameIndex int) ([]float64, error) {
	g, err := fd.functionalGroupsFor(frameIndex)
	if err != nil {
		return nil, err
	}
	return fd.applyModalityLUT(g, arr)
}

func (fd *FileDataset) applyModalityLUT(g *functionalGroups, arr []float64) ([]float64, error) {
	p, err := fd.modalityParams(g)
	if err != nil {
		return nil, err
	}
	return pixels.ApplyModalityLUT(arr, p)
}

// ApplyVOILUT applies VOI LUT Sequence or Window Center/Width.
// preferLUT matches pydicom apply_voi_lut(prefer_lut=...).
//
// index selects among several windows or VOI LUTs, not among frames. For an
// enhanced multi-frame instance this method sees only the shared functional
// group and the top level; use ApplyVOILUTForFrame for the per-frame item.
func (fd *FileDataset) ApplyVOILUT(arr []float64, index int, preferLUT bool) ([]float64, error) {
	return fd.applyVOILUTForFrame(arr, -1, index, preferLUT)
}

// ApplyVOILUTForFrame is ApplyVOILUT for one frame of an enhanced multi-frame
// instance. The options select the window index and whether a VOI LUT wins over
// windowing, exactly as they do for DisplayFrame.
func (fd *FileDataset) ApplyVOILUTForFrame(arr []float64, frameIndex int, opts ...pixels.DisplayOption) ([]float64, error) {
	if frameIndex < 0 {
		return nil, fmt.Errorf("godicom: frame index %d out of range", frameIndex)
	}
	dopts := applyDisplayOptions(opts)
	return fd.applyVOILUTForFrame(arr, frameIndex, dopts.WindowIndex, dopts.PreferLUT)
}

func (fd *FileDataset) applyVOILUTForFrame(arr []float64, frameIndex, index int, preferLUT bool) ([]float64, error) {
	g, err := fd.functionalGroupsFor(frameIndex)
	if err != nil {
		return nil, err
	}
	return fd.applyVOILUT(g, arr, index, preferLUT)
}

func (fd *FileDataset) applyVOILUT(g *functionalGroups, arr []float64, index int, preferLUT bool) ([]float64, error) {
	p, err := fd.voiParams(g, index, preferLUT)
	if err != nil {
		return nil, err
	}
	return pixels.ApplyVOILUT(arr, p)
}

// ApplyPresentationLUTShape applies (2050,0020) Presentation LUT Shape when
// present. It has no frame index, so an enhanced instance is resolved through
// its shared functional group and top level only; ApplyPresentationLUTShapeForFrame
// is the per-frame variant.
func (fd *FileDataset) ApplyPresentationLUTShape(arr []float64) ([]float64, error) {
	g, err := fd.functionalGroupsFor(-1)
	if err != nil {
		return nil, err
	}
	return fd.applyPresentationLUTShape(g, arr)
}

// ApplyPresentationLUTShapeForFrame is ApplyPresentationLUTShape for one frame
// of an enhanced multi-frame instance.
func (fd *FileDataset) ApplyPresentationLUTShapeForFrame(arr []float64, frameIndex int) ([]float64, error) {
	if frameIndex < 0 {
		return nil, fmt.Errorf("godicom: frame index %d out of range", frameIndex)
	}
	g, err := fd.functionalGroupsFor(frameIndex)
	if err != nil {
		return nil, err
	}
	return fd.applyPresentationLUTShape(g, arr)
}

func (fd *FileDataset) applyPresentationLUTShape(g *functionalGroups, arr []float64) ([]float64, error) {
	shape, ok := g.str(tag.FrameVOILUTSequence, tag.PresentationLUTShape)
	if !ok || shape == "" {
		out := make([]float64, len(arr))
		copy(out, arr)
		return out, nil
	}
	return pixels.ApplyPresentationLUTShape(arr, shape)
}

func (fd *FileDataset) pixelLittleEndian() bool {
	ts, ok := fd.TransferSyntaxUID()
	if !ok || ts == "" {
		return true
	}
	return ts.IsLittleEndian()
}

func (fd *FileDataset) modalityParams(g *functionalGroups) (pixels.ModalityParams, error) {
	if lut, ok, err := readModalityLUT(g, fd.pixelLittleEndian()); err != nil {
		return pixels.ModalityParams{}, err
	} else if ok {
		return pixels.ModalityParams{LUT: &lut}, nil
	}
	slope, hasSlope := g.float(tag.PixelValueTransformationSequence, tag.RescaleSlope)
	intercept, hasIntercept := g.float(tag.PixelValueTransformationSequence, tag.RescaleIntercept)
	if hasSlope && hasIntercept {
		return pixels.ModalityParams{
			HasRescale:       true,
			RescaleSlope:     slope,
			RescaleIntercept: intercept,
		}, nil
	}
	return pixels.ModalityParams{}, nil
}

func (fd *FileDataset) voiParams(g *functionalGroups, index int, preferLUT bool) (pixels.VOIParams, error) {
	p := pixels.VOIParams{PreferLUT: preferLUT, Index: index}
	if lut, ok, err := readVOILUT(g, index, fd.pixelLittleEndian()); err != nil {
		return p, err
	} else if ok {
		p.LUT = &lut
	}
	centers, hasC := g.floats(tag.FrameVOILUTSequence, tag.WindowCenter)
	widths, hasW := g.floats(tag.FrameVOILUTSequence, tag.WindowWidth)
	if hasC && hasW && len(centers) > 0 && len(widths) > 0 {
		if index < 0 || index >= len(centers) || index >= len(widths) {
			return p, fmt.Errorf("godicom: VOI window index %d out of range", index)
		}
		cfg, err := fd.windowConfig(g, centers[index], widths[index])
		if err != nil {
			return p, err
		}
		p.Window = &cfg
	}
	return p, nil
}

func (fd *FileDataset) windowConfig(g *functionalGroups, center, width float64) (pixels.WindowConfig, error) {
	desc, err := pixels.DescriptorFromFile(fd)
	if err != nil {
		return pixels.WindowConfig{}, err
	}
	cfg := pixels.WindowConfig{
		Center:                    center,
		Width:                     width,
		PhotometricInterpretation: desc.PhotometricInterpretation,
		BitsStored:                desc.BitsStored,
		PixelRepresentation:       desc.PixelRepresentation,
	}
	if fn, ok := g.str(tag.FrameVOILUTSequence, tag.VOILUTFunction); ok {
		cfg.Function = fn
	}
	if lut, ok, err := readModalityLUT(g, fd.pixelLittleEndian()); err != nil {
		return cfg, err
	} else if ok {
		cfg.HasModalityLUT = true
		cfg.ModalityLUTOutputBits = lut.OutputBits
	} else {
		slope, hasSlope := g.float(tag.PixelValueTransformationSequence, tag.RescaleSlope)
		intercept, hasIntercept := g.float(tag.PixelValueTransformationSequence, tag.RescaleIntercept)
		if hasSlope && hasIntercept {
			cfg.HasRescale = true
			cfg.RescaleSlope = slope
			cfg.RescaleIntercept = intercept
		}
	}
	return cfg, nil
}

// readModalityLUT returns the Modality LUT that applies, looking inside the
// Pixel Value Transformation macro before the top level. PS3.3 keeps the
// Modality LUT Module at the top level for enhanced instances too, but writers
// that build the transformation inside the macro put it there.
func readModalityLUT(g *functionalGroups, littleEndian bool) (pixels.LUT, bool, error) {
	for _, item := range g.macros(tag.PixelValueTransformationSequence) {
		if lut, ok, err := readLUTFromItem(item, tag.ModalityLUTSequence, 0, littleEndian); err != nil || ok {
			return lut, ok, err
		}
	}
	if g.top != nil {
		return readLUTFromItem(g.top, tag.ModalityLUTSequence, 0, littleEndian)
	}
	return pixels.LUT{}, false, nil
}

// readVOILUT returns the VOI LUT at index, inside the Frame VOI LUT macro first
// and then at the top level.
func readVOILUT(g *functionalGroups, index int, littleEndian bool) (pixels.LUT, bool, error) {
	for _, item := range g.macros(tag.FrameVOILUTSequence) {
		if lut, ok, err := readLUTFromItem(item, tag.VOILUTSequence, index, littleEndian); err != nil || ok {
			return lut, ok, err
		}
	}
	if g.top != nil {
		return readLUTFromItem(g.top, tag.VOILUTSequence, index, littleEndian)
	}
	return pixels.LUT{}, false, nil
}

func (fd *FileDataset) readLUTFromSequence(seqTag tag.Tag, index int) (pixels.LUT, bool, error) {
	if fd == nil || fd.Dataset == nil {
		return pixels.LUT{}, false, nil
	}
	return readLUTFromItem(fd.Dataset, seqTag, index, fd.pixelLittleEndian())
}

func readLUTFromItem(h *Dataset, seqTag tag.Tag, index int, littleEndian bool) (pixels.LUT, bool, error) {
	if h == nil {
		return pixels.LUT{}, false, nil
	}
	seq, ok := h.GetSequence(seqTag)
	if !ok || seq == nil || seq.Len() == 0 {
		return pixels.LUT{}, false, nil
	}
	if index < 0 || index >= seq.Len() {
		return pixels.LUT{}, false, fmt.Errorf("godicom: LUT sequence index %d out of range", index)
	}
	item := seq.Get(index)
	if item == nil {
		return pixels.LUT{}, false, nil
	}
	descElem, ok := item.Get(tag.LUTDescriptor)
	if !ok || descElem == nil {
		return pixels.LUT{}, false, nil
	}
	nrEntries, firstMap, depth, ok := lutDescriptorParts(descElem.Value)
	if !ok {
		return pixels.LUT{}, false, fmt.Errorf("godicom: invalid LUT Descriptor")
	}
	if nrEntries == 0 {
		nrEntries = 1 << 16
	}
	dataElem, ok := item.Get(tag.LUTData)
	if !ok || dataElem == nil {
		return pixels.LUT{}, false, nil
	}
	entries, err := unpackLUTData(dataElem, nrEntries, littleEndian)
	if err != nil {
		return pixels.LUT{}, false, err
	}
	return pixels.LUT{
		FirstMap:   firstMap,
		Entries:    entries,
		OutputBits: depth,
	}, true, nil
}

func lutDescriptorParts(v interface{}) (nrEntries, firstMap, depth int, ok bool) {
	switch x := v.(type) {
	case []int:
		if len(x) < 3 {
			return 0, 0, 0, false
		}
		return x[0], x[1], x[2], true
	case *MultiValue[int]:
		if x.Len() < 3 {
			return 0, 0, 0, false
		}
		return x.Get(0), x.Get(1), x.Get(2), true
	case *MultiValue[interface{}]:
		if x.Len() < 3 {
			return 0, 0, 0, false
		}
		a, oka := asInt(x.Get(0))
		b, okb := asInt(x.Get(1))
		c, okc := asInt(x.Get(2))
		return a, b, c, oka && okb && okc
	default:
		return 0, 0, 0, false
	}
}

func asInt(v interface{}) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case uint16:
		return int(x), true
	case int16:
		return int(x), true
	default:
		return 0, false
	}
}

func unpackLUTData(elem *DataElement, nrEntries int, littleEndian bool) ([]uint16, error) {
	switch v := elem.Value.(type) {
	case []int:
		out := make([]uint16, len(v))
		for i, n := range v {
			out[i] = uint16(n)
		}
		return out, nil
	case *MultiValue[int]:
		vals := v.Values()
		out := make([]uint16, len(vals))
		for i, n := range vals {
			out[i] = uint16(n)
		}
		return out, nil
	case []uint16:
		out := make([]uint16, len(v))
		copy(out, v)
		return out, nil
	case *MultiValue[uint16]:
		out := make([]uint16, v.Len())
		copy(out, v.Values())
		return out, nil
	case []byte:
		return lutEntriesFromOW(v, nrEntries, littleEndian)
	default:
		return nil, fmt.Errorf("godicom: unsupported LUT Data type %T", elem.Value)
	}
}

func lutEntriesFromOW(b []byte, nrEntries int, littleEndian bool) ([]uint16, error) {
	need := nrEntries * 2
	if len(b) < need {
		// Some files store 8-bit entries in OW padding; try 1 byte/entry.
		if len(b) >= nrEntries {
			out := make([]uint16, nrEntries)
			for i := 0; i < nrEntries; i++ {
				out[i] = uint16(b[i])
			}
			return out, nil
		}
		return nil, fmt.Errorf("godicom: LUT Data too short: have %d want %d", len(b), need)
	}
	out := make([]uint16, nrEntries)
	for i := 0; i < nrEntries; i++ {
		if littleEndian {
			out[i] = binary.LittleEndian.Uint16(b[i*2 : i*2+2])
		} else {
			out[i] = binary.BigEndian.Uint16(b[i*2 : i*2+2])
		}
	}
	return out, nil
}
