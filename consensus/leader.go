// Copyright 2025 Blink Labs Software
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
	// Threshold is the leadership threshold that was used
	Threshold *big.Int
}

// IsSlotLeader checks if a pool is eligible to produce a block in the given slot
// using CPRAOS consensus. For TPraos compatibility, use IsSlotLeaderWithMode.
//
// The CPRAOS algorithm:
//  1. Compute VRF input using MkInputVrf(slot, epochNonce)
//  2. Generate VRF proof: proof, output = vrfSigner.Prove(input)
//  3. Compute leader value: BLAKE2b-256("L" || output)
//  4. Compute threshold: T = 2^256 * (1 - (1-f)^(poolStake/totalStake))
//  5. Compare: eligible = leaderValue < T
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
	return IsSlotLeaderWithMode(slot, epochNonce, poolStake, totalStake, activeSlotCoeff, vrfSigner, ConsensusModeCPraos)
}

// IsSlotLeaderWithMode checks if a pool is eligible to produce a block in the given slot
// using the specified consensus mode.
//
// For CPRAOS (Babbage+):
//  1. Compute VRF input using MkInputVrf(slot, epochNonce)
//  2. Generate VRF proof: proof, output = vrfSigner.Prove(input)
//  3. Compute leader value: BLAKE2b-256("L" || output)
//  4. Compute threshold: T = 2^256 * (1 - (1-f)^σ)
//  5. Compare: eligible = leaderValue < T
//
// For TPraos (Shelley-Alonzo):
//  1. Compute VRF input using MkInputVrf(slot, epochNonce)
//  2. Generate VRF proof: proof, output = vrfSigner.Prove(input)
//  3. Use raw 64-byte VRF output directly
//  4. Compute threshold: T = 2^512 * (1 - (1-f)^σ)
//  5. Compare: eligible = output < T
func IsSlotLeaderWithMode(
	slot uint64,
	epochNonce []byte,
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
	vrfSigner VRFSigner,
	mode ConsensusMode,
) (*LeaderElectionResult, error) {
	if vrfSigner == nil {
		return nil, errors.New("vrfSigner is nil")
	}
	// Validate epochNonce is exactly 32 bytes to prevent panic in vrf.MkInputVrf
	if len(epochNonce) != 32 {
		return nil, fmt.Errorf(
			"epochNonce must be 32 bytes, got %d",
			len(epochNonce),
		)
	}
	if activeSlotCoeff == nil {
		return nil, errors.New("activeSlotCoeff is nil")
	}
	if totalStake == 0 {
		return &LeaderElectionResult{
			Eligible:  false,
			Threshold: big.NewInt(0),
		}, nil
	}
	if poolStake == 0 {
		return &LeaderElectionResult{
			Eligible:  false,
			Threshold: big.NewInt(0),
		}, nil
	}

	// Step 1: Compute VRF input
	// Slot numbers in Cardano are far below int64 max (mainnet ~100M, max ~9.2 quintillion)
	vrfInput := vrf.MkInputVrf(
		int64(slot), //nolint:gosec
		epochNonce,
	)

	// Step 2: Generate VRF proof
	proof, output, err := vrfSigner.Prove(vrfInput)
	if err != nil {
		return nil, err
	}
	if len(proof) != 80 {
		return nil, fmt.Errorf(
			"VRF proof: expected 80 bytes, got %d",
			len(proof),
		)
	}
	if len(output) != 64 {
		return nil, fmt.Errorf(
			"VRF output: expected 64 bytes, got %d",
			len(output),
		)
	}

	// Step 3: Compute threshold (mode-aware)
	threshold := CertifiedNatThresholdWithMode(poolStake, totalStake, activeSlotCoeff, mode)

	// Step 4: Check eligibility (mode-aware)
	eligible := IsVRFOutputBelowThresholdWithMode(output, threshold, mode)

	return &LeaderElectionResult{
		Eligible:  eligible,
		Proof:     proof,
		Output:    output,
		Threshold: threshold,
	}, nil
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
	return IsSlotLeaderFromComponentsWithMode(vrfOutput, poolStake, totalStake, activeSlotCoeff, ConsensusModeCPraos)
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
// Returns true if the pool is eligible to be the slot leader.
func IsSlotLeaderFromComponentsWithMode(
	vrfOutput []byte,
	poolStake uint64,
	totalStake uint64,
	activeSlotCoeff *big.Rat,
	mode ConsensusMode,
) bool {
	if activeSlotCoeff == nil || totalStake == 0 || poolStake == 0 {
		return false
	}
	if len(vrfOutput) != 64 {
		return false
	}

	threshold := CertifiedNatThresholdWithMode(poolStake, totalStake, activeSlotCoeff, mode)
	return IsVRFOutputBelowThresholdWithMode(vrfOutput, threshold, mode)
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
	if maxSlot == math.MaxUint64 {
		return 0, nil, nil, errors.New(
			"maxSlot cannot be math.MaxUint64 to prevent overflow",
		)
	}
	for slot := startSlot; slot <= maxSlot; slot++ {
		result, err := IsSlotLeader(
			slot,
			epochNonce,
			poolStake,
			totalStake,
			activeSlotCoeff,
			vrfSigner,
		)
		if err != nil {
			return 0, nil, nil, err
		}
		if result.Eligible {
			return slot, result.Proof, result.Output, nil
		}
	}
	return 0, nil, nil, nil
}
