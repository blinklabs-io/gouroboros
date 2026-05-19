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
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// WithOriginSlot is the CBOR encoding of Haskell's WithOrigin SlotNo:
//
//	Origin    -> [0]
//	At slot   -> [1, slotNo]
//
// HasSlot is false when the snapshot was taken at chain origin (no slot
// reached yet); otherwise Slot holds the SlotNo at which the snapshot was
// taken.
type WithOriginSlot struct {
	HasSlot bool
	Slot    uint64
}

func (w *WithOriginSlot) UnmarshalCBOR(data []byte) error {
	listLen, err := cbor.ListLength(data)
	if err != nil {
		return err
	}
	tag, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	switch {
	case listLen == 1 && tag == 0:
		w.HasSlot = false
		w.Slot = 0
		return nil
	case listLen == 2 && tag == 1:
		var tmp struct {
			cbor.StructAsArray
			Tag  int
			Slot uint64
		}
		if _, err := cbor.Decode(data, &tmp); err != nil {
			return err
		}
		w.HasSlot = true
		w.Slot = tmp.Slot
		return nil
	default:
		return fmt.Errorf(
			"unexpected WithOriginSlot: tag=%d, length=%d",
			tag,
			listLen,
		)
	}
}

func (w WithOriginSlot) MarshalCBOR() ([]byte, error) {
	if !w.HasSlot {
		return cbor.Encode([]any{0})
	}
	return cbor.Encode([]any{1, w.Slot})
}

// RelayKind identifies the kind of relay access point in a LedgerPeerSnapshot.
// The integer values match the wire-format discriminator used by the Haskell
// Serialise instance for RelayAccessPoint.
type RelayKind int

const (
	RelayKindIPv4   RelayKind = 0 // [0, ipv4-uint32, port]
	RelayKindIPv6   RelayKind = 1 // [1, [w0,w1,w2,w3], port]
	RelayKindDomain RelayKind = 2 // [2, domain-bytes, port]
	RelayKindSRV    RelayKind = 3 // [3, domain-bytes]
)

// RelayAccessPoint is the Go representation of Haskell's RelayAccessPoint.
// Exactly one of IPv4, IPv6, or Domain is populated, selected by Kind:
//
//	RelayKindIPv4   -> IPv4 set, Port set
//	RelayKindIPv6   -> IPv6 set, Port set
//	RelayKindDomain -> Domain set, Port set
//	RelayKindSRV    -> Domain set, Port nil (resolver supplies port via SRV)
type RelayAccessPoint struct {
	Kind   RelayKind
	IPv4   *net.IP
	IPv6   *net.IP
	Domain *string
	Port   *uint16
}

func (r *RelayAccessPoint) UnmarshalCBOR(data []byte) error {
	listLen, err := cbor.ListLength(data)
	if err != nil {
		return err
	}
	kind, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	switch RelayKind(kind) {
	case RelayKindIPv4:
		if listLen != 3 {
			return fmt.Errorf(
				"RelayAccessPoint IPv4: expected list length 3, got %d",
				listLen,
			)
		}
		var tmp struct {
			cbor.StructAsArray
			Tag  int
			Addr cbor.RawMessage
			Port uint16
		}
		if _, err := cbor.Decode(data, &tmp); err != nil {
			return err
		}
		ip, err := decodeIPv4(tmp.Addr)
		if err != nil {
			return err
		}
		r.Kind = RelayKindIPv4
		r.IPv4 = &ip
		port := tmp.Port
		r.Port = &port
	case RelayKindIPv6:
		if listLen != 3 {
			return fmt.Errorf(
				"RelayAccessPoint IPv6: expected list length 3, got %d",
				listLen,
			)
		}
		var tmp struct {
			cbor.StructAsArray
			Tag  int
			Addr cbor.RawMessage
			Port uint16
		}
		if _, err := cbor.Decode(data, &tmp); err != nil {
			return err
		}
		ip, err := decodeIPv6(tmp.Addr)
		if err != nil {
			return err
		}
		r.Kind = RelayKindIPv6
		r.IPv6 = &ip
		port := tmp.Port
		r.Port = &port
	case RelayKindDomain:
		if listLen != 3 {
			return fmt.Errorf(
				"RelayAccessPoint Domain: expected list length 3, got %d",
				listLen,
			)
		}
		var tmp struct {
			cbor.StructAsArray
			Tag    int
			Domain []byte
			Port   uint16
		}
		if _, err := cbor.Decode(data, &tmp); err != nil {
			return err
		}
		dn := string(tmp.Domain)
		r.Kind = RelayKindDomain
		r.Domain = &dn
		port := tmp.Port
		r.Port = &port
	case RelayKindSRV:
		if listLen != 2 {
			return fmt.Errorf(
				"RelayAccessPoint SRV: expected list length 2, got %d",
				listLen,
			)
		}
		var tmp struct {
			cbor.StructAsArray
			Tag    int
			Domain []byte
		}
		if _, err := cbor.Decode(data, &tmp); err != nil {
			return err
		}
		dn := string(tmp.Domain)
		r.Kind = RelayKindSRV
		r.Domain = &dn
	default:
		return fmt.Errorf("unknown relay kind: %d", kind)
	}
	return nil
}

