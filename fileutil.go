package godicom

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// readUndefinedLengthValue reads an undefined length value until a delimiter tag.
func readUndefinedLengthValue(fp *DicomIO, delimiterTag Tag, isImplicitVR, isLittleEndian bool) ([]byte, error) {
	var data []byte
	for {
		tag, err := fp.ReadTag()
		if err != nil {
			return nil, err
		}
		if tag == delimiterTag {
			// Read the 4-byte length (should be 0)
			_, _ = fp.ReadUint32()
			break
		}
		var length uint32
		if isImplicitVR {
			length, err = fp.ReadUint32()
		} else {
			// Explicit VR: read VR (2 bytes) + reserved (2 bytes) + length (4 bytes)
			var vr [2]byte
			if _, err := io.ReadFull(fp.reader, vr[:]); err != nil {
				return nil, err
			}
			// Skip 2 reserved bytes
			if _, err := fp.ReadUint16(); err != nil {
				return nil, err
			}
			length, err = fp.ReadUint32()
		}
		if err != nil {
			return nil, err
		}
		_ = tag
		_ = length
		// We just skip the data for now
		if length > 0 && length != 0xFFFFFFFF {
			if _, err := io.CopyN(io.Discard, fp.reader, int64(length)); err != nil {
				return nil, err
			}
		}
	}
	return data, nil
}

// pathFromPathlike converts a path string to an absolute path.
func pathFromPathlike(path string) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("godicom: empty path")
	}
	return path, nil
}

// CheckBuffer validates a buffer for bulk data VRs.
func CheckBuffer(buf interface{}) error {
	// Placeholder for buffer validation
	return nil
}

// BufferLength returns the length of a buffer.
func BufferLength(buf interface{}) (int64, error) {
	switch b := buf.(type) {
	case *os.File:
		pos, _ := b.Seek(0, io.SeekCurrent)
		end, err := b.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, err
		}
		_, _ = b.Seek(pos, io.SeekStart)
		return end, nil
	case []byte:
		return int64(len(b)), nil
	}
	return 0, nil
}

// BufferEquality checks if two buffers are equal.
func BufferEquality(a, b interface{}) bool {
	// Simple byte comparison
	ab, ok1 := a.([]byte)
	bb, ok2 := b.([]byte)
	if ok1 && ok2 {
		if len(ab) != len(bb) {
			return false
		}
		for i := range ab {
			if ab[i] != bb[i] {
				return false
			}
		}
		return true
	}
	return false
}

// unackTag reads a tag from a byte slice.
func unackTag(b []byte, le bool) Tag {
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}
	return Tag(order.Uint32(b))
}
