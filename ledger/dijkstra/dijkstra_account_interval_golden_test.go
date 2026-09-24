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

	"github.com/blinklabs-io/gouroboros/cbor"
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
	decodeKey := func(encoded string) cbor.ByteString {
		keyBytes, err := hex.DecodeString(encoded)
		require.NoError(t, err)
		return cbor.NewByteString(keyBytes)
	}
	fiveHundred := uint64(500)
	tenThousand := uint64(10_000)
	oneHundred := uint64(100)
	fiveThousand := uint64(5_000)
	expected := DijkstraAccountBalanceIntervals{
		decodeKey("e1415082a4d7a407bb3837bca2179336d8f9fa51fc4eecba911ead8407"): dijkstraIntervalBounds(&fiveHundred, nil),
		decodeKey("e101a1d395abb1baa33c53d26889d484437301cbba548c0fa0d28b4bd7"): dijkstraIntervalBounds(nil, &tenThousand),
		decodeKey("f1a9bfee58b8bda1a3df2861735baacb11b594c51dcfe49a4f2a6ea1c4"): dijkstraIntervalBounds(&oneHundred, &fiveThousand),
	}
	require.Equal(t, expected, tx.Body.TxBalanceIntervals)
	require.Nil(t, tx.Body.TxStartingBalanceIntervals)
}

func TestDijkstraAccountBalanceIntervalsRejectNonRewardAccounts(t *testing.T) {
	fixture, err := os.ReadFile(
		"testdata/cardano_ledger_dijkstra_current_tx.hex",
	)
	require.NoError(t, err)
	encoded := strings.Join(strings.Fields(string(fixture)), "")
	for _, rewardAccount := range []string{
		"581de101a1d395abb1baa33c53d26889d484437301cbba548c0fa0d28b4bd7",
		"581de1415082a4d7a407bb3837bca2179336d8f9fa51fc4eecba911ead8407",
	} {
		require.Contains(t, encoded, rewardAccount)
		encoded = strings.ReplaceAll(encoded, rewardAccount, "581d61"+rewardAccount[6:])
	}
	txCbor, err := hex.DecodeString(encoded)
	require.NoError(t, err)
	_, err = NewDijkstraTransactionFromCbor(txCbor)
	require.ErrorContains(
		t,
		err,
		"account balance intervals contains an invalid reward account",
	)
}
