package pixels

import (
	"fmt"

	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// DatasetSource provides dataset element access for pixel decode.
type DatasetSource interface {
	GetBytes(tag.Tag) ([]byte, bool)
	GetInt(tag.Tag) (int, bool)
	GetString(tag.Tag) (string, bool)
}

// FileSource is a DICOM file dataset with file meta transfer syntax.
type FileSource interface {
	DatasetSource
	TransferSyntaxUID() (string, bool)
}

// Descriptor holds image-related dataset attributes needed for pixel decode.
type Descriptor struct {
	TransferSyntaxUID         uid.UID
	Rows                      int
	Columns                   int
	SamplesPerPixel           int
	BitsAllocated             int
	BitsStored                int
	PixelRepresentation       int
	NumberOfFrames            int
	PhotometricInterpretation string
	PlanarConfiguration       int
	ExtendedOffsets           *ExtendedOffsets
}

// ExtendedOffsets holds optional extended offset table tags.
type ExtendedOffsets struct {
	Offsets []uint64
	Lengths []uint64
}

// BytesPerSample returns bytes per sample (1 for BitsAllocated 1).
func (d Descriptor) BytesPerSample() int {
	if d.BitsAllocated == 1 {
		return 1
	}
	return d.BitsAllocated / 8
}

// UnpackedFrameBytes returns decoded frame size in bytes for one frame.
func (d Descriptor) UnpackedFrameBytes() int {
	return d.Rows * d.Columns * d.SamplesPerPixel * d.BytesPerSample()
}

// DescriptorFromFile builds a Descriptor from a file dataset source.
func DescriptorFromFile(fd FileSource) (Descriptor, error) {
	ts, ok := fd.TransferSyntaxUID()
	if !ok || ts == "" {
		return Descriptor{}, fmt.Errorf("pixels: missing TransferSyntaxUID in file meta")
	}
	return DescriptorFromDataset(fd, uid.UID(ts))
}

// DescriptorFromDataset builds a Descriptor using an explicit transfer syntax UID.
func DescriptorFromDataset(ds DatasetSource, ts uid.UID) (Descriptor, error) {
	desc := Descriptor{TransferSyntaxUID: ts}

	var missing []string
	if v, ok := ds.GetInt(tag.Rows); ok {
		desc.Rows = v
	} else {
		missing = append(missing, "Rows")
	}
	if v, ok := ds.GetInt(tag.Columns); ok {
		desc.Columns = v
	} else {
		missing = append(missing, "Columns")
	}
	if v, ok := ds.GetInt(tag.SamplesPerPixel); ok {
		desc.SamplesPerPixel = v
	} else {
		missing = append(missing, "SamplesPerPixel")
	}
	if v, ok := ds.GetInt(tag.BitsAllocated); ok {
		desc.BitsAllocated = v
	} else {
		missing = append(missing, "BitsAllocated")
	}
	if v, ok := ds.GetInt(tag.BitsStored); ok {
		desc.BitsStored = v
	} else {
		desc.BitsStored = desc.BitsAllocated
	}
	if v, ok := ds.GetInt(tag.PixelRepresentation); ok {
		desc.PixelRepresentation = v
	} else {
		desc.PixelRepresentation = 0
	}
	if v, ok := ds.GetInt(tag.NumberOfFrames); ok && v > 0 {
		desc.NumberOfFrames = v
	} else {
		desc.NumberOfFrames = 1
	}
	if v, ok := ds.GetString(tag.PhotometricInterpretation); ok {
		desc.PhotometricInterpretation = v
	}
	if v, ok := ds.GetInt(tag.PlanarConfiguration); ok {
		desc.PlanarConfiguration = v
	}
	if len(missing) > 0 {
		return Descriptor{}, fmt.Errorf("pixels: missing required elements: %v", missing)
	}

	if off, ok := ds.GetBytes(tag.ExtendedOffsetTable); ok && len(off) > 0 {
		if lens, ok2 := ds.GetBytes(tag.ExtendedOffsetTableLengths); ok2 {
			desc.ExtendedOffsets = parseExtendedOffsets(off, lens)
		}
	}
	return desc, nil
}

func parseExtendedOffsets(offsetsRaw, lengthsRaw []byte) *ExtendedOffsets {
	n := len(offsetsRaw) / 8
	if n == 0 || len(lengthsRaw)/8 != n {
		return nil
	}
	eot := &ExtendedOffsets{
		Offsets: make([]uint64, n),
		Lengths: make([]uint64, n),
	}
	for i := 0; i < n; i++ {
		eot.Offsets[i] = leUint64(offsetsRaw[i*8 : i*8+8])
		eot.Lengths[i] = leUint64(lengthsRaw[i*8 : i*8+8])
	}
	return eot
}

func leUint64(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}
