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

// PoolRegistered reports whether pool is in the current stake-pool map, which
// is the one question the reference asks wherever a rule needs a registered
// pool: delegation (DELEG), retirement (POOL), the deposit of a registration,
// and a stake-pool voter (GOV). Every such rule in gOuroboros decides it here,
// from PoolCurrentState, and none trusts IsPoolRegistered on its own.
//
// A pool is registered when PoolCurrentState returns a registration and any
// pending retirement it reports has not been reached. POOLREAP removes a
// retiring pool at the start of its retirement epoch, but a provider may still
// return the reaped pool's last registration together with that retirement
// epoch, and reading only the registration would let a transaction delegate
// to, retire, or vote as a pool that no longer exists.
//
// On Preview, pool 864295fcb8da8782 registered at slot 16988057 and retired
// eleven slots later with a target of epoch 197. It registered again at slot
// 17020890, ninety slots into epoch 197, and the chain charged a fresh 500 ADA
// deposit, because the pool was no longer registered. A node that read only
// the stale registration skipped the deposit, computed a produced value
// exactly one deposit short, and rejected the block.
//
// This relies on the PoolState guarantee that a retirement superseded by a
// later registration is not returned. Without it a pool re-registered after a
// retirement would be treated as reaped once that retirement epoch passed.
//
// The current epoch comes from EpochState. A pending retirement cannot be
// classified without it, so the decision fails closed when it is unavailable.
func PoolRegistered(
	ls LedgerState,
	slot uint64,
	pool PoolKeyHash,
) (bool, error) {
	reg, retirementEpoch, err := ls.PoolCurrentState(pool)
	if err != nil {
		return false, err
	}
	if reg == nil {
		return false, nil
	}
	if retirementEpoch == nil {
		return true, nil
	}
	epochState, ok := EpochStateFor(ls)
	if !ok {
		return false, EpochStateUnavailableError{}
	}
	currentEpoch, err := epochState.EpochForSlot(slot)
	if err != nil {
		return false, err
	}
	return currentEpoch < *retirementEpoch, nil
}

// PoolRegistrationDepositDue reports whether a pool registration certificate
// incurs the pool deposit, which it does when the pool is not registered as
// PoolRegistered decides.
func PoolRegistrationDepositDue(
	ls LedgerState,
	slot uint64,
	operator PoolKeyHash,
) (bool, error) {
	registered, err := PoolRegistered(ls, slot, operator)
	if err != nil {
		return false, err
	}
	return !registered, nil
}
