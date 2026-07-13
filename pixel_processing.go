package godicom

import (
	"encoding/binary"
	"fmt"

	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
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
func (fd *FileDataset) ApplyModalityLUT(arr []float64) ([]float64, error) {
	p, err := fd.modalityParams()
	if err != nil {
		return nil, err
	}
	return pixels.ApplyModalityLUT(arr, p)
}

// ApplyVOILUT applies VOI LUT Sequence or Window Center/Width.
// preferLUT matches pydicom apply_voi_lut(prefer_lut=...).
func (fd *FileDataset) ApplyVOILUT(arr []float64, index int, preferLUT bool) ([]float64, error) {
	p, err := fd.voiParams(index, preferLUT)
	if err != nil {
		return nil, err
	}
	return pixels.ApplyVOILUT(arr, p)
}

// ApplyPresentationLUTShape applies (2050,0020) Presentation LUT Shape when present.
func (fd *FileDataset) ApplyPresentationLUTShape(arr []float64) ([]float64, error) {
	shape, ok := fd.GetString(tag.PresentationLUTShape)
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
	return uid.UID(ts).IsLittleEndian()
}

func (fd *FileDataset) modalityParams() (pixels.ModalityParams, error) {
	if lut, ok, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, 0); err != nil {
		return pixels.ModalityParams{}, err
	} else if ok {
		return pixels.ModalityParams{LUT: &lut}, nil
	}
	slope, hasSlope := fd.GetFloat(tag.RescaleSlope)
	intercept, hasIntercept := fd.GetFloat(tag.RescaleIntercept)
	if hasSlope && hasIntercept {
		return pixels.ModalityParams{
			HasRescale:       true,
			RescaleSlope:     slope,
			RescaleIntercept: intercept,
		}, nil
	}
	return pixels.ModalityParams{}, nil
}

func (fd *FileDataset) voiParams(index int, preferLUT bool) (pixels.VOIParams, error) {
	p := pixels.VOIParams{PreferLUT: preferLUT, Index: index}
	if lut, ok, err := fd.readLUTFromSequence(tag.VOILUTSequence, index); err != nil {
		return p, err
	} else if ok {
		p.LUT = &lut
	}
	centers, hasC := fd.GetFloats(tag.WindowCenter)
	widths, hasW := fd.GetFloats(tag.WindowWidth)
	if hasC && hasW && len(centers) > 0 && len(widths) > 0 {
		if index < 0 || index >= len(centers) || index >= len(widths) {
			return p, fmt.Errorf("godicom: VOI window index %d out of range", index)
		}
		cfg, err := fd.windowConfig(centers[index], widths[index])
		if err != nil {
			return p, err
		}
		p.Window = &cfg
	}
	return p, nil
}

func (fd *FileDataset) windowConfig(center, width float64) (pixels.WindowConfig, error) {
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
	if fn, ok := fd.GetString(tag.VOILUTFunction); ok {
		cfg.Function = fn
	}
	if lut, ok, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, 0); err != nil {
		return cfg, err
	} else if ok {
		cfg.HasModalityLUT = true
		cfg.ModalityLUTOutputBits = lut.OutputBits
	} else {
		slope, hasSlope := fd.GetFloat(tag.RescaleSlope)
		intercept, hasIntercept := fd.GetFloat(tag.RescaleIntercept)
		if hasSlope && hasIntercept {
			cfg.HasRescale = true
			cfg.RescaleSlope = slope
			cfg.RescaleIntercept = intercept
		}
	}
	return cfg, nil
}

func (fd *FileDataset) readLUTFromSequence(seqTag tag.Tag, index int) (pixels.LUT, bool, error) {
	seq, ok := fd.GetSequence(seqTag)
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
	entries, err := unpackLUTData(dataElem, nrEntries, fd.pixelLittleEndian())
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
