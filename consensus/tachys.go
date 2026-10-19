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

package consensus

import (
	"bytes"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"sort"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// TachysSpecificationRevision is the CIP-0177 revision used by this package.
const TachysSpecificationRevision = "86b89208d3b2aabb5dcc5b778dfbe09096b4e114"

// TachysPool is one entry in the active stake distribution.
type TachysPool struct {
	ID    common.PoolKeyHash
	Stake uint64
}

type tachysInterval struct {
	id         common.PoolKeyHash
	upperStake uint64
}

// TachysLeaderSelector is an immutable prepared CIP-0177 stake distribution.
// Its methods are safe for concurrent use.
type TachysLeaderSelector struct {
	intervals  []tachysInterval
	totalStake uint64
}

// NewTachysLeaderSelector prepares a canonical cumulative stake distribution.
// Pool IDs are ordered by their 28-byte representation. totalStake must equal
// the exact sum of every entry.
func NewTachysLeaderSelector(
	pools []TachysPool,
	totalStake uint64,
) (*TachysLeaderSelector, error) {
	if len(pools) == 0 {
		return nil, errors.New("tachys stake distribution is empty")
	}
	if totalStake == 0 {
		return nil, errors.New("tachys total stake cannot be zero")
	}
	ordered := append([]TachysPool(nil), pools...)
	sort.Slice(ordered, func(i, j int) bool {
		return bytes.Compare(ordered[i].ID[:], ordered[j].ID[:]) < 0
	})
	intervals := make([]tachysInterval, len(ordered))
	var cumulative uint64
	for i, pool := range ordered {
		if pool.Stake == 0 {
			return nil, fmt.Errorf(
				"tachys pool %s has zero stake", pool.ID,
			)
		}
		if i > 0 && ordered[i-1].ID == pool.ID {
			return nil, fmt.Errorf(
				"duplicate Tachys pool ID: %s", pool.ID,
			)
		}
		next, carry := bits.Add64(cumulative, pool.Stake, 0)
		if carry != 0 {
			return nil, errors.New("tachys total stake overflows uint64")
		}
		cumulative = next
		intervals[i] = tachysInterval{
			id:         pool.ID,
			upperStake: cumulative,
		}
	}
	if cumulative != totalStake {
		return nil, fmt.Errorf(
			"tachys total stake mismatch: entries sum to %d, total is %d",
			cumulative,
			totalStake,
		)
	}
	return &TachysLeaderSelector{
		intervals:  intervals,
		totalStake: totalStake,
	}, nil
}

// TachysSample returns the CIP-0177 sample for an absolute slot. This pinned
// representation is BLAKE2b-256(nonce || uint64be(slot)); the digest is read
// as an unsigned big-endian integer over [0, 2^256).
func TachysSample(epochNonce []byte, slot uint64) ([32]byte, error) {
	if len(epochNonce) != common.Blake2b256Size {
		return [32]byte{}, fmt.Errorf(
			"tachys epoch nonce must be %d bytes, got %d",
			common.Blake2b256Size,
			len(epochNonce),
		)
	}
	var input [common.Blake2b256Size + 8]byte
	copy(input[:common.Blake2b256Size], epochNonce)
	binary.BigEndian.PutUint64(input[common.Blake2b256Size:], slot)
	return common.Blake2b256Hash(input[:]), nil
}

// Leader returns the unique scheduled pool for an absolute slot.
func (s *TachysLeaderSelector) Leader(
	epochNonce []byte,
	slot uint64,
) (common.PoolKeyHash, error) {
	if s == nil || len(s.intervals) == 0 || s.totalStake == 0 {
		return common.PoolKeyHash{}, errors.New(
			"tachys leader selector is uninitialized",
		)
	}
	sample, err := TachysSample(epochNonce, slot)
	if err != nil {
		return common.PoolKeyHash{}, err
	}
	return s.LeaderForSample(sample[:])
}

// LeaderForSample returns the pool containing a 32-byte unsigned big-endian
// sample. It is exposed for independent conformance and boundary vectors.
func (s *TachysLeaderSelector) LeaderForSample(
	sample []byte,
) (common.PoolKeyHash, error) {
	if s == nil || len(s.intervals) == 0 || s.totalStake == 0 {
		return common.PoolKeyHash{}, errors.New(
			"tachys leader selector is uninitialized",
		)
	}
	if len(sample) != common.Blake2b256Size {
		return common.PoolKeyHash{}, fmt.Errorf(
			"tachys sample must be %d bytes, got %d",
			common.Blake2b256Size,
			len(sample),
		)
	}
	var sampleInt, scaledSample, scaledBoundary big.Int
	sampleInt.SetBytes(sample)
	scaledSample.Mul(&sampleInt, new(big.Int).SetUint64(s.totalStake))
	twoTo256 := new(big.Int).Lsh(big.NewInt(1), 256)
	index := sort.Search(len(s.intervals), func(i int) bool {
		scaledBoundary.SetUint64(s.intervals[i].upperStake)
		scaledBoundary.Mul(&scaledBoundary, twoTo256)
		return scaledSample.Cmp(&scaledBoundary) < 0
	})
	return s.intervals[index].id, nil
}

// IsLeader reports whether poolID is scheduled for the slot.
func (s *TachysLeaderSelector) IsLeader(
	epochNonce []byte,
	slot uint64,
	poolID common.PoolKeyHash,
) (bool, error) {
	leader, err := s.Leader(epochNonce, slot)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(leader[:], poolID[:]) == 1, nil
}

// AuthorizeIssuer reports whether the issuer verification key belongs to the
// pool scheduled for the slot.
func (s *TachysLeaderSelector) AuthorizeIssuer(
	epochNonce []byte,
	slot uint64,
	issuerVkey []byte,
) (bool, error) {
	if len(issuerVkey) != 32 {
		return false, fmt.Errorf(
			"tachys issuer verification key must be 32 bytes, got %d",
			len(issuerVkey),
		)
	}
	poolID := common.PoolKeyHash(common.Blake2b224Hash(issuerVkey))
	return s.IsLeader(epochNonce, slot, poolID)
}

// IsTachysSlotLeader checks public schedule authorization and constructs the
// Praos VRF certificate retained by a Tachys header. An unscheduled issuer
// returns an ineligible result without invoking the signer.
func IsTachysSlotLeader(
	slot uint64,
	epochNonce []byte,
	issuerVkey []byte,
	selector *TachysLeaderSelector,
	vrfSigner VRFSigner,
) (*LeaderElectionResult, error) {
	if selector == nil {
		return nil, errors.New("tachys leader selector is nil")
	}
	if vrfSigner == nil {
		return nil, errors.New("vrfSigner is nil")
	}
	authorized, err := selector.AuthorizeIssuer(
		epochNonce,
		slot,
		issuerVkey,
	)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return &LeaderElectionResult{}, nil
	}
	if slot > math.MaxInt64 {
		return nil, fmt.Errorf(
			"slot %d exceeds maximum int64 value for VRF input", slot,
		)
	}
	vrfInput, err := vrf.MkInputVrf(
		int64(slot), //nolint:gosec // checked above
		epochNonce,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create VRF input: %w", err)
	}
	proof, output, err := vrfSigner.Prove(vrfInput)
	if err != nil {
		return nil, err
	}
	if len(proof) != vrf.ProofSize {
		return nil, fmt.Errorf(
			"VRF proof: expected %d bytes, got %d",
			vrf.ProofSize,
			len(proof),
		)
	}
	if len(output) != vrf.OutputSize {
		return nil, fmt.Errorf(
			"VRF output: expected %d bytes, got %d",
			vrf.OutputSize,
			len(output),
		)
	}
	return &LeaderElectionResult{
		Eligible: true,
		Proof:    proof,
		Output:   output,
	}, nil
}
