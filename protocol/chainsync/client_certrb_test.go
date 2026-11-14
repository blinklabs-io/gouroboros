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

package chainsync_test

import (
	"net"
	"sync"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

// newTestInlinedCertRb builds a CertRB as LocalChainSync sends it: the
// header's block_body_hash covers the on-chain body (only the certificate),
// and the body also carries the certified EB's transactions.
func newTestInlinedCertRb(
	t *testing.T,
	slot uint64,
	txs []dijkstra.DijkstraTransaction,
) []byte {
	t.Helper()
	cert := &dijkstra.DijkstraLeiosCertificate{
		Signers:             []byte{0x01},
		AggregatedSignature: make([]byte, common.LeiosBlsSignatureSize),
	}
	onChain := dijkstra.DijkstraBlockBody{LeiosCertificate: cert}
	block := dijkstra.DijkstraBlock{
		BlockHeader: &dijkstra.DijkstraBlockHeader{
			BabbageBlockHeader: babbage.BabbageBlockHeader{
				Body: babbage.BabbageBlockHeaderBody{
					BlockNumber:   12001,
					Slot:          slot,
					BlockBodyHash: onChain.Hash(),
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
			Transactions:     txs,
			LeiosCertificate: cert,
		},
	}
	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)
	return blockCbor
}

// oneTestTx is a minimal endorser block transaction.
func oneTestTx() []dijkstra.DijkstraTransaction {
	return []dijkstra.DijkstraTransaction{
		{Body: dijkstra.DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
	}
}

// largeTestTxs returns count transactions of about txSize bytes each, padded
// with a Plutus V1 script witness.
func largeTestTxs(count, txSize int) []dijkstra.DijkstraTransaction {
	txs := make([]dijkstra.DijkstraTransaction, count)
	for i := range txs {
		script := make(common.PlutusV1Script, txSize)
		script[0] = byte(i)
		script[1] = byte(i >> 8)
		txs[i] = dijkstra.DijkstraTransaction{
			Body: dijkstra.DijkstraTransactionBody{TxFee: uint64(i + 1)},
			WitnessSet: dijkstra.DijkstraTransactionWitnessSet{
				WsPlutusV1Scripts: cbor.NewSetType(
					[]common.PlutusV1Script{script},
					false,
				),
			},
			TxIsValid: true,
		}
	}
	return txs
}

// certRbConversation scripts a node-to-client sync: find the intersection,
// roll back to it, then roll forward with rollForwardMsg.
func certRbConversation(
	intersect pcommon.Point,
	tip chainsync.Tip,
	rollForwardMsg protocol.Message,
	tail ...ouroboros_mock.ConversationEntry,
) []ouroboros_mock.ConversationEntry {
	conversation := append(
		append(
			[]ouroboros_mock.ConversationEntry{},
			conversationHandshakeFindIntersect...),
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgIntersectFound(intersect, tip),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeRequestNext,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages: []protocol.Message{
				chainsync.NewMsgRollBackward(intersect, tip),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeRequestNext,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: chainsync.ProtocolIdNtC,
			IsResponse: true,
			Messages:   []protocol.Message{rollForwardMsg},
		},
	)
	return append(conversation, tail...)
}

func testCertRbIntersectAndTip() (pcommon.Point, chainsync.Tip) {
	return pcommon.NewPoint(20001, testPointHash(0x12)),
		chainsync.Tip{
			BlockNumber: 12345,
			Point:       pcommon.NewPoint(23456, testPointHash(0x01)),
		}
}

// With block validation on, the node-to-client client still decodes a CertRB
// that carries EB transactions. The body hash is checked without them.
func TestClientDecodesInlinedCertRb(t *testing.T) {
	intersect, tip := testCertRbIntersectAndTip()
	blockCbor := newTestInlinedCertRb(t, 20002, oneTestTx())
	rollForwardMsg, err := chainsync.NewMsgRollForwardNtC(
		ledger.BlockTypeDijkstra,
		blockCbor,
		tip,
	)
	require.NoError(t, err)
	runTest(
		t,
		certRbConversation(intersect, tip, rollForwardMsg),
		func(t *testing.T, oConn *ouroboros.Connection) {
			start, _, err := oConn.ChainSync().Client.GetAvailableBlockRange(
				[]pcommon.Point{intersect},
			)
			require.NoError(t, err)
			require.Equal(t, uint64(20002), start.Slot)
		},
		ouroboros.WithChainSyncConfig(
			chainsync.Config{SkipBlockValidation: false},
		),
	)
}

// syncInlinedCertRb sends a CertRB carrying txs, with block validation on,
// and returns the block that RollForwardFunc receives.
func syncInlinedCertRb(
	t *testing.T,
	txs []dijkstra.DijkstraTransaction,
) (*dijkstra.DijkstraBlock, []byte) {
	t.Helper()
	intersect, tip := testCertRbIntersectAndTip()
	blockCbor := newTestInlinedCertRb(t, 20002, txs)
	rollForwardMsg, err := chainsync.NewMsgRollForwardNtC(
		ledger.BlockTypeDijkstra,
		blockCbor,
		tip,
	)
	require.NoError(t, err)
	conversation := certRbConversation(
		intersect,
		tip,
		rollForwardMsg,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  chainsync.ProtocolIdNtC,
			MessageType: chainsync.MessageTypeDone,
		},
	)
	type rolledForward struct {
		blockType uint
		block     any
	}
	received := make(chan rolledForward, 1)
	var got rolledForward
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			err := oConn.ChainSync().Client.Sync([]pcommon.Point{intersect})
			require.NoError(t, err)
			select {
			case got = <-received:
			case <-time.After(10 * time.Second):
				t.Fatal("timed out waiting for RollForwardFunc")
			}
			require.NoError(t, oConn.ChainSync().Client.Stop())
		},
		ouroboros.WithChainSyncConfig(chainsync.Config{
			SkipBlockValidation: false,
			// One request in flight, so the client's messages match the
			// scripted conversation.
			PipelineLimit: 1,
			RollBackwardFunc: func(
				chainsync.CallbackContext,
				pcommon.Point,
				chainsync.Tip,
			) error {
				return nil
			},
			RollForwardFunc: func(
				_ chainsync.CallbackContext,
				blockType uint,
				block any,
				_ chainsync.Tip,
			) error {
				received <- rolledForward{blockType: blockType, block: block}
				return chainsync.ErrStopSyncProcess
			},
		}),
	)
	require.Equal(t, uint(ledger.BlockTypeDijkstra), got.blockType)
	block, ok := got.block.(*dijkstra.DijkstraBlock)
	require.True(t, ok, "expected *dijkstra.DijkstraBlock, got %T", got.block)
	return block, blockCbor
}

