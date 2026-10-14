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

package txsubmission

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// fakeTx returns the smallest Conway transaction the strict decoder accepts,
// [body, witnesses, true, null] with an empty body of inputs, outputs and a
// fee of n, together with the id a peer derives for it from the body hash.
func fakeTx(n byte) (TxId, []byte) {
	body := []byte{0xa3, 0x00, 0x80, 0x01, 0x80, 0x02, n}
	tx := append([]byte{0x84}, body...)
	tx = append(tx, 0xa0, 0xf5, 0xf6)
	return TxId{EraId: 6, TxId: common.Blake2b256Hash(body)}, tx
}

// TestClientAcceptsPipelinedRequestsWhileReplying covers a server that, like
// the reference node's inbound side, pipelines MsgRequestTxIds and
// MsgRequestTxs behind a MsgRequestTxs the client is still answering. The
// client must queue them until it regains the state they apply to instead of
// failing the connection for a message sent without agency.
func TestClientAcceptsPipelinedRequestsWhileReplying(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(clientMuxer.Stop)
	t.Cleanup(serverMuxer.Stop)

	txId := func(n byte) TxId {
		id, _ := fakeTx(n)
		return id
	}
	txsByHash := make(map[[32]byte][]byte)
	for n := byte(3); n <= 7; n++ {
		id, tx := fakeTx(n)
		txsByHash[id.TxId] = tx
	}

	release := make(chan struct{})
	holding := make(chan struct{}, 1)
	idsHolding := make(chan struct{}, 1)
	releaseIds := make(chan struct{})
	var holdIds atomic.Bool
	var nextId byte = 2
	clientErrors := make(chan error, 1)
	client := NewClient(
		protocol.ProtocolOptions{
			Muxer:        clientMuxer,
			ErrorChan:    clientErrors,
			Mode:         protocol.ProtocolModeNodeToNode,
			ConnectionId: connection.ConnectionId{},
		},
		&Config{
			RequestTxIdsFunc: func(
				_ CallbackContext,
				_ bool,
				_ uint16,
				req uint16,
			) ([]TxIdAndSize, error) {
				if holdIds.CompareAndSwap(true, false) {
					idsHolding <- struct{}{}
					<-releaseIds
				}
				ids := make([]TxIdAndSize, 0, req)
				for range req {
					nextId++
					ids = append(ids, TxIdAndSize{TxId: txId(nextId), Size: 10})
				}
				return ids, nil
			},
			RequestTxsFunc: func(
				_ CallbackContext,
				ids []TxId,
			) ([]TxBody, error) {
				select {
				case holding <- struct{}{}:
					<-release
				default:
				}
				bodies := make([]TxBody, 0, len(ids))
				for _, id := range ids {
					bodies = append(bodies, TxBody{EraId: 6, TxBody: txsByHash[id.TxId]})
				}
				return bodies, nil
			},
		},
	)

	// The peer is a server that may pipeline while the client holds agency.
	peerStateMap := StateMap.Copy()
	for _, state := range []protocol.State{stateTxs, stateTxIdsNonblocking} {
		entry := peerStateMap[state]
		entry.AllowPipelinedSend = true
		entry.PipelinedMessageTypes = []uint8{
			MessageTypeRequestTxIds,
			MessageTypeRequestTxs,
		}
		peerStateMap[state] = entry
	}
	replies := make(chan uint, 16)
	peer := protocol.New(protocol.ProtocolConfig{
		Name:       "tx-submission-pipelining-peer",
		ProtocolId: ProtocolId,
		Muxer:      serverMuxer,
		ErrorChan:  make(chan error, 1),
		Mode:       protocol.ProtocolModeNodeToNode,
		Role:       protocol.ProtocolRoleServer,
		MessageHandlerFunc: func(msg protocol.Message) error {
			replies <- uint(msg.Type())
			return nil
		},
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            peerStateMap,
		InitialState:        stateInit,
	})

	peer.Start()
	t.Cleanup(peer.Stop)
	client.Start()
	t.Cleanup(func() { _ = client.Stop() })
	serverMuxer.Start()
	clientMuxer.Start()
	client.Init()

	expect := func(want uint) {
		t.Helper()
		select {
		case got := <-replies:
			require.Equal(t, want, got)
		case err := <-clientErrors:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for message type %d", want)
		}
	}

	require.NoError(t, peer.SendMessage(NewMsgRequestTxIds(false, 0, 2)))
	expect(MessageTypeInit)
	expect(MessageTypeReplyTxIds)

	// Announced ids are 3 and 4. Request the first and, before the client
	// replies, pipeline a further id request and a request for the second.
	require.NoError(t, peer.SendMessage(NewMsgRequestTxs([]TxId{txId(3)})))
	waitHolding(t, holding, clientErrors)
	require.NoError(t, peer.SendMessage(NewMsgRequestTxIds(false, 0, 1)))
	require.NoError(t, peer.SendMessage(NewMsgRequestTxs([]TxId{txId(4)})))

	// Nothing the client does while holding the first reply may fail it.
	select {
	case err := <-clientErrors:
		require.NoError(t, err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)

	expect(MessageTypeReplyTxs)
	expect(MessageTypeReplyTxIds)
	expect(MessageTypeReplyTxs)

	// Phase two holds the client in TxIdsNonblocking instead. Ids 3 and 4 are
	// acknowledged and id 5 stays outstanding, so the pipelined request for
	// it is still valid.
	holdIds.Store(true)
	require.NoError(t, peer.SendMessage(NewMsgRequestTxIds(false, 2, 1)))
	waitHolding(t, idsHolding, clientErrors)
	require.NoError(t, peer.SendMessage(NewMsgRequestTxIds(false, 0, 1)))
	require.NoError(t, peer.SendMessage(NewMsgRequestTxs([]TxId{txId(5)})))
	select {
	case err := <-clientErrors:
		require.NoError(t, err)
	case <-time.After(300 * time.Millisecond):
	}
	close(releaseIds)

	expect(MessageTypeReplyTxIds)
	expect(MessageTypeReplyTxIds)
	expect(MessageTypeReplyTxs)
}

// waitHolding waits until the client has entered a held callback, failing on
// a client protocol error or a timeout instead of blocking forever.
func waitHolding(t *testing.T, holding <-chan struct{}, errs <-chan error) {
	t.Helper()
	select {
	case <-holding:
	case err := <-errs:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the client to enter the callback")
	}
}
