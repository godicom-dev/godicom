package godicom

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

const esc = 0x1b

// dicomEncodings maps a DICOM defined term to the Go codec that decodes it.
// The term list matches pydicom's python_encoding table (pydicom/charset.py,
// PS3.5 Table C.12-2 plus the CP1234 terms) entry for entry, so a file read by
// either library yields the same text; TestCharacterSetTableMatchesPydicom
// fails if a term goes missing.
//
// Three terms decode with a different codec than pydicom uses, all because
// x/text does not ship the codec the standard calls for:
//   - ISO 2022 IR 159: JIS X 0212 under its own escape, but x/text has no
//     ISO-2022-JP-2, so it decodes as JIS X 0208 and fails on 0212-only runes.
//   - ISO 2022 IR 58: keeps HZGB2312 until the ESC$)A prefix is stripped by the
//     decoder. pydicom does not strip it either (it warns "Found unknown escape
//     sequence" and returns the escape verbatim), so this is a shared gap.
//   - ISO_IR 166: Windows-874 rather than TIS-620; they differ only in 0x80-0x9F.
var dicomEncodings = map[string]encoding.Encoding{
	"":                               encoding.Nop,
	"ISO_IR 6":                       encoding.Nop,
	"ISO_IR 13":                      japanese.ShiftJIS,
	"ISO_IR 100":                     charmap.ISO8859_1,
	"ISO_IR 101":                     charmap.ISO8859_2,
	"ISO_IR 109":                     charmap.ISO8859_3,
	"ISO_IR 110":                     charmap.ISO8859_4,
	"ISO_IR 126":                     charmap.ISO8859_7,
	"ISO_IR 127":                     charmap.ISO8859_6,
	"ISO_IR 138":                     charmap.ISO8859_8,
	"ISO_IR 144":                     charmap.ISO8859_5,
	"ISO_IR 148":                     charmap.ISO8859_9,
	"ISO_IR 166":                     charmap.Windows874,
	"ISO 2022 IR 6":                  encoding.Nop,
	"ISO 2022 IR 100":                charmap.ISO8859_1,
	"ISO 2022 IR 101":                charmap.ISO8859_2,
	"ISO 2022 IR 109":                charmap.ISO8859_3,
	"ISO 2022 IR 110":                charmap.ISO8859_4,
	"ISO 2022 IR 126":                charmap.ISO8859_7,
	"ISO 2022 IR 127":                charmap.ISO8859_6,
	"ISO 2022 IR 138":                charmap.ISO8859_8,
	"ISO 2022 IR 144":                charmap.ISO8859_5,
	"ISO 2022 IR 148":                charmap.ISO8859_9,
	"ISO 2022 IR 166":                charmap.Windows874,
	"ISO 2022 IR 13":                 japanese.ShiftJIS,
	"ISO 2022 IR 87":                 japanese.ISO2022JP,
	"ISO 2022 IR 159":                japanese.ISO2022JP,
	"GB18030":                        simplifiedchinese.GB18030,
	"GBK":                            simplifiedchinese.GBK,
	"ISO 2022 GBK":                   simplifiedchinese.GBK,
	"ISO 2022 IR 149":                korean.EUCKR,
	"ISO 2022 IR 58":                 simplifiedchinese.HZGB2312,
	"ISO 2022 58":                    simplifiedchinese.GBK,
	"BIG5":                           traditionalchinese.Big5,
	"ISO 2022 IR 13\\ISO 2022 IR 87": japanese.ShiftJIS,
	"ISO 2022 IR 87\\ISO 2022 IR 13": japanese.ShiftJIS,
	"ISO_IR 192":                     encoding.Nop,
	"ISO 2022 IR 192":                encoding.Nop,
}

var standaloneCharacterSets = map[string]bool{
	"ISO_IR 192": true,
	"GBK":        true,
	"GB18030":    true,
}

