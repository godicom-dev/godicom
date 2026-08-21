package godicom

import (
	"context"
	"fmt"
	"strings"
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

	// DiagnosticInvalidValue: a value handed to the writer cannot be spelled the
	// way its VR requires, so writing it produces a file godicom's own reader
	// would raise a diagnostic on. Raised while writing, not parsing, so it
	// carries no offset.
	DiagnosticInvalidValue DiagnosticKind = "invalid_value"
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

	// VR is the VR in effect for Tag, empty when it was never determined.
	VR VR

	// Offset is the byte offset, in the dataset being parsed, where the
	// anomaly starts. For a Deflated transfer syntax this is an offset into
	// the inflated bytes, not the file. It is zero for a diagnostic raised
	// while writing, which has no source to point into.
	Offset int64

	// Path holds the tags of the enclosing sequences, outermost first. It is
	// nil for an anomaly in the top-level dataset.
	Path []Tag

	// Need and Have are the byte counts the encoding called for and the byte
	// counts actually available. Both are zero when the anomaly is not about
	// a length.
	Need int64
	Have int64

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
	}
	if !d.Kind.raisedWhileWriting() {
		fmt.Fprintf(&b, " (offset %d/%s)", d.Offset, offsetHex(d.Offset))
	}
	if len(d.Path) > 0 {
		parts := make([]string, len(d.Path))
		for i, t := range d.Path {
			parts[i] = t.String()
		}
		fmt.Fprintf(&b, " in %s", strings.Join(parts, " > "))
	}
	if d.Need != 0 || d.Have != 0 {
		fmt.Fprintf(&b, ": need %d bytes, have %d", d.Need, d.Have)
	}
	if d.Err != nil {
		fmt.Fprintf(&b, ": %v", d.Err)
	}
	return b.String()
}

func (d Diagnostic) Unwrap() error { return d.Err }

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

// report logs d and offers it to the OnDiagnostic hook, returning whatever the
// hook returns. A nil return means "keep what was parsed so far", which is also
// the behaviour when no hook is set. A non-nil return aborts the parse, so every
// caller propagates it.
func (rc *readContext) report(d Diagnostic) error {
	if rc == nil {
		return nil
	}
	if len(rc.seqPath) > 0 {
		d.Path = append([]Tag(nil), rc.seqPath...)
	}
	d.Offset += rc.baseOffset
	logDiagnostic(rc.logCtx(), d)
	if rc.onDiag == nil {
		return nil
	}
	return rc.onDiag(d)
}

// pushSeq records that parsing has descended into sequence tag t, so
// diagnostics raised inside it carry the enclosing path.
func (rc *readContext) pushSeq(t Tag) {
	if rc != nil {
		rc.seqPath = append(rc.seqPath, t)
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
		d.Path = append([]Tag(nil), st.seqPath...)
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
		st.seqPath = append(st.seqPath, t)
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
