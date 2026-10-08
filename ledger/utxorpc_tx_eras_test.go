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

package ledger_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/require"
)

// TestTransactionUtxorpcEraProjection checks that every era's transaction
// carries the validity interval, witnesses and metadata into UTxO-RPC, and
// that the eras with a validity marker or mint field project those too.
func TestTransactionUtxorpcEraProjection(t *testing.T) {
	t.Parallel()
	addr := append([]byte{0x61}, bytes.Repeat([]byte{0x0a}, 28)...)
	policy := cbor.NewByteString(bytes.Repeat([]byte{0x11}, 28))
	for _, tc := range []struct {
		name        string
		txType      uint
		hasValidity bool
		hasMint     bool
		// Dijkstra transactions cannot encode a false marker.
		invalid bool
	}{
		{"shelley", ledger.TxTypeShelley, false, false, false},
		{"allegra", ledger.TxTypeAllegra, false, false, false},
		{"mary", ledger.TxTypeMary, false, true, false},
		{"alonzo", ledger.TxTypeAlonzo, true, true, true},
		{"babbage", ledger.TxTypeBabbage, true, true, true},
		{"conway", ledger.TxTypeConway, true, true, true},
		{"dijkstra", ledger.TxTypeDijkstra, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := map[uint]any{
				0: []any{[]any{bytes.Repeat([]byte{0x01}, 32), uint64(0)}},
				1: []any{[]any{addr, uint64(2_000_000)}},
				2: uint64(170_000),
				3: uint64(100),
			}
			if tc.hasMint {
				body[9] = map[cbor.ByteString]map[cbor.ByteString]int64{
					policy: {cbor.NewByteString([]byte("a")): -4},
				}
			}
			witnesses := map[uint]any{
				0: []any{[]any{
					bytes.Repeat([]byte{0x05}, 32),
					bytes.Repeat([]byte{0x06}, 64),
				}},
			}
			metadata := map[uint]any{1: "x"}
			parts := []any{body, witnesses}
			if tc.hasValidity {
				parts = append(parts, !tc.invalid)
			}
			parts = append(parts, metadata)
			raw, err := cbor.Encode(parts)
			require.NoError(t, err)

			tx, err := ledger.NewTransactionFromCbor(tc.txType, raw)
			require.NoError(t, err)
			got, err := tx.Utxorpc()
			require.NoError(t, err)

			require.Equal(t, uint64(100), got.Validity.GetTtl())
			require.Len(t, got.Witnesses.GetVkeywitness(), 1)
			require.Len(t, got.Auxiliary.GetMetadata(), 1)
			require.Equal(t, uint64(1), got.Auxiliary.Metadata[0].Label)
			require.Equal(t, "x", got.Auxiliary.Metadata[0].Value.GetText())
			require.Equal(t, !tc.invalid, got.Successful)
			if tc.hasMint {
				require.Len(t, got.Mint, 1)
				require.Equal(
					t,
					int64(-4),
					got.Mint[0].Assets[0].GetMintCoin().GetInt(),
				)
			} else {
				require.Empty(t, got.Mint)
			}
		})
	}
}
