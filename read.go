package godicom

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// ElementAction is what a read should do with an element whose header has just
// been decoded, returned by ReadOptions.OnElement. Its zero value keeps the
// element, so a hook that falls off the end of its own logic parses the file the
// way godicom always has.
type ElementAction int

const (
	// ElementKeep parses the value and stores the element. This is the zero
	// value, and what every element gets when no hook is set.
	ElementKeep ElementAction = iota

	// ElementSkip advances past the value without decoding or allocating it, and
	// stores nothing. The parse continues with the next element.
	//
	// Skipping is not the same as deferring: a deferred element keeps its place
	// in the dataset and loads its value on Get, while a skipped one is not in
	// the dataset at all.
	ElementSkip

	// ElementStop abandons the parse at this element, which is not stored, and
	// returns what was read before it. The rest of the stream is never touched.
	ElementStop
)

type ReadOptions struct {
	DeferSize        uint32
	StopBeforePixels bool
	Force            bool
	SpecificTags     []Tag
	// OnElement decides, per element, whether to parse its value, step over it,
	// or stop reading -- the whole point being that a caller who wants seven index
	// tags out of a 200 MB file should not pay to decode the other 199:
	//
	//	want := map[godicom.Tag]bool{tag.PatientID: true, tag.StudyInstanceUID: true}
	//	ds, err := godicom.ReadFile("ct.dcm", &godicom.ReadOptions{
	//		OnElement: func(h godicom.RawDataElement, path []godicom.PathStep) godicom.ElementAction {
	//			if len(path) > 0 || want[h.Tag] {
	//				return godicom.ElementKeep
	//			}
	//			return godicom.ElementSkip
	//		},
	//	})
	//
	// It is called once per element, after the header is decoded and before the
	// value is read, so h carries the Tag, the VR in effect, the declared Length
	// and the ValueTell the value would start at -- but not the value itself,
	// which is the saving. h.Value is always nil.
	//
	// path names the enclosing sequences and the item of each, outermost first,
	// and is nil at the top level. The hook is called at every depth, so an
	// oversized nested sequence can be stepped over element by element; the
	// example above keeps everything inside a sequence it decided to parse.
	//
	// path is the reader's own slice and is only valid for the duration of the
	// call -- the reader overwrites it as it descends and returns. Copy it to keep
	// it. It is not copied for you because the hook is called once per element and
	// most hooks only read it.
	//
	// The skipping is real on a seekable source, where the value bytes are never
	// read. On a non-seekable reader godicom has already buffered the stream, so
	// skipping saves the decode and the allocation but not the I/O.
	//
	// Two elements are exempt and never offered: group 0x0002, which is the File
	// Meta the transfer syntax is read from, and (0008,0005) Specific Character
	// Set, which decides how every text value after it is decoded. Skipping
	// either would change what the rest of the file means rather than merely how
	// much of it is kept.
	//
	// StopBeforePixels and SpecificTags are the two hooks godicom shipped before
	// this one and are implemented on top of it. A hook set here runs after them,
	// and can only narrow what they kept: an element they skipped is already gone.
	OnElement func(RawDataElement, []PathStep) ElementAction
	// Logger overrides the call-scoped slog logger for this read.
	// When nil, LoggerFromContext / DefaultLogger is used.
	Logger *slog.Logger
	// OnDiagnostic observes parse anomalies the reader would otherwise recover
	// from silently, such as a file that ends inside an element. Returning nil
	// keeps the old behaviour (stop parsing, return what was read); returning a
	// non-nil error fails the read with it, so
	//
	//	OnDiagnostic: func(d Diagnostic) error { return d }
	//
	// rejects anything a strict parser would reject.
	//
	// It is also called after the read returns, when a deferred value fails to
	// load. Deferred loads are triggered by Dataset.Get, so a hook must be safe
	// to call from wherever the dataset is used.
	OnDiagnostic func(Diagnostic) error
	// Dictionary resolves tags to their data dictionary entries for this read.
	// When nil, Standard is used.
	//
	// It changes what an element means, not merely how it is described. An
	// implicit VR file carries no VRs, so every element's VR is whatever the
	// dictionary says it is -- and for a private element that answer depends on
	// the vendor. Compose rather than replace, or the standard elements lose
	// their VRs too:
	//
	//	vendor := godicom.NewPrivateDictionary()
	//	if err := vendor.Add("ACME 3.2", tag, godicom.VRUS, "Some Number"); err != nil {
	//		return err
	//	}
	//	ds, err := godicom.ReadFile("ct.dcm", &godicom.ReadOptions{
	//		Dictionary: godicom.NewDictionary(vendor, godicom.Standard()),
	//	})
	//
	// The dictionary is retained by the dataset that comes back, because a
	// deferred value is decoded on Get, long after the read returned, and has to
	// resolve to the VR it was first read under.
	//
	// This is a read option and has no counterpart on the way out. The write path
	// resolves an ambiguous VR from the dataset's own values -- Pixel
	// Representation decides between US and SS -- and never asks the dictionary,
	// so a WriteOptions field would have nothing to do. The typed setters
	// (SetString, SetInt, ...) resolve against PS3.6 too: they take the tag alone,
	// and a caller with a vendor's element to store passes the VR explicitly
	// through Set(NewDataElement(tag, vr, value)).
	Dictionary Dictionary
}

