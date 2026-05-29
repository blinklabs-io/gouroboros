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

package leiosvotes

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConnectionId() connection.ConnectionId {
	return connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
}

func readTestSegment(
	t *testing.T,
	conn net.Conn,
) (*muxer.Segment, error) {
	t.Helper()
	header := muxer.SegmentHeader{}
	if err := binary.Read(conn, binary.BigEndian, &header); err != nil {
		return nil, err
	}
	payload := make([]byte, header.PayloadLength)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}
	return &muxer.Segment{
		SegmentHeader: header,
		Payload:       payload,
	}, nil
}

func requireReadTestSegment(t *testing.T, conn net.Conn) *muxer.Segment {
	t.Helper()
	segment, err := readTestSegment(t, conn)
	require.NoError(t, err)
	return segment
}

func writeTestSegment(
	t *testing.T,
	conn net.Conn,
	segment *muxer.Segment,
) {
	t.Helper()
	require.NotNil(t, segment)
	buf := &bytes.Buffer{}
	require.NoError(t, binary.Write(buf, binary.BigEndian, segment.SegmentHeader))
	_, err := buf.Write(segment.Payload)
	require.NoError(t, err)
	_, err = conn.Write(buf.Bytes())
	require.NoError(t, err)
}

func decodeTestMessageTypes(t *testing.T, payload []byte) []uint8 {
	t.Helper()
	var ret []uint8
	for len(payload) > 0 {
		tmpMsg := []cbor.RawMessage{}
		numBytesRead, err := cbor.Decode(payload, &tmpMsg)
		require.NoError(t, err)
		require.NotZero(t, numBytesRead)
		require.NotEmpty(t, tmpMsg)
		msgType := uint(0)
		_, err = cbor.Decode(tmpMsg[0], &msgType)
		require.NoError(t, err)
		ret = append(ret, uint8(msgType))
		payload = payload[numBytesRead:]
	}
	return ret
}

func isTimeout(err error) bool {
	netErr, ok := err.(net.Error)
	return ok && netErr.Timeout()
}

func TestNewClient(t *testing.T) {
	client := NewClient(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		nil,
	)

	require.NotNil(t, client)
	assert.NotNil(t, client.Protocol)
	assert.NotNil(t, client.config)
}

func TestNewClientNormalizesZeroValueConfig(t *testing.T) {
	cfg := Config{}
	client := NewClient(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		&cfg,
	)

	require.NotNil(t, client)
	assert.Equal(t, DefaultTimeout, client.config.Timeout)
	assert.Equal(t, DefaultPipelineLimit, client.config.PipelineLimit)
	assert.Equal(t, DefaultRequestNextCount, client.config.RequestNextCount)
}

func TestNewClientWithConfig(t *testing.T) {
	cfg := NewConfig(
		WithTimeout(10*time.Second),
		WithRequestNextCount(25),
		WithPipelineLimit(2),
	)

	client := NewClient(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		&cfg,
	)

	require.NotNil(t, client)
	assert.Equal(t, 10*time.Second, client.config.Timeout)
	assert.Equal(t, uint64(25), client.config.RequestNextCount)
	assert.Equal(t, 2, client.config.PipelineLimit)
}

func TestClientMessageHandler(t *testing.T) {
	client := NewClient(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		nil,
	)
	vote := testVote()
	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.Equal(t, vote, <-client.voteChan)
	}()

	err := client.messageHandler(NewMsgVote(vote))
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for vote")
	}
}

func TestClientMessageHandlerUnexpectedType(t *testing.T) {
	client := NewClient(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		nil,
	)

	err := client.messageHandler(NewMsgVotesRequestNext(1))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected message type")
}

func TestStopReturnsOriginalErrorOnRepeatedCalls(t *testing.T) {
	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()

	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        muxer.New(connA),
		},
		nil,
	)
	client.Protocol.Stop()

	err := client.Stop()
	require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	err = client.Stop()
	require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
}

