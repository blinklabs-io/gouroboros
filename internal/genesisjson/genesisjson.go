// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package genesisjson validates genesis JSON documents against the shape the
// reference implementation (cardano-ledger FromJSON instances, built on aeson
// withObject) accepts: required keys must be present, optional keys may be
// absent or null, unknown keys are ignored, and each typed value must lie in
// the range of its Haskell type.
//
// Validation runs on the raw JSON before the permissive struct decode, so the
// ranges are checked on the literal number rather than on a lossy Go value.
// Normalize additionally rewrites every validated number into the canonical
// form the struct decode accepts, so the value that was validated is the value
// that gets decoded. Values the validator does not examine pass through
// byte-for-byte.
package genesisjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Check validates one raw JSON value found at path and returns the value to
// decode in its place: the input unchanged, or a canonical rewrite of it.
// Leaf checks return plain errors that Normalize prefixes with the path;
// nested checks return errors that already carry their full path.
type Check func(path string, raw json.RawMessage) (json.RawMessage, error)

// typeError marks a value of the wrong JSON type or shape, as opposed to a
// value outside its range. aeson's (<|>) falls through to the next alternate
// key on the former only when the range check sits outside the alternation.
type typeError struct{ err error }

func (e *typeError) Error() string { return e.err.Error() }
func (e *typeError) Unwrap() error { return e.err }

func typeErr(err error) error {
	if err == nil {
		return nil
	}
	return &typeError{err: err}
}

// pathError is an error that already names the JSON path it refers to.
type pathError struct{ msg string }

func (e *pathError) Error() string { return e.msg }

func atPath(path string, err error) error {
	var pe *pathError
	if errors.As(err, &pe) {
		return err
	}
	return &pathError{msg: path + ": " + err.Error()}
}

// Field describes one key of a JSON object.
type Field struct {
	Name string
	// Optional keys follow aeson's (.:?): absent and null both mean unset.
	Optional bool
	Check    Check
	// AnyOf lists alternate key names; the field is satisfied by any one of
	// them, checked in order (aeson's o .: a <|> o .: b). A null value or a
	// value of the wrong type falls through to the next name.
	AnyOf []string
}

// Required returns a required field.
func Required(name string, check Check) Field {
	return Field{Name: name, Check: check}
}

// Optional returns an optional field.
func Optional(name string, check Check) Field {
	return Field{Name: name, Optional: true, Check: check}
}

// RequiredAnyOf returns a field satisfied by any one of the named keys.
func RequiredAnyOf(check Check, names ...string) Field {
	return Field{Name: names[0], Check: check, AnyOf: names}
}

// Validate checks data, which must be a JSON object, against fields. The
// context prefixes every error path (for example "protocolParams").
func Validate(context string, data []byte, fields ...Field) error {
	_, err := Normalize(context, data, fields...)
	return err
}

// Normalize validates data like Validate and returns the document to decode:
// every checked field holds the value its check returned, and every other key
// is carried over unchanged. A key that appears more than once resolves to its
// last occurrence, as it does in aeson, rather than being merged by the struct
// decode.
func Normalize(
	context string,
	data []byte,
	fields ...Field,
) (json.RawMessage, error) {
	obj, err := decodeObject(data)
	if err != nil {
		return nil, atPath(pathOrRoot(context), err)
	}
	if err := checkCaseVariants(context, obj, fields); err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(obj))
	for k, v := range obj {
		out[k] = v
	}
	for _, f := range fields {
		names := f.AnyOf
		if len(names) == 0 {
			names = []string{f.Name}
		}
		var found string
		var lastErr error
		for i, name := range names {
			v, ok := obj[name]
			if !ok || (isNull(v) && (f.Optional || len(names) > 1)) {
				continue
			}
			norm := v
			if f.Check != nil {
				path := join(context, name)
				norm, err = f.Check(path, v)
				if err != nil {
					var te *typeError
					if i < len(names)-1 && errors.As(err, &te) {
						lastErr = atPath(path, err)
						continue
					}
					return nil, atPath(path, err)
				}
			}
			out[name], found = norm, name
			break
		}
		if found != "" {
			continue
		}
		if lastErr != nil {
			return nil, lastErr
		}
		if f.Optional {
			continue
		}
		quoted := make([]string, len(names))
		for i, name := range names {
			quoted[i] = strconv.Quote(name)
		}
		return nil, &pathError{msg: fmt.Sprintf(
			"%s: missing required field %s",
			pathOrRoot(context),
			strings.Join(quoted, " or "),
		)}
	}
	return json.Marshal(out)
}

