package time

import "errors"

const (
	numeric = iota
	alphabetic
	separator
	plus
	minus
)

// These are predefined layouts for use in Time.Format.
// The standard time used in the layouts is:
//	Mon Jan 2 15:04:05 MST 2006  (MST is GMT-0700)
// which is Unix time 1136243045.
// (Think of it as 01/02 03:04:05PM '06 -0700.)
// To define your own format, write down what the standard
// time would look like formatted your way.
//
// Within the format string, an underscore _ represents a space that may be
// replaced by a digit if the following number (a day) has two digits; for
// compatibility with fixed-width Unix time formats.
//
// A decimal point followed by one or more zeros represents a fractional
// second. When parsing (only), the input may contain a fractional second
// field immediately after the seconds field, even if the layout does not
// signify its presence. In that case a decimal point followed by a maximal
// series of digits is parsed as a fractional second.
//
// Numeric time zone offsets format as follows:
//	-0700  ±hhmm
//	-07:00 ±hh:mm
// Replacing the sign in the format with a Z triggers
// the ISO 8601 behavior of printing Z instead of an
// offset for the UTC zone.  Thus:
//	Z0700  Z or ±hhmm
//	Z07:00 Z or ±hh:mm
const (
	ANSIC    = "Mon Jan _2 15:04:05 2006"
	UnixDate = "Mon Jan _2 15:04:05 MST 2006"
	RubyDate = "Mon Jan 02 15:04:05 -0700 2006"
	RFC822   = "02 Jan 06 1504 MST"
	RFC822Z  = "02 Jan 06 1504 -0700" // RFC822 with numeric zone
	RFC850   = "Monday, 02-Jan-06 15:04:05 MST"
	RFC1123  = "Mon, 02 Jan 2006 15:04:05 MST"
	RFC1123Z = "Mon, 02 Jan 2006 15:04:05 -0700" // RFC1123 with numeric zone
	RFC3339  = "2006-01-02T15:04:05Z07:00"
	Kitchen  = "3:04PM"
	// Handy time stamps.
	Stamp      = "Jan _2 15:04:05"
	StampMilli = "Jan _2 15:04:05.000"
	StampMicro = "Jan _2 15:04:05.000000"
	StampNano  = "Jan _2 15:04:05.000000000"
)

const (
	stdLongMonth      = "January"
	stdMonth          = "Jan"
	stdNumMonth       = "1"
	stdZeroMonth      = "01"
	stdLongWeekDay    = "Monday"
	stdWeekDay        = "Mon"
	stdDay            = "2"
	stdUnderDay       = "_2"
	stdZeroDay        = "02"
	stdHour           = "15"
	stdHour12         = "3"
	stdZeroHour12     = "03"
	stdMinute         = "4"
	stdZeroMinute     = "04"
	stdSecond         = "5"
	stdZeroSecond     = "05"
	stdLongYear       = "2006"
	stdYear           = "06"
	stdPM             = "PM"
	stdpm             = "pm"
	stdTZ             = "MST"
	stdISO8601TZ      = "Z0700"  // prints Z for UTC
	stdISO8601ColonTZ = "Z07:00" // prints Z for UTC
	stdNumTZ          = "-0700"  // always numeric
	stdNumShortTZ     = "-07"    // always numeric
	stdNumColonTZ     = "-07:00" // always numeric
)

