package godicom

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// DefaultMaxInvalidCharacterDataDiagnostics is how many
// DiagnosticInvalidCharacterData the reader reports element by element for one
// dataset before it collapses the rest into a single summary. It is the value
// ReadOptions.MaxInvalidCharacterDataDiagnostics of zero resolves to.
//
// Eight is arbitrary but not accidental: it is enough to show a caller the shape
// of the damage -- which VRs, which sequences, whether it is one value or the
// whole file -- and small enough that a study with three thousand bad elements
// does not produce three thousand structs and three thousand log lines.
const DefaultMaxInvalidCharacterDataDiagnostics = 8

// errInvalidCharacterDataSuppressed is the Err of the summary diagnostic that
// stands for every invalid_character_data the cap held back. It names the option
// that lifts the cap, because the caller who reads this message is the one who
// wants every element.
var errInvalidCharacterDataSuppressed = errors.New(
	"more elements could not be decoded; set ReadOptions.MaxInvalidCharacterDataDiagnostics to a negative value to report each one",
)

// DiagnosticKind classifies an anomaly reported to ReadOptions.OnDiagnostic or
// WriteOptions.OnDiagnostic.
type DiagnosticKind string

const (
	// DiagnosticTruncatedHeader: the stream ends inside a data element header,
	// so the element and everything after it cannot be parsed.
	DiagnosticTruncatedHeader DiagnosticKind = "truncated_header"

	// DiagnosticTruncatedValue: a data element declares more value bytes than
	// the stream holds. Either the file is cut short or the length field is
	// wrong; the reader cannot tell which.
	DiagnosticTruncatedValue DiagnosticKind = "truncated_value"

	// DiagnosticTruncatedItem: the stream ends inside a sequence item header.
	DiagnosticTruncatedItem DiagnosticKind = "truncated_item"

	// DiagnosticDeferredValueUnreadable: an element parsed as deferred could not
	// be loaded when its value was finally requested. The tag stays visible to
	// SortedTags and Elements while Get reports it as absent.
	DiagnosticDeferredValueUnreadable DiagnosticKind = "deferred_value_unreadable"

	// DiagnosticVRMismatch: an explicit VR element carries a VR the data
	// dictionary cannot reconcile with its tag. Nothing about the parse changes
	// -- godicom keeps what the file said, because what the file says is what
	// the file means -- so this is pure information, and it is the single most
	// useful thing to know when a real device will not accept your output.
	DiagnosticVRMismatch DiagnosticKind = "vr_mismatch"

	// DiagnosticInvalidValue: a value handed to the writer cannot be spelled the
	// way its VR requires, so writing it produces a file godicom's own reader
	// would raise a diagnostic on. Raised while writing, not parsing, so it
	// carries no offset.
	DiagnosticInvalidValue DiagnosticKind = "invalid_value"

	// DiagnosticUnknownCharacterSet: (0008,0005) names a character set godicom
	// does not implement. PS3.5 offers no way to infer what the bytes mean, so
	// the reader keeps its ISO_IR 6 fallback and text comes back as raw bytes
	// rather than decoded text -- a GB18030 Chinese name becomes mojibake. The
	// fallback stays for compatibility, but is no longer silent: the Err holds
	// every unresolved value exactly as it appeared, with its index.
	//
	// Raised once per (0008,0005) element, not once per affected text element.
	// An absent or empty (0008,0005) is legal per PS3.5 and never raises this.
	DiagnosticUnknownCharacterSet DiagnosticKind = "unknown_character_set"

	// DiagnosticInvalidCharacterData: a text VR could not be converted with the
	// character set in effect. The undecoded bytes remain in RawValue, so a
	// strict caller can reject the dataset without losing the source bytes.
	//
	// Reported for at most ReadOptions.MaxInvalidCharacterDataDiagnostics
	// elements -- eight by default -- and then once more for the dataset as a
	// whole, with Count holding how many elements that summary stands for. One
	// dirty study fails thousands of elements, and the first few say everything
	// the rest would; a caller who really wants each one sets the option to a
	// negative value.
	DiagnosticInvalidCharacterData DiagnosticKind = "invalid_character_data"

	// DiagnosticMisdeclaredCharacterSet: a text VR carries bytes outside
	// 0x00-0x7F while the character set in effect is the default repertoire --
	// ISO_IR 6, or no (0008,0005) at all. PS3.5 defines no meaning for those
	// bytes under that declaration, and in practice they are a national encoding
	// that was never declared: a GBK name in a file whose header says ASCII,
	// which is what older Chinese and Japanese equipment and several gateways
	// emit.
	//
	// Raised only when ReadOptions.ReportNonASCIIUnderDefaultCharset is set, and
	// then once per dataset -- the wrong declaration is one fact about the file,
	// and the element named here is the first that showed it. It is off by
	// default because the same bytes also occur in Latin-1 free text that has
	// been archived unremarked for twenty years, so turning it on by default
	// would reject files that read exactly as they always did. pydicom does not
	// detect the case at all: it decodes the default repertoire as Latin-1, which
	// never fails.
	//
	// Detection is not the repair. Dataset.ReinterpretCharacterSet decodes the
	// stored bytes under the set the caller declares, and works whether or not
	// this was reported.
	DiagnosticMisdeclaredCharacterSet DiagnosticKind = "misdeclared_character_set"
)

