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
	"crypto/ed25519"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/stretchr/testify/require"
)

// TestTachysEnforcementPointsFollowPerSlotLeader pins the PR claim
// "an issuer is only accepted when its pool is the leader for that exact
// slot" at the two authorisation enforcement points:
//
//   - header validation (consensus/validate.go, validateLeadership)
//   - block production (consensus/tachys.go, IsTachysSlotLeader)
//
// It fails when either enforcement point stops consulting the per-slot
// schedule.
func TestTachysEnforcementPointsFollowPerSlotLeader(t *testing.T) {
	t.Parallel()
	coldA := ed25519.NewKeyFromSeed(bytes32(0xa1))
	coldB := ed25519.NewKeyFromSeed(bytes32(0xb2))
	keyA := coldA.Public().(ed25519.PublicKey)
	keyB := coldB.Public().(ed25519.PublicKey)
	poolA := common.PoolKeyHash(common.Blake2b224Hash(keyA))
	poolB := common.PoolKeyHash(common.Blake2b224Hash(keyB))
	selector, err := NewTachysLeaderSelector(
		[]TachysPool{{ID: poolA, Stake: 1}, {ID: poolB, Stake: 1}},
		2,
	)
	require.NoError(t, err)
	nonce := common.Blake2b256Hash([]byte("tachys per-slot nonce"))

	// Find a non-zero slot where pool A is the scheduled leader. Using a
	// slot other than 0 also detects a validator that consults a fixed slot.
	var slotA uint64
	found := false
	for slot := uint64(1); slot < 1000; slot++ {
		leader, err := selector.Leader(nonce[:], slot)
		require.NoError(t, err)
		if leader == poolA {
			slotA = slot
			found = true
			break
		}
	}
	require.True(t, found, "no slot found for pool A")
	slotB := slotA

	// Block production path: pool A must not be allowed to lead slot B.
	vrfSeed := common.Blake2b256Hash([]byte("VRF seed"))
	signer, err := NewSimpleVRFSigner(vrfSeed[:])
	require.NoError(t, err)
	builderA, err := NewTachysBlockBuilder(
		signer, nil, nil, keyA, selector,
	)
	require.NoError(t, err)
	result, err := builderA.CheckSlotLeadership(slotB, nonce[:], 0, 0)
	require.NoError(t, err)
	require.True(
		t, result.Eligible,
		"builder refused the pool scheduled for the slot",
	)
	builderB, err := NewTachysBlockBuilder(
		signer, nil, nil, keyB, selector,
	)
	require.NoError(t, err)
	result, err = builderB.CheckSlotLeadership(slotB, nonce[:], 0, 0)
	require.NoError(t, err)
	require.False(
		t, result.Eligible,
		"builder allowed a pool that is not the leader for the slot",
	)

	// Header validation path: a header from pool A at slot B must fail the
	// Tachys authorisation check, and a header from pool B must pass it.
	kesSeed := common.Blake2b256Hash([]byte("KES seed"))
	kesSecret, kesPublic, err := kes.KeyGen(
		kes.CardanoKesDepth, kesSeed[:],
	)
	require.NoError(t, err)
	config := testNetworkConfig()
	denominator := new(big.Int).SetUint64(10_000_000_000_000_000_000)
	config.ActiveSlotCoeff.Rat = new(big.Rat).SetFrac(
		big.NewInt(1), denominator,
	)
	validator, err := NewTachysHeaderValidator(config, selector)
	require.NoError(t, err)

	vrfInput, err := vrf.MkInputVrf(int64(slotB), nonce[:]) //nolint:gosec
	require.NoError(t, err)
	vrfProof, vrfOutput, err := signer.Prove(vrfInput)
	require.NoError(t, err)
	message := []byte("signed header body")
	kesSignature, err := kes.Sign(kesSecret, 0, message)
	require.NoError(t, err)
	previousHash := make([]byte, 32)

	validate := func(cold ed25519.PrivateKey) *ValidateResult {
		public := cold.Public().(ed25519.PublicKey)
		opCertBody := common.OpCertSignableBytes(kesPublic, 0, 0)
		return validator.ValidateHeader(&ValidateHeaderInput{
			Slot:                 slotB,
			BlockNumber:          1,
			PrevHash:             previousHash,
			IssuerVkey:           public,
			VrfKey:               signer.PublicKey(),
			VrfProof:             vrfProof,
			VrfOutput:            vrfOutput,
			KesSignature:         kesSignature,
			HeaderBodyCbor:       message,
			OpCertHotVkey:        kesPublic,
			OpCertSequenceNumber: 0,
			OpCertKesPeriod:      0,
			OpCertSignature:      ed25519.Sign(cold, opCertBody),
			OpCertCounterState:   opCertSequence(0),
			PrevSlot:             0,
			PrevBlockNumber:      0,
			PrevHeaderHash:       previousHash,
			EpochNonce:           nonce[:],
			PoolStake:            1,
			TotalStake:           2,
			RegisteredVrfKeyHash: common.Blake2b256Hash(
				signer.PublicKey(),
			).Bytes(),
		})
	}

	res := validate(coldA)
	require.True(t, res.Valid, "%v", res.Errors)

	res = validate(coldB)
	require.False(t, res.Valid)
	require.ErrorContains(
		t, res.Errors[0], "issuer is not the scheduled Tachys leader",
	)
}
