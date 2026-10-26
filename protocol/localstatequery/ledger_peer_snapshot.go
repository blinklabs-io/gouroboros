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
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// Internal wire versions of a LedgerPeerSnapshot result. Version 1 answers
// the node-to-client version 19 to 22 request; versions 2 (big ledger peers)
// and 3 (all ledger peers) answer the version 23 request, and add the
// snapshot's block hash and network magic.
const (
	ledgerPeerSnapshotWireVersion    = 1
	ledgerPeerSnapshotWireVersionBig = 2
	ledgerPeerSnapshotWireVersionAll = 3
)

// ledgerPeerSnapshotBlockHashSize is the size of a Cardano header hash. The
// snapshot carries it as raw bytes with no length of its own.
const ledgerPeerSnapshotBlockHashSize = 32

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
// The integer values are the constructor tags of ouroboros-network's
// LedgerRelayAccessPoint CBOR encoding.
type RelayKind int

const (
	RelayKindDomain RelayKind = 0 // [0, port, domain-bytes]
	RelayKindIPv4   RelayKind = 1 // [1, port, [o0, o1, o2, o3]]
	RelayKindIPv6   RelayKind = 2 // [2, port, [w0, ..., w7]]
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
	// Reset variant fields so decoding into a reused RelayAccessPoint
	// cannot leak stale values from a previous decode (e.g. SRV inheriting
	// a Port from a prior IPv4 payload).
	r.IPv4 = nil
	r.IPv6 = nil
	r.Domain = nil
	r.Port = nil
	fields, err := decodeBoundedArray(data, 3, "RelayAccessPoint")
	if err != nil {
		return err
	}
	if len(fields) == 0 {
		return errors.New("RelayAccessPoint: empty list")
	}
	var kind uint8
	if _, err := cbor.Decode(fields[0], &kind); err != nil {
		return fmt.Errorf("RelayAccessPoint: kind: %w", err)
	}
	wantLen := 3
	if RelayKind(kind) == RelayKindSRV {
		wantLen = 2
	}
	switch RelayKind(kind) {
	case RelayKindDomain, RelayKindIPv4, RelayKindIPv6, RelayKindSRV:
	default:
		return fmt.Errorf("unknown relay kind: %d", kind)
	}
	if len(fields) != wantLen {
		return fmt.Errorf(
			"RelayAccessPoint kind %d: expected list length %d, got %d",
			kind,
			wantLen,
			len(fields),
		)
	}
	if RelayKind(kind) == RelayKindSRV {
		var domain []byte
		if _, err := cbor.Decode(fields[1], &domain); err != nil {
			return fmt.Errorf("RelayAccessPoint SRV: %w", err)
		}
		dn := string(domain)
		r.Kind = RelayKindSRV
		r.Domain = &dn
		return nil
	}
	var port uint16
	if _, err := cbor.Decode(fields[1], &port); err != nil {
		return fmt.Errorf("RelayAccessPoint: port: %w", err)
	}
	switch RelayKind(kind) {
	case RelayKindDomain:
		var domain []byte
		if _, err := cbor.Decode(fields[2], &domain); err != nil {
			return fmt.Errorf("RelayAccessPoint Domain: %w", err)
		}
		dn := string(domain)
		r.Domain = &dn
	case RelayKindIPv4:
		ip, err := decodeIPWords(fields[2], 4, 8)
		if err != nil {
			return fmt.Errorf("RelayAccessPoint IPv4: %w", err)
		}
		r.IPv4 = &ip
	case RelayKindIPv6:
		ip, err := decodeIPWords(fields[2], 8, 16)
		if err != nil {
			return fmt.Errorf("RelayAccessPoint IPv6: %w", err)
		}
		r.IPv6 = &ip
	case RelayKindSRV:
		// Handled above: an SRV relay carries no port.
	}
	r.Kind = RelayKind(kind)
	r.Port = &port
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
		return encodeRelay(r.Kind, *r.Port, encodeIPWords(ipv4, 8))
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
		return encodeRelay(r.Kind, *r.Port, encodeIPWords(ipv6, 16))
	case RelayKindDomain:
		if r.Domain == nil || r.Port == nil {
			return nil, errors.New(
				"RelayAccessPoint Domain requires Domain and Port",
			)
		}
		domain, err := cbor.Encode([]byte(*r.Domain))
		if err != nil {
			return nil, err
		}
		return encodeRelay(r.Kind, *r.Port, domain)
	case RelayKindSRV:
		if r.Domain == nil {
			return nil, errors.New("RelayAccessPoint SRV requires Domain")
		}
		return cbor.Encode([]any{int(RelayKindSRV), []byte(*r.Domain)})
	}
	return nil, fmt.Errorf("unknown relay kind: %d", r.Kind)
}

