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

package dijkstra

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestDijkstraTransactionHashBodyCopy(t *testing.T) {
	raw, err := cbor.Encode(minimalTxParts())
	require.NoError(t, err)
	tx, err := NewDijkstraTransactionFromCbor(raw)
	require.NoError(t, err)
	expected := common.Blake2b256Hash(tx.Body.Cbor())
	for _, primed := range []bool{false, true} {
		if primed {
			require.Equal(t, expected, tx.Body.Id())
		}
		copy := *tx
		var value common.Transaction = copy
		var pointer common.Transaction = &copy
		require.Equal(t, expected, value.Hash())
		require.Equal(t, expected, pointer.Hash())
		require.Equal(t, expected, copy.Id())
		// Replacing the copy's body bytes must not reset the original's
		// primed memo or leave the copy using that shared memo.
		replacement, err := cbor.Encode(map[uint]any{
			0: []any{}, 1: []any{}, 2: uint64(42),
		})
		require.NoError(t, err)
		copy.Body.SetCbor(replacement)
		require.Equal(t, common.Blake2b256Hash(replacement), copy.Hash())
		require.Equal(t, expected, tx.Hash())
	}
}

var dijkstraHashBenchmarkSink common.Blake2b256

func BenchmarkDijkstraTransactionHash(b *testing.B) {
	for _, primed := range []bool{false, true} {
		name := "unprimed"
		if primed {
			name = "primed"
		}
		b.Run(name, func(b *testing.B) {
			raw, err := cbor.Encode(minimalTxParts())
			require.NoError(b, err)
			tx, err := NewDijkstraTransactionFromCbor(raw)
			require.NoError(b, err)
			if primed {
				tx.Body.Id()
			}
			var transaction common.Transaction = tx
			b.ReportAllocs()
			for b.Loop() {
				dijkstraHashBenchmarkSink = transaction.Hash()
			}
		})
	}
}
