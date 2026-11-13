package encaps

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// bufferedItem is one encapsulation item backed by a readable, seekable
// buffer. It mirrors pydicom.encaps._BufferedItem: the item is the buffer's
// data wrapped as a DICOM Item (FFFE,E000) with the data padded to an even
// length when needed.
type bufferedItem struct {
	// buffer holds the item's data (without the item tag and length).
	buffer io.ReadSeeker
	// blen is the non-padded length of the data in the buffer.
	blen int64
	// length is the total length of the item, including the item tag and
	// value and any trailing padding required to bring the item data up to
	// an even length.
	length int64
	// padding is whether the item data needs trailing padding.
	padding bool
}

const itemHeaderLen = 8

func newBufferedItem(buffer io.ReadSeeker) (*bufferedItem, error) {
	// The length of the buffer's data, and back to the start.
	blen, err := buffer.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("encaps: unable to determine the length of the buffer: %w", err)
	}
	if _, err := buffer.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("encaps: unable to rewind the buffer: %w", err)
	}
	if blen > 1<<32-2 {
		return nil, fmt.Errorf("encaps: buffers containing more than %d bytes are not supported", uint64(1)<<32-2)
	}

	return &bufferedItem{
		buffer:  buffer,
		blen:    blen,
		length:  itemHeaderLen + blen + blen%2,
		padding: blen%2 != 0,
	}, nil
}

// read returns data from the encapsulated item, where start is the position
// in the item (0 being the first byte of the item tag) and size the number
// of bytes to read. It mirrors _BufferedItem.read().
func (it *bufferedItem) read(start, size int64) ([]byte, error) {
	if start < 0 || start >= it.length {
		return nil, fmt.Errorf("encaps: invalid start value %d, must be in the closed interval [0, %d]", start, it.length-1)
	}

	header := []byte{0xFE, 0xFF, 0x00, 0xE0, 0x00, 0x00, 0x00, 0x00}
	binary.LittleEndian.PutUint32(header[4:], uint32(it.length-itemHeaderLen))

	var (
		nrRead int64
		out    []byte
	)
	for size > nrRead {
		offset := start + nrRead
		var chunk []byte
		switch {
		case offset < itemHeaderLen:
			// offset is in the item tag/length
			end := offset + size - nrRead
			if end > itemHeaderLen {
				end = itemHeaderLen
			}
			chunk = header[offset:end]
		case offset-itemHeaderLen < it.blen:
			// offset is in the item value; read from the buffer, restoring
			// its position afterwards
			if _, err := it.buffer.Seek(offset-itemHeaderLen, io.SeekStart); err != nil {
				return nil, err
			}
			buf := make([]byte, min(size-nrRead, it.blen-(offset-itemHeaderLen)))
			n, _ := it.buffer.Read(buf)
			if _, serr := it.buffer.Seek(0, io.SeekStart); serr != nil {
				return nil, serr
			}
			if n > 0 {
				chunk = buf[:n]
			}
		case it.padding && offset == it.length-1:
			// offset is in the item value padding
			chunk = []byte{0x00}
		default:
			// offset is past the end of the item value
			chunk = nil
		}

		if len(chunk) == 0 {
			break
		}
		nrRead += int64(len(chunk))
		out = append(out, chunk...)
	}

	return out, nil
}

// EncapsulatedBuffer manages the encapsulation of one or more buffers
// containing compressed frame data, mirroring pydicom.encaps.EncapsulatedBuffer.
// Each buffer holds a single frame and becomes one item of the encapsulated
// value; the buffers must be readable and seekable.
//
// An EncapsulatedBuffer is itself an io.ReadSeeker over the complete
// encapsulated value: a Basic Offset Table Item (empty unless useBOT was set),
// then one Item per buffer, each padded to an even length. This is the shape
// a seekable synthesized stream needs, as the value can be read out of the
// buffer without the frames ever being in memory together.
type EncapsulatedBuffer struct {
	items  []*bufferedItem
	useBOT bool
	// offset is the current position in the encapsulated value.
	offset int64

	// itemOffsets are the start positions of the regions of the encapsulated
	// value: 0 for the Basic Offset Table, then one per item, then the end.
	itemOffsets []int64
	// botItem is the Basic Offset Table value read as a buffered item.
	botItem *bufferedItem
	bot     []byte
}

