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
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestDijkstraCurrentLedgerAccountBalanceIntervalsGolden(t *testing.T) {
	// Current cardano-ledger golden transaction at commit
	// c2114bdb1f815e8743fef2ce1b9846e97fa5f0bd:
	// eras/dijkstra/impl/golden/tx.cbor
	hexData, err := os.ReadFile(
		"testdata/cardano_ledger_dijkstra_current_tx.hex",
	)
	require.NoError(t, err)
	txCbor, err := hex.DecodeString(strings.Join(strings.Fields(string(hexData)), ""))
	require.NoError(t, err)

	tx, err := NewDijkstraTransactionFromCbor(txCbor)
	require.NoError(t, err)
	require.Len(t, tx.Body.TxBalanceIntervals, 3)

	for key := range tx.Body.TxBalanceIntervals {
		address, err := common.NewAddressFromBytes(key.Bytes())
		require.NoError(t, err)
		require.NoError(t, common.CheckAccountAddress(address))
		require.Len(t, key.Bytes(), 29)
	}
}
