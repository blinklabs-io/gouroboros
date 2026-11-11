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

package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

// inlinedCertRbCbor builds a CertRB as LocalChainSync sends it: the header
// signs the body with the certificate alone, and the body also carries an
// EB transaction.
func inlinedCertRbCbor(t *testing.T) []byte {
	t.Helper()
	cert := &dijkstra.DijkstraLeiosCertificate{
		Signers:             []byte{0x80},
		AggregatedSignature: make([]byte, common.LeiosBlsSignatureSize),
	}
	onChainCbor, err := cbor.Encode(
		dijkstra.DijkstraBlockBody{LeiosCertificate: cert},
	)
	require.NoError(t, err)
	blockCbor, err := cbor.Encode(&dijkstra.DijkstraBlock{
		BlockHeader: &dijkstra.DijkstraBlockHeader{
			BabbageBlockHeader: babbage.BabbageBlockHeader{
				Body: babbage.BabbageBlockHeaderBody{
					Slot:          1010,
					BlockBodySize: uint64(len(onChainCbor)),
					BlockBodyHash: common.Blake2b256Hash(onChainCbor),
					VrfKey:        make([]byte, 32),
					VrfResult: common.VrfResult{
						Output: make([]byte, 64),
						Proof:  make([]byte, 80),
					},
					OpCert: babbage.BabbageOpCert{
						HotVkey:   make([]byte, 32),
						Signature: make([]byte, 64),
					},
					ProtoVersion: babbage.BabbageProtoVersion{
						Major: dijkstra.MinProtocolVersionDijkstra,
					},
				},
				Signature: make([]byte, 448),
			},
			// leios_certified = true, leios_announcement = null
			LeiosHeaderExtension: []cbor.RawMessage{{0xf5}, {0xf6}},
		},
		BlockBody: dijkstra.DijkstraBlockBody{
			Transactions: []dijkstra.DijkstraTransaction{
				{Body: dijkstra.DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
			},
			LeiosCertificate: cert,
		},
	})
	require.NoError(t, err)
	return blockCbor
}

// The decode stage accepts a CertRB with inlined EB transactions only for a
// block received over node-to-client.
func TestDecodeStageInlinedCertRb(t *testing.T) {
	blockCbor := inlinedCertRbCbor(t)
	stage := NewDecodeStage(false)

	item := NewBlockItem(
		uint(ledger.BlockTypeDijkstra),
		blockCbor,
		createTestTip(1010, 1),
		0,
	)
	err := stage.Process(t.Context(), item)
	var vErr *common.ValidationError
	require.ErrorAs(t, err, &vErr)
	require.Equal(t, common.ValidationErrorTypeBodyHash, vErr.Type)

	item = NewBlockItem(
		uint(ledger.BlockTypeDijkstra),
		blockCbor,
		createTestTip(1010, 1),
		0,
	)
	item.nodeToClient = true
	require.NoError(t, stage.Process(t.Context(), item))
	block, ok := item.Block().(*dijkstra.DijkstraBlock)
	require.True(t, ok)
	require.Len(t, block.BlockBody.Transactions, 1)
	require.NotNil(t, block.BlockBody.LeiosCertificate)
}

// SubmitNodeToClient marks the block as received over node-to-client, and
// Submit does not.
func TestBlockPipelineSubmitNodeToClient(t *testing.T) {
	blockCbor := inlinedCertRbCbor(t)
	p := NewBlockPipeline(WithDecodeWorkers(1), WithTrustedDecodeOnly())
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	require.NoError(t, p.Start(ctx))
	var drained sync.WaitGroup
	drained.Go(func() {
		for range p.Errors() {
		}
	})
	// Stop closes Errors, so it must run before the drain is awaited.
	defer drained.Wait()
	defer p.Stop()

	next := func() *BlockItem {
		t.Helper()
		select {
		case item := <-p.Results():
			return item
		case <-ctx.Done():
			t.Fatal("timed out waiting for a pipeline result")
			return nil
		}
	}

	tip := createTestTip(1010, 1)
	require.NoError(t, p.Submit(ctx, uint(ledger.BlockTypeDijkstra), blockCbor, tip))
	item := next()
	require.False(t, item.NodeToClient())
	require.Error(t, item.DecodeError())
	item.Release()

	require.NoError(t, p.SubmitNodeToClient(
		ctx,
		uint(ledger.BlockTypeDijkstra),
		blockCbor,
		tip,
	))
	item = next()
	require.True(t, item.NodeToClient())
	require.NoError(t, item.DecodeError())
	item.Release()
}
