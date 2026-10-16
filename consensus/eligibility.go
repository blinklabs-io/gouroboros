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
	"errors"
	"fmt"
	"math/big"
	"math/bits"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// ConsensusMode represents the consensus protocol variant.
type ConsensusMode int

const (
	// ConsensusModeCPraos is the current Praos consensus (Babbage+).
	// Uses BLAKE2b-256("L" || vrfOutput), a 256-bit certified natural.
	ConsensusModeCPraos ConsensusMode = iota

	// ConsensusModeTPraos is the transitional Praos consensus (Shelley-Alonzo).
	// Uses the raw 64-byte output, a 512-bit certified natural.
	ConsensusModeTPraos
)

// Certified-natural widths, excluding the artificial endpoint M itself.
const (
	// CPraos uses the BLAKE2b-256 leader hash.
	vrfOutputBitsCPraos = 256
	// TPraos uses the raw 64-byte output.
	vrfOutputBitsTPraos = 512
)

// twoTo256 is the CPraos certified-natural maximum M.
// WARNING: These package-level big.Int values must not be mutated. Always use
// them as read-only constants. Create new big.Int instances for calculations.
var twoTo256 = new(
	big.Int,
).Exp(big.NewInt(2), big.NewInt(vrfOutputBitsCPraos), nil)

// twoTo512 is the TPraos certified-natural maximum M.
// WARNING: This package-level big.Int value must not be mutated. Always use
// it as a read-only constant. Create new big.Int instances for calculations.
var twoTo512 = new(
	big.Int,
).Exp(big.NewInt(2), big.NewInt(vrfOutputBitsTPraos), nil)

// LeaderEligibility is an immutable prepared node E34 leader predicate.
// Its methods support concurrent reads. Reassigning a wrapper does not change
// other evaluators or cached state; concurrent reassignment of that same
// wrapper is not supported.
type LeaderEligibility struct {
	state *leaderEligibilityState
}

// Every integer in a published state is read-only. Decision scratch belongs
// to the call, never the state or cache.
type leaderEligibilityState struct {
	mode          ConsensusMode
	maximum       big.Int
	sigma         big.Int
	c             big.Int
	x             big.Int
	unconditional bool
}

// NewLeaderEligibility prepares the pinned node's finite E34 predicate.
// activeSlotCoeff must be in [1/10^19, 1], with a reduced numerator and
// denominator representable as uint64; nondecimal rationals are supported.
// A zero total stake is an error. Pool stake above a positive total is clamped
// as library policy. Unlike pool-election adapters, this primitive accepts
// zero pool stake unconditionally when the exact coefficient is one.
// The caller may mutate activeSlotCoeff after this function returns, but not
// concurrently with construction.
func NewLeaderEligibility(
	poolStake, totalStake uint64,
	activeSlotCoeff *big.Rat,
	mode ConsensusMode,
) (*LeaderEligibility, error) {
	return newLeaderEligibility(
		leaderEligibilityMemo,
		poolStake,
		totalStake,
		activeSlotCoeff,
		mode,
	)
}

func validateConsensusMode(mode ConsensusMode) error {
	switch mode {
	case ConsensusModeCPraos, ConsensusModeTPraos:
		return nil
	default:
		return fmt.Errorf("unknown consensus mode: %d", mode)
	}
}

func validateActiveSlotCoefficient(f *big.Rat) error {
	if f == nil {
		return errors.New("activeSlotCoeff is nil")
	}
	if f.Sign() > 0 && f.Num().Cmp(f.Denom()) <= 0 &&
		f.Num().IsUint64() && f.Denom().IsUint64() {
		// The validated Word64 operands fit a two-word exact product.
		hi, lo := bits.Mul64(
			f.Num().Uint64(), 10_000_000_000_000_000_000,
		)
		if hi != 0 || lo >= f.Denom().Uint64() {
			return nil
		}
	}
	return errors.New(
		"activeSlotCoeff is outside the node PositiveUnitInterval domain",
	)
}

func newLeaderEligibility(
	cache *leaderEligibilityCache,
	poolStake, totalStake uint64,
	activeSlotCoeff *big.Rat,
	mode ConsensusMode,
) (*LeaderEligibility, error) {
	if err := validateConsensusMode(mode); err != nil {
		return nil, err
	}
	if err := validateActiveSlotCoefficient(activeSlotCoeff); err != nil {
		return nil, err
	}
	if totalStake == 0 {
		return nil, errors.New("total stake cannot be zero")
	}
	if poolStake > totalStake {
		poolStake = totalStake
	}
	key := leaderEligibilityCacheKey{
		poolStake:   poolStake,
		totalStake:  totalStake,
		numerator:   activeSlotCoeff.Num().Uint64(),
		denominator: activeSlotCoeff.Denom().Uint64(),
		mode:        mode,
	}
	if state, ok := cache.get(key); ok {
		return &LeaderEligibility{state: state}, nil
	}
	state, err := prepareLeaderEligibility(key)
	if err != nil {
		return nil, err
	}
	cache.put(key, state)
	return &LeaderEligibility{state: state}, nil
}

