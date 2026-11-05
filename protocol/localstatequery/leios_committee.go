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
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

// maxLeiosCommitteeSeats bounds the seat count accepted when decoding a
// committee from a peer, before the seat slice is sized from it. A vote
// refers to a seat by an index that addresses no more than this many seats.
const maxLeiosCommitteeSeats = lcommon.MaxLeiosCommitteeSize

// leiosVerificationKeySize is the length of a BLS12-381 MinSig verification
// key, the ledger's LeiosVerificationKey.
const leiosVerificationKeySize = 96

// ShelleyLeiosCommitteeQuery is GetLeiosCommittee (Shelley query 41), added
// by the Leios prototype: the Leios voting committees of the current and the
// next epoch. Wire form: [41].
type ShelleyLeiosCommitteeQuery struct {
	simpleQueryBase
}

// LeiosCommitteeSeat is one seat of a Leios voting committee: its weight and
// the BLS verification key that votes for it. VKey is nil while the seat has
// no key (the ledger's SNothing).
type LeiosCommitteeSeat struct {
	Weight *big.Rat
	VKey   []byte
}

// LeiosCommitteeResult is the GetLeiosCommittee reply, the ledger's
// QueryResultLeiosCommittee inside the single-element era result wrapper:
// [current, StrictMaybe next]. Seat order is significant, votes refer to a
// seat by its index.
//
// Current is the committee voting in the current epoch. Next is the committee
// for the following epoch, and is nil while the ledger cannot answer it yet
// (SNothing); a non-nil empty slice is a committee with no seats.
type LeiosCommitteeResult struct {
	Current []LeiosCommitteeSeat
	Next    []LeiosCommitteeSeat
}

func (r *LeiosCommitteeResult) UnmarshalCBOR(data []byte) error {
	// A two-element array is the era-mismatch reply, reported below.
	wrapper, err := decodeBoundedArray(data, 2, "Leios committee")
	if err != nil {
		return err
	}
	if len(wrapper) != 1 {
		return fmt.Errorf(
			"leios committee: expected a single-element result array, got %d elements",
			len(wrapper),
		)
	}
	fields, err := decodeBoundedArray(
		[]byte(wrapper[0]),
		2,
		"Leios committee result",
	)
	if err != nil {
		return err
	}
	if len(fields) != 2 {
		return fmt.Errorf(
			"leios committee: expected 2 committees, got %d fields",
			len(fields),
		)
	}
	current, err := decodeLeiosCommittee([]byte(fields[0]))
	if err != nil {
		return fmt.Errorf("leios committee: current committee: %w", err)
	}
	maybeNext, err := decodeBoundedArray(
		[]byte(fields[1]),
		1,
		"Leios committee next",
	)
	if err != nil {
		return err
	}
	var next []LeiosCommitteeSeat
	switch len(maybeNext) {
	case 0:
	case 1:
		next, err = decodeLeiosCommittee([]byte(maybeNext[0]))
		if err != nil {
			return fmt.Errorf("leios committee: next committee: %w", err)
		}
		if next == nil {
			next = []LeiosCommitteeSeat{}
		}
	default:
		return fmt.Errorf(
			"leios committee: next committee holds %d items, expected 0 or 1",
			len(maybeNext),
		)
	}
	r.Current = current
	r.Next = next
	return nil
}

func (r LeiosCommitteeResult) MarshalCBOR() ([]byte, error) {
	current, err := encodeLeiosCommittee(r.Current)
	if err != nil {
		return nil, fmt.Errorf("leios committee: current committee: %w", err)
	}
	// StrictMaybe encodes as a list: [] for SNothing, [x] for SJust.
	next := []cbor.RawMessage{}
	if r.Next != nil {
		encoded, err := encodeLeiosCommittee(r.Next)
		if err != nil {
			return nil, fmt.Errorf("leios committee: next committee: %w", err)
		}
		next = append(next, encoded)
	}
	nextEncoded, err := cbor.Encode(next)
	if err != nil {
		return nil, err
	}
	return cbor.Encode([]any{
		[]cbor.RawMessage{current, nextEncoded},
	})
}