// ISO-2022 escape sequences mapped to DICOM character set names (PS3.3 C.12-3/4).
var escapeToCharacterSet = map[string]string{
	"\x1b(B":  "ISO_IR 6",
	"\x1b-A":  "ISO_IR 100",
	"\x1b)I":  "ISO 2022 IR 13",
	"\x1b(J":  "ISO 2022 IR 13",
	"\x1b$B":  "ISO 2022 IR 87",
	"\x1b-B":  "ISO_IR 101",
	"\x1b-C":  "ISO_IR 109",
	"\x1b-D":  "ISO_IR 110",
	"\x1b-F":  "ISO_IR 126",
	"\x1b-G":  "ISO_IR 127",
	"\x1b-H":  "ISO_IR 138",
	"\x1b-L":  "ISO_IR 144",
	"\x1b-M":  "ISO_IR 148",
	"\x1b-T":  "ISO_IR 166",
	"\x1b$)C": "ISO 2022 IR 149",
	"\x1b$(D": "ISO 2022 IR 159",
	"\x1b$)A": "ISO 2022 IR 58",
}

// Encodings where the decoder consumes embedded ISO-2022 escape sequences.
var iso2022HandledEncodings = map[string]bool{
	"ISO 2022 IR 87":  true,
	"ISO 2022 IR 159": true,
	"ISO 2022 IR 58":  true,
}

var characterSetToEscape map[string]string

// characterSetAliases maps spellings that are unambiguously equivalent to a
// DICOM defined term onto the canonical key in dicomEncodings. It is built from
// dicomEncodings itself in init, so every supported term is covered without a
// second hand-maintained list, and a private or misspelled term is never
// mistaken for a near miss.
var characterSetAliases map[string]string

func init() {
	// Build the alias table first: the escape-sequence loop below calls
	// characterSetKey, which resolves names through patchCharacterSetSpelling.
	characterSetAliases = make(map[string]string, len(dicomEncodings)+1)
	for name := range dicomEncodings {
		if name == "" {
			continue
		}
		characterSetAliases[characterSetAliasKey(name)] = name
	}
	// "UTF-8" is not a DICOM defined term, but devices do write it. pydicom
	// lets any registered Python codec name through; this is the one spelling
	// that has exactly one DICOM meaning.
	characterSetAliases[characterSetAliasKey("UTF-8")] = "ISO_IR 192"

	characterSetToEscape = make(map[string]string)
	for escSeq, cs := range escapeToCharacterSet {
		if _, exists := characterSetToEscape[cs]; !exists {
			characterSetToEscape[cs] = escSeq
		}
		key := characterSetKey(cs)
		if _, exists := characterSetToEscape[key]; !exists {
			characterSetToEscape[key] = escSeq
		}
	}
}

// characterSetAliasKey folds case, spaces, underscores and hyphens so that
// "ISOIR192", "iso_ir_192" and "ISO-IR 192" all land on one entry. Interior
// spaces in "ISO 2022 IR 100" disappear the same way on both sides of the
// table, so they are not treated as padding to be stripped from a value.
func characterSetAliasKey(cs string) string {
	return strings.NewReplacer(" ", "", "_", "", "-", "").Replace(strings.ToUpper(cs))
}

var iso2022FragmentRe = regexp.MustCompile(`(?s)(^[^\x1b]+|[\x1b][^\x1b]*)`)

var textVRDelims = map[byte]bool{
	0x0D: true,
	0x0A: true,
	0x09: true,
	0x0C: true,
}

var pnDelims = map[byte]bool{
	'^': true,
}

var DefaultCharacterSet = "ISO_IR 6"

func DecodeString(b []byte, encodingName string) (string, error) {
	enc, ok := dicomEncodings[encodingName]
	if !ok {
		enc = encoding.Nop
	}
	decoder := enc.NewDecoder()
	return decoder.String(string(b))
}