func prepareLeaderEligibility(
	key leaderEligibilityCacheKey,
) (*leaderEligibilityState, error) {
	state := &leaderEligibilityState{mode: key.mode}
	if key.mode == ConsensusModeCPraos {
		state.maximum.Set(twoTo256)
	} else {
		state.maximum.Set(twoTo512)
	}
	var numerator, denominator, f, oneMinusF big.Int
	numerator.SetUint64(key.poolStake)
	denominator.SetUint64(key.totalStake)
	if err := e34FromRatio(&state.sigma, &numerator, &denominator); err != nil {
		return nil, err
	}
	if key.numerator == key.denominator {
		state.unconditional = true
		return state, nil
	}
	numerator.SetUint64(key.numerator)
	denominator.SetUint64(key.denominator)
	if err := e34FromRatio(&f, &numerator, &denominator); err != nil {
		return nil, err
	}
	oneMinusF.Sub(e34Scale, &f)
	if err := e34Ln(&state.c, &oneMinusF); err != nil {
		return nil, err
	}
	// Negating before the floor multiplication changes raw boundary decisions.
	e34Mul(&state.x, &state.sigma, &state.c)
	state.x.Neg(&state.x)
	return state, nil
}

func (e *LeaderEligibility) initialized() error {
	if e == nil || e.state == nil {
		return errors.New("leader eligibility evaluator is uninitialized")
	}
	return nil
}

// CheckCertifiedNatural evaluates a natural in [0, M], where M is 2^256 for
// CPraos and 2^512 for TPraos. M itself is an artificial oracle endpoint, not
// an N-bit wire value. The caller's natural is never modified. Only the E34
// comparator's BELOW result elects; exhausting 1000 iterations returns false.
func (e *LeaderEligibility) CheckCertifiedNatural(v *big.Int) (bool, error) {
	if err := e.initialized(); err != nil {
		return false, err
	}
	if v == nil {
		return false, errors.New("certified natural is nil")
	}
	if v.Sign() < 0 || v.Cmp(&e.state.maximum) > 0 {
		return false, errors.New("certified natural is outside [0, M]")
	}
	if e.state.unconditional {
		return true, nil
	}
	var q, denominator big.Int
	q.Mul(&e.state.maximum, e34Scale)
	denominator.Sub(&e.state.maximum, v)
	if denominator.Sign() != 0 {
		q.Quo(&q, &denominator)
	}
	classification, _, _, err := e34Compare(&q, &e.state.x)
	if err != nil {
		return false, err
	}
	return classification == e34Below, nil
}

// CheckVRFOutput evaluates exactly 64 raw VRF output bytes. TPraos reads an
// unsigned big-endian 512-bit natural; CPraos reads the 256-bit natural from
// BLAKE2b-256("L" || output). This method does not verify a VRF proof.
func (e *LeaderEligibility) CheckVRFOutput(output []byte) (bool, error) {
	if err := e.initialized(); err != nil {
		return false, err
	}
	if len(output) != 64 {
		return false, fmt.Errorf(
			"VRF output: expected 64 bytes, got %d", len(output),
		)
	}
	var natural big.Int
	if e.state.mode == ConsensusModeCPraos {
		natural.SetBytes(VrfLeaderValue(output))
	} else {
		natural.SetBytes(output)
	}
	return e.CheckCertifiedNatural(&natural)
}

// VrfLeaderValue computes the CPRAOS leader value from a VRF output.
// This applies domain separation by hashing with "L" prefix:
//
//	leaderValue = BLAKE2b-256("L" || vrfOutput)
//
// The result is 32 bytes (256 bits).
func VrfLeaderValue(vrfOutput []byte) []byte {
	// Use fixed-size buffer to avoid allocation (VRF output is 64 bytes)
	var buf [65]byte // 1 + 64 for VRF output
	buf[0] = 0x4C    // "L" prefix for domain separation
	if len(vrfOutput) <= 64 {
		copy(buf[1:], vrfOutput)
		hash := common.Blake2b256Hash(buf[:1+len(vrfOutput)])
		return hash.Bytes()
	}
	// Fallback for larger inputs (not expected in normal operation)
	data := make([]byte, 1+len(vrfOutput))
	data[0] = 0x4C
	copy(data[1:], vrfOutput)
	hash := common.Blake2b256Hash(data)
	return hash.Bytes()
}

// VRFOutputToInt interprets output as an unsigned big-endian integer.
func VRFOutputToInt(output []byte) *big.Int {
	return new(big.Int).SetBytes(output)
}
