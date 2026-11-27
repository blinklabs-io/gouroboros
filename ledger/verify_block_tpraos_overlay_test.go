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

package ledger_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func overlayTestKey(b byte) common.IssuerVkey {
	var key common.IssuerVkey
	copy(key[:], bytes.Repeat([]byte{b}, len(key)))
	return key
}

func overlayTestShelleyBlock(
	slot uint64,
	issuer common.IssuerVkey,
	vrfKey []byte,
) ledger.Block {
	return &shelley.ShelleyBlock{
		BlockHeader: &shelley.ShelleyBlockHeader{
			Body: shelley.ShelleyBlockHeaderBody{
				Slot:       slot,
				IssuerVkey: issuer,
				VrfKey:     vrfKey,
			},
		},
	}
}

// TestVerifyBlockStateTPraosOverlay checks the issuer of a Shelley-through-
// Alonzo block against the overlay schedule, as the reference OVERLAY rule
// does (cardano-ledger libs/cardano-protocol-tpraos/src/Cardano/Protocol/
// TPraos/Rules/Overlay.hs 219-282; isOverlaySlot in
// libs/cardano-ledger-core/src/Cardano/Ledger/Slot.hs 117-130). The slot at
// epoch offset s is an overlay slot when ceiling(s*d) < ceiling((s+1)*d); it is
// active when ceiling(s*d) is a multiple of floor(1/f), and then belongs to
// genesis key (ceiling(s*d) / floor(1/f)) mod |keys| in key-hash order. An
// active slot checks the issuer and VRF key against that genesis delegation,
// with no stake; a non-active one is rejected; any other slot is a pool slot.
// The expected verdicts below are worked from those formulas with f = 1/20.
func TestVerifyBlockStateTPraosOverlay(t *testing.T) {
	poolKey := overlayTestKey(0x01)
	poolVrf := bytes.Repeat([]byte{0x11}, 32)
	delegateA := overlayTestKey(0x02)
	delegateAVrf := bytes.Repeat([]byte{0x12}, 32)
	delegateB := overlayTestKey(0x03)
	delegateBVrf := bytes.Repeat([]byte{0x13}, 32)
	poolHash := common.Blake2b224Hash(poolKey[:])
	poolVrfHash := common.Blake2b256Hash(poolVrf)
	// Genesis key 0x0a sorts before 0x0b, so it owns the even multiples of
	// floor(1/f) and 0x0b the odd ones. They are listed in reverse to show the
	// order comes from the key hashes, not the list.
	delegations := []common.GenesisDelegation{
		{
			GenesisKeyHash:  common.NewBlake2b224(bytes.Repeat([]byte{0x0b}, 28)),
			DelegateKeyHash: common.Blake2b224Hash(delegateB[:]),
			VrfKeyHash:      common.Blake2b256Hash(delegateBVrf),
		},
		{
			GenesisKeyHash:  common.NewBlake2b224(bytes.Repeat([]byte{0x0a}, 28)),
			DelegateKeyHash: common.Blake2b224Hash(delegateA[:]),
			VrfKeyHash:      common.Blake2b256Hash(delegateAVrf),
		},
	}
	schedule := func(d *big.Rat) *common.TPraosOverlaySchedule {
		return &common.TPraosOverlaySchedule{
			Decentralization:   d,
			EpochFirstSlot:     1000,
			ActiveSlotCoeff:    big.NewRat(1, 20),
			GenesisDelegations: delegations,
		}
	}
	full := big.NewRat(1, 1)
	half := big.NewRat(1, 2)
	none := new(big.Rat)

	for _, tc := range []struct {
		name     string
		overlay  *common.TPraosOverlaySchedule
		slot     uint64
		issuer   common.IssuerVkey
		vrfKey   []byte
		wantType common.ValidationErrorType
	}{
		// d = 1: offset 0 is position 0, active, genesis key index 0.
		{name: "active slot, scheduled delegate", overlay: schedule(full),
			slot: 1000, issuer: delegateA, vrfKey: delegateAVrf},
		// Offset 20 is position 20 = 1*20, genesis key index 1.
		{name: "active slot, second genesis key", overlay: schedule(full),
			slot: 1020, issuer: delegateB, vrfKey: delegateBVrf},
		{name: "active slot, other genesis key's delegate",
			overlay: schedule(full), slot: 1020, issuer: delegateA,
			vrfKey: delegateAVrf, wantType: common.ValidationErrorTypeStakePool},
		{name: "active slot, pool issuer", overlay: schedule(full),
			slot: 1000, issuer: poolKey, vrfKey: poolVrf,
			wantType: common.ValidationErrorTypeStakePool},
		{name: "active slot, wrong VRF key", overlay: schedule(full),
			slot: 1000, issuer: delegateA, vrfKey: delegateBVrf,
			wantType: common.ValidationErrorTypeVRF},
		// Offset 1 is position 1, not a multiple of 20.
		{name: "non-active slot, pool issuer", overlay: schedule(full),
			slot: 1001, issuer: poolKey, vrfKey: poolVrf,
			wantType: common.ValidationErrorTypeStakePool},
		{name: "non-active slot, delegate issuer", overlay: schedule(full),
			slot: 1001, issuer: delegateA, vrfKey: delegateAVrf,
			wantType: common.ValidationErrorTypeStakePool},
		// d = 1/2: offset 1 has ceiling(1/2) = ceiling(1) = 1, not overlay.
		{name: "pool slot, pool issuer", overlay: schedule(half),
			slot: 1001, issuer: poolKey, vrfKey: poolVrf},
		{name: "pool slot, delegate issuer", overlay: schedule(half),
			slot: 1001, issuer: delegateA, vrfKey: delegateAVrf,
			wantType: common.ValidationErrorTypeStakePool},
		// d = 1/2: offset 0 has ceiling(0) = 0 < ceiling(1/2) = 1, active.
		{name: "half, active slot, delegate", overlay: schedule(half),
			slot: 1000, issuer: delegateA, vrfKey: delegateAVrf},
		{name: "d = 0, pool issuer", overlay: schedule(none),
			slot: 1000, issuer: poolKey, vrfKey: poolVrf},
		{name: "d = 0, no delegations, pool issuer",
			overlay: &common.TPraosOverlaySchedule{
				Decentralization: none, EpochFirstSlot: 1000,
				GenesisDelegations: []common.GenesisDelegation{},
			}, slot: 1000, issuer: poolKey, vrfKey: poolVrf},
		{name: "missing schedule", slot: 1000, issuer: poolKey,
			vrfKey: poolVrf, wantType: common.ValidationErrorTypeConfiguration},
		{name: "slot before epoch", overlay: schedule(full), slot: 999,
			issuer: poolKey, vrfKey: poolVrf,
			wantType: common.ValidationErrorTypeConfiguration},
		{name: "d above one",
			overlay: &common.TPraosOverlaySchedule{
				Decentralization: big.NewRat(3, 2), EpochFirstSlot: 1000,
				ActiveSlotCoeff:    big.NewRat(1, 20),
				GenesisDelegations: delegations,
			}, slot: 1000, issuer: poolKey, vrfKey: poolVrf,
			wantType: common.ValidationErrorTypeConfiguration},
		{name: "d positive, no delegations",
			overlay: &common.TPraosOverlaySchedule{
				Decentralization: full, EpochFirstSlot: 1000,
				ActiveSlotCoeff: big.NewRat(1, 20),
			}, slot: 1000, issuer: delegateA, vrfKey: delegateAVrf,
			wantType: common.ValidationErrorTypeConfiguration},
		{name: "d positive, no active slot coefficient",
			overlay: &common.TPraosOverlaySchedule{
				Decentralization: full, EpochFirstSlot: 1000,
				GenesisDelegations: delegations,
			}, slot: 1000, issuer: delegateA, vrfKey: delegateAVrf,
			wantType: common.ValidationErrorTypeConfiguration},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := common.VerifyConfig{
				SkipTransactionValidation: true,
				SkipBlockLimitsValidation: true,
				TPraosOverlay:             tc.overlay,
				// The pool distribution holds only the pool, as the
				// field's contract requires: genesis delegates are not
				// pools.
				LeaderPoolVrfKeyHash: func(
					_ uint64,
					pool common.PoolKeyHash,
				) (*common.Blake2b256, error) {
					if pool == poolHash {
						return &poolVrfHash, nil
					}
					return nil, nil
				},
			}
			err := ledger.VerifyBlockState(
				overlayTestShelleyBlock(tc.slot, tc.issuer, tc.vrfKey),
				config,
			)
			if tc.wantType == "" {
				require.NoError(t, err)
				return
			}
			var validationErr *common.ValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, tc.wantType, validationErr.Type)
		})
	}
}

// Praos blocks have no overlay, so they need no schedule.
func TestVerifyBlockStatePraosNeedsNoOverlay(t *testing.T) {
	issuer := overlayTestKey(0x01)
	vrfKey := bytes.Repeat([]byte{0x11}, 32)
	vrfHash := common.Blake2b256Hash(vrfKey)
	block := &babbage.BabbageBlock{
		BlockHeader: &babbage.BabbageBlockHeader{
			Body: babbage.BabbageBlockHeaderBody{
				Slot:       1000,
				IssuerVkey: issuer,
				VrfKey:     vrfKey,
			},
		},
	}
	require.NoError(t, ledger.VerifyBlockState(block, common.VerifyConfig{
		SkipTransactionValidation: true,
		SkipBlockLimitsValidation: true,
		LeaderPoolVrfKeyHash: func(
			uint64,
			common.PoolKeyHash,
		) (*common.Blake2b256, error) {
			return &vrfHash, nil
		},
	}))
}