func EncodeString(s string, encodingName string) ([]byte, error) {
	name := resolveCharacterSetName(encodingName)

	// Default / ISO_IR 6 is ASCII-only. encoding.Nop would otherwise accept
	// UTF-8 and prevent fall-through to ISO-2022 extensions.
	if isDefaultCharacterSet(name) {
		for i := 0; i < len(s); i++ {
			if s[i] > 0x7F {
				return nil, fmt.Errorf("godicom: cannot encode %q with character set %q", s, name)
			}
		}
		return []byte(s), nil
	}

	// UTF-8 standalone sets use Nop in dicomEncodings.
	if name == "ISO_IR 192" || name == "ISO 2022 IR 192" {
		return []byte(s), nil
	}

	enc, ok := dicomEncodings[name]
	if !ok {
		enc = encoding.Nop
	}
	encoder := enc.NewEncoder()
	return encoder.Bytes([]byte(s))
}

// DecodeBytes decodes using the first matching encoding (legacy helper).
func DecodeBytes(b []byte, encodings []string) string {
	return DecodeBytesWithDelimiters(b, encodings, nil)
}

// ParseCharacterSets splits a Specific Character Set value into DICOM encoding
// names. Values godicom cannot resolve keep their historical ISO_IR 6 fallback;
// the reader reports them through OnDiagnostic (see reportUnknownCharacterSets),
// so this exported helper stays backwards compatible for existing callers.
func ParseCharacterSets(value interface{}) []string {
	sets, _ := parseCharacterSetsChecked(value)
	return sets
}

// unknownCharacterSet keeps an unresolved value exactly as it appeared, with
// its position in a multi-valued (0008,0005). Index is -1 when the element was
// not a string at all. A diagnostic saying "ISO_IR 6" for a value that was
// "ACME_PRIVATE" would be no help at all, so the original spelling matters.
type unknownCharacterSet struct {
	Name  string
	Index int
}

type unknownCharacterSetsError struct {
	Values []unknownCharacterSet
}

func (e *unknownCharacterSetsError) Error() string {
	if len(e.Values) == 1 {
		return fmt.Sprintf("godicom: unsupported SpecificCharacterSet value %q (index %d)",
			e.Values[0].Name, e.Values[0].Index)
	}
	parts := make([]string, len(e.Values))
	for i, v := range e.Values {
		parts[i] = fmt.Sprintf("%q (index %d)", v.Name, v.Index)
	}
	return "godicom: unsupported SpecificCharacterSet values: " + strings.Join(parts, ", ")
}

// parseCharacterSetsChecked resolves (0008,0005) and collects every value that
// could not be resolved. It never guesses: an unknown or private term stays
// unknown rather than being mapped onto a likely-looking national encoding.
// A missing element, an empty value and trailing CS padding are all legal per
// PS3.5 and are not reported.
func parseCharacterSetsChecked(value interface{}) ([]string, *unknownCharacterSetsError) {
	var raw []string
	switch v := value.(type) {
	case nil:
		return []string{DefaultCharacterSet}, nil
	case string:
		raw = splitCharacterSetString(v)
	case *MultiValue[string]:
		raw = trimCharacterSetValues(v.Values())
	default:
		return []string{DefaultCharacterSet}, &unknownCharacterSetsError{
			Values: []unknownCharacterSet{{Name: fmt.Sprintf("<%T>", value), Index: -1}},
		}
	}
	if len(raw) == 0 {
		raw = []string{""}
	}

	var unknown []unknownCharacterSet
	for i, cs := range raw {
		if strings.Trim(cs, " \x00") == "" {
			continue
		}
		if _, ok := resolveCharacterSetNameOK(cs); !ok {
			unknown = append(unknown, unknownCharacterSet{Name: cs, Index: i})
		}
	}
	normalized := ConvertCharacterSets(raw)
	if len(unknown) == 0 {
		return normalized, nil
	}
	return normalized, &unknownCharacterSetsError{Values: unknown}
}

func splitCharacterSetString(s string) []string {
	if s == "" {
		return []string{""}
	}
	return trimCharacterSetValues(strings.Split(s, "\\"))
}

