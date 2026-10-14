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
	"math"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// longestHead returns a CBOR head of the given major type in its longest,
// 9-byte form.
func longestHead(majorType byte, value uint64) []byte {
	head := make([]byte, 9)
	head[0] = majorType | 27
	binary.BigEndian.PutUint64(head[1:], value)
	return head
}

// longestHeadVoteMessage encodes a Vote message for voterId with every CBOR
// head in its longest form: the largest encoding of a valid vote that uses
// only definite-length items.
func longestHeadVoteMessage(voterId uint64) []byte {
	vote := testVote()
	var b bytes.Buffer
	b.Write(longestHead(cbor.CborTypeArray, 2))
	b.Write(longestHead(0x00, MessageTypeVote))
	b.Write(longestHead(cbor.CborTypeArray, 4))
	b.Write(longestHead(0x00, vote.SlotNo))
	b.Write(longestHead(cbor.CborTypeByteString, 32))
	b.Write(vote.EndorserBlockHash.Bytes())
	b.Write(longestHead(0x00, voterId))
	b.Write(longestHead(cbor.CborTypeByteString, 48))
	b.Write(vote.VoteSignature)
	return b.Bytes()
}

// paddedVoteMessage encodes a valid Vote message of exactly size bytes by
// splitting the signature into an indefinite-length byte string padded with
// empty chunks.
func paddedVoteMessage(t *testing.T, size int) []byte {
	t.Helper()
	vote := testVote()
	prefix, err := cbor.Encode([]any{
		MessageTypeVote,
		[]any{vote.SlotNo, vote.EndorserBlockHash, vote.VoterId},
	})
	require.NoError(t, err)
	// Widen the vote array from 3 to 4 elements to make room for the
	// signature appended below.
	prefix[2] = cbor.CborTypeArray | 4
	signature, err := cbor.Encode(vote.VoteSignature)
	require.NoError(t, err)
	var b bytes.Buffer
	b.Write(prefix)
	b.WriteByte(cbor.CborTypeByteString | 31)
	b.Write(signature)
	padding := size - b.Len() - 1
	require.GreaterOrEqual(t, padding, 0)
	b.Write(bytes.Repeat([]byte{cbor.CborTypeByteString}, padding))
	b.WriteByte(0xff)
	require.Len(t, b.Bytes(), size)
	return b.Bytes()
}

// TestNewMsgFromCborVoteSizeLimit checks that a Vote message is admitted up
// to MaxVoteMessageBytes, including its longest definite-length encoding,
// and refused above it before decoding.
func TestNewMsgFromCborVoteSizeLimit(t *testing.T) {
	longest := longestHeadVoteMessage(42)
	require.Len(t, longest, 143)
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "longest heads", data: longest},
		{
			name: "padded to limit",
			data: paddedVoteMessage(t, MaxVoteMessageBytes),
		},
		{
			name:    "padded past limit",
			data:    paddedVoteMessage(t, MaxVoteMessageBytes+1),
			wantErr: true,
		},
		{
			name:    "maximally padded",
			data:    paddedVoteMessage(t, 60_000),
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := NewMsgFromCbor(MessageTypeVote, tc.data)
			if tc.wantErr {
				require.ErrorContains(t, err, "vote message is")
				return
			}
			require.NoError(t, err)
			vote := msg.(*MsgVote).Vote
			assert.Equal(t, testVote().SlotNo, vote.SlotNo)
			assert.Equal(t, testVote().VoteSignature, vote.VoteSignature)
		})
	}
}

// TestNewMsgFromCborVoteSharesEncoding checks that a decoded Vote message
// and its vote retain one copy of the encoding, owned rather than aliasing
// the caller's buffer, for definite- and indefinite-length message arrays.
func TestNewMsgFromCborVoteSharesEncoding(t *testing.T) {
	voteData, err := cbor.Encode(testVote())
	require.NoError(t, err)
	tests := map[string][]byte{
		"definite": append([]byte{0x82, MessageTypeVote}, voteData...),
		"indefinite": append(
			append([]byte{0x9f, MessageTypeVote}, voteData...),
			0xff,
		),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			input := bytes.Clone(data)
			msg, err := NewMsgFromCbor(MessageTypeVote, input)
			require.NoError(t, err)
			msgVote := msg.(*MsgVote)
			require.Equal(t, data, msgVote.Cbor())
			require.Equal(t, voteData, msgVote.Vote.Cbor())
			assert.Same(t, &msgVote.Cbor()[2], &msgVote.Vote.Cbor()[0])
			input[2] ^= 0xff
			assert.Equal(t, data, msgVote.Cbor())
		})
	}
}

