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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test seed (exactly 32 bytes)
var testVRFSeed = []byte("test_vrf_seed_for_consensus!!!!!")

// policyVRFSigner deliberately returns configurable non-cryptographic data.
// It is used only to exercise invalid-input adapter validation.
type policyVRFSigner struct {
	proof  []byte
	output []byte
	err    error
	calls  int
}

func (s *policyVRFSigner) Prove([]byte) ([]byte, []byte, error) {
	s.calls++
	return s.proof, s.output, s.err
}

func (*policyVRFSigner) PublicKey() []byte {
	return make([]byte, 32)
}

func TestNewSimpleVRFSigner(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err, "NewSimpleVRFSigner failed")

	assert.Len(t, signer.PublicKey(), 32, "expected 32-byte public key")
}

func TestSimpleVRFSignerProve(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err, "NewSimpleVRFSigner failed")

	input := []byte("test input")
	proof, output, err := signer.Prove(input)
	require.NoError(t, err, "Prove failed")

	assert.Len(t, proof, 80, "expected 80-byte proof")
	assert.Len(t, output, 64, "expected 64-byte output")
}

func TestIsSlotLeader(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}

	// Test with mainnet-like parameters (f = 0.05)
	activeSlotCoeff := big.NewRat(1, 20)

	// A positive-stake decision retains its generated proof and output.
	result, err := IsSlotLeader(
		1000, // slot
		epochNonce,
		1000000000000, // poolStake = 100% of total
		1000000000000, // totalStake
		activeSlotCoeff,
		signer,
	)
	if err != nil {
		t.Fatalf("IsSlotLeader failed: %v", err)
	}

	// With 100% stake, probability ≈ 0.05, so not guaranteed but likely
	// We just check that the function runs correctly
	require.Len(t, result.Proof, 80)
	require.Len(t, result.Output, 64)
}

func TestIsSlotLeaderZeroStake(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	activeSlotCoeff := big.NewRat(1, 20)

	// Zero pool stake should never be eligible
	result, err := IsSlotLeader(
		1000,
		epochNonce,
		0,             // poolStake = 0
		1000000000000, // totalStake
		activeSlotCoeff,
		signer,
	)
	if err != nil {
		t.Fatalf("IsSlotLeader failed: %v", err)
	}

	if result.Eligible {
		t.Error("pool with zero stake should not be eligible")
	}
}

func TestIsSlotLeaderZeroTotalStake(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	activeSlotCoeff := big.NewRat(1, 20)

	// Zero total stake should return not eligible (edge case)
	result, err := IsSlotLeader(
		1000,
		epochNonce,
		1000,
		0, // totalStake = 0
		activeSlotCoeff,
		signer,
	)
	if err != nil {
		t.Fatalf("IsSlotLeader failed: %v", err)
	}

	if result.Eligible {
		t.Error("should not be eligible with zero total stake")
	}
}

func TestIsSlotLeaderNilSigner(t *testing.T) {
	epochNonce := make([]byte, 32)
	activeSlotCoeff := big.NewRat(1, 20)

	_, err := IsSlotLeader(
		1000,
		epochNonce,
		1000,
		10000,
		activeSlotCoeff,
		nil,
	)
	if err == nil {
		t.Error("expected error for nil signer")
	}
}

func TestIsSlotLeaderEmptyNonce(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	activeSlotCoeff := big.NewRat(1, 20)

	_, err = IsSlotLeader(
		1000,
		[]byte{}, // empty nonce
		1000,
		10000,
		activeSlotCoeff,
		signer,
	)
	if err == nil {
		t.Error("expected error for empty epoch nonce")
	}
}

func TestIsSlotLeaderNilActiveSlotCoeff(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)

	_, err = IsSlotLeader(
		1000,
		epochNonce,
		1000,
		10000,
		nil, // nil activeSlotCoeff
		signer,
	)
	if err == nil {
		t.Error("expected error for nil activeSlotCoeff")
	}
}

