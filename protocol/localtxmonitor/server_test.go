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

func TestServerHasTxMatchesEraAndHash(t *testing.T) {
	txCbor, err := hex.DecodeString("82838080a080")
	require.NoError(t, err)
	tx, err := ledger.NewTransactionFromCbor(ledger.TxTypeByron, txCbor)
	require.NoError(t, err)
	txId := tx.Hash().Bytes()
	server := &Server{
		mempoolTxs: []TxAndEraId{{
			EraId: ledger.TxTypeByron,
			txObj: tx,
		}},
	}

	require.True(t, server.hasTx(NewMsgHasTxForEra(ledger.TxTypeByron, txId)))
	require.False(t, server.hasTx(NewMsgHasTxForEra(ledger.TxTypeShelley, txId)))
}