// trimCharacterSetValues strips CS padding from each value. VR=CS conversion
// only trims the end of the whole string, so "GB18030 \ISO_IR 100" used to hand
// "GB18030 " to the lookup and silently degrade every text value after it.
// pydicom strips each multi-valued CS item; trim only the ends so the interior
// spaces of "ISO 2022 IR 100" survive.
func trimCharacterSetValues(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.Trim(v, " \x00")
	}
	return out
}

// ConvertCharacterSets normalizes DICOM Specific Character Set values for decoding.
func ConvertCharacterSets(values []string) []string {
	if len(values) == 0 {
		return []string{DefaultCharacterSet}
	}
	out := make([]string, len(values))
	copy(out, values)
	if out[0] == "" {
		out[0] = DefaultCharacterSet
	}
	for i, cs := range out {
		out[i] = resolveCharacterSetName(cs)
	}
	if len(out) > 1 {
		out = filterStandaloneExtensions(out)
	}
	return out
}

func resolveCharacterSetName(cs string) string {
	name, ok := resolveCharacterSetNameOK(cs)
	if !ok {
		return DefaultCharacterSet
	}
	return name
}

// resolveCharacterSetNameOK is resolveCharacterSetName with its fallback made
// explicit, so the (0008,0005) reader can tell a supported term from one that
// was silently replaced. A fuzzy spelling correction only counts when its
// result is a key the decoder actually implements.
func resolveCharacterSetNameOK(cs string) (string, bool) {
	if _, ok := dicomEncodings[cs]; ok {
		return cs, true
	}
	if patched := patchCharacterSetSpelling(cs); patched != "" {
		if _, ok := dicomEncodings[patched]; ok {
			return patched, true
		}
	}
	return "", false
}

func patchCharacterSetSpelling(cs string) string {
	// Exact, case-insensitive aliases first. The fuzzy corrections below were
	// written for "ISOIR..." typos: "ISOIR192" used to produce "ISO_IR192",
	// which is not a dicomEncodings key and so silently became a Nop decode.
	if name, ok := characterSetAliases[characterSetAliasKey(cs)]; ok {
		return name
	}
	if len(cs) >= 6 && strings.HasPrefix(cs, "ISO") && !strings.HasPrefix(cs, "ISO_") && !strings.HasPrefix(cs, "ISO ") {
		if strings.HasPrefix(cs, "ISOIR") {
			return "ISO_IR" + cs[5:]
		}
		if strings.HasPrefix(cs, "ISO-IR") {
			return "ISO_IR" + cs[6:]
		}
	}
	if strings.Contains(cs, "2022") && !strings.HasPrefix(cs, "ISO 2022 IR ") {
		idx := strings.Index(cs, "IR")
		if idx >= 0 && idx+2 < len(cs) {
			suffix := strings.TrimLeft(cs[idx+2:], " _-+")
			if suffix != "" {
				return "ISO 2022 IR " + suffix
			}
		}
	}
	return ""
}

func filterStandaloneExtensions(values []string) []string {
	if standaloneCharacterSets[values[0]] {
		return values[:1]
	}
	out := append([]string(nil), values...)
	for i := len(out) - 1; i >= 1; i-- {
		if standaloneCharacterSets[out[i]] {
			out = append(out[:i], out[i+1:]...)
		}
	}
	return out
}

// DecodeBytesWithDelimiters decodes a DICOM text byte string with ISO-2022 code
// extensions. It keeps the historical fallback for undecodable bytes; the
// reader uses decodeBytesWithDelimitersChecked to raise a diagnostic instead of
// letting the error disappear into a plausible-looking string.
func DecodeBytesWithDelimiters(b []byte, encodings []string, delimiters map[byte]bool) string {
	s, _ := decodeBytesWithDelimitersChecked(b, encodings, delimiters)
	return s
}