func encodeRelay(kind RelayKind, port uint16, addr []byte) ([]byte, error) {
	return cbor.Encode(
		[]any{int(kind), port, cbor.RawMessage(addr)},
	)
}

// decodeIPWords decodes the address list ouroboros-network writes with
// iproute's fromIPv4/fromIPv6: 4 octets for IPv4, 8 big-endian 16-bit words
// for IPv6. Decoding into the word type rejects an out-of-range word.
func decodeIPWords(data []byte, count int, bits uint) (net.IP, error) {
	var ip net.IP
	var n int
	if err := boundArray(data, count, "address words"); err != nil {
		return nil, err
	}
	if bits == 8 {
		var octets []uint8
		if _, err := cbor.Decode(data, &octets); err != nil {
			return nil, err
		}
		n = len(octets)
		ip = net.IP(octets)
	} else {
		var words []uint16
		if _, err := cbor.Decode(data, &words); err != nil {
			return nil, err
		}
		n = len(words)
		for _, w := range words {
			ip = binary.BigEndian.AppendUint16(ip, w)
		}
	}
	if n != count {
		return nil, fmt.Errorf(
			"expected %d address words, got %d",
			count,
			n,
		)
	}
	return ip, nil
}

// encodeIPWords is the inverse of decodeIPWords. The list is
// indefinite-length because cardano-binary's list decoder accepts nothing
// else.
func encodeIPWords(ip net.IP, bits uint) []byte {
	size := int(bits / 8)
	items := make([][]byte, 0, len(ip)/size)
	for i := 0; i < len(ip); i += size {
		var w uint64
		if size == 1 {
			w = uint64(ip[i])
		} else {
			w = uint64(binary.BigEndian.Uint16(ip[i : i+2]))
		}
		// A small unsigned integer encodes infallibly.
		item, _ := cbor.Encode(w)
		items = append(items, item)
	}
	return encodeIndefiniteList(items)
}