func readDictionary(opts *ReadOptions) Dictionary {
	if opts == nil {
		return nil
	}
	return opts.Dictionary
}

func diagnosticHook(opts *ReadOptions) func(Diagnostic) error {
	if opts == nil {
		return nil
	}
	return opts.OnDiagnostic
}

func hasExplicitVRAt(data []byte, pos int64) bool {
	if pos+6 > int64(len(data)) {
		return false
	}
	rawVR := data[pos+4 : pos+6]
	return rawVR[0] >= 0x41 && rawVR[0] <= 0x5A && rawVR[1] >= 0x41 && rawVR[1] <= 0x5A
}

func inflateRaw(data []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	return io.ReadAll(r)
}

func readTagBytes(data []byte, pos int64, isLittleEndian bool) Tag {
	var order binary.ByteOrder = binary.LittleEndian
	if !isLittleEndian {
		order = binary.BigEndian
	}
	group := order.Uint16(data[pos : pos+2])
	element := order.Uint16(data[pos+2 : pos+4])
	return NewTag(int(group), int(element))
}

// decideElement is the one place a read decides what to do with an element whose
// header it has just decoded. Every loop -- the two over a byte slice and the one
// over a ReaderAt -- calls it at the same point, immediately after
// decodeElementHeader, so the three agree on the answer by construction rather
// than by three copies of the same condition staying in step.
//
// It is called before the value is read. Diagnostics are not suppressed for a
// skipped element: they describe the file, not the subset the caller chose to
// keep, and a caller narrowing a read has no reason to stop hearing that the
// bytes are malformed. An ElementStop is the exception, because the element it
// stops at is not parsed at all.
//
// enc and valueStart fill in the RawDataElement handed to the hook; rc supplies
// the enclosing sequence path, which is empty at the top level.
func decideElement(
	opts *ReadOptions,
	rc *readContext,
	h elementHeader,
	tag Tag,
	valueStart int64,
	enc EncodingInfo,
) ElementAction {
	if opts == nil {
		return ElementKeep
	}
	var path []PathStep
	if rc != nil {
		path = rc.seqPath
	}
	// The File Meta carries the transfer syntax the rest of the file is encoded
	// in, and Specific Character Set decides how every text value after it
	// decodes. Skipping either changes what the remaining elements mean, not just
	// how many are kept, so neither is offered to any of the three mechanisms
	// below.
	if tag.Group() == 0x0002 || tag == TagCharset {
		return ElementKeep
	}
	if opts.StopBeforePixels && tag == tagPixelData {
		return ElementStop
	}
	// SpecificTags filters the top-level dataset only. pydicom scopes it the same
	// way -- read_sequence_item calls read_dataset without passing specific_tags --
	// and the alternative is worse than merely different: filtering inside a
	// sequence keeps the sequence element and empties its items, which is a
	// dataset no file could have produced.
	if len(opts.SpecificTags) > 0 && len(path) == 0 {
		found := false
		for _, specificTag := range opts.SpecificTags {
			if tag == specificTag {
				found = true
				break
			}
		}
		if !found {
			return ElementSkip
		}
	}
	if opts.OnElement == nil {
		return ElementKeep
	}
	// In source coordinates, the same way a Diagnostic reports its Offset: the
	// streaming reader parses a sequence out of a chunk it copied, so the position
	// the loop holds is relative to that chunk and means nothing to a caller
	// holding the file.
	if rc != nil {
		valueStart += rc.baseOffset
	}
	return opts.OnElement(RawDataElement{
		Tag:            tag,
		VR:             h.VR,
		Length:         h.Length,
		ValueTell:      valueStart,
		IsImplicitVR:   enc.IsImplicitVR,
		IsLittleEndian: enc.IsLittleEndian,
		IsRaw:          true,
	}, path)
}

func readDeferSize(opts *ReadOptions) uint32 {
	if opts == nil {
		return 0
	}
	return opts.DeferSize
}

