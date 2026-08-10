package godicom

import (
	"fmt"

	"github.com/godicom-dev/godicom/pixels"
)

// PixelArray returns decoded pixel samples and geometry, similar to pydicom pixel_array.
// By default photometric colour transforms apply (YBR→RGB, planar→interleaved).
func (fd *FileDataset) PixelArray(opts ...pixels.DecodeOption) (*pixels.Array, error) {
	if fd == nil {
		return nil, fmt.Errorf("godicom: nil dataset")
	}
	o := applyDecodeOptionsForArray(opts)
	desc, err := pixels.DescriptorFromFile(fd)
	if err != nil {
		return nil, err
	}
	raw, err := fd.PixelBytes(opts...)
	if err != nil {
		return nil, err
	}
	samples, err := pixels.UnpackSamples(raw, desc.BitsAllocated, desc.PixelRepresentation, fd.pixelLittleEndian())
	if err != nil {
		return nil, err
	}
	frames := desc.NumberOfFrames
	if o.FrameIndex != nil {
		frames = 1
	}
	return pixels.BuildArray(samples, desc, frames)
}

func applyDecodeOptionsForArray(opts []pixels.DecodeOption) pixels.DecodeOptions {
	out := pixels.DecodeOptions{}
	for _, fn := range opts {
		if fn != nil {
			fn(&out)
		}
	}
	return out
}
