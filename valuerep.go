package godicom

import (
	"fmt"
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
