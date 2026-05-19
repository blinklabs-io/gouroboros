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
	"encoding/hex"
	"errors"
	"math/big"
	"net"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// helper: build a *cbor.Rat from int64 num/denom.
func newRat(num, denom int64) *cbor.Rat {
	return &cbor.Rat{Rat: big.NewRat(num, denom)}
}

func ptrUint16(v uint16) *uint16 { return &v }
func ptrIP(ip net.IP) *net.IP    { return &ip }
func ptrStr(s string) *string    { return &s }

// TestShelleyGetLedgerPeerSnapshotQueryEncode verifies that the inner
// leaf query encodes as [34, peerKind] per the v15+ wire format documented
// in ouroboros-consensus.
func TestShelleyGetLedgerPeerSnapshotQueryEncode(t *testing.T) {
	q := ShelleyGetLedgerPeerSnapshotQuery{
		Type:     QueryTypeShelleyGetLedgerPeerSnapshot,
		PeerKind: LedgerPeerKindAll,
	}
	data, err := cbor.Encode(q)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	// list-2 + uint8(34) + 0  =>  82 18 22 00
	want := "82182200"
	got := hex.EncodeToString(data)
	if got != want {
		t.Fatalf("encoded query mismatch:\n  got:  %s\n  want: %s", got, want)
	}
}

// TestShelleyGetLedgerPeerSnapshotQueryBuildShelleyQuery verifies that
// wrapping the leaf through buildShelleyQuery produces the full
// BlockQuery → ShelleyQuery → era → [34, peerKind] envelope expected on
// the wire.
func TestShelleyGetLedgerPeerSnapshotQueryBuildShelleyQuery(t *testing.T) {
	const era = 6 // Conway
	q := buildShelleyQuery(
		era,
		QueryTypeShelleyGetLedgerPeerSnapshot,
		int(LedgerPeerKindAll),
	)
	data, err := cbor.Encode(q)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	// [0, [0, [6, [34, 0]]]]  =>  82 00 82 00 82 06 82 18 22 00
	want := "82008200820682182200"
	got := hex.EncodeToString(data)
	if got != want {
		t.Fatalf("built query mismatch:\n  got:  %s\n  want: %s", got, want)
	}
}

func TestWithOriginSlotRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		val  WithOriginSlot
		hex  string
	}{
		{"origin", WithOriginSlot{HasSlot: false}, "8100"},
		{"at_slot_12345", WithOriginSlot{HasSlot: true, Slot: 12345}, "8201193039"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := cbor.Encode(tc.val)
			if err != nil {
				t.Fatalf("encode: %s", err)
			}
			if got := hex.EncodeToString(data); got != tc.hex {
				t.Fatalf(
					"encode mismatch:\n  got:  %s\n  want: %s",
					got,
					tc.hex,
				)
			}
			var decoded WithOriginSlot
			if _, err := cbor.Decode(data, &decoded); err != nil {
				t.Fatalf("decode: %s", err)
			}
			if !reflect.DeepEqual(decoded, tc.val) {
				t.Fatalf(
					"round-trip mismatch:\n  got:  %#v\n  want: %#v",
					decoded,
					tc.val,
				)
			}
		})
	}
}

