package nvueschema

import (
	"fmt"
	"math"
)

// knownConstants maps well-known integer constants for display.
var knownConstants = map[float64]string{
	math.MaxUint32: "UINT32_MAX",
	math.MaxInt32:  "INT32_MAX",
	math.MaxUint16: "UINT16_MAX",
	math.MaxInt16:  "INT16_MAX",
}

// fmtNum formats a float64 as a clean integer when possible,
// substituting well-known constants for readability.
func fmtNum(v float64) string {
	if name, ok := knownConstants[v]; ok {
		return name
	}
	// If it's a whole number, format without decimals.
	if v == math.Trunc(v) && !math.IsInf(v, 0) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}

// fmtNumPtr formats a *float64 for display.
func fmtNumPtr(p *float64) string {
	if p == nil {
		return "(none)"
	}
	return fmtNum(*p)
}

// fmtDefault formats a default value for display.
// Numbers are formatted cleanly, strings are quoted.
func fmtDefault(v any) string {
	if v == nil {
		return "null"
	}
	switch n := v.(type) {
	case float64:
		return fmtNum(n)
	case string:
		return quoteLiteral(n)
	default:
		return fmt.Sprint(v)
	}
}

// quoteLiteral quotes string literals but leaves numeric values bare.
func quoteLiteral(s string) string {
	if len(s) > 0 && (s[0] >= '0' && s[0] <= '9' || s[0] == '-' || s[0] == '.') {
		return s
	}
	return fmt.Sprintf("%q", s)
}