// checkCaseVariants rejects a key that differs from a declared key only in
// case. aeson ignores such a key, but encoding/json matches struct fields
// case-insensitively and lets a later key overwrite an earlier one, so the
// struct decode would replace the validated value with an unvalidated one.
func checkCaseVariants(
	context string,
	obj map[string]json.RawMessage,
	fields []Field,
) error {
	declared := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f.AnyOf) > 0 {
			declared = append(declared, f.AnyOf...)
		} else {
			declared = append(declared, f.Name)
		}
	}
	for key := range obj {
		for _, name := range declared {
			if key != name && strings.EqualFold(key, name) {
				return &pathError{msg: fmt.Sprintf(
					"%s: key %q differs only in case from %q",
					pathOrRoot(context),
					key,
					name,
				)}
			}
		}
	}
	return nil
}

func join(context, name string) string {
	if context == "" {
		return name
	}
	return context + "." + name
}

func pathOrRoot(context string) string {
	if context == "" {
		return "genesis"
	}
	return context
}

func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func decodeObject(data []byte) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("expected a JSON object")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// Object checks a nested object against fields.
func Object(fields ...Field) Check {
	return func(path string, raw json.RawMessage) (json.RawMessage, error) {
		return Normalize(path, raw, fields...)
	}
}

// MapOf checks that raw is an object whose every value satisfies check.
func MapOf(check Check) Check {
	return func(path string, raw json.RawMessage) (json.RawMessage, error) {
		obj, err := decodeObject(raw)
		if err != nil {
			return nil, err
		}
		if check == nil {
			return raw, nil
		}
		for key, value := range obj {
			child := path + "." + key
			norm, err := check(child, value)
			if err != nil {
				return nil, atPath(child, err)
			}
			obj[key] = norm
		}
		return json.Marshal(obj)
	}
}

// AnyObject checks that raw is a JSON object.
func AnyObject() Check {
	return func(_ string, raw json.RawMessage) (json.RawMessage, error) {
		if _, err := decodeObject(raw); err != nil {
			return nil, err
		}
		return raw, nil
	}
}

// Array checks that raw is a JSON array.
func Array() Check {
	return func(_ string, raw json.RawMessage) (json.RawMessage, error) {
		if t := bytes.TrimSpace(raw); len(t) == 0 || t[0] != '[' {
			return nil, errors.New("expected a JSON array")
		}
		return raw, nil
	}
}

// ArrayOrObject checks that raw is a JSON array or object, and so rejects
// null.
func ArrayOrObject() Check {
	return func(_ string, raw json.RawMessage) (json.RawMessage, error) {
		if t := bytes.TrimSpace(raw); len(t) == 0 ||
			(t[0] != '[' && t[0] != '{') {
			return nil, errors.New("expected a JSON array or object")
		}
		return raw, nil
	}
}

// Text checks that raw is a JSON string.
func Text() Check {
	return func(_ string, raw json.RawMessage) (json.RawMessage, error) {
		if t := bytes.TrimSpace(raw); len(t) == 0 || t[0] != '"' {
			return nil, errors.New("expected a JSON string")
		}
		return raw, nil
	}
}

// OneOf checks that raw is a JSON string equal to one of values.
func OneOf(values ...string) Check {
	return func(_ string, raw json.RawMessage) (json.RawMessage, error) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, errors.New("expected a JSON string")
		}
		for _, v := range values {
			if s == v {
				return raw, nil
			}
		}
		return nil, fmt.Errorf("value %q is not one of %v", s, values)
	}
}