func TestRelayAccessPointRoundTrip(t *testing.T) {
	ipv4 := net.IPv4(192, 0, 2, 1).To4()
	ipv6 := net.ParseIP("2001:db8::1")
	cases := []struct {
		name string
		val  RelayAccessPoint
		hex  string
	}{
		{
			name: "ipv4",
			val: RelayAccessPoint{
				Kind: RelayKindIPv4,
				IPv4: ptrIP(ipv4),
				Port: ptrUint16(3001),
			},
			// list-3 + 0 + uint32(0xC0000201) + uint16(0x0BB9)
			hex: "83001ac0000201190bb9",
		},
		{
			name: "domain",
			val: RelayAccessPoint{
				Kind:   RelayKindDomain,
				Domain: ptrStr("relays.example"),
				Port:   ptrUint16(3001),
			},
			// list-3 + 2 + bstr"relays.example" + uint16(0x0BB9)
			// 4e = bytestring of length 14
			hex: "8302" +
				"4e" + "72656c6179732e6578616d706c65" + // "relays.example"
				"190bb9",
		},
		{
			name: "srv",
			val: RelayAccessPoint{
				Kind:   RelayKindSRV,
				Domain: ptrStr("_cardano._tcp.example"),
			},
			// list-2 + 3 + bstr"_cardano._tcp.example"
			// 55 = bytestring of length 21
			hex: "8203" +
				"55" + "5f63617264616e6f2e5f7463702e6578616d706c65",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := cbor.Encode(tc.val)
			if err != nil {
				t.Fatalf("encode: %s", err)
			}
			if got := hex.EncodeToString(data); got != tc.hex {
				t.Fatalf(
					"encode mismatch:\n  got:  %s\n  want: %s",
					got,
					tc.hex,
				)
			}
			var decoded RelayAccessPoint
			if _, err := cbor.Decode(data, &decoded); err != nil {
				t.Fatalf("decode: %s", err)
			}
			if decoded.Kind != tc.val.Kind {
				t.Fatalf("kind: got %d want %d", decoded.Kind, tc.val.Kind)
			}
			switch tc.val.Kind {
			case RelayKindIPv4:
				if !decoded.IPv4.Equal(*tc.val.IPv4) {
					t.Fatalf(
						"ipv4: got %s want %s",
						*decoded.IPv4,
						*tc.val.IPv4,
					)
				}
				if *decoded.Port != *tc.val.Port {
					t.Fatalf(
						"port: got %d want %d",
						*decoded.Port,
						*tc.val.Port,
					)
				}
			case RelayKindDomain:
				if *decoded.Domain != *tc.val.Domain {
					t.Fatalf(
						"domain: got %s want %s",
						*decoded.Domain,
						*tc.val.Domain,
					)
				}
				if *decoded.Port != *tc.val.Port {
					t.Fatalf("port mismatch")
				}
			case RelayKindSRV:
				if *decoded.Domain != *tc.val.Domain {
					t.Fatalf("srv domain mismatch")
				}
				if decoded.Port != nil {
					t.Fatalf("srv must have nil port, got %d", *decoded.Port)
				}
			}
		})
	}

	// IPv6 round-trip is checked separately because reflect.DeepEqual on
	// net.IP byte slices is brittle when To16() reshapes a 4-in-6 form.
	t.Run("ipv6", func(t *testing.T) {
		val := RelayAccessPoint{
			Kind: RelayKindIPv6,
			IPv6: ptrIP(ipv6),
			Port: ptrUint16(3001),
		}
		data, err := cbor.Encode(val)
		if err != nil {
			t.Fatalf("encode: %s", err)
		}
		var decoded RelayAccessPoint
		if _, err := cbor.Decode(data, &decoded); err != nil {
			t.Fatalf("decode: %s", err)
		}
		if decoded.Kind != RelayKindIPv6 {
			t.Fatalf("kind: got %d", decoded.Kind)
		}
		if !decoded.IPv6.Equal(ipv6) {
			t.Fatalf("ipv6 mismatch: got %s want %s", *decoded.IPv6, ipv6)
		}
		if *decoded.Port != 3001 {
			t.Fatalf("port mismatch: got %d", *decoded.Port)
		}
	})
}

