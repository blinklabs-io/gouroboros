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

package localtxmonitor

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/require"
)

const testByronTxHex = "82838080a080"

func newTestMempoolServer(
	t *testing.T,
	txs *[]TxAndEraId,
) *Server {
	t.Helper()
	return &Server{
		config: &Config{
			GetMempoolFunc: func(CallbackContext) (uint64, uint32, []TxAndEraId, error) {
				return 12345, 1 << 20, *txs, nil
			},
		},
	}
}

func testByronTx(t *testing.T) ([]byte, []byte) {
	t.Helper()
	txCbor, err := hex.DecodeString(testByronTxHex)
	require.NoError(t, err)
	tx, err := ledger.NewTransactionFromCbor(ledger.TxTypeByron, txCbor)
	require.NoError(t, err)
	return txCbor, tx.Hash().Bytes()
}

func TestServerHasTxMatchesEraAndHash(t *testing.T) {
	txCbor, txId := testByronTx(t)
	txs := []TxAndEraId{{EraId: ledger.TxTypeByron, Tx: txCbor}}
	server := newTestMempoolServer(t, &txs)
	_, err := server.acquireMempool()
	require.NoError(t, err)

	found, err := server.hasTx(NewMsgHasTxForEra(ledger.TxTypeByron, txId))
	require.NoError(t, err)
	require.True(t, found)
	found, err = server.hasTx(NewMsgHasTxForEra(ledger.TxTypeShelley, txId))
	require.NoError(t, err)
	require.False(t, found)
}

func TestServerAcquireDoesNotParseMempool(t *testing.T) {
	txCbor, _ := testByronTx(t)
	txs := make([]TxAndEraId, 1000)
	for i := range txs {
		txs[i] = TxAndEraId{EraId: ledger.TxTypeByron, Tx: txCbor}
	}
	server := newTestMempoolServer(t, &txs)

	allocs := testing.AllocsPerRun(5, func() {
		_, err := server.acquireMempool()
		require.NoError(t, err)
	})
	// One allocation holds the snapshot; parsing each transaction would cost
	// several per transaction.
	require.LessOrEqual(t, allocs, float64(10))
}

func TestServerHasTxReusesIdsAcrossAcquire(t *testing.T) {
	txCbor, _ := testByronTx(t)
	txs := make([]TxAndEraId, 1000)
	for i := range txs {
		txs[i] = TxAndEraId{EraId: ledger.TxTypeByron, Tx: txCbor}
	}
	server := newTestMempoolServer(t, &txs)
	missing := NewMsgHasTxForEra(ledger.TxTypeByron, make([]byte, 32))
	_, err := server.acquireMempool()
	require.NoError(t, err)
	found, err := server.hasTx(missing)
	require.NoError(t, err)
	require.False(t, found)

	allocs := testing.AllocsPerRun(5, func() {
		_, err := server.acquireMempool()
		require.NoError(t, err)
		_, err = server.hasTx(missing)
		require.NoError(t, err)
	})
	// A miss compares against every transaction. Reparsing them would cost
	// several allocations each, rather than a few for the poll as a whole.
	require.LessOrEqual(t, allocs, float64(10))
}

func TestServerAcquireDropsIdsLeavingTheMempool(t *testing.T) {
	txCbor, txId := testByronTx(t)
	txs := []TxAndEraId{{EraId: ledger.TxTypeByron, Tx: txCbor}}
	server := newTestMempoolServer(t, &txs)
	_, err := server.acquireMempool()
	require.NoError(t, err)
	found, err := server.hasTx(NewMsgHasTxForEra(ledger.TxTypeByron, txId))
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, server.txHashes[ledger.TxTypeByron], 1)

	txs = nil
	_, err = server.acquireMempool()
	require.NoError(t, err)
	require.Empty(t, server.txHashes)
	found, err = server.hasTx(NewMsgHasTxForEra(ledger.TxTypeByron, txId))
	require.NoError(t, err)
	require.False(t, found)
}

func TestServerAcquireKeepsIdsOfRemainingTransactions(t *testing.T) {
	staysCbor, txId := testByronTx(t)
	// An equivalent Byron transaction with indefinite-length lists: the same
	// ID under a different encoding, so it is cached as a separate entry.
	leavesCbor, err := hex.DecodeString("82839fff9fffa080")
	require.NoError(t, err)
	txs := []TxAndEraId{
		{EraId: ledger.TxTypeByron, Tx: staysCbor},
		{EraId: ledger.TxTypeByron, Tx: leavesCbor},
	}
	server := newTestMempoolServer(t, &txs)
	_, err = server.acquireMempool()
	require.NoError(t, err)
	_, err = server.hasTx(
		NewMsgHasTxForEra(ledger.TxTypeByron, make([]byte, 32)),
	)
	require.NoError(t, err)
	require.Len(t, server.txHashes[ledger.TxTypeByron], 2)
	stays := server.txHashes[ledger.TxTypeByron][string(staysCbor)]
	require.NotNil(t, stays)

	txs = txs[:1]
	_, err = server.acquireMempool()
	require.NoError(t, err)
	require.Len(t, server.txHashes[ledger.TxTypeByron], 1)
	require.Same(
		t,
		stays,
		server.txHashes[ledger.TxTypeByron][string(staysCbor)],
	)
	found, err := server.hasTx(NewMsgHasTxForEra(ledger.TxTypeByron, txId))
	require.NoError(t, err)
	require.True(t, found)
}

func TestServerHasTxReportsUnparseableTransaction(t *testing.T) {
	txs := []TxAndEraId{{EraId: ledger.TxTypeConway, Tx: []byte{0x80}}}
	server := newTestMempoolServer(t, &txs)
	_, err := server.acquireMempool()
	require.NoError(t, err)

	_, err = server.hasTx(
		NewMsgHasTxForEra(ledger.TxTypeConway, make([]byte, 32)),
	)
	require.ErrorContains(t, err, "parse mempool transaction 0 (era 6)")
}
