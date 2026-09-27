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
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

type replyValidationPeer struct {
	server    *Server
	conn      net.Conn
	outbound  chan struct{}
	errorChan chan error
}

func newReplyValidationPeer(t *testing.T) *replyValidationPeer {
	t.Helper()
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	m.Start()
	errorChan := make(chan error, 10)
	outbound := make(chan struct{}, 10)
	initialized := make(chan struct{})
	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  localConn.LocalAddr(),
			RemoteAddr: localConn.RemoteAddr(),
		},
		Muxer:     m,
		ErrorChan: errorChan,
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &Config{InitFunc: func(CallbackContext) error {
		close(initialized)
		return nil
	}})
	server.Start()
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		for {
			var header [8]byte
			if _, err := io.ReadFull(peerConn, header[:]); err != nil {
				return
			}
			payloadLen := binary.BigEndian.Uint16(header[6:8])
			if _, err := io.CopyN(io.Discard, peerConn, int64(payloadLen)); err != nil {
				return
			}
			protocolId := binary.BigEndian.Uint16(header[4:6]) & 0x7fff
			if protocolId == ProtocolId {
				select {
				case outbound <- struct{}{}:
				default:
				}
			}
		}
	}()
	t.Cleanup(func() {
		proto := server.ProtocolInstance()
		proto.Stop()
		select {
		case <-proto.DoneChan():
		case <-time.After(5 * time.Second):
			t.Error("TxSubmission server did not stop")
		}
		m.Stop()
		_ = peerConn.Close()
		select {
		case <-peerDone:
		case <-time.After(5 * time.Second):
			t.Error("TxSubmission peer drain did not stop")
		}
	})
	peer := &replyValidationPeer{
		server:    server,
		conn:      peerConn,
		outbound:  outbound,
		errorChan: errorChan,
	}
	peer.send(t, NewMsgInit())
	select {
	case <-initialized:
	case <-time.After(time.Second):
		t.Fatal("TxSubmission server did not receive Init")
	}
	return peer
}

func (p *replyValidationPeer) send(t *testing.T, msg protocol.Message) {
	t.Helper()
	sendAsSegments(t, p.conn, msg, false)
}

func (p *replyValidationPeer) waitForRequest(t *testing.T) {
	t.Helper()
	select {
	case <-p.outbound:
	case err := <-p.errorChan:
		t.Fatalf("TxSubmission server failed before sending its request: %v", err)
	case <-time.After(time.Second):
		t.Fatal("TxSubmission server did not send its request")
	}
}

func (p *replyValidationPeer) announceTxIds(
	t *testing.T,
	txIds []TxIdAndSize,
) {
	t.Helper()
	type result struct {
		txIds []TxIdAndSize
		err   error
	}
	resultChan := make(chan result, 1)
	go func() {
		got, err := p.server.RequestTxIds(false, len(txIds))
		resultChan <- result{txIds: got, err: err}
	}()
	p.waitForRequest(t)
	p.send(t, NewMsgReplyTxIds(txIds))
	select {
	case got := <-resultChan:
		require.NoError(t, got.err)
		require.Equal(t, txIds, got.txIds)
	case <-time.After(time.Second):
		t.Fatal("RequestTxIds did not return after its reply")
	}
}

func (p *replyValidationPeer) requestTxs(
	t *testing.T,
	txIds []TxId,
	reply []TxBody,
) ([]TxBody, error) {
	t.Helper()
	type result struct {
		txs []TxBody
		err error
	}
	resultChan := make(chan result, 1)
	go func() {
		txs, err := p.server.RequestTxs(txIds)
		resultChan <- result{txs: txs, err: err}
	}()
	p.waitForRequest(t)
	p.send(t, NewMsgReplyTxs(reply))
	select {
	case got := <-resultChan:
		return got.txs, got.err
	case <-time.After(time.Second):
		t.Fatal("RequestTxs did not return after its reply")
		return nil, nil
	}
}

