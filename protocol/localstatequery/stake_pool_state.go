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
	"fmt"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

// ShelleyStakePoolStateQuery is GetStakePoolState (Shelley sub-query 40): the
// state of the registered stake pools. Only Leios prototype nodes answer it
// today, from node-to-client protocol version 23; released nodes do not have
// the query. The wire form is
//
//	[40, poolFilter, includeDelegators]
//
// where poolFilter is a StrictMaybe of a pool-id set, so an empty list asks
// about every pool and [ 258{poolids} ] restricts the reply to the given
// pools. The last argument asks for the delegators of each pool to be
// included, which is most of the size of the reply.
type ShelleyStakePoolStateQuery struct {
	cbor.StructAsArray
	Type              int
	Pools             []cbor.SetType[ledger.PoolId]
	IncludeDelegators bool
}

func (q *ShelleyStakePoolStateQuery) UnmarshalCBOR(data []byte) error {
	fields, err := cborpreflight.ArrayItems(
		data,
		3,
		"GetStakePoolState query",
	)
	if err != nil {
		return err
	}
	if len(fields) != 3 {
		return fmt.Errorf(
			"invalid GetStakePoolState query: expected 3 fields, got %d",
			len(fields),
		)
	}
	// The pool filter holds at most one pool set, whose size the node bounds
	// before it decodes it.
	filters, err := cborpreflight.ArrayItems(
		fields[1],
		1,
		"GetStakePoolState pool filter",
	)
	if err != nil {
		return err
	}
	if len(filters) == 1 {
		if err := validateStakePoolIdSet(filters[0]); err != nil {
			return err
		}
	}
	var tmp struct {
		cbor.StructAsArray
		Type              int
		Pools             []cbor.SetType[ledger.PoolId]
		IncludeDelegators bool
	}
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	q.Type = tmp.Type
	q.Pools = tmp.Pools
	q.IncludeDelegators = tmp.IncludeDelegators
	return nil
}

// MarshalCBOR writes the pool set the way the node's decodeSetSkel and
// UnmarshalCBOR accept it, whatever order and number of pools the query was
// built with. An empty set in the filter is written as the empty set, which
// is not the absent filter.
func (q ShelleyStakePoolStateQuery) MarshalCBOR() ([]byte, error) {
	if len(q.Pools) > 1 {
		return nil, fmt.Errorf(
			"GetStakePoolState pool filter: expected at most 1 set, got %d",
			len(q.Pools),
		)
	}
	poolFilter := []any{}
	if len(q.Pools) == 1 {
		poolSet, err := encodeStakePoolIdSet(q.Pools[0].Items())
		if err != nil {
			return nil, err
		}
		if poolSet == nil {
			poolSet, err = cbor.Encode(cbor.RawTag{
				Number:  cbor.CborTagSet,
				Content: []byte{0x80},
			})
			if err != nil {
				return nil, err
			}
		}
		poolFilter = append(poolFilter, cbor.RawMessage(poolSet))
	}
	return cbor.Encode([]any{q.Type, poolFilter, q.IncludeDelegators})
}

// encodeStakePoolIdSet encodes the pool filter of GetStakePoolState the way
// the node's decodeSetSkel accepts it: a tag-258, definite-length array whose
// members are in strictly ascending order of their encoding. A Set holds a
// member once, so duplicates are dropped. It returns nil for no pools, which
// the query writes as an absent filter.
//
// SetType is not used for this: it writes an indefinite-length array above 23
// members and keeps the order it was given, which the node rejects.
func encodeStakePoolIdSet(poolIds []ledger.PoolId) ([]byte, error) {
	if len(poolIds) == 0 {
		return nil, nil
	}
	if len(poolIds) > maxLocalStateQuerySetItems {
		return nil, fmt.Errorf(
			"pool IDs contains %d items; maximum is %d",
			len(poolIds),
			maxLocalStateQuerySetItems,
		)
	}
	members := make([]cbor.RawMessage, 0, len(poolIds))
	for _, poolId := range poolIds {
		encoded, err := cbor.Encode(poolId)
		if err != nil {
			return nil, fmt.Errorf("encode pool IDs item: %w", err)
		}
		members = append(members, encoded)
	}
	slices.SortFunc(members, func(a, b cbor.RawMessage) int {
		return bytes.Compare(a, b)
	})
	members = slices.CompactFunc(members, func(a, b cbor.RawMessage) bool {
		return bytes.Equal(a, b)
	})
	array, err := cbor.Encode(members)
	if err != nil {
		return nil, err
	}
	return cbor.Encode(cbor.RawTag{Number: cbor.CborTagSet, Content: array})
}

