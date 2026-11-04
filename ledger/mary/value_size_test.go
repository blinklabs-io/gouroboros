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

package mary

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// cborBoundaries are the values on either side of each change in CBOR
// head length.
var cborBoundaries = []uint64{
	0, 1, 23, 24, 255, 256, 65535, 65536,
	math.MaxUint32, math.MaxUint32 + 1, math.MaxUint64,
}

func randomBoundaryValue(r *rand.Rand) uint64 {
	if r.IntN(2) == 0 {
		return cborBoundaries[r.IntN(len(cborBoundaries))]
	}
	return r.Uint64() >> r.IntN(64)
}

// TestLedgerSerializedSizeMatchesEncoding checks the size computed without
// encoding against the length of the encoding the size was derived from, for
// values with no assets, policy and asset counts on both sides of the
// 23-entry map head change, asset names of every length, and quantities on
// both sides of every CBOR head length change.
func TestLedgerSerializedSizeMatchesEncoding(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for i := range 3000 {
		value := &MaryTransactionOutputValue{Amount: randomBoundaryValue(r)}
		if i%5 != 0 {
			policies := r.IntN(30) + 1
			if i%7 == 0 {
				policies = 0
			}
			data := make(map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput, policies)
			for range policies {
				var policy common.Blake2b224
				for j := range policy {
					policy[j] = byte(r.Uint32())
				}
				count := r.IntN(30) + 1
				assets := make(map[cbor.ByteString]common.MultiAssetTypeOutput, count)
				for range count {
					name := make([]byte, r.IntN(33))
					for j := range name {
						name[j] = byte(r.Uint32())
					}
					quantity := new(big.Int).SetUint64(randomBoundaryValue(r))
					switch r.IntN(8) {
					case 0:
						// Past 64 bits, where the encoder writes a bignum.
						quantity.Lsh(quantity, uint(r.IntN(70)+1))
					case 1:
						quantity.Add(quantity, big.NewInt(1))
					}
					if quantity.Sign() == 0 {
						quantity.SetInt64(1)
					}
					assets[cbor.NewByteString(name)] = quantity
				}
				data[policy] = assets
			}
			assets := common.NewMultiAsset(data)
			value.Assets = &assets
		}
		encoded, err := value.MarshalCBOR()
		if err != nil {
			t.Fatal(err)
		}
		want := len(encoded)
		if value.Assets != nil && len(value.Assets.Policies()) > 0 {
			want += value.Assets.LedgerSerializedSizeAdjustment()
		}
		got, err := value.LedgerSerializedSize()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("value %d: size %d, encoding is %d bytes", i, got, want)
		}
	}
}