func testShelleyTxBody(t *testing.T, fee uint64) (TxBody, TxId) {
	t.Helper()
	body := map[uint64]any{
		0: []any{},
		1: []any{},
		2: fee,
		3: uint64(1),
	}
	metadata := map[uint64]any{0: bytes.Repeat([]byte{0x01}, 64)}
	txCbor, err := cbor.Encode([]any{body, map[uint64]any{}, metadata})
	require.NoError(t, err)
	tx, err := ledger.NewTransactionFromCbor(ledger.TxTypeShelley, txCbor)
	require.NoError(t, err)
	txId := TxId{EraId: uint16(ledger.TxTypeShelley)}
	hash := tx.Id()
	copy(txId.TxId[:], hash[:])
	return TxBody{EraId: txId.EraId, TxBody: txCbor}, txId
}

func txIdAndSize(txBody TxBody, txId TxId) TxIdAndSize {
	return TxIdAndSize{TxId: txId, Size: uint32(len(txBody.TxBody))}
}

func TestServerRequestTxsMatchesAndOrdersBodies(t *testing.T) {
	peer := newReplyValidationPeer(t)
	bodyA, idA := testShelleyTxBody(t, 1)
	bodyB, idB := testShelleyTxBody(t, 2)
	bodyC, idC := testShelleyTxBody(t, 3)
	peer.announceTxIds(t, []TxIdAndSize{
		txIdAndSize(bodyA, idA),
		txIdAndSize(bodyB, idB),
		txIdAndSize(bodyC, idC),
	})

	got, err := peer.requestTxs(t, []TxId{idA, idB, idC}, []TxBody{bodyC, bodyA})
	require.NoError(t, err)
	require.Equal(t, []TxBody{bodyA, bodyC}, got)
}

func TestServerRequestTxsRejectsUnrequestedBody(t *testing.T) {
	peer := newReplyValidationPeer(t)
	bodyA, idA := testShelleyTxBody(t, 1)
	bodyB, _ := testShelleyTxBody(t, 2)
	peer.announceTxIds(t, []TxIdAndSize{txIdAndSize(bodyA, idA)})

	got, err := peer.requestTxs(t, []TxId{idA}, []TxBody{bodyB})
	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
	require.Nil(t, got)
}

func TestServerRequestTxsRejectsUnannouncedTxId(t *testing.T) {
	peer := newReplyValidationPeer(t)
	_, err := peer.server.RequestTxs([]TxId{testTxId(1, 0x01)})
	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
}

func TestServerRequestTxsRejectsDuplicateBodies(t *testing.T) {
	peer := newReplyValidationPeer(t)
	bodyA, idA := testShelleyTxBody(t, 1)
	bodyB, idB := testShelleyTxBody(t, 2)
	peer.announceTxIds(t, []TxIdAndSize{txIdAndSize(bodyA, idA), txIdAndSize(bodyB, idB)})

	got, err := peer.requestTxs(t, []TxId{idA, idB}, []TxBody{bodyA, bodyA})
	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
	require.Nil(t, got)
}

func TestServerRequestTxsAcceptsAdvertisedSizeDifferenceOf32(t *testing.T) {
	for _, delta := range []int{-32, 32} {
		t.Run(fmt.Sprintf("delta_%+d", delta), func(t *testing.T) {
			peer := newReplyValidationPeer(t)
			body, txId := testShelleyTxBody(t, 1)
			size := uint32(len(body.TxBody) + delta)
			peer.announceTxIds(t, []TxIdAndSize{{TxId: txId, Size: size}})
			got, err := peer.requestTxs(t, []TxId{txId}, []TxBody{body})
			require.NoError(t, err)
			require.Equal(t, []TxBody{body}, got)
		})
	}
}

