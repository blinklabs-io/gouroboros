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
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// TestSegmentReadTimeout_CustomShortTimeoutFires proves
// NewWithSegmentReadTimeout's override actually reaches the read loop's
// deadline: with nothing sent by the peer, the muxer must report an error
// once the configured (short, test-friendly) timeout elapses. Uses
// net.Pipe rather than mockConn, since mockConn's SetReadDeadline is a
// no-op (it never blocks a real Read call), so it cannot exercise this
// behavior at all.
func TestSegmentReadTimeout_CustomShortTimeoutFires(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })

	m := muxer.NewWithSegmentReadTimeout(serverConn, 50*time.Millisecond)
	defer m.Stop()
	m.Start()

	select {
	case err := <-m.ErrorChan():
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal(
			"expected a read-timeout error within the configured 50ms " +
				"deadline, got none after 2s",
		)
	}
}

// TestSegmentReadTimeout_DisabledNeverFires is the regression test for the
// actual bug this option exists to fix: a legitimate peer that is still
// computing a reply (e.g. Dingo resolving a whole-UTxO-set GetUTxOWhole
// query) must not have its connection killed. segmentReadTimeout <= 0
// disables the deadline entirely -- not just "a longer" one -- so the
// muxer must report no error even after outliving the short timeout
// TestSegmentReadTimeout_CustomShortTimeoutFires proved does fire, and the
// connection must still be able to deliver a real, late-arriving segment
// afterward.
func TestSegmentReadTimeout_DisabledNeverFires(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })

	m := muxer.NewWithSegmentReadTimeout(serverConn, 0)
	defer m.Stop()

	_, recvChan, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	require.NotNil(t, recvChan)
	m.Start()

	// Comfortably longer than the 50ms proven to trigger a timeout above,
	// with nothing sent -- simulating a peer legitimately still computing.
	select {
	case err := <-m.ErrorChan():
		t.Fatalf(
			"expected no error with segmentReadTimeout disabled, got: %v",
			err,
		)
	case <-time.After(300 * time.Millisecond):
		// Good -- no premature timeout.
	}

	// The connection must still be alive and able to deliver a real,
	// late-arriving segment, proving this is a genuinely unbounded wait,
	// not just a longer bound.
	segment := muxer.NewSegment(0x01, []byte("late reply"), false)
	require.NotNil(t, segment, "payload is well within NewSegment's size limit")
	data := createSegmentData(segment)
	writeErrChan := make(chan error, 1)
	go func() {
		_, err := clientConn.Write(data)
		writeErrChan <- err
	}()

	select {
	case err := <-writeErrChan:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out writing the late segment")
	}

	select {
	case seg := <-recvChan:
		require.Equal(t, []byte("late reply"), seg.Payload)
	case err := <-m.ErrorChan():
		t.Fatalf("expected the late segment to be delivered, got error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("expected the late segment to be delivered within 2s")
	}
}
