package godicom

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/godicom-dev/godicom/tag"
)

// Overlay is one Overlay Plane, unpacked from (60xx,3000) Overlay Data.
// Samples holds one byte per pixel, 0 or 1, row-major. A single frame is
// Rows*Columns long. More than one frame is concatenated in frame order,
// which is the (frames, rows, columns) layout of pydicom's overlay_array.
type Overlay struct {
	Samples []byte
	Rows    int
	Columns int
	Frames  int
}

// At returns the bit at frame, row, column. A single frame ignores frame.
func (o Overlay) At(frame, row, col int) byte {
	if o.Frames > 1 {
		return o.Samples[(frame*o.Rows+row)*o.Columns+col]
	}
	return o.Samples[row*o.Columns+col]
}

// OverlayArray returns the Overlay Data in group as an unpacked bit plane.
// group is the group number, for example 0x6000. It must be between 0x6000
// and 0x60FF. Bits are unpacked least-significant-bit first, which is how
// 1-bit Overlay Data is packed. Number of Frames in Overlay defaults to 1
// when the element is absent.
//
// Mirrors pydicom.overlays.numpy_handler.get_overlay_array and Dataset.overlay_array.
func (d *Dataset) OverlayArray(group int) (Overlay, error) {
	if group < 0x6000 || group > 0x60FF {
		return Overlay{}, fmt.Errorf("godicom: the group part of the 'Overlay Data' element tag must be between 0x6000 and 0x60FF (inclusive)")
	}
	rowsTag := overlayTag(group, 0x0010)
	colsTag := overlayTag(group, 0x0011)
	framesTag := overlayTag(group, 0x0015)
	bitsTag := overlayTag(group, 0x0100)
	dataTag := overlayTag(group, 0x3000)

	var missing []string
	data, hasData := d.GetBytes(dataTag)
	if !hasData {
		missing = append(missing, "OverlayData")
	}
	if _, ok := d.GetInt(bitsTag); !ok {
		missing = append(missing, "OverlayBitsAllocated")
	}
	rows, hasRows := d.GetInt(rowsTag)
	if !hasRows {
		missing = append(missing, "OverlayRows")
	}
	cols, hasCols := d.GetInt(colsTag)
	if !hasCols {
		missing = append(missing, "OverlayColumns")
	}
	if len(missing) > 0 {
		return Overlay{}, fmt.Errorf("godicom: unable to convert the overlay data as the following required elements are missing from the dataset: %s", joinComma(missing))
	}

	frames := 1
	if n, ok := d.GetInt(framesTag); ok {
		frames = n
	}
	if frames < 1 {
		return Overlay{}, fmt.Errorf("godicom: unable to reshape the overlay array as a value of %d for (60xx,0015) 'Number of Frames in Overlay' is invalid", frames)
	}

	nPixels := rows * cols * frames
	expected := overlayByteLength(nPixels)
	padded := expected + expected%2
	actual := len(data)
	switch {
	case actual < padded:
		if actual == expected {
			logOverlay(d, "The overlay data length is odd and misses a padding byte.")
		} else {
			return Overlay{}, fmt.Errorf("godicom: the length of the overlay data in the dataset (%d bytes) doesn't match the expected length (%d bytes). The dataset may be corrupted or there may be an issue with the overlay data handler", actual, padded)
		}
	case actual > padded:
		logOverlay(d, fmt.Sprintf("The length of the overlay data in the dataset (%d bytes) indicates it contains excess padding. %d bytes will be removed from the end of the data", actual, actual-expected))
	}

	return Overlay{
		Samples: unpackOverlayBits(data, nPixels),
		Rows:    rows,
		Columns: cols,
		Frames:  frames,
	}, nil
}

func overlayByteLength(nPixels int) int {
	n := nPixels / 8
	if nPixels%8 > 0 {
		n++
	}
	return n
}

func unpackOverlayBits(src []byte, nPixels int) []byte {
	out := make([]byte, nPixels)
	for i := 0; i < nPixels; i++ {
		if int(src[i/8])&(1<<uint(i%8)) != 0 {
			out[i] = 1
		}
	}
	return out
}

func overlayTag(group, element int) Tag {
	return tag.Tag(uint32(group)<<16 | uint32(element))
}

func joinComma(parts []string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += ", " + p
	}
	return out
}

func logOverlay(ds *Dataset, msg string) {
	ctx := context.Background()
	if ds != nil && ds.readCtx != nil {
		ctx = ds.readCtx.logCtx()
	}
	l := LoggerFromContext(ctx)
	if l.Enabled(ctx, slog.LevelWarn) {
		l.WarnContext(ctx, msg)
	}
}
