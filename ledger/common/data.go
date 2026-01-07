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
	"github.com/blinklabs-io/plutigo/data"
)

type DatumHash = Blake2b256

// DatumHashToBech32 encodes a DatumHash as a CIP-0005 bech32 string with "datum" prefix.
func DatumHashToBech32(d DatumHash) string {
	return d.Bech32("datum")
}

// Datum represents a Plutus datum
type Datum struct {
	cbor.DecodeStoreCbor
	Data data.PlutusData `json:"data"`
}

func (d *Datum) UnmarshalCBOR(cborData []byte) error {
	d.SetCbor(cborData)
	tmpData, err := data.Decode(cborData)
	if err != nil {
		return err
	}
	d.Data = tmpData
	return nil
}

func (d *Datum) MarshalCBOR() ([]byte, error) {
	tmpCbor, err := data.Encode(d.Data)
	if err != nil {
		return nil, err
	}
	return tmpCbor, nil
}

func (d *Datum) Hash() DatumHash {
	return Blake2b256Hash(d.Cbor())
}
