package godicom

import "encoding/binary"

// elementHeader is what the wire says about one data element, before the value
// is touched: the VR in effect, the declared value length, and how many bytes
// the header itself occupies.
type elementHeader struct {
	VR VR
	// Length is the declared value length, uint32 because that is what the wire
	// carries and because 0xFFFFFFFF -- undefined length -- has to survive the
	// trip. An int would not hold that sentinel on a 32-bit platform.
	Length uint32
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
// private creator -- hence vr, which carries both and whose zero value resolves
// against Standard.
//
// enc is the encoding pair rather than a codecContext: a header holds no text,
// so no character set can change what it means.
//
// When the header runs past data, ok is false and need is how many bytes it
// would have taken: 8, or 12 once the VR is known to use a 32-bit length.
// Callers turn that into a DiagnosticTruncatedHeader.
func decodeElementHeader(
	data []byte,
	pos int64,
	tag Tag,
	enc EncodingInfo,
	vr vrResolver,
) (h elementHeader, need int64, ok bool) {
	if pos+8 > int64(len(data)) {
		return elementHeader{}, 8, false
	}

	if enc.IsImplicitVR {
		return elementHeader{
			VR:     vr.vrFor(tag),
			Length: uint32At(data, pos+4, enc.IsLittleEndian),
			Size:   8,
		}, 8, true
	}

	vrBytes := data[pos+4 : pos+6]
	encoded := VR(string(vrBytes))

	if !isVRByte(vrBytes[0]) || !isVRByte(vrBytes[1]) {
		return elementHeader{
			VR:     vr.vrFor(tag),
			Length: uint32At(data, pos+4, enc.IsLittleEndian),
			Size:   8,
		}, 8, true
	}

	if ExplicitVRLength16[encoded] {
		var length uint32
		if enc.IsLittleEndian {
			length = uint32(binary.LittleEndian.Uint16(data[pos+6 : pos+8]))
		} else {
			length = uint32(binary.BigEndian.Uint16(data[pos+6 : pos+8]))
		}
		return elementHeader{VR: encoded, Length: length, Size: 8}, 8, true
	}

	// Explicit VR with a 32-bit length: two reserved bytes, then the length.
	if pos+12 > int64(len(data)) {
		return elementHeader{}, 12, false
	}
	return elementHeader{VR: encoded, Length: uint32At(data, pos+8, enc.IsLittleEndian), Size: 12}, 12, true
}

// isVRByte reports whether b is one of the two uppercase ASCII letters a valid
// explicit VR is made of.
func isVRByte(b byte) bool { return b >= 'A' && b <= 'Z' }

func uint32At(data []byte, off int64, littleEndian bool) uint32 {
	if littleEndian {
		return binary.LittleEndian.Uint32(data[off : off+4])
	}
	return binary.BigEndian.Uint32(data[off : off+4])
}

// elementsResolver resolves VRs against rc's dictionary and the private creators
// found among the elements parsed so far, which is what the top-level readers
// have.
func elementsResolver(rc *readContext, elements *[]*DataElement) vrResolver {
	return vrResolver{
		dict:    rc.dictionary(),
		creator: func(t Tag) string { return privateCreatorFromElements(*elements, t) },
	}
}

// datasetResolver resolves VRs against rc's dictionary and the private creators
// in a dataset being filled in, which is what sequence items and deferred loads
// have.
//
// The dictionary comes from rc rather than from the codecContext because it has
// to outlive the parse: a deferred load runs after the read returned, and checks
// the VR it resolves against the one the element was first read under. A
// dictionary that went out of scope with the parse would turn every deferred
// private element into a "does not match original" error.
func datasetResolver(rc *readContext, ds *Dataset) vrResolver {
	return vrResolver{
		dict:    rc.dictionary(),
		creator: func(t Tag) string { return privateCreatorFromDataset(ds, t) },
	}
}
