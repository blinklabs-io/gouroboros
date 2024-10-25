// Copyright 2024 Blink Labs Software
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
	"fmt"
	"log/slog"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type ProtocolParameterUpdate interface {
	IsProtocolParameterUpdate()
	Cbor() []byte
}

type ProtocolParametersProtocolVersion struct {
	cbor.StructAsArray
	Major uint
	Minor uint
}

type ProtocolParametersUtxorpc interface {
	Utxorpc() *cardano.PParams
}

const (
	NonceType0 = 0
	NonceType1 = 1
)

var NeutralNonce = Nonce{
	Type: NonceType0,
}

type Nonce struct {
	cbor.StructAsArray
	Type  uint
	Value [32]byte
}

func (n *Nonce) UnmarshalCBOR(data []byte) error {
	nonceType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}

	n.Type = uint(nonceType)

	switch nonceType {
	case NonceType0:
		// Value uses default value
	case NonceType1:
		if err := cbor.DecodeGeneric(data, n); err != nil {
			fmt.Printf("Nonce decode error: %+v\n", data)
			return err
		}
	default:
		return fmt.Errorf("unsupported nonce type %d", nonceType)
	}
	return nil
}

type ExUnit struct {
	cbor.StructAsArray
	Mem   uint
	Steps uint
}

type ExUnitPrice struct {
	cbor.StructAsArray
	MemPrice  *cbor.Rat
	StepPrice *cbor.Rat
}

// ConvertToUtxorpcCardanoCostModels converts a map of cost models for Plutus scripts into cardano.CostModels
// Only PlutusV(keys 1, 2, and 3) are supported.
func ConvertToUtxorpcCardanoCostModels(
	models map[uint][]int64,
) *cardano.CostModels {
	costModels := &cardano.CostModels{}
	for k, v := range models {
		costModel := &cardano.CostModel{Values: v}
		switch k {
		case 1:
			costModels.PlutusV1 = costModel
		case 2:
			costModels.PlutusV2 = costModel
		case 3:
			costModels.PlutusV3 = costModel
		default:
			slog.Warn("unsupported cost model version", "version", k)
		}
	}
	return costModels
}
