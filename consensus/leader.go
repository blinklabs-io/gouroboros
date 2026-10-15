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
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/blinklabs-io/gouroboros/vrf"
)

// LeaderElectionResult contains the result of a slot leadership check
type LeaderElectionResult struct {
	// Eligible is true if the pool is eligible to produce a block
	Eligible bool
	// Proof is the VRF proof for the slot (nil only on early-return cases like zero stake)
	Proof []byte
	// Output is the VRF output (nil only on early-return cases like zero stake)
	Output []byte
}

// IsSlotLeader checks if a pool is eligible to produce a block in the given slot
// using CPRAOS consensus. For TPraos compatibility, use IsSlotLeaderWithMode.
//
// CPraos constructs MkInputVrf(slot, epochNonce), generates a VRF proof,
// then evaluates the domain-separated leader natural with the finite E34
// predicate. Zero pool or total stake is a pool-policy false result.
//
// Parameters:
//   - slot: the slot number to check
//   - epochNonce: the epoch nonce (eta0) for randomness
//   - poolStake: the pool's delegated stake
//   - totalStake: the total active stake
//   - activeSlotCoeff: the active slot coefficient (f)
//   - vrfSigner: the VRF signer for generating proofs
//
// Returns a LeaderElectionResult containing eligibility status and proof.
func IsSlotLeader(
	slot uint64,
	epochNonce []byte,
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
	vrfSigner VRFSigner,
) (*LeaderElectionResult, error) {
	return IsSlotLeaderWithMode(
		slot,
		epochNonce,
		poolStake,
		totalStake,
		activeSlotCoeff,
		vrfSigner,
		ConsensusModeCPraos,
	)
}

// IsSlotLeaderWithMode checks pool leadership using the selected mode's
// VRF input and the prepared finite E34 predicate. TPraos uses MkSeedTPraos
// with SeedL and the raw output; CPraos uses MkInputVrf and the leader hash.
// Positive-stake ineligible results retain their proof and output.
func IsSlotLeaderWithMode(
	slot uint64,
	epochNonce []byte,
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
	vrfSigner VRFSigner,
	mode ConsensusMode,
) (*LeaderElectionResult, error) {
	if err := validateConsensusMode(mode); err != nil {
		return nil, err
	}
	evaluator, zeroStake, err := prepareSlotLeadership(
		epochNonce, poolStake, totalStake, activeSlotCoeff, vrfSigner, mode,
	)
	if err != nil {
		return nil, err
	}
	if zeroStake {
		return &LeaderElectionResult{}, nil
	}
	return checkSlotLeadership(
		slot, epochNonce, vrfSigner, mode, evaluator,
	)
}

func prepareSlotLeadership(
	epochNonce []byte,
	poolStake, totalStake uint64,
	activeSlotCoeff *big.Rat,
	vrfSigner VRFSigner,
	mode ConsensusMode,
) (*LeaderEligibility, bool, error) {
	if vrfSigner == nil {
		return nil, false, errors.New("vrfSigner is nil")
	}
	if len(epochNonce) != 32 {
		return nil, false, fmt.Errorf(
			"epochNonce must be 32 bytes, got %d", len(epochNonce),
		)
	}
	if activeSlotCoeff == nil {
		return nil, false, errors.New("activeSlotCoeff is nil")
	}
	if totalStake == 0 || poolStake == 0 {
		if activeSlotCoeff.Sign() > 0 {
			if err := validateActiveSlotCoefficient(activeSlotCoeff); err != nil {
				return nil, false, err
			}
		}
		return nil, true, nil
	}
	if activeSlotCoeff.Sign() <= 0 {
		return nil, false, nil
	}
	evaluator, err := NewLeaderEligibility(
		poolStake, totalStake, activeSlotCoeff, mode,
	)
	return evaluator, false, err
}

