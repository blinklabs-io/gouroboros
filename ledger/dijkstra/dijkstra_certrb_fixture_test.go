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
	"path/filepath"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// Real CertRBs from the Musashi testnet (network magic 164, ouroboros-leios
// prototype-2026w36), as stored on chain. Each file holds one block as hex,
// wrapped as [8, block].
//
// Source: dijkstra8.block and dijkstra9.block from txpipe/pallas test_data at
// commit daecef59a1979d89515524d9583cc28655af182c (Apache-2.0). That
// repository's test_data/dijkstra-fixtures.md describes how they were captured.
var musashiCertRbFixtures = []struct {
	file        string
	blockNumber uint64
	slot        uint64
	hash        string
}{
	{
		// The first Leios certificate, in a block that also announces an EB.
		file:        "musashi_certrb_17403.hex",
		blockNumber: 17403,
		slot:        371723,
		hash:        "33a48eee693522320891dd4d1da8ee33c288c854eb25337802dbc4c912571d07",
	},
	{
		// A Leios certificate without an announcement.
		file:        "musashi_certrb_17512.hex",
		blockNumber: 17512,
		slot:        374306,
		hash:        "112495349409ccaa7a57e810aa59b014337612438a3e07cd5ce1bb3126dfbbea",
	},
}

// loadMusashiCertRb returns the block CBOR of a CertRB fixture, without the
// [8, block] wrapper.
func loadMusashiCertRb(t *testing.T, file string) []byte {
	t.Helper()
	hexData, err := os.ReadFile(filepath.Join("testdata", file))
	require.NoError(t, err)
	wrapped, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err)
	var wrapper []cbor.RawMessage
	_, err = cbor.DecodeExact(wrapped, &wrapper)
	require.NoError(t, err)
	require.Len(t, wrapper, 2)
	var tag uint64
	_, err = cbor.Decode(wrapper[0], &tag)
	require.NoError(t, err)
	require.Equal(t, uint64(8), tag, "Dijkstra block wrapper tag")
	return wrapper[1]
}

// inlineTransactions puts txs into the body's transaction list of blockCbor,
// the way the node inlines an EB's transactions into a CertRB over
// node-to-client. Only the transaction list changes; the header and the other
// body fields keep their original bytes.
func inlineTransactions(
	t *testing.T,
	blockCbor []byte,
	txs []DijkstraTransaction,
) []byte {
	t.Helper()
	var top []cbor.RawMessage
	_, err := cbor.DecodeExact(blockCbor, &top)
	require.NoError(t, err)
	require.Len(t, top, 2)
	var bodyFields []cbor.RawMessage
	_, err = cbor.DecodeExact(top[1], &bodyFields)
	require.NoError(t, err)
	txsBody, err := (&DijkstraBlockBody{Transactions: txs}).MarshalCBOR()
	require.NoError(t, err)
	var txsFields []cbor.RawMessage
	_, err = cbor.DecodeExact(txsBody, &txsFields)
	require.NoError(t, err)
	bodyFields[0] = txsFields[0]
	inlinedBody, err := cbor.Encode(bodyFields)
	require.NoError(t, err)
	inlinedBlock, err := cbor.Encode([]cbor.RawMessage{top[0], inlinedBody})
	require.NoError(t, err)
	return inlinedBlock
}

// OnChainBlockBodyCbor rebuilds a CertRB's on-chain body by re-encoding it with
// an empty transaction list. This test checks, on real CertRBs from the node,
// that the rebuilt bytes match the stored body exactly. If they didn't, the
// header's body hash would not match.
func TestOnChainBlockBodyCborMatchesRealCertRbs(t *testing.T) {
	t.Parallel()
	for _, fixture := range musashiCertRbFixtures {
		t.Run(fixture.file, func(t *testing.T) {
			t.Parallel()
			blockCbor := loadMusashiCertRb(t, fixture.file)

			// The fixture is the on-chain form: it passes the default body
			// hash check and has a certificate but no transactions.
			block, err := NewDijkstraBlockFromCbor(blockCbor)
			require.NoError(t, err)
			require.Equal(t, fixture.blockNumber, block.BlockNumber())
			require.Equal(t, fixture.slot, block.SlotNumber())
			require.Equal(t, fixture.hash, block.Hash().String())
			certified, present := block.BlockHeader.LeiosCertified()
			require.True(t, present)
			require.True(t, certified)
			require.NotNil(t, block.BlockBody.LeiosCertificate)
			require.Empty(t, block.BlockBody.Transactions)

			var top []cbor.RawMessage
			_, err = cbor.DecodeExact(blockCbor, &top)
			require.NoError(t, err)
			onChain, err := block.OnChainBlockBodyCbor()
			require.NoError(t, err)
			require.Equal(t, []byte(top[1]), onChain)

			// The same CertRB with transactions inlined, as sent over
			// node-to-client.
			inlined := inlineTransactions(t, blockCbor, []DijkstraTransaction{
				{Body: DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
				{Body: DijkstraTransactionBody{TxFee: 2}, TxIsValid: true},
			})

			_, err = NewDijkstraBlockFromCbor(inlined)
			var vErr *common.ValidationError
			require.ErrorAs(t, err, &vErr)
			require.Equal(t, common.ValidationErrorTypeBodyHash, vErr.Type)

			decoded, err := NewDijkstraBlockFromCbor(
				inlined,
				common.VerifyConfig{AllowCertifiedBlockInlinedTransactions: true},
			)
			require.NoError(t, err)
			require.True(t, decoded.HasInlinedCertifiedTransactions())
			require.Len(t, decoded.BlockBody.Transactions, 2)
			require.Equal(t, block.Hash(), decoded.Hash())
			require.Equal(
				t,
				block.BlockBody.LeiosCertificate.Cbor(),
				decoded.BlockBody.LeiosCertificate.Cbor(),
			)
			decodedOnChain, err := decoded.OnChainBlockBodyCbor()
			require.NoError(t, err)
			require.Equal(t, []byte(top[1]), decodedOnChain)
		})
	}
}
