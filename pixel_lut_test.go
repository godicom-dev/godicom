package godicom

import (
	"testing"

	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// newLUTDataset builds the smallest file dataset that can satisfy the display
// pipeline: a complete pixel descriptor plus a file meta transfer syntax.
func newLUTDataset() *FileDataset {
	fd := &FileDataset{Dataset: NewDataset(), FileMeta: NewFileMetaDataset()}
	fd.FileMeta.Set(NewDataElement(tag.TransferSyntaxUID, VRUI, string(uid.ExplicitVRLittleEndian)))
	fd.Set(NewDataElement(tag.Rows, VRUS, 2))
	fd.Set(NewDataElement(tag.Columns, VRUS, 2))
	fd.Set(NewDataElement(tag.SamplesPerPixel, VRUS, 1))
	fd.Set(NewDataElement(tag.BitsAllocated, VRUS, 16))
	fd.Set(NewDataElement(tag.BitsStored, VRUS, 16))
	fd.Set(NewDataElement(tag.PixelRepresentation, VRUS, 0))
	fd.Set(NewDataElement(tag.PhotometricInterpretation, VRCS, "MONOCHROME2"))
	return fd
}

func lutItem(descriptor interface{}, data interface{}) *Dataset {
	item := NewDataset()
	item.Set(NewDataElement(tag.LUTDescriptor, VRUS, descriptor))
	if data != nil {
		item.Set(NewDataElement(tag.LUTData, VROW, data))
	}
	return item
}

func TestReadLUTFromSequenceSixteenBit(t *testing.T) {
	fd := newLUTDataset()
	fd.SetSequence(tag.ModalityLUTSequence, NewSequence([]*Dataset{
		lutItem([]int{4, 0, 16}, []byte{1, 0, 2, 0, 3, 0, 4, 0}),
	}))
	lut, ok, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("LUT not found")
	}
	if lut.FirstMap != 0 || lut.OutputBits != 16 {
		t.Fatalf("got %+v", lut)
	}
	want := []uint16{1, 2, 3, 4}
	if len(lut.Entries) != len(want) {
		t.Fatalf("entries = %v, want %v", lut.Entries, want)
	}
	for i := range want {
		if lut.Entries[i] != want[i] {
			t.Fatalf("entries = %v, want %v", lut.Entries, want)
		}
	}
}

func TestReadLUTFromSequenceEightBitInOW(t *testing.T) {
	// Some writers put 8-bit entries in an OW value without zero-extension, so
	// the value is half the length the descriptor implies. Reading that as
	// 16-bit would halve the table and shift every entry.
	fd := newLUTDataset()
	fd.SetSequence(tag.ModalityLUTSequence, NewSequence([]*Dataset{
		lutItem([]int{4, 0, 8}, []byte{10, 20, 30, 40}),
	}))
	lut, ok, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("LUT not found")
	}
	if len(lut.Entries) != 4 || lut.Entries[3] != 40 {
		t.Fatalf("entries = %v", lut.Entries)
	}
}

func TestReadLUTFromSequenceMultiValueDescriptor(t *testing.T) {
	// A descriptor read from a real file arrives as a typed MultiValue, not a
	// plain []int; both spellings have to work.
	fd := newLUTDataset()
	fd.SetSequence(tag.ModalityLUTSequence, NewSequence([]*Dataset{
		lutItem(NewMultiValue([]int{2, 100, 16}), []byte{7, 0, 8, 0}),
	}))
	lut, ok, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || lut.FirstMap != 100 || len(lut.Entries) != 2 {
		t.Fatalf("got %+v ok=%v", lut, ok)
	}

	mixed := newLUTDataset()
	mixed.SetSequence(tag.ModalityLUTSequence, NewSequence([]*Dataset{
		lutItem(NewMultiValue([]interface{}{2, int64(0), uint16(16)}), NewMultiValue([]uint16{1, 2})),
	}))
	lut, ok, err = mixed.readLUTFromSequence(tag.ModalityLUTSequence, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(lut.Entries) != 2 || lut.Entries[1] != 2 {
		t.Fatalf("got %+v ok=%v", lut, ok)
	}
}

func TestReadLUTFromSequenceMissingPieces(t *testing.T) {
	cases := []struct {
		name string
		item *Dataset
	}{
		{"no descriptor", func() *Dataset {
			item := NewDataset()
			item.Set(NewDataElement(tag.LUTData, VROW, []byte{1, 0}))
			return item
		}()},
		{"no data", lutItem([]int{1, 0, 16}, nil)},
		{"short descriptor", lutItem([]int{1, 0}, []byte{1, 0})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fd := newLUTDataset()
			fd.SetSequence(tag.ModalityLUTSequence, NewSequence([]*Dataset{tc.item}))
			_, ok, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, 0)
			if tc.name == "short descriptor" {
				if err == nil {
					t.Fatal("expected invalid descriptor error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				t.Fatal("should report no LUT")
			}
		})
	}
}

func TestReadLUTFromSequenceEmptyAndOutOfRange(t *testing.T) {
	fd := newLUTDataset()
	if _, ok, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, 0); err != nil || ok {
		t.Fatalf("absent sequence: ok=%v err=%v", ok, err)
	}
	fd.SetSequence(tag.ModalityLUTSequence, NewSequence(nil))
	if _, ok, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, 0); err != nil || ok {
		t.Fatalf("empty sequence: ok=%v err=%v", ok, err)
	}
	fd.SetSequence(tag.ModalityLUTSequence, NewSequence([]*Dataset{lutItem([]int{1, 0, 16}, []byte{1, 0})}))
	if _, _, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, 3); err == nil {
		t.Fatal("expected index out of range error")
	}
	if _, _, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, -1); err == nil {
		t.Fatal("expected negative index error")
	}
}