func TestStopDuringSyncDoesNotRefillAfterDone(t *testing.T) {
	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()

	m := muxer.New(connA)
	defer m.Stop()

	stopErr := make(chan error, 1)
	var client *Client
	cfg := NewConfig(
		WithPipelineLimit(1),
		WithRequestNextCount(1),
		WithVoteFunc(func(ctx CallbackContext, vote Vote) error {
			stopErr <- client.Stop()
			return nil
		}),
	)
	client = NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        m,
		},
		&cfg,
	)
	client.Start()
	m.Start()
	defer client.Protocol.Stop()

	require.NoError(t, client.Sync())

	firstSegment := requireReadTestSegment(t, connB)
	assert.Equal(
		t,
		[]uint8{MessageTypeVotesRequestNext},
		decodeTestMessageTypes(t, firstSegment.Payload),
	)

	voteData, err := cbor.Encode(NewMsgVote(testVote()))
	require.NoError(t, err)
	writeTestSegment(t, connB, muxer.NewSegment(ProtocolId, voteData, true))

	select {
	case err := <-stopErr:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Stop from vote callback")
	}

	secondSegment := requireReadTestSegment(t, connB)
	assert.Equal(
		t,
		[]uint8{MessageTypeDone},
		decodeTestMessageTypes(t, secondSegment.Payload),
	)

	require.NoError(t, connB.SetReadDeadline(time.Now().Add(50*time.Millisecond)))
	extraSegment, err := readTestSegment(t, connB)
	if err != nil {
		require.True(t, isTimeout(err), "unexpected read error: %v", err)
		return
	}
	assert.NotContains(
		t,
		decodeTestMessageTypes(t, extraSegment.Payload),
		MessageTypeVotesRequestNext,
	)
}

func TestRequestNextRejectsWhileSyncRunning(t *testing.T) {
	client := NewClient(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		nil,
	)
	client.syncRunning = true

	_, err := client.RequestNext(1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vote sync loop is already running")
}

func TestStateMap(t *testing.T) {
	assert.Equal(t, uint(1), StateIdle.Id)
	assert.Equal(t, uint(2), StateBusy.Id)
	assert.Equal(t, uint(3), StateDone.Id)

	assert.Contains(t, StateMap, StateIdle)
	assert.Contains(t, StateMap, StateBusy)
	assert.Contains(t, StateMap, StateDone)

	assert.Equal(t, protocol.AgencyClient, StateMap[StateIdle].Agency)
	assert.Equal(t, protocol.AgencyServer, StateMap[StateBusy].Agency)
	assert.Equal(t, protocol.AgencyNone, StateMap[StateDone].Agency)
}

func TestStateTransitions(t *testing.T) {
	idleEntry := StateMap[StateIdle]
	require.Len(t, idleEntry.Transitions, 2)

	ctx := &stateContext{}
	req := NewMsgVotesRequestNext(2)
	assert.True(t, idleEntry.Transitions[0].MatchFunc(ctx, req))
	assert.Equal(t, uint64(2), ctx.tokens)

	busyEntry := StateMap[StateBusy]
	require.Len(t, busyEntry.Transitions, 2)
	assert.True(t, busyEntry.Transitions[0].MatchFunc(ctx, NewMsgVote(testVote())))
	assert.Equal(t, uint64(1), ctx.tokens)
	assert.True(t, busyEntry.Transitions[1].MatchFunc(ctx, NewMsgVote(testVote())))
	assert.Equal(t, uint64(0), ctx.tokens)
}

func TestStateTransitionRejectsInvalidCount(t *testing.T) {
	idleEntry := StateMap[StateIdle]
	ctx := &stateContext{}

	assert.False(
		t,
		idleEntry.Transitions[0].MatchFunc(ctx, NewMsgVotesRequestNext(0)),
	)
	assert.False(
		t,
		idleEntry.Transitions[0].MatchFunc(
			ctx,
			NewMsgVotesRequestNext(MaxRequestNextCount+1),
		),
	)
}

func TestConfig(t *testing.T) {
	cfg := NewConfig()
	assert.Equal(t, DefaultTimeout, cfg.Timeout)
	assert.Equal(t, DefaultPipelineLimit, cfg.PipelineLimit)
	assert.Equal(t, DefaultRequestNextCount, cfg.RequestNextCount)
	assert.Nil(t, cfg.RequestNextFunc)
	assert.Nil(t, cfg.VoteFunc)

	requestNextCalled := false
	voteCalled := false
	cfg = NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext, count uint64) ([]Vote, error) {
			requestNextCalled = true
			return make([]Vote, count), nil
		}),
		WithVoteFunc(func(ctx CallbackContext, vote Vote) error {
			voteCalled = true
			return nil
		}),
	)

	_, _ = cfg.RequestNextFunc(CallbackContext{}, 1)
	assert.True(t, requestNextCalled)
	_ = cfg.VoteFunc(CallbackContext{}, Vote{})
	assert.True(t, voteCalled)
}

func TestConfigRejectsInvalidTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		cfg := Config{
			PipelineLimit:    DefaultPipelineLimit,
			RequestNextCount: DefaultRequestNextCount,
			Timeout:          timeout,
		}
		assert.EqualError(t, cfg.validate(), "timeout must be positive")
	}
}

func TestProtocolConstants(t *testing.T) {
	assert.Equal(t, "leios-votes", ProtocolName)
	assert.Equal(t, uint16(20), ProtocolId)
}
