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
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"time"
)

// ErrUtxoSpentInBlock is returned by BlockLedgerState.UtxoById for an output
// that an earlier transaction in the same block consumed.
var ErrUtxoSpentInBlock = errors.New(
	"utxo spent by an earlier transaction in the block",
)

// BlockLedgerState is a block-scoped view of a LedgerState with the effects of
// the block's earlier transactions applied, so each transaction is validated
// against the state its predecessors produced, as the ledger's LEDGERS rule
// folds them. It never writes to the wrapped state.
//
// ApplyTransaction folds one transaction's effects in: a phase-2-valid
// transaction consumes its inputs, adds its outputs, applies its withdrawals
// and certificates, and records its governance proposals; a phase-2-invalid
// one consumes its collateral and adds its collateral return only.
//
// Effects that do not take hold within a block are not modelled: genesis key
// delegations wait for the stability window, pool re-registration parameters
// and retirements wait for an epoch boundary, and governance enactment and
// treasury donations happen at the epoch boundary.
//
// Optional capabilities are answered through UnwrapLedgerState, which reaches
// the wrapped state. The capabilities a transaction can change are resolved
// with the *For helpers (StakeCredentialDepositStateFor and the rest), which
// return a view with the block's effects applied when the wrapped state
// provides that capability.
type BlockLedgerState struct {
	base LedgerState
	// seq numbers applied certificates so a DRep deregistration clears only
	// delegations made before it.
	seq              uint64
	spent            map[utxoCacheKey]struct{}
	created          map[utxoCacheKey]Utxo
	accounts         map[credOverlayKey]*blockAccount
	pools            map[PoolKeyHash]*blockPool
	vrfKeys          map[Blake2b256]PoolKeyHash
	dreps            map[credOverlayKey]*blockDRep
	committeeColdHot map[credOverlayKey]*credOverlayKey
	proposals        map[GovActionId]GovActionState
}

// credOverlayKey identifies a credential by type and hash, so a key-hash and a
// script-hash credential sharing the same bytes stay distinct.
type credOverlayKey struct {
	credType uint
	hash     Blake2b224
}

func blockCredKey(cred Credential) credOverlayKey {
	return credOverlayKey{credType: cred.CredType, hash: cred.Credential}
}

// blockAccount is a stake credential's account as earlier transactions in the
// block left it. Each part is known only once a certificate or withdrawal in
// the block set it; otherwise the wrapped state's answer holds.
type blockAccount struct {
	registered      bool
	registeredKnown bool
	deposit         uint64
	depositKnown    bool
	balance         uint64
	balanceKnown    bool
	drep            *Drep
	drepKnown       bool
	drepSeq         uint64
}

type blockPool struct {
	registration    *PoolRegistrationCertificate
	retirementEpoch *uint64
}

// blockDRep is a DRep's registration as earlier transactions left it.
// registration is nil once deregistered. deregSeq records the latest
// deregistration and survives a re-registration, because the delegations it
// cleared stay cleared.
type blockDRep struct {
	registration *DRepRegistration
	deregSeq     uint64
}

var _ LedgerState = (*BlockLedgerState)(nil)

// NewBlockLedgerState returns a view of base with no transactions applied.
func NewBlockLedgerState(base LedgerState) *BlockLedgerState {
	return &BlockLedgerState{
		base:             base,
		spent:            make(map[utxoCacheKey]struct{}),
		created:          make(map[utxoCacheKey]Utxo),
		accounts:         make(map[credOverlayKey]*blockAccount),
		pools:            make(map[PoolKeyHash]*blockPool),
		vrfKeys:          make(map[Blake2b256]PoolKeyHash),
		dreps:            make(map[credOverlayKey]*blockDRep),
		committeeColdHot: make(map[credOverlayKey]*credOverlayKey),
		proposals:        make(map[GovActionId]GovActionState),
	}
}

// UnwrapLedgerState returns the wrapped state, so optional capabilities that
// no transaction changes are read from it directly.
func (b *BlockLedgerState) UnwrapLedgerState() LedgerState {
	return b.base
}

