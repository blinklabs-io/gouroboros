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
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oversizedDijkstraTxCbor builds a well-formed Dijkstra transaction whose CBOR
// exceeds the current Cardano max_tx_size of 16384 bytes. The bulk is carried
// by direct_deposits (body key 25), which is a Dijkstra-only body key, so the
// transaction is unambiguously a Dijkstra era candidate.
func oversizedDijkstraTxCbor(t *testing.T) []byte {
	t.Helper()
	const entries = 600
	deposits := make(map[cbor.ByteString]uint64, entries)
	for i := range entries {
		credential := bytes.Repeat([]byte{0x00}, common.Blake2b224Size)
		credential[0] = byte(i)
		credential[1] = byte(i >> 8)
		credential[2] = byte(i >> 16)
		deposits[cbor.NewByteString(credential)] = uint64(i + 1)
	}
	txCbor, err := cbor.Encode([]any{
		map[uint]any{
			0:  []any{},
			1:  []any{},
			2:  uint64(0),
			25: deposits,
		},
		map[uint]any{},
		nil,
	})
	require.NoError(t, err)
	require.Greater(t, len(txCbor), 16*1024)
	return txCbor
}

// TestDetermineTransactionTypeOversizedDijkstra covers the era classification
// consequence of a decode-time size limit. looksLikeDijkstraTransaction
// consulted the same constant the decoder did, so an oversized Dijkstra
// transaction was not reported as oversized: the Dijkstra candidate was
// dropped and DetermineTransactionType fell through to the earlier eras.
func TestDetermineTransactionTypeOversizedDijkstra(t *testing.T) {
	txType, err := ledger.DetermineTransactionType(oversizedDijkstraTxCbor(t))
	require.NoError(t, err)
	assert.Equal(t, uint(ledger.TxTypeDijkstra), txType)
}

// TestOversizedDijkstraTransactionFailsTheSizeRule pins where the bound lives.
// max_tx_size is a protocol parameter, so an oversized transaction is rejected
// by UtxoValidateMaxTxSizeUtxo against the value in force, not by the decoder
// against a hardcoded constant.
func TestOversizedDijkstraTransactionFailsTheSizeRule(t *testing.T) {
	txCbor := oversizedDijkstraTxCbor(t)
	tx, err := dijkstra.NewDijkstraTransactionFromCbor(txCbor)
	require.NoError(t, err)
	ls := mockledger.NewLedgerStateBuilder().Build()

	t.Run("rejected at the current max_tx_size", func(t *testing.T) {
		pparams := &dijkstra.DijkstraProtocolParameters{}
		pparams.MaxTxSize = 16384
		err := dijkstra.UtxoValidateMaxTxSizeUtxo(tx, 0, ls, pparams)
		var sizeErr shelley.MaxTxSizeUtxoError
		require.ErrorAs(t, err, &sizeErr)
		assert.Equal(t, uint(len(txCbor)), sizeErr.TxSize)
		assert.Equal(t, uint(16384), sizeErr.MaxTxSize)
	})

	t.Run("accepted when max_tx_size permits it", func(t *testing.T) {
		pparams := &dijkstra.DijkstraProtocolParameters{}
		pparams.MaxTxSize = uint(len(txCbor))
		require.NoError(
			t,
			dijkstra.UtxoValidateMaxTxSizeUtxo(tx, 0, ls, pparams),
		)
	})
}
