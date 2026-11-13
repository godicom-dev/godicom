// Package waveforms reads the Waveform module of PS3.3 C.10.9: the
// multiplex groups of a (5400,0100) Waveform Sequence as arrays of samples.
//
// It is pydicom's pydicom.waveforms, which is one handler: multiplex_array
// and the generate_multiplex iterator over it.
package waveforms

import (
	"errors"
	"fmt"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/tag"
)

// ErrNoWaveformSequence is returned for a dataset without the
// (5400,0100) Waveform Sequence.
var ErrNoWaveformSequence = errors.New("godicom/waveforms: no (5400,0100) Waveform Sequence element found in the dataset")

// ErrMissingElements is returned for a multiplex group that lacks one or more
// of the elements the Waveform module requires.
var ErrMissingElements = errors.New("godicom/waveforms: required elements are missing from the sequence item")

// ErrUnsupportedSampleEncoding is returned for a Waveform Bits Allocated and
// Waveform Sample Interpretation pair the module does not define.
var ErrUnsupportedSampleEncoding = errors.New("godicom/waveforms: unsupported Waveform Bits Allocated and Waveform Sample Interpretation pair")

// ChannelSensitivity is one channel definition's conversion from the raw
// unitless samples to the quantity the Channel Sensitivity names:
//
//	value * Sensitivity * Correction + Baseline
type ChannelSensitivity struct {
	Sensitivity float64
	Baseline    float64
	Correction  float64
}

// Multiplex is one multiplex group: Samples per channel, Channels, and the
// samples laid out sample-major, (C1S1, C2S1, ..., CnS1, C1S2, ...), which is
// the order PS3.5 C.9.2 puts them in the Waveform Data.
//
// The raw samples are integers of one to eight bytes; RawAt returns them
// exactly. At returns them as float64, which is exact to 2 to the 53rd and so
// exact for every encoding but the unsigned 64-bit one above that.
type Multiplex struct {
	Samples  int // NumberOfWaveformSamples, one channel's length
	Channels int // NumberOfWaveformChannels

	raw           []int64
	values        []float64 // set by WithChannelSensitivity, nil before it
	sensitivities []ChannelSensitivity
}

// RawAt returns the raw sample of one channel at one sample position. The
// indexes are unchecked and panic like a slice index out of range.
func (m *Multiplex) RawAt(sample, channel int) int64 {
	return m.raw[sample*m.Channels+channel]
}

// At returns the sample as a float64: the value WithChannelSensitivity put
// there, or the raw sample converted when it was not called.
func (m *Multiplex) At(sample, channel int) float64 {
	if m.values != nil {
		return m.values[sample*m.Channels+channel]
	}
	return float64(m.raw[sample*m.Channels+channel])
}

// Sample returns one sample position across every channel.
func (m *Multiplex) Sample(i int) []float64 {
	out := make([]float64, m.Channels)
	for c := range out {
		out[c] = m.At(i, c)
	}
	return out
}

// WithChannelSensitivity returns a copy of the multiplex whose samples have
// been converted by each channel definition's Channel Sensitivity, Channel
// Sensitivity Correction Factor and Channel Baseline, as pydicom's
// as_raw=False does.
//
// A channel without a definition keeps its raw value, which is what pydicom's
// loop over the definitions leaves behind when the sequence is shorter than
// the channel count. The source values are the raw samples, so calling it
// twice is the same as calling it once.
func (m *Multiplex) WithChannelSensitivity() (*Multiplex, error) {
	out := &Multiplex{
		Samples:  m.Samples,
		Channels: m.Channels,
		raw:      m.raw,
		values:   make([]float64, len(m.raw)),
	}
	for s := 0; s < m.Samples; s++ {
		for c := 0; c < m.Channels; c++ {
			value := float64(m.raw[s*m.Channels+c])
			if c < len(m.sensitivities) {
				ch := m.sensitivities[c]
				value = value*ch.Sensitivity*ch.Correction + ch.Baseline
			}
			out.values[s*m.Channels+c] = value
		}
	}
	return out, nil
}

// requiredElements are the elements of a multiplex group the reader cannot
// do without, in the order pydicom lists them when one is missing.
var requiredElements = []struct {
	keyword string
	t       tag.Tag
}{
	{"NumberOfWaveformChannels", tag.NumberOfWaveformChannels},
	{"NumberOfWaveformSamples", tag.NumberOfWaveformSamples},
	{"WaveformBitsAllocated", tag.WaveformBitsAllocated},
	{"WaveformSampleInterpretation", tag.WaveformSampleInterpretation},
	{"WaveformData", tag.WaveformData},
}