// raisedWhileWriting reports whether k comes from the writer rather than the
// parser. A write diagnostic has no source offset -- nothing was read, and the
// output position is not something the caller can act on -- so the offset is
// left out of its message and its log line. Extend this alongside any further
// write-side kind.
func (k DiagnosticKind) raisedWhileWriting() bool {
	return k == DiagnosticInvalidValue
}

// Diagnostic describes an anomaly godicom recovered from rather than failed on:
// while parsing, by keeping what it had already read and stopping there; while
// writing, by encoding the value as it stands. Set ReadOptions.OnDiagnostic or
// WriteOptions.OnDiagnostic to observe them.
//
// Diagnostic implements error, so returning the diagnostic itself from the hook
// turns any anomaly into a failure:
//
//	opts := &ReadOptions{OnDiagnostic: func(d Diagnostic) error { return d }}
type Diagnostic struct {
	Kind DiagnosticKind

	// Tag is the element the anomaly concerns. It is the zero Tag when the
	// stream ended before a tag could be read.
	Tag Tag

	// VR is the VR in effect for Tag, empty when it was never determined. For a
	// DiagnosticVRMismatch this is the VR the file actually carried, since that
	// is the one godicom went on to use.
	VR VR

	// ExpectedVR is the VR the data dictionary gives Tag, set only when it
	// disagrees with VR. It is empty for every other kind, and also for a tag
	// the dictionary has no entry for -- an unrecognised or private tag has no
	// expectation to fall short of.
	ExpectedVR VR

	// Offset is the byte offset, in the dataset being parsed, where the
	// anomaly starts. For a Deflated transfer syntax this is an offset into
	// the inflated bytes, not the file. It is zero for a diagnostic raised
	// while writing, which has no source to point into.
	Offset int64

	// Path holds the enclosing sequences and the item of each that was being
	// processed, outermost first. It is nil for an anomaly in the top-level
	// dataset.
	Path []PathStep

	// Need and Have are the byte counts the encoding called for and the byte
	// counts actually available. Both are zero when the anomaly is not about
	// a length.
	Need int64
	Have int64

	// Count is how many elements the diagnostic stands for, set only on a
	// summary that collapsed a run of them: the invalid_character_data reported
	// once a dataset has produced more than its cap allows. Tag, VR, Offset and
	// Path then describe the first element the summary covers. It is zero on a
	// diagnostic that concerns exactly one element, which is every other case.
	Count int

	// Err is the underlying cause, when the anomaly wraps one.
	Err error
}

func (d Diagnostic) Error() string {
	var b strings.Builder
	b.WriteString("godicom: ")
	b.WriteString(string(d.Kind))
	if d.Tag != 0 {
		fmt.Fprintf(&b, " at %s", d.Tag)
		if d.VR != "" {
			fmt.Fprintf(&b, " %s", d.VR)
		}
		if d.ExpectedVR != "" {
			fmt.Fprintf(&b, ", dictionary says %s", d.ExpectedVR)
		}
	}
	if !d.Kind.raisedWhileWriting() {
		fmt.Fprintf(&b, " (offset %d/%s)", d.Offset, offsetHex(d.Offset))
	}
	if len(d.Path) > 0 {
		parts := make([]string, len(d.Path))
		for i, step := range d.Path {
			parts[i] = step.String()
		}
		fmt.Fprintf(&b, " in %s", strings.Join(parts, " > "))
	}
	if d.Need != 0 || d.Have != 0 {
		fmt.Fprintf(&b, ": need %d bytes, have %d", d.Need, d.Have)
	}
	if d.Count > 0 {
		fmt.Fprintf(&b, " for %d elements", d.Count)
	}
	if d.Err != nil {
		fmt.Fprintf(&b, ": %v", d.Err)
	}
	return b.String()
}

