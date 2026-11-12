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
	"net"
	"reflect"
	"slices"
)

// PendingPoolParametersState is an optional LedgerState capability that
// reports the pool parameters registered for a pool but not yet active,
// because a re-registration takes effect at the next epoch boundary.
//
// PoolPendingParameters returns nil when the pool has no pending parameters.
// A state that does not implement this interface cannot say whether a
// pending update exists, so PoolOwnersWitnessRequired treats every
// re-registration against it as requiring the owner witnesses.
type PendingPoolParametersState interface {
	PoolPendingParameters(PoolKeyHash) (*PoolRegistrationCertificate, error)
}

// PoolOwnerWitnessPolicy reports whether a pool registration certificate
// requires its pool owners to witness the transaction. The operator witness
// is required regardless of the policy.
type PoolOwnerWitnessPolicy func(*PoolRegistrationCertificate) bool

// PoolOwnersWitnessRequired implements the Dijkstra needed-witness rule for
// pool owners. Owners are required for a pool that is not registered at
// slot, and for a re-registration whose parameters, apart from the BLS key,
// differ from the pending parameters or, with none pending, the current
// parameters. A pool whose retirement epoch has been reached is not
// registered; one whose retirement is still pending is.
//
// It fails closed: when ls is nil or a nil pointer, cannot report the pool or
// its epoch, or does not implement PendingPoolParametersState, owners are
// required.
func PoolOwnersWitnessRequired(
	ls LedgerState,
	slot uint64,
	cert *PoolRegistrationCertificate,
) bool {
	if ls == nil || cert == nil ||
		(reflect.ValueOf(ls).Kind() == reflect.Pointer &&
			reflect.ValueOf(ls).IsNil()) {
		return true
	}
	unregistered, err := PoolRegistrationDepositDue(ls, slot, cert.Operator)
	if err != nil || unregistered {
		return true
	}
	current, _, err := ls.PoolCurrentState(cert.Operator)
	if err != nil || current == nil {
		return true
	}
	pendingState, ok := ledgerStateCapabilityFor[PendingPoolParametersState](ls)
	if !ok {
		return true
	}
	pending, err := pendingState.PoolPendingParameters(cert.Operator)
	if err != nil {
		return true
	}
	if pending != nil {
		return !poolParametersEqualExceptBlsKey(cert, pending, true)
	}
	// The active pool state keeps only the reward credential, and the
	// reference rebuilds its account with the certificate's network id.
	return !poolParametersEqualExceptBlsKey(cert, current, false)
}

func poolParametersEqualExceptBlsKey(
	a, b *PoolRegistrationCertificate,
	compareNetwork bool,
) bool {
	if compareNetwork && !rewardAccountNetworkEqual(a, b) {
		return false
	}
	if a.Operator != b.Operator ||
		a.VrfKeyHash != b.VrfKeyHash ||
		a.Pledge != b.Pledge ||
		a.Cost != b.Cost ||
		!ratEqual(a.Margin, b.Margin) ||
		!rewardAccountCredentialEqual(a, b) ||
		!poolOwnersEqual(a.PoolOwners, b.PoolOwners) ||
		!slices.EqualFunc(a.Relays, b.Relays, poolRelayEqual) {
		return false
	}
	switch {
	case a.PoolMetadata == nil || b.PoolMetadata == nil:
		return a.PoolMetadata == b.PoolMetadata
	default:
		return a.PoolMetadata.Url == b.PoolMetadata.Url &&
			bytes.Equal(
				a.PoolMetadata.Hash[:],
				b.PoolMetadata.Hash[:],
			)
	}
}

func ratEqual(a, b GenesisRat) bool {
	if a.Rat == nil || b.Rat == nil {
		return a.Rat == b.Rat
	}
	return a.Cmp(b.Rat) == 0
}

func rewardAccountCredentialEqual(a, b *PoolRegistrationCertificate) bool {
	aCred, bCred := a.RewardAccountCredential(), b.RewardAccountCredential()
	return aCred.CredType == bCred.CredType &&
		aCred.Credential == bCred.Credential
}

// rewardAccountNetworkEqual compares the network ids only when both are
// known; a certificate built without a wire header carries none.
func rewardAccountNetworkEqual(a, b *PoolRegistrationCertificate) bool {
	aNet, aKnown := a.RewardAccountNetworkId()
	bNet, bKnown := b.RewardAccountNetworkId()
	return !aKnown || !bKnown || aNet == bNet
}

// poolOwnersEqual compares owners as sets, as the ledger's owner set does.
func poolOwnersEqual(a, b []AddrKeyHash) bool {
	set := make(map[AddrKeyHash]struct{}, len(a))
	for _, owner := range a {
		set[owner] = struct{}{}
	}
	other := make(map[AddrKeyHash]struct{}, len(b))
	for _, owner := range b {
		other[owner] = struct{}{}
	}
	if len(set) != len(other) {
		return false
	}
	for owner := range set {
		if _, ok := other[owner]; !ok {
			return false
		}
	}
	return true
}

func poolRelayEqual(a, b PoolRelay) bool {
	return a.Type == b.Type &&
		ptrEqual(a.Port, b.Port) &&
		ipPtrEqual(a.Ipv4, b.Ipv4) &&
		ipPtrEqual(a.Ipv6, b.Ipv6) &&
		ptrEqual(a.Hostname, b.Hostname)
}

func ptrEqual[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func ipPtrEqual(a, b *net.IP) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
