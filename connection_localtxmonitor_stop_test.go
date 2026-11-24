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

package ouroboros_test

import (
	"net"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	"github.com/stretchr/testify/require"
)

func TestLocalTxMonitorStopPreservesConnection(t *testing.T) {
	for _, name := range []string{
		"idle", "acquire_release", "acquire", "get_sizes", "next_tx", "has_tx",
	} {
		t.Run(name, func(t *testing.T) {
			local, remote := net.Pipe()
			defer local.Close()
			defer remote.Close()
			type result struct {
				conn *ouroboros.Connection
				err  error
			}
			serverResult := make(chan result, 1)
			cfg := localtxmonitor.NewConfig(localtxmonitor.WithGetMempoolFunc(
				func(localtxmonitor.CallbackContext) (
					uint64, uint32, []localtxmonitor.TxAndEraId, error,
				) {
					return 42, 1000, nil, nil
				},
			))
			go func() {
				conn, err := ouroboros.NewConnection(
					ouroboros.WithConnection(
						remote,
					),
					ouroboros.WithServer(true),
					ouroboros.WithNetworkMagic(
						42,
					),
					ouroboros.WithHandshakeTimeout(time.Second),
					ouroboros.WithLocalTxMonitorConfig(cfg),
				)
				serverResult <- result{conn, err}
			}()
			client, err := ouroboros.NewConnection(
				ouroboros.WithConnection(local), ouroboros.WithNetworkMagic(42),
				ouroboros.WithHandshakeTimeout(time.Second),
			)
			require.NoError(t, err)
			defer client.Close()
			server := <-serverResult
			require.NoError(t, server.err)
			defer server.conn.Close()
			monitor := client.LocalTxMonitor().Client
			switch name {
			case "acquire", "acquire_release":
				require.NoError(t, monitor.Acquire())
				if name == "acquire_release" {
					require.NoError(t, monitor.Release())
				}
			case "get_sizes":
				_, _, _, err := monitor.GetSizes()
				require.NoError(t, err)
			case "next_tx":
				_, err := monitor.NextTx()
				require.NoError(t, err)
			case "has_tx":
				_, err := monitor.HasTxForEra(6, make([]byte, 32))
				require.NoError(t, err)
			}
			require.NoError(t, monitor.Stop())
			require.NoError(t, monitor.Stop())
			require.Eventually(
				t,
				server.conn.LocalTxMonitor().Server.IsDone,
				time.Second,
				time.Millisecond,
			)
			select {
			case err := <-client.ErrorChan():
				t.Fatalf("unexpected connection error: %v", err)
			default:
			}
			select {
			case <-client.Muxer().DoneChan():
				t.Fatal("connection closed")
			default:
			}
			require.NoError(t, client.LocalTxSubmission().Client.Stop())
			require.Eventually(
				t,
				server.conn.LocalTxSubmission().Server.IsDone,
				time.Second,
				time.Millisecond,
			)

		})
	}
}