func creatorStringFromElement(elem *DataElement) string {
	if elem == nil || elem.Value == nil {
		return ""
	}
	switch v := elem.Value.(type) {
	case string:
		return strings.TrimRight(v, " ")
	case []byte:
		return strings.TrimRight(string(v), " \x00")
	default:
		return strings.TrimRight(fmt.Sprintf("%v", v), " ")
	}
}

func privateCreatorFromElements(elements []*DataElement, tag Tag) string {
	creatorTag := tag.PrivateCreator()
	for i := len(elements) - 1; i >= 0; i-- {
		if elements[i].Tag == creatorTag {
			return creatorStringFromElement(elements[i])
		}
	}
	return ""
}

func privateCreatorFromDataset(ds *Dataset, tag Tag) string {
	if ds == nil {
		return ""
	}
	if elem, ok := ds.elements[tag.PrivateCreator()]; ok {
		return creatorStringFromElement(elem)
	}
	return ""
}

// ReadBytes parses a Part 10 DICOM file from data (preamble optional when Force).
func ReadBytes(data []byte, opts *ReadOptions) (*FileDataset, error) {
	return ReadBytesContext(context.Background(), data, opts)
}

// ReadBytesContext is like ReadBytes but uses ctx for cancellation and logging.
func ReadBytesContext(ctx context.Context, data []byte, opts *ReadOptions) (*FileDataset, error) {
	return readBytes(ctx, data, "", 0, opts)
}

