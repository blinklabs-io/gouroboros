// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

// PoolRegistrationDepositDue reports whether a pool registration certificate
// incurs the pool deposit, which it does when the pool is not currently
// registered.
//
// A pool whose retirement has already taken effect is not currently registered,
// even though a registration certificate for it is still on record.
// PoolCurrentState returns that stale registration together with the retirement
// epoch, and reading only the registration treats the re-registration as a
// parameter update and charges nothing.
//
// On Preview, pool 864295fcb8da8782 registered at slot 16988057 and retired
// eleven slots later with a target of epoch 197. It registered again at slot
// 17020890, ninety slots into epoch 197, and the chain charged a fresh 500 ADA
// deposit. A node that skipped it computed a produced value exactly one deposit
// short and rejected the block (blinklabs-io/dingo#3908).
//
// This relies on the PoolState guarantee that a retirement superseded by a
// later registration is not returned. Without it a parameter update made after
// a re-registration would see a retirement epoch the current epoch has already
// passed and be charged a second deposit.
//
// The current epoch comes from the optional EpochState capability. Without it
// the retirement bound cannot be evaluated, so the pre-existing behaviour is
// kept: the registration on record is taken at face value. Both possible errors
// are loud -- charging a deposit that is not due and skipping one that is both
// fail conservation -- so neither silently admits an invalid transaction.
func PoolRegistrationDepositDue(
	ls LedgerState,
	slot uint64,
	operator PoolKeyHash,
) (bool, error) {
	reg, retirementEpoch, err := ls.PoolCurrentState(operator)
	if err != nil {
		return false, err
	}
	if reg == nil {
		return true, nil
	}
	if retirementEpoch == nil {
		return false, nil
	}
	epochState, ok := ls.(EpochState)
	if !ok {
		return false, nil
	}
	currentEpoch, err := epochState.EpochForSlot(slot)
	if err != nil {
		return false, err
	}
	// The retirement takes effect at the start of its target epoch, so the
	// pool is retired once the current epoch has reached it.
	return currentEpoch >= *retirementEpoch, nil
}