func TestReadLUTFromSequenceZeroEntryCountMeansFullTable(t *testing.T) {
	// A zero entry count in the descriptor means 2^16 entries per PS3.6. A value
	// that cannot possibly hold them must be rejected rather than read as empty.
	fd := newLUTDataset()
	fd.SetSequence(tag.ModalityLUTSequence, NewSequence([]*Dataset{
		lutItem([]int{0, 0, 16}, []byte{1, 0, 2, 0}),
	}))
	if _, _, err := fd.readLUTFromSequence(tag.ModalityLUTSequence, 0); err == nil {
		t.Fatal("expected LUT Data too short error")
	}
}

func TestLUTDescriptorParts(t *testing.T) {
	if n, f, d, ok := lutDescriptorParts([]int{4, 5, 16}); !ok || n != 4 || f != 5 || d != 16 {
		t.Fatalf("[]int: %d %d %d %v", n, f, d, ok)
	}
	if n, _, _, ok := lutDescriptorParts(NewMultiValue([]int{4, 5, 16})); !ok || n != 4 {
		t.Fatalf("MultiValue[int]: %d %v", n, ok)
	}
	if n, f, d, ok := lutDescriptorParts(NewMultiValue([]interface{}{4, int64(5), int16(16)})); !ok || n != 4 || f != 5 || d != 16 {
		t.Fatalf("MultiValue[interface{}]: %d %d %d %v", n, f, d, ok)
	}
	for _, bad := range []interface{}{
		[]int{4, 5},
		NewMultiValue([]int{4, 5}),
		NewMultiValue([]interface{}{4, "x", 16}),
		"nope",
		nil,
	} {
		if _, _, _, ok := lutDescriptorParts(bad); ok {
			t.Fatalf("%#v should not parse", bad)
		}
	}
}

func TestAsInt(t *testing.T) {
	for _, v := range []interface{}{int(1), int64(1), uint16(1), int16(1)} {
		if got, ok := asInt(v); !ok || got != 1 {
			t.Fatalf("%#v -> %d %v", v, got, ok)
		}
	}
	for _, v := range []interface{}{"1", uint32(1), nil, 1.0} {
		if _, ok := asInt(v); ok {
			t.Fatalf("%#v should not convert", v)
		}
	}
}