// decodeBytesWithDelimitersChecked returns the decoded text with the first
// decode error, if any. Every fragment still contributes exactly the string the
// loose wrapper produced before, so the exported behaviour is unchanged.
func decodeBytesWithDelimitersChecked(b []byte, encodings []string, delimiters map[byte]bool) (string, error) {
	encodings = ConvertCharacterSets(encodings)
	if len(b) == 0 {
		return "", nil
	}
	if !bytes.Contains(b, []byte{esc}) {
		return decodeSimpleBytesChecked(b, encodings[0])
	}

	var out strings.Builder
	var firstErr error
	for _, fragment := range iso2022FragmentRe.FindAll(b, -1) {
		s, err := decodeFragmentChecked(fragment, encodings, delimiters)
		out.WriteString(s)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return out.String(), firstErr
}

// decodeStringChecked resolves the encoding by name and rejects byte sequences
// a plain Nop decoder would otherwise pass through untouched. DecodeString has
// to keep its unknown-name fallback for existing callers (charset_test.go
// asserts it), so this is a checked counterpart rather than a change to that
// API. allowReplacement is set for the ISO-2022 "undeclared escape" path, where
// substitution is the intended outcome.
func decodeStringChecked(b []byte, encodingName string, allowReplacement bool) (string, error) {
	enc, ok := dicomEncodings[encodingName]
	if !ok {
		return string(b), &unknownCharacterSetsError{
			Values: []unknownCharacterSet{{Name: encodingName, Index: -1}},
		}
	}
	s, err := enc.NewDecoder().String(string(b))
	if err != nil {
		return string(b), fmt.Errorf("godicom: decode %s: %w", encodingName, err)
	}
	// encoding.Nop (ISO_IR 6 / ISO_IR 192) does not validate UTF-8, and the
	// x/text charmap decoders substitute U+FFFD rather than returning an
	// error. Neither may be allowed to look like a successful decode.
	if !utf8.ValidString(s) {
		return string(b), fmt.Errorf("godicom: invalid %s byte sequence", encodingName)
	}
	if !allowReplacement && strings.ContainsRune(s, utf8.RuneError) {
		return s, fmt.Errorf("godicom: undecodable byte sequence for character set %q", encodingName)
	}
	return s, nil
}

func decodeSimpleBytesChecked(b []byte, encodingName string) (string, error) {
	return decodeStringChecked(b, encodingName, false)
}

func decodeWithReplacementChecked(b []byte, encodingName string) (string, error) {
	return decodeStringChecked(b, encodingName, true)
}

func decodeFragmentChecked(fragment []byte, encodings []string, delimiters map[byte]bool) (string, error) {
	if len(fragment) == 0 {
		return "", nil
	}
	if fragment[0] != esc {
		return decodeSimpleBytesChecked(fragment, encodings[0])
	}
	return decodeEscapedFragmentChecked(fragment, encodings, delimiters)
}

func decodeEscapedFragmentChecked(fragment []byte, encodings []string, delimiters map[byte]bool) (string, error) {
	seqLen := escapeSequenceLength(fragment)
	if seqLen > len(fragment) {
		return decodeSimpleBytesChecked(fragment, encodings[0])
	}

	csName := escapeToCharacterSet[string(fragment[:seqLen])]
	if !encodingAllowed(csName, encodings) {
		return decodeWithReplacementChecked(fragment, encodings[0])
	}
	if iso2022HandledEncodings[csName] || iso2022HandledEncodings[characterSetKey(csName)] {
		return decodeSimpleBytesChecked(fragment, csName)
	}

	payload := fragment[seqLen:]
	decodeName := csName
	if _, ok := dicomEncodings[decodeName]; !ok {
		decodeName = characterSetKey(csName)
	}
	if len(delimiters) == 0 {
		return decodeSimpleBytesChecked(payload, decodeName)
	}

	idx := indexDelimiter(payload, delimiters)
	if idx < 0 {
		return decodeSimpleBytesChecked(payload, decodeName)
	}
	a, errA := decodeSimpleBytesChecked(payload[:idx], decodeName)
	b, errB := decodeSimpleBytesChecked(payload[idx:], encodings[0])
	if errA != nil {
		return a + b, errA
	}
	return a + b, errB
}

func escapeSequenceLength(fragment []byte) int {
	if len(fragment) >= 4 && (bytes.HasPrefix(fragment, []byte{esc, '$', '('}) ||
		bytes.HasPrefix(fragment, []byte{esc, '$', ')'})) {
		return 4
	}
	if len(fragment) >= 3 {
		return 3
	}
	return len(fragment)
}

func characterSetKey(cs string) string {
	cs = resolveCharacterSetName(cs)
	if strings.HasPrefix(cs, "ISO 2022 IR ") {
		return "ISO_IR " + strings.TrimPrefix(cs, "ISO 2022 IR ")
	}
	return cs
}

func isDefaultCharacterSet(cs string) bool {
	switch cs {
	case "", DefaultCharacterSet, "ISO_IR 6", "ISO 2022 IR 6":
		return true
	default:
		return false
	}
}

func encodingAllowed(csName string, encodings []string) bool {
	if csName == "" {
		return false
	}
	if isDefaultCharacterSet(csName) {
		return true
	}
	key := characterSetKey(csName)
	for _, enc := range encodings {
		if characterSetKey(enc) == key {
			return true
		}
	}
	return false
}

func indexDelimiter(b []byte, delimiters map[byte]bool) int {
	for i, ch := range b {
		if delimiters[ch] {
			return i
		}
	}
	return -1
}

func trimRightNullSpaceBytes(b []byte) []byte {
	return bytes.TrimRight(b, " \x00")
}

func vrUsesCharacterSet(vr VR) bool {
	switch vr {
	case VRPN, VRLO, VRLT, VRSH, VRST, VRUT, VRUC:
		return true
	default:
		return false
	}
}

func needsCharsetEncode(encodings []string) bool {
	normalized := ConvertCharacterSets(encodings)
	if len(normalized) > 1 {
		return true
	}
	if len(normalized) == 0 {
		return false
	}
	return !isDefaultCharacterSet(normalized[0])
}

func escapeForCharacterSet(cs string) []byte {
	cs = resolveCharacterSetName(cs)
	if escSeq, ok := characterSetToEscape[cs]; ok {
		return []byte(escSeq)
	}
	if escSeq, ok := characterSetToEscape[characterSetKey(cs)]; ok {
		return []byte(escSeq)
	}
	return nil
}

func isISO2022HandledEncoding(cs string) bool {
	cs = resolveCharacterSetName(cs)
	return iso2022HandledEncodings[cs] || iso2022HandledEncodings[characterSetKey(cs)]
}

// EncodeBytesWithCharsets encodes a Unicode string for DICOM text VRs.
func EncodeBytesWithCharsets(s string, encodings []string) []byte {
	encodings = ConvertCharacterSets(encodings)
	if s == "" {
		return nil
	}
	if !needsCharsetEncode(encodings) {
		return []byte(s)
	}
	if len(encodings) == 1 || standaloneCharacterSets[encodings[0]] {
		b, err := EncodeString(s, encodings[0])
		if err != nil {
			return []byte(s)
		}
		return b
	}
	// Prefer the primary set when it can encode the whole value.
	if b, err := EncodeString(s, encodings[0]); err == nil {
		return b
	}
	// Otherwise encode by longest runs so ISO-2022 escapes are re-emitted
	// after PN/text delimiters (which reset to the initial character set).
	return encodeStringParts(s, encodings)
}

func encodeStringParts(value string, encodings []string) []byte {
	encodings = ConvertCharacterSets(encodings)
	var out []byte
	for _, segment := range splitCharsetEncodeSegments(value) {
		if segment == "^" || segment == "=" || segment == "\\" {
			out = append(out, segment...)
			continue
		}
		out = append(out, encodeCharsetSegment(segment, encodings)...)
	}
	return out
}

// splitCharsetEncodeSegments splits on PN/text delimiters that reset the
// active ISO-2022 repertoire to the initial character set.
func splitCharsetEncodeSegments(value string) []string {
	if value == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '^', '=', '\\':
			if i > start {
				parts = append(parts, value[start:i])
			}
			parts = append(parts, value[i:i+1])
			start = i + 1
		}
	}
	if start < len(value) {
		parts = append(parts, value[start:])
	}
	return parts
}

