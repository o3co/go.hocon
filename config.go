// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/o3co/go.hocon/internal/resolver"
)

// integerRegex matches optional sign followed by one or more decimal digits.
// Compiled once at package scope per the Lightbend isWholeNumber pattern.
var integerRegex = regexp.MustCompile(`^[+-]?[0-9]+$`)

// isHoconWS reports whether r is a HOCON whitespace character.
// This is an inlined copy of internal/lexer.isHoconWhitespace (unexported).
// Keep in sync with internal/lexer/lexer.go:isHoconWhitespace.
// Note: Go's unicode.IsSpace includes U+0085 (NEL) which HOCON does not, and
// excludes U+001C-U+001F which HOCON does include. Do not substitute IsSpace.
func isHoconWS(r rune) bool {
	switch {
	case r == '\t', r == '\n', r == '\v', r == '\f', r == '\r':
		return true
	case r >= 0x1C && r <= 0x1F:
		return true
	case r == ' ', r == 0xA0, r == 0xFEFF:
		return true
	case r == 0x1680:
		return true
	case r >= 0x2000 && r <= 0x200A:
		return true
	case r == 0x2028, r == 0x2029, r == 0x202F, r == 0x205F:
		return true
	case r == 0x3000:
		return true
	}
	return false
}

// Config wraps a HOCON value tree.  When resolved is true, the tree contains
// no substitution/concat placeholders and all getters succeed (modulo
// type-mismatch panics per existing semantics).  When resolved is false, the
// tree contains placeholders for unresolved substitutions; getters on paths
// touching a placeholder panic with ErrNotResolved.
// All *Config values are safe for concurrent read access.
type Config struct {
	root              *resolver.ObjectVal
	resolved          bool
	parseBaseDir      string // base directory for relative include re-runs (unused in v1)
	originDescription string // E12 ParseOptions.OriginDescription
}

// newConfig wraps an ObjectVal as a fully resolved Config.
func newConfig(obj *resolver.ObjectVal) *Config {
	return &Config{root: obj, resolved: true}
}

// newUnresolvedConfig wraps an ObjectVal that may contain placeholders.
// Computes the resolved flag by checking the tree once.
func newUnresolvedConfig(obj *resolver.ObjectVal, baseDir, originDescription string) *Config {
	return &Config{
		root:              obj,
		resolved:          !resolver.ContainsPlaceholders(obj),
		parseBaseDir:      baseDir,
		originDescription: originDescription,
	}
}

// ── path resolution ──────────────────────────────────────────────

func (c *Config) lookup(path string) (resolver.Val, bool) {
	if path == "" {
		panicConfig(path, "empty path")
	}
	segments := splitPath(path)
	v, ok := lookupSegments(c.root, segments)
	if ok && isUnresolvedPlaceholder(v) {
		panicNotResolved(path)
	}
	return v, ok
}

// lookupSoft is like lookup but returns (nil, false) for unresolved placeholders
// instead of panicking.  Used by GetXxxOption getters which must not panic
// on unresolved paths.
func (c *Config) lookupSoft(path string) (resolver.Val, bool) {
	if path == "" {
		panicConfig(path, "empty path")
	}
	segments := splitPath(path)
	v, ok := lookupSegments(c.root, segments)
	if ok && isUnresolvedPlaceholder(v) {
		return nil, false
	}
	return v, ok
}

// isUnresolvedPlaceholder reports whether v is (or directly contains) an
// unresolved substitution / concat placeholder.  Used by lookup() to convert
// getter access into an ErrNotResolved-wrapped panic.
func isUnresolvedPlaceholder(v resolver.Val) bool {
	if v == nil {
		return false
	}
	switch vv := v.(type) {
	case *resolver.ObjectVal:
		return resolver.ContainsPlaceholders(vv)
	default:
		// Wrap as a single-key object to reuse the existing recursive walker.
		// Synthetic key "$" is invisible to the caller.
		wrap := resolver.NewObjectVal()
		wrap.Set("$", vv)
		return resolver.ContainsPlaceholders(wrap)
	}
}

// panicNotResolved panics with a *ConfigError whose error chain includes
// ErrNotResolved (for errors.Is matching).
func panicNotResolved(path string) {
	panic(&ConfigError{
		Path:    path,
		Message: "value is not resolved (call Resolve() first)",
		cause:   ErrNotResolved,
	})
}