// TestLedgerPeerSnapshotGolden decodes a hand-rolled wire-format snapshot
// (the canonical Haskell LedgerPeerSnapshotV1 layout) and verifies that
// every typed field is preserved.
func TestLedgerPeerSnapshotGolden(t *testing.T) {
	// Canonical encoding of a LedgerPeerSnapshotV1 with:
	//   slot = At 12345
	//   one pool: accStake=1/10, poolStake=1/10,
	//             one IPv4 relay 192.0.2.1:3001
	//
	// Layout:
	//   82 00                       version=0 (V1)
	//   82                          inner [slot, pools]
	//     82 01 19 3039             slot = At 12345
	//     81                        pools = list-1
	//       82                      pool entry
	//         d8 1e 82 01 0a        acc stake = tag30 [1, 10]
	//         82                    detail
	//           d8 1e 82 01 0a      pool stake = tag30 [1, 10]
	//           81                  relays = list-1
	//             83 00             IPv4 entry, kind=0
	//               1a c0 00 02 01    uint32 192.0.2.1
	//               19 0b b9          uint16 3001
	wire, err := hex.DecodeString(
		"82008282011930398182d81e82010a82d81e82010a8183001ac0000201190bb9",
	)
	if err != nil {
		t.Fatalf("hex: %s", err)
	}
	var snap LedgerPeerSnapshotResult
	if _, err := cbor.Decode(wire, &snap); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if snap.Version != 0 {
		t.Fatalf("version: got %d want 0", snap.Version)
	}
	if !snap.Slot.HasSlot {
		t.Fatal("expected HasSlot=true")
	}
	if snap.Slot.Slot != 12345 {
		t.Fatalf("slot: got %d want 12345", snap.Slot.Slot)
	}
	if len(snap.Pools) != 1 {
		t.Fatalf("pools: got %d want 1", len(snap.Pools))
	}
	pool := snap.Pools[0]
	if pool.AccumulatedStake.Cmp(big.NewRat(1, 10)) != 0 {
		t.Fatalf(
			"acc stake: got %s want 1/10",
			pool.AccumulatedStake.RatString(),
		)
	}
	if pool.Detail.PoolStake.Cmp(big.NewRat(1, 10)) != 0 {
		t.Fatalf(
			"pool stake: got %s want 1/10",
			pool.Detail.PoolStake.RatString(),
		)
	}
	if len(pool.Detail.Relays) != 1 {
		t.Fatalf("relays: got %d want 1", len(pool.Detail.Relays))
	}
	relay := pool.Detail.Relays[0]
	if relay.Kind != RelayKindIPv4 {
		t.Fatalf("relay kind: got %d want %d", relay.Kind, RelayKindIPv4)
	}
	want4 := net.IPv4(192, 0, 2, 1).To4()
	if !relay.IPv4.Equal(want4) {
		t.Fatalf("relay ipv4: got %s want %s", *relay.IPv4, want4)
	}
	if *relay.Port != 3001 {
		t.Fatalf("relay port: got %d want 3001", *relay.Port)
	}
}

// TestLedgerPeerSnapshotRejectsUnknownVersion ensures a forward-incompatible
// snapshot version is reported as a decode error rather than silently
// dropping fields.
func TestLedgerPeerSnapshotRejectsUnknownVersion(t *testing.T) {
	// [1, [[0], []]]  -- version 1 (unknown), origin, no pools
	wire, err := hex.DecodeString("82018281008080")
	if err != nil {
		t.Fatalf("hex: %s", err)
	}
	var snap LedgerPeerSnapshotResult
	if _, err := cbor.Decode(wire, &snap); err == nil {
		t.Fatal("expected decode error for unknown version, got nil")
	}
}

// newClientWithVersion is a test-only helper that constructs a Client
// without starting its underlying protocol goroutines. It exists so the
// version-gating logic can be exercised in isolation, without paying the
// cost of a full ouroboros-mock handshake conversation.
func newClientWithVersion(version uint16) *Client {
	opts := protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Version: version,
	}
	return NewClient(opts, nil)
}

// TestGetLedgerPeerSnapshotVersionGate verifies the NtC v19 boundary:
// versions below 19 must surface ErrLedgerPeerSnapshotUnsupportedVersion
// without any wire activity; v19+ flips the gate on so the call would
// proceed to send a query (callers above 19 are exercised via integration
// tests against a real node — see TestGetLedgerPeerSnapshotIntegration).
func TestGetLedgerPeerSnapshotVersionGate(t *testing.T) {
	cases := []struct {
		name    string
		ntcVer  uint16
		enabled bool
	}{
		{"ntc_v18", 18 + protocol.ProtocolVersionNtCOffset, false},
		{"ntc_v19", 19 + protocol.ProtocolVersionNtCOffset, true},
		{"ntc_v20", 20 + protocol.ProtocolVersionNtCOffset, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newClientWithVersion(tc.ntcVer)
			if c.enableGetLedgerPeerSnapshot != tc.enabled {
				t.Fatalf(
					"version %d: enableGetLedgerPeerSnapshot=%v want %v",
					tc.ntcVer,
					c.enableGetLedgerPeerSnapshot,
					tc.enabled,
				)
			}
			if !tc.enabled {
				_, err := c.GetLedgerPeerSnapshot(LedgerPeerKindAll)
				if !errors.Is(
					err,
					ErrLedgerPeerSnapshotUnsupportedVersion,
				) {
					t.Fatalf("expected unsupported-version error, got %v", err)
				}
			}
		})
	}
}

