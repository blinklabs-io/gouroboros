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

package blockfetch

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// gateHandler holds every log record whose message starts with prefix until
// gate is closed. It keeps no other state, so logging through it creates no
// synchronisation between goroutines.
type gateHandler struct {
	prefix string
	gate   <-chan struct{}
}

func (*gateHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *gateHandler) Handle(_ context.Context, r slog.Record) error {
	if strings.HasPrefix(r.Message, h.prefix) {
		<-h.gate
	}
	return nil
}

func (h *gateHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *gateHandler) WithGroup(string) slog.Handler { return h }

// TestFetchDuringRestartReadsProtocolUnderLock runs GetBlockRange and
// GetBlock on their own goroutines while Start replaces the stopped client's
// protocol instance. Under the race detector it fails if either reads the
// instance without protocolMu. Each call is held in its first log record, so
// it reaches the client's lock only after Start has written the instance,
// and nothing else orders it with Start.
func TestFetchDuringRestartReadsProtocolUnderLock(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fetch func(*Client) error
	}{
		{"GetBlockRange", func(c *Client) error {
			return c.GetBlockRange(
				pcommon.NewPointOrigin(),
				pcommon.NewPointOrigin(),
			)
		}},
		{"GetBlock", func(c *Client) error {
			_, err := c.GetBlock(pcommon.NewPointOrigin())
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			localConn, peerConn := net.Pipe()
			m := muxer.New(localConn)
			m.Start()
			peerDone := make(chan struct{})
			go func() {
				_, _ = io.Copy(io.Discard, peerConn)
				close(peerDone)
			}()
			gate := make(chan struct{})
			c := NewClient(protocol.ProtocolOptions{
				ConnectionId: connection.ConnectionId{
					LocalAddr:  localConn.LocalAddr(),
					RemoteAddr: localConn.RemoteAddr(),
				},
				Muxer: m,
				Mode:  protocol.ProtocolModeNodeToNode,
				Logger: slog.New(&gateHandler{
					prefix: "calling " + tc.name,
					gate:   gate,
				}),
			}, &Config{SkipBlockValidation: true})
			c.Start()
			require.NoError(t, c.Stop(context.Background()))

			fetched := make(chan error, 1)
			go func() { fetched <- tc.fetch(c) }()
			c.Start()
			close(gate)
			// The fetch waits for a batch the peer never starts; Stop
			// ends that wait.
			require.NoError(t, c.Stop(context.Background()))
			require.Error(t, <-fetched)
			m.Stop()
			_ = peerConn.Close()
			<-peerDone
		})
	}
}
