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
	"context"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// startMeasuresPair connects a client and a server that negotiated version.
func startMeasuresPair(
	t *testing.T,
	version uint16,
	txs []TxAndEraId,
) (*Client, chan error) {
	t.Helper()
	connA, connB := net.Pipe()
	clientMuxer := muxer.New(connA)
	serverMuxer := muxer.New(connB)
	errorChan := make(chan error, 10)
	opts := func(m *muxer.Muxer) protocol.ProtocolOptions {
		return protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  &net.TCPAddr{},
				RemoteAddr: &net.TCPAddr{},
			},
			Muxer:     m,
			ErrorChan: errorChan,
			Mode:      protocol.ProtocolModeNodeToClient,
			Version:   version,
		}
	}
	cfg := NewConfig(WithGetMempoolFunc(
		func(CallbackContext) (uint64, uint32, []TxAndEraId, error) {
			return 7, 1000, txs, nil
		},
	))
	client := NewClient(opts(clientMuxer), &cfg)
	server := NewServer(opts(serverMuxer), &cfg)
	client.Start()
	server.Start()
	clientMuxer.Start()
	serverMuxer.Start()
	t.Cleanup(func() {
		_ = client.Protocol.Stop(context.Background())
		_ = server.Stop(context.Background())
		_ = clientMuxer.Stop(context.Background())
		_ = serverMuxer.Stop(context.Background())
		connA.Close()
		connB.Close()
	})
	return client, errorChan
}

// TestGetMeasuresCountsRemainingTransactions checks a client and server
// round trip, and that the measures cover the transactions NextTx has not
// returned, as the reference server's do.
func TestGetMeasuresCountsRemainingTransactions(t *testing.T) {
	txs := []TxAndEraId{
		{EraId: 6, Tx: make([]byte, 10)},
		{EraId: 6, Tx: make([]byte, 25)},
	}
	client, errorChan := startMeasuresPair(
		t,
		protocol.ProtocolVersionNtCOffset+getMeasuresMinVersion,
		txs,
	)
	measures, err := client.GetMeasures()
	require.NoError(t, err)
	require.Equal(t, MempoolMeasures{
		TxCount: 2,
		Measures: map[string]MeasureSizeAndCapacity{
			MeasureTransactionBytes: {Size: 35, Capacity: 1000},
		},
	}, measures)
	_, err = client.NextTx()
	require.NoError(t, err)
	measures, err = client.GetMeasures()
	require.NoError(t, err)
	require.Equal(t, uint32(1), measures.TxCount)
	require.Equal(
		t,
		MeasureSizeAndCapacity{Size: 25, Capacity: 1000},
		measures.Measures[MeasureTransactionBytes],
	)
	select {
	case err := <-errorChan:
		t.Fatalf("protocol failed: %v", err)
	default:
	}
}

// TestGetMeasuresNeedsVersion20 checks that before node-to-client version
// 20 the client refuses GetMeasures without sending it, and the connection
// stays usable.
func TestGetMeasuresNeedsVersion20(t *testing.T) {
	client, errorChan := startMeasuresPair(
		t,
		protocol.ProtocolVersionNtCOffset+getMeasuresMinVersion-1,
		nil,
	)
	_, err := client.GetMeasures()
	require.ErrorIs(t, err, ErrGetMeasuresUnsupported)
	_, _, _, err = client.GetSizes()
	require.NoError(t, err)
	select {
	case err := <-errorChan:
		t.Fatalf("protocol failed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestServerRefusesGetMeasuresBeforeVersion20 checks that a server on an
// older version fails the protocol instead of replying, as the reference
// encoder does.
func TestServerRefusesGetMeasuresBeforeVersion20(t *testing.T) {
	s := &Server{
		config:            &Config{},
		enableGetMeasures: supportsGetMeasures(protocol.ProtocolVersionNtCOffset + 19),
	}
	s.proto = protocol.New(protocol.ProtocolConfig{
		Name:         ProtocolName,
		Role:         protocol.ProtocolRoleServer,
		StateMap:     StateMap,
		InitialState: stateIdle,
	})
	require.ErrorContains(t, s.handleGetMeasures(), "version 20")
	require.True(t, supportsGetMeasures(protocol.ProtocolVersionNtCOffset+20))
}