// NewEncapsulatedBuffer returns an EncapsulatedBuffer for the frame data in
// buffers. When useBOT is true the Basic Offset Table includes the offset of
// each item, otherwise it is empty.
func NewEncapsulatedBuffer(buffers []io.ReadSeeker, useBOT bool) (*EncapsulatedBuffer, error) {
	eb := &EncapsulatedBuffer{useBOT: useBOT}

	total := int64(itemHeaderLen)
	for _, b := range buffers {
		item, err := newBufferedItem(b)
		if err != nil {
			return nil, err
		}
		eb.items = append(eb.items, item)
		total += item.length
	}

	bot, err := eb.basicOffsetTable()
	if err != nil {
		return nil, err
	}
	eb.bot = bot
	botValue := bot
	if len(botValue) >= itemHeaderLen {
		botValue = botValue[itemHeaderLen:]
	}
	if eb.botItem, err = newBufferedItem(bytes.NewReader(botValue)); err != nil {
		return nil, err
	}

	// The region boundaries: the Basic Offset Table, then each item, then
	// the end of the encapsulated value.
	eb.itemOffsets = []int64{0}
	pos := int64(len(eb.bot))
	for _, item := range eb.items {
		eb.itemOffsets = append(eb.itemOffsets, pos)
		pos += item.length
	}
	eb.itemOffsets = append(eb.itemOffsets, pos)

	return eb, nil
}

// basicOffsetTable returns an encoded Basic Offset Table.
func (eb *EncapsulatedBuffer) basicOffsetTable() ([]byte, error) {
	if !eb.useBOT {
		return []byte{0xFE, 0xFF, 0x00, 0xE0, 0x00, 0x00, 0x00, 0x00}, nil
	}

	offsets := eb.Offsets()
	out := make([]byte, itemHeaderLen+4*len(offsets))
	out[0], out[1], out[2], out[3] = 0xFE, 0xFF, 0x00, 0xE0
	binary.LittleEndian.PutUint32(out[4:], uint32(4*len(offsets)))
	for i, offset := range offsets {
		if offset > 1<<32-1 {
			return nil, fmt.Errorf(
				"encaps: the Basic Offset Table cannot hold the offset %d (%#x), use the Extended Offset Table instead", offset, offset,
			)
		}
		binary.LittleEndian.PutUint32(out[itemHeaderLen+4*i:], uint32(offset))
	}

	return out, nil
}

// BasicOffsetTable returns an encoded Basic Offset Table Item: empty unless
// the buffer was created with useBOT set, in which case it holds the offset
// of each item.
func (eb *EncapsulatedBuffer) BasicOffsetTable() []byte {
	return bytes.Clone(eb.bot)
}

// Lengths returns the total length of each item, including the item tag and
// length and any trailing padding.
func (eb *EncapsulatedBuffer) Lengths() []uint64 {
	out := make([]uint64, len(eb.items))
	for i, item := range eb.items {
		out[i] = uint64(item.length)
	}
	return out
}

// Offsets returns the offset of the first byte of the item tag of each item,
// starting at 0 for the first item.
func (eb *EncapsulatedBuffer) Offsets() []uint64 {
	lengths := eb.Lengths()
	out := make([]uint64, len(lengths))
	var total uint64
	for i := range lengths {
		out[i] = total
		total += lengths[i]
	}
	return out
}

// EncapsulatedLength returns the total length of the encapsulated value.
func (eb *EncapsulatedBuffer) EncapsulatedLength() int64 {
	return eb.itemOffsets[len(eb.itemOffsets)-1]
}

// ExtendedOffsets returns an encoded Extended Offset Table value: the offset
// to the first byte of the item tag of the first fragment for every frame, as
// measured from the first byte of the first item tag following the empty
// Basic Offset Table Item.
func (eb *EncapsulatedBuffer) ExtendedOffsets() []byte {
	offsets := eb.Offsets()
	out := make([]byte, 8*len(offsets))
	for i, offset := range offsets {
		binary.LittleEndian.PutUint64(out[8*i:], offset)
	}
	return out
}