func splitPath(path string) []string {
	var segments []string
	i := 0
	for i < len(path) {
		if path[i] == '"' {
			i++ // skip opening quote
			var seg []byte
			closed := false
			for i < len(path) {
				if path[i] == '\\' && i+1 < len(path) {
					seg = append(seg, path[i+1])
					i += 2
					continue
				}
				if path[i] == '"' {
					closed = true
					i++
					break
				}
				seg = append(seg, path[i])
				i++
			}
			segments = append(segments, string(seg))
			if !closed {
				break
			}
			if i < len(path) && path[i] == '.' {
				i++
			}
		} else {
			dot := strings.IndexByte(path[i:], '.')
			if dot == -1 {
				segments = append(segments, path[i:])
				break
			}
			segments = append(segments, path[i:i+dot])
			i = i + dot + 1
		}
	}
	return segments
}

func lookupSegments(obj *resolver.ObjectVal, segments []string) (resolver.Val, bool) {
	if obj == nil || len(segments) == 0 {
		return nil, false
	}
	v, ok := obj.Get(segments[0])
	if !ok {
		return nil, false
	}
	if len(segments) == 1 {
		return v, true
	}
	child, ok2 := v.(*resolver.ObjectVal)
	if !ok2 {
		return nil, false
	}
	return lookupSegments(child, segments[1:])
}

// ── Has / Keys ────────────────────────────────────────────────────

// IsResolved reports whether the Config's value tree contains any unresolved
// substitution placeholders.  Returns true if the tree is fully resolved.
// Whole-config granularity (no per-value isResolved).  Matches Lightbend
// Config.isResolved(). Per E12 decision 11.
func (c *Config) IsResolved() bool {
	if c.resolved {
		return true
	}
	// re-check (defensive) in case priorValues mutation outpaced the flag.
	return !resolver.ContainsPlaceholders(c.root)
}

// Has returns true if the path exists, including when the value is null.
// Unresolved-but-present keys return true (the key exists, value just isn't
// resolved yet).  Bypasses lookup() / lookupSoft() to avoid placeholder panic.
func (c *Config) Has(path string) bool {
	if path == "" {
		return false
	}
	segments := splitPath(path)
	_, ok := lookupSegments(c.root, segments)
	return ok
}

// Keys returns the direct child key names of the current object.
func (c *Config) Keys() []string {
	if c.root == nil {
		return nil
	}
	return c.root.Keys()
}

// ── scalar getters ────────────────────────────────────────────────