func decodeLeiosCommittee(data []byte) ([]LeiosCommitteeSeat, error) {
	rawSeats, err := decodeBoundedArray(
		data,
		maxLeiosCommitteeSeats,
		"Leios committee seats",
	)
	if err != nil {
		return nil, err
	}
	if len(rawSeats) == 0 {
		return nil, nil
	}
	seats := make([]LeiosCommitteeSeat, 0, len(rawSeats))
	for i, rawSeat := range rawSeats {
		seat, err := decodeLeiosCommitteeSeat([]byte(rawSeat))
		if err != nil {
			return nil, fmt.Errorf("seat %d: %w", i, err)
		}
		seats = append(seats, seat)
	}
	return seats, nil
}

func decodeLeiosCommitteeSeat(data []byte) (LeiosCommitteeSeat, error) {
	var seat LeiosCommitteeSeat
	fields, err := decodeBoundedArray(data, 2, "Leios committee seat")
	if err != nil {
		return seat, err
	}
	if len(fields) != 2 {
		return seat, fmt.Errorf(
			"expected [weight, key], got %d fields",
			len(fields),
		)
	}
	var weight cbor.Rat
	if _, err := cbor.Decode([]byte(fields[0]), &weight); err != nil {
		return seat, fmt.Errorf("weight: %w", err)
	}
	if weight.Rat == nil {
		return seat, errors.New("weight: missing rational")
	}
	seat.Weight = weight.Rat
	maybeKey, err := decodeBoundedArray(
		[]byte(fields[1]),
		1,
		"Leios committee seat key",
	)
	if err != nil {
		return seat, err
	}
	switch len(maybeKey) {
	case 0:
	case 1:
		key := []byte{}
		if _, err := cbor.Decode([]byte(maybeKey[0]), &key); err != nil {
			return seat, fmt.Errorf("key: %w", err)
		}
		if len(key) != leiosVerificationKeySize {
			return seat, fmt.Errorf(
				"key: expected %d bytes, got %d",
				leiosVerificationKeySize,
				len(key),
			)
		}
		seat.VKey = key
	default:
		return seat, fmt.Errorf(
			"key holds %d items, expected 0 or 1",
			len(maybeKey),
		)
	}
	return seat, nil
}

func encodeLeiosCommittee(seats []LeiosCommitteeSeat) ([]byte, error) {
	items := make([][]byte, 0, len(seats))
	for i, seat := range seats {
		item, err := encodeLeiosCommitteeSeat(seat)
		if err != nil {
			return nil, fmt.Errorf("seat %d: %w", i, err)
		}
		items = append(items, item)
	}
	// The ledger writes a list definite up to 23 items and indefinite above.
	if len(items) > 23 {
		return encodeIndefiniteList(items), nil
	}
	raw := make([]cbor.RawMessage, len(items))
	for i, item := range items {
		raw[i] = item
	}
	return cbor.Encode(raw)
}

func encodeLeiosCommitteeSeat(seat LeiosCommitteeSeat) ([]byte, error) {
	if seat.Weight == nil {
		return nil, errors.New("missing weight")
	}
	weight, err := (&cbor.Rat{Rat: seat.Weight}).MarshalCBOR()
	if err != nil {
		return nil, fmt.Errorf("weight: %w", err)
	}
	key := []any{}
	if seat.VKey != nil {
		if len(seat.VKey) != leiosVerificationKeySize {
			return nil, fmt.Errorf(
				"key: expected %d bytes, got %d",
				leiosVerificationKeySize,
				len(seat.VKey),
			)
		}
		key = append(key, seat.VKey)
	}
	keyEncoded, err := cbor.Encode(key)
	if err != nil {
		return nil, err
	}
	return cbor.Encode([]cbor.RawMessage{weight, keyEncoded})
}
