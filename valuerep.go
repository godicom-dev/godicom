package godicom

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DA holds a DICOM Date (VR=DA) value.
type DA struct {
	Time     time.Time
	Original string
}

// TM holds a DICOM Time (VR=TM) value.
type TM struct {
	Time     time.Time
	Original string
}

// DT holds a DICOM DateTime (VR=DT) value.
type DT struct {
	Time     time.Time
	Original string
}

var reTM = regexp.MustCompile(`^([01][0-9]|2[0-3])` +
	`(([0-5][0-9])` +
	`(([0-5][0-9]|60)` +
	`(\.([0-9]{1,6})?)?)?)?$`)

var reDT = regexp.MustCompile(`^((\d{4,14})(\.(\d{1,6}))?)([+-]\d{4})?$`)

// ParseDA parses a DICOM DA string.
func ParseDA(s string) (DA, error) {
	s = strings.TrimRight(s, " \x00")
	if s == "" {
		return DA{}, nil
	}
	var year, month, day int
	var err error
	switch len(s) {
	case 8:
		year, err = strconv.Atoi(s[0:4])
		if err != nil {
			return DA{}, fmt.Errorf("godicom: invalid DA %q", s)
		}
		month, err = strconv.Atoi(s[4:6])
		if err != nil {
			return DA{}, fmt.Errorf("godicom: invalid DA %q", s)
		}
		day, err = strconv.Atoi(s[6:8])
		if err != nil {
			return DA{}, fmt.Errorf("godicom: invalid DA %q", s)
		}
	case 10:
		if s[4] != '.' || s[7] != '.' {
			return DA{}, fmt.Errorf("godicom: invalid DA %q", s)
		}
		year, err = strconv.Atoi(s[0:4])
		if err != nil {
			return DA{}, fmt.Errorf("godicom: invalid DA %q", s)
		}
		month, err = strconv.Atoi(s[5:7])
		if err != nil {
			return DA{}, fmt.Errorf("godicom: invalid DA %q", s)
		}
		day, err = strconv.Atoi(s[8:10])
		if err != nil {
			return DA{}, fmt.Errorf("godicom: invalid DA %q", s)
		}
	default:
		return DA{}, fmt.Errorf("godicom: invalid DA %q", s)
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return DA{Time: t, Original: s}, nil
}

func (d DA) String() string {
	if d.Original != "" {
		return d.Original
	}
	if d.Time.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d%02d%02d", d.Time.Year(), int(d.Time.Month()), d.Time.Day())
}

func (d DA) IsZero() bool {
	return d.Original == "" && d.Time.IsZero()
}

// ParseTM parses a DICOM TM string.
func ParseTM(s string) (TM, error) {
	s = strings.TrimRight(s, " \x00")
	if s == "" {
		return TM{}, nil
	}
	match := reTM.FindStringSubmatch(s)
	if match == nil {
		return TM{}, fmt.Errorf("godicom: invalid TM %q", s)
	}
	hour, _ := strconv.Atoi(match[1])
	minute := 0
	if match[3] != "" {
		minute, _ = strconv.Atoi(match[3])
	}
	second := 0
	if match[5] != "" {
		second, _ = strconv.Atoi(match[5])
		if second == 60 {
			second = 59
		}
	}
	nsec := 0
	if match[7] != "" {
		frac := match[7]
		if len(frac) < 6 {
			frac = frac + strings.Repeat("0", 6-len(frac))
		}
		usec, _ := strconv.Atoi(frac)
		nsec = usec * 1000
	}
	t := time.Date(1, 1, 1, hour, minute, second, nsec, time.UTC)
	return TM{Time: t, Original: s}, nil
}

func (t TM) String() string {
	if t.Original != "" {
		return t.Original
	}
	if t.Time.IsZero() {
		return ""
	}
	return fmt.Sprintf("%02d%02d%02d", t.Time.Hour(), t.Time.Minute(), t.Time.Second())
}

func (t TM) IsZero() bool {
	return t.Original == "" && t.Time.IsZero()
}

// ParseDT parses a DICOM DT string.
func ParseDT(s string) (DT, error) {
	s = strings.TrimRight(s, " \x00")
	if s == "" {
		return DT{}, nil
	}
	if len(s) > 26 {
		return DT{}, fmt.Errorf("godicom: invalid DT %q", s)
	}
	match := reDT.FindStringSubmatch(s)
	if match == nil {
		return DT{}, fmt.Errorf("godicom: invalid DT %q", s)
	}
	dt := match[2]
	year, _ := strconv.Atoi(dt[0:4])
	month := 1
	if len(dt) >= 6 {
		month, _ = strconv.Atoi(dt[4:6])
	}
	day := 1
	if len(dt) >= 8 {
		day, _ = strconv.Atoi(dt[6:8])
	}
	hour, minute, second := 0, 0, 0
	if len(dt) >= 10 {
		hour, _ = strconv.Atoi(dt[8:10])
	}
	if len(dt) >= 12 {
		minute, _ = strconv.Atoi(dt[10:12])
	}
	if len(dt) >= 14 {
		second, _ = strconv.Atoi(dt[12:14])
		if second == 60 {
			second = 59
		}
	}
	nsec := 0
	if match[4] != "" {
		frac := match[4]
		if len(frac) < 6 {
			frac = frac + strings.Repeat("0", 6-len(frac))
		}
		usec, _ := strconv.Atoi(frac)
		nsec = usec * 1000
	}
	loc := time.UTC
	if tz := match[5]; tz != "" {
		sign := 1
		if tz[0] == '-' {
			sign = -1
		}
		offHour, _ := strconv.Atoi(tz[1:3])
		offMin, _ := strconv.Atoi(tz[3:5])
		offset := sign * ((offHour * 60) + offMin) * 60
		loc = time.FixedZone(tz, offset)
	}
	tm := time.Date(year, time.Month(month), day, hour, minute, second, nsec, loc)
	return DT{Time: tm, Original: s}, nil
}

func (d DT) String() string {
	if d.Original != "" {
		return d.Original
	}
	if d.Time.IsZero() {
		return ""
	}
	return d.Time.Format("20060102150405")
}

func (d DT) IsZero() bool {
	return d.Original == "" && d.Time.IsZero()
}

func parseDAValue(s string) (interface{}, error) {
	da, err := ParseDA(s)
	if err != nil {
		return nil, err
	}
	if da.IsZero() {
		return "", nil
	}
	return da, nil
}

func parseTMValue(s string) (interface{}, error) {
	tm, err := ParseTM(s)
	if err != nil {
		return nil, err
	}
	if tm.IsZero() {
		return "", nil
	}
	return tm, nil
}

func parseDTValue(s string) (interface{}, error) {
	dt, err := ParseDT(s)
	if err != nil {
		return nil, err
	}
	if dt.IsZero() {
		return "", nil
	}
	return dt, nil
}

// DS holds a DICOM Decimal String (VR=DS) value.
type DS struct {
	Value    float64
	Original string
}

// IS holds a DICOM Integer String (VR=IS) value.
type IS struct {
	Value    int64
	Original string
}

// ParseDS parses a DICOM DS string.
func ParseDS(s string) (DS, error) {
	raw := strings.TrimRight(s, " \x00")
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DS{}, nil
	}
	f, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return DS{}, fmt.Errorf("godicom: invalid DS %q", s)
	}
	return DS{Value: f, Original: raw}, nil
}

