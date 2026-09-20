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

package conway

import "github.com/blinklabs-io/gouroboros/ledger/common"

// conwayCertsOverlay is a transaction-scoped, read-only view of the
// certificate-derived ledger state that a transaction's own certificates
// would produce if applied sequentially, ahead of the transaction's own
// governance predicates.
//
// Reference: Cardano.Ledger.Conway.Rules.Ledger.conwayLedgerTransitionTRC
// (eras/conway/impl/src/Cardano/Ledger/Conway/Rules/Ledger.hs) runs the
// CERTS transition before the GOV transition and passes GOV the resulting
// CertState (certStateAfterCERTS), so a certificate earlier in a
// transaction's own certificate list is visible to that same transaction's
// governance checks (gouroboros#2386). Withdrawals are drained from account
// balances before CERTS runs, using the *pre*-CERTS account state
// (validateWithdrawalsDelegated is called against `accounts` before
// `certState'` folds in the certificates) -- this overlay does not model
// balances or withdrawals at all, and UtxoValidateWithdrawals continues to
// validate directly against ls, preserving that ordering.
//
// newConwayCertsOverlay is the single implementation shared by every
// governance predicate that needs post-CERTS visibility
// (UtxoValidateProposalReturnAccounts, UtxoValidateUnknownVoters), rather
// than each predicate re-deriving its own in-transaction certificate
// bookkeeping. Each predicate constructs its own overlay instance -- the
// (tx, slot, ls, pp) validation-rule signature carries no state across
// sibling rule invocations -- but all instances are built by this one
// function.
//
// The overlay never mutates ls: every method either answers from its own
// maps or delegates to ls unchanged, so a rejected transaction leaves no
// trace and ls itself is never written to.
//
// Certificate legality (whether a given certificate is itself allowed
// given the certificates before it in the same transaction, e.g. deposit
// amounts, double registration, resigned-member authorization) is
// out of scope: that is validated separately by UtxoValidateDelegation,
// UtxoValidateCertificateDeposits, and UtxoValidateCommitteeCertificates
// (individual certificate-legality issues gouroboros#2384, #4377, #4433).
// This overlay only answers "what would the registration/authorization
// state be after this transaction's certificates ran", for predicates that
// read that state, not certificates themselves.
type conwayCertsOverlay struct {
	ls common.LedgerState

	// stakeCredentialState overrides ls.IsStakeCredentialRegistered for a
	// credential this transaction's own certificates registered or
	// deregistered. An absent key defers to ls.
	stakeCredentialState map[credOverlayKey]bool

	// drepRegistrations overrides a DRep registration lookup for a full
	// DRep credential this transaction's own certificates registered
	// (true) or deregistered (false). Keyed by credential type and hash
	// together, matching GovState.DRepRegistration(Credential): a
	// key-hash and a script-hash DRep sharing a hash are distinct
	// registrations. An absent key defers to ls.
	drepRegistrations map[credOverlayKey]bool

	// poolRegistrations records pools this transaction's own certificates
	// registered. Pool retirement does not remove a pool from the active
	// set until POOLREAP at the end of the retirement epoch (see the NOTE
	// in UtxoValidateDelegation's PoolRetirementCertificate case), so
	// there is no corresponding deregistration set.
	poolRegistrations map[common.PoolKeyHash]struct{}

	// committeeHot overrides a hot-credential committee-member lookup,
	// keyed by the hot credential's bare hash, for a hot credential this
	// transaction's own certificates authorized or invalidated (by a later
	// re-authorization of the same cold credential, or by resignation). A
	// stored nil means the hot credential is no longer authorized. Keying
	// by bare hash (rather than a type-qualified credential) matches
	// CommitteeMember.HotKey, which is itself a bare hash with no
	// key/script tag; a cold credential's previously authorized hot key
	// learned from ls carries no type to preserve here; a hot key learned
	// directly from an AuthCommitteeHotCertificate in this transaction
	// does carry a type, but is still recorded by bare hash for a single
	// consistent lookup. An absent key defers to ls.
	committeeHot map[common.Blake2b224]*common.CommitteeMember

	// committeeColdHot tracks, for a cold credential this transaction's
	// own certificates touched, the hot credential currently authorized
	// for it within this transaction (nil if resigned), so a later
	// certificate for the same cold credential can find and invalidate
	// the hot key it supersedes. A key absent here means "not yet touched
	// in this transaction"; committeeColdHot[k] == (nil, true) means
	// "resigned in this transaction, no hot key".
	committeeColdHot map[credOverlayKey]*common.Blake2b224
}