// RollForwardFunc, the path most node-to-client consumers use, receives the
// CertRB with both its certificate and the inlined transactions.
func TestRollForwardFuncReceivesInlinedCertRb(t *testing.T) {
	block, blockCbor := syncInlinedCertRb(t, oneTestTx())
	require.Len(t, block.BlockBody.Transactions, 1)
	require.NotNil(t, block.BlockBody.LeiosCertificate)
	certified, present := block.BlockHeader.LeiosCertified()
	require.True(t, present)
	require.True(t, certified)
	require.Equal(t, blockCbor, block.Cbor())
}

// syncCertRbOverPipe runs a real node-to-client server and client over a
// pipe, so large messages are split into parts.
// The server first rolls back to the intersection, then sends blockCbor.
// With block validation on, it returns the block RollForwardFunc gets.
func syncCertRbOverPipe(t *testing.T, blockCbor []byte) *dijkstra.DijkstraBlock {
	t.Helper()
	const timeout = 30 * time.Second
	intersect, tip := testCertRbIntersectAndTip()
	errs := make(chan error, 8)
	var requests int
	var requestsMu sync.Mutex
	serverCfg := chainsync.NewConfig(
		chainsync.WithFindIntersectFunc(
			func(
				chainsync.CallbackContext,
				[]pcommon.Point,
			) (pcommon.Point, chainsync.Tip, error) {
				return intersect, tip, nil
			},
		),
		chainsync.WithRequestNextFunc(func(ctx chainsync.CallbackContext) error {
			requestsMu.Lock()
			requests++
			n := requests
			requestsMu.Unlock()
			switch n {
			case 1:
				return ctx.Server.RollBackward(intersect, tip)
			case 2:
				return ctx.Server.RollForward(
					ledger.BlockTypeDijkstra,
					blockCbor,
					tip,
				)
			default:
				return ctx.Server.AwaitReply()
			}
		}),
	)
	received := make(chan any, 1)
	clientCfg := chainsync.NewConfig(
		chainsync.WithPipelineLimit(1),
		chainsync.WithRollBackwardFunc(
			func(
				chainsync.CallbackContext,
				pcommon.Point,
				chainsync.Tip,
			) error {
				return nil
			},
		),
		chainsync.WithRollForwardFunc(
			func(
				_ chainsync.CallbackContext,
				_ uint,
				block any,
				_ chainsync.Tip,
			) error {
				received <- block
				return chainsync.ErrStopSyncProcess
			},
		),
	)
	clientPipe, serverPipe := net.Pipe()
	t.Cleanup(func() {
		_ = clientPipe.Close()
		_ = serverPipe.Close()
	})
	serverCh := make(chan *ouroboros.Connection, 1)
	go func() {
		conn, err := ouroboros.NewConnection(
			ouroboros.WithConnection(serverPipe),
			ouroboros.WithServer(true),
			ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
			ouroboros.WithChainSyncConfig(serverCfg),
			ouroboros.WithErrorChan(errs),
		)
		if err != nil {
			errs <- err
			close(serverCh)
			return
		}
		serverCh <- conn
	}()
	client, err := ouroboros.NewConnection(
		ouroboros.WithConnection(clientPipe),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithChainSyncConfig(clientCfg),
		ouroboros.WithErrorChan(errs),
	)
	require.NoError(t, err)
	var server *ouroboros.Connection
	select {
	case conn, ok := <-serverCh:
		if !ok {
			t.Fatalf("server connection failed: %v", <-errs)
		}
		server = conn
	case <-time.After(timeout):
		t.Fatal("server connection did not finish its handshake")
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	require.NoError(t, client.ChainSync().Client.Sync([]pcommon.Point{intersect}))
	var got any
	select {
	case got = <-received:
	case err := <-errs:
		t.Fatalf("connection failed: %v", err)
	case <-time.After(timeout):
		t.Fatal("timed out waiting for RollForwardFunc")
	}
	block, ok := got.(*dijkstra.DijkstraBlock)
	require.True(t, ok, "expected *dijkstra.DijkstraBlock, got %T", got)
	return block
}

// A CertRB with a full EB is over 8 MiB. The node-to-client receive budget
// is 8 MiB, but it limits unhandled bytes, not the size of one message, so
// the client must still receive and decode the block.
func TestClientReceivesCertRbLargerThanReceiveBudget(t *testing.T) {
	const (
		txSize  = 16 * 1024
		txCount = 800 // about 12.5 MiB of transactions
	)
	blockCbor := newTestInlinedCertRb(
		t,
		20002,
		largeTestTxs(txCount, txSize),
	)
	require.Greater(t, len(blockCbor), chainsync.PendingReceiveBytesNtC)
	block := syncCertRbOverPipe(t, blockCbor)
	require.Len(t, block.BlockBody.Transactions, txCount)
	require.NotNil(t, block.BlockBody.LeiosCertificate)
	require.Equal(t, blockCbor, block.Cbor())
}