func (d DS) String() string {
	if d.Original != "" {
		return d.Original
	}
	return strconv.FormatFloat(d.Value, 'g', -1, 64)
}

func (d DS) IsZero() bool {
	return d.Original == "" && d.Value == 0
}

// ParseIS parses a DICOM IS string.
func ParseIS(s string) (IS, error) {
	raw := strings.TrimRight(s, " \x00")
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return IS{}, nil
	}
	i, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return IS{}, fmt.Errorf("godicom: invalid IS %q", s)
	}
	return IS{Value: i, Original: raw}, nil
}

func (i IS) String() string {
	if i.Original != "" {
		return i.Original
	}
	return strconv.FormatInt(i.Value, 10)
}

func (i IS) IsZero() bool {
	return i.Original == "" && i.Value == 0
}

func (i IS) Equal(other IS) bool {
	return i.Value == other.Value
}

const (
	maxDSLength = 16
	maxISLength = 12
	minISValue  = -1 << 31
	maxISValue  = 1<<31 - 1
)

var (
	reValidDS = regexp.MustCompile(`^ *[+\-]?(\d+|\d+\.\d*|\.\d+)([eE][+\-]?\d+)? *$`)
	reValidIS = regexp.MustCompile(`^ *[+\-]?\d+ *$`)
)

// IsValidDS reports whether s is a valid DICOM Decimal String (VR=DS).
// Mirrors pydicom.valuerep.is_valid_ds.
func IsValidDS(s string) bool {
	if len(s) > maxDSLength {
		return false
	}
	return reValidDS.MatchString(s)
}