func TestIsSlotLeaderDeterminism(t *testing.T) {
	signer1, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("failed to create signer1: %v", err)
	}
	signer2, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("failed to create signer2: %v", err)
	}

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}
	activeSlotCoeff := big.NewRat(1, 20)

	result1, err := IsSlotLeader(
		12345,
		epochNonce,
		1000000,
		10000000,
		activeSlotCoeff,
		signer1,
	)
	if err != nil {
		t.Fatalf("IsSlotLeader failed: %v", err)
	}

	result2, err := IsSlotLeader(
		12345,
		epochNonce,
		1000000,
		10000000,
		activeSlotCoeff,
		signer2,
	)
	if err != nil {
		t.Fatalf("IsSlotLeader failed: %v", err)
	}

	// Results should be identical for same inputs
	if result1.Eligible != result2.Eligible {
		t.Error("leader election should be deterministic")
	}
}

func TestFindNextSlotLeadership(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i * 3)
	}
	activeSlotCoeff := big.NewRat(
		9,
		10,
	) // 90% active slot coefficient for faster tests

	// With 100% stake and these fixed inputs, slot 1 is eligible.
	slot, proof, output, err := FindNextSlotLeadership(
		1,   // start slot
		100, // max slot (reduced from 1000)
		epochNonce,
		10000000000, // poolStake = 100%
		10000000000, // totalStake
		activeSlotCoeff,
		signer,
	)
	if err != nil {
		t.Fatalf("FindNextSlotLeadership failed: %v", err)
	}

	// Slot 1 is deterministic for these fixed inputs; require the search to
	// return it rather than silently accepting an untested no-leader result.
	require.Equal(t, uint64(1), slot)
	require.NotNil(t, proof)
	require.NotNil(t, output)
}

func TestFindNextSlotLeadershipNoEligibility(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	activeSlotCoeff := big.NewRat(1, 20)

	// With zero stake, should never find eligibility
	slot, proof, output, err := FindNextSlotLeadership(
		1,
		100,
		epochNonce,
		0,          // zero stake
		1000000000, // total stake
		activeSlotCoeff,
		signer,
	)
	if err != nil {
		t.Fatalf("FindNextSlotLeadership failed: %v", err)
	}

	if slot != 0 || proof != nil || output != nil {
		t.Error("should not find eligibility with zero stake")
	}
}

func TestFindNextSlotLeadershipContextAllowsLargeRange(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err)
	startSlot := uint64(42)
	maxSlot := startSlot + 1_000_000
	slot, proof, output, err := FindNextSlotLeadershipContext(
		context.Background(),
		startSlot,
		maxSlot,
		make([]byte, 32),
		1,
		1,
		big.NewRat(1, 1),
		signer,
	)
	require.NoError(t, err)
	require.Equal(t, startSlot, slot)
	require.NotNil(t, proof)
	require.NotNil(t, output)
}

func TestFindNextSlotLeadershipContextHonorsCancellation(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err = FindNextSlotLeadershipContext(
		ctx,
		0,
		10,
		make([]byte, 32),
		1,
		1,
		big.NewRat(1, 2),
		signer,
	)
	require.ErrorIs(t, err, context.Canceled)
}

func TestFindNextSlotLeadershipContextHonorsCancellationForLargeRange(
	t *testing.T,
) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	startSlot := uint64(42)
	_, _, _, err = FindNextSlotLeadershipContext(
		ctx,
		startSlot,
		startSlot+1_000_000,
		make([]byte, 32),
		1,
		1,
		big.NewRat(1, 1),
		signer,
	)
	require.ErrorIs(t, err, context.Canceled)
}

func TestFindNextSlotLeadershipContextHandlesMaxUint64Boundary(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	slot, proof, output, err := FindNextSlotLeadershipContext(
		ctx,
		math.MaxUint64,
		math.MaxUint64,
		make([]byte, 32),
		0,
		1,
		big.NewRat(1, 2),
		signer,
	)
	require.NoError(t, err)
	require.Zero(t, slot)
	require.Nil(t, proof)
	require.Nil(t, output)
}

func TestIsSlotLeaderWithModeSlotOverflow(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err)

	epochNonce := make([]byte, 32)
	activeSlotCoeff := big.NewRat(1, 20)

	_, err = IsSlotLeaderWithMode(
		math.MaxUint64, // overflows int64
		epochNonce,
		1000,
		10000,
		activeSlotCoeff,
		signer,
		ConsensusModeCPraos,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum int64 value")
}

func TestIsSlotLeaderWithModeUnknownMode(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err)

	epochNonce := make([]byte, 32)
	activeSlotCoeff := big.NewRat(1, 20)

	_, err = IsSlotLeaderWithMode(
		1000,
		epochNonce,
		1000,
		10000,
		activeSlotCoeff,
		signer,
		ConsensusMode(99), // unknown mode
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown consensus mode")
}

