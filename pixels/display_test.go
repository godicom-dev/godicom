package pixels_test

import (
	"testing"

	"github.com/godicom-dev/godicom/pixels"
)

func TestPackDisplayU8_alreadyByteRange(t *testing.T) {
	in := []float64{0, 127.4, 255.6}
	got := pixels.PackDisplayU8(in)
	want := []byte{0, 127, 255}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%d want %d", i, got[i], want[i])
		}
	}
}

func TestPackDisplayU8_scalesWideRange(t *testing.T) {
	in := []float64{100, 200, 300}
	got := pixels.PackDisplayU8(in)
	if got[0] != 0 || got[2] != 255 {
		t.Fatalf("got %v, want scaled 0..255", got)
	}
}

func TestBuildArray_validatesLength(t *testing.T) {
	desc := pixels.Descriptor{Rows: 2, Columns: 2, SamplesPerPixel: 1}
	_, err := pixels.BuildArray([]float64{1, 2, 3}, desc, 1)
	if err == nil {
		t.Fatal("expected length error")
	}
}
