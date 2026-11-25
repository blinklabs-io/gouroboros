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

package common

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"slices"
)

// TPraosOverlaySchedule carries what the TPraos overlay schedule of a
// Shelley-through-Alonzo block's epoch depends on.
type TPraosOverlaySchedule struct {
	// Decentralization is the protocol parameter d in effect at the block's
	// slot, between 0 and 1. With d = 0 there is no overlay.
	Decentralization *big.Rat
	// EpochFirstSlot is the first slot of the epoch that contains the
	// block's slot.
	EpochFirstSlot uint64
	// ActiveSlotCoeff is the Shelley genesis active slot coefficient f. It
	// may be nil when Decentralization is 0.
	ActiveSlotCoeff *big.Rat
	// GenesisDelegations holds one delegation per genesis key, as the ledger
	// has it at the block's slot. It may be empty only when Decentralization
	// is 0.
	GenesisDelegations []GenesisDelegation
}

// OverlaySlotKind is a slot's place in the TPraos overlay schedule.
type OverlaySlotKind int

const (
	// OverlaySlotNone is a slot outside the overlay schedule, led by pools.
	OverlaySlotNone OverlaySlotKind = iota
	// OverlaySlotActive is an overlay slot assigned to a genesis key.
	OverlaySlotActive
	// OverlaySlotNonActive is an overlay slot assigned to no one, in which no
	// block may be made.
	OverlaySlotNonActive
)

// Classify places slot in the overlay schedule, as isOverlaySlot
// (cardano-ledger Cardano/Ledger/Slot.hs) and classifyOverlaySlot
// (Cardano/Protocol/TPraos/Rules/Overlay.hs) do. The slot at epoch offset s
// is in the schedule when ceiling(s*d) < ceiling((s+1)*d); it is active when
// ceiling(s*d) is a multiple of floor(1/f), and then belongs to the genesis
// key at index (ceiling(s*d) / floor(1/f)) mod |keys| in byte order of the
// genesis key hashes. The delegation is returned only for an active slot.
// An error reports an unusable schedule, not a verdict on the slot.
func (s *TPraosOverlaySchedule) Classify(
	slot uint64,
) (OverlaySlotKind, GenesisDelegation, error) {
	d := s.Decentralization
	switch {
	case d == nil:
		return 0, GenesisDelegation{}, errors.New("decentralization is nil")
	case d.Sign() < 0 || d.Cmp(big.NewRat(1, 1)) > 0:
		return 0, GenesisDelegation{}, fmt.Errorf(
			"decentralization %s is outside [0, 1]", d.RatString(),
		)
	case slot < s.EpochFirstSlot:
		return 0, GenesisDelegation{}, fmt.Errorf(
			"epoch first slot %d is after slot %d", s.EpochFirstSlot, slot,
		)
	case d.Sign() == 0:
		return OverlaySlotNone, GenesisDelegation{}, nil
	case len(s.GenesisDelegations) == 0:
		return 0, GenesisDelegation{}, errors.New(
			"no genesis delegations while decentralization is positive",
		)
	}
	f := s.ActiveSlotCoeff
	if f == nil || f.Sign() <= 0 || f.Cmp(big.NewRat(1, 1)) > 0 {
		return 0, GenesisDelegation{}, errors.New(
			"active slot coefficient is missing or outside (0, 1]",
		)
	}
	offset := slot - s.EpochFirstSlot
	position := ceilMulRat(offset, d)
	if position.Cmp(ceilMulRat(offset+1, d)) >= 0 {
		return OverlaySlotNone, GenesisDelegation{}, nil
	}
	ascInv := new(big.Int).Quo(f.Denom(), f.Num())
	index, rem := new(big.Int).QuoRem(position, ascInv, new(big.Int))
	if rem.Sign() != 0 {
		return OverlaySlotNonActive, GenesisDelegation{}, nil
	}
	keys := slices.Clone(s.GenesisDelegations)
	slices.SortFunc(keys, func(a, b GenesisDelegation) int {
		return bytes.Compare(a.GenesisKeyHash[:], b.GenesisKeyHash[:])
	})
	for i := 1; i < len(keys); i++ {
		if keys[i].GenesisKeyHash == keys[i-1].GenesisKeyHash {
			return 0, GenesisDelegation{}, fmt.Errorf(
				"genesis key %s is delegated twice",
				keys[i].GenesisKeyHash.String(),
			)
		}
	}
	index.Mod(index, big.NewInt(int64(len(keys))))
	return OverlaySlotActive, keys[index.Int64()], nil
}

// ceilMulRat returns ceiling(n * r) for r >= 0.
func ceilMulRat(n uint64, r *big.Rat) *big.Int {
	num := new(big.Int).Mul(new(big.Int).SetUint64(n), r.Num())
	den := r.Denom()
	num.Add(num, den)
	num.Sub(num, big.NewInt(1))
	return num.Quo(num, den)
}