func (d Diagnostic) Unwrap() error { return d.Err }

// PathStep names one hop on the way down to a nested element: the sequence
// descended into, and which of its items. PS3.5 gives sequence items an ordinal
// position and nothing else to name them by, so without the index two items of
// the same sequence produce diagnostics that read identically -- which is no help
// at all when one item of forty is the malformed one.
type PathStep struct {
	// Tag is the sequence element descended into.
	Tag Tag

	// Item is the zero-based index of the item being processed, or -1 when the
	// sequence has been entered but no item has: the item header itself was
	// unreadable, or the anomaly concerns the sequence rather than any one item.
	Item int
}

// String renders the step the way the DICOM tooling convention does,
// "(0040,0100)[0]", dropping the subscript when there is no item to name.
func (p PathStep) String() string {
	if p.Item < 0 {
		return p.Tag.String()
	}
	return fmt.Sprintf("%s[%d]", p.Tag, p.Item)
}

// truncatedHeader describes a header of need bytes starting at pos that runs
// past total.
func truncatedHeader(tag Tag, pos, need, total int64) Diagnostic {
	return Diagnostic{
		Kind:   DiagnosticTruncatedHeader,
		Tag:    tag,
		Offset: pos,
		Need:   need,
		Have:   total - pos,
	}
}

// truncatedValue describes a value of need bytes starting at valueStart that
// runs past total.
func truncatedValue(tag Tag, vr VR, valueStart, need, total int64) Diagnostic {
	return Diagnostic{
		Kind:   DiagnosticTruncatedValue,
		Tag:    tag,
		VR:     vr,
		Offset: valueStart,
		Need:   need,
		Have:   total - valueStart,
	}
}

// reportVRMismatch offers a disagreement between the VR encoded for tag at pos
// and the VR the dictionary this read was given gives it. Nothing about the parse
// changes, so unlike the truncation diagnostics this one is pure information.
//
// Judged against the same dictionary the header decoder resolved against, not
// always PS3.6: a caller who overrode an entry through ReadOptions.Dictionary said
// what they expect the file to contain, and a diagnostic measuring against a
// different expectation than the parse used would be reporting on nothing.
//
// Because it is only information, nobody who is not listening should pay for it:
// the dictionary lookup runs once per explicit VR element, which is once per
// element in most files, so it is skipped unless a hook is set or warn-level
// logging is on. That keeps the invariant that every diagnostic godicom raises
// is also logged, without charging the quiet default path for it.
//
// Implicit VR is not checked at all: it carries no VR of its own, so the value
// decodeElementHeader resolved came from this same dictionary and cannot differ
// from it.
func (rc *readContext) reportVRMismatch(tag Tag, encoded VR, pos int64, isImplicitVR bool) error {
	if rc == nil || isImplicitVR {
		return nil
	}
	if rc.onDiag == nil {
		ctx := rc.logCtx()
		if !LoggerFromContext(ctx).Enabled(ctx, slog.LevelWarn) {
			return nil
		}
	}
	want := vrDisagreesWithDictionary(rc.dictionary(), tag, encoded)
	if want == "" {
		return nil
	}
	return rc.report(Diagnostic{
		Kind:       DiagnosticVRMismatch,
		Tag:        tag,
		VR:         encoded,
		ExpectedVR: want,
		Offset:     pos,
	})
}

// reportUnknownCharacterSets offers the unresolved values of one (0008,0005)
// element to OnDiagnostic. The reader still applies the ISO_IR 6 fallback it
// always did, so without a hook nothing about the parse changes -- but the
// anomaly is logged, and a hook that returns the diagnostic refuses a dataset
// whose text it cannot decode. Reporting once here, rather than once per text
// element, keeps the signal to one diagnostic per declaration.
//
// As in reportVRMismatch, the diagnostic is not even built when nobody is
// listening and warn-level logging is off.
func (rc *readContext) reportUnknownCharacterSets(value interface{}, valueStart int64) error {
	if rc == nil {
		return nil
	}
	_, unknown := parseCharacterSetsChecked(value)
	if unknown == nil {
		return nil
	}
	if rc.onDiag == nil {
		ctx := rc.logCtx()
		if !LoggerFromContext(ctx).Enabled(ctx, slog.LevelWarn) {
			return nil
		}
	}
	return rc.report(Diagnostic{
		Kind:   DiagnosticUnknownCharacterSet,
		Tag:    TagCharset,
		VR:     VRCS,
		Offset: valueStart,
		Err:    unknown,
	})
}

