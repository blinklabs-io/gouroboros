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

package muxer_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestStartProtocolGatesOtherProtocols(t *testing.T) {
	defer goleak.VerifyNone(t)
	for _, resume := range []bool{false, true} {
		name := "stop"
		if resume {
			name = "start"
		}
		t.Run(name, func(t *testing.T) {
			local, peer := net.Pipe()
			defer peer.Close()
			m := muxer.New(local)
			defer m.Stop()
			_, startup, _ := m.RegisterProtocol(0, muxer.ProtocolRoleResponder)
			m.StartProtocol(0)
			written := make(chan error, 1)
			go func() {
				for _, id := range []uint16{0, 0, 1} {
					segment := muxer.NewSegment(id, []byte{1}, false)
					if segment == nil {
						written <- fmt.Errorf("could not create segment")
						return
					}
					_, err := peer.Write(createSegmentData(segment))
					if err != nil {
						written <- err
						return
					}
				}
				written <- nil
			}()
			for range 2 {
				select {
				case segment := <-startup:
					require.NotNil(t, segment)
				case <-time.After(time.Second):
					t.Fatal("startup segment was not delivered")
				}
			}
			select {
			case err := <-written:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("muxer did not read the following segment")
			}
			select {
			case err := <-m.ErrorChan():
				t.Fatalf(
					"dispatched unregistered protocol before Start: %v",
					err,
				)
			case <-time.After(20 * time.Millisecond):
			}
			if resume {
				_, next, _ := m.RegisterProtocol(1, muxer.ProtocolRoleResponder)
				m.SetDiffusionMode(muxer.DiffusionModeResponder)
				m.Start()
				select {
				case segment := <-next:
					require.NotNil(t, segment)
					require.Equal(t, []byte{1}, segment.Payload)
				case <-time.After(time.Second):
					t.Fatal("held segment was not dispatched after Start")
				}
			}
			m.Stop()
			select {
			case <-m.ErrorChan():
			case <-time.After(time.Second):
				t.Fatal("muxer did not stop while gated")
			}
		})
	}
}

func TestStartOverridesPendingProtocolStart(t *testing.T) {
	defer goleak.VerifyNone(t)
	local, peer := net.Pipe()
	defer peer.Close()
	m := muxer.New(local)
	defer m.Stop()
	_, recv, _ := m.RegisterProtocol(1, muxer.ProtocolRoleResponder)
	m.StartProtocol(0)
	m.Start()
	require.NoError(t, peer.SetWriteDeadline(time.Now().Add(time.Second)))
	segment := muxer.NewSegment(1, []byte{1}, false)
	require.NotNil(t, segment)
	_, err := peer.Write(createSegmentData(segment))
	require.NoError(t, err)
	select {
	case segment := <-recv:
		require.NotNil(t, segment)
	case <-time.After(time.Second):
		t.Fatal("Start was lost behind pending protocol start")
	}
}