func (r RelayAccessPoint) MarshalCBOR() ([]byte, error) {
	switch r.Kind {
	case RelayKindIPv4:
		if r.IPv4 == nil || r.Port == nil {
			return nil, errors.New(
				"RelayAccessPoint IPv4 requires IPv4 and Port",
			)
		}
		ipv4 := r.IPv4.To4()
		if ipv4 == nil {
			return nil, errors.New(
				"RelayAccessPoint IPv4: not a valid IPv4 address",
			)
		}
		addr := binary.BigEndian.Uint32(ipv4)
		return cbor.Encode([]any{int(RelayKindIPv4), addr, *r.Port})
	case RelayKindIPv6:
		if r.IPv6 == nil || r.Port == nil {
			return nil, errors.New(
				"RelayAccessPoint IPv6 requires IPv6 and Port",
			)
		}
		ipv6 := r.IPv6.To16()
		if ipv6 == nil {
			return nil, errors.New(
				"RelayAccessPoint IPv6: not a valid IPv6 address",
			)
		}
		// Haskell encodes IPv6 as a 4-tuple of Word32 (toHostAddress6).
		a := binary.BigEndian.Uint32(ipv6[0:4])
		b := binary.BigEndian.Uint32(ipv6[4:8])
		c := binary.BigEndian.Uint32(ipv6[8:12])
		d := binary.BigEndian.Uint32(ipv6[12:16])
		return cbor.Encode(
			[]any{int(RelayKindIPv6), []uint32{a, b, c, d}, *r.Port},
		)
	case RelayKindDomain:
		if r.Domain == nil || r.Port == nil {
			return nil, errors.New(
				"RelayAccessPoint Domain requires Domain and Port",
			)
		}
		return cbor.Encode(
			[]any{int(RelayKindDomain), []byte(*r.Domain), *r.Port},
		)
	case RelayKindSRV:
		if r.Domain == nil {
			return nil, errors.New("RelayAccessPoint SRV requires Domain")
		}
		return cbor.Encode([]any{int(RelayKindSRV), []byte(*r.Domain)})
	}
	return nil, fmt.Errorf("unknown relay kind: %d", r.Kind)
}

// decodeIPv4 accepts either the canonical Haskell encoding (CBOR uint of the
// host address in network byte order) or a raw 4-byte bytestring fallback.
func decodeIPv4(data []byte) (net.IP, error) {
	var word uint32
	if _, err := cbor.Decode(data, &word); err == nil {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, word)
		return net.IP(b), nil
	}
	var bs []byte
	if _, err := cbor.Decode(data, &bs); err == nil {
		if len(bs) != 4 {
			return nil, fmt.Errorf(
				"IPv4 bytestring must be 4 bytes, got %d",
				len(bs),
			)
		}
		return net.IP(bs), nil
	}
	return nil, errors.New("invalid IPv4 encoding")
}