// encodeIndefiniteList joins already-encoded items into an
// indefinite-length CBOR list.
func encodeIndefiniteList(items [][]byte) []byte {
	buf := bytes.NewBuffer([]byte{0x9f})
	for _, item := range items {
		buf.Write(item)
	}
	buf.WriteByte(cborBreak)
	return buf.Bytes()
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
// Note: LedgerPeerSnapshotV2 does not carry a pool identity (PoolKeyHash);
// it is a stake-and-relays summary only.
type PoolLedgerPeers struct {
	AccumulatedStake *cbor.Rat
	Detail           PoolLedgerPeersDetail
}

func (p *PoolLedgerPeers) UnmarshalCBOR(data []byte) error {
	fields, err := decodeBoundedArray(data, 2, "PoolLedgerPeers")
	if err != nil {
		return err
	}
	if len(fields) != 2 {
		return fmt.Errorf(
			"PoolLedgerPeers: expected 2 elements, got %d",
			len(fields),
		)
	}
	acc, err := decodeRational(fields[0])
	if err != nil {
		return fmt.Errorf("PoolLedgerPeers: accumulated stake: %w", err)
	}
	var detail PoolLedgerPeersDetail
	if _, err := cbor.Decode(fields[1], &detail); err != nil {
		return err
	}
	p.AccumulatedStake = &cbor.Rat{Rat: acc}
	p.Detail = detail
	return nil
}

func (p PoolLedgerPeers) MarshalCBOR() ([]byte, error) {
	acc, err := encodeStake(p.AccumulatedStake)
	if err != nil {
		return nil, err
	}
	detail, err := cbor.Encode(p.Detail)
	if err != nil {
		return nil, err
	}
	return cbor.Encode(
		[]cbor.RawMessage{acc, detail},
	)
}

// PoolLedgerPeersDetail is the inner (PoolStake, NonEmpty RelayAccessPoint)
// pair of a PoolLedgerPeers entry.
type PoolLedgerPeersDetail struct {
	PoolStake *cbor.Rat
	Relays    []RelayAccessPoint
}

func (d *PoolLedgerPeersDetail) UnmarshalCBOR(data []byte) error {
	fields, err := decodeBoundedArray(data, 2, "PoolLedgerPeersDetail")
	if err != nil {
		return err
	}
	if len(fields) != 2 {
		return fmt.Errorf(
			"PoolLedgerPeersDetail: expected 2 elements, got %d",
			len(fields),
		)
	}
	stake, relays, err := decodeStakeAndRelays(fields[0], fields[1])
	if err != nil {
		return err
	}
	d.PoolStake = stake
	d.Relays = relays
	return nil
}

func (d PoolLedgerPeersDetail) MarshalCBOR() ([]byte, error) {
	stake, relays, err := encodeStakeAndRelays(d.PoolStake, d.Relays)
	if err != nil {
		return nil, err
	}
	return cbor.Encode([]cbor.RawMessage{stake, relays})
}

func decodeStakeAndRelays(
	stakeData, relaysData []byte,
) (*cbor.Rat, []RelayAccessPoint, error) {
	stake, err := decodeRational(stakeData)
	if err != nil {
		return nil, nil, fmt.Errorf("pool stake: %w", err)
	}
	var relays []RelayAccessPoint
	if _, err := cbor.Decode(relaysData, &relays); err != nil {
		return nil, nil, err
	}
	if len(relays) == 0 {
		return nil, nil, errors.New("pool relays: empty NonEmpty list")
	}
	return &cbor.Rat{Rat: stake}, relays, nil
}

func encodeStakeAndRelays(
	stake *cbor.Rat,
	relays []RelayAccessPoint,
) ([]byte, []byte, error) {
	stakeData, err := encodeStake(stake)
	if err != nil {
		return nil, nil, err
	}
	if len(relays) == 0 {
		return nil, nil, errors.New("pool relays: empty NonEmpty list")
	}
	items := make([][]byte, 0, len(relays))
	for _, relay := range relays {
		item, err := cbor.Encode(relay)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	return stakeData, encodeIndefiniteList(items), nil
}

func encodeStake(r *cbor.Rat) ([]byte, error) {
	if r == nil {
		return nil, errors.New("pool stake: nil value")
	}
	return encodeRationalUntagged(r.Rat)
}

// LedgerPeerSnapshotResult is the typed result of a GetLedgerPeerSnapshot
// query, without the era result wrapper. Version selects the wire layout:
//
//	1: [1, [with_origin_slot, [* [acc_stake, [stake, relays]]]]]
//	2: [2, [point, network_magic, [* [acc_stake, stake, relays]]]]
//	3: [3, [point, network_magic, [* [stake, relays]]]]
//
// where point is [0] at origin or [1, slot, block_hash]. Version 1 answers
// node-to-client versions 19 to 22, which ask only for big ledger peers.
// Version 23 answers with 2 for big ledger peers and 3 for all of them.
//
// Slot is filled for every version; BlockHash and NetworkMagic only for 2
// and 3, and BlockHash is nil at origin and a 32-byte header hash otherwise.
// Version 3 entries carry no accumulated stake, so their AccumulatedStake is
// nil.
type LedgerPeerSnapshotResult struct {
	// Version is the wire version. Zero is accepted as an unset value when
	// constructing a result and encodes as version 1.
	Version      uint64
	Slot         WithOriginSlot
	BlockHash    []byte
	NetworkMagic uint32
	Pools        []PoolLedgerPeers
}

func (l *LedgerPeerSnapshotResult) UnmarshalCBOR(data []byte) error {
	outer, err := decodeBoundedArray(data, 2, "LedgerPeerSnapshot")
	if err != nil {
		return err
	}
	if len(outer) != 2 {
		return fmt.Errorf(
			"LedgerPeerSnapshot: expected 2 elements, got %d",
			len(outer),
		)
	}
	var version uint64
	if _, err := cbor.Decode(outer[0], &version); err != nil {
		return err
	}
	var inner []cbor.RawMessage
	switch version {
	case ledgerPeerSnapshotWireVersion:
		inner, err = decodeBoundedArray(outer[1], 2, "LedgerPeerSnapshot v1")
		if err != nil {
			return err
		}
	case ledgerPeerSnapshotWireVersionBig,
		ledgerPeerSnapshotWireVersionAll:
		inner, err = decodeBoundedArray(outer[1], 3, "LedgerPeerSnapshot body")
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: %d (expected 1, 2 or 3)",
			ErrLedgerPeerSnapshotUnsupportedWireVersion,
			version,
		)
	}
	var tmp LedgerPeerSnapshotResult
	tmp.Version = version
	if version == ledgerPeerSnapshotWireVersion {
		if len(inner) != 2 {
			return fmt.Errorf(
				"LedgerPeerSnapshot v1: expected 2 elements, got %d",
				len(inner),
			)
		}
		if _, err := cbor.Decode(inner[0], &tmp.Slot); err != nil {
			return err
		}
		if _, err := cbor.Decode(inner[1], &tmp.Pools); err != nil {
			return err
		}
		*l = tmp
		return nil
	}
	if len(inner) != 3 {
		return fmt.Errorf(
			"LedgerPeerSnapshot v%d: expected 3 elements, got %d",
			version,
			len(inner),
		)
	}
	slot, hash, err := decodeSnapshotPoint(inner[0])
	if err != nil {
		return err
	}
	tmp.Slot = slot
	tmp.BlockHash = hash
	if _, err := cbor.Decode(inner[1], &tmp.NetworkMagic); err != nil {
		return fmt.Errorf("LedgerPeerSnapshot: network magic: %w", err)
	}
	var rawPools []cbor.RawMessage
	if _, err := cbor.Decode(inner[2], &rawPools); err != nil {
		return err
	}
	tmp.Pools = make([]PoolLedgerPeers, 0, len(rawPools))
	for _, rawPool := range rawPools {
		// A pool is a fixed-shape record of at most three fields, so its
		// length is bounded before a slice is allocated for it.
		fields, err := decodeBoundedArray(rawPool, 3, "LedgerPeerSnapshot pool")
		if err != nil {
			return err
		}
		var pool PoolLedgerPeers
		if version == ledgerPeerSnapshotWireVersionBig {
			if len(fields) != 3 {
				return fmt.Errorf(
					"LedgerPeerSnapshot v2 pool: expected 3 elements, got %d",
					len(fields),
				)
			}
			acc, err := decodeRational(fields[0])
			if err != nil {
				return fmt.Errorf("accumulated stake: %w", err)
			}
			pool.AccumulatedStake = &cbor.Rat{Rat: acc}
			fields = fields[1:]
		} else if len(fields) != 2 {
			return fmt.Errorf(
				"LedgerPeerSnapshot v3 pool: expected 2 elements, got %d",
				len(fields),
			)
		}
		stake, relays, err := decodeStakeAndRelays(fields[0], fields[1])
		if err != nil {
			return err
		}
		pool.Detail = PoolLedgerPeersDetail{PoolStake: stake, Relays: relays}
		tmp.Pools = append(tmp.Pools, pool)
	}
	*l = tmp
	return nil
}

func (l LedgerPeerSnapshotResult) MarshalCBOR() ([]byte, error) {
	version := l.Version
	if version == 0 {
		version = ledgerPeerSnapshotWireVersion
	}
	pools := make([][]byte, 0, len(l.Pools))
	var inner []cbor.RawMessage
	switch version {
	case ledgerPeerSnapshotWireVersion:
		for _, pool := range l.Pools {
			item, err := cbor.Encode(pool)
			if err != nil {
				return nil, err
			}
			pools = append(pools, item)
		}
		slot, err := cbor.Encode(l.Slot)
		if err != nil {
			return nil, err
		}
		inner = []cbor.RawMessage{slot, encodeIndefiniteList(pools)}
	case ledgerPeerSnapshotWireVersionBig, ledgerPeerSnapshotWireVersionAll:
		for _, pool := range l.Pools {
			stake, relays, err := encodeStakeAndRelays(
				pool.Detail.PoolStake,
				pool.Detail.Relays,
			)
			if err != nil {
				return nil, err
			}
			fields := []cbor.RawMessage{stake, relays}
			if version == ledgerPeerSnapshotWireVersionBig {
				acc, err := encodeStake(pool.AccumulatedStake)
				if err != nil {
					return nil, err
				}
				fields = append([]cbor.RawMessage{acc}, fields...)
			}
			item, err := cbor.Encode(fields)
			if err != nil {
				return nil, err
			}
			pools = append(pools, item)
		}
		point, err := encodeSnapshotPoint(l.Slot, l.BlockHash)
		if err != nil {
			return nil, err
		}
		magic, err := cbor.Encode(l.NetworkMagic)
		if err != nil {
			return nil, err
		}
		inner = []cbor.RawMessage{
			point,
			magic,
			encodeIndefiniteList(pools),
		}
	default:
		return nil, fmt.Errorf("%w: %d (expected 1, 2 or 3)",
			ErrLedgerPeerSnapshotUnsupportedWireVersion,
			l.Version,
		)
	}
	innerData, err := cbor.Encode(inner)
	if err != nil {
		return nil, err
	}
	return cbor.Encode([]any{version, cbor.RawMessage(innerData)})
}

// decodeSnapshotPoint decodes the snapshot's Point RawBlockHash: [0] at
// origin, [1, slot, hash] otherwise.
func decodeSnapshotPoint(data []byte) (WithOriginSlot, []byte, error) {
	fields, err := decodeBoundedArray(data, 3, "LedgerPeerSnapshot point")
	if err != nil {
		return WithOriginSlot{}, nil, err
	}
	if len(fields) == 0 {
		return WithOriginSlot{}, nil, errors.New(
			"LedgerPeerSnapshot point: empty list",
		)
	}
	var tag uint8
	if _, err := cbor.Decode(fields[0], &tag); err != nil {
		return WithOriginSlot{}, nil, err
	}
	switch {
	case tag == 0 && len(fields) == 1:
		return WithOriginSlot{}, nil, nil
	case tag == 1 && len(fields) == 3:
		var slot uint64
		if _, err := cbor.Decode(fields[1], &slot); err != nil {
			return WithOriginSlot{}, nil, err
		}
		var hash []byte
		if _, err := cbor.Decode(fields[2], &hash); err != nil {
			return WithOriginSlot{}, nil, err
		}
		if len(hash) != ledgerPeerSnapshotBlockHashSize {
			return WithOriginSlot{}, nil, fmt.Errorf(
				"LedgerPeerSnapshot point: block hash is %d bytes, expected %d",
				len(hash),
				ledgerPeerSnapshotBlockHashSize,
			)
		}
		return WithOriginSlot{HasSlot: true, Slot: slot}, hash, nil
	}
	return WithOriginSlot{}, nil, fmt.Errorf(
		"LedgerPeerSnapshot point: tag %d with %d elements",
		tag,
		len(fields),
	)
}

func encodeSnapshotPoint(slot WithOriginSlot, hash []byte) ([]byte, error) {
	if !slot.HasSlot {
		if hash != nil {
			return nil, errors.New(
				"LedgerPeerSnapshot point: block hash at origin",
			)
		}
		return cbor.Encode([]int{0})
	}
	if len(hash) != ledgerPeerSnapshotBlockHashSize {
		return nil, fmt.Errorf(
			"LedgerPeerSnapshot point: block hash is %d bytes, expected %d",
			len(hash),
			ledgerPeerSnapshotBlockHashSize,
		)
	}
	return cbor.Encode([]any{1, slot.Slot, hash})
}
