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
	"context"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestServerAnswersPipelinedRequestNext sends a second VotesRequestNext while
// the server is still in Busy answering the first. CIP-0164 has the client
// send the next request before the previous batch is delivered, through
// ordinary protocol pipelining, so the server must hold the second request
// and answer it after the first, not fail the connection.
func TestServerAnswersPipelinedRequestNext(t *testing.T) {
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	errs := make(chan error, 4)
	entered := make(chan uint64, 2)
	release := make(chan struct{})
	server := NewServer(
		protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
				RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
			},
			ErrorChan: errs,
			Muxer:     m,
		},
		&Config{
			RequestNextFunc: func(_ CallbackContext, count uint64) ([]Vote, error) {
				entered <- count
				<-release
				votes := make([]Vote, count)
				for i := range votes {
					votes[i] = testVote()
				}
				return votes, nil
			},
		},
	)
	server.Start()
	m.Start()
	t.Cleanup(func() {
		_ = server.Stop(context.Background())
		_ = m.Stop(context.Background())
		_ = connA.Close()
		_ = connB.Close()
	})

	send := func(count uint64) {
		data, err := cbor.Encode(NewMsgVotesRequestNext(count))
		require.NoError(t, err)
		writeTestSegment(t, connB, muxer.NewSegment(ProtocolId, data, false))
	}
	send(2)
	// The callback runs after the transition to Busy, so the next request
	// reaches a server that holds agency.
	require.Equal(t, uint64(2), <-entered)
	send(1)
	select {
	case err := <-errs:
		t.Fatalf("pipelined request failed the connection: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)

	voteData, err := cbor.Encode(NewMsgVote(testVote()))
	require.NoError(t, err)
	stream := make([]byte, 0, 3*len(voteData))
	for len(stream) < 3*len(voteData) {
		select {
		case err := <-errs:
			t.Fatalf("protocol error: %v", err)
		default:
		}
		require.NoError(t, connB.SetReadDeadline(time.Now().Add(5*time.Second)))
		stream = append(stream, requireReadTestSegment(t, connB).Payload...)
	}
	require.Len(t, stream, 3*len(voteData), "three votes and nothing else")
	for i := range 3 {
		require.Equal(t, voteData, stream[i*len(voteData):(i+1)*len(voteData)])
	}
	select {
	case count := <-entered:
		require.Equal(t, uint64(1), count, "second request answered")
	case <-time.After(5 * time.Second):
		t.Fatal("second request never reached the callback")
	}
}