// TestClientStateMapBoundsVotes checks the client's Busy limits: one vote
// or the client's queued requests per message, and the pending receive
// budget, while the shared StateMap used by servers stays unbounded.
func TestClientStateMapBoundsVotes(t *testing.T) {
	cfg := NewConfig()
	busy := clientStateMap(&cfg)[StateBusy]
	assert.Equal(t, cfg.Timeout, busy.Timeout)
	assert.Equal(t, clientBusyMessageBytes, busy.PendingMessageByteLimit)
	assert.Equal(t, PendingReceiveBytes, busy.PendingReceiveByteBudget)
	assert.GreaterOrEqual(t, clientBusyMessageBytes, MaxVoteMessageBytes)
	assert.GreaterOrEqual(
		t,
		clientBusyMessageBytes,
		(MaxPipelineLimit+1)*maxClientRequestBytes,
	)

	request, err := cbor.Encode(NewMsgVotesRequestNext(math.MaxUint64))
	require.NoError(t, err)
	assert.Len(t, request, maxClientRequestBytes)

	assert.Zero(t, StateMap[StateBusy].PendingMessageByteLimit)
	assert.Zero(t, StateMap[StateBusy].PendingReceiveByteBudget)
}

// startBudgetTestClient starts a client on one end of a pipe with the given
// vote callback and returns the peer end and the client's error channel.
func startBudgetTestClient(
	t *testing.T,
	count uint64,
	voteFunc VoteFunc,
) (net.Conn, chan error) {
	t.Helper()
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})
	m := muxer.New(connA)
	t.Cleanup(m.Stop)
	errorChan := make(chan error, 10)
	cfg := NewConfig(
		WithRequestNextCount(count),
		WithVoteFunc(voteFunc),
	)
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        m,
			ErrorChan:    errorChan,
		},
		&cfg,
	)
	client.Start()
	m.Start()
	t.Cleanup(client.Protocol.Stop)
	require.NoError(t, client.Sync())
	assert.Equal(
		t,
		[]uint8{MessageTypeVotesRequestNext},
		decodeTestMessageTypes(t, requireReadTestSegment(t, connB).Payload),
	)
	return connB, errorChan
}

// writeTestMessages sends messages to the client, packing as many as fit
// into each segment.
func writeTestMessages(t *testing.T, conn net.Conn, messages [][]byte) {
	t.Helper()
	var payload []byte
	for _, msg := range messages {
		if len(payload)+len(msg) > muxer.SegmentMaxPayloadLength {
			writeTestSegment(
				t,
				conn,
				muxer.NewSegment(ProtocolId, payload, true),
			)
			payload = nil
		}
		payload = append(payload, msg...)
	}
	if len(payload) > 0 {
		writeTestSegment(t, conn, muxer.NewSegment(ProtocolId, payload, true))
	}
}

// TestClientSlowConsumerRejectsPaddedVote checks that a client whose
// consumer is stalled fails the protocol on a maximally padded vote instead
// of queueing it, and never hands that vote to the consumer.
func TestClientSlowConsumerRejectsPaddedVote(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	votes := make(chan Vote, 2)
	conn, errorChan := startBudgetTestClient(
		t,
		2,
		func(_ CallbackContext, vote Vote) error {
			votes <- vote
			<-release
			return nil
		},
	)

	// Stall the consumer on a first vote before the padded one arrives.
	writeTestMessages(t, conn, [][]byte{longestHeadVoteMessage(1)})
	select {
	case vote := <-votes:
		require.Equal(t, uint64(1), vote.VoterId)
	case <-time.After(5 * time.Second):
		t.Fatal("first vote was not delivered")
	}

	writeTestMessages(t, conn, [][]byte{paddedVoteMessage(t, 60_000)})
	select {
	case err := <-errorChan:
		require.ErrorContains(t, err, "exceeding limit")
	case <-time.After(5 * time.Second):
		t.Fatal("padded vote was not refused")
	}
	assert.Empty(t, votes)
}

// TestClientSlowConsumerReceivesFullBatch checks that votes held back for a
// stalled consumer are delivered in order once it resumes, without failing
// an honest peer that sent a full batch of longest-encoded votes.
func TestClientSlowConsumerReceivesFullBatch(t *testing.T) {
	const count = MaxRequestNextCount
	release := make(chan struct{})
	votes := make(chan Vote, count)
	conn, errorChan := startBudgetTestClient(
		t,
		count,
		func(_ CallbackContext, vote Vote) error {
			<-release
			votes <- vote
			return nil
		},
	)

	messages := make([][]byte, 0, count)
	for i := range uint64(count) {
		messages = append(messages, longestHeadVoteMessage(i))
	}
	writeTestMessages(t, conn, messages)

	select {
	case err := <-errorChan:
		t.Fatalf("stalled consumer failed the protocol: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	for i := range uint64(count) {
		select {
		case vote := <-votes:
			require.Equal(t, i, vote.VoterId)
		case err := <-errorChan:
			t.Fatalf("protocol failed after %d votes: %v", i, err)
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out after %d votes", i)
		}
	}
}