func readBytes(ctx context.Context, data []byte, filename string, modTime int64, opts *ReadOptions) (*FileDataset, error) {
	if opts == nil {
		opts = &ReadOptions{}
	}
	ctx = loggerContext(ctx, opts.Logger, ComponentReader)

	if len(data) < 8 {
		return nil, &InvalidDICOMError{Message: "file too small"}
	}

	var preamble []byte
	pos := int64(0)

	if len(data) >= 132 && string(data[128:132]) == "DICM" {
		preamble = data[:128]
		pos = 132
		logDebug(ctx, "Reading File Meta Information preamble", AttrOffset, int64(0), AttrOffsetHex, offsetHex(0), AttrPath, filename)
		if len(preamble) >= 8 {
			logDebug(ctx, "preamble sample", AttrOffsetHex, offsetHex(0), AttrHex, bytesHex(preamble[:8]))
		}
		logDebug(ctx, "Reading File Meta Information prefix", AttrOffset, int64(128), AttrOffsetHex, offsetHex(128))
		logDebug(ctx, "'DICM' prefix found", AttrOffset, int64(128), AttrOffsetHex, offsetHex(128), AttrPath, filename)
	} else if !opts.Force {
		return nil, &InvalidDICOMError{Message: "missing DICM prefix"}
	} else {
		logDebug(ctx, "reading without DICM prefix", AttrPath, filename)
	}
	cc := codecContext{EncodingInfo: EncodingInfo{IsLittleEndian: true}}
	inFileMeta := true

	if pos+6 <= int64(len(data)) {
		cc.IsImplicitVR = !hasExplicitVRAt(data, pos)
	}

	// Read all elements in one pass, then separate file meta
	allElements := make([]*DataElement, 0)
	cc.Charsets = []string{DefaultCharacterSet}
	readCtx := &readContext{
		data:     data,
		filename: filename,
		modTime:  modTime,
		ctx:      ctx,
		onDiag:   diagnosticHook(opts),
		dict:     readDictionary(opts),
	}
	resolve := elementsResolver(readCtx, &allElements)

	for pos+4 <= int64(len(data)) {
		currentTag := readTagBytes(data, pos, cc.IsLittleEndian)
		if inFileMeta && currentTag.Group() != 0x0002 {
			inFileMeta = false
			if len(allElements) > 0 {
				ts := determineTransferSyntaxFromElements(allElements)
				logDebug(ctx, "transfer syntax", AttrTransferSyntax, string(ts), AttrOffset, pos)
				if ts == DeflatedExplicitVRLittleEndian {
					inflated, err := inflateRaw(data[pos:])
					if err != nil {
						return nil, err
					}
					data = inflated
					// Every offset from here on -- including the ValueTell of a
					// deferred element -- is relative to the inflated bytes, so the
					// deferred source has to follow. Leaving it on the compressed
					// buffer made a deferred load read whatever byte happened to
					// live at that offset before decompression.
					readCtx.data = data
					pos = 0
					cc.EncodingInfo = EncodingInfo{IsImplicitVR: false, IsLittleEndian: true}
					currentTag = readTagBytes(data, pos, cc.IsLittleEndian)
				} else {
					cc.EncodingInfo = EncodingInfo{
						IsImplicitVR:   ts.IsImplicitVR(),
						IsLittleEndian: ts.IsLittleEndian(),
					}
					currentTag = readTagBytes(data, pos, cc.IsLittleEndian)
				}
			} else {
				littleTag := readTagBytes(data, pos, true)
				bigTag := readTagBytes(data, pos, false)
				switch {
				case hasExplicitVRAt(data, pos) && !dictionaryHasTag(littleTag) && dictionaryHasTag(bigTag):
					cc.EncodingInfo = EncodingInfo{IsImplicitVR: false, IsLittleEndian: false}
					currentTag = bigTag
				case hasExplicitVRAt(data, pos):
					cc.IsImplicitVR = false
					currentTag = littleTag
				default:
					cc.EncodingInfo = EncodingInfo{IsImplicitVR: true, IsLittleEndian: true}
					currentTag = littleTag
				}
			}
		}

		if currentTag == ItemDelimiterTag || currentTag == SequenceDelimiterTag {
			break
		}

		h, need, ok := decodeElementHeader(data, pos, currentTag, cc.EncodingInfo, resolve)
		if !ok {
			if err := readCtx.report(truncatedHeader(currentTag, pos, need, int64(len(data)))); err != nil {
				return nil, err
			}
			break
		}
		vr, length, hdrSize := h.VR, h.Length, h.Size

		action := decideElement(opts, readCtx, h, currentTag, pos+int64(hdrSize), cc.EncodingInfo)
		if action == ElementStop {
			break
		}
		keep := action == ElementKeep

		if err := readCtx.reportVRMismatch(currentTag, vr, pos, cc.IsImplicitVR); err != nil {
			return nil, err
		}

		logElementHeader(ctx, pos, data[pos:pos+int64(hdrSize)], currentTag, vr, length)

		elem := NewDataElement(currentTag, vr, nil)

		if length == 0 {
			elem.Value = emptyValueForVR(vr)
			pos += int64(hdrSize)
			if keep {
				allElements = append(allElements, elem)
			}
			continue
		}

		if length == 0xFFFFFFFF {
			elem.IsUndefinedLength = true
			valueStart := pos + int64(hdrSize)
			if shouldReadUndefinedLengthAsSequence(vr) {
				if vr == VRUN {
					elem.VR = VRSQ
				}
				logDebug(ctx, "Reading/parsing undefined length sequence",
					AttrOffset, valueStart, AttrOffsetHex, offsetHex(valueStart), AttrTag, currentTag.String())
				readCtx.pushSeq(currentTag)
				seq, newPos, err := readSequenceItems(data, valueStart, cc, opts, readCtx)
				readCtx.popSeq()
				elem.Value = seq
				pos = newPos
				if err != nil {
					return nil, err
				}
			} else if !keep {
				// Step over the item stream without copying it. An undefined-length
				// value has no length field, so finding the end means walking the
				// items either way -- but walking them is cheap and copying them is
				// the whole cost of the element.
				pos = skipUndefinedLengthValue(data, valueStart, cc.IsLittleEndian)
			} else {
				logDebug(ctx, "Reading undefined length data element",
					AttrOffset, valueStart, AttrOffsetHex, offsetHex(valueStart), AttrTag, currentTag.String())
				if encapsulated, endPos, ok := readEncapsulatedPixelData(data, valueStart, cc.IsLittleEndian); ok {
					if shouldDeferElement(currentTag, uint32(len(encapsulated)), readDeferSize(opts)) {
						logDebug(ctx, "Defer size exceeded. Skipping forward to next data element.",
							AttrTag, currentTag.String(), AttrLen, len(encapsulated))
						markElementDeferred(elem, valueStart, uint32(len(encapsulated)), cc)
					} else {
						logElementValue(ctx, valueStart, encapsulated)
						assignElementBytes(elem, encapsulated, vr, cc)
					}
					pos = endPos
				} else {
					raw, newPos := readBytesUntilDelimiter(data, valueStart, SequenceDelimiterTag, cc.IsLittleEndian)
					logElementValue(ctx, valueStart, raw)
					elem.RawValue = raw
					pos = newPos
				}
			}
			if keep {
				allElements = append(allElements, elem)
			}
			continue
		}

		if vr == VRSQ {
			if keep {
				readCtx.pushSeq(currentTag)
				seq, newPos, err := readDefinedLengthSequence(
					data,
					pos+int64(hdrSize),
					length,
					cc,
					opts,
					readCtx,
				)
				readCtx.popSeq()
				elem.Value = seq
				pos = newPos
				if err != nil {
					return nil, err
				}
				allElements = append(allElements, elem)
			} else {
				// A defined-length sequence declares its own extent, so stepping over
				// it costs nothing at all: no items are walked and no nested element
				// is decoded.
				pos += int64(hdrSize) + int64(length)
			}
			continue
		}

		if pos+int64(hdrSize)+int64(length) > int64(len(data)) {
			if err := readCtx.report(truncatedValue(currentTag, vr, pos+int64(hdrSize), int64(length), int64(len(data)))); err != nil {
				return nil, err
			}
			break
		}

		if keep {
			value := data[pos+int64(hdrSize) : pos+int64(hdrSize)+int64(length)]
			valueTell := pos + int64(hdrSize)

			if shouldDeferElement(currentTag, length, readDeferSize(opts)) {
				logDebug(ctx, "Defer size exceeded. Skipping forward to next data element.",
					AttrTag, currentTag.String(), AttrLen, length)
				markElementDeferred(elem, valueTell, length, cc)
			} else {
				logElementValue(ctx, valueTell, value)
				assignElementBytes(elem, value, vr, cc)
			}

			allElements = append(allElements, elem)
		}
		pos += int64(hdrSize) + int64(length)

		if keep && currentTag == TagCharset {
			cc = cc.withCharsets(ParseCharacterSets(elem.Value))
		}
	}

	// Separate file meta (group 0x0002) from dataset
	fileMeta := NewFileMetaDataset()
	ds := NewDataset()

	for _, elem := range allElements {
		if elem.Tag.Group() == 0x0002 {
			fileMeta.Set(elem)
		} else {
			ds.Set(elem)
		}
	}

	if fileMeta.Len() > 0 {
		ts := determineTransferSyntax(fileMeta)
		ds.originalEnc = EncodingInfo{
			IsImplicitVR:   ts.IsImplicitVR(),
			IsLittleEndian: ts.IsLittleEndian(),
		}
	} else {
		ds.originalEnc = cc.EncodingInfo
	}
	propagateEncoding(ds, ds.originalEnc)
	captureOriginalCharsets(ds)

	fd := &FileDataset{
		Dataset:  ds,
		Filename: filename,
		Preamble: preamble,
		FileMeta: fileMeta,
	}
	if modTime != 0 {
		fd.Timestamp = fmt.Sprintf("%d", modTime)
	}
	ds.readCtx = readCtx

	return fd, nil
}