func (c *Config) getScalar(path string) *resolver.ScalarVal {
	v, ok := c.lookup(path)
	if !ok {
		panicConfig(path, "key not found")
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok {
		panicConfig(path, fmt.Sprintf("expected scalar, got %T", v))
	}
	if sv.Type == resolver.ScalarNull {
		panicConfig(path, "value is null")
	}
	return sv
}

func (c *Config) GetString(path string) string {
	sv := c.getScalar(path)
	return sv.Raw
}

func (c *Config) GetStringOption(path string) Option[string] {
	v, ok := c.lookupSoft(path)
	if !ok {
		return None[string]()
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok || sv.Type == resolver.ScalarNull {
		return None[string]()
	}
	return Some(sv.Raw)
}

func (c *Config) GetInt64(path string) int64 {
	sv := c.getScalar(path)
	parsed, err := strconv.ParseInt(sv.Raw, 10, 64)
	if err != nil {
		panicConfig(path, fmt.Sprintf("expected int64, got %q", sv.Raw))
	}
	return parsed
}

func (c *Config) GetInt64Option(path string) Option[int64] {
	v, ok := c.lookupSoft(path)
	if !ok {
		return None[int64]()
	}
	sv, ok2 := v.(*resolver.ScalarVal)
	if !ok2 || sv.Type == resolver.ScalarNull {
		return None[int64]()
	}
	if parsed, err := strconv.ParseInt(sv.Raw, 10, 64); err == nil {
		return Some(parsed)
	}
	return None[int64]()
}

func (c *Config) GetInt(path string) int { return int(c.GetInt64(path)) }
func (c *Config) GetIntOption(path string) Option[int] {
	o := c.GetInt64Option(path)
	if o.IsNone() {
		return None[int]()
	}
	v, _ := o.Get()
	return Some(int(v))
}

func (c *Config) GetFloat64(path string) float64 {
	sv := c.getScalar(path)
	parsed, err := strconv.ParseFloat(sv.Raw, 64)
	if err != nil {
		panicConfig(path, fmt.Sprintf("expected float64, got %q", sv.Raw))
	}
	return parsed
}

func (c *Config) GetFloat64Option(path string) Option[float64] {
	v, ok := c.lookupSoft(path)
	if !ok {
		return None[float64]()
	}
	sv, ok2 := v.(*resolver.ScalarVal)
	if !ok2 || sv.Type == resolver.ScalarNull {
		return None[float64]()
	}
	if parsed, err := strconv.ParseFloat(sv.Raw, 64); err == nil {
		return Some(parsed)
	}
	return None[float64]()
}

func (c *Config) GetFloat32(path string) float32 { return float32(c.GetFloat64(path)) }
func (c *Config) GetFloat32Option(path string) Option[float32] {
	o := c.GetFloat64Option(path)
	if o.IsNone() {
		return None[float32]()
	}
	v, _ := o.Get()
	return Some(float32(v))
}

func (c *Config) GetBool(path string) bool {
	sv := c.getScalar(path)
	parsed, ok := parseBool(sv.Raw)
	if !ok {
		panicConfig(path, fmt.Sprintf("expected bool, got %q", sv.Raw))
	}
	return parsed
}

func (c *Config) GetBoolOption(path string) Option[bool] {
	v, ok := c.lookupSoft(path)
	if !ok {
		return None[bool]()
	}
	sv, ok2 := v.(*resolver.ScalarVal)
	if !ok2 || sv.Type == resolver.ScalarNull {
		return None[bool]()
	}
	if parsed, ok3 := parseBool(sv.Raw); ok3 {
		return Some(parsed)
	}
	return None[bool]()
}

// parseBool parses a boolean string per the HOCON spec.
// Accepted values (case-insensitive): true, yes, on → true; false, no, off → false.
// Falls back to strconv.ParseBool for 1/0/t/f/T/F compatibility.
func parseBool(s string) (bool, bool) {
	switch strings.ToLower(s) {
	case "true", "yes", "on":
		return true, true
	case "false", "no", "off":
		return false, true
	}
	if v, err := strconv.ParseBool(s); err == nil {
		return v, true
	}
	return false, false
}

// ── duration ──────────────────────────────────────────────────────

func (c *Config) GetDuration(path string) time.Duration {
	s := c.GetString(path)
	d, err := parseDuration(s)
	if err != nil {
		panicConfig(path, "invalid duration: "+s)
	}
	return d
}

func (c *Config) GetDurationOption(path string) Option[time.Duration] {
	opt := c.GetStringOption(path)
	if opt.IsNone() {
		return None[time.Duration]()
	}
	s, _ := opt.Get()
	d, err := parseDuration(s)
	if err != nil {
		return None[time.Duration]()
	}
	return Some(d)
}

func parseDuration(s string) (time.Duration, error) {
	// Strip HOCON whitespace (not unicode.IsSpace — see isHoconWS).
	s = strings.TrimFunc(s, isHoconWS)

	// Scan optional sign + digits + optional fractional part.
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digStart := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == digStart {
		return 0, fmt.Errorf("no number in duration %q", s)
	}
	if i < len(s) && s[i] == '.' {
		i++ // consume dot
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	numStr := s[:i]

	// Consume optional HOCON whitespace between number and unit.
	for i < len(s) {
		r, w := utf8.DecodeRuneInString(s[i:])
		if !isHoconWS(r) {
			break
		}
		i += w
	}

	// Consume unit (letters only; error if non-letter non-WS remains).
	unitStart := i
	for i < len(s) && (s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z') {
		i++
	}
	unit := s[unitStart:i]

	// Trailing HOCON whitespace.
	for i < len(s) {
		r, w := utf8.DecodeRuneInString(s[i:])
		if !isHoconWS(r) {
			break
		}
		i += w
	}
	if i < len(s) {
		return 0, fmt.Errorf("unexpected characters in duration %q", s)
	}

	// Resolve multiplier.
	var mult time.Duration
	switch unit {
	case "": // S18.4 / S18.1: no unit → default milliseconds (HOCON.md L1301)
		mult = time.Millisecond
	case "ns", "nanosecond", "nanoseconds", "nano", "nanos": // S19.1
		mult = time.Nanosecond
	case "us", "micro", "micros", "microsecond", "microseconds": // S19.2
		mult = time.Microsecond
	case "ms", "millisecond", "milliseconds":
		mult = time.Millisecond
	case "s", "second", "seconds":
		mult = time.Second
	case "m", "minute", "minutes":
		mult = time.Minute
	case "h", "hour", "hours":
		mult = time.Hour
	case "d", "day", "days":
		mult = 24 * time.Hour
	default:
		return 0, fmt.Errorf("unknown duration unit %q", unit)
	}

	// Lightbend-faithful per-family fractional handling:
	// integer-form → int64 * mult; fractional → float64 * float64(mult).
	if integerRegex.MatchString(numStr) {
		n, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * mult, nil
	}
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(f * float64(mult)), nil
}

// ── bytes ─────────────────────────────────────────────────────────

func (c *Config) GetBytes(path string) int64 {
	s := c.GetString(path)
	n, err := parseBytes(s)
	if err != nil {
		panicConfig(path, "invalid byte size: "+s)
	}
	// S18.4 accessor invariant: byte sizes must be non-negative (Lightbend getBytesBigInteger).
	if n < 0 {
		panicConfig(path, "byte size must not be negative")
	}
	return n
}

func (c *Config) GetBytesOption(path string) Option[int64] {
	opt := c.GetStringOption(path)
	if opt.IsNone() {
		return None[int64]()
	}
	s, _ := opt.Get()
	n, err := parseBytes(s)
	// S18.4 accessor invariant: negative byte sizes return None (not panic).
	if err != nil || n < 0 {
		return None[int64]()
	}
	return Some(n)
}

func parseBytes(s string) (int64, error) {
	// Strip HOCON whitespace (not unicode.IsSpace — see isHoconWS).
	s = strings.TrimFunc(s, isHoconWS)

	// Scan optional sign + digits + optional fractional part.
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digStart := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == digStart {
		return 0, fmt.Errorf("no number in byte size %q", s)
	}
	hasFrac := false
	if i < len(s) && s[i] == '.' {
		hasFrac = true
		i++ // consume dot
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	numStr := s[:i]

	// Consume optional HOCON whitespace between number and unit.
	for i < len(s) {
		r, w := utf8.DecodeRuneInString(s[i:])
		if !isHoconWS(r) {
			break
		}
		i += w
	}

	// Consume unit (letters + uppercase allowed for bytes per HOCON.md L1344).
	unitStart := i
	for i < len(s) && (s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z') {
		i++
	}
	unit := s[unitStart:i]

	// Trailing HOCON whitespace.
	for i < len(s) {
		r, w := utf8.DecodeRuneInString(s[i:])
		if !isHoconWS(r) {
			break
		}
		i += w
	}
	if i < len(s) {
		return 0, fmt.Errorf("unexpected characters in byte size %q", s)
	}

	multipliers := map[string]int64{
		"":  1, // S18.4: no unit → bytes (HOCON.md L1341)
		"B": 1, "byte": 1, "bytes": 1,
		// S21.4: single-letter abbreviations → powers of two (HOCON.md L1385,
		// java -Xmx convention). Both upper- and lower-case are accepted.
		"K": 1 << 10, "k": 1 << 10,
		"M": 1 << 20, "m": 1 << 20,
		"G": 1 << 30, "g": 1 << 30,
		"T": 1 << 40, "t": 1 << 40,
		"P": 1 << 50, "p": 1 << 50,
		"E": 1 << 60, "e": 1 << 60,
		"KB": 1000, "kilobyte": 1000, "kilobytes": 1000,
		"KiB": 1024, "kibibyte": 1024, "kibibytes": 1024,
		"MB": 1_000_000, "megabyte": 1_000_000, "megabytes": 1_000_000,
		"MiB": 1024 * 1024, "mebibyte": 1024 * 1024, "mebibytes": 1024 * 1024,
		"GB": 1_000_000_000, "gigabyte": 1_000_000_000, "gigabytes": 1_000_000_000,
		"GiB": 1024 * 1024 * 1024, "gibibyte": 1024 * 1024 * 1024, "gibibytes": 1024 * 1024 * 1024,
		"TB": 1_000_000_000_000, "terabyte": 1_000_000_000_000, "terabytes": 1_000_000_000_000,
		"TiB": 1024 * 1024 * 1024 * 1024, "tebibyte": 1024 * 1024 * 1024 * 1024, "tebibytes": 1024 * 1024 * 1024 * 1024,
	}
	mult, ok := multipliers[unit]
	if !ok {
		return 0, fmt.Errorf("unknown byte unit %q", unit)
	}

	// Lightbend-faithful per-family fractional handling:
	// integer-form → int64 * mult; fractional → int64(f * float64(mult)) (truncate AFTER multiply).
	if !hasFrac && integerRegex.MatchString(numStr) {
		n, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			return 0, err
		}
		// Overflow check: detect int64 multiplication overflow (e.g. 8E = 8×2^60 > MaxInt64).
		if mult != 0 && n != 0 {
			result := n * mult
			if result/mult != n {
				return 0, fmt.Errorf("byte size %q overflows int64 representable range", s)
			}
			return result, nil
		}
		return n * mult, nil
	}
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, err
	}
	prod := f * float64(mult)
	// float64(math.MaxInt64) rounds UP to 2^63 (9223372036854775808.0), so
	// "prod > math.MaxInt64" is equivalent to "prod > 2^63" — it passes 2^63
	// itself (e.g. "8.0E" = 8×2^60 = 2^63) and then int64(prod) traps in
	// implementation-defined behaviour.  Use math.Exp2(63) (which IS exactly
	// representable as float64) so the boundary is caught correctly.
	if math.IsInf(prod, 0) || math.IsNaN(prod) || prod >= math.Exp2(63) || prod < math.MinInt64 {
		return 0, fmt.Errorf("byte size %q overflows int64 representable range", s)
	}
	return int64(prod), nil
}

// ── slices ────────────────────────────────────────────────────────

// lookupArray resolves path and returns an *ArrayVal, applying S15 numeric-object
// conversion when the value is an ObjectVal.  Returns (arr, true) on success,
// (nil, false) when the path is missing or the value cannot be converted.
// The caller must NOT call this when the path is expected to panic (use getArray).
func (c *Config) lookupArray(path string) (*resolver.ArrayVal, bool) {
	v, ok := c.lookupSoft(path)
	if !ok {
		return nil, false
	}
	// S15: try numeric-object → array conversion before the type check.
	if obj, isObj := v.(*resolver.ObjectVal); isObj {
		if converted, convOK := resolver.NumericObjectToArray(obj); convOK {
			return converted, true
		}
		// Non-eligible object (empty or no int keys) → not an array.
		return nil, false
	}
	arr, isArr := v.(*resolver.ArrayVal)
	if !isArr {
		return nil, false
	}
	return arr, true
}

func (c *Config) getArray(path string) *resolver.ArrayVal {
	v, ok := c.lookup(path)
	if !ok {
		panicConfig(path, "key not found")
	}
	// S15: if the value is a numerically-indexed object, attempt conversion to
	// an array before the type check.  Non-eligible objects (empty, no int keys)
	// fall through to the panic below — preserving existing error semantics.
	if obj, isObj := v.(*resolver.ObjectVal); isObj {
		if converted, convOK := resolver.NumericObjectToArray(obj); convOK {
			return converted
		}
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		panicConfig(path, fmt.Sprintf("expected array, got %T", v))
	}
	return arr
}

func (c *Config) GetStringSlice(path string) []string {
	arr := c.getArray(path)
	result := make([]string, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			panicConfig(path, fmt.Sprintf("element %d is not a non-null scalar", i))
		}
		result[i] = sv.Raw
	}
	return result
}

func (c *Config) GetStringSliceOption(path string) Option[[]string] {
	arr, ok := c.lookupArray(path)
	if !ok {
		return None[[]string]()
	}
	result := make([]string, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			return None[[]string]()
		}
		result[i] = sv.Raw
	}
	return Some(result)
}