func encodeCharsetSegment(segment string, encodings []string) []byte {
	if segment == "" {
		return nil
	}
	if b, err := EncodeString(segment, encodings[0]); err == nil {
		return b
	}
	bestPrefix := ""
	bestEnc := encodings[0]
	for _, enc := range encodings[1:] {
		prefix := longestEncodablePrefix(segment, enc)
		if len([]rune(prefix)) > len([]rune(bestPrefix)) {
			bestPrefix = prefix
			bestEnc = enc
		}
	}
	if bestPrefix == "" {
		var out []byte
		for _, r := range segment {
			repl, _ := EncodeString(string(r), encodings[0])
			out = append(out, repl...)
		}
		return out
	}
	if bestPrefix != segment {
		// Mixed within a delimiter segment: fall back to run-length encoding.
		var out []byte
		remaining := segment
		for remaining != "" {
			if prefix := longestEncodablePrefix(remaining, encodings[0]); prefix != "" {
				chunk, _ := EncodeString(prefix, encodings[0])
				out = append(out, chunk...)
				remaining = remaining[len(prefix):]
				continue
			}
			bestPrefix = ""
			bestEnc = encodings[0]
			for _, enc := range encodings[1:] {
				prefix := longestEncodablePrefix(remaining, enc)
				if len([]rune(prefix)) > len([]rune(bestPrefix)) {
					bestPrefix = prefix
					bestEnc = enc
				}
			}
			if bestPrefix == "" {
				r, size := utf8.DecodeRuneInString(remaining)
				if r == utf8.RuneError && size == 0 {
					break
				}
				repl, _ := EncodeString(string(r), encodings[0])
				out = append(out, repl...)
				remaining = remaining[size:]
				continue
			}
			chunk, err := EncodeString(bestPrefix, bestEnc)
			if err != nil {
				break
			}
			if !isISO2022HandledEncoding(bestEnc) {
				if esc := escapeForCharacterSet(bestEnc); len(esc) > 0 {
					out = append(out, esc...)
				}
			}
			out = append(out, chunk...)
			remaining = remaining[len(bestPrefix):]
		}
		return out
	}
	chunk, err := EncodeString(bestPrefix, bestEnc)
	if err != nil {
		return []byte(segment)
	}
	if !isISO2022HandledEncoding(bestEnc) {
		if esc := escapeForCharacterSet(bestEnc); len(esc) > 0 {
			return append(append([]byte(nil), esc...), chunk...)
		}
	}
	return chunk
}