func captureOriginalCharsets(ds *Dataset) {
	if ds == nil {
		return
	}
	ds.originalCharsets = datasetCharacterSets(ds)
	for _, elem := range ds.Iter() {
		if elem.VR != VRSQ {
			continue
		}
		seq, ok := elem.Value.(*Sequence)
		if !ok || seq == nil {
			continue
		}
		for _, item := range seq.Items() {
			captureOriginalCharsets(item)
		}
	}
}

func datasetCharacterSets(ds *Dataset) []string {
	if ds == nil {
		return []string{DefaultCharacterSet}
	}
	if elem, ok := ds.elements[TagCharset]; ok {
		return ParseCharacterSets(elem.Value)
	}
	return []string{DefaultCharacterSet}
}

func propagateEncoding(ds *Dataset, enc EncodingInfo) {
	ds.originalEnc = enc
	for _, elem := range ds.Iter() {
		if elem.VR != VRSQ {
			continue
		}
		seq, ok := elem.Value.(*Sequence)
		if !ok {
			continue
		}
		for _, item := range seq.Items() {
			propagateEncoding(item, enc)
		}
	}
}

func determineTransferSyntaxFromElements(elements []*DataElement) UID {
	for _, elem := range elements {
		if elem.Tag == MustTag(0x00020010) {
			if uid, ok := elem.Value.(UID); ok {
				return uid
			}
			if s, ok := elem.Value.(string); ok {
				return UID(s)
			}
		}
	}
	return ImplicitVRLittleEndian
}

func determineTransferSyntax(fileMeta *FileMetaDataset) UID {
	if elem, ok := fileMeta.Get(MustTag(0x00020010)); ok {
		if uid, ok2 := elem.Value.(UID); ok2 {
			return uid
		}
		if s, ok2 := elem.Value.(string); ok2 {
			return UID(s)
		}
	}
	return ImplicitVRLittleEndian
}