// credOverlayKey identifies a credential by both its type and its hash, so
// a key-hash and a script-hash credential sharing the same 28 bytes are
// tracked as distinct entries.
type credOverlayKey struct {
	credType uint
	hash     common.Blake2b224
}

func credKey(cred common.Credential) credOverlayKey {
	return credOverlayKey{credType: cred.CredType, hash: cred.Credential}
}

// newConwayCertsOverlay builds a conwayCertsOverlay by folding tx's own
// certificates, in order, over ls. It performs no I/O beyond what ls itself
// requires and never mutates ls.
func newConwayCertsOverlay(
	tx common.Transaction,
	ls common.LedgerState,
) *conwayCertsOverlay {
	o := &conwayCertsOverlay{
		ls:                   ls,
		stakeCredentialState: make(map[credOverlayKey]bool),
		drepRegistrations:    make(map[credOverlayKey]bool),
		poolRegistrations:    make(map[common.PoolKeyHash]struct{}),
		committeeHot:         make(map[common.Blake2b224]*common.CommitteeMember),
		committeeColdHot:     make(map[credOverlayKey]*common.Blake2b224),
	}
	for _, cert := range tx.Certificates() {
		switch c := cert.(type) {
		case *common.RegistrationCertificate:
			o.stakeCredentialState[credKey(c.StakeCredential)] = true
		case *common.StakeRegistrationCertificate:
			o.stakeCredentialState[credKey(c.StakeCredential)] = true
		case *common.StakeRegistrationDelegationCertificate:
			o.stakeCredentialState[credKey(c.StakeCredential)] = true
		case *common.VoteRegistrationDelegationCertificate:
			o.stakeCredentialState[credKey(c.StakeCredential)] = true
		case *common.StakeVoteRegistrationDelegationCertificate:
			o.stakeCredentialState[credKey(c.StakeCredential)] = true
		case *common.StakeDeregistrationCertificate:
			o.stakeCredentialState[credKey(c.StakeCredential)] = false
		case *common.DeregistrationCertificate:
			o.stakeCredentialState[credKey(c.StakeCredential)] = false
		case *common.RegistrationDrepCertificate:
			o.drepRegistrations[credKey(c.DrepCredential)] = true
		case *common.DeregistrationDrepCertificate:
			o.drepRegistrations[credKey(c.DrepCredential)] = false
		case *common.PoolRegistrationCertificate:
			o.poolRegistrations[c.Operator] = struct{}{}
		case *common.AuthCommitteeHotCertificate:
			o.authorizeCommitteeHot(c.ColdCredential, c.HotCredential)
		case *common.ResignCommitteeColdCertificate:
			o.resignCommitteeCold(c.ColdCredential)
		}
	}
	return o
}

// IsStakeCredentialRegistered reports whether cred is registered after this
// transaction's own certificates, falling back to ls when this transaction
// does not touch cred.
func (o *conwayCertsOverlay) IsStakeCredentialRegistered(
	cred common.Credential,
) bool {
	if registered, tracked := o.stakeCredentialState[credKey(cred)]; tracked {
		return registered
	}
	return o.ls.IsStakeCredentialRegistered(cred)
}

// DRepRegistration resolves a full DRep credential after this transaction's
// own certificates, falling back to ls when this transaction does not touch
// cred. A DRep this transaction registers has no on-chain deposit record
// yet; callers in this package only test registration presence (never
// Deposit), so the synthesized registration carries the credential and
// nothing else.
func (o *conwayCertsOverlay) DRepRegistration(
	cred common.Credential,
) (*common.DRepRegistration, error) {
	if registered, tracked := o.drepRegistrations[credKey(cred)]; tracked {
		if !registered {
			return nil, nil
		}
		return &common.DRepRegistration{Credential: cred}, nil
	}
	return o.ls.DRepRegistration(cred)
}

// IsPoolRegistered reports whether pool is registered after this
// transaction's own certificates, falling back to ls otherwise.
func (o *conwayCertsOverlay) IsPoolRegistered(pool common.PoolKeyHash) bool {
	if _, tracked := o.poolRegistrations[pool]; tracked {
		return true
	}
	return o.ls.IsPoolRegistered(pool)
}