// decodeIPv6 accepts either the canonical Haskell encoding (4-element list
// of Word32 from toHostAddress6) or a raw 16-byte bytestring fallback.
func decodeIPv6(data []byte) (net.IP, error) {
	var tuple []uint32
	if _, err := cbor.Decode(data, &tuple); err == nil && len(tuple) == 4 {
		b := make([]byte, 16)
		binary.BigEndian.PutUint32(b[0:4], tuple[0])
		binary.BigEndian.PutUint32(b[4:8], tuple[1])
		binary.BigEndian.PutUint32(b[8:12], tuple[2])
		binary.BigEndian.PutUint32(b[12:16], tuple[3])
		return net.IP(b), nil
	}
	var bs []byte
	if _, err := cbor.Decode(data, &bs); err == nil {
		if len(bs) != 16 {
			return nil, fmt.Errorf(
				"IPv6 bytestring must be 16 bytes, got %d",
				len(bs),
			)
		}
		return net.IP(bs), nil
	}
	return nil, errors.New("invalid IPv6 encoding")
}

// PoolLedgerPeers is a single pool's entry in a LedgerPeerSnapshot. The
// Haskell type is (AccPoolStake, (PoolStake, NonEmpty RelayAccessPoint))
// which serialises as a 2-element array containing a nested 2-element array.
//
// AccumulatedStake is the cumulative stake fraction of all preceding pools
// plus this one (used by the peer-selection bisection algorithm).
// PoolStake is this pool's own stake fraction. Relays is the non-empty
// list of ledger-advertised relay endpoints.
//
// Note: LedgerPeerSnapshot v1 does not carry a pool identity (PoolKeyHash);
// it is a stake-and-relays summary only.
type PoolLedgerPeers struct {
	cbor.StructAsArray
	AccumulatedStake *cbor.Rat
	Detail           PoolLedgerPeersDetail
}

// PoolLedgerPeersDetail is the inner (PoolStake, NonEmpty RelayAccessPoint)
// pair of a PoolLedgerPeers entry.
type PoolLedgerPeersDetail struct {
	cbor.StructAsArray
	PoolStake *cbor.Rat
	Relays    []RelayAccessPoint
}

// LedgerPeerSnapshotResult is the typed result of a GetLedgerPeerSnapshot
// query.
//
// The wire layout is a versioned tagged 2-element array:
//
//	[version, [WithOriginSlot, [PoolLedgerPeers ...]]]
//
// Version 0 corresponds to LedgerPeerSnapshotV1 in ouroboros-network.
// Unknown versions return a decode error so callers do not silently lose
// fields that a newer node may have added.
type LedgerPeerSnapshotResult struct {
	Version uint64
	Slot    WithOriginSlot
	Pools   []PoolLedgerPeers
}

func (l *LedgerPeerSnapshotResult) UnmarshalCBOR(data []byte) error {
	var outer struct {
		cbor.StructAsArray
		Version uint64
		Inner   struct {
			cbor.StructAsArray
			Slot  WithOriginSlot
			Pools []PoolLedgerPeers
		}
	}
	if _, err := cbor.Decode(data, &outer); err != nil {
		return err
	}
	if outer.Version != 0 {
		return fmt.Errorf(
			"unsupported LedgerPeerSnapshot version: %d (expected 0/V1)",
			outer.Version,
		)
	}
	l.Version = outer.Version
	l.Slot = outer.Inner.Slot
	l.Pools = outer.Inner.Pools
	return nil
}

func (l LedgerPeerSnapshotResult) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(struct {
		cbor.StructAsArray
		Version uint64
		Inner   struct {
			cbor.StructAsArray
			Slot  WithOriginSlot
			Pools []PoolLedgerPeers
		}
	}{
		Version: l.Version,
		Inner: struct {
			cbor.StructAsArray
			Slot  WithOriginSlot
			Pools []PoolLedgerPeers
		}{
			Slot:  l.Slot,
			Pools: l.Pools,
		},
	})
}