// nextStdChunk finds the first occurrence of a std string in
// layout and returns the text before, the std string, and the text after.
func nextStdChunk(layout string) (prefix, std, suffix string) {
	for i := 0; i < len(layout); i++ {
		switch layout[i] {
		case 'J': // January, Jan
			if len(layout) >= i+7 && layout[i:i+7] == stdLongMonth {
				return layout[0:i], stdLongMonth, layout[i+7:]
			}
			if len(layout) >= i+3 && layout[i:i+3] == stdMonth {
				return layout[0:i], stdMonth, layout[i+3:]
			}

		case 'M': // Monday, Mon, MST
			if len(layout) >= i+6 && layout[i:i+6] == stdLongWeekDay {
				return layout[0:i], stdLongWeekDay, layout[i+6:]
			}
			if len(layout) >= i+3 {
				if layout[i:i+3] == stdWeekDay {
					return layout[0:i], stdWeekDay, layout[i+3:]
				}
				if layout[i:i+3] == stdTZ {
					return layout[0:i], stdTZ, layout[i+3:]
				}
			}

		case '0': // 01, 02, 03, 04, 05, 06
			if len(layout) >= i+2 && '1' <= layout[i+1] && layout[i+1] <= '6' {
				return layout[0:i], layout[i : i+2], layout[i+2:]
			}

		case '1': // 15, 1
			if len(layout) >= i+2 && layout[i+1] == '5' {
				return layout[0:i], stdHour, layout[i+2:]
			}
			return layout[0:i], stdNumMonth, layout[i+1:]

		case '2': // 2006, 2
			if len(layout) >= i+4 && layout[i:i+4] == stdLongYear {
				return layout[0:i], stdLongYear, layout[i+4:]
			}
			return layout[0:i], stdDay, layout[i+1:]

		case '_': // _2
			if len(layout) >= i+2 && layout[i+1] == '2' {
				return layout[0:i], stdUnderDay, layout[i+2:]
			}

		case '3', '4', '5': // 3, 4, 5
			return layout[0:i], layout[i : i+1], layout[i+1:]

		case 'P': // PM
			if len(layout) >= i+2 && layout[i+1] == 'M' {
				return layout[0:i], layout[i : i+2], layout[i+2:]
			}

		case 'p': // pm
			if len(layout) >= i+2 && layout[i+1] == 'm' {
				return layout[0:i], layout[i : i+2], layout[i+2:]
			}

		case '-': // -0700, -07:00, -07
			if len(layout) >= i+5 && layout[i:i+5] == stdNumTZ {
				return layout[0:i], layout[i : i+5], layout[i+5:]
			}
			if len(layout) >= i+6 && layout[i:i+6] == stdNumColonTZ {
				return layout[0:i], layout[i : i+6], layout[i+6:]
			}
			if len(layout) >= i+3 && layout[i:i+3] == stdNumShortTZ {
				return layout[0:i], layout[i : i+3], layout[i+3:]
			}
		case 'Z': // Z0700, Z07:00
			if len(layout) >= i+5 && layout[i:i+5] == stdISO8601TZ {
				return layout[0:i], layout[i : i+5], layout[i+5:]
			}
			if len(layout) >= i+6 && layout[i:i+6] == stdISO8601ColonTZ {
				return layout[0:i], layout[i : i+6], layout[i+6:]
			}
		case '.': // .000 - multiple digits of zeros (only) for fractional seconds.
			numZeros := 0
			var j int
			for j = i + 1; j < len(layout) && layout[j] == '0'; j++ {
				numZeros++
			}
			// String of digits must end here - only fractional second is all zeros.
			if numZeros > 0 && !isDigit(layout, j) {
				return layout[0:i], layout[i : i+1+numZeros], layout[i+1+numZeros:]
			}
		}
	}
	return layout, "", ""
}

var longDayNames = []string{
	"Sunday",
	"Monday",
	"Tuesday",
	"Wednesday",
	"Thursday",
	"Friday",
	"Saturday",
}

var shortDayNames = []string{
	"Sun",
	"Mon",
	"Tue",
	"Wed",
	"Thu",
	"Fri",
	"Sat",
}

var shortMonthNames = []string{
	"---",
	"Jan",
	"Feb",
	"Mar",
	"Apr",
	"May",
	"Jun",
	"Jul",
	"Aug",
	"Sep",
	"Oct",
	"Nov",
	"Dec",
}

var longMonthNames = []string{
	"---",
	"January",
	"February",
	"March",
	"April",
	"May",
	"June",
	"July",
	"August",
	"September",
	"October",
	"November",
	"December",
}

// match returns true if s1 and s2 match ignoring case.
// It is assumed s1 and s2 are the same length.
func match(s1, s2 string) bool {
	for i := 0; i < len(s1); i++ {
		c1 := s1[i]
		c2 := s2[i]
		if c1 != c2 {
			// Switch to lower-case; 'a'-'A' is known to be a single bit.
			c1 |= 'a' - 'A'
			c2 |= 'a' - 'A'
			if c1 != c2 || c1 < 'a' || c1 > 'z' {
				return false
			}
		}
	}
	return true
}

func lookup(tab []string, val string) (int, string, error) {
	for i, v := range tab {
		if len(val) >= len(v) && match(val[0:len(v)], v) {
			return i, val[len(v):], nil
		}
	}
	return -1, val, errBad
}