func TestServerRequestTxsRejectsAdvertisedSizeDifferenceOf33(t *testing.T) {
	for _, delta := range []int{-33, 33} {
		t.Run(fmt.Sprintf("delta_%+d", delta), func(t *testing.T) {
			peer := newReplyValidationPeer(t)
			body, txId := testShelleyTxBody(t, 1)
			size := uint32(len(body.TxBody) + delta)
			peer.announceTxIds(t, []TxIdAndSize{{TxId: txId, Size: size}})
			got, err := peer.requestTxs(t, []TxId{txId}, []TxBody{body})
			require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
			require.Nil(t, got)
		})
	}
}

func TestServerRequestTxIdsRejectsEmptyBlockingReply(t *testing.T) {
	peer := newReplyValidationPeer(t)
	resultChan := make(chan error, 1)
	go func() {
		_, err := peer.server.RequestTxIds(true, 1)
		resultChan <- err
	}()
	peer.waitForRequest(t)
	peer.send(t, NewMsgReplyTxIds(nil))
	select {
	case err := <-resultChan:
		require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
	case <-time.After(time.Second):
		t.Fatal("RequestTxIds did not reject an empty blocking reply")
	}
}

func TestServerShutdownClearsPendingTxIdRequest(t *testing.T) {
	peer := newReplyValidationPeer(t)
	resultChan := make(chan error, 1)
	go func() {
		_, err := peer.server.RequestTxIds(false, 1)
		resultChan <- err
	}()
	peer.waitForRequest(t)
	peer.server.ProtocolInstance().Stop()
	select {
	case err := <-resultChan:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		t.Fatal("RequestTxIds did not stop waiting after protocol shutdown")
	}
	peer.server.stateMu.Lock()
	require.Nil(t, peer.server.pendingTxIds)
	require.True(t, peer.server.stopping)
	peer.server.stateMu.Unlock()
}

func TestClientRejectsRequestForUnannouncedTxId(t *testing.T) {
	called := false
	callbackErr := errors.New("RequestTxsFunc should not be called")
	client := NewClient(protocol.ProtocolOptions{Mode: protocol.ProtocolModeNodeToNode}, &Config{
		RequestTxsFunc: func(CallbackContext, []TxId) ([]TxBody, error) {
			called = true
			return nil, callbackErr
		},
	})
	err := client.handleRequestTxs(NewMsgRequestTxs([]TxId{testTxId(1, 0x01)}))
	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
	require.False(t, called)
}

func TestClientRejectsUnrequestedCallbackBody(t *testing.T) {
	body, _ := testShelleyTxBody(t, 1)
	requested := testTxId(uint16(ledger.TxTypeShelley), 0x01)
	client := NewClient(protocol.ProtocolOptions{Mode: protocol.ProtocolModeNodeToNode}, &Config{
		RequestTxsFunc: func(CallbackContext, []TxId) ([]TxBody, error) {
			return []TxBody{body}, nil
		},
	})
	client.unackedMu.Lock()
	client.unackedTxIds = []TxIdAndSize{{
		TxId: requested,
		Size: uint32(len(body.TxBody)),
	}}
	client.unackedMu.Unlock()

	err := client.handleRequestTxs(NewMsgRequestTxs([]TxId{requested}))
	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
}

func TestClientRejectsCallbackBodySizeMismatch(t *testing.T) {
	body, txId := testShelleyTxBody(t, 1)
	client := NewClient(protocol.ProtocolOptions{Mode: protocol.ProtocolModeNodeToNode}, &Config{
		RequestTxsFunc: func(CallbackContext, []TxId) ([]TxBody, error) {
			return []TxBody{body}, nil
		},
	})
	client.unackedMu.Lock()
	client.unackedTxIds = []TxIdAndSize{{
		TxId: txId,
		Size: uint32(len(body.TxBody) + 33),
	}}
	client.unackedMu.Unlock()

	err := client.handleRequestTxs(NewMsgRequestTxs([]TxId{txId}))
	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
}
