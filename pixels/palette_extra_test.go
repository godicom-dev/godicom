package pixels_test

import (
	"testing"

	"github.com/godicom-dev/godicom/pixels"
)

func TestPalettePixelsAtAndPack(t *testing.T) {
	p := pixels.PalettePixels{
		Samples:  []uint16{1, 2, 3, 4, 5, 6},
		Channels: 3,
		Bits:     8,
	}
	if got := p.At(1, 2); got != 6 {
		t.Fatalf("At(1,2) = %d, want 6", got)
	}
	if got := p.At(0, 0); got != 1 {
		t.Fatalf("At(0,0) = %d, want 1", got)
	}
	if got := p.PackDisplayU8(); len(got) != 6 || got[5] != 6 {
		t.Fatalf("PackDisplayU8 at 8 bits = %v", got)
	}
}

func TestPalettePixelsPackDisplayU8ShiftsSixteenBit(t *testing.T) {
	// A 16-bit table has no 8-bit form, so the display byte is the high byte --
	// the same reduction pydicom's palette output does when it writes 8 bits.
	p := pixels.PalettePixels{
		Samples:  []uint16{0x0000, 0x00FF, 0xFF00, 0xFFFF},
		Channels: 1,
		Bits:     16,
	}
	got := p.PackDisplayU8()
	want := []byte{0x00, 0x00, 0xFF, 0xFF}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func colorLUTFixture() pixels.ColorLUT {
	return pixels.ColorLUT{
		FirstMap:  0,
		NrEntries: 3,
		Bits:      8,
		Tables: [][]uint16{
			{10, 20, 30},
			{11, 21, 31},
			{12, 22, 32},
		},
	}
}

func TestApplyColorLUTClampsAndKeepsInput(t *testing.T) {
	lut := colorLUTFixture()
	in := []float64{-5, 0, 1, 2, 9}
	out, err := pixels.ApplyColorLUT(in, lut)
	if err != nil {
		t.Fatal(err)
	}
	if out.Channels != 3 || out.Bits != 8 {
		t.Fatalf("got %+v", out)
	}
	if out.At(0, 0) != 10 || out.At(1, 0) != 10 || out.At(2, 0) != 20 {
		t.Fatalf("clamped samples wrong: %v", out.Samples)
	}
	if out.At(4, 2) != 32 {
		t.Fatalf("value past the last entry should use it: %v", out.Samples)
	}
	if in[0] != -5 {
		t.Fatal("ApplyColorLUT modified its input")
	}
}

func TestApplyColorLUTFirstMapOffset(t *testing.T) {
	lut := colorLUTFixture()
	lut.FirstMap = 100
	out, err := pixels.ApplyColorLUT([]float64{99, 100, 102}, lut)
	if err != nil {
		t.Fatal(err)
	}
	if out.At(0, 0) != 10 || out.At(1, 0) != 10 || out.At(2, 0) != 30 {
		t.Fatalf("first-map offset wrong: %v", out.Samples)
	}
}

func TestApplyColorLUTAlphaChannel(t *testing.T) {
	lut := colorLUTFixture()
	lut.Tables = append(lut.Tables, []uint16{0, 127, 255})
	out, err := pixels.ApplyColorLUT([]float64{2}, lut)
	if err != nil {
		t.Fatal(err)
	}
	if out.Channels != 4 {
		t.Fatalf("channels = %d, want 4", out.Channels)
	}
	if out.At(0, 3) != 255 {
		t.Fatalf("alpha = %d, want 255", out.At(0, 3))
	}
}

func TestApplyColorLUTErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		lut  pixels.ColorLUT
	}{
		{"no entries", pixels.ColorLUT{NrEntries: 0, Tables: [][]uint16{{1}, {2}, {3}}}},
		{"negative entries", pixels.ColorLUT{NrEntries: -1, Tables: [][]uint16{{1}, {2}, {3}}}},
		{"two tables", pixels.ColorLUT{NrEntries: 1, Tables: [][]uint16{{1}, {2}}}},
		{"ragged tables", pixels.ColorLUT{NrEntries: 1, Tables: [][]uint16{{1}, {2}, {1, 2}}}},
		{"table shorter than descriptor", pixels.ColorLUT{NrEntries: 4, Tables: [][]uint16{{1}, {2}, {3}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pixels.ApplyColorLUT([]float64{0}, tc.lut); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
