package pixels

import (
	"bytes"
	"compress/flate"
	"fmt"

	"github.com/godicom-dev/godicom/encaps"
	"github.com/godicom-dev/godicom/uid"
	"github.com/godicom-dev/gorle"
)

// EncodeOption configures EncodeOptions.
type EncodeOption func(*EncodeOptions)

// EncodeOptions configures frame encoding.
type EncodeOptions struct {
	// TransferSyntaxUID selects the encoder (native / RLE / Deflated).
	TransferSyntaxUID uid.UID
	// FragmentsPerFrame is passed to encaps.Encapsulate (default 1).
	FragmentsPerFrame int
	// HasBOT controls Basic Offset Table population (default true for compressed).
	HasBOT *bool
	// UseExtendedOffsetTable uses encaps.EncapsulateExtended instead of Encapsulate.
	UseExtendedOffsetTable bool
}

// WithEncodeTransferSyntax sets the target transfer syntax (also set via EncodeOptions).
func WithEncodeTransferSyntax(ts uid.UID) EncodeOption {
	return func(o *EncodeOptions) { o.TransferSyntaxUID = ts }
}

// WithFragmentsPerFrame sets fragments per frame for encapsulation.
func WithFragmentsPerFrame(n int) EncodeOption {
	return func(o *EncodeOptions) { o.FragmentsPerFrame = n }
}

// WithBasicOffsetTable sets whether the Basic Offset Table is populated.
func WithBasicOffsetTable(hasBOT bool) EncodeOption {
	return func(o *EncodeOptions) { o.HasBOT = &hasBOT }
}

// WithExtendedOffsetTable requests EncapsulateExtended.
func WithExtendedOffsetTable(enabled bool) EncodeOption {
	return func(o *EncodeOptions) { o.UseExtendedOffsetTable = enabled }
}

// EncodedPixelData is the result of encoding one or more frames.
type EncodedPixelData struct {
	PixelData                  []byte
	TransferSyntaxUID          uid.UID
	IsEncapsulated             bool
	ExtendedOffsetTable        []byte
	ExtendedOffsetTableLengths []byte
}

// EncodeFrame encodes a single uncompressed frame to the target transfer syntax.
// Supported: native (uncompressed), RLE Lossless, Deflated Image Frame Compression.
// JPEG / JPEG-LS / JPEG2000 encode requires golibjpeg/goopenjpeg encode APIs (not yet).
func EncodeFrame(src []byte, desc Descriptor, ts uid.UID) ([]byte, error) {
	switch {
	case !ts.IsCompressed():
		out := make([]byte, len(src))
		copy(out, src)
		return out, nil
	case ts == uid.RLELossless:
		return encodeRLE(src, desc)
	case ts == uid.DeflatedImageFrameCompression:
		return encodeDeflated(src)
	default:
		return nil, fmt.Errorf("pixels: encode unsupported for transfer syntax %s (JPEG/J2K encode not available yet)", ts)
	}
}

// EncodeFrames encodes uncompressed frames and builds PixelData bytes
// (concatenated for native, encapsulated for compressed).
func EncodeFrames(frames [][]byte, desc Descriptor, opts EncodeOptions) (*EncodedPixelData, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("pixels: no frames to encode")
	}
	ts := opts.TransferSyntaxUID
	if ts == "" {
		return nil, fmt.Errorf("pixels: TransferSyntaxUID required for encode")
	}

	encoded := make([][]byte, len(frames))
	for i, frame := range frames {
		var err error
		encoded[i], err = EncodeFrame(frame, desc, ts)
		if err != nil {
			return nil, fmt.Errorf("pixels: encode frame %d: %w", i, err)
		}
	}

	out := &EncodedPixelData{TransferSyntaxUID: ts}
	if !ts.IsCompressed() {
		var total int
		for _, f := range encoded {
			total += len(f)
		}
		buf := make([]byte, 0, total)
		for _, f := range encoded {
			buf = append(buf, f...)
		}
		out.PixelData = buf
		return out, nil
	}

	out.IsEncapsulated = true
	if opts.UseExtendedOffsetTable {
		pd, offsets, lengths, err := encaps.EncapsulateExtended(encoded)
		if err != nil {
			return nil, err
		}
		out.PixelData = pd
		out.ExtendedOffsetTable = offsets
		out.ExtendedOffsetTableLengths = lengths
		return out, nil
	}

	frags := opts.FragmentsPerFrame
	if frags <= 0 {
		frags = 1
	}
	hasBOT := true
	if opts.HasBOT != nil {
		hasBOT = *opts.HasBOT
	}
	pd, err := encaps.Encapsulate(encoded, frags, hasBOT)
	if err != nil {
		return nil, err
	}
	out.PixelData = pd
	return out, nil
}

func encodeRLE(src []byte, desc Descriptor) ([]byte, error) {
	return gorle.EncodePixelData(src, gorle.PixelDataOptions{
		FrameOptions: gorle.FrameOptions{
			Rows:            desc.Rows,
			Columns:         desc.Columns,
			SamplesPerPixel: desc.SamplesPerPixel,
			BitsAllocated:   desc.BitsAllocated,
			ByteOrder:       gorle.LittleEndian,
		},
	})
}

func encodeDeflated(src []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(src); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
