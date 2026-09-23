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

package peersharing

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// maxSizePeerAddress returns the largest PeerAddress this protocol can encode:
// the IPv6 form with every uint32 word and the port at their maximum, which is
// 25 bytes.
func maxSizePeerAddress() PeerAddress {
	return PeerAddress{
		IP:   net.ParseIP("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"),
		Port: 0xffff,
	}
}

func maxSizePeerAddresses(count int) []PeerAddress {
	peers := make([]PeerAddress, count)
	for i := range peers {
		peers[i] = maxSizePeerAddress()
	}
	return peers
}

func smallPeerAddresses(count int) []PeerAddress {
	peers := make([]PeerAddress, count)
	for i := range peers {
		peers[i] = PeerAddress{IP: net.IPv4(10, 0, 0, byte(i)), Port: 3001}
	}
	return peers
}

func testPeerSharingClient(t *testing.T) (*Client, net.Conn, chan error) {
	t.Helper()
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	errs := make(chan error, 1)
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
				RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
			},
			Muxer:     m,
			ErrorChan: errs,
			Mode:      protocol.ProtocolModeNodeToNode,
		},
		nil,
	)
	client.Start()
	m.Start()
	t.Cleanup(func() {
		client.Protocol.Stop()
		m.Stop()
		_ = connA.Close()
		_ = connB.Close()
	})
	return client, connB, errs
}

// startGetPeers issues a request and consumes the resulting ShareRequest
// segment, leaving the peer free to answer it.
func startGetPeers(t *testing.T, client *Client, connB net.Conn, amount uint8) chan peersResult {
	t.Helper()
	results := make(chan peersResult, 1)
	go func() {
		peers, err := client.GetPeers(amount)
		results <- peersResult{peers: peers, err: err}
	}()
	require.NoError(t, connB.SetReadDeadline(time.Now().Add(5*time.Second)))
	segment := readPeerSharingSegment(t, connB)
	expected, err := cbor.Encode(NewMsgShareRequest(amount))
	require.NoError(t, err)
	require.Equal(t, expected, segment.Payload)
	return results
}

type peersResult struct {
	peers []PeerAddress
	err   error
}

// writeSharePeers sends a SharePeers reply from the peer side and returns its
// encoded size.
func writeSharePeers(t *testing.T, connB net.Conn, peers []PeerAddress) int {
	t.Helper()
	data, err := cbor.Encode(NewMsgSharePeers(peers))
	require.NoError(t, err)
	writePeerSharingSegment(t, connB, muxer.NewSegment(ProtocolId, data, true))
	return len(data)
}

// readSharePeersReply reads segments until a complete SharePeers message has
// been reassembled and returns its addresses along with the encoded size.
func readSharePeersReply(t *testing.T, connB net.Conn) ([]PeerAddress, int) {
	t.Helper()
	var buf []byte
	for range 16 {
		require.NoError(t, connB.SetReadDeadline(time.Now().Add(5*time.Second)))
		segment := readPeerSharingSegment(t, connB)
		require.True(t, segment.IsResponse())
		require.Equal(t, uint16(ProtocolId), segment.GetProtocolId())
		buf = append(buf, segment.Payload...)
		var values []cbor.RawMessage
		if _, err := cbor.Decode(buf, &values); err != nil {
			continue
		}
		require.Len(t, values, 2)
		var msgType uint
		_, err := cbor.Decode(values[0], &msgType)
		require.NoError(t, err)
		require.Equal(t, uint(MessageTypeSharePeers), msgType)
		var peers []PeerAddress
		_, err = cbor.Decode(values[1], &peers)
		require.NoError(t, err)
		return peers, len(buf)
	}
	t.Fatal("SharePeers reply never completed")
	return nil, 0
}

// TestClientRejectsSharePeersAboveByteLimit proves the Busy-state byte limit
// refuses a reply larger than MaxPendingMessageBytes before it is handled. The
// reply carries exactly the 255 addresses that were requested, so the
// requested-count rule cannot be what rejects it.
func TestClientRejectsSharePeersAboveByteLimit(t *testing.T) {
	client, connB, errs := testPeerSharingClient(t)
	results := startGetPeers(t, client, connB, 255)

	size := writeSharePeers(t, connB, maxSizePeerAddresses(255))
	require.Greater(t, size, MaxPendingMessageBytes)

	select {
	case err := <-errs:
		require.Error(t, err)
		require.ErrorContains(t, err, "received oversized message")
		require.ErrorContains(t, err, strconv.Itoa(MaxPendingMessageBytes))
	case <-time.After(5 * time.Second):
		t.Fatal("oversized SharePeers reply was not refused")
	}

	select {
	case result := <-results:
		require.ErrorIs(t, result.err, protocol.ErrProtocolShuttingDown)
	case <-time.After(5 * time.Second):
		t.Fatal("GetPeers stayed blocked after the oversized reply")
	}
}

