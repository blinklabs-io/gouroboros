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
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionMarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		cborHex   string
		txType    uint
		checkKeys []string
	}{
		{
			name:      "Byron",
			cborHex:   byronTxCborHex,
			txType:    ledger.TxTypeByron,
			checkKeys: []string{"Body"},
		},
		{
			name:      "Shelley",
			cborHex:   shelleyTxCborHex,
			txType:    ledger.TxTypeShelley,
			checkKeys: []string{"Body", "WitnessSet"},
		},
		{
			name:      "Allegra",
			cborHex:   allegraTxCborHex,
			txType:    ledger.TxTypeAllegra,
			checkKeys: []string{"Body", "WitnessSet"},
		},
		{
			name:      "Mary",
			cborHex:   maryTxCborHex,
			txType:    ledger.TxTypeMary,
			checkKeys: []string{"Body", "WitnessSet"},
		},
		{
			name:      "Alonzo",
			cborHex:   alonzoTxCborHex,
			txType:    ledger.TxTypeAlonzo,
			checkKeys: []string{"Body", "WitnessSet", "TxIsValid"},
		},
		{
			name:      "Babbage",
			cborHex:   babbageTxCborHex,
			txType:    ledger.TxTypeBabbage,
			checkKeys: []string{"Body", "WitnessSet", "TxIsValid"},
		},
		{
			name:      "Conway",
			cborHex:   conwayTxCborHex,
			txType:    ledger.TxTypeConway,
			checkKeys: []string{"Body", "WitnessSet", "TxIsValid"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cborData, err := hex.DecodeString(tc.cborHex)
			require.NoError(t, err)

			tx, err := ledger.NewTransactionFromCbor(tc.txType, cborData)
			require.NoError(t, err)

			jsonData, err := json.Marshal(tx)
			require.NoError(t, err, "json.Marshal should not fail for %s transaction", tc.name)
			assert.NotEmpty(t, jsonData)

			// Verify the JSON is valid and contains expected top-level keys
			var parsed map[string]json.RawMessage
			err = json.Unmarshal(jsonData, &parsed)
			require.NoError(t, err, "JSON output should be valid for %s transaction", tc.name)

			for _, key := range tc.checkKeys {
				assert.Contains(t, parsed, key, "%s transaction JSON should contain %q key", tc.name, key)
			}
		})
	}
}
