package godicom

import (
	"strings"
	"testing"
)

func TestOverlayExpectedLength(t *testing.T) {
	// (frames, rows, cols) -> bytes, pixels. From test_overlay_np.REFERENCE_LENGTH.
	cases := []struct {
		frames, rows, cols int
		bytes, pixels      int
	}{
		{1, 0, 0, 0, 0},
		{1, 1, 1, 1, 1},
		{1, 2, 2, 1, 4},
		{1, 2, 4, 1, 8},
		{1, 3, 3, 2, 9},
		{1, 512, 512, 32768, 262144},
		{3, 0, 0, 0, 0},
		{3, 1, 1, 1, 3},
		{3, 2, 4, 3, 24},
		{3, 2, 2, 2, 12},
		{3, 3, 3, 4, 27},
		{3, 512, 512, 98304, 786432},
		{4, 0, 0, 0, 0},
		{4, 1, 1, 1, 4},
		{4, 2, 4, 4, 32},
		{4, 2, 2, 2, 16},
		{4, 3, 3, 5, 36},
		{4, 512, 512, 131072, 1048576},
	}
	for _, tc := range cases {
		pixels := tc.frames * tc.rows * tc.cols
		got := overlayByteLength(pixels)
		if got != tc.bytes || pixels != tc.pixels {
			t.Fatalf("(%d,%d,%d) = %d bytes %d pixels, want %d %d", tc.frames, tc.rows, tc.cols, got, pixels, tc.bytes, tc.pixels)
		}
	}
}

func TestOverlayReshape(t *testing.T) {
	// 1 frame, 4x5, values packed LSB first would not match the pydicom
	// reshape fixture (those values are already unpacked). Check the layout
	// OverlayArray returns for two frames of 1x8.
	ds := NewDataset()
	group := 0x6000
	ds.Set(NewDataElement(overlayTag(group, 0x0010), VRUS, 1))
	ds.Set(NewDataElement(overlayTag(group, 0x0011), VRUS, 8))
	ds.Set(NewDataElement(overlayTag(group, 0x0015), VRIS, "2"))
	ds.Set(NewDataElement(overlayTag(group, 0x0100), VRUS, 1))
	// Frame 0: 0x01 = bit 0 set. Frame 1: 0x80 = bit 7 set.
	ds.Set(NewDataElement(overlayTag(group, 0x3000), VROB, []byte{0x01, 0x80}))

	ov, err := ds.OverlayArray(group)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Frames != 2 || ov.Rows != 1 || ov.Columns != 8 || len(ov.Samples) != 16 {
		t.Fatalf("shape frames=%d rows=%d cols=%d len=%d", ov.Frames, ov.Rows, ov.Columns, len(ov.Samples))
	}
	if ov.At(0, 0, 0) != 1 || ov.At(1, 0, 7) != 1 {
		t.Fatalf("bits = %v", ov.Samples)
	}
	for i, v := range ov.Samples {
		if i == 0 || i == 15 {
			continue
		}
		if v != 0 {
			t.Fatalf("sample %d = %d", i, v)
		}
	}
}

func TestOverlayArrayInvalidFrames(t *testing.T) {
	ds := NewDataset()
	group := 0x6000
	ds.Set(NewDataElement(overlayTag(group, 0x0010), VRUS, 4))
	ds.Set(NewDataElement(overlayTag(group, 0x0011), VRUS, 5))
	ds.Set(NewDataElement(overlayTag(group, 0x0015), VRIS, "0"))
	ds.Set(NewDataElement(overlayTag(group, 0x0100), VRUS, 1))
	ds.Set(NewDataElement(overlayTag(group, 0x3000), VROB, []byte{0}))
	_, err := ds.OverlayArray(group)
	if err == nil || !strings.Contains(err.Error(), "value of 0 for (60xx,0015)") {
		t.Fatalf("err = %v", err)
	}
}
