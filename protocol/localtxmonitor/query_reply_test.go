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

package localtxmonitor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testQuery is one client query and the reply that answers it.
type testQuery struct {
	name      string
	queryType uint8
	reply     protocol.Message
	call      func(*Client) error
}

func testQueries() []testQuery {
	txId := bytes.Repeat([]byte{0xab}, 32)
	return []testQuery{
		{
			name:      "HasTx",
			queryType: MessageTypeHasTx,
			reply:     NewMsgReplyHasTx(true),
			call: func(c *Client) error {
				_, err := c.HasTxForEra(6, txId)
				return err
			},
		},
		{
			name:      "NextTx",
			queryType: MessageTypeNextTx,
			reply:     NewMsgReplyNextTx(0, nil),
			call: func(c *Client) error {
				_, err := c.NextTx()
				return err
			},
		},
		{
			name:      "GetSizes",
			queryType: MessageTypeGetSizes,
			reply:     NewMsgReplyGetSizes(1, 2, 3),
			call: func(c *Client) error {
				_, _, _, err := c.GetSizes()
				return err
			},
		},
	}
}

// TestMatchReplyAcceptsOnlyQueryReply checks every query/reply pair against
// the Busy matchers: only the reply to the recorded query is accepted, and a
// missing or foreign state context accepts nothing.
func TestMatchReplyAcceptsOnlyQueryReply(t *testing.T) {
	for _, query := range testQueries() {
		for _, reply := range testQueries() {
			ctx := &stateContext{}
			queryMsg := &protocol.MessageBase{MessageType: query.queryType}
			require.True(t, matchQuery(ctx, queryMsg))
			assert.Equal(
				t,
				query.name == reply.name,
				matchReply(ctx, reply.reply),
				"query %s, reply %s",
				query.name,
				reply.name,
			)
		}
	}
	reply := NewMsgReplyHasTx(true)
	assert.False(t, matchReply(&stateContext{}, reply))
	assert.False(t, matchReply(nil, reply))
	assert.False(t, matchQuery(nil, NewMsgHasTxForEra(6, nil)))
}

// TestStateMapMatchesQueriesAndReplies checks that every query leaving
// Acquired records itself and every reply leaving Busy is matched against
// the recorded query.
func TestStateMapMatchesQueriesAndReplies(t *testing.T) {
	for _, transition := range StateMap[stateAcquired].Transitions {
		if transition.NewState == stateBusy {
			assert.NotNil(
				t,
				transition.MatchFunc,
				"query %d",
				transition.MsgType,
			)
		}
	}
	busy := StateMap[stateBusy].Transitions
	require.Len(t, busy, len(queryReplyTypes))
	for _, transition := range busy {
		assert.NotNil(t, transition.MatchFunc, "reply %d", transition.MsgType)
	}
}

func readQueryTestSegment(t *testing.T, conn net.Conn) *muxer.Segment {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	header := muxer.SegmentHeader{}
	require.NoError(t, binary.Read(conn, binary.BigEndian, &header))
	payload := make([]byte, header.PayloadLength)
	_, err := io.ReadFull(conn, payload)
	require.NoError(t, err)
	return &muxer.Segment{SegmentHeader: header, Payload: payload}
}

func writeQueryTestMessage(t *testing.T, conn net.Conn, msg protocol.Message) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	segment := muxer.NewSegment(ProtocolId, data, true)
	require.NotNil(t, segment)
	var buf bytes.Buffer
	require.NoError(
		t,
		binary.Write(&buf, binary.BigEndian, segment.SegmentHeader),
	)
	buf.Write(segment.Payload)
	_, err = conn.Write(buf.Bytes())
	require.NoError(t, err)
}

