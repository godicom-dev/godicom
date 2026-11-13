package godicom

import "bytes"

// Hooks carries the two callbacks pydicom spells "raw_element_vr" and
// "raw_element_value" in its hooks module (3.0), reshaped for the way godicom
// reads: pydicom registers them on a process-global singleton, while godicom
// takes them per read, on ReadOptions.Hooks, because process-global state is
// what the private dictionary support exists to avoid.
//
// Both are middleware rather than replacement: each receives the function
// godicom would otherwise call, and a hook that wants the default behaviour
// calls it. That is the wrapper shape pydicom's own convenience callbacks
// (raw_element_value_fix_separator, raw_element_value_retry) take -- they run
// their fix-up and then call the default raw_element_value -- so the Go form
// makes every hook as composable as those two are.
//
// Where pydicom passes a kwargs dict (raw_element_kwargs), Go closures close
// over their configuration, so there is nothing to register: build the hook
// with what it needs.
//
// Neither hook fires when the encoding itself named the VR and the value
// converted. ResolveVR is the reader asking the only question it cannot answer
// alone -- what VR does this element have when the file did not say -- and
// ConvertValue is the conversion of the bytes the header promised. pydicom's
// callbacks see every raw element conversion, including the ones where the
// answer was already on the wire; here an explicit VR is the file speaking,
// and godicom keeps what the file says, the same rule the VR-mismatch
// diagnostic follows.
type Hooks struct {
	// ResolveVR determines the VR for an element whose encoding did not carry
	// one: implicit VR, or an explicit header whose VR bytes are not two
	// uppercase letters (pydicom issues 1067 and 1035). raw.VR is empty -- the
	// encoding said nothing, which is the question being asked -- and Value is
	// nil, because only the header has been read when the hook runs; Length is
	// the declared value length. resolve is what godicom would otherwise do:
	// the data dictionary, the private dictionary the read was given, the
	// private creator scan, and the UN fallback, in that order.
	//
	// The hook does not fire on a deferred reload. A reload resolves the VR
	// only to compare it against the one the original read recorded, and a
	// hook that answered differently would turn every deferred private
	// element into a mismatch error.
	ResolveVR func(raw RawDataElement, resolve func() VR) VR

	// ConvertValue converts the encoded bytes of one element to the Go value
	// its VR calls for. raw.VR is the VR in effect -- resolved, for the cases
	// ResolveVR answers -- and raw.Value is the encoded bytes; charsets is the
	// SpecificCharacterSet in effect, for the text VRs. convert is the
	// conversion godicom would otherwise do.
	//
	// A hook may edit raw.Value before calling convert -- FixSeparatorValue
	// works exactly that way -- but the bytes elem.RawValue keeps are cloned
	// before the hook runs, so an edit changes the converted value without
	// touching the record of what the file said.
	//
	// It does not fire on a deferred reload, which passes no read context and
	// keeps the historical fallback, and it does not fire on the character set
	// reinterpretation pass, which re-decodes bytes that already converted
	// once and carries its own error handling.
	ConvertValue func(raw *RawDataElement, charsets []string, convert func() (any, error)) (any, error)
}

// convertElementValue converts raw's bytes, running the read's ConvertValue
// hook when there is one. hooks may be nil -- a deferred load passes nil, and
// keeps the historical fallback.
func convertElementValue(raw *RawDataElement, charsets []string, hooks *Hooks) (any, error) {
	if hooks != nil && hooks.ConvertValue != nil {
		return hooks.ConvertValue(raw, charsets, func() (any, error) {
			return convertValueWithCharsets(raw, charsets)
		})
	}
	return convertValueWithCharsets(raw, charsets)
}

// FixSeparatorValue returns a ConvertValue middleware that replaces separator
// with the ASCII backslash (0x5C) a DICOM multi-value is built from, in the
// encoded bytes of the target VRs, before converting. pydicom spells it
// raw_element_value_fix_separator, for the files whose DS or IS values use
// some other character as the separator:
//
//	opts := &ReadOptions{Hooks: &Hooks{ConvertValue: FixSeparatorValue(":", VRDS, VRIS)}}
//
// An element whose VR is not among targetVRs passes through untouched, and a
// value holding no separator converts as it would have anyway.
func FixSeparatorValue(separator string, targetVRs ...VR) func(*RawDataElement, []string, func() (any, error)) (any, error) {
	targets := make(map[VR]bool, len(targetVRs))
	for _, vr := range targetVRs {
		targets[vr] = true
	}
	return func(raw *RawDataElement, _ []string, convert func() (any, error)) (any, error) {
		if targets[raw.VR] && len(raw.Value) > 0 && bytes.Contains(raw.Value, []byte(separator)) {
			raw.Value = bytes.ReplaceAll(raw.Value, []byte(separator), []byte{0x5C})
			raw.Length = uint32(len(raw.Value))
		}
		return convert()
	}
}

// RetryValue returns a ConvertValue middleware that, when the conversion of an
// element fails, tries the alternative VRs its own VR maps to and reports the
// first that works. pydicom spells it raw_element_value_retry, for the files
// whose element data is written under a VR that cannot describe it:
//
//	opts := &ReadOptions{Hooks: &Hooks{ConvertValue: RetryValue(map[VR][]VR{
//		VRDS: {VRUS, VRSS},
//	})}}
//
// On success the element's VR becomes the one that converted, as pydicom's
// data["VR"] does, so the value and the name it is read under agree. When no
// alternative converts either, the original value and error come back
// unchanged. The alternative conversion runs with the same character set in
// effect, which pydicom's convert_value call does not pass on -- for the US
// and SS alternatives it documents that changes nothing, and here it changes
// nothing only by luck rather than by construction.
func RetryValue(alternatives map[VR][]VR) func(*RawDataElement, []string, func() (any, error)) (any, error) {
	return func(raw *RawDataElement, charsets []string, convert func() (any, error)) (any, error) {
		value, err := convert()
		if err == nil {
			return value, nil
		}
		for _, candidate := range alternatives[raw.VR] {
			alt := *raw
			alt.VR = candidate
			if value, altErr := convertValueWithCharsets(&alt, charsets); altErr == nil {
				raw.VR = candidate
				return value, nil
			}
		}
		return value, err
	}
}
