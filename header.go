package godicom

import "encoding/binary"

// elementHeader is what the wire says about one data element, before the value
// is touched: the VR in effect, the declared value length, and how many bytes
// the header itself occupies.
type elementHeader struct {
	VR     VR
	Length int
	// Size is the header length in bytes: 8, or 12 for an explicit VR that
	// carries a 32-bit length.
	Size int
}

// creatorFunc resolves the private creator string for a tag. Reading it costs a
// scan of what has been parsed so far, so the header decoder only calls it when
// a private tag actually needs the dictionary.
type creatorFunc func(Tag) string

// decodeElementHeader decodes the data element header at pos.
//
// The wire does not always mean what it appears to say. Implicit VR carries no
// VR at all, and an explicit VR element whose VR bytes are not two uppercase
// letters is read as implicit for that element (pydicom issues 1067 and 1035).
// Both cases fall back to the data dictionary, which for a private tag needs the
// private creator -- hence creator, which may be nil.
//
// When the header runs past data, ok is false and need is how many bytes it
// would have taken: 8, or 12 once the VR is known to use a 32-bit length.
// Callers turn that into a DiagnosticTruncatedHeader.
func decodeElementHeader(
	data []byte,
	pos int64,
	tag Tag,
	implicitVR, littleEndian bool,
	creator creatorFunc,
) (h elementHeader, need int64, ok bool) {
	if pos+8 > int64(len(data)) {
		return elementHeader{}, 8, false
	}

	if implicitVR {
		return elementHeader{
			VR:     dictionaryVRForRead(tag, creator),
			Length: uint32At(data, pos+4, littleEndian),
			Size:   8,
		}, 8, true
	}

	vrBytes := data[pos+4 : pos+6]
	vr := VR(string(vrBytes))

	if !isVRByte(vrBytes[0]) || !isVRByte(vrBytes[1]) {
		return elementHeader{
			VR:     dictionaryVRForRead(tag, creator),
			Length: uint32At(data, pos+4, littleEndian),
			Size:   8,
		}, 8, true
	}

	if ExplicitVRLength16[vr] {
		var length int
		if littleEndian {
			length = int(binary.LittleEndian.Uint16(data[pos+6 : pos+8]))
		} else {
			length = int(binary.BigEndian.Uint16(data[pos+6 : pos+8]))
		}
		return elementHeader{VR: vr, Length: length, Size: 8}, 8, true
	}

	// Explicit VR with a 32-bit length: two reserved bytes, then the length.
	if pos+12 > int64(len(data)) {
		return elementHeader{}, 12, false
	}
	return elementHeader{VR: vr, Length: uint32At(data, pos+8, littleEndian), Size: 12}, 12, true
}

// isVRByte reports whether b is one of the two uppercase ASCII letters a valid
// explicit VR is made of.
func isVRByte(b byte) bool { return b >= 'A' && b <= 'Z' }

// dictionaryVRForRead resolves the VR of a tag whose encoding did not carry one.
func dictionaryVRForRead(tag Tag, creator creatorFunc) VR {
	if !tag.IsPrivate() {
		return LookupVR(tag)
	}
	c := ""
	if creator != nil {
		c = creator(tag)
	}
	return lookupVRWithCreator(tag, c)
}

func uint32At(data []byte, off int64, littleEndian bool) int {
	if littleEndian {
		return int(binary.LittleEndian.Uint32(data[off : off+4]))
	}
	return int(binary.BigEndian.Uint32(data[off : off+4]))
}

// elementsCreator resolves private creators against the elements parsed so far,
// which is what the top-level readers have.
func elementsCreator(elements *[]*DataElement) creatorFunc {
	return func(t Tag) string { return privateCreatorFromElements(*elements, t) }
}

// datasetCreator resolves private creators against a dataset being filled in,
// which is what sequence items and deferred loads have.
func datasetCreator(ds *Dataset) creatorFunc {
	return func(t Tag) string { return privateCreatorFromDataset(ds, t) }
}