func TestLeaderEligibilityComponentPolicies(t *testing.T) {
	tiny := new(
		big.Rat,
	).SetFrac(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil))
	for _, mode := range []ConsensusMode{ConsensusModeCPraos, ConsensusModeTPraos} {
		for _, tc := range []struct {
			name        string
			pool, total uint64
			f           *big.Rat
			length      int
			wantError   bool
		}{
			{"nil coefficient", 1, 1, nil, 64, false},
			{"zero coefficient", 1, 1, big.NewRat(0, 1), 64, false},
			{"negative coefficient", 1, 1, big.NewRat(-1, 20), 64, false},
			{"zero pool even unconditional", 0, 1, big.NewRat(1, 1), 64, false},
			{"zero total", 1, 0, big.NewRat(1, 20), 64, false},
			{"short output even unconditional", 1, 1, big.NewRat(1, 1), 63, false},
			{"long output", 1, 1, big.NewRat(1, 20), 65, false},
			{"above one zero pool", 0, 1, big.NewRat(3, 2), 64, true},
			{"below domain zero total", 1, 0, tiny, 64, true},
			{"unsupported malformed output", 1, 1, tiny, 0, true},
		} {
			t.Run(tc.name+fmt.Sprint(mode), func(t *testing.T) {
				got, err := IsSlotLeaderFromComponentsWithMode(
					make([]byte, tc.length),
					tc.pool,
					tc.total,
					tc.f,
					mode,
				)
				require.False(t, got)
				if tc.wantError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				require.False(
					t,
					IsSlotLeaderFromComponents(
						make([]byte, tc.length),
						tc.pool,
						tc.total,
						tc.f,
					),
				)
			})
		}
	}
	got, err := IsSlotLeaderFromComponentsWithMode(
		nil,
		0,
		0,
		nil,
		ConsensusMode(99),
	)
	require.Error(t, err)
	require.False(t, got)
}

func TestLeaderEligibilityElectionPolicies(t *testing.T) {
	nonce := make([]byte, 32)
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err)
	tiny := new(
		big.Rat,
	).SetFrac(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil))
	for _, mode := range []ConsensusMode{ConsensusModeCPraos, ConsensusModeTPraos} {
		for _, stake := range [][2]uint64{{0, 1}, {1, 0}, {0, 0}} {
			result, err := IsSlotLeaderWithMode(
				math.MaxUint64,
				nonce,
				stake[0],
				stake[1],
				big.NewRat(1, 1),
				signer,
				mode,
			)
			require.NoError(t, err)
			require.False(t, result.Eligible)
			require.Nil(t, result.Proof)
			require.Nil(t, result.Output)
			for _, f := range []*big.Rat{tiny, big.NewRat(3, 2), nil} {
				_, err := IsSlotLeaderWithMode(
					0,
					nonce,
					stake[0],
					stake[1],
					f,
					signer,
					mode,
				)
				require.Error(t, err)
			}
			_, err = IsSlotLeaderWithMode(
				0,
				nonce,
				stake[0],
				stake[1],
				big.NewRat(1, 20),
				nil,
				mode,
			)
			require.Error(t, err)
			_, err = IsSlotLeaderWithMode(
				0,
				nil,
				stake[0],
				stake[1],
				big.NewRat(1, 20),
				signer,
				mode,
			)
			require.Error(t, err)
		}
		for _, f := range []*big.Rat{big.NewRat(0, 1), big.NewRat(-1, 20)} {
			result, err := IsSlotLeaderWithMode(
				42,
				nonce,
				1,
				1,
				f,
				signer,
				mode,
			)
			require.NoError(t, err)
			require.False(t, result.Eligible)
			require.Len(t, result.Proof, 80)
			require.Len(t, result.Output, 64)
			signerFailure := errors.New("signing failed")
			for _, bad := range []*policyVRFSigner{
				{proof: make([]byte, 79), output: make([]byte, 64)},
				{proof: make([]byte, 80), output: make([]byte, 63)},
				{err: signerFailure},
			} {
				_, err := IsSlotLeaderWithMode(42, nonce, 1, 1, f, bad, mode)
				require.Error(t, err)
				require.Equal(t, 1, bad.calls)
				if bad.err != nil {
					require.ErrorIs(t, err, signerFailure)
				}
			}
		}
	}
	_, err = IsSlotLeaderWithMode(0, nil, 0, 0, nil, nil, ConsensusMode(99))
	require.ErrorContains(t, err, "unknown consensus mode")
}