// Multiplexes returns every multiplex group of the dataset's Waveform
// Sequence, in the order the sequence holds them.
func Multiplexes(ds *godicom.Dataset) ([]*Multiplex, error) {
	seq, ok := ds.GetSequence(tag.WaveformSequence)
	if !ok {
		return nil, ErrNoWaveformSequence
	}

	out := make([]*Multiplex, 0, seq.Len())
	for i := 0; i < seq.Len(); i++ {
		m, err := multiplex(seq.Get(i), i)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// MultiplexArray returns the multiplex group at index of the dataset's
// Waveform Sequence.
func MultiplexArray(ds *godicom.Dataset, index int) (*Multiplex, error) {
	seq, ok := ds.GetSequence(tag.WaveformSequence)
	if !ok {
		return nil, ErrNoWaveformSequence
	}
	if index < 0 || index >= seq.Len() {
		return nil, fmt.Errorf("godicom/waveforms: multiplex group index %d out of range, the sequence has %d", index, seq.Len())
	}
	return multiplex(seq.Get(index), index)
}

// multiplex reads one item of the Waveform Sequence.
func multiplex(item *godicom.Dataset, index int) (*Multiplex, error) {
	var missing []string
	for _, req := range requiredElements {
		if _, ok := item.Get(req.t); !ok {
			missing = append(missing, req.keyword)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w with index %d: %v", ErrMissingElements, index, missing)
	}

	channels, _ := item.GetInt(tag.NumberOfWaveformChannels)
	samples, _ := item.GetInt(tag.NumberOfWaveformSamples)
	bits, _ := item.GetInt(tag.WaveformBitsAllocated)
	interpretation, _ := item.GetString(tag.WaveformSampleInterpretation)

	if channels <= 0 || samples <= 0 {
		return nil, fmt.Errorf("godicom/waveforms: multiplex group with index %d has %d channels and %d samples, both have to be positive", index, channels, samples)
	}

	width, signed, err := sampleEncoding(bits, interpretation)
	if err != nil {
		return nil, fmt.Errorf("godicom/waveforms: multiplex group with index %d: %w", index, err)
	}

	data, _ := item.GetBytes(tag.WaveformData)
	expected := samples * channels * (bits / 8)
	if len(data) < expected {
		return nil, fmt.Errorf("godicom/waveforms: multiplex group with index %d has %d bytes of Waveform Data, want %d for %d samples of %d channels at %d bits",
			index, len(data), expected, samples, channels, bits)
	}

	m := &Multiplex{
		Samples:  samples,
		Channels: channels,
		raw:      make([]int64, samples*channels),
	}

	// Waveform Data is little endian. pydicom's handler hardcodes the
	// little endian dtypes, and so does this; a big endian transfer syntax
	// reads as pydicom reads it.
	for i := range m.raw {
		m.raw[i] = decodeSample(data[i*(bits/8):], width, signed)
	}

	// The channel definitions the conversion to a quantity comes from, in the
	// order pydicom walks them: ChannelSensitivity 1.0, correction 1.0 and
	// baseline 0.0 when a definition does not carry them.
	if defs, ok := item.GetSequence(tag.ChannelDefinitionSequence); ok {
		m.sensitivities = make([]ChannelSensitivity, defs.Len())
		for i := 0; i < defs.Len(); i++ {
			ch := defs.Get(i)
			s := ChannelSensitivity{Sensitivity: 1.0, Correction: 1.0}
			if v, ok := ch.GetFloat(tag.ChannelSensitivity); ok {
				s.Sensitivity = v
			}
			if v, ok := ch.GetFloat(tag.ChannelSensitivityCorrectionFactor); ok {
				s.Correction = v
			}
			if v, ok := ch.GetFloat(tag.ChannelBaseline); ok {
				s.Baseline = v
			}
			m.sensitivities[i] = s
		}
	}

	return m, nil
}

// sampleEncoding maps a Waveform Bits Allocated and Waveform Sample
// Interpretation pair to the byte width and the signedness of the samples.
func sampleEncoding(bits int, interpretation string) (width int, signed bool, err error) {
	switch bits {
	case 8:
		switch interpretation {
		case "SB":
			return 1, true, nil
		case "UB", "MB", "AB":
			// MB and AB are the modified and the alternate two's complement
			// encodings of cardiology; both come out unsigned, which is the
			// uint8 pydicom's table gives them.
			return 1, false, nil
		}
	case 16:
		switch interpretation {
		case "SS":
			return 2, true, nil
		case "US":
			return 2, false, nil
		}
	case 32:
		switch interpretation {
		case "SL":
			return 4, true, nil
		case "UL":
			return 4, false, nil
		}
	case 64:
		switch interpretation {
		case "SV":
			return 8, true, nil
		case "UV":
			return 8, false, nil
		}
	}
	return 0, false, fmt.Errorf("%w: %d bits, %q", ErrUnsupportedSampleEncoding, bits, interpretation)
}

// decodeSample reads the little endian sample at the start of b. Unsigned
// 64-bit samples above 2 to the 63rd come back as their two's complement,
// which no other sample encoding can produce and a waveform has no use for.
func decodeSample(b []byte, width int, signed bool) int64 {
	var v uint64
	for i := width - 1; i >= 0; i-- {
		v = v<<8 | uint64(b[i])
	}
	if signed {
		switch width {
		case 1:
			return int64(int8(v))
		case 2:
			return int64(int16(v))
		case 4:
			return int64(int32(v))
		default:
			return int64(v)
		}
	}
	return int64(v)
}

// Exact reports whether the float64 conversion At makes of the raw samples is
// lossless. It is, for everything but the unsigned 64-bit encoding with a
// value above 2 to the 53rd.
func (m *Multiplex) Exact() bool {
	for _, v := range m.raw {
		if v > 1<<53 || v < -(1<<53) {
			return false
		}
	}
	return true
}