// validateStakePoolIdSet checks the pool set of the query the way the node's
// decodeSetSkel does: a tag-258, definite-length array whose members are in
// strictly ascending order of their encoding, which also rules out
// duplicates. The node bounds the size of the set before it decodes it.
func validateStakePoolIdSet(set []byte) error {
	const label = "GetStakePoolState pool set"
	if len(set) < 4 || set[0] != 0xd9 || set[1] != 0x01 || set[2] != 0x02 {
		return fmt.Errorf("invalid %s: expected tag 258", label)
	}
	body := set[3:]
	if body[0] == 0x9f {
		return fmt.Errorf("invalid %s: expected a definite-length array", label)
	}
	items, err := cborpreflight.ArrayItems(
		body,
		maxLocalStateQuerySetItems,
		label,
	)
	if err != nil {
		return err
	}
	for idx := 1; idx < len(items); idx++ {
		switch cmp := bytes.Compare(items[idx-1], items[idx]); {
		case cmp == 0:
			return fmt.Errorf("invalid %s: duplicate pool ID", label)
		case cmp > 0:
			return fmt.Errorf(
				"invalid %s: pool IDs not in ascending order",
				label,
			)
		}
	}
	return nil
}

// PoolFilter reports the pool IDs the query is restricted to. When all is
// true the query covers every pool (the StrictMaybe was empty) and pools is
// nil; when all is false only the returned pools are requested (which may be
// empty for an explicit filter of the empty set).
func (q *ShelleyStakePoolStateQuery) PoolFilter() (
	pools []ledger.PoolId,
	all bool,
) {
	if len(q.Pools) == 0 {
		return nil, true
	}
	return q.Pools[0].Items(), false
}

// StakePoolStateResult is the GetStakePoolState reply: the state of every
// registered stake pool, inside the single-element era result wrapper.
type StakePoolStateResult struct {
	cbor.StructAsArray
	Results map[ledger.PoolId]StakePoolStateEntry
}

const maxStakePoolStateResultItems = 131_072

func (r *StakePoolStateResult) UnmarshalCBOR(data []byte) error {
	fields, err := cborpreflight.ArrayItems(
		data,
		1,
		"stake pool state result",
	)
	if err != nil {
		return err
	}
	if len(fields) != 1 {
		return fmt.Errorf(
			"stake pool state result: expected 1 field, got %d",
			len(fields),
		)
	}
	if err := cborpreflight.ValidateMap(
		fields[0],
		maxStakePoolStateResultItems,
		"stake pool state result map",
		nil,
	); err != nil {
		return err
	}
	var results map[ledger.PoolId]StakePoolStateEntry
	if _, err := cbor.Decode(fields[0], &results); err != nil {
		return err
	}
	r.Results = results
	return nil
}

// stakePoolStateFieldCount is the number of fields in the ledger's
// QueryResultStakePoolState record.
const stakePoolStateFieldCount = 14

// StakePoolStateEntry is the ledger's QueryResultStakePoolState: the state of
// a registered stake pool. On the wire it is a 14-element array:
//
//	[ vrf, blsKey, pledge, cost, margin, accountId, owners, relays,
//	  metadata, deposit, numDelegators, delegators, retiring,
//	  futureStakePoolParams ]
//
// The ledger writes a null for the fields a pool has no value for.
type StakePoolStateEntry struct {
	cbor.StructAsArray
	// Vrf is the hash of the pool's VRF verification key.
	Vrf ledger.Blake2b256
	// BlsKey is the pool's registered BLS voting key, if it has one.
	BlsKey *BlsKeyState
	// Pledge is the amount the pool's operator has pledged to it.
	Pledge uint64
	// Cost is the pool's fixed cost per epoch.
	Cost uint64
	// Margin is the pool's variable fee, as a fraction of the epoch's rewards.
	Margin *cbor.Rat
	// AccountId is the stake credential the pool's rewards are paid to.
	AccountId lcommon.Credential
	// Owners are the stake key hashes that own the pool. The ledger writes
	// them as a CBOR set; build one with cbor.NewSetType to write the same
	// shape.
	Owners cbor.SetType[ledger.Blake2b224]
	// Relays are the pool's relays.
	Relays []ledger.PoolRelay
	// Metadata is the pool's metadata, if it has any.
	Metadata *lcommon.PoolMetadata
	// Deposit is the pool's deposit.
	Deposit uint64
	// NumDelegators is the number of credentials delegating to the pool.
	NumDelegators int
	// Delegators are the credentials delegating to the pool. The ledger only
	// fills them in when the query asked for them, and writes a null
	// otherwise.
	Delegators *cbor.SetType[lcommon.Credential]
	// Retiring is the epoch in which the pool is retiring, if it is.
	Retiring *uint64
	// FutureStakePoolParams is the pool's pending registration, if it has one.
	FutureStakePoolParams *StakePoolParamsEntry
}