// ApplyTransaction folds tx's effects into the view. pp supplies the key
// deposit recorded for a pre-Conway stake registration certificate, which
// carries no amount.
func (b *BlockLedgerState) ApplyTransaction(
	tx Transaction,
	pp ProtocolParameters,
) error {
	for _, input := range tx.Consumed() {
		key := utxoCacheKey{id: input.Id(), index: input.Index()}
		delete(b.created, key)
		b.spent[key] = struct{}{}
	}
	for _, utxo := range tx.Produced() {
		key := utxoCacheKey{id: utxo.Id.Id(), index: utxo.Id.Index()}
		delete(b.spent, key)
		b.created[key] = utxo
	}
	if !tx.IsValid() {
		return nil
	}
	// Withdrawals apply before certificates run, matching the Conway LEDGER
	// rule; earlier eras allow no certificate that a same-transaction
	// withdrawal could observe.
	for addr, amount := range tx.Withdrawals() {
		cred, err := addr.RewardAccountCredential()
		if err != nil {
			continue
		}
		balance, err := b.RewardAccountBalance(cred)
		if err != nil {
			return err
		}
		// A withdrawal need not drain the account from Dijkstra on, so the
		// remainder stays available to later transactions.
		remaining := uint64(0)
		if balance != nil && amount != nil && amount.IsUint64() &&
			amount.Uint64() <= *balance {
			remaining = *balance - amount.Uint64()
		}
		account := b.account(cred)
		account.balance = remaining
		account.balanceKnown = true
	}
	for _, cert := range tx.Certificates() {
		if err := b.applyCertificate(cert, pp); err != nil {
			return err
		}
	}
	for idx, proposal := range tx.ProposalProcedures() {
		if idx > int(MaxGovActionIdx) {
			break
		}
		action := proposal.GovAction()
		actionType, ok := blockGovActionType(action)
		if !ok {
			continue
		}
		id := GovActionId{
			TransactionId: tx.Hash(),
			GovActionIdx:  uint32(idx), // #nosec G115 -- bounded above
		}
		b.proposals[id] = GovActionState{
			ActionId:   id,
			ActionType: actionType,
			Action:     action,
		}
	}
	return nil
}

// account returns the block's record for cred, creating an empty one.
func (b *BlockLedgerState) account(cred Credential) *blockAccount {
	key := blockCredKey(cred)
	if account, ok := b.accounts[key]; ok {
		return account
	}
	account := &blockAccount{}
	b.accounts[key] = account
	return account
}

func (b *BlockLedgerState) registerAccount(cred Credential, deposit uint64) {
	b.accounts[blockCredKey(cred)] = &blockAccount{
		registered:      true,
		registeredKnown: true,
		deposit:         deposit,
		depositKnown:    true,
		balanceKnown:    true,
		drepKnown:       true,
	}
}

func (b *BlockLedgerState) deregisterAccount(cred Credential) {
	b.accounts[blockCredKey(cred)] = &blockAccount{
		registeredKnown: true,
		depositKnown:    true,
		balanceKnown:    true,
		drepKnown:       true,
	}
}

func (b *BlockLedgerState) delegateVote(cred Credential, drep Drep) {
	account := b.account(cred)
	account.drep = &drep
	account.drepKnown = true
	account.drepSeq = b.seq
}

// drep returns the block's record for cred, creating one with no
// deregistration. A DRep the block has not touched reports the wrapped
// state's registration, so callers only reach this when setting one.
func (b *BlockLedgerState) drep(cred Credential) *blockDRep {
	key := blockCredKey(cred)
	if drep, ok := b.dreps[key]; ok {
		return drep
	}
	drep := &blockDRep{}
	b.dreps[key] = drep
	return drep
}

