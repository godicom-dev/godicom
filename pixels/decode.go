package pixels

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"

	"github.com/godicom-dev/goopenjpeg"
	"github.com/godicom-dev/godicom/encaps"
	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
	"github.com/godicom-dev/golibjpeg"
	"github.com/godicom-dev/gorle"
)

// DecodeFrame decodes a single encapsulated frame's compressed bytes.
func DecodeFrame(frame []byte, desc Descriptor, opts DecodeOptions) ([]byte, error) {
	switch {
	case desc.TransferSyntaxUID == uid.RLELossless:
		return decodeRLE(frame, desc, opts)
	case isLibjpegSyntax(desc.TransferSyntaxUID):
		return decodeLibjpeg(frame, desc, opts)
	case isOpenjpegSyntax(desc.TransferSyntaxUID):
		return decodeOpenjpeg(frame, desc, opts)
	case desc.TransferSyntaxUID == uid.DeflatedImageFrameCompression:
		return decodeDeflatedFrame(frame)
	default:
		return nil, fmt.Errorf("pixels: unsupported transfer syntax %s", desc.TransferSyntaxUID)
	}
}

// DecodePixelData decodes all (or selected) frames from a file dataset.
func DecodePixelData(fd FileSource, opts ...DecodeOption) ([][]byte, error) {
	o := applyDecodeOptions(opts)
	desc, err := DescriptorFromFile(fd)
	if err != nil {
		return nil, err
	}
	pixelData, ok := fd.GetBytes(tag.PixelData)
	if !ok {
		return nil, fmt.Errorf("pixels: PixelData missing")
	}

	if !desc.TransferSyntaxUID.IsCompressed() {
		return decodeNativeFrames(pixelData, desc, o)
	}

	encOpts := encaps.FramesOptions{
		NumberOfFrames: desc.NumberOfFrames,
		LittleEndian:   true,
	}
	if desc.ExtendedOffsets != nil {
		encOpts.ExtendedOffsets = &encaps.ExtendedOffsetTable{
			Offsets: desc.ExtendedOffsets.Offsets,
			Lengths: desc.ExtendedOffsets.Lengths,
		}
	}

	encodedFrames, err := encaps.GenerateFrames(pixelData, encOpts)
	if err != nil {
		return nil, err
	}
	if o.FrameIndex != nil {
		idx := *o.FrameIndex
		if idx < 0 || idx >= len(encodedFrames) {
			return nil, fmt.Errorf("pixels: frame index %d out of range (have %d)", idx, len(encodedFrames))
		}
		decoded, err := DecodeFrame(encodedFrames[idx], desc, o)
		if err != nil {
			return nil, err
		}
		return [][]byte{decoded}, nil
	}

	out := make([][]byte, len(encodedFrames))
	for i, enc := range encodedFrames {
		out[i], err = DecodeFrame(enc, desc, o)
		if err != nil {
			return nil, fmt.Errorf("pixels: frame %d: %w", i, err)
		}
	}
	return out, nil
}

// DecodeAllFrames concatenates decoded frames into one buffer (native layout per frame).
func DecodeAllFrames(fd FileSource, opts ...DecodeOption) ([]byte, error) {
	frames, err := DecodePixelData(fd, opts...)
	if err != nil {
		return nil, err
	}
	var total int
	for _, f := range frames {
		total += len(f)
	}
	out := make([]byte, 0, total)
	for _, f := range frames {
		out = append(out, f...)
	}
	return out, nil
}

func decodeNativeFrames(pixelData []byte, desc Descriptor, opts DecodeOptions) ([][]byte, error) {
	frameBytes := desc.UnpackedFrameBytes()
	want := frameBytes * desc.NumberOfFrames
	if len(pixelData) != want {
		return nil, fmt.Errorf("pixels: native pixel data length %d, want %d", len(pixelData), want)
	}
	if opts.FrameIndex != nil {
		idx := *opts.FrameIndex
		if idx < 0 || idx >= desc.NumberOfFrames {
			return nil, fmt.Errorf("pixels: frame index %d out of range (have %d)", idx, desc.NumberOfFrames)
		}
		start := idx * frameBytes
		out := make([]byte, frameBytes)
		copy(out, pixelData[start:start+frameBytes])
		return [][]byte{out}, nil
	}
	if desc.NumberOfFrames == 1 {
		out := make([]byte, frameBytes)
		copy(out, pixelData)
		return [][]byte{out}, nil
	}
	out := make([][]byte, desc.NumberOfFrames)
	for i := 0; i < desc.NumberOfFrames; i++ {
		start := i * frameBytes
		frame := make([]byte, frameBytes)
		copy(frame, pixelData[start:start+frameBytes])
		out[i] = frame
	}
	return out, nil
}

func decodeDeflatedFrame(src []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(src))
	defer r.Close()
	return io.ReadAll(r)
}

func decodeRLE(frame []byte, desc Descriptor, opts DecodeOptions) ([]byte, error) {
	version := gorle.PixelDataV2
	if !opts.Raw {
		version = gorle.PixelDataV1
	}
	return gorle.DecodePixelData(frame, gorle.PixelDataOptions{
		Version: version,
		FrameOptions: gorle.FrameOptions{
			Rows:            desc.Rows,
			Columns:         desc.Columns,
			SamplesPerPixel: desc.SamplesPerPixel,
			BitsAllocated:   desc.BitsAllocated,
			ByteOrder:       gorle.LittleEndian,
		},
	})
}

func decodeLibjpeg(frame []byte, desc Descriptor, opts DecodeOptions) ([]byte, error) {
	version := golibjpeg.PixelDataV2
	if !opts.Raw {
		version = golibjpeg.PixelDataV1
	}
	return golibjpeg.DecodePixelData(frame, golibjpeg.PixelDataOptions{
		Version:                   version,
		PhotometricInterpretation: desc.PhotometricInterpretation,
	})
}

func decodeOpenjpeg(frame []byte, desc Descriptor, opts DecodeOptions) ([]byte, error) {
	version := goopenjpeg.PixelDataV2
	if !opts.Raw {
		version = goopenjpeg.PixelDataV1
	}
	codec := goopenjpeg.CodecJ2K
	if len(frame) >= 8 && frame[4] == 'j' && frame[5] == 'P' && frame[6] == ' ' && frame[7] == ' ' {
		codec = goopenjpeg.CodecJP2
	}
	return goopenjpeg.DecodePixelData(frame, goopenjpeg.PixelDataOptions{
		Version:                   version,
		Codec:                     codec,
		PhotometricInterpretation: desc.PhotometricInterpretation,
	})
}

func isLibjpegSyntax(ts uid.UID) bool {
	switch ts {
	case uid.JPEGBaseline8Bit, uid.JPEGExtended12Bit, uid.JPEGLossless, uid.JPEGLosslessSV1,
		uid.JPEGLSLossless, uid.JPEGLSNearLossless:
		return true
	default:
		return false
	}
}

func isOpenjpegSyntax(ts uid.UID) bool {
	switch ts {
	case uid.JPEG2000Lossless, uid.JPEG2000, uid.HTJ2KLossless, uid.HTJ2KLosslessRPCL, uid.HTJ2K:
		return true
	default:
		return false
	}
}