func (c *Config) GetInt64Slice(path string) []int64 {
	arr := c.getArray(path)
	result := make([]int64, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			panicConfig(path, fmt.Sprintf("element %d is not an int", i))
		}
		parsed, err := strconv.ParseInt(sv.Raw, 10, 64)
		if err != nil {
			panicConfig(path, fmt.Sprintf("element %d: expected int64, got %q", i, sv.Raw))
		}
		result[i] = parsed
	}
	return result
}

func (c *Config) GetInt64SliceOption(path string) Option[[]int64] {
	arr, ok := c.lookupArray(path)
	if !ok {
		return None[[]int64]()
	}
	result := make([]int64, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			return None[[]int64]()
		}
		parsed, err := strconv.ParseInt(sv.Raw, 10, 64)
		if err != nil {
			return None[[]int64]()
		}
		result[i] = parsed
	}
	return Some(result)
}

func (c *Config) GetIntSlice(path string) []int {
	s := c.GetInt64Slice(path)
	r := make([]int, len(s))
	for i, v := range s {
		r[i] = int(v)
	}
	return r
}

func (c *Config) GetIntSliceOption(path string) Option[[]int] {
	arr, ok := c.lookupArray(path)
	if !ok {
		return None[[]int]()
	}
	result := make([]int, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			return None[[]int]()
		}
		parsed, err := strconv.ParseInt(sv.Raw, 10, 64)
		if err != nil {
			return None[[]int]()
		}
		result[i] = int(parsed)
	}
	return Some(result)
}