// readSequenceItems reads an undefined-length sequence, stopping at a Sequence
// Delimiter or the first non-Item tag. An error means a diagnostic hook rejected
// something inside the sequence and the whole parse is being abandoned; the
// partial sequence is still returned so callers need not special-case it.
func readSequenceItems(data []byte, offset int64, cc codecContext, opts *ReadOptions, ctx *readContext) (*Sequence, int64, error) {
	seq, newPos, err := readSequenceItemsUntil(data, offset, int64(len(data)), true, cc, opts, ctx)
	seq.IsUndefinedLength = true
	return seq, newPos, err
}

func readDefinedLengthSequence(data []byte, offset int64, length uint32, cc codecContext, opts *ReadOptions, ctx *readContext) (*Sequence, int64, error) {
	return readSequenceItemsUntil(
		data,
		offset,
		offset+int64(length),
		false,
		cc,
		opts,
		ctx,
	)
}

func readSequenceItemsUntil(
	data []byte,
	offset int64,
	end int64,
	undefinedLength bool,
	cc codecContext,
	opts *ReadOptions,
	ctx *readContext,
) (*Sequence, int64, error) {
	seq := NewSequence(nil)
	seq.IsUndefinedLength = undefinedLength
	pos := offset

	for pos+8 <= end && pos+4 <= int64(len(data)) {
		currentTag := readTagBytes(data, pos, cc.IsLittleEndian)

		if currentTag == SequenceDelimiterTag {
			logDebug(ctx.logCtx(), "End of Sequence", AttrOffset, pos, AttrOffsetHex, offsetHex(pos))
			pos += 8
			break
		}

		if currentTag != ItemTag {
			break
		}

		// Everything from here to the end of this iteration belongs to one item of
		// this sequence: the one about to be appended, so its index is the count
		// already there. Naming it is the difference between "something in this
		// forty-item sequence is wrong" and knowing which one.
		ctx.setItem(seq.Len())

		if pos+8 > int64(len(data)) {
			// The item header runs past the buffer: the enclosing element's
			// length claimed more bytes than the file holds.
			err := ctx.report(Diagnostic{
				Kind:   DiagnosticTruncatedItem,
				Tag:    currentTag,
				Offset: pos,
				Need:   8,
				Have:   int64(len(data)) - pos,
			})
			return seq, pos, err
		}

		var itemLength uint32
		if cc.IsLittleEndian {
			itemLength = binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		} else {
			itemLength = binary.BigEndian.Uint32(data[pos+4 : pos+8])
		}
		pos += 8

		logDebug(ctx.logCtx(), "Found Item tag (start of item)",
			AttrOffset, pos-8,
			AttrOffsetHex, offsetHex(pos-8),
			AttrHex, bytesHex(data[pos-8:pos]),
			AttrLen, itemLength,
			AttrUndefined, itemLength == 0xFFFFFFFF,
		)

		item := NewDataset()
		item.parent = seq
		item.readCtx = ctx

		if itemLength == 0xFFFFFFFF {
			item.IsUndefinedLengthSequenceItem = true
			var err error
			pos, err = readDatasetElements(data, pos, int64(len(data)), item, cc, opts, ctx)
			if err != nil {
				return seq, pos, err
			}
		} else if itemLength > 0 {
			itemEnd := pos + int64(itemLength)
			var err error
			pos, err = readDatasetElements(data, pos, itemEnd, item, cc, opts, ctx)
			if err != nil {
				return seq, pos, err
			}
			if pos < itemEnd {
				pos = itemEnd
			}
			logDebug(ctx.logCtx(), "Finished sequence item", AttrOffset, pos, AttrOffsetHex, offsetHex(pos))
		} else {
			logDebug(ctx.logCtx(), "Finished sequence item", AttrOffset, pos, AttrOffsetHex, offsetHex(pos))
		}

		seq.Append(item)
	}

	return seq, pos, nil
}

