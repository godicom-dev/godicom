package godicom

import (
	"fmt"

	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// CompressPixelData re-encodes current Pixel Data to transferSyntaxUID and
// updates PixelData + FileMeta.TransferSyntaxUID.
//
// Supported targets: uncompressed (native), RLE Lossless, Deflated Image Frame
// Compression. JPEG / JPEG2000 encode is not available yet.
//
// Source frames are decoded with Raw=true (no photometric post-process).
func (fd *FileDataset) CompressPixelData(transferSyntaxUID string, opts ...pixels.EncodeOption) error {
	ts := uid.UID(transferSyntaxUID)
	if ts == "" {
		return fmt.Errorf("godicom: TransferSyntaxUID required")
	}
	desc, err := pixels.DescriptorFromFile(fd)
	if err != nil {
		return err
	}
	frames, err := fd.PixelFrames(pixels.WithRaw(true))
	if err != nil {
		return err
	}
	encOpts := pixels.EncodeOptions{TransferSyntaxUID: ts}
	for _, fn := range opts {
		if fn != nil {
			fn(&encOpts)
		}
	}
	encoded, err := pixels.EncodeFrames(frames, desc, encOpts)
	if err != nil {
		return err
	}
	return fd.SetEncodedPixelData(encoded)
}

// SetEncodedPixelData writes encoded Pixel Data and updates transfer syntax metadata.
func (fd *FileDataset) SetEncodedPixelData(encoded *pixels.EncodedPixelData) error {
	if fd == nil || encoded == nil {
		return fmt.Errorf("godicom: nil dataset or encoded pixel data")
	}
	elem := NewDataElement(tag.PixelData, VROB, encoded.PixelData)
	elem.IsUndefinedLength = encoded.IsEncapsulated
	fd.Set(elem)

	if encoded.IsEncapsulated && len(encoded.ExtendedOffsetTable) > 0 {
		fd.Set(NewDataElement(tag.ExtendedOffsetTable, VROV, encoded.ExtendedOffsetTable))
		fd.Set(NewDataElement(tag.ExtendedOffsetTableLengths, VROV, encoded.ExtendedOffsetTableLengths))
	} else {
		fd.Delete(tag.ExtendedOffsetTable)
		fd.Delete(tag.ExtendedOffsetTableLengths)
	}

	if fd.FileMeta == nil {
		fd.FileMeta = NewFileMetaDataset()
	}
	fd.FileMeta.Set(NewDataElement(tag.TransferSyntaxUID, VRUI, string(encoded.TransferSyntaxUID)))
	return nil
}