// maxNumberExponent bounds the base-10 exponent of a Number. Beyond it the
// struct decode either refuses the literal or materialises a huge value.
const maxNumberExponent = 1000

// Number checks that raw is a JSON number and returns it as a plain decimal
// without an exponent, so the struct decode reads exactly the validated value.
func Number() Check {
	return func(_ string, raw json.RawMessage) (json.RawMessage, error) {
		coeff, exp, err := parseNumber(raw)
		if err != nil {
			return nil, typeErr(err)
		}
		if exp > maxNumberExponent || exp < -maxNumberExponent {
			return nil, fmt.Errorf("exponent out of range: %s", raw)
		}
		return json.RawMessage(plainDecimal(coeff, exp)), nil
	}
}

// plainDecimal renders coeff*10^exp without an exponent.
func plainDecimal(coeff *big.Int, exp int64) string {
	if coeff.Sign() == 0 {
		return "0"
	}
	sign := ""
	if coeff.Sign() < 0 {
		sign = "-"
	}
	digits := new(big.Int).Abs(coeff).String()
	switch {
	case exp >= 0:
		return sign + digits + strings.Repeat("0", int(exp))
	case int64(len(digits)) > -exp:
		cut := len(digits) + int(exp)
		return sign + digits[:cut] + "." + digits[cut:]
	default:
		return sign + "0." + strings.Repeat("0", int(-exp)-len(digits)) + digits
	}
}

// destination describes the Go type a validated integer is decoded into, when
// it is narrower than the reference type.
type destination struct {
	name string
	max  *big.Int
}

func goUintDest() *destination {
	max := new(big.Int).Lsh(big.NewInt(1), uint(strconv.IntSize))
	return &destination{name: "uint", max: max.Sub(max, big.NewInt(1))}
}

func goIntDest() *destination {
	max := new(big.Int).Lsh(big.NewInt(1), uint(strconv.IntSize-1))
	return &destination{name: "int", max: max.Sub(max, big.NewInt(1))}
}

func uintCheck(bits uint, nonZero bool, dest *destination) Check {
	return func(_ string, raw json.RawMessage) (json.RawMessage, error) {
		v, err := parseInteger(raw)
		if err != nil {
			return nil, typeErr(err)
		}
		if err := checkUint(v, bits); err != nil {
			return nil, err
		}
		if nonZero && v.Sign() == 0 {
			return nil, errors.New("value is zero")
		}
		if dest != nil && v.Cmp(dest.max) > 0 {
			return nil, fmt.Errorf(
				"value %s exceeds %s, the largest the decoded Go %s "+
					"(%d bits) field holds",
				v, dest.max, dest.name, strconv.IntSize,
			)
		}
		return json.RawMessage(v.String()), nil
	}
}

// Uint checks that raw is an integral JSON number within [0, 2^bits-1],
// matching a Haskell WordN, and returns it as a plain integer. aeson accepts
// an integral value written with an exponent or a trailing ".0"; the plain
// form is what the struct decode accepts.
func Uint(bits uint) Check {
	return uintCheck(bits, false, nil)
}

// NonZeroUint64 checks a Haskell NonZero Word64.
func NonZeroUint64() Check {
	return uintCheck(64, true, nil)
}

// GoUint is Uint for a value decoded into a Go uint. A value the reference
// accepts but the platform's uint cannot hold is refused with an error naming
// the limit; this is an intentional divergence, since uint is 32 bits on
// 32-bit platforms.
func GoUint(bits uint) Check {
	return uintCheck(bits, false, goUintDest())
}

// GoInt is Uint for a value decoded into a Go int, which cannot hold the upper
// half of the Word range.
func GoInt(bits uint) Check {
	return uintCheck(bits, false, goIntDest())
}

// NonZeroGoInt is NonZeroUint64 for a value decoded into a Go int.
func NonZeroGoInt() Check {
	return uintCheck(64, true, goIntDest())
}

