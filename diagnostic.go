package godicom

import (
	"fmt"
	"strings"
)

// DiagnosticKind classifies a parse anomaly reported to
// ReadOptions.OnDiagnostic.
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
)

// Diagnostic describes an anomaly found while parsing that the reader recovered
// from by keeping what it had already parsed and stopping there. Set
// ReadOptions.OnDiagnostic to observe them.
//
// Diagnostic implements error, so returning the diagnostic itself from the hook
// turns any anomaly into a read failure:
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
	// the inflated bytes, not the file.
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
	fmt.Fprintf(&b, " (offset %d/%s)", d.Offset, offsetHex(d.Offset))
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
