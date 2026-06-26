package godicom

import (
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

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
	"ISO 2022 IR 13":                 japanese.ShiftJIS,
	"GB18030":                        simplifiedchinese.GB18030,
	"GBK":                            simplifiedchinese.GBK,
	"ISO 2022 IR 149":                korean.EUCKR,
	"BIG5":                           traditionalchinese.Big5,
	"ISO 2022 IR 13\\ISO 2022 IR 87": japanese.ShiftJIS,
	"ISO 2022 IR 87\\ISO 2022 IR 13": japanese.ShiftJIS,
	"ISO_IR 192":                     encoding.Nop,
	"ISO 2022 IR 192":                encoding.Nop,
}

const defaultEncoding = "ISO_IR 6"

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

func DecodeBytes(b []byte, encodings []string) string {
	for _, enc := range encodings {
		s, err := DecodeString(b, enc)
		if err == nil {
			return s
		}
	}
	return string(b)
}