func checkUint(v *big.Int, bits uint) error {
	max := new(big.Int).Lsh(big.NewInt(1), bits)
	max.Sub(max, big.NewInt(1))
	if v.Sign() < 0 || v.Cmp(max) > 0 {
		return fmt.Errorf("value %s is outside of Word%d bounds", v, bits)
	}
	return nil
}

// IntervalKind selects a bounded rational type from cardano-ledger BaseTypes.
type IntervalKind int

const (
	// UnitInterval is [0, 1].
	UnitInterval IntervalKind = iota
	// PositiveUnitInterval is [1e-19, 1]; the lower bound is
	// positiveIntervalEpsilon.
	PositiveUnitInterval
	// NonNegativeInterval is [0, 2^64-1].
	NonNegativeInterval
)

// maxDecimals is maxDecimalsWord64 in BaseTypes: the most decimal places a
// bounded ratio written as a JSON number may carry.
const maxDecimals = 19

// Interval checks a bounded-ratio JSON value the way its FromJSON does: an
// object with integral numerator and denominator, or a JSON number. A
// failure of either shape is a type error, because the reference applies the
// bounds inside the parser.
//
// The value is returned in a form the decode of a GenesisRat reads exactly: a
// number as a plain decimal, an object as its reduced numerator and
// denominator. The decode holds object components in int64, so a reduced
// component above math.MaxInt64 is refused; this is an intentional divergence
// from the Word64 range of the reference.
func Interval(kind IntervalKind) Check {
	return func(path string, raw json.RawMessage) (json.RawMessage, error) {
		r, isObject, err := parseRational(path, raw)
		if err != nil {
			var pe *pathError
			if errors.As(err, &pe) {
				return nil, err
			}
			return nil, typeErr(err)
		}
		if err := checkInterval(r, kind); err != nil {
			return nil, typeErr(err)
		}
		if !isObject {
			return json.RawMessage(ratDecimal(r)), nil
		}
		if !r.Num().IsInt64() || !r.Denom().IsInt64() {
			return nil, typeErr(fmt.Errorf(
				"ratio %s has a component above %d, the largest the "+
					"decoded int64 component holds",
				r.RatString(), int64(math.MaxInt64),
			))
		}
		return json.RawMessage(fmt.Sprintf(
			`{"numerator":%s,"denominator":%s}`,
			r.Num(), r.Denom(),
		)), nil
	}
}

// ratDecimal renders a rational whose denominator divides 10^maxDecimals as a
// plain decimal.
func ratDecimal(r *big.Rat) string {
	s := r.FloatString(maxDecimals)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s
}

func parseRational(path string, raw json.RawMessage) (*big.Rat, bool, error) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return nil, false, errors.New("expected a rational value")
	}
	if t[0] == '{' {
		obj, err := decodeObject(t)
		if err != nil {
			return nil, false, err
		}
		// The ratio is decoded case-insensitively, so a variant key could
		// replace the validated component.
		if err := checkCaseVariants(path, obj, []Field{
			Required("numerator", nil),
			Required("denominator", nil),
		}); err != nil {
			return nil, false, err
		}
		numRaw, ok := obj["numerator"]
		if !ok {
			return nil, false, errors.New(`missing required field "numerator"`)
		}
		denRaw, ok := obj["denominator"]
		if !ok {
			return nil, false, errors.New(
				`missing required field "denominator"`,
			)
		}
		num, err := parseInteger(numRaw)
		if err != nil {
			return nil, false, fmt.Errorf("numerator: %w", err)
		}
		den, err := parseInteger(denRaw)
		if err != nil {
			return nil, false, fmt.Errorf("denominator: %w", err)
		}
		if den.Sign() == 0 {
			return nil, false, errors.New("ratio denominator was 0")
		}
		return new(big.Rat).SetFrac(num, den), true, nil
	}
	coeff, exp, err := parseNumber(t)
	if err != nil {
		return nil, false, err
	}
	if coeff.Sign() < 0 {
		return nil, false, fmt.Errorf("value is negative: %s", t)
	}
	if exp <= 0 {
		if exp < -maxDecimals {
			return nil, false, fmt.Errorf("value is too precise: %s", t)
		}
		den := new(big.Int).Exp(big.NewInt(10), big.NewInt(-exp), nil)
		return new(big.Rat).SetFrac(coeff, den), false, nil
	}
	if exp > maxDecimals {
		return nil, false, fmt.Errorf("value is too big: %s", t)
	}
	num := new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil)
	num.Mul(num, coeff)
	return new(big.Rat).SetInt(num), false, nil
}