// IsValidIS reports whether s is a valid DICOM Integer String (VR=IS) by
// length and character set (not numeric range).
func IsValidIS(s string) bool {
	if len(s) > maxISLength {
		return false
	}
	return reValidIS.MatchString(s)
}

// ISInRange reports whether v fits the DICOM IS value range [-2^31, 2^31).
func ISInRange(v int64) bool {
	return v >= minISValue && v <= maxISValue
}

// FormatNumberAsDS formats a float as a DICOM Decimal String (≤16 chars).
// Mirrors pydicom.valuerep.format_number_as_ds.
func FormatNumberAsDS(val float64) (string, error) {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return "", fmt.Errorf("godicom: cannot encode non-finite float %v as DS", val)
	}

	valstr := pythonStyleFloatString(val)
	if len(valstr) <= maxDSLength {
		return valstr, nil
	}

	absVal := math.Abs(val)
	logval := math.Log10(absVal)
	signChars := 0
	if val < 0 || (val == 0 && math.Signbit(val)) {
		signChars = 1
	}

	useScientific := logval < -4 || logval >= float64(14-signChars)
	if useScientific {
		remaining := 10 - signChars
		trunc := formatScientificDS(val, remaining)
		if len(trunc) > maxDSLength {
			trunc = formatScientificDS(val, remaining-1)
		}
		return trunc, nil
	}

	remaining := 14 - signChars
	if logval >= 1.0 {
		remaining = 14 - signChars - int(math.Floor(logval))
	}
	if remaining < 0 {
		remaining = 0
	}
	return strconv.FormatFloat(val, 'f', remaining, 64), nil
}

func pythonStyleFloatString(val float64) string {
	if val == 0 {
		if math.Signbit(val) {
			return "-0.0"
		}
		return "0.0"
	}
	s := strconv.FormatFloat(val, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	// Python uses lowercase e with explicit sign in exponent for |exp|>=1.
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		s = s[:i] + "e" + normalizeExponent(s[i+1:])
	}
	return s
}

func normalizeExponent(exp string) string {
	if exp == "" {
		return "+0"
	}
	sign := ""
	if exp[0] == '+' || exp[0] == '-' {
		sign = string(exp[0])
		exp = exp[1:]
	} else {
		sign = "+"
	}
	// Trim leading zeros but keep at least one digit; Python often uses 2+ digits.
	for len(exp) > 1 && exp[0] == '0' {
		exp = exp[1:]
	}
	if len(exp) < 2 {
		exp = "0" + exp
	}
	return sign + exp
}

func formatScientificDS(val float64, precision int) string {
	if precision < 0 {
		precision = 0
	}
	s := strconv.FormatFloat(val, 'e', precision, 64)
	// strconv uses 'e+09'; normalize like Python's default e format.
	if i := strings.IndexByte(s, 'e'); i >= 0 {
		s = s[:i] + "e" + normalizeExponent(s[i+1:])
	}
	return s
}

// DSFromFloat builds a DS whose Original is a valid DICOM decimal string.
func DSFromFloat(val float64) (DS, error) {
	s, err := FormatNumberAsDS(val)
	if err != nil {
		return DS{}, err
	}
	return DS{Value: val, Original: s}, nil
}

func (d DS) Equal(other DS) bool {
	if d.IsZero() && other.IsZero() {
		return true
	}
	return d.Value == other.Value
}

func (d DA) Equal(other DA) bool {
	if d.IsZero() && other.IsZero() {
		return true
	}
	y1, m1, day1 := d.Time.Date()
	y2, m2, day2 := other.Time.Date()
	return y1 == y2 && m1 == m2 && day1 == day2
}

func (t TM) Equal(other TM) bool {
	if t.IsZero() && other.IsZero() {
		return true
	}
	return t.Time.Hour() == other.Time.Hour() &&
		t.Time.Minute() == other.Time.Minute() &&
		t.Time.Second() == other.Time.Second() &&
		t.Time.Nanosecond() == other.Time.Nanosecond()
}

func (d DT) Equal(other DT) bool {
	if d.IsZero() && other.IsZero() {
		return true
	}
	return d.Time.Equal(other.Time)
}

func parseDSValue(s string) (interface{}, error) {
	ds, err := ParseDS(s)
	if err != nil {
		return nil, err
	}
	if ds.IsZero() {
		return nil, nil
	}
	return ds, nil
}

func parseISValue(s string) (interface{}, error) {
	is, err := ParseIS(s)
	if err != nil {
		return nil, err
	}
	if is.IsZero() {
		return nil, nil
	}
	return is, nil
}
