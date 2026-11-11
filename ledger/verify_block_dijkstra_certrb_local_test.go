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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

// signedInlinedCertRb builds a signed CertRB as LocalChainSync sends it: the
// header signs the body with onChainCert alone, and the body carries
// sentCert and an EB transaction.
func signedInlinedCertRb(
	t *testing.T,
	onChainCert, sentCert *dijkstra.DijkstraLeiosCertificate,
) ledger.Block {
	t.Helper()
	onChainCbor, err := cbor.Encode(
		dijkstra.DijkstraBlockBody{LeiosCertificate: onChainCert},
	)
	require.NoError(t, err)
	header := signedDijkstraTestHeader(
		t,
		func(h *dijkstra.DijkstraBlockHeader) {
			h.Body.BlockBodySize = uint64(len(onChainCbor))
			h.Body.BlockBodyHash = common.Blake2b256Hash(onChainCbor)
			h.LeiosHeaderExtension = leiosHeaderExtension(t, true, nil)
		},
	)
	blockCbor, err := cbor.Encode(&dijkstra.DijkstraBlock{
		BlockHeader: header,
		BlockBody: dijkstra.DijkstraBlockBody{
			Transactions: []dijkstra.DijkstraTransaction{
				{Body: dijkstra.DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
			},
			LeiosCertificate: sentCert,
		},
	})
	require.NoError(t, err)
	block, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeDijkstra,
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	return block
}

// VerifyBlockLocal checks the body size and hash against the full body, so it
// rejects a CertRB with inlined EB transactions. VerifyBlockLocalNodeToClient
// checks them against the body without those transactions.
func TestVerifyBlockLocalInlinedCertRb(t *testing.T) {
	cert := leiosWiringCertificate()

	t.Run("rejected by VerifyBlockLocal", func(t *testing.T) {
		valid, _, _, _, err := ledger.VerifyBlockLocal(
			signedInlinedCertRb(t, cert, cert),
			blockLimitsTestEta0Hex,
			blockLimitsTestSlotsPerKesPeriod,
		)
		require.False(t, valid)
		require.ErrorIs(t, err, common.ErrBlockBodySizeMismatch)
	})

	t.Run("accepted by VerifyBlockLocalNodeToClient", func(t *testing.T) {
		valid, _, _, _, err := ledger.VerifyBlockLocalNodeToClient(
			signedInlinedCertRb(t, cert, cert),
			blockLimitsTestEta0Hex,
			blockLimitsTestSlotsPerKesPeriod,
		)
		require.NoError(t, err)
		require.True(t, valid)
	})

	// The certificate is still covered by the header's body hash. Both
	// certificates have the same size, so the size check passes and the
	// hash check fails.
	t.Run("tampered certificate rejected", func(t *testing.T) {
		tampered := leiosWiringCertificate()
		tampered.Signers = []byte{0x81}
		valid, _, _, _, err := ledger.VerifyBlockLocalNodeToClient(
			signedInlinedCertRb(t, cert, tampered),
			blockLimitsTestEta0Hex,
			blockLimitsTestSlotsPerKesPeriod,
		)
		require.False(t, valid)
		var vErr *common.ValidationError
		require.ErrorAs(t, err, &vErr)
		require.Equal(t, common.ValidationErrorTypeBodyHash, vErr.Type)
	})
}