func checkInterval(r *big.Rat, kind IntervalKind) error {
	// boundRational rejects a numerator or denominator that does not fit a
	// Word64 before it looks at the bounds.
	maxWord := new(big.Int).Lsh(big.NewInt(1), 64)
	maxWord.Sub(maxWord, big.NewInt(1))
	if r.Num().Sign() < 0 || r.Num().Cmp(maxWord) > 0 ||
		r.Denom().Cmp(maxWord) > 0 {
		return fmt.Errorf("value is outside of bounds: %s", r.RatString())
	}
	var lower, upper *big.Rat
	switch kind {
	case UnitInterval:
		lower, upper = new(big.Rat), big.NewRat(1, 1)
	case PositiveUnitInterval:
		lower = new(big.Rat).SetFrac(
			big.NewInt(1),
			new(big.Int).Exp(big.NewInt(10), big.NewInt(maxDecimals), nil),
		)
		upper = big.NewRat(1, 1)
	case NonNegativeInterval:
		lower, upper = new(big.Rat), new(big.Rat).SetInt(maxWord)
	default:
		return errors.New("unknown interval kind")
	}
	if r.Cmp(lower) < 0 || r.Cmp(upper) > 0 {
		return fmt.Errorf("value is outside of bounds: %s", r.RatString())
	}
	return nil
}

// parseInteger parses a JSON number that must be integral.
func parseInteger(raw json.RawMessage) (*big.Int, error) {
	coeff, exp, err := parseNumber(raw)
	if err != nil {
		return nil, err
	}
	if exp < 0 {
		return nil, fmt.Errorf("value is not an integer: %s", raw)
	}
	// A value this large is out of range for every bounded integer type;
	// refuse before materialising it.
	if exp > 40 {
		return nil, fmt.Errorf("value is too big: %s", raw)
	}
	v := new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil)
	return v.Mul(v, coeff), nil
}

// maxSignificantDigits bounds the coefficient of any JSON number. Every
// bounded type here is exceeded by 40 significant digits, and converting a
// longer digit string to a big.Int costs superlinear time.
const maxSignificantDigits = 1000

// parseNumber splits a JSON number into a normalised coefficient and base-10
// exponent (trailing zeros of the coefficient folded into the exponent), the
// representation of aeson's Scientific. A negative zero has coefficient 0.
func parseNumber(raw json.RawMessage) (*big.Int, int64, error) {
	t := strings.TrimSpace(string(raw))
	if t == "" || (t[0] != '-' && (t[0] < '0' || t[0] > '9')) ||
		!json.Valid([]byte(t)) {
		return nil, 0, errors.New("expected a JSON number")
	}
	neg := strings.HasPrefix(t, "-")
	t = strings.TrimPrefix(t, "-")
	var exp int64
	if i := strings.IndexAny(t, "eE"); i >= 0 {
		e, err := strconv.ParseInt(strings.TrimPrefix(t[i+1:], "+"), 10, 32)
		if err != nil {
			return nil, 0, errors.New("exponent out of range")
		}
		exp = e
		t = t[:i]
	}
	intPart, fracPart, _ := strings.Cut(t, ".")
	digits := intPart + fracPart
	exp -= int64(len(fracPart))
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return new(big.Int), 0, nil
	}
	for strings.HasSuffix(digits, "0") {
		digits = digits[:len(digits)-1]
		exp++
	}
	if len(digits) > maxSignificantDigits {
		return nil, 0, fmt.Errorf(
			"value has too many significant digits: %d",
			len(digits),
		)
	}
	coeff, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, 0, errors.New("expected a JSON number")
	}
	if neg {
		coeff.Neg(coeff)
	}
	return coeff, exp, nil
}