// requireQueryTestMessageType reads the next segment from the client and
// checks it carries a single message of the given type.
func requireQueryTestMessageType(t *testing.T, conn net.Conn, want uint8) {
	t.Helper()
	var msg []cbor.RawMessage
	_, err := cbor.Decode(readQueryTestSegment(t, conn).Payload, &msg)
	require.NoError(t, err)
	require.NotEmpty(t, msg)
	var msgType uint8
	_, err = cbor.Decode(msg[0], &msgType)
	require.NoError(t, err)
	require.Equal(t, want, msgType)
}

// startAcquiredTestClient starts a client on one end of a pipe and acquires
// a mempool snapshot, returning the client, the peer end and the client's
// error channel.
func startAcquiredTestClient(t *testing.T) (*Client, net.Conn, chan error) {
	t.Helper()
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})
	m := muxer.New(connA)
	t.Cleanup(m.Stop)
	errorChan := make(chan error, 10)
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Muxer:     m,
		ErrorChan: errorChan,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, nil)
	client.Start()
	m.Start()
	t.Cleanup(client.Protocol.Stop)

	acquired := make(chan error, 1)
	go func() { acquired <- client.Acquire() }()
	requireQueryTestMessageType(t, connB, MessageTypeAcquire)
	writeQueryTestMessage(t, connB, NewMsgAcquired(1))
	select {
	case err := <-acquired:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Acquire did not complete")
	}
	return client, connB, errorChan
}

// TestClientRejectsMismatchedReply sends each wrong reply to each query and
// checks that the protocol fails without delivering it, and that shutdown
// releases the caller instead of leaving it or the receive loop blocked.
func TestClientRejectsMismatchedReply(t *testing.T) {
	for _, query := range testQueries() {
		for _, reply := range testQueries() {
			if query.name == reply.name {
				continue
			}
			t.Run(query.name+"/"+reply.name, func(t *testing.T) {
				client, conn, errorChan := startAcquiredTestClient(t)
				result := make(chan error, 1)
				go func() { result <- query.call(client) }()
				requireQueryTestMessageType(t, conn, query.queryType)
				writeQueryTestMessage(t, conn, reply.reply)

				select {
				case err := <-errorChan:
					require.ErrorContains(t, err, "not allowed")
				case <-time.After(5 * time.Second):
					t.Fatal("mismatched reply was not refused")
				}
				select {
				case err := <-result:
					require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
				case <-time.After(5 * time.Second):
					t.Fatal("caller was not released")
				}
			})
		}
	}
}

// TestClientAcceptsMatchingReply checks that each query still returns the
// reply that answers it.
func TestClientAcceptsMatchingReply(t *testing.T) {
	for _, query := range testQueries() {
		t.Run(query.name, func(t *testing.T) {
			client, conn, errorChan := startAcquiredTestClient(t)
			result := make(chan error, 1)
			go func() { result <- query.call(client) }()
			requireQueryTestMessageType(t, conn, query.queryType)
			writeQueryTestMessage(t, conn, query.reply)
			select {
			case err := <-result:
				require.NoError(t, err)
			case err := <-errorChan:
				t.Fatalf("protocol failed: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("query did not complete")
			}
		})
	}
}

// TestClientStopReleasesCaller checks that stopping the protocol while a
// query waits for its reply returns an error to the caller.
func TestClientStopReleasesCaller(t *testing.T) {
	client, conn, _ := startAcquiredTestClient(t)
	result := make(chan error, 1)
	go func() {
		_, err := client.HasTxForEra(6, bytes.Repeat([]byte{0xab}, 32))
		result <- err
	}()
	requireQueryTestMessageType(t, conn, MessageTypeHasTx)
	client.Protocol.Stop()
	select {
	case err := <-result:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(5 * time.Second):
		t.Fatal("caller was not released")
	}
}

// TestReplyDeliveryStopsAtShutdown checks that a reply handler waiting for a
// caller that never reads is released by shutdown, so the receive loop is
// not left blocked.
func TestReplyDeliveryStopsAtShutdown(t *testing.T) {
	client, _, _ := startAcquiredTestClient(t)
	delivered := make(chan struct{})
	go func() {
		defer close(delivered)
		client.handleReplyHasTx(NewMsgReplyHasTx(true))
	}()
	select {
	case <-delivered:
		t.Fatal("reply was delivered with no caller waiting")
	case <-time.After(100 * time.Millisecond):
	}
	client.Protocol.Stop()
	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		t.Fatal("reply handler was not released by shutdown")
	}
}