func (b *BlockLedgerState) applyCertificate(
	cert Certificate,
	pp ProtocolParameters,
) error {
	b.seq++
	switch c := cert.(type) {
	case *StakeRegistrationCertificate:
		deposit, err := blockKeyDeposit(pp)
		if err != nil {
			return err
		}
		b.registerAccount(c.StakeCredential, deposit)
	case *RegistrationCertificate:
		b.registerAccount(c.StakeCredential, blockAmount(c.Amount))
	case *StakeRegistrationDelegationCertificate:
		b.registerAccount(c.StakeCredential, blockAmount(c.Amount))
	case *VoteRegistrationDelegationCertificate:
		b.registerAccount(c.StakeCredential, blockAmount(c.Amount))
		b.delegateVote(c.StakeCredential, c.Drep)
	case *StakeVoteRegistrationDelegationCertificate:
		b.registerAccount(c.StakeCredential, blockAmount(c.Amount))
		b.delegateVote(c.StakeCredential, c.Drep)
	case *StakeDeregistrationCertificate:
		b.deregisterAccount(c.StakeCredential)
	case *DeregistrationCertificate:
		b.deregisterAccount(c.StakeCredential)
	case *VoteDelegationCertificate:
		b.delegateVote(c.StakeCredential, c.Drep)
	case *StakeVoteDelegationCertificate:
		b.delegateVote(c.StakeCredential, c.Drep)
	case *PoolRegistrationCertificate:
		b.applyPoolRegistration(c)
	case *PoolRetirementCertificate:
		reg, _, err := b.PoolCurrentState(c.PoolKeyHash)
		if err != nil {
			return err
		}
		epoch := c.Epoch
		b.pools[c.PoolKeyHash] = &blockPool{
			registration:    reg,
			retirementEpoch: &epoch,
		}
	case *RegistrationDrepCertificate:
		deposit := blockAmount(c.Amount)
		b.drep(c.DrepCredential).registration = &DRepRegistration{
			Credential: c.DrepCredential,
			Anchor:     c.Anchor,
			Deposit:    &deposit,
		}
	case *UpdateDrepCertificate:
		reg, err := b.DRepRegistration(c.DrepCredential)
		if err != nil {
			return err
		}
		if reg == nil {
			break
		}
		updated := *reg
		updated.Anchor = c.Anchor
		b.drep(c.DrepCredential).registration = &updated
	case *DeregistrationDrepCertificate:
		// The reference GOVCERT rule clears every delegation to a DRep
		// when it deregisters; see DRepDelegation.
		drep := b.drep(c.DrepCredential)
		drep.registration = nil
		drep.deregSeq = b.seq
	case *AuthCommitteeHotCertificate:
		hot := blockCredKey(c.HotCredential)
		b.committeeColdHot[blockCredKey(c.ColdCredential)] = &hot
	case *ResignCommitteeColdCertificate:
		b.committeeColdHot[blockCredKey(c.ColdCredential)] = nil
	}
	return nil
}

// applyPoolRegistration records a registration. A re-registration replaces
// the reported registration and cancels a pending retirement at once, as the
// POOL rule does, though its parameters only take effect at the next epoch.
func (b *BlockLedgerState) applyPoolRegistration(
	cert *PoolRegistrationCertificate,
) {
	b.pools[cert.Operator] = &blockPool{registration: cert}
	b.vrfKeys[cert.VrfKeyHash] = cert.Operator
}

func blockAmount(amount int64) uint64 {
	if amount < 0 {
		return 0
	}
	return uint64(amount)
}

func blockKeyDeposit(pp ProtocolParameters) (uint64, error) {
	keyDeposit, ok := pp.(interface{ KeyDepositAmount() *big.Int })
	if !ok {
		return 0, errors.New(
			"protocol parameters do not expose the key deposit",
		)
	}
	amount := keyDeposit.KeyDepositAmount()
	if amount == nil || !amount.IsUint64() {
		return 0, fmt.Errorf("invalid key deposit %v", amount)
	}
	return amount.Uint64(), nil
}

func blockGovActionType(action GovAction) (GovActionType, bool) {
	if action == nil {
		return 0, false
	}
	if rv := reflect.ValueOf(action); rv.Kind() == reflect.Pointer &&
		rv.IsNil() {
		return 0, false
	}
	switch action.(type) {
	case ParameterChangeGovAction:
		return GovActionTypeParameterChange, true
	case *HardForkInitiationGovAction:
		return GovActionTypeHardForkInitiation, true
	case *TreasuryWithdrawalGovAction:
		return GovActionTypeTreasuryWithdrawal, true
	case *NoConfidenceGovAction:
		return GovActionTypeNoConfidence, true
	case *UpdateCommitteeGovAction:
		return GovActionTypeUpdateCommittee, true
	case *NewConstitutionGovAction:
		return GovActionTypeNewConstitution, true
	case *InfoGovAction:
		return GovActionTypeInfo, true
	default:
		return 0, false
	}
}

