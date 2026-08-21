package godicom

// codecContext is the encoding state a value codec needs to turn bytes into
// values or back: how the bytes are laid out, and how text in them is spelled.
//
// These travelled the write chain as three separate parameters. That cost two
// things worth fixing. The two bools are adjacent and interchangeable, so
// transposing them at a call site compiles and silently writes a file in the
// wrong encoding. And every new piece of codec state -- a diagnostic hook, a
// dictionary, a validation policy -- had to be threaded through every signature
// between writeDataset and the leaf that needed it.
//
// EncodingInfo is embedded rather than restated field by field: it is already
// the type for the implicit-VR/endianness pair, and it is what a Dataset stores
// its own encoding in.
type codecContext struct {
	EncodingInfo

	// Charsets is the SpecificCharacterSet in effect at this point. A sequence
	// item inherits its parent's, and a (0008,0005) element changes it for
	// everything after it in the same dataset, so this is per-nesting-level
	// state -- unlike readContext, which spans a whole parse.
	Charsets []string
}

// fileMetaCodec is the encoding PS3.10 fixes for File Meta Information:
// Explicit VR Little Endian, with no SpecificCharacterSet, since group 0x0002
// holds no VR whose meaning a character set could change.
var fileMetaCodec = codecContext{EncodingInfo: EncodingInfo{IsLittleEndian: true}}

// withCharsets returns cc with a different SpecificCharacterSet in effect, for
// crossing a (0008,0005) element.
func (cc codecContext) withCharsets(charsets []string) codecContext {
	cc.Charsets = charsets
	return cc
}
