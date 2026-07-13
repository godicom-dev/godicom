package pixels

import (
	"encoding/binary"
	"fmt"
)

// UnpackSamples converts raw pixel bytes to float64 samples (one value per
// sample). Supports BitsAllocated 8 and 16; PixelRepresentation 0 (unsigned)
// or 1 (signed twos-complement).
func UnpackSamples(data []byte, bitsAllocated, pixelRepresentation int, littleEndian bool) ([]float64, error) {
	switch bitsAllocated {
	case 8:
		out := make([]float64, len(data))
		if pixelRepresentation == 1 {
			for i, b := range data {
				out[i] = float64(int8(b))
			}
		} else {
			for i, b := range data {
				out[i] = float64(b)
			}
		}
		return out, nil
	case 16:
		if len(data)%2 != 0 {
			return nil, fmt.Errorf("pixels: 16-bit sample data length %d is odd", len(data))
		}
		n := len(data) / 2
		out := make([]float64, n)
		for i := 0; i < n; i++ {
			var u uint16
			if littleEndian {
				u = binary.LittleEndian.Uint16(data[i*2 : i*2+2])
			} else {
				u = binary.BigEndian.Uint16(data[i*2 : i*2+2])
			}
			if pixelRepresentation == 1 {
				out[i] = float64(int16(u))
			} else {
				out[i] = float64(u)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("pixels: UnpackSamples unsupported BitsAllocated=%d", bitsAllocated)
	}
}