func TestUnpackLUTDataTypes(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		want  []uint16
	}{
		{"int slice", []int{1, 2}, []uint16{1, 2}},
		{"int multivalue", NewMultiValue([]int{3, 4}), []uint16{3, 4}},
		{"uint16 slice", []uint16{5, 6}, []uint16{5, 6}},
		{"uint16 multivalue", NewMultiValue([]uint16{7, 8}), []uint16{7, 8}},
		{"ow bytes", []byte{9, 0, 10, 0}, []uint16{9, 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := unpackLUTData(NewDataElement(tag.LUTData, VROW, tc.value), len(tc.want), true)
			if err != nil {
				t.Fatal(err)
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
	if _, err := unpackLUTData(NewDataElement(tag.LUTData, VROW, "text"), 1, true); err == nil {
		t.Fatal("expected unsupported type error")
	}
}

func TestLUTEntriesFromOWByteOrders(t *testing.T) {
	le := []byte{0x01, 0x02, 0x03, 0x04}
	got, err := lutEntriesFromOW(le, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 0x0201 || got[1] != 0x0403 {
		t.Fatalf("little endian = %v", got)
	}
	got, err = lutEntriesFromOW(le, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 0x0102 || got[1] != 0x0304 {
		t.Fatalf("big endian = %v", got)
	}
	if _, err := lutEntriesFromOW([]byte{1}, 2, true); err == nil {
		t.Fatal("expected too-short error")
	}
}

func TestApplyModalityLUTRescalePath(t *testing.T) {
	fd := newLUTDataset()
	fd.Set(NewDataElement(tag.RescaleSlope, VRDS, "2"))
	fd.Set(NewDataElement(tag.RescaleIntercept, VRDS, "-10"))
	out, err := fd.ApplyModalityLUT([]float64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != -8 || out[1] != -6 {
		t.Fatalf("got %v, want [-8 -6]", out)
	}
}

func TestApplyModalityLUTSequenceWins(t *testing.T) {
	fd := newLUTDataset()
	fd.SetSequence(tag.ModalityLUTSequence, NewSequence([]*Dataset{
		lutItem([]int{2, 0, 16}, []byte{50, 0, 60, 0}),
	}))
	fd.Set(NewDataElement(tag.RescaleSlope, VRDS, "2"))
	fd.Set(NewDataElement(tag.RescaleIntercept, VRDS, "0"))
	out, err := fd.ApplyModalityLUT([]float64{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 50 || out[1] != 60 {
		t.Fatalf("got %v, want the LUT values", out)
	}
	// windowConfig must report the same precedence, since the output range of a
	// windowing operation depends on which modality step actually ran.
	cfg, err := fd.windowConfig(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HasModalityLUT || cfg.HasRescale || cfg.ModalityLUTOutputBits != 16 {
		t.Fatalf("got %+v", cfg)
	}
}

func TestApplyVOILUTWindowingPath(t *testing.T) {
	fd := newLUTDataset()
	fd.Set(NewDataElement(tag.WindowCenter, VRDS, "128"))
	fd.Set(NewDataElement(tag.WindowWidth, VRDS, "256"))
	out, err := fd.ApplyVOILUT([]float64{0, 128, 255}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 0 {
		t.Fatalf("below the window = %v, want 0", out[0])
	}
	if out[2] != 65535 {
		t.Fatalf("above the window = %v, want 65535 for 16 stored bits", out[2])
	}
	if out[1] <= 0 || out[1] >= 65535 {
		t.Fatalf("inside the window = %v, want an intermediate value", out[1])
	}
}

func TestApplyVOILUTWindowIndexOutOfRange(t *testing.T) {
	fd := newLUTDataset()
	fd.Set(NewDataElement(tag.WindowCenter, VRDS, "128"))
	fd.Set(NewDataElement(tag.WindowWidth, VRDS, "256"))
	if _, err := fd.ApplyVOILUT([]float64{0}, 1, false); err == nil {
		t.Fatal("expected window index out of range error")
	}
}

func TestApplyVOILUTSequencePath(t *testing.T) {
	fd := newLUTDataset()
	fd.SetSequence(tag.VOILUTSequence, NewSequence([]*Dataset{
		lutItem([]int{3, 0, 16}, []byte{5, 0, 6, 0, 7, 0}),
	}))
	out, err := fd.ApplyVOILUT([]float64{0, 2}, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 5 || out[1] != 7 {
		t.Fatalf("got %v, want [5 7]", out)
	}
}

func TestApplyVOILUTFunctionFromDataset(t *testing.T) {
	fd := newLUTDataset()
	fd.Set(NewDataElement(tag.WindowCenter, VRDS, "128"))
	fd.Set(NewDataElement(tag.WindowWidth, VRDS, "256"))
	fd.Set(NewDataElement(tag.VOILUTFunction, VRCS, "SIGMOID"))
	out, err := fd.ApplyVOILUT([]float64{128}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	// Sigmoid is continuous: the centre maps to half the output range.
	if out[0] < 32700 || out[0] > 32830 {
		t.Fatalf("sigmoid centre = %v, want ~32767", out[0])
	}
}

func TestApplyPresentationLUTShapeFromDataset(t *testing.T) {
	fd := newLUTDataset()
	out, err := fd.ApplyPresentationLUTShape([]float64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 1 || out[1] != 2 {
		t.Fatalf("absent shape should copy, got %v", out)
	}
	fd.Set(NewDataElement(tag.PresentationLUTShape, VRCS, "INVERSE"))
	out, err = fd.ApplyPresentationLUTShape([]float64{1, 5})
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 4 || out[1] != 0 {
		t.Fatalf("inverse got %v, want [4 0]", out)
	}
}