type cancelAfterProofSigner struct {
	VRFSigner
	cancel context.CancelFunc
	calls  int
}

func (s *cancelAfterProofSigner) Prove(input []byte) ([]byte, []byte, error) {
	s.calls++
	proof, output, err := s.VRFSigner.Prove(input)
	s.cancel()
	return proof, output, err
}

func TestLeaderEligibilitySearchPolicies(t *testing.T) {
	signer, err := NewSimpleVRFSigner(make([]byte, 32))
	require.NoError(t, err)
	nonce := make([]byte, 32)
	_, _, _, err = FindNextSlotLeadershipContext(
		nil,
		0,
		1,
		nonce,
		1,
		3,
		big.NewRat(1, 20),
		signer,
	)
	require.Error(t, err)
	_, _, _, err = FindNextSlotLeadershipContext(
		context.Background(),
		2,
		1,
		nonce,
		1,
		3,
		big.NewRat(1, 20),
		signer,
	)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Cancellation wins over invalid preparation inputs.
	_, _, _, err = FindNextSlotLeadershipContext(ctx, 0, 1, nil, 1, 1, nil, nil)
	require.ErrorIs(t, err, context.Canceled)
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	counted := &cancelAfterProofSigner{VRFSigner: signer, cancel: cancel}
	// Shared Praos schedule: first slot is unassigned. Cancellation after
	// its legitimate proof must stop before signing the next slot.
	_, _, _, err = FindNextSlotLeadershipContext(
		ctx,
		10000000,
		10000119,
		nonce,
		1,
		3,
		big.NewRat(1, 20),
		counted,
	)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, counted.calls)
	// The shared Praos schedule's first assigned slot is the inclusive max.
	slot, proof, output, err := FindNextSlotLeadershipContext(
		context.Background(),
		10000000,
		10000054,
		nonce,
		1,
		3,
		big.NewRat(1, 20),
		signer,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(10000054), slot)
	require.Len(t, proof, 80)
	require.Len(t, output, 64)
	slot, proof, output, err = FindNextSlotLeadershipContext(
		context.Background(),
		10000000,
		10000053,
		nonce,
		1,
		3,
		big.NewRat(1, 20),
		signer,
	)
	require.NoError(t, err)
	require.Zero(t, slot)
	require.Nil(t, proof)
	require.Nil(t, output)
	// Positive stake reaches input validation, rather than wrapping at max.
	_, _, _, err = FindNextSlotLeadershipContext(
		context.Background(),
		math.MaxUint64,
		math.MaxUint64,
		nonce,
		1,
		3,
		big.NewRat(1, 20),
		signer,
	)
	require.Error(t, err)
}

func TestLeaderEligibilityElectionReferenceDecisions(t *testing.T) {
	// Independently recorded shared 120-slot node schedules, seed/nonce zero,
	// poolStake=1, activeStake=3, f=1/20, d=0.
	signer, err := NewSimpleVRFSigner(make([]byte, 32))
	require.NoError(t, err)
	for _, tc := range []struct {
		mode ConsensusMode
		slot uint64
		want bool
	}{
		{ConsensusModeCPraos, 10000054, true},
		{ConsensusModeCPraos, 10000005, false},
		{ConsensusModeTPraos, 10000005, true},
		{ConsensusModeTPraos, 10000054, false},
	} {
		result, err := IsSlotLeaderWithMode(
			tc.slot,
			make([]byte, 32),
			1,
			3,
			big.NewRat(1, 20),
			signer,
			tc.mode,
		)
		require.NoError(t, err)
		require.Equal(t, tc.want, result.Eligible)
		require.Len(t, result.Proof, 80)
		require.Len(t, result.Output, 64)
		got, err := IsSlotLeaderFromComponentsWithMode(
			result.Output,
			1,
			3,
			big.NewRat(1, 20),
			tc.mode,
		)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
		if tc.mode == ConsensusModeCPraos {
			require.Equal(
				t,
				tc.want,
				IsSlotLeaderFromComponents(
					result.Output,
					1,
					3,
					big.NewRat(1, 20),
				),
			)
		}
	}
}