// ── object getters ────────────────────────────────────────────────

func (c *Config) GetConfig(path string) *Config {
	v, ok := c.lookup(path)
	if !ok {
		panicConfig(path, "key not found")
	}
	obj, ok := v.(*resolver.ObjectVal)
	if !ok {
		panicConfig(path, fmt.Sprintf("expected object, got %T", v))
	}
	return newConfig(obj)
}

func (c *Config) GetConfigOption(path string) Option[*Config] {
	v, ok := c.lookupSoft(path)
	if !ok {
		return None[*Config]()
	}
	sv, isSc := v.(*resolver.ScalarVal)
	if isSc && sv.Type == resolver.ScalarNull {
		return None[*Config]()
	}
	obj, ok2 := v.(*resolver.ObjectVal)
	if !ok2 {
		return None[*Config]()
	}
	return Some(newConfig(obj))
}

func (c *Config) GetConfigSlice(path string) []*Config {
	arr := c.getArray(path)
	result := make([]*Config, len(arr.Elements))
	for i, elem := range arr.Elements {
		obj, ok := elem.(*resolver.ObjectVal)
		if !ok {
			panicConfig(path, fmt.Sprintf("element %d is not an object", i))
		}
		result[i] = newConfig(obj)
	}
	return result
}

func (c *Config) GetConfigSliceOption(path string) Option[[]*Config] {
	arr, ok := c.lookupArray(path)
	if !ok {
		return None[[]*Config]()
	}
	result := make([]*Config, len(arr.Elements))
	for i, elem := range arr.Elements {
		obj, ok := elem.(*resolver.ObjectVal)
		if !ok {
			return None[[]*Config]()
		}
		result[i] = newConfig(obj)
	}
	return Some(result)
}

