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

//go:build peersharing_interop

// Package peersharinginterop verifies gouroboros against a real Haskell
// cardano-node, focused on the peer-sharing mini-protocol (N2N protocol 10).
//
// The Docker Compose harness in this directory brings up two cardano-node
// containers on the preview network with identical configs except for the
// PeerSharing flag:
//
//	cardano-shares-on   PeerSharing=true   PEERSHARING_SHARES_ON_ADDR
//	cardano-shares-off  PeerSharing=false  PEERSHARING_SHARES_OFF_ADDR
//
// The tests connect with the Node-to-Node handshake and assert that the
// gating implemented by `peersharing.Client` matches what the remote
// advertised — both directions:
//
//   - Remote advertises PeerSharing: handshake reports PeerSharing()==true,
//     `Client.GetPeers` succeeds (the returned slice may be empty if the
//     remote has no peers to share yet, which is still spec-compliant).
//
//   - Remote advertises NoPeerSharing: handshake reports PeerSharing()==false,
//     `Client.GetPeers` fails with `peersharing.ErrRemotePeerSharingDisabled`
//     without sending any bytes.
//
// Run with: ./run-tests.sh
package peersharinginterop

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
)

// previewNetworkMagic is the well-known network magic of the preview testnet.
// Both docker-compose nodes are configured against preview, and the handshake
// will fail with a magic mismatch if this is wrong.
const previewNetworkMagic = 2

func envAddr(t *testing.T, key, fallback string) string {
	t.Helper()
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// dialNode opens a TCP connection to the given address and performs the
// Ouroboros NtN handshake with the supplied local peer-sharing preference.
// The returned connection has the peer-sharing mini-protocol available iff
// the handshake negotiated version 11+.
func dialNode(
	t *testing.T,
	addr string,
	localPeerSharing bool,
) *ouroboros.Connection {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		t.Fatalf("tcp dial %s: %v", addr, err)
	}

	oConn, err := ouroboros.NewConnection(
		ouroboros.WithConnection(conn),
		ouroboros.WithNetworkMagic(previewNetworkMagic),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithPeerSharing(localPeerSharing),
	)
	if err != nil {
		_ = conn.Close()
		t.Fatalf("ouroboros handshake with %s: %v", addr, err)
	}
	t.Cleanup(func() {
		_ = oConn.Close()
	})
	return oConn
}

// TestRemoteAdvertisesPeerSharing connects to the cardano-shares-on node and
// verifies that:
//  1. The negotiated handshake-version data reports PeerSharing()==true.
//  2. The Client.GetPeers call succeeds — the slice may be empty if the node
//     has not yet learned of any peers, but it must not error.
func TestRemoteAdvertisesPeerSharing(t *testing.T) {
	addr := envAddr(t, "PEERSHARING_SHARES_ON_ADDR", "localhost:3010")
	oConn := dialNode(t, addr, true)

	version, vd := oConn.ProtocolVersion()
	t.Logf("handshake: negotiated NtN version %d, peer-sharing=%v", version, vd.PeerSharing())
	if !vd.PeerSharing() {
		t.Fatalf(
			"shares-on node should advertise PeerSharing, "+
				"but handshake VersionData reports PeerSharing()=false (version=%d)",
			version,
		)
	}

	ps := oConn.PeerSharing()
	if ps == nil {
		t.Fatalf(
			"peer-sharing mini-protocol unavailable; "+
				"negotiated NtN version %d is below 11", version,
		)
	}

	peers, err := ps.Client.GetPeers(5)
	if err != nil {
		t.Fatalf("Client.GetPeers against shares-on: %v", err)
	}
	t.Logf("Client.GetPeers returned %d peer(s)", len(peers))
	for i, p := range peers {
		t.Logf("  peer[%d] = %s:%d", i, p.IP, p.Port)
	}
}

// TestRemoteDisablesPeerSharing connects to the cardano-shares-off node and
// verifies that:
//  1. The handshake completes successfully (a NoPeerSharing peer is still a
//     valid peer; only the protocol's traffic is forbidden).
//  2. The version data reports PeerSharing()==false.
//  3. Client.GetPeers short-circuits with ErrRemotePeerSharingDisabled — i.e.
//     gouroboros refuses to send ShareRequest to a peer that advertised
//     NoPeerSharing, which is exactly the spec-mandated behaviour and the
//     bidirectional-gating acceptance criterion of issue #1703.
func TestRemoteDisablesPeerSharing(t *testing.T) {
	addr := envAddr(t, "PEERSHARING_SHARES_OFF_ADDR", "localhost:3011")
	oConn := dialNode(t, addr, true)

	version, vd := oConn.ProtocolVersion()
	t.Logf("handshake: negotiated NtN version %d, peer-sharing=%v", version, vd.PeerSharing())
	if vd.PeerSharing() {
		t.Fatalf(
			"shares-off node should advertise NoPeerSharing, "+
				"but handshake VersionData reports PeerSharing()=true (version=%d)",
			version,
		)
	}

	ps := oConn.PeerSharing()
	if ps == nil {
		t.Fatalf(
			"peer-sharing mini-protocol unavailable; "+
				"negotiated NtN version %d is below 11", version,
		)
	}

	peers, err := ps.Client.GetPeers(5)
	if !errors.Is(err, peersharing.ErrRemotePeerSharingDisabled) {
		t.Fatalf(
			"expected ErrRemotePeerSharingDisabled when remote advertised "+
				"NoPeerSharing, got peers=%v err=%v",
			peers, err,
		)
	}
	if peers != nil {
		t.Fatalf("expected nil peer slice on refused request, got: %#v", peers)
	}
}