// Duplicates functionality in strconv, but avoids dependency.
func itoa(x int) string {
	var buf [32]byte
	n := len(buf)
	if x == 0 {
		return "0"
	}
	u := uint(x)
	if x < 0 {
		u = -u
	}
	for u > 0 {
		n--
		buf[n] = byte(u%10 + '0')
		u /= 10
	}
	if x < 0 {
		n--
		buf[n] = '-'
	}
	return string(buf[n:])
}

// Never printed, just needs to be non-nil for return by atoi.
var atoiError = errors.New("time: invalid number")

// Duplicates functionality in strconv, but avoids dependency.
func atoi(s string) (x int, err error) {
	i := 0
	if len(s) > 0 && s[0] == '-' {
		i++
	}
	if i >= len(s) {
		return 0, atoiError
	}
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, atoiError
		}
		if x >= (1<<31-10)/10 {
			// will overflow
			return 0, atoiError
		}
		x = x*10 + int(c) - '0'
	}
	if s[0] == '-' {
		x = -x
	}
	return x, nil
}

func pad(i int, padding string) string {
	s := itoa(i)
	if i < 10 {
		s = padding + s
	}
	return s
}

func zeroPad(i int) string { return pad(i, "0") }

// formatNano formats a fractional second, as nanoseconds.
func formatNano(nanosec, n int) string {
	// User might give us bad data. Make sure it's positive and in range.
	// They'll get nonsense output but it will have the right format.
	s := itoa(int(uint(nanosec) % 1e9))
	// Zero pad left without fmt.
	if len(s) < 9 {
		s = "000000000"[:9-len(s)] + s
	}
	if n > 9 {
		n = 9
	}
	return "." + s[:n]
}

// String returns the time formatted using the format string
//	"Mon Jan _2 15:04:05 -0700 MST 2006"
func (t Time) String() string {
	return t.Format("Mon Jan _2 15:04:05 -0700 MST 2006")
}

type buffer []byte

func (b *buffer) WriteString(s string) {
	*b = append(*b, s...)
}

func (b *buffer) WriteByte(c byte) {
	*b = append(*b, c)
}

func (b *buffer) String() string {
	return string([]byte(*b))
}

// Format returns a textual representation of the time value formatted
// according to layout.  The layout defines the format by showing the
// representation of a standard time, which is then used to describe
// the time to be formatted.  Predefined layouts ANSIC, UnixDate,
// RFC3339 and others describe standard representations. For more
// information about the formats, see the documentation for ANSIC.
func (t Time) Format(layout string) string {
	var (
		year  int = -1
		month Month
		day   int
		hour  int = -1
		min   int
		sec   int
		b     buffer
	)
	// Each iteration generates one std value.
	for {
		prefix, std, suffix := nextStdChunk(layout)
		b.WriteString(prefix)
		if std == "" {
			break
		}

		// Compute year, month, day if needed.
		if year < 0 {
			// Jan 01 02 2006
			if a, z := std[0], std[len(std)-1]; a == 'J' || a == 'j' || z == '1' || z == '2' || z == '6' {
				year, month, day = t.Date()
			}
		}

		// Compute hour, minute, second if needed.
		if hour < 0 {
			// 03 04 05 15 pm
			if z := std[len(std)-1]; z == '3' || z == '4' || z == '5' || z == 'm' || z == 'M' {
				hour, min, sec = t.Clock()
			}
		}

		var p string
		switch std {
		case stdYear:
			p = zeroPad(year % 100)
		case stdLongYear:
			p = itoa(year)
		case stdMonth:
			p = month.String()[:3]
		case stdLongMonth:
			p = month.String()
		case stdNumMonth:
			p = itoa(int(month))
		case stdZeroMonth:
			p = zeroPad(int(month))
		case stdWeekDay:
			p = t.Weekday().String()[:3]
		case stdLongWeekDay:
			p = t.Weekday().String()
		case stdDay:
			p = itoa(day)
		case stdUnderDay:
			p = pad(day, " ")
		case stdZeroDay:
			p = zeroPad(day)
		case stdHour:
			p = zeroPad(hour)
		case stdHour12:
			// Noon is 12PM, midnight is 12AM.
			hr := hour % 12
			if hr == 0 {
				hr = 12
			}
			p = itoa(hr)
		case stdZeroHour12:
			// Noon is 12PM, midnight is 12AM.
			hr := hour % 12
			if hr == 0 {
				hr = 12
			}
			p = zeroPad(hr)
		case stdMinute:
			p = itoa(min)
		case stdZeroMinute:
			p = zeroPad(min)
		case stdSecond:
			p = itoa(sec)
		case stdZeroSecond:
			p = zeroPad(sec)
		case stdPM:
			if hour >= 12 {
				p = "PM"
			} else {
				p = "AM"
			}
		case stdpm:
			if hour >= 12 {
				p = "pm"
			} else {
				p = "am"
			}
		case stdISO8601TZ, stdISO8601ColonTZ, stdNumTZ, stdNumColonTZ:
			// Ugly special case.  We cheat and take the "Z" variants
			// to mean "the time zone as formatted for ISO 8601".
			_, offset := t.Zone()
			if offset == 0 && std[0] == 'Z' {
				p = "Z"
				break
			}
			zone := offset / 60 // convert to minutes
			if zone < 0 {
				p = "-"
				zone = -zone
			} else {
				p = "+"
			}
			p += zeroPad(zone / 60)
			if std == stdISO8601ColonTZ || std == stdNumColonTZ {
				p += ":"
			}
			p += zeroPad(zone % 60)
		case stdTZ:
			name, offset := t.Zone()
			if name != "" {
				p = name
			} else {
				// No time zone known for this time, but we must print one.
				// Use the -0700 format.
				zone := offset / 60 // convert to minutes
				if zone < 0 {
					p = "-"
					zone = -zone
				} else {
					p = "+"
				}
				p += zeroPad(zone / 60)
				p += zeroPad(zone % 60)
			}
		default:
			if len(std) >= 2 && std[0:2] == ".0" {
				p = formatNano(t.Nanosecond(), len(std)-1)
			}
		}
		b.WriteString(p)
		layout = suffix
	}
	return b.String()
}

