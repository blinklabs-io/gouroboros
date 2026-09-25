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

package common

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// ProtocolParameterUpdateEra identifies the classic era whose update fields
// are being validated.
type ProtocolParameterUpdateEra uint8

const (
	// ProtocolParameterUpdateEraShelley identifies the Shelley update schema.
	ProtocolParameterUpdateEraShelley ProtocolParameterUpdateEra = iota
	// ProtocolParameterUpdateEraMary identifies the Mary update schema.
	ProtocolParameterUpdateEraMary
	// ProtocolParameterUpdateEraAlonzo identifies the Alonzo update schema.
	ProtocolParameterUpdateEraAlonzo
	// ProtocolParameterUpdateEraBabbage identifies the Babbage update schema.
	ProtocolParameterUpdateEraBabbage
)

// ValidateClassicCostModelUpdate applies the strict pre-PV9 cost-model
// decoder rules for the languages supported by the target era.
func ValidateClassicCostModelUpdate(
	models map[uint][]int64,
	protocolMajor uint,
	expectedLengths map[uint]int,
) error {
	if protocolMajor >= 9 {
		return nil
	}
	languages := make([]uint, 0, len(models))
	for language := range models {
		languages = append(languages, language)
	}
	sort.Slice(languages, func(i, j int) bool { return languages[i] < languages[j] })
	for _, language := range languages {
		model := models[language]
		expected, ok := expectedLengths[language]
		if !ok {
			return ProtocolParameterUpdateCostModelError{
				Language: language,
				Unknown:  true,
			}
		}
		if len(model) != expected {
			return ProtocolParameterUpdateCostModelError{
				Language: language,
				Expected: expected,
				Actual:   len(model),
			}
		}
	}
	return nil
}

// ProtocolParameterUpdateDomainError identifies a classic parameter field
// whose encoded value is outside the cardano-ledger domain.
type ProtocolParameterUpdateDomainError struct {
	Field  string
	Reason string
}

var protocolParameterUpdateWidths = [...]struct {
	key     uint64
	maximum uint64
}{
	{2, math.MaxUint32},
	{3, math.MaxUint32},
	{4, math.MaxUint16},
	{7, math.MaxUint32},
	{8, math.MaxUint16},
	{22, math.MaxUint32},
	{23, math.MaxUint16},
	{24, math.MaxUint16},
}

var protocolParameterUpdateIntervals = [...]struct {
	key  uint64
	unit bool
}{
	{9, false},
	{10, true},
	{11, true},
	{12, true},
}

var protocolParameterUpdateFieldNames = map[uint64]string{
	2:  "max block body size",
	3:  "max transaction size",
	4:  "max block header size",
	8:  "nOpt",
	9:  "a0",
	10: "rho",
	11: "tau",
	12: "decentralization",
	14: "protocol version",
	19: "execution prices",
	20: "max transaction execution units",
	21: "max block execution units",
	22: "max value size",
	23: "collateral percentage",
	24: "max collateral inputs",
}

func (e ProtocolParameterUpdateDomainError) Error() string {
	return fmt.Sprintf("protocol parameter update %s: %s", e.Field, e.Reason)
}

// ValidateProtocolParameterUpdateDomains checks encoded classic parameter
// updates against each era's domains and widths, then returns decoded fields
// for callers that need to inspect them.
func ValidateProtocolParameterUpdateDomains(
	data []byte,
	era ProtocolParameterUpdateEra,
) (map[uint64]cbor.RawMessage, error) {
	var fields map[uint64]cbor.RawMessage
	if _, err := cbor.Decode(data, &fields); err != nil {
		return nil, err
	}
	for _, width := range protocolParameterUpdateWidths {
		key, maximum := width.key, width.maximum
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var value uint64
		if _, err := cbor.Decode(raw, &value); err != nil {
			return nil, ProtocolParameterUpdateDomainError{
				Field:  protocolParameterUpdateFieldName(key),
				Reason: "must be an unsigned integer",
			}
		}
		if value > maximum {
			return nil, ProtocolParameterUpdateDomainError{
				Field: protocolParameterUpdateFieldName(key),
				Reason: fmt.Sprintf(
					"value %d exceeds maximum %d",
					value,
					maximum,
				),
			}
		}
	}
	for _, interval := range protocolParameterUpdateIntervals {
		key, unitInterval := interval.key, interval.unit
		if key == 12 && era > ProtocolParameterUpdateEraAlonzo {
			continue
		}
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var value cbor.Rat
		if _, err := cbor.Decode(raw, &value); err != nil || value.Rat == nil {
			return nil, ProtocolParameterUpdateDomainError{
				Field:  protocolParameterUpdateFieldName(key),
				Reason: "must be a bounded rational",
			}
		}
		if err := ValidateNonNegativeInterval(&value, unitInterval); err != nil {
			return nil, ProtocolParameterUpdateDomainError{
				Field:  protocolParameterUpdateFieldName(key),
				Reason: err.Error(),
			}
		}
	}
	if raw, ok := fields[14]; ok {
		var version ProtocolParametersProtocolVersion
		if _, err := cbor.Decode(raw, &version); err != nil {
			return nil, ProtocolParameterUpdateDomainError{
				Field:  "protocol version",
				Reason: "must contain unsigned 16-bit major and minor values",
			}
		}
		if version.Major > math.MaxUint16 || version.Minor > math.MaxUint16 {
			return nil, ProtocolParameterUpdateDomainError{
				Field:  "protocol version",
				Reason: "major and minor values must fit in 16 bits",
			}
		}
	}
	if era >= ProtocolParameterUpdateEraAlonzo {
		if raw, ok := fields[19]; ok {
			var prices ExUnitPrice
			if _, err := cbor.Decode(raw, &prices); err != nil {
				return nil, ProtocolParameterUpdateDomainError{
					Field:  "execution prices",
					Reason: "must contain bounded non-negative memory and step prices",
				}
			}
			for _, price := range []*cbor.Rat{prices.MemPrice, prices.StepPrice} {
				if ValidateNonNegativeInterval(price, false) != nil {
					return nil, ProtocolParameterUpdateDomainError{
						Field:  "execution prices",
						Reason: "memory and step prices must be bounded non-negative rationals",
					}
				}
			}
		}
		for _, key := range []uint64{20, 21} {
			raw, ok := fields[key]
			if !ok {
				continue
			}
			var units ExUnits
			if _, err := cbor.Decode(raw, &units); err != nil || units.Memory < 0 || units.Steps < 0 {
				return nil, ProtocolParameterUpdateDomainError{
					Field:  protocolParameterUpdateFieldName(key),
					Reason: "memory and steps must be non-negative",
				}
			}
		}
	}
	return fields, nil
}

// ValidateNonNegativeInterval checks that a CBOR rational is non-negative,
// fits in unsigned 64-bit numerator and denominator values, and optionally
// does not exceed one.
func ValidateNonNegativeInterval(value *cbor.Rat, unit bool) error {
	if value == nil || value.Rat == nil {
		return errors.New("must be a rational value")
	}
	numerator := value.Num()
	denominator := value.Denom()
	if !numerator.IsUint64() || !denominator.IsUint64() {
		return errors.New("numerator and denominator must fit in 64 bits")
	}
	if unit && numerator.Cmp(denominator) > 0 {
		return errors.New("must be between zero and one inclusive")
	}
	return nil
}

func protocolParameterUpdateFieldName(key uint64) string {
	if name, ok := protocolParameterUpdateFieldNames[key]; ok {
		return name
	}
	return fmt.Sprintf("field %d", key)
}
