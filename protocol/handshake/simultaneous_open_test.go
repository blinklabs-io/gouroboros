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

package handshake_test

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

type simOpenResult struct {
	version uint16
	data    protocol.VersionData
	err     error
}

// simOpenTable returns the full-duplex node-to-node version table for
// magic, restricted to versions.
func simOpenTable(
	t *testing.T,
	magic uint32,
	versions ...uint16,
) protocol.ProtocolVersionMap {
	t.Helper()
	return simOpenTableMode(
		t,
		magic,
		protocol.DiffusionModeInitiatorAndResponder,
		versions...,
	)
}

// simOpenTableMode returns the node-to-node version table for magic and
// diffusion mode, restricted to versions.
func simOpenTableMode(
	t *testing.T,
	magic uint32,
	diffusionMode bool,
	versions ...uint16,
) protocol.ProtocolVersionMap {
	t.Helper()
	all := protocol.GetProtocolVersionMap(
		protocol.ProtocolModeNodeToNode,
		magic,
		diffusionMode,
		false,
		false,
	)
	table := protocol.ProtocolVersionMap{}
	for _, version := range versions {
		data, ok := all[version]
		require.True(t, ok, "no node-to-node version %d", version)
		table[version] = data
	}
	return table
}

// simOpenPointerTable returns table with each entry held as a pointer to a
// copy, which the version data interface allows since its methods have
// value receivers.
func simOpenPointerTable(
	table protocol.ProtocolVersionMap,
) protocol.ProtocolVersionMap {
	pointers := protocol.ProtocolVersionMap{}
	for version, data := range table {
		ptr := reflect.New(reflect.TypeOf(data))
		ptr.Elem().Set(reflect.ValueOf(data))
		pointers[version] = ptr.Interface().(protocol.VersionData)
	}
	return pointers
}

// startSimOpenSide runs a handshake client on conn, as a dialling node does,
// and reports how its handshake ended.
func startSimOpenSide(
	t *testing.T,
	conn net.Conn,
	table protocol.ProtocolVersionMap,
) <-chan simOpenResult {
	t.Helper()
	results := make(chan simOpenResult, 2)
	protoMuxer := muxer.New(conn)
	t.Cleanup(func() { _ = protoMuxer.Stop(context.Background()) })
	errorChan := make(chan error, 1)
	config := handshake.NewConfig(
		handshake.WithProtocolVersionMap(table),
		handshake.WithFinishedFunc(func(
			_ handshake.CallbackContext,
			version uint16,
			data protocol.VersionData,
		) error {
			results <- simOpenResult{version: version, data: data}
			return nil
		}),
	)
	client := handshake.NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Muxer:     protoMuxer,
		ErrorChan: errorChan,
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &config)
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	go func() {
		select {
		case err := <-errorChan:
			results <- simOpenResult{err: err}
		case err, ok := <-protoMuxer.ErrorChan():
			if ok {
				results <- simOpenResult{err: err}
			}
		}
	}()
	client.Start()
	protoMuxer.StartOnce()
	return results
}

func awaitSimOpen(t *testing.T, results <-chan simOpenResult) simOpenResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the handshake to end")
		return simOpenResult{}
	}
}