var errBad = errors.New("bad value for field") // placeholder not passed to user

// ParseError describes a problem parsing a time string.
type ParseError struct {
	Layout     string
	Value      string
	LayoutElem string
	ValueElem  string
	Message    string
}

func quote(s string) string {
	return "\"" + s + "\""
}

// Error returns the string representation of a ParseError.
func (e *ParseError) Error() string {
	if e.Message == "" {
		return "parsing time " +
			quote(e.Value) + " as " +
			quote(e.Layout) + ": cannot parse " +
			quote(e.ValueElem) + " as " +
			quote(e.LayoutElem)
	}
	return "parsing time " +
		quote(e.Value) + e.Message
}

// isDigit returns true if s[i] is a decimal digit, false if not or
// if s[i] is out of range.
func isDigit(s string, i int) bool {
	if len(s) <= i {
		return false
	}
	c := s[i]
	return '0' <= c && c <= '9'
}

// getnum parses s[0:1] or s[0:2] (fixed forces the latter)
// as a decimal integer and returns the integer and the
// remainder of the string.
func getnum(s string, fixed bool) (int, string, error) {
	if !isDigit(s, 0) {
		return 0, s, errBad
	}
	if !isDigit(s, 1) {
		if fixed {
			return 0, s, errBad
		}
		return int(s[0] - '0'), s[1:], nil
	}
	return int(s[0]-'0')*10 + int(s[1]-'0'), s[2:], nil
}

func cutspace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	return s
}

// skip removes the given prefix from value,
// treating runs of space characters as equivalent.
func skip(value, prefix string) (string, error) {
	for len(prefix) > 0 {
		if prefix[0] == ' ' {
			if len(value) > 0 && value[0] != ' ' {
				return "", errBad
			}
			prefix = cutspace(prefix)
			value = cutspace(value)
			continue
		}
		if len(value) == 0 || value[0] != prefix[0] {
			return "", errBad
		}
		prefix = prefix[1:]
		value = value[1:]
	}
	return value, nil
}