// UtxoById resolves an output created earlier in the block, rejects one an
// earlier transaction consumed, and otherwise defers to the wrapped state.
func (b *BlockLedgerState) UtxoById(input TransactionInput) (Utxo, error) {
	key := utxoCacheKey{id: input.Id(), index: input.Index()}
	if utxo, ok := b.created[key]; ok {
		return utxo, nil
	}
	if _, ok := b.spent[key]; ok {
		return Utxo{}, fmt.Errorf("%w: %s", ErrUtxoSpentInBlock, input.String())
	}
	return b.base.UtxoById(input)
}

// IsStakeCredentialRegistered reports registration after earlier
// transactions' certificates.
func (b *BlockLedgerState) IsStakeCredentialRegistered(cred Credential) bool {
	if account, ok := b.accounts[blockCredKey(cred)]; ok &&
		account.registeredKnown {
		return account.registered
	}
	return b.base.IsStakeCredentialRegistered(cred)
}

// IsRewardAccountRegistered reports reward account registration after
// earlier transactions' certificates.
func (b *BlockLedgerState) IsRewardAccountRegistered(cred Credential) bool {
	if account, ok := b.accounts[blockCredKey(cred)]; ok &&
		account.registeredKnown {
		return account.registered
	}
	return b.base.IsRewardAccountRegistered(cred)
}

// RewardAccountBalance reports the balance after earlier transactions'
// withdrawals and certificates. A deregistered account has no balance.
func (b *BlockLedgerState) RewardAccountBalance(
	cred Credential,
) (*uint64, error) {
	account, ok := b.accounts[blockCredKey(cred)]
	if !ok {
		return b.base.RewardAccountBalance(cred)
	}
	if account.registeredKnown && !account.registered {
		return nil, nil
	}
	if !account.balanceKnown {
		return b.base.RewardAccountBalance(cred)
	}
	if !account.registeredKnown {
		// A withdrawal reduced an account registered in the wrapped state.
		base, err := b.base.RewardAccountBalance(cred)
		if err != nil || base == nil {
			return base, err
		}
	}
	balance := account.balance
	return &balance, nil
}

// PoolCurrentState reports a registration or retirement an earlier
// transaction made, and otherwise defers to the wrapped state.
func (b *BlockLedgerState) PoolCurrentState(
	pool PoolKeyHash,
) (*PoolRegistrationCertificate, *uint64, error) {
	if state, ok := b.pools[pool]; ok {
		return state.registration, state.retirementEpoch, nil
	}
	return b.base.PoolCurrentState(pool)
}

// IsPoolRegistered reports a pool an earlier transaction registered as
// registered. Retirement takes effect at an epoch boundary, so it does not
// unregister a pool within the block.
func (b *BlockLedgerState) IsPoolRegistered(pool PoolKeyHash) bool {
	if state, ok := b.pools[pool]; ok && state.registration != nil {
		return true
	}
	return b.base.IsPoolRegistered(pool)
}

// IsVrfKeyInUse reports a VRF key an earlier transaction's pool registration
// claimed, and otherwise defers to the wrapped state.
func (b *BlockLedgerState) IsVrfKeyInUse(
	vrfKeyHash Blake2b256,
) (bool, PoolKeyHash, error) {
	if pool, ok := b.vrfKeys[vrfKeyHash]; ok {
		return true, pool, nil
	}
	return b.base.IsVrfKeyInUse(vrfKeyHash)
}

// DRepRegistration reports a registration, update or deregistration an
// earlier transaction made, and otherwise defers to the wrapped state.
func (b *BlockLedgerState) DRepRegistration(
	cred Credential,
) (*DRepRegistration, error) {
	if drep, ok := b.dreps[blockCredKey(cred)]; ok {
		return drep.registration, nil
	}
	return b.base.DRepRegistration(cred)
}

