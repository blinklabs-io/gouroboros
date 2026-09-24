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

package chainsync

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestRequestNextBurstPipelinesWhileServerHasAgency checks that a burst of
// MsgRequestNext reaches the wire before the server sends any reply. The
// first RequestNext moves the client from Idle to CanAwait, where the server
// holds agency, so every later one depends on CanAwait permitting pipelined
// RequestNext. Without that, the client sends one RequestNext per server
// reply and PipelineLimit has no effect.
func TestRequestNextBurstPipelinesWhileServerHasAgency(t *testing.T) {
	for name, stateMap := range map[string]protocol.StateMap{
		"NtN": StateMapNtN,
		"NtC": StateMapNtC,
	} {
		t.Run(name, func(t *testing.T) {
			const burst = 5
			localConn, peerConn := net.Pipe()
			t.Cleanup(func() {
				_ = localConn.Close()
				_ = peerConn.Close()
			})
			m := muxer.New(localConn)
			m.Start()
			t.Cleanup(m.Stop)

			payloadBytes := make(chan int, 100)
			go func() {
				var buf []byte
				chunk := make([]byte, 4096)
				for {
					n, err := peerConn.Read(chunk)
					if err != nil {
						return
					}
					buf = append(buf, chunk[:n]...)
					// Segment header: 4-byte timestamp, 2-byte protocol ID,
					// 2-byte payload length.
					for len(buf) >= 8 {
						length := int(binary.BigEndian.Uint16(buf[6:8]))
						if len(buf) < 8+length {
							break
						}
						payloadBytes <- length
						buf = buf[8+length:]
					}
				}
			}()

			errorChan := make(chan error, 1)
			p := protocol.New(protocol.ProtocolConfig{
				Name:         "chainsync-pipeline",
				ErrorChan:    errorChan,
				Muxer:        m,
				Role:         protocol.ProtocolRoleClient,
				StateMap:     stateMap,
				InitialState: stateIdle,
			})
			p.Start()
			t.Cleanup(p.Stop)

			encoded, err := cbor.Encode(NewMsgRequestNext())
			require.NoError(t, err)
			msgLen := len(encoded)
			for range burst {
				require.NoError(t, p.SendMessage(NewMsgRequestNext()))
			}

			total := 0
			deadline := time.After(2 * time.Second)
			for total < burst*msgLen {
				select {
				case n := <-payloadBytes:
					total += n
				case err := <-errorChan:
					t.Fatalf("unexpected protocol error: %v", err)
				case <-deadline:
					t.Fatalf(
						"only %d of %d RequestNext bytes were written before any server reply",
						total,
						burst*msgLen,
					)
				}
			}
			require.Equal(t, burst*msgLen, total)
		})
	}
}