// stakePoolStateWire mirrors StakePoolStateEntry. The delegators are held as
// raw CBOR because the ledger leaves them out when the query did not ask for
// them, and a null there means something different from an empty set.
type stakePoolStateWire struct {
	cbor.StructAsArray
	Vrf                   ledger.Blake2b256
	BlsKey                *BlsKeyState
	Pledge                uint64
	Cost                  uint64
	Margin                *cbor.Rat
	AccountId             lcommon.Credential
	Owners                cbor.SetType[ledger.Blake2b224]
	Relays                []ledger.PoolRelay
	Metadata              *lcommon.PoolMetadata
	Deposit               uint64
	NumDelegators         int
	Delegators            cbor.RawMessage
	Retiring              *uint64
	FutureStakePoolParams cbor.RawMessage
}

func (e *StakePoolStateEntry) UnmarshalCBOR(data []byte) error {
	fields, err := cborpreflight.ArrayItems(
		data,
		stakePoolStateFieldCount,
		"stake pool state",
	)
	if err != nil {
		return err
	}
	if len(fields) != stakePoolStateFieldCount {
		return fmt.Errorf(
			"stake pool state: expected %d fields, got %d",
			stakePoolStateFieldCount,
			len(fields),
		)
	}
	for _, collection := range []struct {
		field int
		label string
	}{
		{field: 6, label: "stake pool owners"},
		{field: 7, label: "stake pool relays"},
	} {
		if _, err := cborpreflight.ArrayItems(
			fields[collection.field],
			maxLocalStateQuerySetItems,
			collection.label,
		); err != nil {
			return err
		}
	}
	if !cborIsNull(fields[11]) {
		if _, err := cborpreflight.ArrayItems(
			fields[11],
			maxStakePoolStateResultItems,
			"stake pool delegators",
		); err != nil {
			return err
		}
	}
	var wire stakePoolStateWire
	if _, err := cbor.Decode(data, &wire); err != nil {
		return err
	}
	tmp := StakePoolStateEntry{
		Vrf:           wire.Vrf,
		BlsKey:        wire.BlsKey,
		Pledge:        wire.Pledge,
		Cost:          wire.Cost,
		Margin:        wire.Margin,
		AccountId:     wire.AccountId,
		Owners:        wire.Owners,
		Relays:        wire.Relays,
		Metadata:      wire.Metadata,
		Deposit:       wire.Deposit,
		NumDelegators: wire.NumDelegators,
		Retiring:      wire.Retiring,
	}
	if !cborIsNull(wire.Delegators) {
		var delegators cbor.SetType[lcommon.Credential]
		if _, err := cbor.Decode(wire.Delegators, &delegators); err != nil {
			return fmt.Errorf("stake pool state: delegators: %w", err)
		}
		tmp.Delegators = &delegators
	}
	if !cborIsNull(wire.FutureStakePoolParams) {
		var params StakePoolParamsEntry
		if _, err := cbor.Decode(
			wire.FutureStakePoolParams,
			&params,
		); err != nil {
			return fmt.Errorf("stake pool state: future params: %w", err)
		}
		tmp.FutureStakePoolParams = &params
	}
	*e = tmp
	return nil
}

// BlsKeyState is a registered BLS voting key, together with the epoch its
// registration took effect in.
type BlsKeyState struct {
	cbor.StructAsArray
	// Key is the pool's BLS voting key.
	Key BlsKey
	// RegisteredIn is the epoch the key was registered in.
	RegisteredIn uint64
}

// BlsKey is a BLS voting key: the public key and its possession proof.
type BlsKey struct {
	cbor.StructAsArray
	// PublicKey is the BLS public key of the pool.
	PublicKey []byte
	// PossessionProof is the proof of possession of the public key.
	PossessionProof []byte
}

// stakePoolParamsFieldCount is the number of fields in the ledger's
// StakePoolParams record without a BLS key.
const stakePoolParamsFieldCount = 9