// committeeCredentialState resolves ls's optional CommitteeCredentialState
// capability. Rules invoked through VerifyTransaction receive a
// transaction-scoped caching wrapper around the caller's ledger state
// (common.UnwrapLedgerState), so the type assertion must unwrap first or it
// will never see the capability even when the wrapped state implements it.
func (o *conwayCertsOverlay) committeeCredentialState() (
	common.CommitteeCredentialState,
	bool,
) {
	cs, ok := common.UnwrapLedgerState(o.ls).(common.CommitteeCredentialState)
	return cs, ok
}

// CommitteeStateAvailable reports whether committee state is available,
// deferring entirely to ls: this transaction's certificates cannot make
// committee state available where ls has none.
func (o *conwayCertsOverlay) CommitteeStateAvailable() (bool, error) {
	cs, ok := o.committeeCredentialState()
	if !ok {
		return false, nil
	}
	return cs.CommitteeStateAvailable()
}

// CommitteeCredentialMember resolves a cold committee credential, deferring
// to ls. This transaction's own certificates do not change how a cold
// credential itself resolves (only its hot-key authorization, tracked
// separately by CommitteeHotCredentialMember); this method exists so the
// overlay can look up a cold credential's ls-recorded hot key when a
// resignation or re-authorization needs to invalidate it.
func (o *conwayCertsOverlay) CommitteeCredentialMember(
	cold common.Credential,
) (*common.CommitteeMember, error) {
	cs, ok := o.committeeCredentialState()
	if !ok {
		return nil, nil
	}
	return cs.CommitteeCredentialMember(cold)
}

// CommitteeHotCredentialMember resolves a hot committee credential after
// this transaction's own certificates, falling back to ls when this
// transaction does not touch it.
func (o *conwayCertsOverlay) CommitteeHotCredentialMember(
	hot common.Credential,
) (*common.CommitteeMember, error) {
	if member, tracked := o.committeeHot[hot.Credential]; tracked {
		return member, nil
	}
	cs, ok := o.committeeCredentialState()
	if !ok {
		return nil, nil
	}
	return cs.CommitteeHotCredentialMember(hot)
}

// currentHotForCold resolves the hot-credential hash currently authorized
// for coldKey, checking this transaction's own certificates first and
// falling back to ls. ok is false only when neither source has an opinion.
func (o *conwayCertsOverlay) currentHotForCold(
	coldKey credOverlayKey,
) (hot *common.Blake2b224, ok bool) {
	if hot, tracked := o.committeeColdHot[coldKey]; tracked {
		return hot, true
	}
	cold := common.Credential{CredType: coldKey.credType, Credential: coldKey.hash}
	member, err := o.CommitteeCredentialMember(cold)
	if err != nil || member == nil {
		return nil, false
	}
	return member.HotKey, true
}

// authorizeCommitteeHot applies an AuthCommitteeHotCertificate: it
// invalidates whichever hot credential was previously authorized for cold
// (from an earlier certificate in this same transaction, or from ls), then
// records hot as authorized for cold.
func (o *conwayCertsOverlay) authorizeCommitteeHot(
	cold, hot common.Credential,
) {
	coldKey := credKey(cold)
	if prevHot, ok := o.currentHotForCold(coldKey); ok && prevHot != nil &&
		*prevHot != hot.Credential {
		o.committeeHot[*prevHot] = nil
	}
	member := &common.CommitteeMember{
		ColdKey: cold.Credential,
		HotKey:  &hot.Credential,
	}
	if base, err := o.CommitteeCredentialMember(cold); err == nil && base != nil {
		member.ExpiryEpoch = base.ExpiryEpoch
	}
	newHot := hot.Credential
	o.committeeHot[hot.Credential] = member
	o.committeeColdHot[coldKey] = &newHot
}

// resignCommitteeCold applies a ResignCommitteeColdCertificate: it
// invalidates whichever hot credential was authorized for cold (from an
// earlier certificate in this same transaction, or from ls), so a vote
// cast with that hot credential later in this same transaction is no
// longer recognized.
func (o *conwayCertsOverlay) resignCommitteeCold(cold common.Credential) {
	coldKey := credKey(cold)
	if prevHot, ok := o.currentHotForCold(coldKey); ok && prevHot != nil {
		o.committeeHot[*prevHot] = nil
	}
	o.committeeColdHot[coldKey] = nil
}
