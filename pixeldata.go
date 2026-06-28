package godicom

import (
	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
)

// TransferSyntaxUID returns the file meta transfer syntax UID string.
func (fd *FileDataset) TransferSyntaxUID() (string, bool) {
	if fd == nil || fd.FileMeta == nil {
		return "", false
	}
	return fd.FileMeta.GetString(tag.TransferSyntaxUID)
}

// PixelBytes returns decoded pixel data as a contiguous byte buffer.
// For multi-frame images, frames are concatenated in order.
func (fd *FileDataset) PixelBytes(opts ...pixels.DecodeOption) ([]byte, error) {
	return pixels.DecodeAllFrames(fd, opts...)
}

// PixelFrames returns decoded pixel data, one slice per frame.
func (fd *FileDataset) PixelFrames(opts ...pixels.DecodeOption) ([][]byte, error) {
	return pixels.DecodePixelData(fd, opts...)
}
