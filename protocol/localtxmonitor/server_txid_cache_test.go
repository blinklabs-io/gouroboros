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
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/require"
)

type cacheTestTx struct {
	name string
	cbor []byte
	id   []byte
}

type cacheTestOp struct {
	name    string
	acquire []int // indexes into the transactions; nil for other ops
	hasTx   []byte
	release bool
}

// TestServerTxIdCacheMatchesSnapshot runs every sequence of four server steps
// (acquire one of several snapshots, ask HasTx for each ID, release) and
// checks after each step that HasTx answers membership in the held
// snapshot, that every cache entry is a transaction of that snapshot with
// its own ID, and that nothing is cached while no snapshot is held. Two of
// the transactions are the same transaction under different encodings, so
// they share an ID and have separate entries.
func TestServerTxIdCacheMatchesSnapshot(t *testing.T) {
	var txs []cacheTestTx
	for i, h := range []string{
		"82838080a080",
		"82839fff9fffa080", // the first, re-encoded: same ID
		"82838080a100410180",
		"82838080a100410280",
	} {
		txCbor, err := hex.DecodeString(h)
		require.NoError(t, err)
		tx, err := ledger.NewTransactionFromCbor(ledger.TxTypeByron, txCbor)
		require.NoError(t, err)
		txs = append(txs, cacheTestTx{
			name: fmt.Sprintf("t%d", i),
			cbor: txCbor,
			id:   tx.Hash().Bytes(),
		})
	}
	ops := []cacheTestOp{{name: "release", release: true}}
	for _, snap := range [][]int{{}, {0}, {0, 2}, {1, 2, 3}, {0, 1, 2, 3}} {
		ops = append(ops, cacheTestOp{
			name:    fmt.Sprintf("acquire%v", snap),
			acquire: snap,
		})
	}
	for _, i := range []int{0, 2, 3} {
		ops = append(ops, cacheTestOp{
			name:  "hasTx(" + txs[i].name + ")",
			hasTx: txs[i].id,
		})
	}
	ops = append(ops, cacheTestOp{name: "hasTx(missing)", hasTx: make([]byte, 32)})

	const depth = 4
	seq := make([]int, depth)
	var run func(int)
	run = func(pos int) {
		if pos == depth {
			runCacheSequence(t, txs, ops, seq)
			return
		}
		for i := range ops {
			seq[pos] = i
			run(pos + 1)
		}
	}
	run(0)
}

func runCacheSequence(
	t *testing.T,
	txs []cacheTestTx,
	ops []cacheTestOp,
	seq []int,
) {
	var mempool []TxAndEraId
	server := newTestMempoolServer(t, &mempool)
	held := false
	var snap []cacheTestTx
	var names []string
	for _, opIdx := range seq {
		op := ops[opIdx]
		names = append(names, op.name)
		trace := strings.Join(names, " ")
		switch {
		case op.release:
			// MsgRelease is legal only while a snapshot is held.
			if !held {
				continue
			}
			server.releaseMempool()
			held = false
			snap = nil
		case op.acquire != nil:
			mempool = nil
			snap = nil
			for _, i := range op.acquire {
				// A fresh copy, as a GetMempoolFunc would return, so a
				// carried entry must be re-pointed rather than compared by
				// address.
				mempool = append(mempool, TxAndEraId{
					EraId: ledger.TxTypeByron,
					Tx:    bytes.Clone(txs[i].cbor),
				})
				snap = append(snap, txs[i])
			}
			_, err := server.acquireMempool()
			require.NoError(t, err, trace)
			held = true
		default:
			// Queries are legal only while a snapshot is held.
			if !held {
				continue
			}
			found, err := server.hasTx(NewMsgHasTxForEra(ledger.TxTypeByron, op.hasTx))
			require.NoError(t, err, trace)
			want := false
			for _, tx := range snap {
				if bytes.Equal(tx.id, op.hasTx) {
					want = true
				}
			}
			require.Equal(t, want, found, "AnswersMatchSnapshot: %s", trace)
		}
		requireCacheWithinSnapshot(t, server, snap, held, trace)
	}
}

func requireCacheWithinSnapshot(
	t *testing.T,
	server *Server,
	snap []cacheTestTx,
	held bool,
	trace string,
) {
	t.Helper()
	if !held {
		require.Empty(t, server.txHashes, "NothingCachedWhenIdle: %s", trace)
		return
	}
	entries := 0
	for _, eraHashes := range server.txHashes {
		for _, entry := range eraHashes {
			entries++
			var owner *cacheTestTx
			for i := range snap {
				if bytes.Equal(snap[i].cbor, entry.tx) {
					owner = &snap[i]
				}
			}
			require.NotNil(t, owner, "CacheWithinSnapshot: %s", trace)
			require.Equal(t, owner.id, entry.hash.Bytes(), "EntriesCorrect: %s", trace)
			inSnapshot := false
			for _, tx := range server.mempoolTxs {
				if len(tx.Tx) > 0 && len(entry.tx) > 0 && &tx.Tx[0] == &entry.tx[0] {
					inSnapshot = true
				}
			}
			require.True(t, inSnapshot, "entry not re-pointed at the held snapshot: %s", trace)
		}
	}
	require.LessOrEqual(t, entries, len(snap), "CacheWithinSnapshot: %s", trace)
}
