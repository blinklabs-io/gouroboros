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
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestDijkstraLeiosW30PoolKeyGoldenTransaction(t *testing.T) {
	// Golden transaction from cardano-ledger PR #5940 at commit
	// e1bdd8b493b094ce8bd59100d1aa08b492e8ab92:
	// eras/dijkstra/impl/golden/tx.cbor
	hexData, err := os.ReadFile(
		"testdata/cardano_ledger_dijkstra_w30_tx.hex",
	)
	require.NoError(t, err)
	txCbor, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err)

	tx, err := NewDijkstraTransactionFromCbor(txCbor)
	require.NoError(t, err)

	var poolRegistration *common.PoolRegistrationCertificate
	for _, cert := range tx.Certificates() {
		pool, ok := cert.(*common.PoolRegistrationCertificate)
		if ok && pool.LeiosKey != nil {
			poolRegistration = pool
			break
		}
	}
	require.NotNil(t, poolRegistration)
	require.Len(
		t,
		poolRegistration.LeiosKey.PublicKey,
		common.LeiosBlsPublicKeySize,
	)
	require.Len(
		t,
		poolRegistration.LeiosKey.PossessionProof,
		common.LeiosBlsPossessionProofSize,
	)

	encoded, err := cbor.Encode(tx)
	require.NoError(t, err)
	require.True(t, bytes.Equal(txCbor, encoded))
}
