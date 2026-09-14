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

package localstatequery

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

type acquisitionStep struct {
	target    AcquireTarget
	reacquire bool
	failure   error
}

type acquisitionObservation struct {
	target    AcquireTarget
	reacquire bool
}

func TestAcquisitionWireMatrix(t *testing.T) {
	point := pcommon.NewPoint(42, make([]byte, 32))
	nextPoint := pcommon.NewPoint(43, make([]byte, 32))
	tests := []struct {
		name  string
		steps []acquisitionStep
	}{
		{"specific points", []acquisitionStep{
			{target: AcquireSpecificPoint{Point: point}},
			{target: AcquireSpecificPoint{Point: nextPoint}, reacquire: true},
		}},
		{"volatile tip", []acquisitionStep{
			{target: AcquireVolatileTip{}},
			{target: AcquireVolatileTip{}, reacquire: true},
		}},
		{"immutable tip", []acquisitionStep{
			{target: AcquireImmutableTip{}},
			{target: AcquireImmutableTip{}, reacquire: true},
			{target: AcquireImmutableTip{}, reacquire: true},
		}},
	}
	for _, failure := range []error{
		ErrAcquireFailurePointTooOld,
		ErrAcquireFailurePointNotOnChain,
	} {
		tests = append(tests, struct {
			name  string
			steps []acquisitionStep
		}{
			name: "acquire failure " + failure.Error(),
			steps: []acquisitionStep{
				{target: AcquireVolatileTip{}, failure: failure},
				{target: AcquireVolatileTip{}},
				{target: AcquireImmutableTip{}, reacquire: true},
			},
		}, struct {
			name  string
			steps []acquisitionStep
		}{
			name: "reacquire failure " + failure.Error(),
			steps: []acquisitionStep{
				{target: AcquireVolatileTip{}},
				{target: AcquireImmutableTip{}, reacquire: true, failure: failure},
				{target: AcquireVolatileTip{}},
				{target: AcquireSpecificPoint{Point: point}, reacquire: true},
			},
		})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testAcquisitionSequence(t, tc.steps)
		})
	}
}

func testAcquisitionSequence(t *testing.T, steps []acquisitionStep) {
	t.Helper()
	local, remote := net.Pipe()
	serverMuxer := muxer.New(local)
	clientMuxer := muxer.New(remote)
	observations := make(chan acquisitionObservation, len(steps)+1)
	serverErrors := make(chan error, 8)
	clientErrors := make(chan error, 8)
	connID := connection.ConnectionId{
		LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
		RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
	}
	callbackIndex := 0
	server := NewServer(protocol.ProtocolOptions{
		Muxer: serverMuxer, ConnectionId: connID, ErrorChan: serverErrors,
	}, &Config{
		AcquireFunc: func(
			_ CallbackContext,
			target AcquireTarget,
			reacquire bool,
		) error {
			if callbackIndex >= len(steps) {
				return errors.New("unexpected additional acquisition")
			}
			step := steps[callbackIndex]
			callbackIndex++
			observations <- acquisitionObservation{target, reacquire}
			return step.failure
		},
		QueryFunc: func(CallbackContext, QueryWrapper) (any, error) {
			return []uint64{1, 42}, nil
		},
	})
	client := NewClient(protocol.ProtocolOptions{
		Muxer: clientMuxer, ConnectionId: connID, ErrorChan: clientErrors,
	}, &Config{AcquireTimeout: 2 * time.Second, QueryTimeout: 2 * time.Second})
	t.Cleanup(func() {
		// Release transport reads/writes even when an assertion aborts the sequence.
		_ = local.Close()
		_ = remote.Close()
		server.Stop()
		client.Stop()
		serverMuxer.Stop()
		clientMuxer.Stop()
	})
	require.NoError(t, local.SetDeadline(time.Now().Add(10*time.Second)))
	require.NoError(t, remote.SetDeadline(time.Now().Add(10*time.Second)))
	server.Start()
	client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	for i, step := range steps {
		var err error
		switch target := step.target.(type) {
		case AcquireSpecificPoint:
			err = client.Acquire(&target.Point)
		case AcquireVolatileTip:
			err = client.AcquireVolatileTip()
		case AcquireImmutableTip:
			err = client.AcquireImmutableTip()
		default:
			t.Fatalf("unsupported test target %T", target)
		}
		if step.failure == nil {
			require.NoError(t, err, "step %d", i)
		} else {
			require.ErrorIs(t, err, step.failure, "step %d", i)
		}
		select {
		case observation := <-observations:
			require.Equal(t, step.target, observation.target, "step %d", i)
			require.Equal(t, step.reacquire, observation.reacquire, "step %d", i)
		case <-time.After(2 * time.Second):
			t.Fatalf("missing acquisition callback at step %d", i)
		}
		if step.failure == nil {
			// A result is a wire-order barrier: an extra Acquired cannot hide in
			// another mux segment or be mistaken for the recovery acquisition.
			blockNo, err := client.GetChainBlockNo()
			require.NoError(t, err, "query after step %d", i)
			require.Equal(t, int64(42), blockNo)
		}
	}
	require.Empty(t, observations)
	select {
	case err := <-serverErrors:
		t.Fatalf("server protocol error: %v", err)
	default:
	}
	select {
	case err := <-clientErrors:
		t.Fatalf("client protocol error: %v", err)
	default:
	}
}