// DRepRegistrations reports the wrapped state's registrations with earlier
// transactions' registrations, updates and deregistrations applied.
func (b *BlockLedgerState) DRepRegistrations() ([]DRepRegistration, error) {
	base, err := b.base.DRepRegistrations()
	if err != nil {
		return nil, err
	}
	ret := make([]DRepRegistration, 0, len(base)+len(b.dreps))
	for _, reg := range base {
		if _, ok := b.dreps[blockCredKey(reg.Credential)]; ok {
			continue
		}
		ret = append(ret, reg)
	}
	for _, drep := range b.dreps {
		if drep.registration != nil {
			ret = append(ret, *drep.registration)
		}
	}
	return ret, nil
}

// GovActionById resolves a proposal an earlier transaction made, and
// otherwise defers to the wrapped state.
func (b *BlockLedgerState) GovActionById(
	id GovActionId,
) (*GovActionState, error) {
	if state, ok := b.proposals[id]; ok {
		ret := state
		return &ret, nil
	}
	return b.base.GovActionById(id)
}

// GovActionExists reports a proposal an earlier transaction made as existing.
func (b *BlockLedgerState) GovActionExists(id GovActionId) bool {
	if _, ok := b.proposals[id]; ok {
		return true
	}
	return b.base.GovActionExists(id)
}

// CommitteeMember applies earlier transactions' hot-key authorizations and
// resignations to the wrapped state's member.
func (b *BlockLedgerState) CommitteeMember(
	coldKey Blake2b224,
) (*CommitteeMember, error) {
	member, err := b.base.CommitteeMember(coldKey)
	if err != nil || member == nil {
		return member, err
	}
	return b.applyCommitteeAuthorization(member), nil
}

// applyCommitteeAuthorization returns member with the hot key an earlier
// transaction authorized for its cold credential, or resigned, applied.
// CommitteeMember carries no credential tag, so the cold key is matched by
// hash; see conwayCertsOverlay.coldCredentialTouched for why that cannot
// conflate a key-hash and a script-hash cold credential.
func (b *BlockLedgerState) applyCommitteeAuthorization(
	member *CommitteeMember,
) *CommitteeMember {
	for cold, hot := range b.committeeColdHot {
		if cold.hash != member.ColdKey {
			continue
		}
		updated := *member
		if hot == nil {
			updated.HotKey = nil
			updated.Resigned = true
		} else {
			hotHash := hot.hash
			updated.HotKey = &hotHash
			updated.Resigned = false
		}
		return &updated
	}
	return member
}

func (b *BlockLedgerState) coldCredentialTouched(coldHash Blake2b224) bool {
	for cold := range b.committeeColdHot {
		if cold.hash == coldHash {
			return true
		}
	}
	return false
}

// coldCredentialsFor returns the cold credentials an earlier transaction
// left authorizing hot.
func (b *BlockLedgerState) coldCredentialsFor(hot Credential) []Credential {
	hotKey := blockCredKey(hot)
	var ret []Credential
	for cold, curHot := range b.committeeColdHot {
		if curHot != nil && *curHot == hotKey {
			ret = append(ret, Credential{
				CredType:   cold.credType,
				Credential: cold.hash,
			})
		}
	}
	return ret
}

// The remaining LedgerState methods are not changed by a transaction within
// the block.

func (b *BlockLedgerState) StakeRegistration(
	stakeKey []byte,
) ([]StakeRegistrationCertificate, error) {
	return b.base.StakeRegistration(stakeKey)
}

func (b *BlockLedgerState) SlotToTime(slot uint64) (time.Time, error) {
	return b.base.SlotToTime(slot)
}

func (b *BlockLedgerState) TimeToSlot(t time.Time) (uint64, error) {
	return b.base.TimeToSlot(t)
}

func (b *BlockLedgerState) CalculateRewards(
	pots AdaPots,
	snapshot RewardSnapshot,
	params RewardParameters,
) (*RewardCalculationResult, error) {
	return b.base.CalculateRewards(pots, snapshot, params)
}

func (b *BlockLedgerState) GetAdaPots() AdaPots {
	return b.base.GetAdaPots()
}

func (b *BlockLedgerState) UpdateAdaPots(pots AdaPots) error {
	return b.base.UpdateAdaPots(pots)
}

func (b *BlockLedgerState) GetRewardSnapshot(
	epoch uint64,
) (RewardSnapshot, error) {
	return b.base.GetRewardSnapshot(epoch)
}