// TestHasTxRequiresTxIdSize checks that a HasTx transaction ID must be
// exactly 32 bytes, both when decoding a received message and before the
// client sends one.
func TestHasTxRequiresTxIdSize(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "empty", size: 0, wantErr: true},
		{name: "short", size: 31, wantErr: true},
		{name: "exact", size: 32},
		{name: "long", size: 33, wantErr: true},
		{name: "oversized", size: 1 << 20, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			txId := bytes.Repeat([]byte{0xab}, tc.size)
			data, err := cbor.Encode(NewMsgHasTxForEra(6, txId))
			require.NoError(t, err)
			msg, err := NewMsgFromCbor(MessageTypeHasTx, data)
			if tc.wantErr {
				require.ErrorContains(t, err, "transaction ID must be 32 bytes")
				assert.ErrorContains(t, validateTxId(txId), "must be 32 bytes")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, txId, msg.(*MsgHasTx).TxId)
		})
	}

	// The client is never started, so a HasTx that reached the network
	// would block rather than fail.
	client := NewClient(protocol.ProtocolOptions{}, nil)
	result := make(chan error, 1)
	go func() {
		_, err := client.HasTxForEra(6, bytes.Repeat([]byte{0xab}, 31))
		result <- err
	}()
	select {
	case err := <-result:
		require.ErrorContains(t, err, "transaction ID must be 32 bytes")
	case <-time.After(5 * time.Second):
		t.Fatal("HasTx did not reject the transaction ID before sending")
	}
}

// TestClientHasTxRequiresEra checks that the deprecated untagged HasTx
// returns an error without sending anything to the node.
func TestClientHasTxRequiresEra(t *testing.T) {
	client, conn, _ := startAcquiredTestClient(t)
	result := make(chan error, 1)
	go func() {
		_, err := client.HasTx(bytes.Repeat([]byte{0xab}, 32))
		result <- err
	}()
	select {
	case err := <-result:
		require.ErrorIs(t, err, errHasTxEraRequired)
	case <-time.After(5 * time.Second):
		t.Fatal("HasTx did not return")
	}
	require.NoError(
		t,
		conn.SetReadDeadline(time.Now().Add(100*time.Millisecond)),
	)
	n, err := conn.Read(make([]byte, 1))
	require.Zero(t, n, "HasTx sent a request without an era")
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
}

// TestClientHasTxForEraSendsEra checks that the era passed to HasTxForEra is
// the one encoded in the request the node receives.
func TestClientHasTxForEraSendsEra(t *testing.T) {
	txId := bytes.Repeat([]byte{0xab}, 32)
	for _, eraId := range []uint8{1, 6} {
		t.Run(fmt.Sprintf("era%d", eraId), func(t *testing.T) {
			client, conn, errorChan := startAcquiredTestClient(t)
			result := make(chan error, 1)
			go func() {
				hasTx, err := client.HasTxForEra(eraId, txId)
				if err == nil && !hasTx {
					err = errors.New("HasTxForEra returned false")
				}
				result <- err
			}()
			var msg MsgHasTx
			_, err := cbor.Decode(readQueryTestSegment(t, conn).Payload, &msg)
			require.NoError(t, err)
			require.Equal(t, uint8(MessageTypeHasTx), msg.MessageType)
			require.Equal(t, eraId, msg.EraId)
			require.Equal(t, txId, msg.TxId)
			writeQueryTestMessage(t, conn, NewMsgReplyHasTx(true))
			select {
			case err := <-result:
				require.NoError(t, err)
			case err := <-errorChan:
				t.Fatalf("protocol failed: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("query did not complete")
			}
		})
	}
}