func longestEncodablePrefix(s, encName string) string {
	var b strings.Builder
	for _, r := range s {
		trial := b.String() + string(r)
		if _, err := EncodeString(trial, encName); err != nil {
			return b.String()
		}
		b.WriteRune(r)
	}
	return b.String()
}

// EncodePersonNameWithCharsets encodes a PersonName for writing.
func EncodePersonNameWithCharsets(pn PersonName, encodings []string) []byte {
	if pn.IsZero() {
		return nil
	}
	if !needsCharsetEncode(encodings) {
		return []byte(strings.TrimRight(pn.String(), " \x00"))
	}

	groups := []string{pn.Alphabetic, pn.Ideographic, pn.Phonetic}
	for len(groups) > 0 && groups[len(groups)-1] == "" {
		groups = groups[:len(groups)-1]
	}
	if len(groups) == 0 && pn.Original != "" {
		return EncodeBytesWithCharsets(strings.TrimRight(pn.Original, " \x00"), encodings)
	}

	parts := make([][]byte, len(groups))
	for i, comp := range groups {
		hatParts := strings.Split(comp, "^")
		encodedHat := make([][]byte, len(hatParts))
		for j, hp := range hatParts {
			encodedHat[j] = EncodeBytesWithCharsets(hp, encodings)
		}
		parts[i] = bytes.Join(encodedHat, []byte{'^'})
	}
	return bytes.Join(parts, []byte{'='})
}