// reportInvalidCharacterData offers one text element that could not be decoded
// with the character set in effect, as long as this dataset has not already used
// up ReadOptions.MaxInvalidCharacterDataDiagnostics.
//
// Past the cap the element is counted and dropped, and flushInvalidCharacterData
// reports one summary at the end of the parse. The first few diagnostics say
// everything the rest would -- same Kind, same cause, different tags -- so a
// caller auditing an archive gets a readable report instead of one line per bad
// element, and a caller building a data structure does not allocate thousands of
// Diagnostics it will throw away.
func (rc *readContext) reportInvalidCharacterData(tag Tag, vr VR, pos int64, cause error) error {
	if rc == nil {
		return nil
	}
	// A zero limit is the default cap, not "report nothing": readContext is built
	// in several places and the zero value has to mean the sane thing, or a
	// decoder that forgets to set the field silently loses the diagnostic.
	limit := rc.invalidCharLimit
	if limit == 0 {
		limit = DefaultMaxInvalidCharacterDataDiagnostics
	}
	if limit > 0 && rc.invalidCharReported >= limit {
		if rc.invalidCharSuppressed == 0 {
			// Kept as the offset and path of the element itself, already in
			// source coordinates: the summary is raised from the top level, long
			// after the sequence this element was in has been popped, so nothing
			// down there can be recomputed at flush time.
			rc.invalidCharFirst = Diagnostic{
				Tag:    tag,
				VR:     vr,
				Offset: pos + rc.baseOffset,
			}
			if len(rc.seqPath) > 0 {
				rc.invalidCharFirst.Path = append([]PathStep(nil), rc.seqPath...)
			}
		}
		rc.invalidCharSuppressed++
		return nil
	}
	rc.invalidCharReported++
	return rc.report(Diagnostic{
		Kind:   DiagnosticInvalidCharacterData,
		Tag:    tag,
		VR:     vr,
		Offset: pos,
		Err:    cause,
	})
}

// flushInvalidCharacterData reports what the cap held back, once, when the
// top-level parse ends. It is a no-op when nothing was suppressed, which is the
// case for every file whose text all decodes.
//
// rc.baseOffset is zero here by construction -- every setBase is undone on the
// way out of the function that made it -- so the offset recorded with the first
// suppressed element is already final and rc.report adds nothing to it.
func (rc *readContext) flushInvalidCharacterData() error {
	if rc == nil || rc.invalidCharSuppressed == 0 {
		return nil
	}
	suppressed := rc.invalidCharSuppressed
	rc.invalidCharSuppressed = 0
	d := rc.invalidCharFirst
	d.Kind = DiagnosticInvalidCharacterData
	d.Count = suppressed
	d.Err = errInvalidCharacterDataSuppressed
	return rc.report(d)
}

// checkDefaultCharacterSet offers one text value to OnDiagnostic when it carries
// bytes the default repertoire does not define. Off unless the caller asked for
// it, and then once per dataset: the declaration is wrong or it is not, and the
// element that proves it is this one.
//
// Nothing about the value changes. The bytes are already in RawValue and the
// element value is the same string it was before -- what the caller gains is a
// way to refuse, quarantine, or re-declare the file, which is what
// Dataset.ReinterpretCharacterSet is for.
func (rc *readContext) checkDefaultCharacterSet(tag Tag, vr VR, pos int64, value []byte, charsets []string) error {
	if rc == nil || !rc.reportDefaultCharset || rc.misdeclaredReported {
		return nil
	}
	if !vrUsesCharacterSet(vr) || !defaultCharacterSetInEffect(charsets) {
		return nil
	}
	high := 0
	for _, b := range value {
		if b >= 0x80 {
			high++
		}
	}
	if high == 0 {
		return nil
	}
	rc.misdeclaredReported = true
	return rc.report(Diagnostic{
		Kind:   DiagnosticMisdeclaredCharacterSet,
		Tag:    tag,
		VR:     vr,
		Offset: pos,
		Err: fmt.Errorf(
			"%d byte(s) above 0x7F, which %s does not define: the value is most likely a national encoding that was never declared",
			high, DefaultCharacterSet),
	})
}