func checkSlotLeadership(
	slot uint64,
	epochNonce []byte,
	vrfSigner VRFSigner,
	mode ConsensusMode,
	evaluator *LeaderEligibility,
) (*LeaderElectionResult, error) {
	if slot > math.MaxInt64 {
		return nil, fmt.Errorf(
			"slot %d exceeds maximum int64 value for VRF input", slot,
		)
	}
	var vrfInput []byte
	var err error
	switch mode {
	case ConsensusModeTPraos:
		vrfInput, err = vrf.MkSeedTPraos(
			int64(slot), //nolint:gosec
			epochNonce,
			vrf.SeedL(),
		)
	case ConsensusModeCPraos:
		vrfInput, err = vrf.MkInputVrf(
			int64(slot), //nolint:gosec
			epochNonce,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create VRF input: %w", err)
	}
	proof, output, err := vrfSigner.Prove(vrfInput)
	if err != nil {
		return nil, err
	}
	if len(proof) != 80 {
		return nil, fmt.Errorf(
			"VRF proof: expected 80 bytes, got %d", len(proof),
		)
	}
	if len(output) != 64 {
		return nil, fmt.Errorf(
			"VRF output: expected 64 bytes, got %d", len(output),
		)
	}
	result := &LeaderElectionResult{Proof: proof, Output: output}
	// Nonpositive coefficients preserve the election adapter's false policy,
	// but do not bypass proof/output validation for a positive-stake pool.
	if evaluator != nil {
		result.Eligible, err = evaluator.CheckVRFOutput(output)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// IsSlotLeaderFromComponents performs leader election check from pre-computed components
// using CPRAOS consensus. For TPraos compatibility, use IsSlotLeaderFromComponentsWithMode.
//
// Parameters:
//   - vrfOutput: the VRF output (64 bytes)
//   - poolStake: the pool's delegated stake
//   - totalStake: the total active stake
//   - activeSlotCoeff: the active slot coefficient (f)
//
// Returns true if the pool is eligible to be the slot leader.
func IsSlotLeaderFromComponents(
	vrfOutput []byte,
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
) bool {
	eligible, _ := IsSlotLeaderFromComponentsWithMode(
		vrfOutput,
		poolStake,
		totalStake,
		activeSlotCoeff,
		ConsensusModeCPraos,
	)
	return eligible
}

// IsSlotLeaderFromComponentsWithMode performs leader election check from pre-computed
// components using the specified consensus mode.
//
// Parameters:
//   - vrfOutput: the VRF output (64 bytes)
//   - poolStake: the pool's delegated stake
//   - totalStake: the total active stake
//   - activeSlotCoeff: the active slot coefficient (f)
//   - mode: the consensus mode (CPRAOS or TPraos)
//
// Returns true if the pool is eligible to be the slot leader, or an error if
// the consensus mode is unknown.
func IsSlotLeaderFromComponentsWithMode(
	vrfOutput []byte,
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
	mode ConsensusMode,
) (bool, error) {
	if err := validateConsensusMode(mode); err != nil {
		return false, err
	}
	if activeSlotCoeff == nil || activeSlotCoeff.Sign() <= 0 {
		return false, nil
	}
	if totalStake == 0 || poolStake == 0 || len(vrfOutput) != 64 {
		if err := validateActiveSlotCoefficient(activeSlotCoeff); err != nil {
			return false, err
		}
		return false, nil
	}
	evaluator, err := NewLeaderEligibility(
		poolStake, totalStake, activeSlotCoeff, mode,
	)
	if err != nil {
		return false, err
	}
	return evaluator.CheckVRFOutput(vrfOutput)
}

// SimpleVRFSigner is a simple implementation of VRFSigner using the vrf package.
// This can be used for testing or when keys are available in memory.
type SimpleVRFSigner struct {
	secretKey []byte
	publicKey []byte
}

// NewSimpleVRFSigner creates a new SimpleVRFSigner from a 32-byte seed.
func NewSimpleVRFSigner(seed []byte) (*SimpleVRFSigner, error) {
	pk, sk, err := vrf.KeyGen(seed)
	if err != nil {
		return nil, err
	}
	return &SimpleVRFSigner{
		secretKey: sk,
		publicKey: pk,
	}, nil
}

// Prove generates a VRF proof for the given input.
func (s *SimpleVRFSigner) Prove(input []byte) ([]byte, []byte, error) {
	return vrf.Prove(s.secretKey, input)
}

// PublicKey returns the VRF verification key.
func (s *SimpleVRFSigner) PublicKey() []byte {
	return s.publicKey
}

// Destroy zeroes the secret key material. Call when the signer is no longer needed.
func (s *SimpleVRFSigner) Destroy() {
	for i := range s.secretKey {
		s.secretKey[i] = 0
	}
}

// FindNextSlotLeadership finds the next slot where the pool is eligible to be leader.
// This is useful for looking ahead to schedule block production.
//
// Note: This function uses CPRAOS consensus mode. For TPraos compatibility,
// you would need to implement a separate function or add a mode parameter.
//
// Parameters:
//   - startSlot: the slot to start searching from
//   - maxSlot: the maximum slot to search to (e.g., end of epoch)
//   - epochNonce: the epoch nonce for randomness
//   - poolStake: the pool's delegated stake
//   - totalStake: the total active stake
//   - activeSlotCoeff: the active slot coefficient
//   - vrfSigner: the VRF signer
//
// Returns the next eligible slot and proof, or (0, nil, nil) if no slot found.
func FindNextSlotLeadership(
	startSlot uint64,
	maxSlot uint64,
	epochNonce []byte,
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
	vrfSigner VRFSigner,
) (uint64, []byte, []byte, error) {
	return FindNextSlotLeadershipContext(
		context.Background(),
		startSlot,
		maxSlot,
		epochNonce,
		poolStake,
		totalStake,
		activeSlotCoeff,
		vrfSigner,
	)
}

// FindNextSlotLeadershipContext searches for leadership with cancellation and
// an inclusive maximum slot. The legacy function delegates here with a
// background context.
func FindNextSlotLeadershipContext(
	ctx context.Context,
	startSlot uint64,
	maxSlot uint64,
	epochNonce []byte,
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
	vrfSigner VRFSigner,
) (uint64, []byte, []byte, error) {
	if ctx == nil {
		return 0, nil, nil, errors.New("context is nil")
	}
	if maxSlot < startSlot {
		return 0, nil, nil, errors.New("maxSlot must not precede startSlot")
	}
	if err := ctx.Err(); err != nil {
		return 0, nil, nil, err
	}
	evaluator, _, err := prepareSlotLeadership(
		epochNonce, poolStake, totalStake, activeSlotCoeff, vrfSigner,
		ConsensusModeCPraos,
	)
	if err != nil {
		return 0, nil, nil, err
	}
	if evaluator == nil {
		return 0, nil, nil, nil
	}
	for slot := startSlot; ; slot++ {
		if err := ctx.Err(); err != nil {
			return 0, nil, nil, err
		}
		result, err := checkSlotLeadership(
			slot, epochNonce, vrfSigner, ConsensusModeCPraos, evaluator,
		)
		if err != nil {
			return 0, nil, nil, err
		}
		if result.Eligible {
			return slot, result.Proof, result.Output, nil
		}
		if slot == maxSlot {
			break
		}
	}
	return 0, nil, nil, nil
}
