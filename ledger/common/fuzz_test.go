// Copyright 2024 Cardano Foundation
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
