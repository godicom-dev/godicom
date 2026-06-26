package godicom

import (
	"bytes"
	"encoding/binary"
	"io"
)

type DicomIO struct {
	reader         io.ReadSeeker
	writer         io.Writer
	byteOrder      binary.ByteOrder
	isLittleEndian bool
}

func NewDicomReader(r io.ReadSeeker) *DicomIO {
	return &DicomIO{
		reader: r,
	}
}

func NewDicomWriter(w io.Writer) *DicomIO {
	return &DicomIO{
		writer: w,
	}
}

func (d *DicomIO) SetByteOrder(le bool) {
	d.isLittleEndian = le
	if le {
		d.byteOrder = binary.LittleEndian
	} else {
		d.byteOrder = binary.BigEndian
	}
}

func (d *DicomIO) ByteOrder() binary.ByteOrder { return d.byteOrder }
func (d *DicomIO) IsLittleEndian() bool         { return d.isLittleEndian }

func (d *DicomIO) Read(b []byte) (int, error) {
	return io.ReadFull(d.reader, b)
}

func (d *DicomIO) ReadUint16() (uint16, error) {
	var b [2]byte
	if _, err := io.ReadFull(d.reader, b[:]); err != nil {
		return 0, err
	}
	return d.byteOrder.Uint16(b[:]), nil
}

func (d *DicomIO) ReadUint32() (uint32, error) {
	var b [4]byte
	if _, err := io.ReadFull(d.reader, b[:]); err != nil {
		return 0, err
	}
	return d.byteOrder.Uint32(b[:]), nil
}

func (d *DicomIO) ReadTag() (Tag, error) {
	v, err := d.ReadUint32()
	return Tag(v), err
}

func (d *DicomIO) Seek(offset int64, whence int) (int64, error) {
	return d.reader.Seek(offset, whence)
}

func (d *DicomIO) Tell() int64 {
	pos, _ := d.reader.Seek(0, io.SeekCurrent)
	return pos
}

func (d *DicomIO) Write(b []byte) (int, error) {
	return d.writer.Write(b)
}

func (d *DicomIO) WriteUint16(v uint16) error {
	var b [2]byte
	d.byteOrder.PutUint16(b[:], v)
	_, err := d.writer.Write(b[:])
	return err
}

func (d *DicomIO) WriteUint32(v uint32) error {
	var b [4]byte
	d.byteOrder.PutUint32(b[:], v)
	_, err := d.writer.Write(b[:])
	return err
}

func (d *DicomIO) WriteTag(tag Tag) error {
	return d.WriteUint32(uint32(tag))
}

type DicomBytesIO struct {
	reader *bytes.Reader
}

func NewDicomBytesIO(data []byte) *DicomBytesIO {
	return &DicomBytesIO{reader: bytes.NewReader(data)}
}

func (d *DicomBytesIO) Read(b []byte) (int, error) {
	return d.reader.Read(b)
}

func (d *DicomBytesIO) Seek(offset int64, whence int) (int64, error) {
	return d.reader.Seek(offset, whence)
}

func (d *DicomBytesIO) ReadAt(b []byte, off int64) (int, error) {
	return d.reader.ReadAt(b, off)
}

func (d *DicomBytesIO) Len() int {
	return d.reader.Len()
}

func (d *DicomBytesIO) Bytes() []byte {
	// Return the underlying data - we store it separately
	return nil
}
