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
	"bytes"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func TestDijkstraPlutusV4TxInfoConvertedOncePerLevel(t *testing.T) {
	var hash common.CredentialHash
	copy(hash[:], bytes.Repeat([]byte{0x42}, len(hash)))
	guard := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: hash,
	}
	tx := &DijkstraTransaction{TxIsValid: true}
	tx.Body.SetValidityIntervalUpperBound(7)
	level := dijkstraV4TestLevel(t, tx)
	purpose := script.ScriptPurposeGuarding{Guard: guard}

	txInfoOf := func(index uint32, value int64) data.PlutusData {
		t.Helper()
		context, err := dijkstraPlutusV4Context(
			level,
			purpose,
			common.RedeemerKey{Tag: common.RedeemerTagGuarding, Index: index},
			common.RedeemerValue{Data: common.Datum{
				Data: data.NewInteger(big.NewInt(value)),
			}},
		)
		require.NoError(t, err)
		return requireDijkstraV4Constr(t, context, 0, 4).Fields[0]
	}
	first := txInfoOf(0, 1)
	second := txInfoOf(1, 2)
	require.Same(t, first, second, "TxInfo converted once per level")

	fresh, err := dijkstraTxInfoV4(level)
	require.NoError(t, err)
	want, err := data.Encode(fresh)
	require.NoError(t, err)
	got, err := data.Encode(second)
	require.NoError(t, err)
	require.Equal(t, want, got)
}