func readDatasetElements(data []byte, offset int64, end int64, ds *Dataset, cc codecContext, opts *ReadOptions, ctx *readContext) (int64, error) {
	ds.readCtx = ctx
	if len(cc.Charsets) == 0 {
		cc.Charsets = []string{DefaultCharacterSet}
	}
	pos := offset
	resolve := datasetResolver(ctx, ds)

	for pos+4 <= end && pos+4 <= int64(len(data)) {
		currentTag := readTagBytes(data, pos, cc.IsLittleEndian)

		if currentTag == ItemDelimiterTag || currentTag == SequenceDelimiterTag {
			return pos + 8, nil
		}

		h, need, ok := decodeElementHeader(data, pos, currentTag, cc.EncodingInfo, resolve)
		if !ok {
			if err := ctx.report(truncatedHeader(currentTag, pos, need, int64(len(data)))); err != nil {
				return pos, err
			}
			break
		}
		vr, length, hdrSize := h.VR, h.Length, h.Size

		action := decideElement(opts, ctx, h, currentTag, pos+int64(hdrSize), cc.EncodingInfo)
		if action == ElementStop {
			return pos, nil
		}
		keep := action == ElementKeep

		if err := ctx.reportVRMismatch(currentTag, vr, pos, cc.IsImplicitVR); err != nil {
			return pos, err
		}

		logElementHeader(ctx.logCtx(), pos, data[pos:pos+int64(hdrSize)], currentTag, vr, length)

		elem := NewDataElement(currentTag, vr, nil)

		if length == 0 {
			elem.Value = emptyValueForVR(vr)
			pos += int64(hdrSize)
			if keep {
				ds.Set(elem)
			}
			continue
		}

		if length == 0xFFFFFFFF {
			elem.IsUndefinedLength = true
			valueStart := pos + int64(hdrSize)
			if shouldReadUndefinedLengthAsSequence(vr) {
				if vr == VRUN {
					elem.VR = VRSQ
				}
				logDebug(ctx.logCtx(), "Reading/parsing undefined length sequence",
					AttrOffset, valueStart, AttrOffsetHex, offsetHex(valueStart), AttrTag, currentTag.String())
				ctx.pushSeq(currentTag)
				seq, newPos, err := readSequenceItems(data, valueStart, cc, opts, ctx)
				ctx.popSeq()
				elem.Value = seq
				pos = newPos
				if err != nil {
					return pos, err
				}
			} else if !keep {
				pos = skipUndefinedLengthValue(data, valueStart, cc.IsLittleEndian)
			} else {
				logDebug(ctx.logCtx(), "Reading undefined length data element",
					AttrOffset, valueStart, AttrOffsetHex, offsetHex(valueStart), AttrTag, currentTag.String())
				if encapsulated, endPos, ok := readEncapsulatedPixelData(data, valueStart, cc.IsLittleEndian); ok {
					if shouldDeferElement(currentTag, uint32(len(encapsulated)), readDeferSize(opts)) {
						logDebug(ctx.logCtx(), "Defer size exceeded. Skipping forward to next data element.",
							AttrTag, currentTag.String(), AttrLen, len(encapsulated))
						markElementDeferred(elem, valueStart, uint32(len(encapsulated)), cc)
					} else {
						logElementValue(ctx.logCtx(), valueStart, encapsulated)
						assignElementBytes(elem, encapsulated, vr, cc)
					}
					pos = endPos
				} else {
					raw, newPos := readBytesUntilDelimiter(data, valueStart, SequenceDelimiterTag, cc.IsLittleEndian)
					logElementValue(ctx.logCtx(), valueStart, raw)
					elem.RawValue = raw
					pos = newPos
				}
			}
			if keep {
				ds.Set(elem)
			}
			continue
		}

		if vr == VRSQ {
			if keep {
				ctx.pushSeq(currentTag)
				seq, newPos, err := readDefinedLengthSequence(
					data,
					pos+int64(hdrSize),
					length,
					cc,
					opts,
					ctx,
				)
				ctx.popSeq()
				elem.Value = seq
				pos = newPos
				if err != nil {
					return pos, err
				}
				ds.Set(elem)
			} else {
				pos += int64(hdrSize) + int64(length)
			}
			continue
		}

		if pos+int64(hdrSize)+int64(length) > int64(len(data)) {
			if err := ctx.report(truncatedValue(currentTag, vr, pos+int64(hdrSize), int64(length), int64(len(data)))); err != nil {
				return pos, err
			}
			break
		}

		if keep {
			value := data[pos+int64(hdrSize) : pos+int64(hdrSize)+int64(length)]
			valueTell := pos + int64(hdrSize)

			if shouldDeferElement(currentTag, length, readDeferSize(opts)) {
				logDebug(ctx.logCtx(), "Defer size exceeded. Skipping forward to next data element.",
					AttrTag, currentTag.String(), AttrLen, length)
				markElementDeferred(elem, valueTell, length, cc)
			} else {
				logElementValue(ctx.logCtx(), valueTell, value)
				assignElementBytes(elem, value, vr, cc)
			}

			ds.Set(elem)
		}
		pos += int64(hdrSize) + int64(length)

		if keep && currentTag == TagCharset {
			cc = cc.withCharsets(ParseCharacterSets(elem.Value))
		}
	}

	return pos, nil
}

func shouldReadUndefinedLengthAsSequence(vr VR) bool {
	if vr == VRSQ || vr == "" {
		return true
	}
	// PS3.5 6.2.2: undefined-length UN values are encoded as sequences.
	// Private tags resolve to UN via LookupVR and follow the same rule.
	if vr == VRUN {
		return true
	}
	return false
}

