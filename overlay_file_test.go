package godicom_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

func TestOverlayArraySiemens(t *testing.T) {
	ds := readOverlay(t, nil)
	if ts, ok := ds.TransferSyntaxUID(); !ok || ts != uid.ExplicitVRLittleEndian {
		t.Fatalf("transfer syntax = %s", ts)
	}
	bits, ok := ds.GetInt(overlayTag(0x6000, 0x0100))
	if !ok || bits != 1 {
		t.Fatalf("OverlayBitsAllocated = %d", bits)
	}
	ov, err := ds.OverlayArray(0x6000)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Rows != 484 || ov.Columns != 484 || ov.Frames != 1 {
		t.Fatalf("shape = %d x %d x %d", ov.Frames, ov.Rows, ov.Columns)
	}
	if maxByte(ov.Samples) != 1 || minByte(ov.Samples) != 0 {
		t.Fatal("samples are not a 0/1 mask")
	}
	var sum int
	for col := 393; col < 422; col++ {
		sum += int(ov.At(0, 422, col))
	}
	if sum != 29 {
		t.Fatalf("row 422 cols 393:422 sum = %d, want 29", sum)
	}
	if ov.At(0, 0, 0) != 0 {
		t.Fatal("origin bit is set")
	}
	ov.Samples[0] = 1
	if ov.At(0, 0, 0) != 1 {
		t.Fatal("overlay samples are not writable")
	}

	rows, _ := ds.GetInt(overlayTag(0x6000, 0x0010))
	cols, _ := ds.GetInt(overlayTag(0x6000, 0x0011))
	frames, _ := ds.GetInt(overlayTag(0x6000, 0x0015))
	raw, _ := ds.GetBytes(overlayTag(0x6000, 0x3000))
	size := rows * cols * frames / 8
	if len(raw) != size+size%2 {
		t.Fatalf("overlay bytes = %d, want %d", len(raw), size+size%2)
	}
}

func TestOverlayArrayNoFrames(t *testing.T) {
	ds := readOverlay(t, nil)
	ds.Delete(overlayTag(0x6000, 0x0015))
	ov, err := ds.OverlayArray(0x6000)
	if err != nil {
		t.Fatal(err)
	}
	var sum int
	for col := 393; col < 422; col++ {
		sum += int(ov.At(0, 422, col))
	}
	if sum != 29 || ov.Frames != 1 {
		t.Fatalf("sum = %d frames = %d", sum, ov.Frames)
	}
}

func TestOverlayArrayBadGroup(t *testing.T) {
	ds := readOverlay(t, nil)
	for _, group := range []int{0x5FFF, 0x6100} {
		_, err := ds.OverlayArray(group)
		if err == nil || !strings.Contains(err.Error(), "between 0x6000 and 0x60FF") {
			t.Fatalf("group %04X err = %v", group, err)
		}
	}
}

func TestOverlayArrayMissingData(t *testing.T) {
	ds := readOverlay(t, nil)
	ds.Delete(overlayTag(0x6000, 0x3000))
	_, err := ds.OverlayArray(0x6000)
	if err == nil || !strings.Contains(err.Error(), "dataset: OverlayData") {
		t.Fatalf("err = %v", err)
	}
}

func TestOverlayArrayBadLength(t *testing.T) {
	ds := readOverlay(t, nil)
	raw, _ := ds.GetBytes(overlayTag(0x6000, 0x3000))
	ds.Set(godicom.NewDataElement(overlayTag(0x6000, 0x3000), godicom.VROW, raw[:len(raw)-1]))
	_, err := ds.OverlayArray(0x6000)
	if err == nil || !strings.Contains(err.Error(), "29281 bytes") || !strings.Contains(err.Error(), "29282 bytes") {
		t.Fatalf("err = %v", err)
	}
}

func TestOverlayArrayMissingPadding(t *testing.T) {
	var buf bytes.Buffer
	ds := readOverlay(t, &buf)
	ds.Set(godicom.NewDataElement(overlayTag(0x6000, 0x0010), godicom.VRUS, 15))
	ds.Set(godicom.NewDataElement(overlayTag(0x6000, 0x0011), godicom.VRUS, 14))
	raw, _ := ds.GetBytes(overlayTag(0x6000, 0x3000))
	ds.Set(godicom.NewDataElement(overlayTag(0x6000, 0x3000), godicom.VROW, raw[:27]))
	if _, err := ds.OverlayArray(0x6000); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "odd and misses a padding byte") {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestOverlayArrayExcessPadding(t *testing.T) {
	var buf bytes.Buffer
	ds := readOverlay(t, &buf)
	ds.Set(godicom.NewDataElement(overlayTag(0x6000, 0x0010), godicom.VRUS, 15))
	ds.Set(godicom.NewDataElement(overlayTag(0x6000, 0x0011), godicom.VRUS, 14))
	raw, _ := ds.GetBytes(overlayTag(0x6000, 0x3000))
	padded := append(append([]byte{}, raw[:27]...), 0, 0, 0)
	ds.Set(godicom.NewDataElement(overlayTag(0x6000, 0x3000), godicom.VROW, padded))
	if _, err := ds.OverlayArray(0x6000); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "30 bytes") || !strings.Contains(buf.String(), "3 bytes will be removed") {
		t.Fatalf("log = %q", buf.String())
	}
}

func readOverlay(t *testing.T, log *bytes.Buffer) *godicom.FileDataset {
	t.Helper()
	opts := &godicom.ReadOptions{}
	if log != nil {
		opts.Logger = slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}
	ds, err := godicom.ReadFile(testFilePath("MR-SIEMENS-DICOM-WithOverlays.dcm"), opts)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func overlayTag(group, element int) godicom.Tag {
	return tag.Tag(uint32(group)<<16 | uint32(element))
}

func maxByte(b []byte) byte {
	var m byte
	for _, v := range b {
		if v > m {
			m = v
		}
	}
	return m
}

func minByte(b []byte) byte {
	if len(b) == 0 {
		return 0
	}
	m := b[0]
	for _, v := range b[1:] {
		if v < m {
			m = v
		}
	}
	return m
}