// Parse parses a formatted string and returns the time value it represents.
// The layout defines the format by showing the representation of a standard
// time, which is then used to describe the string to be parsed.  Predefined
// layouts ANSIC, UnixDate, RFC3339 and others describe standard
// representations.For more information about the formats, see the
// documentation for ANSIC.
//
// Elements omitted from the value are assumed to be zero, or when
// zero is impossible, one, so parsing "3:04pm" returns the time
// corresponding to Jan 1, year 0, 15:04:00 UTC.
// Years must be in the range 0000..9999. The day of the week is checked
// for syntax but it is otherwise ignored.
func Parse(layout, value string) (Time, error) {
	alayout, avalue := layout, value
	rangeErrString := "" // set if a value is out of range
	amSet := false       // do we need to subtract 12 from the hour for midnight?
	pmSet := false       // do we need to add 12 to the hour?

	// Time being constructed.
	var (
		year       int
		month      int = 1 // January
		day        int = 1
		hour       int
		min        int
		sec        int
		nsec       int
		z          *Location
		zoneOffset int = -1
		zoneName   string
	)

	// Each iteration processes one std value.
	for {
		var err error
		prefix, std, suffix := nextStdChunk(layout)
		value, err = skip(value, prefix)
		if err != nil {
			return Time{}, &ParseError{alayout, avalue, prefix, value, ""}
		}
		if len(std) == 0 {
			if len(value) != 0 {
				return Time{}, &ParseError{alayout, avalue, "", value, ": extra text: " + value}
			}
			break
		}
		layout = suffix
		var p string
		switch std {
		case stdYear:
			if len(value) < 2 {
				err = errBad
				break
			}
			p, value = value[0:2], value[2:]
			year, err = atoi(p)
			if year >= 69 { // Unix time starts Dec 31 1969 in some time zones
				year += 1900
			} else {
				year += 2000
			}
		case stdLongYear:
			if len(value) < 4 || !isDigit(value, 0) {
				err = errBad
				break
			}
			p, value = value[0:4], value[4:]
			year, err = atoi(p)
		case stdMonth:
			month, value, err = lookup(shortMonthNames, value)
		case stdLongMonth:
			month, value, err = lookup(longMonthNames, value)
		case stdNumMonth, stdZeroMonth:
			month, value, err = getnum(value, std == stdZeroMonth)
			if month <= 0 || 12 < month {
				rangeErrString = "month"
			}
		case stdWeekDay:
			// Ignore weekday except for error checking.
			_, value, err = lookup(shortDayNames, value)
		case stdLongWeekDay:
			_, value, err = lookup(longDayNames, value)
		case stdDay, stdUnderDay, stdZeroDay:
			if std == stdUnderDay && len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}
			day, value, err = getnum(value, std == stdZeroDay)
			if day < 0 || 31 < day {
				rangeErrString = "day"
			}
		case stdHour:
			hour, value, err = getnum(value, false)
			if hour < 0 || 24 <= hour {
				rangeErrString = "hour"
			}
		case stdHour12, stdZeroHour12:
			hour, value, err = getnum(value, std == stdZeroHour12)
			if hour < 0 || 12 < hour {
				rangeErrString = "hour"
			}
		case stdMinute, stdZeroMinute:
			min, value, err = getnum(value, std == stdZeroMinute)
			if min < 0 || 60 <= min {
				rangeErrString = "minute"
			}
		case stdSecond, stdZeroSecond:
			sec, value, err = getnum(value, std == stdZeroSecond)
			if sec < 0 || 60 <= sec {
				rangeErrString = "second"
			}
			// Special case: do we have a fractional second but no
			// fractional second in the format?
			if len(value) > 2 && value[0] == '.' && isDigit(value, 1) {
				_, std, _ := nextStdChunk(layout)
				if len(std) > 0 && std[0] == '.' && isDigit(std, 1) {
					// Fractional second in the layout; proceed normally
					break
				}
				// No fractional second in the layout but we have one in the input.
				n := 2
				for ; n < len(value) && isDigit(value, n); n++ {
				}
				nsec, rangeErrString, err = parseNanoseconds(value, n)
				value = value[n:]
			}
		case stdPM:
			if len(value) < 2 {
				err = errBad
				break
			}
			p, value = value[0:2], value[2:]
			switch p {
			case "PM":
				pmSet = true
			case "AM":
				amSet = true
			default:
				err = errBad
			}
		case stdpm:
			if len(value) < 2 {
				err = errBad
				break
			}
			p, value = value[0:2], value[2:]
			switch p {
			case "pm":
				pmSet = true
			case "am":
				amSet = true
			default:
				err = errBad
			}
		case stdISO8601TZ, stdISO8601ColonTZ, stdNumTZ, stdNumShortTZ, stdNumColonTZ:
			if std[0] == 'Z' && len(value) >= 1 && value[0] == 'Z' {
				value = value[1:]
				z = UTC
				break
			}
			var sign, hour, min string
			if std == stdISO8601ColonTZ || std == stdNumColonTZ {
				if len(value) < 6 {
					err = errBad
					break
				}
				if value[3] != ':' {
					err = errBad
					break
				}
				sign, hour, min, value = value[0:1], value[1:3], value[4:6], value[6:]
			} else if std == stdNumShortTZ {
				if len(value) < 3 {
					err = errBad
					break
				}
				sign, hour, min, value = value[0:1], value[1:3], "00", value[3:]
			} else {
				if len(value) < 5 {
					err = errBad
					break
				}
				sign, hour, min, value = value[0:1], value[1:3], value[3:5], value[5:]
			}
			var hr, mm int
			hr, err = atoi(hour)
			if err == nil {
				mm, err = atoi(min)
			}
			zoneOffset = (hr*60 + mm) * 60 // offset is in seconds
			switch sign[0] {
			case '+':
			case '-':
				zoneOffset = -zoneOffset
			default:
				err = errBad
			}
		case stdTZ:
			// Does it look like a time zone?
			if len(value) >= 3 && value[0:3] == "UTC" {
				z = UTC
				value = value[3:]
				break
			}

			if len(value) >= 3 && value[2] == 'T' {
				p, value = value[0:3], value[3:]
			} else if len(value) >= 4 && value[3] == 'T' {
				p, value = value[0:4], value[4:]
			} else {
				err = errBad
				break
			}
			for i := 0; i < len(p); i++ {
				if p[i] < 'A' || 'Z' < p[i] {
					err = errBad
				}
			}
			if err != nil {
				break
			}
			// It's a valid format.
			zoneName = p
		default:
			if len(value) < len(std) {
				err = errBad
				break
			}
			if len(std) >= 2 && std[0:2] == ".0" {
				nsec, rangeErrString, err = parseNanoseconds(value, len(std))
				value = value[len(std):]
			}
		}
		if rangeErrString != "" {
			return Time{}, &ParseError{alayout, avalue, std, value, ": " + rangeErrString + " out of range"}
		}
		if err != nil {
			return Time{}, &ParseError{alayout, avalue, std, value, ""}
		}
	}
	if pmSet && hour < 12 {
		hour += 12
	} else if amSet && hour == 12 {
		hour = 0
	}

	// TODO: be more aggressive checking day?
	if z != nil {
		return Date(year, Month(month), day, hour, min, sec, nsec, z), nil
	}

	t := Date(year, Month(month), day, hour, min, sec, nsec, UTC)
	if zoneOffset != -1 {
		t.sec -= int64(zoneOffset)

		// Look for local zone with the given offset.
		// If that zone was in effect at the given time, use it.
		name, offset, _, _, _ := Local.lookup(t.sec + internalToUnix)
		if offset == zoneOffset && (zoneName == "" || name == zoneName) {
			t.loc = Local
			return t, nil
		}

		// Otherwise create fake zone to record offset.
		t.loc = FixedZone(zoneName, zoneOffset)
		return t, nil
	}

	if zoneName != "" {
		// Look for local zone with the given offset.
		// If that zone was in effect at the given time, use it.
		offset, _, ok := Local.lookupName(zoneName)
		if ok {
			name, off, _, _, _ := Local.lookup(t.sec + internalToUnix - int64(offset))
			if name == zoneName && off == offset {
				t.sec -= int64(offset)
				t.loc = Local
				return t, nil
			}
		}

		// Otherwise, create fake zone with unknown offset.
		t.loc = FixedZone(zoneName, 0)
		return t, nil
	}

	// Otherwise, fall back to UTC.
	return t, nil
}

func parseNanoseconds(value string, nbytes int) (ns int, rangeErrString string, err error) {
	if value[0] != '.' {
		err = errBad
		return
	}
	ns, err = atoi(value[1:nbytes])
	if err != nil {
		return
	}
	if ns < 0 || 1e9 <= ns {
		rangeErrString = "fractional second"
		return
	}
	// We need nanoseconds, which means scaling by the number
	// of missing digits in the format, maximum length 10. If it's
	// longer than 10, we won't scale.
	scaleDigits := 10 - nbytes
	for i := 0; i < scaleDigits; i++ {
		ns *= 10
	}
	return
}
