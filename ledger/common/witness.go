// Copyright 2025 Blink Labs Software
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
	"github.com/blinklabs-io/gouroboros/cbor"
)

type VkeyWitness struct {
	cbor.StructAsArray
	Vkey      []byte
	Signature []byte
}

type BootstrapWitness struct {
	cbor.StructAsArray
	PublicKey  []byte
	Signature  []byte
	ChainCode  []byte
	Attributes []byte
}

// ValidateCollateralVKeyWitnesses ensures collateral inputs are backed by vkey witnesses (payment key).
// This is a shared helper used across Alonzo, Babbage, and Conway eras.
func ValidateCollateralVKeyWitnesses(
	tx Transaction,
	ls LedgerState,
) error {
	collateral := tx.Collateral()
	if len(collateral) == 0 {
		return nil
	}
	// Collect vkey hashes from witnesses
	w := tx.Witnesses()
	if w == nil || len(w.Vkey()) == 0 {
		return NewValidationError(
			ValidationErrorTypeTransaction,
			"missing vkey witnesses for collateral",
			nil,
			nil,
		)
	}
	hashes := make(map[Blake2b224]struct{})
	for _, vw := range w.Vkey() {
		hashes[Blake2b224Hash(vw.Vkey)] = struct{}{}
	}
	// Ensure each collateral input is owned by a provided vkey witness
	for _, input := range collateral {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"UTxO not found for collateral input",
				map[string]any{"input": input.String()},
				err,
			)
		}
		addr := utxo.Output.Address()
		cred := addr.PayloadPayload()
		pk, ok := cred.(*AddressPayloadKeyHash)
		if !ok {
			// Collateral should be key-locked; scripts cannot serve
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"collateral input must be key-locked",
				map[string]any{"input": input.String()},
				nil,
			)
		}
		h := pk.Hash
		if _, ok := hashes[h]; !ok {
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"missing vkey witness for collateral input",
				map[string]any{
					"input":   input.String(),
					"keyhash": h.String(),
				},
				nil,
			)
		}
	}
	return nil
}
