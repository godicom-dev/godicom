package godicom

import (
	"bytes"
	"encoding/binary"
	"io"
)

type dicomIO struct {
	reader         io.ReadSeeker
	writer         io.Writer
	byteOrder      binary.ByteOrder
	isLittleEndian bool
}

func newDicomReader(r io.ReadSeeker) *dicomIO {
	return &dicomIO{
		reader: r,
	}
}

func newDicomWriter(w io.Writer) *dicomIO {
	return &dicomIO{
		writer: w,
	}
}

// DicomIO reads and writes DICOM primitive values.
//
// Deprecated: this type is internal and will be removed.
type DicomIO = dicomIO

// NewDicomReader creates a DICOM reader.
//
// Deprecated: this constructor is internal and will be removed.
func NewDicomReader(r io.ReadSeeker) *DicomIO {
	return newDicomReader(r)
}

// NewDicomWriter creates a DICOM writer.
//
// Deprecated: this constructor is internal and will be removed.
func NewDicomWriter(w io.Writer) *DicomIO {
	return newDicomWriter(w)
}

func (d *dicomIO) SetByteOrder(le bool) {
	d.isLittleEndian = le
	if le {
		d.byteOrder = binary.LittleEndian
	} else {
		d.byteOrder = binary.BigEndian
	}
}

func (d *dicomIO) ByteOrder() binary.ByteOrder { return d.byteOrder }
func (d *dicomIO) IsLittleEndian() bool        { return d.isLittleEndian }

func (d *dicomIO) Read(b []byte) (int, error) {
	return io.ReadFull(d.reader, b)
}

func (d *dicomIO) ReadUint16() (uint16, error) {
	var b [2]byte
	if _, err := io.ReadFull(d.reader, b[:]); err != nil {
		return 0, err
	}
	return d.byteOrder.Uint16(b[:]), nil
}

func (d *dicomIO) ReadUint32() (uint32, error) {
	var b [4]byte
	if _, err := io.ReadFull(d.reader, b[:]); err != nil {
		return 0, err
	}
	return d.byteOrder.Uint32(b[:]), nil
}

func (d *dicomIO) ReadTag() (Tag, error) {
	v, err := d.ReadUint32()
	return Tag(v), err
}

func (d *dicomIO) Seek(offset int64, whence int) (int64, error) {
	return d.reader.Seek(offset, whence)
}

func (d *dicomIO) Tell() int64 {
	pos, _ := d.reader.Seek(0, io.SeekCurrent)
	return pos
}

func (d *dicomIO) Write(b []byte) (int, error) {
	return d.writer.Write(b)
}

func (d *dicomIO) WriteUint16(v uint16) error {
	var b [2]byte
	d.byteOrder.PutUint16(b[:], v)
	_, err := d.writer.Write(b[:])
	return err
}

func (d *dicomIO) WriteUint32(v uint32) error {
	var b [4]byte
	d.byteOrder.PutUint32(b[:], v)
	_, err := d.writer.Write(b[:])
	return err
}

func (d *dicomIO) WriteTag(tag Tag) error {
	return d.WriteUint32(uint32(tag))
}

type dicomBytesIO struct {
	reader *bytes.Reader
}

func newDicomBytesIO(data []byte) *dicomBytesIO {
	return &dicomBytesIO{reader: bytes.NewReader(data)}
}

// DicomBytesIO wraps an in-memory DICOM byte reader.
//
// Deprecated: this type is internal and will be removed.
type DicomBytesIO = dicomBytesIO

// NewDicomBytesIO creates an in-memory DICOM byte reader.
//
// Deprecated: this constructor is internal and will be removed.
func NewDicomBytesIO(data []byte) *DicomBytesIO {
	return newDicomBytesIO(data)
}

func (d *dicomBytesIO) Read(b []byte) (int, error) {
	return d.reader.Read(b)
}

func (d *dicomBytesIO) Seek(offset int64, whence int) (int64, error) {
	return d.reader.Seek(offset, whence)
}

func (d *dicomBytesIO) ReadAt(b []byte, off int64) (int, error) {
	return d.reader.ReadAt(b, off)
}

func (d *dicomBytesIO) Len() int {
	return d.reader.Len()
}

func (d *dicomBytesIO) Bytes() []byte {
	// Return the underlying data - we store it separately
	return nil
}