// TestSimultaneousOpen covers two dialling clients on one connection: each
// reads the other's MsgProposeVersions as MsgReplyVersions and chooses the
// version itself, as the network specification's simultaneous open section
// and the reference client's MsgReplyVersions case require. Nothing is sent
// after the proposals, so a refusing side only reports its own reason. Both
// sides end with the agreed version data, as the reference acceptOrRefuse
// returns it (Handshake/Client.hs 201-215), so their connections derive the
// same diffusion mode.
func TestSimultaneousOpen(t *testing.T) {
	const magic = 764824073
	for _, test := range []struct {
		name        string
		left, right protocol.ProtocolVersionMap
		wantVersion uint16
		wantRefusal error
		wantMode    bool
	}{
		{
			name:        "both choose the highest common version",
			left:        simOpenTable(t, magic, 13, 14),
			right:       simOpenTable(t, magic, 13, 14),
			wantVersion: 14,
			wantMode:    protocol.DiffusionModeInitiatorAndResponder,
		},
		{
			name:        "highest version only one side speaks is skipped",
			left:        simOpenTable(t, magic, 13, 14),
			right:       simOpenTable(t, magic, 13),
			wantVersion: 13,
			wantMode:    protocol.DiffusionModeInitiatorAndResponder,
		},
		{
			name: "initiator-only on either side is agreed by both",
			left: simOpenTable(t, magic, 13, 14),
			right: simOpenTableMode(
				t,
				magic,
				protocol.DiffusionModeInitiatorOnly,
				13, 14,
			),
			wantVersion: 14,
			wantMode:    protocol.DiffusionModeInitiatorOnly,
		},
		{
			name: "a table holding pointers agrees like one holding values",
			left: simOpenPointerTable(simOpenTable(t, magic, 13, 14)),
			right: simOpenTableMode(
				t,
				magic,
				protocol.DiffusionModeInitiatorOnly,
				13, 14,
			),
			wantVersion: 14,
			wantMode:    protocol.DiffusionModeInitiatorOnly,
		},
		{
			name:        "no common version",
			left:        simOpenTable(t, magic, 13),
			right:       simOpenTable(t, magic, 14),
			wantRefusal: &handshake.VersionMismatchError{},
		},
		{
			name:        "network magic mismatch",
			left:        simOpenTable(t, magic, 14),
			right:       simOpenTable(t, magic+1, 14),
			wantRefusal: &handshake.RefusedError{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Registered first so it runs after the muxers' cleanups.
			t.Cleanup(func() { goleak.VerifyNone(t) })
			leftConn, rightConn := net.Pipe()
			left := startSimOpenSide(t, leftConn, test.left)
			right := startSimOpenSide(t, rightConn, test.right)
			got := []simOpenResult{
				awaitSimOpen(t, left),
				awaitSimOpen(t, right),
			}
			for i, result := range got {
				if test.wantRefusal != nil {
					var refusal handshake.RefusalError
					require.ErrorAs(t, result.err, &refusal, "side %d", i)
					require.Equal(
						t,
						test.wantRefusal.(handshake.RefusalError).ReasonCode(),
						refusal.ReasonCode(),
						"side %d", i,
					)
					continue
				}
				require.NoError(t, result.err, "side %d", i)
				require.Equal(t, test.wantVersion, result.version, "side %d", i)
			}
			if test.wantRefusal == nil {
				require.Equal(t, got[0].data, got[1].data)
				require.Equal(t, test.wantMode, got[0].data.DiffusionMode())
			}
		})
	}
}

// TestSimultaneousOpenVersionMismatchListsOwnVersions checks a side that
// finds no common version reports its own versions, as the reference
// acceptOrRefuse does, since it has no server to hear them from.
func TestSimultaneousOpenVersionMismatchListsOwnVersions(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })
	leftConn, rightConn := net.Pipe()
	left := startSimOpenSide(t, leftConn, simOpenTable(t, 1, 11, 13))
	right := startSimOpenSide(t, rightConn, simOpenTable(t, 1, 14))
	var mismatch *handshake.VersionMismatchError
	require.True(t, errors.As(awaitSimOpen(t, left).err, &mismatch))
	require.Equal(t, []uint16{11, 13}, mismatch.SupportedVersions)
	require.True(t, errors.As(awaitSimOpen(t, right).err, &mismatch))
	require.Equal(t, []uint16{14}, mismatch.SupportedVersions)
}

// gatedConn holds its first write until release is closed.
type gatedConn struct {
	net.Conn
	writing chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *gatedConn) Write(b []byte) (int, error) {
	c.once.Do(func() {
		close(c.writing)
		<-c.release
	})
	return c.Conn.Write(b)
}

// TestSimultaneousOpenEndsOnlyAfterProposalWritten checks a side does not
// end the handshake while its own proposal is still unwritten. Ending stops
// the protocol and the muxer drops the unwritten proposal, so the peer never
// hears it and cannot make its own choice. The reference writes the proposal
// synchronously before it reads anything (Client.hs 117-121 with
// network-mux Mux/Types.hs 327-330), so it cannot end first.
func TestSimultaneousOpenEndsOnlyAfterProposalWritten(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })
	leftConn, rightConn := net.Pipe()
	gated := &gatedConn{
		Conn:    rightConn,
		writing: make(chan struct{}),
		release: make(chan struct{}),
	}
	left := startSimOpenSide(t, leftConn, simOpenTable(t, 1, 11, 13))
	right := startSimOpenSide(t, gated, simOpenTable(t, 1, 14))
	select {
	case <-gated.writing:
	case <-time.After(5 * time.Second):
		t.Fatal("right side never started writing its proposal")
	}
	// The right side now has the left proposal to read and handle, while its
	// own is held. A side that ends without waiting for its write does so
	// well within this window; one that waits cannot end until release.
	select {
	case result := <-right:
		close(gated.release)
		t.Fatalf("right side ended with its proposal unwritten: %v", result.err)
	case <-time.After(250 * time.Millisecond):
	}
	close(gated.release)
	var mismatch *handshake.VersionMismatchError
	require.True(t, errors.As(awaitSimOpen(t, left).err, &mismatch))
	require.Equal(t, []uint16{11, 13}, mismatch.SupportedVersions)
	require.True(t, errors.As(awaitSimOpen(t, right).err, &mismatch))
	require.Equal(t, []uint16{14}, mismatch.SupportedVersions)
}
