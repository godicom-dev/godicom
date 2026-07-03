package godicom

import (
	"bytes"
	"regexp"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

const esc = 0x1b

var dicomEncodings = map[string]encoding.Encoding{
	"":                               encoding.Nop,
	"ISO_IR 6":                       encoding.Nop,
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
	"ISO 2022 IR 149":                korean.EUCKR,
	"ISO 2022 IR 58":                 simplifiedchinese.HZGB2312,
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
	"ISO 2022 IR 149": true,
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
	enc, ok := dicomEncodings[encodingName]
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

// ParseCharacterSets splits a Specific Character Set value into DICOM encoding names.
func ParseCharacterSets(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return []string{DefaultCharacterSet}
	case string:
		return ConvertCharacterSets(splitCharacterSetString(v))
	case *MultiValue[string]:
		return ConvertCharacterSets(v.Values())
	default:
		return []string{DefaultCharacterSet}
	}
}

func splitCharacterSetString(s string) []string {
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\\")
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
	if _, ok := dicomEncodings[cs]; ok {
		return cs
	}
	if patched := patchCharacterSetSpelling(cs); patched != "" {
		return patched
	}
	return DefaultCharacterSet
}

func patchCharacterSetSpelling(cs string) string {
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

// DecodeBytesWithDelimiters decodes a DICOM text byte string with ISO-2022 code extensions.
func DecodeBytesWithDelimiters(b []byte, encodings []string, delimiters map[byte]bool) string {
	encodings = ConvertCharacterSets(encodings)
	if len(b) == 0 {
		return ""
	}
	if !bytes.Contains(b, []byte{esc}) {
		return decodeSimpleBytes(b, encodings[0])
	}

	var out strings.Builder
	for _, fragment := range iso2022FragmentRe.FindAll(b, -1) {
		out.WriteString(decodeFragment(fragment, encodings, delimiters))
	}
	return out.String()
}

func decodeSimpleBytes(b []byte, encodingName string) string {
	s, err := DecodeString(b, encodingName)
	if err != nil {
		return string(b)
	}
	return s
}

func decodeFragment(fragment []byte, encodings []string, delimiters map[byte]bool) string {
	if len(fragment) == 0 {
		return ""
	}
	if fragment[0] != esc {
		return decodeSimpleBytes(fragment, encodings[0])
	}
	return decodeEscapedFragment(fragment, encodings, delimiters)
}

func decodeEscapedFragment(fragment []byte, encodings []string, delimiters map[byte]bool) string {
	seqLen := escapeSequenceLength(fragment)
	if seqLen > len(fragment) {
		return decodeSimpleBytes(fragment, encodings[0])
	}

	csName := escapeToCharacterSet[string(fragment[:seqLen])]
	if !encodingAllowed(csName, encodings) {
		return decodeWithReplacement(fragment, encodings[0])
	}
	if iso2022HandledEncodings[csName] || iso2022HandledEncodings[characterSetKey(csName)] {
		return decodeSimpleBytes(fragment, csName)
	}

	payload := fragment[seqLen:]
	decodeName := csName
	if _, ok := dicomEncodings[decodeName]; !ok {
		decodeName = characterSetKey(csName)
	}
	if len(delimiters) == 0 {
		return decodeSimpleBytes(payload, decodeName)
	}

	idx := indexDelimiter(payload, delimiters)
	if idx < 0 {
		return decodeSimpleBytes(payload, decodeName)
	}
	return decodeSimpleBytes(payload[:idx], decodeName) + decodeSimpleBytes(payload[idx:], encodings[0])
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

func decodeWithReplacement(b []byte, encodingName string) string {
	s, err := DecodeString(b, encodingName)
	if err == nil {
		return s
	}
	return string(b)
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