// skipUndefinedLengthValue reports where the element after an undefined-length
// value at offset begins, without materialising the value. It mirrors the two
// shapes the reading path handles: a well-formed item stream (PS3.5 A.4), and
// anything else, where the only thing to go on is the Sequence Delimiter.
func skipUndefinedLengthValue(data []byte, offset int64, isLittleEndian bool) int64 {
	if _, endPos, ok := scanEncapsulatedPixelData(data, offset, isLittleEndian); ok {
		return endPos
	}
	return scanUntilDelimiter(data, offset, SequenceDelimiterTag, isLittleEndian)
}

func readBytesUntilDelimiter(data []byte, offset int64, delimiter Tag, isLittleEndian bool) (value []byte, endPos int64) {
	pos := offset
	for pos+4 <= int64(len(data)) {
		if readTagBytes(data, pos, isLittleEndian) == delimiter {
			return append([]byte(nil), data[offset:pos]...), pos + 8
		}
		pos++
	}
	return append([]byte(nil), data[offset:pos]...), pos
}

// scanUntilDelimiter is readBytesUntilDelimiter without the copy: it returns
// only where the next element begins.
func scanUntilDelimiter(data []byte, offset int64, delimiter Tag, isLittleEndian bool) int64 {
	pos := offset
	for pos+4 <= int64(len(data)) {
		if readTagBytes(data, pos, isLittleEndian) == delimiter {
			return pos + 8
		}
		pos++
	}
	return pos
}

// readEncapsulatedPixelData reads undefined-length encapsulated pixel data
// (PS3.5 A.4) as a contiguous item stream ending before the sequence delimiter.
func readEncapsulatedPixelData(data []byte, offset int64, isLittleEndian bool) (value []byte, endPos int64, ok bool) {
	valueEnd, endPos, ok := scanEncapsulatedPixelData(data, offset, isLittleEndian)
	if !ok {
		return nil, offset, false
	}
	return append([]byte(nil), data[offset:valueEnd]...), endPos, true
}

// scanEncapsulatedPixelData walks the item stream readEncapsulatedPixelData
// decodes and reports where the value ends and where the next element begins,
// without copying the items. Skipping an encapsulated element -- a caller after
// the index tags of a file whose Pixel Data is compressed -- needs the second
// number and nothing else, and that value is the largest in the file.
func scanEncapsulatedPixelData(data []byte, offset int64, isLittleEndian bool) (valueEnd, endPos int64, ok bool) {
	pos := offset
	for pos+4 <= int64(len(data)) {
		tag := readTagBytes(data, pos, isLittleEndian)
		if tag == SequenceDelimiterTag {
			if pos+8 > int64(len(data)) {
				return 0, offset, false
			}
			return pos, pos + 8, true
		}
		if tag != ItemTag {
			return 0, offset, false
		}
		if pos+8 > int64(len(data)) {
			return 0, offset, false
		}
		var itemLen uint32
		if isLittleEndian {
			itemLen = binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		} else {
			itemLen = binary.BigEndian.Uint32(data[pos+4 : pos+8])
		}
		if itemLen == 0xFFFFFFFF {
			return 0, offset, false
		}
		pos += 8 + int64(itemLen)
		if pos > int64(len(data)) {
			return 0, offset, false
		}
	}
	return 0, offset, false
}

func cloneElementBytes(value []byte) []byte {
	return append([]byte(nil), value...)
}

func assignElementBytes(elem *DataElement, value []byte, vr VR, cc codecContext) {
	elem.RawValue = cloneElementBytes(value)
	var decodeCharsets []string
	if vrUsesCharacterSet(vr) {
		decodeCharsets = cc.Charsets
	}
	raw := &RawDataElement{
		Tag:            elem.Tag,
		VR:             vr,
		Length:         uint32(len(value)),
		Value:          value,
		IsImplicitVR:   cc.IsImplicitVR,
		IsLittleEndian: cc.IsLittleEndian,
		IsRaw:          true,
	}
	converted, err := convertValueWithCharsets(raw, decodeCharsets)
	if err != nil {
		elem.Value = value
		return
	}
	elem.Value = converted
}

// ReadFile reads a DICOM file from filename.
func ReadFile(filename string, opts *ReadOptions) (*FileDataset, error) {
	return ReadFileContext(context.Background(), filename, opts)
}

// ReadFileContext is like ReadFile but uses ctx for cancellation and logging.
func ReadFileContext(ctx context.Context, filename string, opts *ReadOptions) (*FileDataset, error) {
	return readFile(ctx, filename, opts)
}
