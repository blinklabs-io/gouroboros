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

package transactionwire

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// AlonzoFamilyEnvelope is the common outer transaction shape used from Alonzo
// through Conway. Its byte slices reference the input passed to
// DecodeAlonzoFamilyEnvelope.
type AlonzoFamilyEnvelope struct {
	Body       []byte
	WitnessSet []byte
	WireSize   int
	third      []byte
	fourth     []byte
}

type rawCborReference []byte

func (r *rawCborReference) UnmarshalCBOR(data []byte) error {
	*r = data
	return nil
}

// DecodeAlonzoFamilyEnvelope classifies the common three- or four-component
// transaction envelope without decoding the era-specific components.
func DecodeAlonzoFamilyEnvelope(data []byte) (AlonzoFamilyEnvelope, error) {
	var components []rawCborReference
	bytesRead, err := cbor.Decode(data, &components)
	if err != nil {
		return AlonzoFamilyEnvelope{}, err
	}
	if len(components) != 3 && len(components) != 4 {
		return AlonzoFamilyEnvelope{}, fmt.Errorf(
			"invalid transaction: expected 3 or 4 components, got %d",
			len(components),
		)
	}

	ret := AlonzoFamilyEnvelope{
		Body:       components[0],
		WitnessSet: components[1],
		WireSize:   bytesRead,
		third:      components[2],
	}
	if len(components) == 4 {
		ret.fourth = components[3]
	}
	return ret, nil
}

// DecodeValidityAndAuxiliaryData classifies the envelope fields that follow
// the body and witness set. Era decoders call it only after those components
// have decoded successfully, preserving their error precedence.
func (e AlonzoFamilyEnvelope) DecodeValidityAndAuxiliaryData() (
	bool,
	[]byte,
	error,
) {
	if e.fourth != nil {
		isValid, err := cbor.DecodeBool(e.third)
		if err != nil {
			return false, nil, fmt.Errorf(
				"failed to decode TxIsValid: %w",
				err,
			)
		}
		return isValid, e.fourth, nil
	}
	if len(e.third) == 1 && (e.third[0] == 0xf4 || e.third[0] == 0xf5) {
		return false, nil, errors.New(
			"invalid transaction: IsValid requires an auxiliary data component",
		)
	}
	return true, e.third, nil
}