// defaultCharacterSetInEffect reports whether the declared character set is the
// default repertoire, for which PS3.5 defines nothing above 0x7F. An absent or
// empty (0008,0005) is the default too.
func defaultCharacterSetInEffect(charsets []string) bool {
	normalized := ConvertCharacterSets(charsets)
	if len(normalized) == 0 {
		return true
	}
	return isDefaultCharacterSet(normalized[0])
}

// report logs d and offers it to the OnDiagnostic hook, returning whatever the
// hook returns. A nil return means "keep what was parsed so far", which is also
// the behaviour when no hook is set. A non-nil return aborts the parse, so every
// caller propagates it.
func (rc *readContext) report(d Diagnostic) error {
	if rc == nil {
		return nil
	}
	if len(rc.seqPath) > 0 {
		d.Path = append([]PathStep(nil), rc.seqPath...)
	}
	d.Offset += rc.baseOffset
	logDiagnostic(rc.logCtx(), d)
	if rc.onDiag == nil {
		return nil
	}
	return rc.onDiag(d)
}

// pushSeq records that parsing has descended into sequence tag t, so
// diagnostics raised inside it carry the enclosing path. The item is unknown
// until the item loop names it, because the sequence is entered where the
// element is read and the items are walked somewhere else.
func (rc *readContext) pushSeq(t Tag) {
	if rc != nil {
		rc.seqPath = append(rc.seqPath, PathStep{Tag: t, Item: -1})
	}
}

// setItem names which item of the innermost sequence is being processed. The
// item loop calls it once per item, so a diagnostic can say which of forty items
// is the malformed one.
func (rc *readContext) setItem(i int) {
	if rc != nil && len(rc.seqPath) > 0 {
		rc.seqPath[len(rc.seqPath)-1].Item = i
	}
}

func (rc *readContext) popSeq() {
	if rc != nil && len(rc.seqPath) > 0 {
		rc.seqPath = rc.seqPath[:len(rc.seqPath)-1]
	}
}

// setBase declares that the buffer about to be parsed was copied from base in
// the source, so diagnostic offsets can be reported in source coordinates. The
// returned func restores the previous base.
func (rc *readContext) setBase(base int64) func() {
	if rc == nil {
		return func() {}
	}
	prev := rc.baseOffset
	rc.baseOffset = base
	return func() { rc.baseOffset = prev }
}

// report is writeState's counterpart to readContext.report: it logs d and offers
// it to the OnDiagnostic hook, returning whatever the hook returns. A nil return
// means "write the value as it stands", which is also the behaviour when no hook
// is set, so nothing an existing caller writes changes. A non-nil return aborts
// the write, so every caller propagates it.
func (st *writeState) report(d Diagnostic) error {
	if st == nil {
		return nil
	}
	if len(st.seqPath) > 0 {
		d.Path = append([]PathStep(nil), st.seqPath...)
	}
	logDiagnostic(st.logCtx(), d)
	if st.onDiag == nil {
		return nil
	}
	return st.onDiag(d)
}

// pushSeq records that writing has descended into sequence tag t, so diagnostics
// raised inside it carry the enclosing path.
func (st *writeState) pushSeq(t Tag) {
	if st != nil {
		st.seqPath = append(st.seqPath, PathStep{Tag: t, Item: -1})
	}
}

// setItem names which item of the innermost sequence is being written. Unlike the
// reader the writer knows the index at the moment it descends, but it says so
// through the same two calls so both sides speak one vocabulary.
func (st *writeState) setItem(i int) {
	if st != nil && len(st.seqPath) > 0 {
		st.seqPath[len(st.seqPath)-1].Item = i
	}
}

func (st *writeState) popSeq() {
	if st != nil && len(st.seqPath) > 0 {
		st.seqPath = st.seqPath[:len(st.seqPath)-1]
	}
}

func (st *writeState) logCtx() context.Context {
	if st != nil && st.ctx != nil {
		return st.ctx
	}
	return context.Background()
}
