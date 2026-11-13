package waveforms_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/waveforms"
)

// fixtureDataset reads the ECG waveform the pydicom test suite uses.
func fixtureDataset(t *testing.T) *godicom.Dataset {
	t.Helper()
	ds, err := godicom.ReadFile("../pydicom/src/pydicom/data/test_files/waveform_ecg.dcm", nil)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return ds.Dataset
}

// TestWaveformECGMatchesPydicom reads the fixture pydicom's own waveform
// tests read, and checks the values against pydicom 3.1.0.dev0. Every raw
// sample and every channel-sensitivity-corrected sample of both multiplex
// groups agrees -- 134400 values, and a digest over the walk pins them, so a
// change to the decoding has to be re-checked against pydicom to be kept.
func TestWaveformECGMatchesPydicom(t *testing.T) {
	ds := fixtureDataset(t)

	groups, err := waveforms.Multiplexes(ds)
	if err != nil {
		t.Fatalf("Multiplexes: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("multiplex groups = %d, want 2", len(groups))
	}
	if groups[0].Samples != 10000 || groups[0].Channels != 12 {
		t.Fatalf("group 0 = %d x %d, want 10000 x 12", groups[0].Samples, groups[0].Channels)
	}
	if groups[1].Samples != 1200 || groups[1].Channels != 12 {
		t.Fatalf("group 1 = %d x %d, want 1200 x 12", groups[1].Samples, groups[1].Channels)
	}

	// pydicom's multiplex_array(ds, 0, as_raw=True)[:5, :2].
	want := [][]int64{{80, 90}, {65, 85}, {50, 80}, {35, 75}, {37, 77}}
	for s, row := range want {
		for c, v := range row {
			if got := groups[0].RawAt(s, c); got != v {
				t.Fatalf("RawAt(%d, %d) = %d, want %d", s, c, got, v)
			}
		}
	}

	var b strings.Builder
	for _, m := range groups {
		corrected, err := m.WithChannelSensitivity()
		if err != nil {
			t.Fatalf("WithChannelSensitivity: %v", err)
		}
		for s := 0; s < m.Samples; s++ {
			for c := 0; c < m.Channels; c++ {
				b.WriteString(strconv.FormatInt(m.RawAt(s, c), 10))
				b.WriteByte('|')
				b.WriteString(strconv.FormatFloat(corrected.At(s, c), 'f', -1, 64))
				b.WriteByte('\n')
			}
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	if got := fmt.Sprintf("%x", sum); got != "d3262f4b4e1535742291ddd40a43a811c680a5c140fbfb814f76594a10f725f6" {
		t.Fatalf("digest over raw and corrected samples = %s, want the one pydicom agrees with", got)
	}
}

// multiplexItem builds a Waveform Sequence item around the given data.
func multiplexItem(bits int, interpretation string, channels, samples int, data []byte, defs ...*godicom.Dataset) *godicom.Dataset {
	item := godicom.NewDataset()
	item.Set(godicom.NewDataElement(tag.NumberOfWaveformChannels, godicom.VRUS, uint16(channels)))
	item.Set(godicom.NewDataElement(tag.NumberOfWaveformSamples, godicom.VRUS, uint32(samples)))
	item.Set(godicom.NewDataElement(tag.WaveformBitsAllocated, godicom.VRUS, uint16(bits)))
	item.Set(godicom.NewDataElement(tag.WaveformSampleInterpretation, godicom.VRCS, interpretation))
	item.Set(godicom.NewDataElement(tag.WaveformData, godicom.VROB, data))
	if len(defs) > 0 {
		item.Set(godicom.NewDataElement(tag.ChannelDefinitionSequence, godicom.VRSQ, godicom.NewSequence(defs)))
	}
	return item
}

// datasetWith builds a dataset whose Waveform Sequence holds the items.
func datasetWith(items ...*godicom.Dataset) *godicom.Dataset {
	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(tag.WaveformSequence, godicom.VRSQ, godicom.NewSequence(items)))
	return ds
}

// TestEverySampleInterpretationDecodes walks the ten pairs the Waveform
// module defines, over bytes whose value reads back the same in all of them.
func TestEverySampleInterpretationDecodes(t *testing.T) {
	tests := []struct {
		bits   int
		interp string
		want   int64
	}{
		{8, "SB", -2},
		{8, "UB", 254},
		{8, "MB", 254},
		{8, "AB", 254},
		{16, "SS", -2},
		{16, "US", 65534},
		{32, "SL", -2},
		{32, "UL", 4294967294},
		{64, "SV", -2},
		// The unsigned 64-bit value comes back as its two's complement,
		// which is what the decode of a sample this size documents.
		{64, "UV", -2},
	}

	for _, test := range tests {
		// 0xFF...FE little endian, the two's complement of -2 at this width.
		data := make([]byte, test.bits/8)
		for i := range data {
			data[i] = 0xFF
		}
		data[0] = 0xFE

		ds := datasetWith(multiplexItem(test.bits, test.interp, 1, 1, data))
		groups, err := waveforms.Multiplexes(ds)
		if err != nil {
			t.Fatalf("%d bits %q: %v", test.bits, test.interp, err)
		}
		if got := groups[0].RawAt(0, 0); got != test.want {
			t.Errorf("%d bits %q: RawAt(0, 0) = %d, want %d", test.bits, test.interp, got, test.want)
		}
	}
}

// TestWaveformDataPaddingIsSliced covers the value longer than the samples
// and channels call for: the extra bytes are padding and are not samples.
func TestWaveformDataPaddingIsSliced(t *testing.T) {
	data := []byte{0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00}
	ds := datasetWith(multiplexItem(16, "US", 2, 1, data))

	groups, err := waveforms.Multiplexes(ds)
	if err != nil {
		t.Fatalf("Multiplexes: %v", err)
	}
	if got := groups[0].RawAt(0, 0); got != 1 {
		t.Fatalf("RawAt(0, 0) = %d, want 1", got)
	}
	if got := groups[0].RawAt(0, 1); got != 2 {
		t.Fatalf("RawAt(0, 1) = %d, want 2", got)
	}
}

// TestChannelDefinitionsShorterThanChannels pins what the conversion does
// when the definitions run out before the channels do: pydicom's loop over
// the definitions leaves the remaining channels raw, and so does this.
func TestChannelDefinitionsShorterThanChannels(t *testing.T) {
	def := godicom.NewDataset()
	def.Set(godicom.NewDataElement(tag.ChannelSensitivity, godicom.VRDS, "2"))
	item := multiplexItem(8, "UB", 2, 1, []byte{3, 5}, def)

	ds := datasetWith(item)
	groups, err := waveforms.Multiplexes(ds)
	if err != nil {
		t.Fatalf("Multiplexes: %v", err)
	}
	corrected, err := groups[0].WithChannelSensitivity()
	if err != nil {
		t.Fatalf("WithChannelSensitivity: %v", err)
	}
	if got := corrected.At(0, 0); got != 6 {
		t.Fatalf("At(0, 0) = %v, want 3 * 2", got)
	}
	if got := corrected.At(0, 1); got != 5 {
		t.Fatalf("At(0, 1) = %v, want the raw 5", got)
	}
}

// TestChannelDefinitionDefaults covers the elements a definition may leave
// out: sensitivity 1.0, correction 1.0, baseline 0.0.
func TestChannelDefinitionDefaults(t *testing.T) {
	def := godicom.NewDataset()
	item := multiplexItem(8, "UB", 1, 1, []byte{7}, def)

	ds := datasetWith(item)
	groups, err := waveforms.Multiplexes(ds)
	if err != nil {
		t.Fatalf("Multiplexes: %v", err)
	}
	corrected, err := groups[0].WithChannelSensitivity()
	if err != nil {
		t.Fatalf("WithChannelSensitivity: %v", err)
	}
	if got := corrected.At(0, 0); got != 7 {
		t.Fatalf("At(0, 0) = %v, want the raw 7", got)
	}
}

// TestWithChannelSensitivityIsRepeatable covers the source values the
// conversion works from: the raw samples, so calling it twice over the same
// multiplex does not compound.
func TestWithChannelSensitivityIsRepeatable(t *testing.T) {
	def := godicom.NewDataset()
	def.Set(godicom.NewDataElement(tag.ChannelSensitivity, godicom.VRDS, "2"))
	ds := datasetWith(multiplexItem(8, "UB", 1, 1, []byte{3}, def))

	groups, err := waveforms.Multiplexes(ds)
	if err != nil {
		t.Fatal(err)
	}
	first, err := groups[0].WithChannelSensitivity()
	if err != nil {
		t.Fatal(err)
	}
	second, err := groups[0].WithChannelSensitivity()
	if err != nil {
		t.Fatal(err)
	}
	if first.At(0, 0) != second.At(0, 0) {
		t.Fatalf("first = %v, second = %v", first.At(0, 0), second.At(0, 0))
	}
}

// TestMultiplexErrors walks the ways a Waveform Sequence can be wrong.
func TestMultiplexErrors(t *testing.T) {
	if _, err := waveforms.Multiplexes(godicom.NewDataset()); err != waveforms.ErrNoWaveformSequence {
		t.Fatalf("no sequence: err = %v", err)
	}
	if _, err := waveforms.MultiplexArray(godicom.NewDataset(), 0); err != waveforms.ErrNoWaveformSequence {
		t.Fatalf("no sequence: err = %v", err)
	}

	// A group missing every required element, named in the error.
	_, err := waveforms.Multiplexes(datasetWith(godicom.NewDataset()))
	if err == nil || !strings.Contains(err.Error(), "NumberOfWaveformChannels") {
		t.Fatalf("empty item: err = %v", err)
	}

	// A bits and interpretation pair the module does not define.
	_, err = waveforms.Multiplexes(datasetWith(multiplexItem(12, "SS", 1, 1, []byte{1, 2})))
	if !errors.Is(err, waveforms.ErrUnsupportedSampleEncoding) {
		t.Fatalf("12 bits: err = %v", err)
	}

	// Data shorter than the samples and channels call for.
	_, err = waveforms.Multiplexes(datasetWith(multiplexItem(16, "SS", 2, 2, []byte{1, 2})))
	if err == nil || !strings.Contains(err.Error(), "bytes of Waveform Data") {
		t.Fatalf("short data: err = %v", err)
	}

	// A non-positive channel or sample count.
	_, err = waveforms.Multiplexes(datasetWith(multiplexItem(8, "UB", 0, 1, nil)))
	if err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("zero channels: err = %v", err)
	}

	// An index past the sequence.
	ds := datasetWith(multiplexItem(8, "UB", 1, 1, []byte{1}))
	if _, err := waveforms.MultiplexArray(ds, 1); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("index 1 of one: err = %v", err)
	}
	if _, err := waveforms.MultiplexArray(ds, -1); err == nil {
		t.Fatal("index -1 was accepted")
	}
}
