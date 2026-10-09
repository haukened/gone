package i18n

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Args holds a message's named arguments. Values are strings, integers,
// float64, or one of the formatted kinds Number, Bytes, and Duration, which
// the browser re-formats the same way when the locale changes.
type Args map[string]any

// Number is a decimal shown with exactly Digits fraction digits.
type Number struct {
	Value  float64
	Digits int
}

// MarshalJSON encodes n as {"num":…,"digits":…} for data-i18n-args.
func (n Number) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"num": n.Value, "digits": n.Digits})
}

// Bytes is a size shown in the largest fitting unit, e.g. "10.0 MB".
type Bytes int64

// MarshalJSON encodes b as {"bytes":…} for data-i18n-args.
func (b Bytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]int64{"bytes": int64(b)})
}

// Duration is a length of time shown in its largest whole unit: compactly
// ("2h") when Style is "short", spelled out ("2 hours") when "long".
type Duration struct {
	Seconds int64
	Style   string
}

// MarshalJSON encodes d as {"seconds":…,"style":…} for data-i18n-args.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"seconds": d.Seconds, "style": d.Style})
}

// byteUnits are the size message keys from smallest to largest.
var byteUnits = []string{"common.size.kb", "common.size.mb", "common.size.gb", "common.size.tb", "common.size.pb"}

// durationUnits are the whole units a Duration may be shown in, largest first.
var durationUnits = []struct {
	name    string
	seconds int64
}{{"days", 86400}, {"hours", 3600}, {"minutes", 60}, {"seconds", 1}}

// formatValue renders one argument value in loc.
//
// Parameters:
//   - info: the locale's formatting rules.
//   - v: the argument value.
//
// Returns:
//   - string: the formatted text.
func (b *Bundle) formatValue(info localeInfo, v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return formatInt(info, int64(x))
	case int64:
		return formatInt(info, x)
	case float64:
		return formatDecimal(info, x, -1)
	case Number:
		return formatDecimal(info, x.Value, x.Digits)
	case Bytes:
		return b.formatBytes(info, int64(x))
	case Duration:
		return b.formatDuration(info, x)
	}
	return ""
}

// formatBytes renders n bytes in the largest unit below 1024 of the next.
func (b *Bundle) formatBytes(info localeInfo, n int64) string {
	if n < 1024 {
		return b.render(info, "common.size.b", Args{"n": n})
	}
	f := float64(n)
	for i, key := range byteUnits {
		f /= 1024
		if f < 1024 || i == len(byteUnits)-1 {
			return b.render(info, key, Args{"n": Number{Value: f, Digits: 1}})
		}
	}
	return ""
}

// formatDuration renders d in its largest whole unit.
func (b *Bundle) formatDuration(info localeInfo, d Duration) string {
	style := "long"
	if d.Style == "short" {
		style = "short"
	}
	for _, u := range durationUnits {
		if d.Seconds > 0 && d.Seconds%u.seconds == 0 {
			return b.render(info, "common.duration."+style+"."+u.name, Args{"n": d.Seconds / u.seconds})
		}
	}
	return b.render(info, "common.duration."+style+".seconds", Args{"n": int64(0)})
}

// formatInt renders n with the locale's grouping separator.
func formatInt(info localeInfo, n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := groupDigits(info, strconv.FormatInt(n, 10))
	if neg {
		return "-" + s
	}
	return s
}

// groupDigits inserts grouping separators into a run of integer digits when
// it is long enough for the locale to group (Spanish waits for five digits).
func groupDigits(info localeInfo, digits string) string {
	if len(digits) < 3+info.minGroup {
		return digits
	}
	var sb strings.Builder
	lead := len(digits) % 3
	if lead > 0 {
		sb.WriteString(digits[:lead])
	}
	for i := lead; i < len(digits); i += 3 {
		if sb.Len() > 0 {
			sb.WriteString(info.group)
		}
		sb.WriteString(digits[i : i+3])
	}
	return sb.String()
}

// formatDecimal renders v with the locale's separators: exactly digits
// fraction digits, or as few as needed when digits is negative.
func formatDecimal(info localeInfo, v float64, digits int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	s := strconv.FormatFloat(math.Abs(v), 'f', digits, 64)
	whole, frac, _ := strings.Cut(s, ".")
	out := groupDigits(info, whole)
	if frac != "" {
		out += info.decimal + frac
	}
	if v < 0 {
		return "-" + out
	}
	return out
}

// pluralCategory returns the CLDR category for a plural input value.
// Server-rendered counts are integers; any other value selects "other".
func pluralCategory(info localeInfo, v any) string {
	switch x := v.(type) {
	case int:
		return info.plural(int64(x))
	case int64:
		return info.plural(x)
	case Number:
		if x.Digits == 0 && x.Value == math.Trunc(x.Value) {
			return info.plural(int64(x.Value))
		}
	}
	return "other"
}
