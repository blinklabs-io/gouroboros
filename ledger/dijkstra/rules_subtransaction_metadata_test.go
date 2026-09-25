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

func dijkstraRawMetadataTransaction(
	t *testing.T,
	parentHash *common.Blake2b256,
	parentAux []byte,
	childBodies []map[uint]any,
	childAux [][]byte,
) *DijkstraTransaction {
	t.Helper()
	require.Len(t, childBodies, len(childAux))
	children := make([]cbor.RawMessage, len(childBodies))
	for i, bodyFields := range childBodies {
		bodyFields[0] = cbor.NewSetType([]any{}, true)
		bodyFields[1] = []any{}
		bodyRaw, err := cbor.Encode(bodyFields)
		require.NoError(t, err)
		auxRaw := childAux[i]
		if auxRaw == nil {
			auxRaw = []byte{0xf6}
		}
		childRaw, err := cbor.Encode([]any{
			cbor.RawMessage(bodyRaw),
			map[uint]any{},
			cbor.RawMessage(auxRaw),
		})
		require.NoError(t, err)
		children[i] = childRaw
	}
	topBodyFields := map[uint]any{
		0:  cbor.NewSetType([]any{}, true),
		1:  []any{},
		2:  uint64(0),
		23: cbor.NewSetType(children, true),
	}
	if parentHash != nil {
		topBodyFields[7] = *parentHash
	}
	topBody, err := cbor.Encode(topBodyFields)
	require.NoError(t, err)
	if parentAux == nil {
		parentAux = []byte{0xf6}
	}
	txRaw, err := cbor.Encode([]any{
		cbor.RawMessage(topBody),
		map[uint]any{},
		cbor.RawMessage(parentAux),
	})
	require.NoError(t, err)
	tx, err := NewDijkstraTransactionFromCbor(txRaw)
	require.NoError(t, err)
	return tx
}

func validateDijkstraMetadata(t *testing.T, tx *DijkstraTransaction) error {
	t.Helper()
	return dijkstraRule(t, common.UtxoValidationRuleMetadata)(
		tx,
		0,
		nil,
		dijkstraGuardTestPParams(),
	)
}

func TestDijkstraMetadataValidationUsesEachSubtransactionAuxiliaryData(
	t *testing.T,
) {
	parentAux := []byte{0xa1, 0x01, 0x02}
	firstChildAux := []byte{0xa1, 0x01, 0x03}
	secondChildAux := []byte{0xa1, 0x01, 0x04}
	parentHash := common.Blake2b256Hash(parentAux)
	firstChildHash := common.Blake2b256Hash(firstChildAux)
	secondChildHash := common.Blake2b256Hash(secondChildAux)
	tx := dijkstraRawMetadataTransaction(
		t,
		&parentHash,
		parentAux,
		[]map[uint]any{{7: firstChildHash}, {7: secondChildHash}},
		[][]byte{firstChildAux, secondChildAux},
	)
	require.NoError(t, validateDijkstraMetadata(t, tx))
}

func TestDijkstraMetadataValidationRejectsMalformedSubtransactionPairs(
	t *testing.T,
) {
	matchingAux := []byte{0xa1, 0x01, 0x02}
	matchingHash := common.Blake2b256Hash(matchingAux)
	wrongHash := common.Blake2b256Hash([]byte{0xa1, 0x01, 0x03})
	tests := []struct {
		name      string
		body      map[uint]any
		auxiliary []byte
	}{
		{
			name:      "hash without child auxiliary data",
			body:      map[uint]any{7: matchingHash},
			auxiliary: nil,
		},
		{
			name:      "child auxiliary data without hash",
			body:      map[uint]any{},
			auxiliary: matchingAux,
		},
		{
			name:      "child hash does not match child auxiliary data",
			body:      map[uint]any{7: wrongHash},
			auxiliary: matchingAux,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := dijkstraRawMetadataTransaction(
				t,
				nil,
				nil,
				[]map[uint]any{tt.body},
				[][]byte{tt.auxiliary},
			)
			require.Error(t, validateDijkstraMetadata(t, tx))
		})
	}
}
