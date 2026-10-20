package pixels

import (
	"fmt"
	"math"
)

// PalettePixels is interleaved RGB or RGBA produced by a palette color LUT.
// Bits is 8 or 16, the entry depth the descriptor and the LUT bytes agreed on.
// Samples holds one value per channel; an 8-bit entry is stored in the low byte.
type PalettePixels struct {
	Samples  []uint16
	Channels int
	Bits     int
}

// At returns the channel value for pixel index.
func (p PalettePixels) At(pixel, channel int) uint16 {
	return p.Samples[pixel*p.Channels+channel]
}

// PackDisplayU8 reduces each entry to 8 bits for an encoder that wants bytes.
// A 16-bit LUT entry keeps its high byte, which is the 8-bit display value of a
// table scaled across the full 16-bit range.
func (p PalettePixels) PackDisplayU8() []byte {
	shift := 0
	if p.Bits > 8 {
		shift = p.Bits - 8
	}
	out := make([]byte, len(p.Samples))
	for i, v := range p.Samples {
		out[i] = byte(v >> shift)
	}
	return out
}

// ColorLUT is one palette after its descriptor and LUT bytes have been decoded.
// Tables are RGB, plus alpha when the dataset carried one. Each table has the
// same length. FirstMap is the stored pixel value that indexes table entry 0;
// smaller values use entry 0, and values past the last entry use that entry.
type ColorLUT struct {
	FirstMap  int
	NrEntries int
	Bits      int
	Tables    [][]uint16
}

// ApplyColorLUT maps stored pixel values through lut.
// The input slice is left unchanged.
// Mirrors pydicom.pixels.processing.apply_color_lut's index arithmetic.
func ApplyColorLUT(samples []float64, lut ColorLUT) (PalettePixels, error) {
	if lut.NrEntries <= 0 {
		return PalettePixels{}, fmt.Errorf("pixels: palette LUT has no entries")
	}
	if len(lut.Tables) < 3 {
		return PalettePixels{}, fmt.Errorf("pixels: palette LUT needs red, green and blue tables")
	}
	n := lut.Tables[0]
	for _, table := range lut.Tables[1:] {
		if len(table) != len(n) {
			return PalettePixels{}, fmt.Errorf("pixels: LUT data must be the same length")
		}
	}
	if len(n) < lut.NrEntries {
		return PalettePixels{}, fmt.Errorf("pixels: palette LUT has %d entries, descriptor says %d", len(n), lut.NrEntries)
	}

	channels := len(lut.Tables)
	out := make([]uint16, len(samples)*channels)
	last := lut.NrEntries - 1
	for i, iv := range samples {
		idx := 0
		if iv >= float64(lut.FirstMap) {
			idx = int(iv) - lut.FirstMap
		}
		if idx < 0 {
			idx = 0
		}
		if idx > last {
			idx = last
		}
		base := i * channels
		for c, table := range lut.Tables {
			out[base+c] = table[idx]
		}
	}
	return PalettePixels{Samples: out, Channels: channels, Bits: lut.Bits}, nil
}

// ExpandSegmentedLUT reconstructs a segmented palette color lookup table.
// bits is 8 or 16. littleEndian selects the indirect-segment byte order for
// 8-bit data; 16-bit data is already decoded into data. nrSegments limits how
// many segments are read; 0 means the whole table. lastValue is the sample a
// recursive indirect expansion continues from. A lastValue of 0 is ignored,
// matching pydicom treating a missing and a zero previous value the same way.
//
// Mirrors pydicom.pixels.processing._expand_segmented_lut.
func ExpandSegmentedLUT(data []int, bits int, littleEndian bool, nrSegments, lastValue int) ([]int, error) {
	lut := make([]int, 0)
	offset := 0
	segmentsRead := 0
	for offset+1 < len(data) {
		opcode := data[offset]
		length := data[offset+1]
		offset += 2

		switch opcode {
		case 0:
			if length < 0 || offset+length > len(data) {
				return nil, fmt.Errorf("pixels: discrete palette segment runs past the LUT data")
			}
			lut = append(lut, data[offset:offset+length]...)
			offset += length
		case 1:
			var y0 int
			switch {
			case len(lut) > 0:
				y0 = lut[len(lut)-1]
			case lastValue != 0:
				y0 = lastValue
			default:
				return nil, fmt.Errorf("pixels: error expanding a segmented palette color lookup table: the first segment cannot be a linear segment")
			}
			if offset >= len(data) {
				return nil, fmt.Errorf("pixels: linear palette segment is missing its end value")
			}
			y1 := data[offset]
			offset++
			if y0 == y1 {
				for i := 0; i < length; i++ {
					lut = append(lut, y1)
				}
			} else {
				step := float64(y1-y0) / float64(length)
				start := float64(y0) + step
				stop := float64(y1)
				for i := 0; i < length; i++ {
					var v float64
					if length == 1 {
						v = start
					} else {
						v = start + (stop-start)*float64(i)/float64(length-1)
					}
					lut = append(lut, roundHalfEven(v))
				}
			}
		case 2:
			if len(lut) == 0 {
				return nil, fmt.Errorf("pixels: error expanding a segmented palette color lookup table: the first segment cannot be an indirect segment")
			}
			var byteOffset int
			if bits == 8 {
				if offset+4 > len(data) {
					return nil, fmt.Errorf("pixels: indirect palette segment is missing its offset")
				}
				var ii [4]int
				if littleEndian {
					ii = [4]int{data[offset+3], data[offset+2], data[offset+1], data[offset]}
				} else {
					ii = [4]int{data[offset+2], data[offset+3], data[offset], data[offset+1]}
				}
				byteOffset = ((ii[0]<<8)|ii[1])<<16 | ((ii[2] << 8) | ii[3])
				offset += 4
			} else {
				if offset+2 > len(data) {
					return nil, fmt.Errorf("pixels: indirect palette segment is missing its offset")
				}
				byteOffset = data[offset+1]<<16 | data[offset]
				offset += 2
			}
			if byteOffset < 0 || byteOffset > len(data) {
				return nil, fmt.Errorf("pixels: indirect palette segment offset %d is outside the LUT data", byteOffset)
			}
			nested, err := ExpandSegmentedLUT(data[byteOffset:], bits, littleEndian, length, lut[len(lut)-1])
			if err != nil {
				return nil, err
			}
			lut = append(lut, nested...)
		default:
			return nil, fmt.Errorf("pixels: error expanding a segmented palette lookup table: unknown segment type '%d'", opcode)
		}

		segmentsRead++
		if nrSegments > 0 && segmentsRead == nrSegments {
			return lut, nil
		}
	}
	return lut, nil
}

func roundHalfEven(v float64) int {
	floor := math.Floor(v)
	diff := v - floor
	if diff > 0.5 {
		return int(floor) + 1
	}
	if diff < 0.5 {
		return int(floor)
	}
	if int64(floor)%2 == 0 {
		return int(floor)
	}
	return int(floor) + 1
}
