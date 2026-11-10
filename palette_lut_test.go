package godicom

import (
	"testing"

	"github.com/godicom-dev/godicom/tag"
)

// newPaletteDataset is a file dataset that can carry a Palette Color Lookup
// Table module; the descriptor is required for any of the palette paths.
func newPaletteDataset(descriptor interface{}, nominal int) *FileDataset {
	fd := newLUTDataset()
	fd.Set(NewDataElement(tag.RedPaletteColorLookupTableDescriptor, VRUS, descriptor))
	return fd
}

func setPaletteTable(fd *FileDataset, tg Tag, data []byte) {
	fd.Set(NewDataElement(tg, VROW, data))
}

func TestIntSliceFromValueTypes(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want []int
		ok   bool
	}{
		{"int slice", []int{1, 2}, []int{1, 2}, true},
		{"int64 slice", []int64{1, 2}, []int{1, 2}, true},
		{"uint64 slice", []uint64{1, 2}, []int{1, 2}, true},
		{"uint16 slice", []uint16{1, 2}, []int{1, 2}, true},
		{"int multivalue", NewMultiValue([]int{1, 2}), []int{1, 2}, true},
		{"int64 multivalue", NewMultiValue([]int64{1, 2}), []int{1, 2}, true},
		{"uint64 multivalue", NewMultiValue([]uint64{1, 2}), []int{1, 2}, true},
		{"uint16 multivalue", NewMultiValue([]uint16{1, 2}), []int{1, 2}, true},
		{"single int", 4, []int{4}, true},
		{"single int64", int64(4), []int{4}, true},
		{"single uint64", uint64(4), []int{4}, true},
		{"single uint16", uint16(4), []int{4}, true},
		{"nil multivalue", (*MultiValue[int])(nil), nil, false},
		{"string", "4", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := intSliceFromValue(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}

func TestColorLUTDirectEightBit(t *testing.T) {
	fd := newPaletteDataset([]int{4, 0, 8}, 8)
	setPaletteTable(fd, tag.RedPaletteColorLookupTableData, []byte{10, 20, 30, 40})
	setPaletteTable(fd, tag.GreenPaletteColorLookupTableData, []byte{11, 21, 31, 41})
	setPaletteTable(fd, tag.BluePaletteColorLookupTableData, []byte{12, 22, 32, 42})

	lut, err := fd.colorLUT()
	if err != nil {
		t.Fatal(err)
	}
	if lut.Bits != 8 || lut.NrEntries != 4 || len(lut.Tables) != 3 {
		t.Fatalf("got %+v", lut)
	}
	if lut.Tables[2][3] != 42 {
		t.Fatalf("blue table = %v", lut.Tables[2])
	}
}

func TestColorLUTDirectWithAlpha(t *testing.T) {
	fd := newPaletteDataset([]int{2, 0, 8}, 8)
	for _, tg := range []Tag{
		tag.RedPaletteColorLookupTableData,
		tag.GreenPaletteColorLookupTableData,
		tag.BluePaletteColorLookupTableData,
		tag.AlphaPaletteColorLookupTableData,
	} {
		setPaletteTable(fd, tg, []byte{1, 2})
	}
	lut, err := fd.colorLUT()
	if err != nil {
		t.Fatal(err)
	}
	if len(lut.Tables) != 4 {
		t.Fatalf("got %d tables, want 4", len(lut.Tables))
	}
}

func TestColorLUTDirectSixteenBit(t *testing.T) {
	fd := newPaletteDataset([]int{2, 0, 16}, 16)
	setPaletteTable(fd, tag.RedPaletteColorLookupTableData, []byte{0x01, 0x02, 0x03, 0x04})
	setPaletteTable(fd, tag.GreenPaletteColorLookupTableData, []byte{0x05, 0x06, 0x07, 0x08})
	setPaletteTable(fd, tag.BluePaletteColorLookupTableData, []byte{0x09, 0x0A, 0x0B, 0x0C})
	lut, err := fd.colorLUT()
	if err != nil {
		t.Fatal(err)
	}
	if lut.Bits != 16 || len(lut.Tables[0]) != 2 || lut.Tables[0][0] != 0x0201 {
		t.Fatalf("got %+v", lut)
	}
}

func TestColorLUTSegmentedEightBit(t *testing.T) {
	fd := newPaletteDataset([]int{4, 0, 8}, 8)
	// One discrete segment: opcode 0, length 4, then four values.
	seg := []byte{0, 4, 10, 20, 30, 40}
	setPaletteTable(fd, tag.SegmentedRedPaletteColorLookupTableData, seg)
	setPaletteTable(fd, tag.SegmentedGreenPaletteColorLookupTableData, seg)
	setPaletteTable(fd, tag.SegmentedBluePaletteColorLookupTableData, seg)

	lut, err := fd.colorLUT()
	if err != nil {
		t.Fatal(err)
	}
	if lut.Bits != 8 || len(lut.Tables) != 3 || len(lut.Tables[0]) != 4 {
		t.Fatalf("got %+v", lut)
	}
	if lut.Tables[0][3] != 40 {
		t.Fatalf("expanded table = %v", lut.Tables[0])
	}
}

func TestColorLUTErrors(t *testing.T) {
	withTables := func() *FileDataset {
		fd := newPaletteDataset([]int{4, 0, 8}, 8)
		setPaletteTable(fd, tag.RedPaletteColorLookupTableData, []byte{1, 2, 3, 4})
		setPaletteTable(fd, tag.GreenPaletteColorLookupTableData, []byte{1, 2, 3, 4})
		setPaletteTable(fd, tag.BluePaletteColorLookupTableData, []byte{1, 2, 3, 4})
		return fd
	}

	t.Run("no descriptor", func(t *testing.T) {
		if _, err := newLUTDataset().colorLUT(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("supplemental palette", func(t *testing.T) {
		fd := withTables()
		fd.Set(NewDataElement(tag.PixelPresentation, VRCS, "MIXED"))
		if _, err := fd.colorLUT(); err == nil {
			t.Fatal("expected unsupported module error")
		}
	})
	t.Run("nominal depth not a multiple of eight", func(t *testing.T) {
		if _, err := newPaletteDataset([]int{4, 0, 12}, 12).colorLUT(); err == nil {
			t.Fatal("expected depth error")
		}
	})
	t.Run("data length implies an invalid depth", func(t *testing.T) {
		fd := newPaletteDataset([]int{4, 0, 8}, 8)
		setPaletteTable(fd, tag.RedPaletteColorLookupTableData, []byte{1, 2, 3})
		setPaletteTable(fd, tag.GreenPaletteColorLookupTableData, []byte{1, 2, 3})
		setPaletteTable(fd, tag.BluePaletteColorLookupTableData, []byte{1, 2, 3})
		if _, err := fd.colorLUT(); err == nil {
			t.Fatal("expected bit depth error")
		}
	})
	t.Run("missing green table", func(t *testing.T) {
		fd := newPaletteDataset([]int{4, 0, 8}, 8)
		setPaletteTable(fd, tag.RedPaletteColorLookupTableData, []byte{1, 2, 3, 4})
		if _, err := fd.colorLUT(); err == nil {
			t.Fatal("expected module-missing error")
		}
	})
	t.Run("mismatched table lengths", func(t *testing.T) {
		fd := withTables()
		setPaletteTable(fd, tag.GreenPaletteColorLookupTableData, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		if _, err := fd.colorLUT(); err == nil {
			t.Fatal("expected length mismatch error")
		}
	})
	t.Run("no palette data at all", func(t *testing.T) {
		if _, err := newPaletteDataset([]int{4, 0, 8}, 8).colorLUT(); err == nil {
			t.Fatal("expected module-missing error")
		}
	})
	t.Run("segmented data with a bad depth", func(t *testing.T) {
		fd := newPaletteDataset([]int{4, 0, 8}, 8)
		seg := []byte{0, 4, 1, 2, 3, 4}
		setPaletteTable(fd, tag.SegmentedRedPaletteColorLookupTableData, seg)
		setPaletteTable(fd, tag.SegmentedGreenPaletteColorLookupTableData, seg)
		setPaletteTable(fd, tag.SegmentedBluePaletteColorLookupTableData, seg)
		fd.Set(NewDataElement(tag.RedPaletteColorLookupTableDescriptor, VRUS, []int{4, 0, 6}))
		if _, err := fd.colorLUT(); err == nil {
			t.Fatal("expected depth error")
		}
	})
}

func TestPaletteBytesRejectsMissingOrEmpty(t *testing.T) {
	fd := newLUTDataset()
	if _, ok := fd.paletteBytes(tag.RedPaletteColorLookupTableData); ok {
		t.Fatal("absent element should not be found")
	}
	fd.Set(NewDataElement(tag.RedPaletteColorLookupTableData, VROW, []byte{}))
	if _, ok := fd.paletteBytes(tag.RedPaletteColorLookupTableData); ok {
		t.Fatal("empty value should not be found")
	}
	fd.Set(NewDataElement(tag.RedPaletteColorLookupTableData, VRUS, "not bytes"))
	if _, ok := fd.paletteBytes(tag.RedPaletteColorLookupTableData); ok {
		t.Fatal("non-byte value should not be found")
	}
}

func TestUnpackPaletteBytes(t *testing.T) {
	got := unpackPaletteBytes([]byte{1, 2, 3}, 1)
	if len(got) != 3 || got[2] != 3 {
		t.Fatalf("8-bit: %v", got)
	}
	got = unpackPaletteBytes([]byte{0x01, 0x02, 0x03, 0x04}, 2)
	if len(got) != 2 || got[0] != 0x0201 || got[1] != 0x0403 {
		t.Fatalf("16-bit: %v", got)
	}
}

func TestUnpackSegmentWords(t *testing.T) {
	if _, err := unpackSegmentWords([]byte{1, 2, 3}, 16, true); err == nil {
		t.Fatal("expected odd-length error")
	}
	got, err := unpackSegmentWords([]byte{0x01, 0x02}, 16, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 0x0201 {
		t.Fatalf("little endian: %v", got)
	}
	got, err = unpackSegmentWords([]byte{0x01, 0x02}, 16, false)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 0x0102 {
		t.Fatalf("big endian: %v", got)
	}
	got, err = unpackSegmentWords([]byte{9, 8}, 8, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1] != 8 {
		t.Fatalf("8-bit: %v", got)
	}
}

func TestSegmentEndianFallbacks(t *testing.T) {
	// Without file meta the dataset falls back on the encoding recorded at parse
	// time, and refuses when there is neither.
	fd := &FileDataset{Dataset: NewDataset()}
	little, err := fd.segmentEndian()
	if err != nil {
		t.Fatal(err)
	}
	if !little {
		t.Fatal("default encoding should be little endian")
	}
	if _, err := (&FileDataset{}).segmentEndian(); err == nil {
		t.Fatal("expected undetermined-endianness error")
	}
}

func TestApplyColorLUTFromDatasetNeedsBothTables(t *testing.T) {
	// colorLUT is the only place a dataset's alpha table can enter: it must come
	// out as a fourth channel and stay in descriptor order.
	fd := newPaletteDataset([]int{3, 0, 8}, 8)
	setPaletteTable(fd, tag.RedPaletteColorLookupTableData, []byte{10, 20, 30})
	setPaletteTable(fd, tag.GreenPaletteColorLookupTableData, []byte{11, 21, 31})
	setPaletteTable(fd, tag.BluePaletteColorLookupTableData, []byte{12, 22, 32})
	setPaletteTable(fd, tag.AlphaPaletteColorLookupTableData, []byte{0, 128, 255})
	lut, err := fd.colorLUT()
	if err != nil {
		t.Fatal(err)
	}
	if len(lut.Tables) != 4 || lut.Tables[3][2] != 255 {
		t.Fatalf("got %+v", lut)
	}
}
