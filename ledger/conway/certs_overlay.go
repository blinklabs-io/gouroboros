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
// (individual certificate-legality issues gouroboros#2384,
// blinklabs-io/dingo#4377, blinklabs-io/dingo#4433).
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

	// committeeColdHot tracks, for a cold credential this transaction's
	// own certificates touched, the hot credential currently authorized
	// for it within this transaction (nil if resigned). A key absent here
	// means "not yet touched in this transaction"; committeeColdHot[k] ==
	// (nil, true) means "resigned in this transaction, no hot key".
	//
	// This is the only state CommitteeHotCredentialMembers derives from: a
	// hot credential is resolved by scanning for a cold credential that
	// currently maps to it, never by a separate hot-keyed cache. Reference:
	// Cardano.Ledger.Conway.Rules.GovCert's csCommitteeCreds is a Map keyed
	// by cold credential (ConwayAuthCommitteeHotKey /
	// ConwayResignCommitteeColdKey each write only their own cold
	// credential's entry), and Cardano.Ledger.Conway.Rules.Gov treats a hot
	// credential as a known voter whenever *any* entry in that map
	// currently authorizes it -- GOVCERT does not prevent two cold
	// credentials from sharing one hot credential. A flat hot-keyed cache
	// that a resignation or re-authorization overwrites destructively would
	// drop a different, untouched cold credential's authorization of the
	// same hot key.
	//
	// The hot credential keeps its key/script tag: csCommitteeCreds stores
	// a typed Credential HotCommitteeRole, and a certificate's hot
	// credential needs no witness, so matching on the hash alone would let
	// a script-hash authorization admit a key-hash voter with the same
	// bytes.
	committeeColdHot map[credOverlayKey]*credOverlayKey
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
		committeeColdHot:     make(map[credOverlayKey]*credOverlayKey),
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

// committeeHotCredentialMembersState resolves ls's optional
// CommitteeHotCredentialMembers capability (see the committeeColdHot field
// comment for why a plural lookup is needed). Absent this capability,
// callers fall back to the singular CommitteeCredentialState.
func (o *conwayCertsOverlay) committeeHotCredentialMembersState() (
	common.CommitteeHotCredentialMembers,
	bool,
) {
	cs, ok := common.UnwrapLedgerState(o.ls).(common.CommitteeHotCredentialMembers)
	return cs, ok
}

// CommitteeHotCredentialMembers resolves every cold credential currently
// authorizing hot after this transaction's own certificates: every touched
// cold credential in committeeColdHot that currently maps to hot, plus
// whatever ls itself reports for any cold credential this transaction did
// not touch. A hot credential may be shared by more than one cold
// credential (see the committeeColdHot field comment), so resigning or
// re-authorizing one cold credential must not take voting rights away from
// a different cold credential that still authorizes the same hot key.
//
// When ls does not implement CommitteeHotCredentialMembers, this falls back
// to the singular CommitteeCredentialState.CommitteeHotCredentialMember,
// which returns at most one witness and is excluded here only when this
// transaction touched the cold credential it names -- the same degraded
// but backward-compatible behavior as before this capability existed.
func (o *conwayCertsOverlay) CommitteeHotCredentialMembers(
	hot common.Credential,
) ([]*common.CommitteeMember, error) {
	var members []*common.CommitteeMember
	hotKey := credKey(hot)
	for coldKey, curHot := range o.committeeColdHot {
		if curHot != nil && *curHot == hotKey {
			hotHash := hot.Credential
			members = append(members, &common.CommitteeMember{
				ColdKey: coldKey.hash,
				HotKey:  &hotHash,
			})
		}
	}
	if plural, ok := o.committeeHotCredentialMembersState(); ok {
		lsMembers, err := plural.CommitteeHotCredentialMembers(hot)
		if err != nil {
			return nil, err
		}
		for _, member := range lsMembers {
			if member == nil || o.coldCredentialTouched(member.ColdKey) {
				continue
			}
			members = append(members, member)
		}
		return members, nil
	}
	cs, ok := o.committeeCredentialState()
	if !ok {
		return members, nil
	}
	member, err := cs.CommitteeHotCredentialMember(hot)
	if err != nil {
		return nil, err
	}
	// ls resolved hot against a cold credential from before this
	// transaction's own certificates ran. If this transaction touched that
	// cold credential, the loop above already returned its current answer,
	// or found none because it moved away from hot -- either way ls's
	// answer is stale here and must not be returned.
	if member != nil && !o.coldCredentialTouched(member.ColdKey) {
		members = append(members, member)
	}
	return members, nil
}

// coldCredentialTouched reports whether this transaction's own certificates
// recorded any state for a cold credential with this bare hash. It matches
// by hash because CommitteeMember.ColdKey carries no key/script tag. That
// cannot conflate two credentials in practice: ls reports only cold
// credentials holding a csCommitteeCreds entry, and every entry, like every
// certificate touching one here, required that cold credential's witness,
// so a key-hash and a script-hash cold credential with the same bytes would
// need a Blake2b-224 collision between a verification key and a script.
func (o *conwayCertsOverlay) coldCredentialTouched(
	coldHash common.Blake2b224,
) bool {
	for coldKey := range o.committeeColdHot {
		if coldKey.hash == coldHash {
			return true
		}
	}
	return false
}

// authorizeCommitteeHot applies an AuthCommitteeHotCertificate: cold now
// authorizes hot, superseding whichever hot credential cold authorized
// before (from an earlier certificate in this same transaction, or from
// ls). A different cold credential that separately authorizes the
// superseded hot credential is unaffected; see the committeeColdHot field
// comment.
func (o *conwayCertsOverlay) authorizeCommitteeHot(
	cold, hot common.Credential,
) {
	newHot := credKey(hot)
	o.committeeColdHot[credKey(cold)] = &newHot
}

// resignCommitteeCold applies a ResignCommitteeColdCertificate: cold no
// longer authorizes any hot credential, so a vote cast under its previous
// hot credential later in this same transaction is no longer recognized as
// coming from cold. A different cold credential that separately authorizes
// the same hot credential is unaffected; see the committeeColdHot field
// comment.
func (o *conwayCertsOverlay) resignCommitteeCold(cold common.Credential) {
	o.committeeColdHot[credKey(cold)] = nil
}