// StakePoolParamsEntry is the ledger's StakePoolParams, as it appears in a
// GetStakePoolState reply. On the wire it is a flat list of nine fields:
//
//	[ id, vrf, pledge, cost, margin, accountAddress, owners, relays,
//	  metadata ]
//
// From protocol version 12 on, a pool with a BLS key has a tenth field for it,
// after the VRF hash.
type StakePoolParamsEntry struct {
	cbor.StructAsArray
	// Id is the pool's key hash.
	Id ledger.PoolId
	// Vrf is the hash of the pool's VRF verification key.
	Vrf ledger.Blake2b256
	// BlsKey is the pool's BLS voting key, if it has one.
	BlsKey *BlsKey
	// Pledge is the amount the pool's operator has pledged to it.
	Pledge uint64
	// Cost is the pool's fixed cost per epoch.
	Cost uint64
	// Margin is the pool's variable fee, as a fraction of the epoch's rewards.
	Margin *cbor.Rat
	// AccountAddress is the address the pool's rewards are paid to. The
	// ledger writes it as a byte string holding the address header and the
	// hash of the stake credential.
	AccountAddress []byte
	// Owners are the stake key hashes that own the pool. The ledger writes
	// them as a CBOR set; build one with cbor.NewSetType to write the same
	// shape.
	Owners cbor.SetType[ledger.Blake2b224]
	// Relays are the pool's relays.
	Relays []ledger.PoolRelay
	// Metadata is the pool's metadata, if it has any.
	Metadata *lcommon.PoolMetadata
}

func (p *StakePoolParamsEntry) UnmarshalCBOR(data []byte) error {
	fields, err := cborpreflight.ArrayItems(
		data,
		stakePoolParamsFieldCount+1,
		"stake pool params",
	)
	if err != nil {
		return err
	}
	// The BLS key is only in the record from protocol version 12 on, so the
	// record has nine fields without it and ten with it.
	switch len(fields) {
	case stakePoolParamsFieldCount, stakePoolParamsFieldCount + 1:
	default:
		return fmt.Errorf(
			"stake pool params: expected %d or %d fields, got %d",
			stakePoolParamsFieldCount,
			stakePoolParamsFieldCount+1,
			len(fields),
		)
	}
	offset := 0
	if len(fields) == stakePoolParamsFieldCount+1 {
		offset = 1
	}
	tmp := StakePoolParamsEntry{}
	targets := []any{
		&tmp.Id,
		&tmp.Vrf,
	}
	if _, err := cbor.Decode(fields[0], targets[0]); err != nil {
		return fmt.Errorf("stake pool params: pool id: %w", err)
	}
	if _, err := cbor.Decode(fields[1], targets[1]); err != nil {
		return fmt.Errorf("stake pool params: VRF hash: %w", err)
	}
	rest := fields[2+offset:]
	for _, collection := range []struct {
		field int
		label string
	}{
		{field: 4, label: "stake pool params owners"},
		{field: 5, label: "stake pool params relays"},
	} {
		if _, err := cborpreflight.ArrayItems(
			rest[collection.field],
			maxLocalStateQuerySetItems,
			collection.label,
		); err != nil {
			return err
		}
	}
	// A null in the BLS slot is no key, as the ledger reads it
	// (decodeBlsKeyBackwardsCompatible), not a zero key.
	if offset == 1 && !cborIsNull(fields[2]) {
		var blsKey BlsKey
		if _, err := cbor.Decode(fields[2], &blsKey); err != nil {
			return fmt.Errorf("stake pool params: BLS key: %w", err)
		}
		tmp.BlsKey = &blsKey
	}
	restTargets := []any{
		&tmp.Pledge,
		&tmp.Cost,
		&tmp.Margin,
		&tmp.AccountAddress,
		&tmp.Owners,
		&tmp.Relays,
		&tmp.Metadata,
	}
	names := []string{
		"pledge",
		"cost",
		"margin",
		"account address",
		"owners",
		"relays",
		"metadata",
	}
	for idx, target := range restTargets {
		if _, err := cbor.Decode(rest[idx], target); err != nil {
			return fmt.Errorf("stake pool params: %s: %w", names[idx], err)
		}
	}
	*p = tmp
	return nil
}

// MarshalCBOR writes the nine fields of a record without a BLS key, and the
// ten fields of a record with one, as the ledger does.
func (p StakePoolParamsEntry) MarshalCBOR() ([]byte, error) {
	fields := []any{
		p.Id,
		p.Vrf,
	}
	if p.BlsKey != nil {
		fields = append(fields, *p.BlsKey)
	}
	fields = append(
		fields,
		p.Pledge,
		p.Cost,
		p.Margin,
		p.AccountAddress,
		p.Owners,
		p.Relays,
		p.Metadata,
	)
	return cbor.Encode(fields)
}

// cborIsNull reports whether the encoded item is a CBOR null.
func cborIsNull(data []byte) bool {
	return len(data) == 1 && data[0] == 0xf6
}
