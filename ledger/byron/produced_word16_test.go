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

package byron_test

import (
	"encoding/hex"
	"math"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/stretchr/testify/require"
)

// byronTxWithOutputs returns a transaction whose body repeats a real decoded
// Byron output count times, so the canonical ID still encodes every output.
func byronTxWithOutputs(t *testing.T, count int) *byron.ByronTransaction {
	t.Helper()
	raw, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	block, err := byron.NewByronMainBlockFromCbor(raw)
	require.NoError(t, err)
	txs := block.Transactions()
	require.NotEmpty(t, txs)
	src, ok := txs[0].(*byron.ByronTransaction)
	require.True(t, ok)
	require.NotEmpty(t, src.Body.TxOutputs)
	body := byron.ByronTransactionBody{
		TxInputs:   src.Body.TxInputs,
		TxOutputs:  make([]byron.ByronTransactionOutput, count),
		Attributes: src.Body.Attributes,
	}
	for i := range body.TxOutputs {
		body.TxOutputs[i] = src.Body.TxOutputs[0]
	}
	return &byron.ByronTransaction{Body: body}
}

// TestByronTransactionProducedStopsAtWord16 pins the reference's Word16
// output indexing: txOutputUTxO (Cardano/Chain/UTxO/UTxO.hs) zips the outputs
// with [0 ..] :: [Word16], so only outputs 0 through 65535 enter the UTxO set.
// Later outputs stay in the transaction body and its ID.
func TestByronTransactionProducedStopsAtWord16(t *testing.T) {
	t.Parallel()
	const limit = math.MaxUint16 + 1
	for _, count := range []int{limit - 1, limit, limit + 1, limit + 2} {
		tx := byronTxWithOutputs(t, count)
		require.Equal(t, count, len(tx.Outputs()))

		produced := tx.Produced()
		require.Equal(t, min(count, limit), len(produced), "outputs=%d", count)
		for idx, utxo := range produced {
			require.Equal(t, tx.Hash(), utxo.Id.Id())
			require.Equal(t, uint32(idx), utxo.Id.Index())
		}
	}

	// The tail output is excluded from the UTxO set but not from the
	// transaction: it still changes the transaction ID.
	atLimit := byronTxWithOutputs(t, limit)
	pastLimit := byronTxWithOutputs(t, limit+1)
	require.NotEqual(t, atLimit.Hash(), pastLimit.Hash())
	require.Equal(t, pastLimit.Hash(), pastLimit.Produced()[limit-1].Id.Id())
}