// TestClientAcceptsSharePeersAtByteLimit keeps the rejection above from being
// unconditional: the largest reply that still encodes within
// MaxPendingMessageBytes is delivered to the caller.
func TestClientAcceptsSharePeersAtByteLimit(t *testing.T) {
	client, connB, errs := testPeerSharingClient(t)
	results := startGetPeers(t, client, connB, 255)

	size := writeSharePeers(t, connB, maxSizePeerAddresses(MaxSharedPeers))
	require.LessOrEqual(t, size, MaxPendingMessageBytes)

	select {
	case result := <-results:
		require.NoError(t, result.err)
		require.Len(t, result.peers, MaxSharedPeers)
	case <-time.After(5 * time.Second):
		t.Fatal("a reply within the byte limit was not delivered")
	}
	select {
	case err := <-errs:
		t.Fatalf("protocol reported an error for a legal reply: %v", err)
	default:
	}
}

// TestClientRejectsMorePeersThanRequested covers the separate protocol rule:
// a reply may be smaller than the request but never larger. The reply here is
// far below MaxPendingMessageBytes, so the byte limit cannot be what rejects
// it.
func TestClientRejectsMorePeersThanRequested(t *testing.T) {
	client, connB, errs := testPeerSharingClient(t)
	results := startGetPeers(t, client, connB, 5)

	size := writeSharePeers(t, connB, smallPeerAddresses(6))
	require.Less(t, size, MaxPendingMessageBytes)

	select {
	case err := <-errs:
		require.ErrorIs(t, err, ErrTooManyPeersShared)
		require.ErrorContains(t, err, "requested 5, received 6")
	case <-time.After(5 * time.Second):
		t.Fatal("a reply larger than the request was accepted")
	}

	select {
	case result := <-results:
		require.ErrorIs(t, result.err, protocol.ErrProtocolShuttingDown)
	case <-time.After(5 * time.Second):
		t.Fatal("GetPeers stayed blocked after the over-count reply")
	}
}

// TestClientAcceptsFewerPeersThanRequested keeps the count rule from being
// unconditional: a shorter reply is legal and is delivered.
func TestClientAcceptsFewerPeersThanRequested(t *testing.T) {
	client, connB, errs := testPeerSharingClient(t)
	results := startGetPeers(t, client, connB, 5)

	writeSharePeers(t, connB, smallPeerAddresses(3))

	select {
	case result := <-results:
		require.NoError(t, result.err)
		require.Len(t, result.peers, 3)
	case <-time.After(5 * time.Second):
		t.Fatal("a reply smaller than the request was not delivered")
	}
	select {
	case err := <-errs:
		t.Fatalf("protocol reported an error for a legal reply: %v", err)
	default:
	}
}

// TestServerClampsResponseToRequestedAmount proves the server does not answer
// with more addresses than the peer asked for, which the peer's own
// requested-count rule would treat as a protocol violation.
func TestServerClampsResponseToRequestedAmount(t *testing.T) {
	cfg := NewConfig(
		WithShareRequestFunc(func(CallbackContext, int) ([]PeerAddress, error) {
			return smallPeerAddresses(10), nil
		}),
	)
	connB, _ := testPeerSharingServer(t, &cfg)
	sendPeerSharingRequest(t, connB, 5)

	peers, _ := readSharePeersReply(t, connB)
	require.Len(t, peers, 5)
}

// TestServerClampsResponseToMaxSharedPeers proves the server cannot fail its
// own protocol on the outbound byte limit. A callback returning 255
// maximum-size addresses would encode past MaxPendingMessageBytes, which
// Protocol.enqueueMessage refuses with ErrProtocolViolationQueueExceeded,
// tearing down the shared bearer. The harness also asserts no protocol error
// was reported.
func TestServerClampsResponseToMaxSharedPeers(t *testing.T) {
	cfg := NewConfig(
		WithShareRequestFunc(func(CallbackContext, int) ([]PeerAddress, error) {
			return maxSizePeerAddresses(255), nil
		}),
	)
	connB, _ := testPeerSharingServer(t, &cfg)
	sendPeerSharingRequest(t, connB, 255)

	peers, size := readSharePeersReply(t, connB)
	require.Len(t, peers, MaxSharedPeers)
	require.LessOrEqual(t, size, MaxPendingMessageBytes)
}

// TestStateMapCarriesPendingMessageByteLimits pins the limit onto both active
// states, matching the reference implementation's byteLimitsPeerSharing.
func TestStateMapCarriesPendingMessageByteLimits(t *testing.T) {
	for _, state := range []protocol.State{stateIdle, stateBusy} {
		entry, ok := StateMap[state]
		require.True(t, ok, "missing state %s", state)
		require.Equal(
			t,
			MaxPendingMessageBytes,
			entry.PendingMessageByteLimit,
			"state %s must bound pending message bytes",
			state,
		)
	}
}