// ExtendedLengths returns an encoded Extended Offset Table Lengths value: the
// length of each frame, excluding the item tag and item length.
func (eb *EncapsulatedBuffer) ExtendedLengths() []byte {
	lengths := eb.Lengths()
	out := make([]byte, 8*len(lengths))
	for i, length := range lengths {
		binary.LittleEndian.PutUint64(out[8*i:], length-itemHeaderLen)
	}
	return out
}

// Read reads up to len(p) bytes of the encapsulated value, mirroring
// EncapsulatedBuffer.read(). It returns io.EOF once the end of the value is
// reached.
func (eb *EncapsulatedBuffer) Read(p []byte) (int, error) {
	total := eb.EncapsulatedLength()
	if eb.offset >= total {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}

	size := int64(len(p))
	var nrRead int64
	for size > nrRead {
		// The region the current position is in.
		idx := -1
		for i := 1; i < len(eb.itemOffsets); i++ {
			if eb.itemOffsets[i-1] <= eb.offset && eb.offset < eb.itemOffsets[i] {
				idx = i - 1
				break
			}
		}
		if idx < 0 {
			break
		}

		var chunk []byte
		var err error
		if idx == 0 {
			chunk, err = eb.botItem.read(eb.offset, size-nrRead)
		} else {
			chunk, err = eb.items[idx-1].read(eb.offset-eb.itemOffsets[idx], size-nrRead)
		}
		if err != nil {
			return int(nrRead), err
		}
		if len(chunk) == 0 {
			break
		}

		copy(p[nrRead:], chunk)
		eb.offset += int64(len(chunk))
		nrRead += int64(len(chunk))

		if eb.offset >= total {
			break
		}
	}

	if nrRead == 0 {
		return 0, io.EOF
	}
	return int(nrRead), nil
}

// Seek sets the position within the encapsulated value, emulating
// io.BytesIO's behavior for negative offsets.
func (eb *EncapsulatedBuffer) Seek(offset int64, whence int) (int64, error) {
	if whence < io.SeekStart || whence > io.SeekEnd {
		return 0, fmt.Errorf("encaps: invalid whence value %d, should be 0, 1 or 2", whence)
	}

	var newOffset int64
	switch whence {
	case io.SeekStart:
		// Relative to the beginning of the value
		if offset < 0 {
			return 0, fmt.Errorf("encaps: negative seek offset value %d", offset)
		}
		newOffset = offset
	case io.SeekCurrent:
		// Relative to the current position
		newOffset = max(eb.offset+offset, 0)
	case io.SeekEnd:
		// Relative to the end of the value
		newOffset = max(eb.EncapsulatedLength()+offset, 0)
	}
	eb.offset = newOffset

	return eb.offset, nil
}

// Tell returns the current position within the encapsulated value.
func (eb *EncapsulatedBuffer) Tell() int64 {
	return eb.offset
}

// EncapsulateBuffer returns the encapsulated Pixel Data value for the frame
// data in buffers, mirroring pydicom.encaps.encapsulate_buffer. Each buffer
// holds a single frame and becomes one item of the encapsulated value. When
// hasBOT is true the Basic Offset Table includes the offset of each item.
func EncapsulateBuffer(buffers []io.ReadSeeker, hasBOT bool) ([]byte, error) {
	eb, err := NewEncapsulatedBuffer(buffers, hasBOT)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(eb)
}

// EncapsulateExtendedBuffer returns the encapsulated Pixel Data value for the
// frame data in buffers together with the encoded values for the Extended
// Offset Table elements, mirroring pydicom.encaps.encapsulate_extended_buffer.
// The Basic Offset Table is left empty, as the Extended Offset Table carries
// the offsets.
func EncapsulateExtendedBuffer(buffers []io.ReadSeeker) (pixelData, offsets, lengths []byte, err error) {
	eb, err := NewEncapsulatedBuffer(buffers, false)
	if err != nil {
		return nil, nil, nil, err
	}
	if pixelData, err = io.ReadAll(eb); err != nil {
		return nil, nil, nil, err
	}
	return pixelData, eb.ExtendedOffsets(), eb.ExtendedLengths(), nil
}