func (b *BlockLedgerState) CommitteeMembers() ([]CommitteeMember, error) {
	return b.base.CommitteeMembers()
}

func (b *BlockLedgerState) Constitution() (*Constitution, error) {
	return b.base.Constitution()
}

func (b *BlockLedgerState) TreasuryValue() (uint64, error) {
	return b.base.TreasuryValue()
}

func (b *BlockLedgerState) NetworkId() uint {
	return b.base.NetworkId()
}

func (b *BlockLedgerState) CostModels() map[PlutusLanguage]CostModel {
	return b.base.CostModels()
}

// findBlockLedgerState returns the BlockLedgerState beneath the validation
// adapters wrapping ls, or nil.
func findBlockLedgerState(ls LedgerState) *BlockLedgerState {
	for ls != nil {
		switch s := ls.(type) {
		case *BlockLedgerState:
			return s
		case *cachedLedgerState:
			ls = s.LedgerState
		case LedgerStateUnwrapper:
			ls = s.UnwrapLedgerState()
		default:
			return nil
		}
	}
	return nil
}

// StakeCredentialDepositStateFor returns ls's StakeCredentialDepositState
// with the block's earlier transactions applied, or false when the provider
// does not implement it.
func StakeCredentialDepositStateFor(
	ls LedgerState,
) (StakeCredentialDepositState, bool) {
	base, ok := UnwrapLedgerState(ls).(StakeCredentialDepositState)
	if !ok {
		return nil, false
	}
	if b := findBlockLedgerState(ls); b != nil {
		return blockStakeCredentialDeposits{block: b, base: base}, true
	}
	return base, true
}

type blockStakeCredentialDeposits struct {
	block *BlockLedgerState
	base  StakeCredentialDepositState
}

func (s blockStakeCredentialDeposits) StakeCredentialDeposit(
	cred Credential,
) (*uint64, error) {
	account, ok := s.block.accounts[blockCredKey(cred)]
	if !ok || !account.depositKnown {
		return s.base.StakeCredentialDeposit(cred)
	}
	if !account.registered {
		return nil, nil
	}
	deposit := account.deposit
	return &deposit, nil
}

// DRepDelegationStateFor returns ls's DRepDelegationState with the block's
// earlier transactions applied, or false when the provider does not
// implement it.
func DRepDelegationStateFor(ls LedgerState) (DRepDelegationState, bool) {
	base, ok := UnwrapLedgerState(ls).(DRepDelegationState)
	if !ok {
		return nil, false
	}
	if b := findBlockLedgerState(ls); b != nil {
		return blockDRepDelegations{block: b, base: base}, true
	}
	return base, true
}

type blockDRepDelegations struct {
	block *BlockLedgerState
	base  DRepDelegationState
}

// DRepDelegation reports cred's vote delegation after earlier transactions'
// certificates. A delegation to a DRep that deregistered after it was made
// is cleared, as the reference GOVCERT rule clears them.
func (s blockDRepDelegations) DRepDelegation(cred Credential) (*Drep, error) {
	var drep *Drep
	var madeAt uint64
	if account, ok := s.block.accounts[blockCredKey(cred)]; ok &&
		account.drepKnown {
		drep, madeAt = account.drep, account.drepSeq
	} else {
		var err error
		drep, err = s.base.DRepDelegation(cred)
		if err != nil {
			return nil, err
		}
	}
	if drep == nil ||
		(drep.Type != DrepTypeAddrKeyHash && drep.Type != DrepTypeScriptHash) ||
		len(drep.Credential) != Blake2b224Size {
		return drep, nil
	}
	key := credOverlayKey{
		credType: uint(drep.Type), // #nosec G115 -- checked above
		hash:     Blake2b224(drep.Credential),
	}
	if state, ok := s.block.dreps[key]; ok && state.deregSeq > madeAt {
		return nil, nil
	}
	return drep, nil
}

// CommitteeCredentialStateFor returns ls's CommitteeCredentialState with the
// block's earlier transactions' committee certificates applied, or false when
// the provider does not implement it.
func CommitteeCredentialStateFor(
	ls LedgerState,
) (CommitteeCredentialState, bool) {
	base, ok := UnwrapLedgerState(ls).(CommitteeCredentialState)
	if !ok {
		return nil, false
	}
	if b := findBlockLedgerState(ls); b != nil {
		return blockCommitteeCredentials{block: b, base: base}, true
	}
	return base, true
}