// ── merge ─────────────────────────────────────────────────────────

// WithFallback returns a new Config that deep-merges receiver over fallback.
// Neither receiver nor fallback is mutated. If fallback is nil, returns receiver.
//
// Per E12 § "Composition" (decision 5):
//   - Receiver's keys win.
//   - Accepts resolved AND unresolved operands. Result is unresolved iff
//     either operand is unresolved.
//   - Substitution placeholders survive merge unchanged. Substitution
//     lookup at Resolve() time uses the merged tree.
//   - Non-object collision captures fallback's value as a prior for cross-
//     layer self-reference lookback (S13a × WithFallback, dr04-dr06).
//
// Composition barrier (HOCON.md §Object Merge L1485, dr10):
//
//	obj.WithFallback(nonObj).WithFallback(otherObj) ignores otherObj because
//	nonObj is non-object and bars the merge.  Once a key holds a non-object
//	scalar, any object at the same key in a subsequent fallback simply does
//	not replace the scalar (receiver wins; the fallback object is captured
//	as prior but does not contribute keys to the result).
func (c *Config) WithFallback(fallback *Config) *Config {
	if fallback == nil || fallback.root == nil {
		return c
	}
	merged := resolver.MergeUnresolved(c.root, fallback.root)
	return &Config{
		root:              merged,
		resolved:          c.resolved && fallback.resolved && !resolver.ContainsPlaceholders(merged),
		parseBaseDir:      c.parseBaseDir,
		originDescription: c.originDescription,
	}
}