// TestGetLedgerPeerSnapshotRejectsInvalidPeerKind verifies the early-input
// validation: calling with an enum value outside {All, Big} must return an
// error without sending any wire traffic. The check sits before the busy
// mutex so a bogus call from a misbehaving caller cannot stall the protocol.
func TestGetLedgerPeerSnapshotRejectsInvalidPeerKind(t *testing.T) {
	c := newClientWithVersion(19 + protocol.ProtocolVersionNtCOffset)
	_, err := c.GetLedgerPeerSnapshot(LedgerPeerKind(7))
	if err == nil {
		t.Fatal("expected error for invalid peer kind, got nil")
	}
	if errors.Is(err, ErrLedgerPeerSnapshotUnsupportedVersion) {
		t.Fatalf(
			"got version error, expected invalid-peerKind error: %v",
			err,
		)
	}
}

// TestRelayAccessPointResetsVariantFields ensures UnmarshalCBOR clears
// stale variant fields when decoding into a previously-populated value.
// Without the reset, decoding SRV into a struct that previously held an
// IPv4 relay would leak the old Port and an old IPv4 pointer.
func TestRelayAccessPointResetsVariantFields(t *testing.T) {
	ipv4 := net.IPv4(192, 0, 2, 1).To4()
	var r RelayAccessPoint
	first, err := cbor.Encode(RelayAccessPoint{
		Kind: RelayKindIPv4,
		IPv4: ptrIP(ipv4),
		Port: ptrUint16(3001),
	})
	if err != nil {
		t.Fatalf("encode ipv4: %s", err)
	}
	if _, err := cbor.Decode(first, &r); err != nil {
		t.Fatalf("decode ipv4: %s", err)
	}
	// Now decode an SRV payload into the same struct.
	second, err := cbor.Encode(RelayAccessPoint{
		Kind:   RelayKindSRV,
		Domain: ptrStr("_cardano._tcp.example"),
	})
	if err != nil {
		t.Fatalf("encode srv: %s", err)
	}
	if _, err := cbor.Decode(second, &r); err != nil {
		t.Fatalf("decode srv: %s", err)
	}
	if r.Kind != RelayKindSRV {
		t.Fatalf("kind: got %d want SRV", r.Kind)
	}
	if r.IPv4 != nil {
		t.Fatalf("stale IPv4 leaked into SRV decode")
	}
	if r.IPv6 != nil {
		t.Fatalf("stale IPv6 leaked into SRV decode")
	}
	if r.Port != nil {
		t.Fatalf("stale Port leaked into SRV decode")
	}
}

// TestLedgerPeerSnapshotRoundTrip verifies encode/decode symmetry on a
// snapshot constructed in Go.
func TestLedgerPeerSnapshotRoundTrip(t *testing.T) {
	ipv4 := net.IPv4(192, 0, 2, 1).To4()
	original := LedgerPeerSnapshotResult{
		Version: 0,
		Slot:    WithOriginSlot{HasSlot: true, Slot: 99},
		Pools: []PoolLedgerPeers{
			{
				AccumulatedStake: newRat(1, 4),
				Detail: PoolLedgerPeersDetail{
					PoolStake: newRat(1, 4),
					Relays: []RelayAccessPoint{
						{
							Kind: RelayKindIPv4,
							IPv4: ptrIP(ipv4),
							Port: ptrUint16(3001),
						},
						{
							Kind:   RelayKindDomain,
							Domain: ptrStr("relays.example"),
							Port:   ptrUint16(3001),
						},
					},
				},
			},
		},
	}
	data, err := cbor.Encode(original)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	var decoded LedgerPeerSnapshotResult
	if _, err := cbor.Decode(data, &decoded); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if decoded.Version != original.Version {
		t.Fatalf("version differs")
	}
	if decoded.Slot != original.Slot {
		t.Fatalf("slot differs: got %#v want %#v", decoded.Slot, original.Slot)
	}
	if len(decoded.Pools) != 1 {
		t.Fatalf("pools length: got %d", len(decoded.Pools))
	}
	if decoded.Pools[0].AccumulatedStake.Cmp(big.NewRat(1, 4)) != 0 {
		t.Fatalf("acc stake")
	}
	if decoded.Pools[0].Detail.PoolStake.Cmp(big.NewRat(1, 4)) != 0 {
		t.Fatalf("pool stake")
	}
	if len(decoded.Pools[0].Detail.Relays) != 2 {
		t.Fatalf("relays length: got %d", len(decoded.Pools[0].Detail.Relays))
	}
}