type blockCommitteeCredentials struct {
	block *BlockLedgerState
	base  CommitteeCredentialState
}

func (s blockCommitteeCredentials) CommitteeStateAvailable() (bool, error) {
	return s.base.CommitteeStateAvailable()
}

func (s blockCommitteeCredentials) CommitteeCredentialMember(
	cold Credential,
) (*CommitteeMember, error) {
	member, err := s.base.CommitteeCredentialMember(cold)
	if err != nil || member == nil {
		return member, err
	}
	return s.block.applyCommitteeAuthorization(member), nil
}

func (s blockCommitteeCredentials) CommitteeHotCredentialMember(
	hot Credential,
) (*CommitteeMember, error) {
	for _, cold := range s.block.coldCredentialsFor(hot) {
		member, err := s.CommitteeCredentialMember(cold)
		if err != nil {
			return nil, err
		}
		if member != nil {
			return member, nil
		}
	}
	member, err := s.base.CommitteeHotCredentialMember(hot)
	if err != nil || member == nil {
		return member, err
	}
	// The wrapped state resolved hot through a cold credential an earlier
	// transaction may have moved away from hot.
	if s.block.coldCredentialTouched(member.ColdKey) {
		return nil, nil
	}
	return member, nil
}

// CommitteeHotCredentialMembersFor returns ls's CommitteeHotCredentialMembers
// with the block's earlier transactions' committee certificates applied, or
// false when the provider does not implement it.
func CommitteeHotCredentialMembersFor(
	ls LedgerState,
) (CommitteeHotCredentialMembers, bool) {
	base, ok := UnwrapLedgerState(ls).(CommitteeHotCredentialMembers)
	if !ok {
		return nil, false
	}
	b := findBlockLedgerState(ls)
	if b == nil {
		return base, true
	}
	return blockCommitteeHotMembers{block: b, base: base}, true
}

type blockCommitteeHotMembers struct {
	block *BlockLedgerState
	base  CommitteeHotCredentialMembers
}

func (s blockCommitteeHotMembers) CommitteeHotCredentialMembers(
	hot Credential,
) ([]*CommitteeMember, error) {
	var members []*CommitteeMember
	for _, cold := range s.block.coldCredentialsFor(hot) {
		hotHash := hot.Credential
		members = append(members, &CommitteeMember{
			ColdKey: cold.Credential,
			HotKey:  &hotHash,
		})
	}
	baseMembers, err := s.base.CommitteeHotCredentialMembers(hot)
	if err != nil {
		return nil, err
	}
	for _, member := range baseMembers {
		if member == nil || s.block.coldCredentialTouched(member.ColdKey) {
			continue
		}
		members = append(members, member)
	}
	return members, nil
}

// CommitteeVotingStateFor returns ls's CommitteeVotingState with the block's
// earlier transactions' committee certificates applied, or false when the
// provider does not implement it.
func CommitteeVotingStateFor(ls LedgerState) (CommitteeVotingState, bool) {
	base, ok := UnwrapLedgerState(ls).(CommitteeVotingState)
	if !ok {
		return nil, false
	}
	if b := findBlockLedgerState(ls); b != nil {
		return blockCommitteeVoting{block: b, base: base}, true
	}
	return base, true
}

type blockCommitteeVoting struct {
	block *BlockLedgerState
	base  CommitteeVotingState
}

func (s blockCommitteeVoting) CommitteeHotCredentialColdCredentials(
	hot Credential,
) ([]Credential, error) {
	ret := s.block.coldCredentialsFor(hot)
	baseColds, err := s.base.CommitteeHotCredentialColdCredentials(hot)
	if err != nil {
		return nil, err
	}
	for _, cold := range baseColds {
		if _, touched := s.block.committeeColdHot[blockCredKey(cold)]; touched {
			continue
		}
		ret = append(ret, cold)
	}
	return ret, nil
}

func (s blockCommitteeVoting) CommitteeCredentialIsElected(
	cold Credential,
) (bool, error) {
	return s.base.CommitteeCredentialIsElected(cold)
}
