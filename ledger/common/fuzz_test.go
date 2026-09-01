// Copyright 2024 Cardano Foundation
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

//go:build go1.18

package common

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

func FuzzMultiAsset(f *testing.F) {
	// Seed with some valid MultiAsset data
	seeds := []string{
		"a1581c0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f1fa1581c202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f1a000f4240",
		"a1581c0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f1fa1", // empty inner map
	}
	for _, seed := range seeds {
		data, _ := hex.DecodeString(seed)
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var ma MultiAsset[uint64]
		_, _ = cbor.Decode(data, &ma)
		// Should not panic - that's the test
	})
}

func FuzzValue(f *testing.F) {
	// Seed with some valid CBOR Value data
	seeds := []string{
		"821a000f42401a000186a0", // coin only
		"831a000f42401a000186a0a1581c0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f1fa1", // with assets
	}
	for _, seed := range seeds {
		data, _ := hex.DecodeString(seed)
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var v cbor.Value
		_, _ = cbor.Decode(data, &v)
		// Should not panic - that's the test
	})
}

func FuzzDecodeMetadatumRaw(f *testing.F) {
	seeds := []string{
		"00",         // int 0
		"626f6b",     // text "ok"
		"420102",     // bytes
		"82016161",   // list
		"a101626f6b", // map
		"a28031f730", // unsupported simple-value key
	}
	for _, seed := range seeds {
		data, _ := hex.DecodeString(seed)
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeMetadatumRaw(data)
		// Should not panic - invalid metadata should return an error.
	})
}

func FuzzDecodeAuxiliaryDataToMetadata(f *testing.F) {
	seeds := []string{
		"a101626f6b",             // Shelley metadata map
		"82a101626f6b80",         // Shelley-MA metadata + scripts
		"82f680",                 // Shelley-MA null metadata
		"d90103a100a101626f6b",   // Alonzo metadata under key 0
		"d90103a200a101626f6b06", // Alonzo metadata + extension key
		"a106d90103a200a101626f6b068101",
	}
	for _, seed := range seeds {
		data, _ := hex.DecodeString(seed)
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeAuxiliaryDataToMetadata(data)
		if aux, err := DecodeAuxiliaryData(data); err == nil {
			_, _ = aux.Metadata()
		}
		// Should not panic - invalid auxiliary data should return an error.
	})
}

func FuzzTransactionMetadataSet(f *testing.F) {
	seeds := []string{
		"a0",                             // empty metadata set
		"a100a101626f6b",                 // Shelley metadata at tx index 0
		"a106d90103a200a101626f6b068101", // Alonzo metadata with extension key
		"a400a101626f6b01a28031f73002f60382f680",
	}
	for _, seed := range seeds {
		data, _ := hex.DecodeString(seed)
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var set TransactionMetadataSet
		_, _ = cbor.Decode(data, &set)
		// Should not panic - malformed metadata sets should return an error.
	})
}
