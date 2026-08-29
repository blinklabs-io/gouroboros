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

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	UtxoValidateMetadata,
	UtxoValidateProposalProcedures,
	UtxoValidateGovActionWellFormedness,
	UtxoValidateHardForkCanFollow,
	UtxoValidateProposalAncestry,
	UtxoValidateProposalDeposit,
	UtxoValidateProposalNetworkIds,
	UtxoValidateProposalReturnAccounts,
	UtxoValidateEmptyTreasuryWithdrawals,
	UtxoValidateBootstrapAllowedGovActions,
	UtxoValidateBootstrapParameterGroups,
	UtxoValidateIsValidFlag,
	UtxoValidateRequiredVKeyWitnesses,
	UtxoValidateCollateralVKeyWitnesses,
	UtxoValidateRedeemerAndScriptWitnesses,
	UtxoValidateSignatures,
	UtxoValidateCostModelsPresent,
	UtxoValidateScriptDataHash,
	UtxoValidateInlineDatumsWithPlutusV1,
	UtxoValidateConwayFeaturesWithPlutusV1V2,
	UtxoValidateDisjointRefInputs,
	UtxoValidateOutsideValidityIntervalUtxo,
	UtxoValidateInputSetEmptyUtxo,
	UtxoValidateNoDuplicateInputs,
	UtxoValidateFeeTooSmallUtxo,
	UtxoValidateInsufficientCollateral,
	UtxoValidateCollateralContainsNonAda,
	UtxoValidateCollateralEqBalance,
	UtxoValidateNoCollateralInputs,
	UtxoValidateBadInputsUtxo,
	// Ensure script witness presence/absence is validated after redeemer/script relation
	UtxoValidateScriptWitnesses,
	UtxoValidateValueNotConservedUtxo,
	UtxoValidateOutputTooSmallUtxo,
	UtxoValidateOutputTooBigUtxo,
	UtxoValidateOutputBootAddrAttrsTooBig,
	UtxoValidateWrongNetwork,
	UtxoValidateWrongNetworkWithdrawal,
	UtxoValidateTransactionNetworkId,
	UtxoValidateMaxTxSizeUtxo,
	UtxoValidateExUnitsTooBigUtxo,
	UtxoValidateTooManyCollateralInputs,
	UtxoValidateSupplementalDatums,
	UtxoValidateExtraneousRedeemers,
	UtxoValidatePlutusScripts,
	UtxoValidateNativeScripts,
	UtxoValidateDelegation,
	UtxoValidateWithdrawals,
	UtxoValidateCommitteeCertificates,
	UtxoValidateUnknownVoters,
	UtxoValidateUnknownGovActionIds,
	UtxoValidateVotingOnExpiredGovAction,
	UtxoValidateBootstrapVotingRestrictions,
	UtxoValidateStakePoolVotingRestrictions,
	UtxoValidateCCVotingRestrictions,
	UtxoValidateMalformedReferenceScripts,
}

// isInConwayBootstrapPhase reports whether the given protocol parameters
// are in the Conway bootstrap phase: protocol major version in the range
// [PV9, PV10). The Plomin hard fork at PV10 lifts the bootstrap restrictions
// on governance actions.
//
// The predicate is bounded on both sides: a sub-PV9 major (e.g., a Babbage
// pp accidentally wrapped in *ConwayProtocolParameters) returns false, and
// PV10+ returns false. Returns false for non-Conway parameter types as a
// defensive fallback; callers in this package always pass
// *ConwayProtocolParameters.
func isInConwayBootstrapPhase(pp common.ProtocolParameters) bool {
	conwayPp, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return false
	}
	major := conwayPp.ProtocolVersion.Major
	return major >= common.ProtocolVersionConway &&
		major < common.ProtocolVersionPlomin
}

// UtxoValidateDisjointRefInputs ensures reference inputs don't overlap with regular inputs.
// For PV11+, this check is skipped when PlutusV1/V2 scripts are present, as the
// NonDisjointRefInputs restriction is reverted for backwards compatibility.
func UtxoValidateDisjointRefInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	conwayPp, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return babbage.UtxoValidateDisjointRefInputs(tx, slot, ls, pp)
	}
	// PV11+ skips this check for transactions with PlutusV1/V2 scripts
	if common.IsProtocolVersionAtLeast(
		conwayPp.ProtocolVersion.Major, 0, common.ProtocolVersionVanRossem,
	) {
		usesV1V2, err := transactionUsesPlutusV1V2(tx, ls)
		if err != nil {
			return err
		}
		if usesV1V2 {
			return nil
		}
	}
	return babbage.UtxoValidateDisjointRefInputs(tx, slot, ls, pp)
}

// transactionUsesPlutusV1V2 checks if the transaction uses PlutusV1 or PlutusV2 scripts,
// either in the witness set or as reference scripts.
// Returns an error if a reference input cannot be resolved.
func transactionUsesPlutusV1V2(
	tx common.Transaction,
	ls common.LedgerState,
) (bool, error) {
	ws := tx.Witnesses()
	if ws != nil {
		if len(ws.PlutusV1Scripts()) > 0 || len(ws.PlutusV2Scripts()) > 0 {
			return true, nil
		}
	}
	// Also check reference scripts on reference inputs
	// For reference inputs, propagate resolution errors
	for _, refInput := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return false, common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script, common.PlutusV2Script:
			return true, nil
		}
	}
	// Check reference scripts on regular inputs
	// For regular inputs, skip on errors (existing behavior)
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			continue
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script, common.PlutusV2Script:
			return true, nil
		}
	}
	return false, nil
}

// UtxoValidateProposalProcedures validates governance proposal contents
func UtxoValidateProposalProcedures(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	for _, proposal := range tx.ProposalProcedures() {
		govAction := proposal.GovAction()
		if isNilGovAction(govAction) {
			continue
		}

		// Check if this is a ParameterChangeGovAction
		paramChangeAction, ok := govAction.(*ConwayParameterChangeGovAction)
		if !ok {
			continue
		}

		// Validate the protocol parameter update
		if err := validateProtocolParameterUpdate(&paramChangeAction.ParamUpdate); err != nil {
			return err
		}
	}
	return nil
}

// UtxoValidateEmptyTreasuryWithdrawals validates that TreasuryWithdrawalGovAction proposals
// do not have empty withdrawal maps and have at least one non-zero withdrawal amount.
// This is distinct from transaction reward withdrawals.
func UtxoValidateEmptyTreasuryWithdrawals(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	for _, proposal := range tx.ProposalProcedures() {
		govAction := proposal.GovAction()
		if isNilGovAction(govAction) {
			continue
		}

		// Check if this is a TreasuryWithdrawalGovAction with empty withdrawals
		if twAction, ok := govAction.(*common.TreasuryWithdrawalGovAction); ok &&
			twAction != nil {
			if len(twAction.Withdrawals) == 0 {
				return EmptyTreasuryWithdrawalsError{}
			}
			// Check that at least one withdrawal has a non-zero amount
			hasNonZero := false
			for _, amount := range twAction.Withdrawals {
				if amount > 0 {
					hasNonZero = true
					break
				}
			}
			if !hasNonZero {
				return ZeroTreasuryWithdrawalAmountError{}
			}
		}
	}
	return nil
}

// UtxoValidateBootstrapAllowedGovActions enforces the Conway bootstrap-phase
// restriction on which governance action types may be proposed.
//
// Pre-Plomin (PV9), only InfoAction, HardForkInitiation, and ParameterChange
// are permitted (ParameterChange's restricted parameter groups are enforced
// separately by UtxoValidateBootstrapParameterGroups). TreasuryWithdrawal,
// NoConfidence, UpdateCommittee, and NewConstitution are rejected.
//
// At PV10 (Plomin) and later, all governance action types are allowed.
func UtxoValidateBootstrapAllowedGovActions(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if !isInConwayBootstrapPhase(pp) {
		return nil
	}
	for _, proposal := range tx.ProposalProcedures() {
		govAction := proposal.GovAction()
		if isNilGovAction(govAction) {
			continue
		}
		// NOTE: closed-set type-switch over the 7 GovActionType constants in
		// ledger/common/gov.go. Unknown GovAction implementations are rejected
		// by the default arm; add an explicit case when a new action is supported.
		switch govAction.(type) {
		case *common.InfoGovAction:
			// always allowed
		case *common.HardForkInitiationGovAction:
			// allowed because it is the path out of bootstrap
		case *ConwayParameterChangeGovAction:
			// allowed shape; group restriction enforced separately
		case *common.TreasuryWithdrawalGovAction:
			return BootstrapDisallowedGovActionError{
				ActionType: common.GovActionTypeTreasuryWithdrawal,
			}
		case *common.NoConfidenceGovAction:
			return BootstrapDisallowedGovActionError{
				ActionType: common.GovActionTypeNoConfidence,
			}
		case *common.UpdateCommitteeGovAction:
			return BootstrapDisallowedGovActionError{
				ActionType: common.GovActionTypeUpdateCommittee,
			}
		case *common.NewConstitutionGovAction:
			return BootstrapDisallowedGovActionError{
				ActionType: common.GovActionTypeNewConstitution,
			}
		default:
			return fmt.Errorf("unknown governance action type %T", govAction)
		}
	}
	return nil
}

// UtxoValidateBootstrapParameterGroups enforces the Conway bootstrap-phase
// restriction that ParameterChange proposals may not touch fields restricted
// during bootstrap. The Plomin hard fork (PV10) lifts this restriction.
//
// See UtxoValidateBootstrapAllowedGovActions for the action-type-level
// restriction enforced first.
func UtxoValidateBootstrapParameterGroups(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if !isInConwayBootstrapPhase(pp) {
		return nil
	}
	for _, proposal := range tx.ProposalProcedures() {
		govAction := proposal.GovAction()
		if isNilGovAction(govAction) {
			continue
		}
		paramChange, ok := govAction.(*ConwayParameterChangeGovAction)
		if !ok {
			continue
		}
		if fields := paramChange.ParamUpdate.BootstrapRestrictedFields(); len(
			fields,
		) > 0 {
			return BootstrapDisallowedParameterChangeError{Fields: fields}
		}
	}
	return nil
}

// UtxoValidateProposalNetworkIds validates that all addresses in proposal procedures use correct network ID
func UtxoValidateProposalNetworkIds(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	networkId := ls.NetworkId()
	badAddrs := []common.Address{}

	for _, proposal := range tx.ProposalProcedures() {
		// Check the return address (where deposit goes back)
		returnAddr := proposal.RewardAccount()
		if returnAddr.NetworkId() != networkId {
			badAddrs = append(badAddrs, returnAddr)
		}

		// Check addresses within governance actions
		govAction := proposal.GovAction()
		if isNilGovAction(govAction) {
			continue
		}

		// TreasuryWithdrawalGovAction contains withdrawal addresses
		if twAction, ok := govAction.(*common.TreasuryWithdrawalGovAction); ok &&
			twAction != nil {
			for addr := range twAction.Withdrawals {
				if addr.NetworkId() != networkId {
					badAddrs = append(badAddrs, *addr)
				}
			}
		}
	}

	if len(badAddrs) == 0 {
		return nil
	}
	return WrongNetworkProposalAddressError{
		NetId: networkId,
		Addrs: badAddrs,
	}
}

// govActionPurpose identifies the "purpose" chain a governance action
// belongs to for ancestry (PrevGovActionId) validation, mirroring the
// GovPurposeId groupings in the cardano-ledger spec: HardFork, Committee
// (shared by NoConfidence and UpdateCommittee), Constitution, and
// PParamUpdate. TreasuryWithdrawal and Info actions have no ancestor field
// and therefore no purpose.
type govActionPurpose int

const (
	govPurposePParamUpdate govActionPurpose = iota
	govPurposeHardFork
	govPurposeCommittee
	govPurposeConstitution
)

// govActionAncestor returns the optional ancestor GovActionId referenced by
// a governance action along with its purpose group. ok is false for action
// types that carry no ancestor field (TreasuryWithdrawal, Info).
func govActionAncestor(
	ga common.GovAction,
) (ancestor *common.GovActionId, purpose govActionPurpose, ok bool) {
	if isNilGovAction(ga) {
		return nil, 0, false
	}
	switch a := ga.(type) {
	case common.ParameterChangeGovAction:
		return a.PreviousGovActionId(), govPurposePParamUpdate, true
	case *common.HardForkInitiationGovAction:
		return a.ActionId, govPurposeHardFork, true
	case *common.NoConfidenceGovAction:
		return a.ActionId, govPurposeCommittee, true
	case *common.UpdateCommitteeGovAction:
		return a.ActionId, govPurposeCommittee, true
	case *common.NewConstitutionGovAction:
		return a.ActionId, govPurposeConstitution, true
	default:
		return nil, 0, false
	}
}

// govActionTypePurpose maps a GovActionState's ActionType to its purpose
// group, for comparison against a proposal's referenced ancestor.
func govActionTypePurpose(
	actionType common.GovActionType,
) (govActionPurpose, bool) {
	switch actionType {
	case common.GovActionTypeParameterChange:
		return govPurposePParamUpdate, true
	case common.GovActionTypeHardForkInitiation:
		return govPurposeHardFork, true
	case common.GovActionTypeNoConfidence, common.GovActionTypeUpdateCommittee:
		return govPurposeCommittee, true
	case common.GovActionTypeNewConstitution:
		return govPurposeConstitution, true
	case common.GovActionTypeTreasuryWithdrawal, common.GovActionTypeInfo:
		// TreasuryWithdrawal and Info actions carry no ancestor and
		// therefore have no purpose chain.
		return 0, false
	default:
		return 0, false
	}
}

// protocolVersionCanFollow reports whether newPV may legally succeed curPV,
// per the cardano-ledger pvCanFollow predicate: either the major version
// increments by exactly one and the minor version resets to zero, or the
// major version is unchanged and the minor version increments by exactly
// one.
func protocolVersionCanFollow(
	curMajor, curMinor, newMajor, newMinor uint,
) bool {
	if newMajor == curMajor+1 && newMinor == 0 {
		return true
	}
	return newMajor == curMajor && newMinor == curMinor+1
}

// txGovProposal is a governance action proposed by the transaction under
// validation, together with its position in the transaction's proposal
// procedures.
type txGovProposal struct {
	idx    int
	action common.GovAction
}

// governanceProposalIndexFits reports whether an int can be represented by
// the uint32 index carried in a governance action ID. Converting first to
// uint64 keeps the comparison well-typed on 32-bit targets, where the
// untyped math.MaxUint32 constant cannot be represented by int.
func governanceProposalIndexFits(idx int) bool {
	return idx >= 0 && int64(idx) <= int64(math.MaxUint32)
}

// txProposalActions indexes the governance actions proposed by tx itself by
// the governance action id each one receives once the transaction is
// accepted, (transaction id, proposal index).
//
// cardano-ledger's conwayGovTransition folds processProposal over the
// proposal procedures in order and threads the accumulated Proposals through
// every subsequent check, so a proposal may name an earlier proposal of the
// same transaction as its predecessor, and a vote may refer to an action
// proposed by its own transaction. Rules that only consult the ledger state
// would reject those as unknown.
func txProposalActions(
	tx common.Transaction,
) map[common.GovActionId]txGovProposal {
	proposals := tx.ProposalProcedures()
	if len(proposals) == 0 {
		return nil
	}
	txId := tx.Hash()
	ret := make(map[common.GovActionId]txGovProposal, len(proposals))
	for idx, proposal := range proposals {
		if !governanceProposalIndexFits(idx) {
			break
		}
		actionId := common.GovActionId{
			TransactionId: txId,
			GovActionIdx:  uint32(idx), // #nosec G115 -- bounded above
		}
		ret[actionId] = txGovProposal{idx: idx, action: proposal.GovAction()}
	}
	return ret
}

// govActionResolver answers questions about the governance action a voting
// procedure names, consulting the proposals of the transaction under
// validation before the ledger state.
//
// cardano-ledger's conwayGovTransition folds the transaction's proposals into
// the proposal set before checking its votes
// (eras/conway/impl/src/Cardano/Ledger/Conway/Rules/Gov.hs at commit
// 08773e9a8f911f67209560a4e401369cbb21a0cb), so a vote may name an action its
// own transaction proposes. A rule that only consults the ledger state either
// rejects such a vote as unknown or leaves its restriction unenforced.
//
// The proposal index is built lazily, so a transaction with no voting
// procedures or no proposals pays nothing.
type govActionResolver struct {
	tx        common.Transaction
	ls        common.LedgerState
	proposals map[common.GovActionId]txGovProposal
	loaded    bool
}

// txProposal returns the proposal of the transaction under validation that
// receives actionId once the transaction is accepted.
func (r *govActionResolver) txProposal(
	actionId common.GovActionId,
) (txGovProposal, bool) {
	if !r.loaded {
		r.proposals = txProposalActions(r.tx)
		r.loaded = true
	}
	proposal, ok := r.proposals[actionId]
	return proposal, ok
}

// exists reports whether actionId names a governance action the ledger state
// records or the transaction under validation proposes. A proposal whose
// action contents are absent still counts as existing: a proposal procedure
// always carries an action on the wire, and cardano-ledger adds the proposal
// to the folded proposal set regardless of what it proposes.
func (r *govActionResolver) exists(actionId common.GovActionId) bool {
	if r.ls != nil && r.ls.GovActionExists(actionId) {
		return true
	}
	_, ok := r.txProposal(actionId)
	return ok
}

// resolve returns the type and, where available, the contents of the
// governance action named by actionId. ok is false when neither the
// transaction nor the ledger state can classify the action, which leaves a
// type-dependent restriction unenforced rather than guessed at.
func (r *govActionResolver) resolve(
	actionId common.GovActionId,
) (actionType common.GovActionType, action common.GovAction, ok bool) {
	if proposal, found := r.txProposal(actionId); found {
		if isNilGovAction(proposal.action) {
			return 0, nil, false
		}
		resolvedType, ok := govActionValidationType(proposal.action)
		if !ok {
			return 0, nil, false
		}
		return resolvedType, proposal.action, true
	}
	if r.ls == nil {
		return 0, nil, false
	}
	actionState, err := r.ls.GovActionById(actionId)
	if err != nil || actionState == nil {
		return 0, nil, false
	}
	action = actionState.Action
	if isNilGovAction(action) {
		action = nil
	}
	return actionState.ActionType, action, true
}

// govActionValidationType classifies an action for shared governance rules.
// Unlike the Conway wire constructor, validation accepts the era-independent
// parameter-change action contract implemented by later eras.
func govActionValidationType(
	action common.GovAction,
) (common.GovActionType, bool) {
	if isNilGovAction(action) {
		return 0, false
	}
	if _, ok := action.(common.ParameterChangeGovAction); ok {
		return common.GovActionTypeParameterChange, true
	}
	actionType, err := conwayGovActionType(action)
	if err != nil {
		return 0, false
	}
	return common.GovActionType(actionType), true
}

// hardForkProposedVersion returns the protocol version proposed by a
// HardForkInitiation action. ok is false for any other action type, matching
// the pattern match on HardForkInitiation in cardano-ledger's
// preceedingHardFork.
func hardForkProposedVersion(
	action common.GovAction,
) (major, minor uint, ok bool) {
	hf, isHardFork := action.(*common.HardForkInitiationGovAction)
	if !isHardFork || hf == nil {
		return 0, 0, false
	}
	return hf.ProtocolVersion.Major, hf.ProtocolVersion.Minor, true
}

// govPurposeRoots returns the current root of each governance-action purpose
// chain when the ledger state implements the optional
// common.GovPurposeRootsState capability, and nil when it does not.
func govPurposeRoots(
	ls common.LedgerState,
) (*common.GovPurposeRoots, error) {
	rootsState, ok := ls.(common.GovPurposeRootsState)
	if !ok {
		return nil, nil
	}
	return rootsState.GovPurposeRoots()
}

// govPurposeRootId returns the root governance action id recorded for a
// purpose, or nil when nothing of that purpose has been enacted yet.
func govPurposeRootId(
	roots *common.GovPurposeRoots,
	purpose govActionPurpose,
) *common.GovActionId {
	if roots == nil {
		return nil
	}
	switch purpose {
	case govPurposePParamUpdate:
		return roots.PParamUpdate
	case govPurposeHardFork:
		return roots.HardFork
	case govPurposeCommittee:
		return roots.Committee
	case govPurposeConstitution:
		return roots.Constitution
	default:
		return nil
	}
}

// UtxoValidateGovActionWellFormedness performs structural well-formedness
// checks on governance actions beyond the ParameterChange-specific checks in
// UtxoValidateProposalProcedures (ConwayGovPredFailure.MalformedProposal),
// plus the ConflictingCommitteeUpdate check for UpdateCommittee actions.
func UtxoValidateGovActionWellFormedness(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	for _, proposal := range tx.ProposalProcedures() {
		govAction := proposal.GovAction()
		if isNilGovAction(govAction) {
			return MalformedGovActionError{
				Reason: "governance action cannot be nil",
			}
		}

		// A governance policy hash, when present, must be a 28-byte script
		// hash.
		if withPolicy, ok := govAction.(common.GovActionWithPolicy); ok {
			policyHash := withPolicy.GetPolicyHash()
			if len(policyHash) != 0 &&
				len(policyHash) != common.Blake2b224Size {
				return MalformedGovActionError{
					Reason: fmt.Sprintf(
						"policy hash has invalid length %d, expected %d",
						len(policyHash),
						common.Blake2b224Size,
					),
				}
			}
		}

		switch a := govAction.(type) {
		case *common.NewConstitutionGovAction:
			if l := len(a.Constitution.ScriptHash); l != 0 &&
				l != common.Blake2b224Size {
				return MalformedGovActionError{
					Reason: fmt.Sprintf(
						"constitution script hash has invalid length %d, expected %d",
						l,
						common.Blake2b224Size,
					),
				}
			}

		case *common.UpdateCommitteeGovAction:
			// common.Credential embeds cbor.DecodeStoreCbor (a slice field),
			// making it non-comparable, so key the set on its logical
			// (CredType, Credential hash) value instead.
			type credKey struct {
				credType uint
				hash     common.Blake2b224
			}
			removed := make(map[credKey]bool, len(a.Credentials))
			for _, cred := range a.Credentials {
				removed[credKey{credType: cred.CredType, hash: cred.Credential}] = true
			}
			var conflicting []common.Credential
			for cred := range a.CredEpochs {
				if cred == nil {
					continue
				}
				if removed[credKey{credType: cred.CredType, hash: cred.Credential}] {
					conflicting = append(conflicting, *cred)
				}
			}
			if len(conflicting) > 0 {
				// Map iteration order is non-deterministic; sort before
				// constructing the error so the message is reproducible
				// across runs.
				slices.SortFunc(
					conflicting,
					func(a, b common.Credential) int {
						if a.CredType != b.CredType {
							if a.CredType < b.CredType {
								return -1
							}
							return 1
						}
						return bytes.Compare(
							a.Credential.Bytes(),
							b.Credential.Bytes(),
						)
					},
				)
				return ConflictingCommitteeUpdateError{Credentials: conflicting}
			}
		}
	}
	return nil
}

// UtxoValidateHardForkCanFollow checks that a HardForkInitiation governance
// action's proposed protocol version can legally follow the protocol version
// it succeeds (ConwayGovPredFailure.ProposalCantFollow).
//
// This mirrors preceedingHardFork plus the pvCanFollow guard in
// cardano-ledger's conwayGovTransition
// (eras/conway/impl/src/Cardano/Ledger/Conway/Rules/Gov.hs lines 488-499 and
// 673-695 at commit 08773e9a8f911f67209560a4e401369cbb21a0cb):
//
//   - a proposed major version more than one above the currently enacted
//     major version is compared against the enacted version, so a chain of
//     pending proposals cannot be used to jump ahead;
//   - otherwise a proposal with a predecessor is compared against that
//     predecessor's proposed protocol version, taken from the transaction
//     itself for a predecessor proposed by the same transaction, or from the
//     ledger state's record of the referenced action;
//   - a proposal with no predecessor is compared against the enacted
//     version. The enacted protocol version is always the version proposed
//     by the hard-fork action that is the current root of the hard-fork
//     purpose chain, since a ParameterChange action cannot alter the
//     protocol version, so comparing against the recorded proposed version
//     of a predecessor that is the root gives the same verdict as the
//     reference implementation's comparison against the enacted version.
//
// A predecessor whose contents the ledger state does not record (see
// common.GovActionState.Action) leaves the numeric check deferred rather
// than run against the wrong reference version; the predecessor's existence
// and purpose are validated by UtxoValidateProposalAncestry.
func UtxoValidateHardForkCanFollow(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	conwayPp, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	curPV := conwayPp.ProtocolVersion
	var txProposals map[common.GovActionId]txGovProposal
	txProposalsLoaded := false
	for idx, proposal := range tx.ProposalProcedures() {
		hf, ok := proposal.GovAction().(*common.HardForkInitiationGovAction)
		if !ok || hf == nil {
			continue
		}
		newPV := hf.ProtocolVersion
		// Equivalent to `Just (pvMajor newProtVer) > succVersion (pvMajor
		// current)` without risking an overflow on curPV.Major+1.
		majorTooHigh := newPV.Major > curPV.Major &&
			newPV.Major-curPV.Major > 1
		refMajor, refMinor := curPV.Major, curPV.Minor
		if hf.ActionId != nil && !majorTooHigh {
			if !txProposalsLoaded {
				txProposals = txProposalActions(tx)
				txProposalsLoaded = true
			}
			var ancestorAction common.GovAction
			if txProposal, ok := txProposals[*hf.ActionId]; ok {
				if txProposal.idx >= idx {
					// Only an earlier proposal of the same transaction has
					// been folded into the proposal set at this point; a
					// forward or self reference is reported by
					// UtxoValidateProposalAncestry.
					continue
				}
				ancestorAction = txProposal.action
			} else if ls != nil {
				ancestorState, err := ls.GovActionById(*hf.ActionId)
				if err != nil || ancestorState == nil {
					// A missing predecessor is reported by
					// UtxoValidateProposalAncestry.
					continue
				}
				ancestorAction = ancestorState.Action
			}
			if isNilGovAction(ancestorAction) {
				continue
			}
			major, minor, isHardFork := hardForkProposedVersion(ancestorAction)
			if !isHardFork {
				// A predecessor of another purpose is reported by
				// UtxoValidateProposalAncestry.
				continue
			}
			refMajor, refMinor = major, minor
		}
		if !protocolVersionCanFollow(
			refMajor,
			refMinor,
			newPV.Major,
			newPV.Minor,
		) {
			return BadHardForkProtocolVersionError{
				Supplied: common.ProtocolParametersProtocolVersion{
					Major: newPV.Major,
					Minor: newPV.Minor,
				},
				Expected: common.ProtocolParametersProtocolVersion{
					Major: refMajor,
					Minor: refMinor,
				},
			}
		}
	}
	return nil
}

// UtxoValidateProposalAncestry checks that a governance action's optional
// PrevGovActionId names a valid predecessor in its purpose chain
// (ConwayGovPredFailure.InvalidPrevGovActionId).
//
// cardano-ledger accepts a proposal only when runProposalsAddAction can
// attach it to the purpose's tree
// (eras/conway/impl/src/Cardano/Ledger/Conway/Governance/Proposals.hs lines
// 305-334 at commit 08773e9a8f911f67209560a4e401369cbb21a0cb, reached from
// the proposalsAddAction call in conwayGovTransition,
// eras/conway/impl/src/Cardano/Ledger/Conway/Rules/Gov.hs lines 550-556):
// the proposal's predecessor must either equal the current root of that
// purpose chain, including the case where both are absent, or be a proposal
// of that purpose that is still pending.
//
// The current root is only available when the ledger state implements the
// optional common.GovPurposeRootsState capability. Without it this rule
// stays limited to ancestor existence and purpose matching, so a ledger
// state that cannot report roots is not made to reject proposals it has no
// way to judge.
func UtxoValidateProposalAncestry(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	proposals := tx.ProposalProcedures()
	if len(proposals) == 0 {
		return nil
	}
	roots, err := govPurposeRoots(ls)
	if err != nil {
		return fmt.Errorf("governance purpose roots lookup failed: %w", err)
	}
	txProposals := txProposalActions(tx)
	for idx, proposal := range proposals {
		govAction := proposal.GovAction()
		if isNilGovAction(govAction) {
			continue
		}
		ancestorId, purpose, hasPurpose := govActionAncestor(govAction)
		if !hasPurpose {
			continue
		}
		rootId := govPurposeRootId(roots, purpose)
		if ancestorId == nil {
			// A proposal without a predecessor is only valid while the
			// purpose chain has no root. Skip the check entirely when the
			// roots are unknown.
			if roots == nil || rootId == nil {
				continue
			}
			return InvalidGovActionAncestorError{
				ActionId: *rootId,
				Reason: "proposal has no predecessor but the purpose chain " +
					"root is set",
			}
		}
		// The predecessor is the current root of its purpose chain.
		if rootId != nil && rootId.Equal(*ancestorId) {
			continue
		}
		// The predecessor is an earlier proposal of the same purpose in this
		// same transaction.
		if txProposal, ok := txProposals[*ancestorId]; ok {
			if txProposal.idx < idx && txProposal.action != nil {
				_, txPurpose, txHasPurpose := govActionAncestor(
					txProposal.action,
				)
				if txHasPurpose && txPurpose == purpose {
					continue
				}
			}
			return InvalidGovActionAncestorError{
				ActionId: *ancestorId,
				Reason: "referenced ancestor governance action is not an " +
					"earlier proposal of the same purpose in this transaction",
			}
		}
		// Otherwise the predecessor must be a pending proposal of the same
		// purpose recorded in the ledger state.
		if ls == nil {
			return InvalidGovActionAncestorError{
				ActionId: *ancestorId,
				Reason:   "no ledger state available to resolve the ancestor",
			}
		}
		ancestorState, err := ls.GovActionById(*ancestorId)
		if err != nil {
			return InvalidGovActionAncestorError{
				ActionId: *ancestorId,
				Reason:   fmt.Sprintf("lookup failed: %v", err),
			}
		}
		if ancestorState == nil {
			return InvalidGovActionAncestorError{
				ActionId: *ancestorId,
				Reason:   "referenced ancestor governance action does not exist",
			}
		}
		ancestorPurpose, ok := govActionTypePurpose(ancestorState.ActionType)
		if !ok || ancestorPurpose != purpose {
			return InvalidGovActionAncestorError{
				ActionId: *ancestorId,
				Reason:   "referenced ancestor governance action has a mismatched purpose",
			}
		}
		// An expired proposal is no longer in the purpose tree, so it cannot
		// be a predecessor. ExpirySlot is optional in the LedgerState
		// contract (see UtxoValidateVotingOnExpiredGovAction): a state
		// provider that does not model expiry leaves it zero, which is
		// treated as "expiry not modeled" rather than "expired at slot 0".
		if ancestorState.ExpirySlot != 0 && slot > ancestorState.ExpirySlot {
			return InvalidGovActionAncestorError{
				ActionId: *ancestorId,
				Reason: fmt.Sprintf(
					"referenced ancestor governance action expired at slot %d",
					ancestorState.ExpirySlot,
				),
			}
		}
	}
	return nil
}

// UtxoValidateProposalDeposit checks that every proposal procedure's deposit
// exactly matches the protocol's GovActionDeposit parameter
// (ConwayGovPredFailure.ProposalDepositIncorrect).
func UtxoValidateProposalDeposit(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	conwayPp, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	for _, proposal := range tx.ProposalProcedures() {
		if proposal.Deposit() != conwayPp.GovActionDeposit {
			return ProposalDepositIncorrectError{
				Supplied: proposal.Deposit(),
				Expected: conwayPp.GovActionDeposit,
			}
		}
	}
	return nil
}

// UtxoValidateProposalReturnAccounts checks that a proposal's return
// (refund) address, and any treasury withdrawal destination addresses, are
// registered reward accounts (ConwayGovPredFailure.ProposalReturnAccountDoesNotExist
// and TreasuryWithdrawalReturnAccountsDoNotExist).
//
// NOTE: this check is genuinely skipped during the Conway bootstrap phase
// (PV9) per the reference implementation: `conwayGovTransition` in
// cardano-ledger's `Cardano.Ledger.Conway.Rules.Gov`
// (eras/conway/impl/src/Cardano/Ledger/Conway/Rules/Gov.hs) wraps this exact
// pair of checks in
// `unless (hardforkConwayBootstrapPhase $ pp ^. ppProtocolVersionL) $ do ...`
// immediately before the ProposalDepositIncorrect check. Bootstrap-phase
// proposals are otherwise restricted to a narrow allow-list of action types
// (see UtxoValidateBootstrapAllowedGovActions /
// DisallowedProposalDuringBootstrap upstream), so this is a deliberate spec
// relaxation, not an oversight.
func UtxoValidateProposalReturnAccounts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if isInConwayBootstrapPhase(pp) {
		return nil
	}
	isRegistered := func(addr common.Address) bool {
		// The CDDL reward_account type only permits the two
		// none-payment-credential address types (AddressTypeNoneKey /
		// AddressTypeNoneScript). addr.StakeCredential() also succeeds for
		// base, pointer, and enterprise addresses that happen to carry a
		// staking payload, so without this check a base address would be
		// wrongly accepted as a valid reward account here.
		addrType := addr.Type()
		if addrType != common.AddressTypeNoneKey &&
			addrType != common.AddressTypeNoneScript {
			return false
		}
		cred, ok := addr.StakeCredential()
		return ok && ls.IsStakeCredentialRegistered(cred)
	}
	for _, proposal := range tx.ProposalProcedures() {
		returnAddr := proposal.RewardAccount()
		if !isRegistered(returnAddr) {
			return ProposalReturnAccountDoesNotExistError{Address: returnAddr}
		}

		govAction := proposal.GovAction()
		twAction, ok := govAction.(*common.TreasuryWithdrawalGovAction)
		if !ok || twAction == nil {
			continue
		}
		var badAddrs []common.Address
		for addr := range twAction.Withdrawals {
			if addr == nil {
				continue
			}
			if !isRegistered(*addr) {
				badAddrs = append(badAddrs, *addr)
			}
		}
		if len(badAddrs) > 0 {
			// Map iteration order is non-deterministic; sort before
			// constructing the error so the message is reproducible
			// across runs.
			slices.SortFunc(
				badAddrs,
				func(a, b common.Address) int {
					aBytes, _ := a.Bytes()
					bBytes, _ := b.Bytes()
					return bytes.Compare(aBytes, bBytes)
				},
			)
			return TreasuryWithdrawalReturnAccountsDoNotExistError{
				Addresses: badAddrs,
			}
		}
	}
	return nil
}

// validateProtocolParameterUpdate validates that a PPU is well-formed
func validateProtocolParameterUpdate(ppu *ConwayProtocolParameterUpdate) error {
	// Check if PPU is empty (no fields set)
	if ppu.MinFeeA == nil &&
		ppu.MinFeeB == nil &&
		ppu.MaxBlockBodySize == nil &&
		ppu.MaxTxSize == nil &&
		ppu.MaxBlockHeaderSize == nil &&
		ppu.KeyDeposit == nil &&
		ppu.PoolDeposit == nil &&
		ppu.MaxEpoch == nil &&
		ppu.NOpt == nil &&
		ppu.A0 == nil &&
		ppu.Rho == nil &&
		ppu.Tau == nil &&
		ppu.ProtocolVersion == nil &&
		ppu.MinPoolCost == nil &&
		ppu.AdaPerUtxoByte == nil &&
		len(ppu.CostModels) == 0 &&
		ppu.ExecutionCosts == nil &&
		ppu.MaxTxExUnits == nil &&
		ppu.MaxBlockExUnits == nil &&
		ppu.MaxValueSize == nil &&
		ppu.CollateralPercentage == nil &&
		ppu.MaxCollateralInputs == nil &&
		ppu.PoolVotingThresholds == nil &&
		ppu.DRepVotingThresholds == nil &&
		ppu.MinCommitteeSize == nil &&
		ppu.CommitteeTermLimit == nil &&
		ppu.GovActionValidityPeriod == nil &&
		ppu.GovActionDeposit == nil &&
		ppu.DRepDeposit == nil &&
		ppu.DRepInactivityPeriod == nil &&
		ppu.MinFeeRefScriptCostPerByte == nil {
		return ProtocolParameterUpdateEmptyError{}
	}

	// Validate individual fields that cannot be zero
	if ppu.MaxBlockHeaderSize != nil && *ppu.MaxBlockHeaderSize == 0 {
		return ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxBHSize",
			Value:     *ppu.MaxBlockHeaderSize,
		}
	}

	if ppu.MaxTxSize != nil && *ppu.MaxTxSize == 0 {
		return ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxTxSize",
			Value:     *ppu.MaxTxSize,
		}
	}

	if ppu.MaxValueSize != nil && *ppu.MaxValueSize == 0 {
		return ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxValSize",
			Value:     uint(*ppu.MaxValueSize),
		}
	}

	if ppu.MaxBlockBodySize != nil && *ppu.MaxBlockBodySize == 0 {
		return ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxBlockBodySize",
			Value:     *ppu.MaxBlockBodySize,
		}
	}

	return nil
}

// UtxoValidateIsValidFlag ensures transactions marked invalid have Plutus scripts
func UtxoValidateIsValidFlag(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// If IsValid is true, no check needed
	if tx.IsValid() {
		return nil
	}

	// If IsValid is false, transaction must have redeemers (indicating phase-2 validation)
	w := tx.Witnesses()
	if w != nil && w.Redeemers() != nil {
		for range w.Redeemers().Iter() {
			// Has at least one redeemer
			return nil
		}
	}

	// IsValid=false but no redeemers present
	return common.InvalidIsValidFlagError{}
}

// UtxoValidateRequiredVKeyWitnesses ensures required signers are accompanied by vkey witnesses
func UtxoValidateRequiredVKeyWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateRequiredVKeyWitnesses(tx)
}

// UtxoValidateCollateralVKeyWitnesses ensures collateral inputs are backed by vkey witnesses
func UtxoValidateCollateralVKeyWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateCollateralVKeyWitnesses(tx, ls)
}

// UtxoValidateRedeemerAndScriptWitnesses performs lightweight UTXOW checks for presence/absence of scripts vs redeemers
// Note: Conway needs custom handling for reference scripts. This function
// intentionally performs its own reference-input-aware checks instead of
// delegating to common.ValidateRedeemerAndScriptWitnesses because Conway
// treats reference scripts differently for extraneous witness validation:
// reference scripts alone do not trigger an extraneous witness error if no
// redeemers are present, unlike the common helper which considers any Plutus
// scripts (witnessed or referenced) as extraneous without redeemers.
func UtxoValidateRedeemerAndScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// Redeemer/script relation applies only to Plutus scripts. Native scripts
	// do NOT require redeemers.
	wits := tx.Witnesses()
	redeemerCount := 0
	if wits != nil {
		if r := wits.Redeemers(); r != nil {
			for range r.Iter() {
				redeemerCount++
			}
		}
	}
	// Detect Plutus availability separately for explicit witnesses and reference scripts.
	// This prevents reference scripts from being treated the same as provided script witnesses
	// when checking for extraneous witnesses.
	hasPlutusWitness := false
	hasPlutusReference := false
	if wits != nil {
		hasPlutusWitness = len(wits.PlutusV1Scripts()) > 0 ||
			len(wits.PlutusV2Scripts()) > 0 ||
			len(wits.PlutusV3Scripts()) > 0 ||
			len(common.PlutusV4ScriptsFromWitnessSet(wits)) > 0
	}

	// Consider Plutus reference scripts on reference inputs. If a reference input
	// cannot be resolved (UTxO lookup fails) validation should fail fast because
	// we cannot determine script availability deterministically without the UTxO.
	for _, refInput := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		if _, ok := common.PlutusScriptVersion(script); ok {
			hasPlutusReference = true
		}
		if hasPlutusReference {
			break
		}
	}

	// Per CIP-33, ScriptRef can also be provided via regular (spent) inputs.
	// Check regular inputs if not found in reference inputs.
	if !hasPlutusReference {
		for _, input := range tx.Inputs() {
			utxo, err := ls.UtxoById(input)
			if err != nil {
				// Skip errors - BadInputsUtxo will catch this
				continue
			}
			if utxo.Output == nil {
				continue
			}
			script := utxo.Output.ScriptRef()
			if script == nil {
				continue
			}
			if _, ok := common.PlutusScriptVersion(script); ok {
				hasPlutusReference = true
			}
			if hasPlutusReference {
				break
			}
		}
	}

	// If the body carries a script data hash, either redeemers or witness
	// datums must be present. Per the Cardano ledger spec, the script data
	// hash is required when the transaction has redeemers OR witness datums
	// (e.g. script deployment transactions that provide datum pre-images
	// without executing any scripts).
	if tx.ScriptDataHash() != nil && redeemerCount == 0 {
		hasDatums := wits != nil && len(wits.PlutusData()) > 0
		if !hasDatums {
			return MissingRedeemersForScriptDataHashError{}
		}
	}

	// If redeemers are present, we expect either a provided Plutus script witness
	// or a Plutus reference script on a reference input.
	if redeemerCount > 0 && (!hasPlutusWitness && !hasPlutusReference) {
		return MissingPlutusScriptWitnessesError{}
	}

	// If no redeemers are present but explicit Plutus script witnesses are supplied,
	// treat those supplied witnesses as extraneous. Reference scripts alone should
	// not trigger an extraneous-witness error since they don't represent supplied
	// script witnesses in the transaction.
	if redeemerCount == 0 && hasPlutusWitness {
		return ExtraneousPlutusScriptWitnessesError{}
	}

	return nil
}

// UtxoValidateScriptWitnesses checks that script witnesses are provided for all script address inputs
// and that there are no extraneous script witnesses.
func UtxoValidateScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateScriptWitnesses(tx, ls)
}

// UtxoValidateExtraneousRedeemers checks that all redeemers have valid purposes.
// A redeemer is "extraneous" if its index is out of bounds for its purpose type:
// - Spending redeemer index >= number of transaction inputs
// - Minting redeemer index >= number of distinct mint policies
// - Certificate redeemer index >= number of certificates
// - Reward redeemer index >= number of withdrawals
// - Voting redeemer index >= number of voters
// - Proposing redeemer index >= number of proposals
func UtxoValidateExtraneousRedeemers(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if err := common.ValidateExtraneousRedeemers(tx); err != nil {
		var extraErr common.ExtraneousRedeemerError
		if errors.As(err, &extraErr) {
			return ExtraRedeemerError{RedeemerKey: extraErr.RedeemerKey}
		}
		return err
	}
	return nil
}

// UtxoValidateCostModelsPresent ensures Plutus scripts have corresponding cost models in protocol parameters
func UtxoValidateCostModelsPresent(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}

	required := map[uint]struct{}{}
	wits := tmpTx.WitnessSet
	if len(wits.WsPlutusV1Scripts.Items()) > 0 {
		required[0] = struct{}{}
	}
	if len(wits.WsPlutusV2Scripts.Items()) > 0 {
		required[1] = struct{}{}
	}
	if len(wits.WsPlutusV3Scripts.Items()) > 0 {
		required[2] = struct{}{}
	}
	if len(common.PlutusV4ScriptsFromWitnessSet(wits)) > 0 {
		required[3] = struct{}{}
	}
	// Also include reference scripts on reference inputs
	for _, refInput := range tmpTx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		if version, ok := common.PlutusScriptVersion(script); ok {
			required[version] = struct{}{}
		}
	}

	// Per CIP-33, also include reference scripts on regular (spent) inputs
	for _, input := range tmpTx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			// Skip errors - BadInputsUtxo will catch this
			continue
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		if version, ok := common.PlutusScriptVersion(script); ok {
			required[version] = struct{}{}
		}
	}

	if len(required) == 0 {
		return nil
	}

	for version := range required {
		model, ok := tmpPparams.CostModels[version]
		if !ok || len(model) == 0 {
			return common.MissingCostModelError{Version: version}
		}
	}

	return nil
}

// UtxoValidateScriptDataHash validates the transaction's ScriptDataHash against the expected hash
// computed from redeemers, datums, and cost models (language views).
func UtxoValidateScriptDataHash(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}

	wits := tmpTx.WitnessSet
	hasRedeemers := wits.WsRedeemers.Len() > 0
	hasDatums := len(wits.WsPlutusData.Items()) > 0

	// Determine which Plutus versions are used
	usedVersions := make(map[uint]struct{})
	if len(wits.WsPlutusV1Scripts.Items()) > 0 {
		usedVersions[0] = struct{}{}
	}
	if len(wits.WsPlutusV2Scripts.Items()) > 0 {
		usedVersions[1] = struct{}{}
	}
	if len(wits.WsPlutusV3Scripts.Items()) > 0 {
		usedVersions[2] = struct{}{}
	}
	if len(common.PlutusV4ScriptsFromWitnessSet(wits)) > 0 {
		usedVersions[3] = struct{}{}
	}

	// Also check reference scripts
	for _, refInput := range tmpTx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		if version, ok := common.PlutusScriptVersion(script); ok {
			usedVersions[version] = struct{}{}
		}
	}

	// Check reference scripts on regular inputs.
	// These may provide reference scripts for minting, spending, etc.
	for _, input := range tmpTx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			continue
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		if version, ok := common.PlutusScriptVersion(script); ok {
			usedVersions[version] = struct{}{}
		}
	}

	declaredHash := tx.ScriptDataHash()

	// ScriptDataHash is required only when the transaction has redeemers or
	// witness datums, indicating actual script execution. The mere presence
	// of ScriptRefs in consumed/referenced UTxOs does NOT require a hash —
	// they are inert data unless matched by a redeemer.
	if !hasRedeemers && !hasDatums {
		if declaredHash != nil {
			return common.ExtraneousScriptDataHashError{Provided: *declaredHash}
		}
		return nil
	}

	if declaredHash == nil {
		return common.MissingScriptDataHashError{}
	}

	// Verify cost models are present for all used Plutus versions
	// (required for phase-2 validation even if we can't verify the exact hash)
	for version := range usedVersions {
		if _, ok := tmpPparams.CostModels[version]; !ok {
			return common.MissingCostModelError{Version: version}
		}
	}

	// Compute the expected ScriptDataHash
	// ScriptDataHash = blake2b256(redeemers_cbor || datums_cbor || langviews_cbor)
	//
	// Use preserved CBOR bytes from the original transaction for exact byte-for-byte match.
	// The hash was computed by the original submitter using their CBOR encoding.

	redeemersCbor := wits.WsRedeemers.Cbor()
	if len(redeemersCbor) == 0 {
		// Fall back to re-encoding if no preserved CBOR.
		// Note: Must encode empty map explicitly, as nil map encodes
		// as 0xf6 (CBOR null) but the spec expects 0xa0 (empty map)
		// for Conway empty redeemers.
		if wits.WsRedeemers.Len() == 0 && !wits.WsRedeemers.legacy {
			redeemersCbor = []byte{0xa0}
		} else {
			var err error
			redeemersCbor, err = cbor.Encode(wits.WsRedeemers)
			if err != nil {
				return err
			}
		}
	}

	// Get preserved CBOR bytes for datums (only if non-empty)
	var datumsCbor []byte
	if hasDatums {
		datumsCbor = wits.WsPlutusData.Cbor()
		if len(datumsCbor) == 0 {
			// Fall back to re-encoding if no preserved CBOR
			var err error
			datumsCbor, err = cbor.Encode(wits.WsPlutusData)
			if err != nil {
				return err
			}
		}
	}

	// Encode language views per the Cardano spec
	langViewsCbor, err := common.EncodeLangViews(
		usedVersions,
		tmpPparams.CostModels,
	)
	if err != nil {
		return err
	}

	// Concatenate and hash
	hashInput := make(
		[]byte,
		0,
		len(redeemersCbor)+len(datumsCbor)+len(langViewsCbor),
	)
	hashInput = append(hashInput, redeemersCbor...)
	hashInput = append(hashInput, datumsCbor...)
	hashInput = append(hashInput, langViewsCbor...)

	computedHash := common.Blake2b256Hash(hashInput)

	// Compare with declared hash
	// Note: declaredHash is guaranteed non-nil here due to earlier checks,
	// but we add an explicit check to satisfy static analysis
	if declaredHash == nil {
		return common.MissingScriptDataHashError{}
	}
	if *declaredHash != computedHash {
		return common.ScriptDataHashMismatchError{
			Declared: *declaredHash,
			Computed: computedHash,
		}
	}

	return nil
}

// UtxoValidateSignatures verifies vkey and bootstrap signatures present in the transaction.
func UtxoValidateSignatures(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.UtxoValidateSignatures(tx, slot, ls, pp)
}

func UtxoValidateOutsideValidityIntervalUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return allegra.UtxoValidateOutsideValidityIntervalUtxo(tx, slot, ls, pp)
}

func UtxoValidateInputSetEmptyUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateInputSetEmptyUtxo(tx, slot, ls, pp)
}

func UtxoValidateNoDuplicateInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateNoDuplicateInputs(tx, slot, ls, pp)
}

func UtxoValidateFeeTooSmallUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	minFee, err := MinFeeTx(tx, pp)
	if err != nil {
		return err
	}
	minFeeBigInt := new(big.Int).SetUint64(minFee)
	fee := tx.Fee()
	if fee == nil {
		fee = new(big.Int)
	}
	if fee.Cmp(minFeeBigInt) >= 0 {
		return nil
	}
	return shelley.FeeTooSmallUtxoError{
		Provided: fee,
		Min:      minFeeBigInt,
	}
}

func UtxoValidateInsufficientCollateral(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if tmpTx.WitnessSet.WsRedeemers.Len() == 0 {
		return nil
	}
	totalCollateral := new(big.Int)
	for _, collateralInput := range tx.Collateral() {
		utxo, err := ls.UtxoById(collateralInput)
		if err != nil {
			return err
		}
		if amount := utxo.Output.Amount(); amount != nil {
			totalCollateral.Add(totalCollateral, amount)
		}
	}
	// minCollateral = fee * collateralPercentage / 100
	fee := tmpTx.Fee()
	if fee == nil {
		fee = new(big.Int)
	}
	minCollateral := new(
		big.Int,
	).Mul(fee, new(big.Int).SetUint64(uint64(tmpPparams.CollateralPercentage)))
	minCollateral.Div(minCollateral, big.NewInt(100))
	if totalCollateral.Cmp(minCollateral) >= 0 {
		return nil
	}
	// Convert to uint64 for error struct (best effort)
	var providedU, requiredU uint64
	if totalCollateral.IsUint64() {
		providedU = totalCollateral.Uint64()
	}
	if minCollateral.IsUint64() {
		requiredU = minCollateral.Uint64()
	}
	return alonzo.InsufficientCollateralError{
		Provided: providedU,
		Required: requiredU,
	}
}

func UtxoValidateCollateralContainsNonAda(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if tmpTx.WitnessSet.WsRedeemers.Len() == 0 {
		return nil
	}
	badOutputs := []common.TransactionOutput{}
	totalCollateral := new(big.Int)
	totalAssets := common.NewMultiAsset[common.MultiAssetTypeOutput](nil)
	for _, collateralInput := range tx.Collateral() {
		utxo, err := ls.UtxoById(collateralInput)
		if err != nil {
			return err
		}
		amount := utxo.Output.Amount()
		if amount != nil {
			totalCollateral.Add(totalCollateral, amount)
		}
		assets := utxo.Output.Assets()
		totalAssets.Add(assets)
		if assets == nil || len(assets.Policies()) == 0 {
			continue
		}
		badOutputs = append(badOutputs, utxo.Output)
	}
	if len(badOutputs) == 0 {
		return nil
	}
	// Check if all collateral assets are accounted for in the collateral return
	collReturn := tx.CollateralReturn()
	if collReturn != nil {
		collReturnAssets := collReturn.Assets()
		if (&totalAssets).Compare(collReturnAssets) {
			return nil
		}
	}
	var providedU uint64
	if totalCollateral.IsUint64() {
		providedU = totalCollateral.Uint64()
	}
	return alonzo.CollateralContainsNonAdaError{
		Provided: providedU,
	}
}

// UtxoValidateCollateralEqBalance ensures that the collateral return amount is equal to the collateral input amount minus the total collateral
func UtxoValidateCollateralEqBalance(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return babbage.UtxoValidateCollateralEqBalance(tx, slot, ls, pp)
}

func UtxoValidateNoCollateralInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if tmpTx.WitnessSet.WsRedeemers.Len() == 0 {
		return nil
	}
	if len(tx.Collateral()) > 0 {
		return nil
	}
	return alonzo.NoCollateralInputsError{}
}

func UtxoValidateBadInputsUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateBadInputsUtxo(tx, slot, ls, pp)
}

func UtxoValidateValueNotConservedUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	// Calculate consumed value
	// consumed = value from input(s) + withdrawals + refunds
	consumedValue := new(big.Int)
	for _, tmpInput := range tx.Inputs() {
		tmpUtxo, err := ls.UtxoById(tmpInput)
		// Ignore errors fetching the UTxO and exclude it from calculations
		if err != nil {
			continue
		}
		if amount := tmpUtxo.Output.Amount(); amount != nil {
			consumedValue.Add(consumedValue, amount)
		}
	}
	for _, tmpWithdrawalAmount := range tx.Withdrawals() {
		if tmpWithdrawalAmount != nil {
			consumedValue.Add(consumedValue, tmpWithdrawalAmount)
		}
	}
	for _, cert := range tx.Certificates() {
		switch tmpCert := cert.(type) {
		case *common.DeregistrationCertificate:
			// CIP-0094 deregistration uses Amount field for refund (symmetric with registration deposit)
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			consumedValue.Add(consumedValue, big.NewInt(tmpCert.Amount))
		case *common.DeregistrationDrepCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			consumedValue.Add(consumedValue, big.NewInt(tmpCert.Amount))
		case *common.StakeDeregistrationCertificate:
			// Traditional stake deregistration uses protocol KeyDeposit parameter
			consumedValue.Add(consumedValue, new(big.Int).SetUint64(uint64(tmpPparams.KeyDeposit)))
			// Note: PoolRetirementCertificate does NOT refund the deposit as part of the transaction.
			// Pool deposits are refunded at epoch boundary after the retirement epoch has passed.
		}
	}
	// Add minted/burned ADA
	if tx.AssetMint() != nil {
		mintedAda := tx.AssetMint().Asset(common.Blake2b224{}, []byte{})
		if mintedAda != nil {
			consumedValue.Add(consumedValue, mintedAda)
		}
	}
	// Calculate produced value
	// produced = value from output(s) + fee + deposits
	producedValue := new(big.Int)
	for _, tmpOutput := range tx.Outputs() {
		if amount := tmpOutput.Amount(); amount != nil {
			producedValue.Add(producedValue, amount)
		}
	}
	if fee := tx.Fee(); fee != nil {
		producedValue.Add(producedValue, fee)
	}
	for _, cert := range tx.Certificates() {
		switch tmpCert := cert.(type) {
		case *common.PoolRegistrationCertificate:
			reg, _, err := ls.PoolCurrentState(common.Blake2b224(tmpCert.Operator))
			if err != nil {
				return err
			}
			if reg == nil {
				producedValue.Add(producedValue, new(big.Int).SetUint64(uint64(tmpPparams.PoolDeposit)))
			}
		case *common.RegistrationCertificate:
			// CIP-0094 registration uses Amount field for deposit
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue.Add(producedValue, big.NewInt(tmpCert.Amount))
		case *common.RegistrationDrepCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue.Add(producedValue, big.NewInt(tmpCert.Amount))
		case *common.StakeRegistrationCertificate:
			// Traditional stake registration uses protocol KeyDeposit parameter
			producedValue.Add(producedValue, new(big.Int).SetUint64(uint64(tmpPparams.KeyDeposit)))
		case *common.StakeRegistrationDelegationCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue.Add(producedValue, big.NewInt(tmpCert.Amount))
		case *common.StakeVoteRegistrationDelegationCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue.Add(producedValue, big.NewInt(tmpCert.Amount))
		case *common.VoteRegistrationDelegationCertificate:
			if tmpCert.Amount <= 0 {
				return shelley.InvalidCertificateDepositError{
					CertificateType: common.CertificateType(tmpCert.CertType),
					Amount:          tmpCert.Amount,
				}
			}
			producedValue.Add(producedValue, big.NewInt(tmpCert.Amount))
		}
	}
	for _, proposal := range tx.ProposalProcedures() {
		producedValue.Add(
			producedValue,
			new(big.Int).SetUint64(proposal.Deposit()),
		)
	}
	// Add treasury donation - value leaving the transaction to go to the treasury
	// Treasury donations are a Conway feature and cannot be used with PlutusV1/V2 scripts
	donation := tx.Donation()
	if donation != nil && donation.Sign() > 0 {
		// Check if transaction uses PlutusV1 or PlutusV2 scripts in witnesses
		witnesses := tx.Witnesses()
		plutusVersion := ""
		if witnesses != nil {
			if len(witnesses.PlutusV1Scripts()) > 0 {
				plutusVersion = "PlutusV1"
			} else if len(witnesses.PlutusV2Scripts()) > 0 {
				plutusVersion = "PlutusV2"
			}
		}
		// Also check reference scripts on reference inputs
		if plutusVersion == "" {
			for _, refInput := range tx.ReferenceInputs() {
				utxo, err := ls.UtxoById(refInput)
				if err != nil {
					return common.ReferenceInputResolutionError{
						Input: refInput,
						Err:   err,
					}
				}
				if utxo.Output == nil {
					continue
				}
				script := utxo.Output.ScriptRef()
				if script != nil {
					switch script.(type) {
					case common.PlutusV1Script:
						plutusVersion = "PlutusV1"
					case common.PlutusV2Script:
						plutusVersion = "PlutusV2"
					}
					if plutusVersion != "" {
						break
					}
				}
			}
		}
		// Return explicit error if donation is used with PlutusV1/V2 scripts
		if plutusVersion != "" {
			var donationU uint64
			if donation.IsUint64() {
				donationU = donation.Uint64()
			}
			return TreasuryDonationWithPlutusV1V2Error{
				Donation:      donationU,
				PlutusVersion: plutusVersion,
			}
		}
		// Only apply donation if not using PlutusV1/V2 scripts
		producedValue.Add(producedValue, donation)
	}
	if consumedValue.Cmp(producedValue) != 0 {
		return shelley.ValueNotConservedUtxoError{
			Consumed: consumedValue,
			Produced: producedValue,
		}
	}

	// Multi-asset value conservation check
	// For each policy and asset: consumed + minted == produced
	type assetKey struct {
		policy common.Blake2b224
		asset  string
	}

	consumedAssets := make(map[assetKey]*big.Int)
	producedAssets := make(map[assetKey]*big.Int)

	// Collect consumed multi-assets from inputs
	for _, tmpInput := range tx.Inputs() {
		tmpUtxo, err := ls.UtxoById(tmpInput)
		if err != nil {
			continue
		}
		if assets := tmpUtxo.Output.Assets(); assets != nil {
			for _, policy := range assets.Policies() {
				for _, assetName := range assets.Assets(policy) {
					amount := assets.Asset(policy, assetName)
					if amount == nil {
						continue
					}
					key := assetKey{policy: policy, asset: string(assetName)}
					if consumedAssets[key] == nil {
						consumedAssets[key] = new(big.Int)
					}
					consumedAssets[key].Add(consumedAssets[key], amount)
				}
			}
		}
	}

	// Add minted/burned assets to consumed (positive for mint, negative for burn)
	if mint := tx.AssetMint(); mint != nil {
		for _, policy := range mint.Policies() {
			// Skip ADA (empty policy ID) as it's tracked separately in consumed/produced value
			if policy == (common.Blake2b224{}) {
				continue
			}
			for _, assetName := range mint.Assets(policy) {
				amount := mint.Asset(policy, assetName)
				if amount == nil {
					continue
				}
				key := assetKey{policy: policy, asset: string(assetName)}
				if consumedAssets[key] == nil {
					consumedAssets[key] = new(big.Int)
				}
				consumedAssets[key].Add(consumedAssets[key], amount)
			}
		}
	}

	// Collect produced multi-assets from outputs
	for _, tmpOutput := range tx.Outputs() {
		if assets := tmpOutput.Assets(); assets != nil {
			for _, policy := range assets.Policies() {
				for _, assetName := range assets.Assets(policy) {
					amount := assets.Asset(policy, assetName)
					if amount == nil {
						continue
					}
					key := assetKey{policy: policy, asset: string(assetName)}
					if producedAssets[key] == nil {
						producedAssets[key] = new(big.Int)
					}
					producedAssets[key].Add(producedAssets[key], amount)
				}
			}
		}
	}

	// Check that all consumed assets match produced assets without building
	// an intermediate union set of keys.
	zero := new(big.Int)
	for key, consumed := range consumedAssets {
		produced := producedAssets[key]
		if produced == nil {
			produced = zero
		}
		if consumed.Cmp(produced) != 0 {
			return shelley.ValueNotConservedUtxoError{
				Consumed: consumed,
				Produced: produced,
			}
		}
		delete(producedAssets, key)
	}
	for _, produced := range producedAssets {
		if produced.Cmp(zero) != 0 {
			return shelley.ValueNotConservedUtxoError{
				Consumed: zero,
				Produced: produced,
			}
		}
	}

	return nil
}

func UtxoValidateOutputTooSmallUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	var badOutputs []common.TransactionOutput
	for _, tmpOutput := range tx.Outputs() {
		minCoin, err := MinCoinTxOut(tmpOutput, pp)
		if err != nil {
			return err
		}
		minCoinBig := new(big.Int).SetUint64(minCoin)
		amount := tmpOutput.Amount()
		if amount == nil {
			amount = new(big.Int)
		}
		if amount.Cmp(minCoinBig) < 0 {
			badOutputs = append(badOutputs, tmpOutput)
		}
	}
	if len(badOutputs) == 0 {
		return nil
	}
	return shelley.OutputTooSmallUtxoError{
		Outputs: badOutputs,
	}
}

func UtxoValidateOutputTooBigUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	badOutputs := []common.TransactionOutput{}
	for _, txOutput := range tx.Outputs() {
		tmpOutput, ok := txOutput.(*babbage.BabbageTransactionOutput)
		if !ok {
			return errors.New("transaction output is not expected type")
		}
		outputValBytes, err := cbor.Encode(tmpOutput.OutputAmount)
		if err != nil {
			return err
		}
		if uint(len(outputValBytes)) <= tmpPparams.MaxValueSize {
			continue
		}
		badOutputs = append(badOutputs, tmpOutput)
	}
	if len(badOutputs) == 0 {
		return nil
	}
	return mary.OutputTooBigUtxoError{
		Outputs: badOutputs,
	}
}

func UtxoValidateOutputBootAddrAttrsTooBig(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateOutputBootAddrAttrsTooBig(tx, slot, ls, pp)
}

func UtxoValidateInlineDatumsWithPlutusV1(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return babbage.UtxoValidateInlineDatumsWithPlutusV1(tx, slot, ls, pp)
}

// UtxoValidateConwayFeaturesWithPlutusV1V2 ensures Conway-specific features
// (CurrentTreasuryValue, ProposalProcedures, VotingProcedures, Conway certificates)
// are not used with PlutusV1/V2 scripts.
// These features are only available in the PlutusV3 script context.
func UtxoValidateConwayFeaturesWithPlutusV1V2(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// First check for PlutusV1/V2 scripts in witness set
	plutusVersion := ""
	witnesses := tx.Witnesses()
	if witnesses != nil {
		if len(witnesses.PlutusV1Scripts()) > 0 {
			plutusVersion = "PlutusV1"
		} else if len(witnesses.PlutusV2Scripts()) > 0 {
			plutusVersion = "PlutusV2"
		}
	}

	// Also check reference scripts on reference inputs
	if plutusVersion == "" {
		for _, refInput := range tx.ReferenceInputs() {
			utxo, err := ls.UtxoById(refInput)
			if err != nil {
				return common.ReferenceInputResolutionError{
					Input: refInput,
					Err:   err,
				}
			}
			if utxo.Output == nil {
				continue
			}
			script := utxo.Output.ScriptRef()
			if script != nil {
				switch script.(type) {
				case common.PlutusV1Script:
					plutusVersion = "PlutusV1"
				case common.PlutusV2Script:
					plutusVersion = "PlutusV2"
				}
				if plutusVersion != "" {
					break
				}
			}
		}
	}

	// Also check reference scripts on regular inputs
	if plutusVersion == "" {
		for _, input := range tx.Inputs() {
			utxo, err := ls.UtxoById(input)
			if err != nil {
				continue
			}
			if utxo.Output == nil {
				continue
			}
			script := utxo.Output.ScriptRef()
			if script != nil {
				switch script.(type) {
				case common.PlutusV1Script:
					plutusVersion = "PlutusV1"
				case common.PlutusV2Script:
					plutusVersion = "PlutusV2"
				}
				if plutusVersion != "" {
					break
				}
			}
		}
	}

	// No V1/V2 scripts found, Conway features are fine
	if plutusVersion == "" {
		return nil
	}

	// Check for Conway-specific features
	hasCurrentTreasuryValue := tx.CurrentTreasuryValue() != nil &&
		tx.CurrentTreasuryValue().Sign() > 0
	hasProposalProcedures := len(tx.ProposalProcedures()) > 0
	hasVotingProcedures := tx.VotingProcedures() != nil &&
		len(tx.VotingProcedures()) > 0

	// Return appropriate error based on which Conway feature is present
	if hasCurrentTreasuryValue {
		return CurrentTreasuryValueWithPlutusV1V2Error{
			PlutusVersion: plutusVersion,
		}
	}
	if hasProposalProcedures {
		return ProposalProceduresWithPlutusV1V2Error{
			PlutusVersion: plutusVersion,
		}
	}
	if hasVotingProcedures {
		return VotingProceduresWithPlutusV1V2Error{PlutusVersion: plutusVersion}
	}

	// Check for Conway-specific certificates that are NOT representable in V1/V2 contexts
	// Note: RegistrationCertificate and DeregistrationCertificate ARE supported (mapped to V1/V2 equivalents)
	for _, cert := range tx.Certificates() {
		certType := ""
		switch cert.(type) {
		case *common.AuthCommitteeHotCertificate:
			certType = "AuthCommitteeHot"
		case *common.ResignCommitteeColdCertificate:
			certType = "ResignCommitteeCold"
		case *common.RegistrationDrepCertificate:
			certType = "DRepRegistration"
		case *common.DeregistrationDrepCertificate:
			certType = "DRepDeregistration"
		case *common.UpdateDrepCertificate:
			certType = "DRepUpdate"
		case *common.StakeVoteDelegationCertificate:
			certType = "StakeVoteDelegation"
		case *common.StakeRegistrationDelegationCertificate:
			certType = "StakeRegistrationDelegation"
		case *common.VoteDelegationCertificate:
			certType = "VoteDelegation"
		case *common.VoteRegistrationDelegationCertificate:
			certType = "VoteRegistrationDelegation"
		case *common.StakeVoteRegistrationDelegationCertificate:
			certType = "StakeVoteRegistrationDelegation"
		}
		if certType != "" {
			return ConwayCertificateWithPlutusV1V2Error{
				PlutusVersion:   plutusVersion,
				CertificateType: certType,
			}
		}
	}

	return nil
}

func UtxoValidateWrongNetwork(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateWrongNetwork(tx, slot, ls, pp)
}

func UtxoValidateWrongNetworkWithdrawal(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateWrongNetworkWithdrawal(tx, slot, ls, pp)
}

// UtxoValidateTransactionNetworkId validates that if the transaction body
// specifies a NetworkId field, it must match the ledger state's network
func UtxoValidateTransactionNetworkId(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// Only Conway transactions have the NetworkId field in the body
	conwayTx, ok := tx.(*ConwayTransaction)
	if !ok {
		// Not a Conway transaction, skip this validation
		return nil
	}

	// Get the transaction's optional NetworkId field
	txNetworkId := conwayTx.NetworkId()
	if txNetworkId == nil {
		// NetworkId not specified in transaction, that's fine
		return nil
	}

	// NetworkId is specified, must match ledger state
	ledgerNetworkId := ls.NetworkId()
	if uint(*txNetworkId) != ledgerNetworkId {
		return WrongTransactionNetworkIdError{
			TxNetworkId:     *txNetworkId,
			LedgerNetworkId: ledgerNetworkId,
		}
	}

	return nil
}

func UtxoValidateMaxTxSizeUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	txBytes, err := cbor.Encode(tx)
	if err != nil {
		return err
	}
	if uint(len(txBytes)) <= tmpPparams.MaxTxSize {
		return nil
	}
	return shelley.MaxTxSizeUtxoError{
		TxSize:    uint(len(txBytes)),
		MaxTxSize: tmpPparams.MaxTxSize,
	}
}

func UtxoValidateExUnitsTooBigUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*ConwayTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	var totalSteps, totalMemory int64
	for _, redeemer := range tmpTx.WitnessSet.WsRedeemers.Iter() {
		newSteps, ok := common.AddInt64Checked(
			totalSteps,
			redeemer.ExUnits.Steps,
		)
		if !ok {
			return alonzo.ExUnitsTooBigUtxoError{
				TotalExUnits: common.ExUnits{
					Memory: totalMemory,
					Steps:  totalSteps,
				},
				MaxTxExUnits: tmpPparams.MaxTxExUnits,
			}
		}
		totalSteps = newSteps
		newMemory, ok := common.AddInt64Checked(
			totalMemory,
			redeemer.ExUnits.Memory,
		)
		if !ok {
			return alonzo.ExUnitsTooBigUtxoError{
				TotalExUnits: common.ExUnits{
					Memory: totalMemory,
					Steps:  totalSteps,
				},
				MaxTxExUnits: tmpPparams.MaxTxExUnits,
			}
		}
		totalMemory = newMemory
	}
	if totalSteps <= tmpPparams.MaxTxExUnits.Steps &&
		totalMemory <= tmpPparams.MaxTxExUnits.Memory {
		return nil
	}
	return alonzo.ExUnitsTooBigUtxoError{
		TotalExUnits: common.ExUnits{
			Memory: totalMemory,
			Steps:  totalSteps,
		},
		MaxTxExUnits: tmpPparams.MaxTxExUnits,
	}
}

func UtxoValidateTooManyCollateralInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	collateralCount := uint(len(tx.Collateral()))
	if collateralCount <= tmpPparams.MaxCollateralInputs {
		return nil
	}
	return babbage.TooManyCollateralInputsError{
		Provided: collateralCount,
		Max:      tmpPparams.MaxCollateralInputs,
	}
}

// MinFeeTx calculates the minimum required fee for a transaction based on
// protocol parameters. The fee-relevant transaction size is determined by
// common.TxSizeForFee, which uses the original on-wire CBOR length and
// subtracts 1 byte for Alonzo+ eras (the IsValid boolean is excluded from
// the fee computation per the Cardano ledger spec toCBORForSizeComputation).
func MinFeeTx(
	tx common.Transaction,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*ConwayProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	txSize, err := common.TxSizeForFee(tx)
	if err != nil {
		return 0, err
	}
	minFee, err := common.CalculateMinFee(
		txSize,
		tmpPparams.MinFeeA,
		tmpPparams.MinFeeB,
	)
	if err != nil {
		return 0, err
	}
	return minFee, nil
}

// MinCoinTxOut calculates the minimum coin for a transaction output based on protocol parameters.
// Per CIP-55, the formula includes a 160-byte constant overhead to account for the transaction
// input and UTxO map entry overhead that is not captured in the CBOR serialization.
// Formula: minCoin = coinsPerUTxOByte * (160 + serializedOutputSize)
// Reference: https://cips.cardano.org/cip/CIP-55
const minUtxoOverheadBytes = 160

func MinCoinTxOut(
	txOut common.TransactionOutput,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*ConwayProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	txOutBytes, err := cbor.Encode(txOut)
	if err != nil {
		return 0, err
	}
	minCoinTxOut := tmpPparams.AdaPerUtxoByte * (minUtxoOverheadBytes + uint64(len(txOutBytes)))
	return minCoinTxOut, nil
}

func UtxoValidateMetadata(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateMetadata(tx, slot, ls, pp)
}

// UtxoValidateSupplementalDatums checks that all datums in the witness set are
// justified by being referenced by a datum hash in spent inputs, reference inputs,
// or transaction outputs.
// Inline datums are not considered - only non-inline datum hashes justify witness datums.
func UtxoValidateSupplementalDatums(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	witnesses := tx.Witnesses()
	if witnesses == nil {
		return nil
	}

	// Get all datums from witness set
	witnessDatums := witnesses.PlutusData()
	if len(witnessDatums) == 0 {
		return nil
	}

	// Collect all "justified" datum hashes - those referenced by UTxOs being spent
	justifiedHashes := make(map[common.Blake2b256]bool)

	// Check regular inputs
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			continue // UTxO not found - will fail BadInputsUtxo rule
		}
		if utxo.Output == nil {
			continue
		}
		// Only non-inline datums justify witness datums
		if utxo.Output.Datum() == nil {
			if datumHash := utxo.Output.DatumHash(); datumHash != nil {
				justifiedHashes[*datumHash] = true
			}
		}
	}

	// Check transaction outputs - datum hashes in outputs also justify witness datums
	for _, output := range tx.Outputs() {
		if output.Datum() == nil {
			if datumHash := output.DatumHash(); datumHash != nil {
				justifiedHashes[*datumHash] = true
			}
		}
	}

	// Check reference inputs as well - datums referenced there are also justified
	for _, input := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			continue
		}
		if utxo.Output == nil {
			continue
		}
		if utxo.Output.Datum() == nil {
			if datumHash := utxo.Output.DatumHash(); datumHash != nil {
				justifiedHashes[*datumHash] = true
			}
		}
	}

	// Check for supplemental (unjustified) datums
	var supplementalHashes []common.Blake2b256
	for _, datum := range witnessDatums {
		datumHash := datum.Hash()
		if !justifiedHashes[datumHash] {
			supplementalHashes = append(supplementalHashes, datumHash)
		}
	}

	if len(supplementalHashes) > 0 {
		return NotAllowedSupplementalDatumsError{
			DatumHashes: supplementalHashes,
		}
	}

	return nil
}

// UtxoValidatePlutusScripts executes all Plutus scripts in the transaction
// and validates that they pass. This is the phase-2 validation.
func UtxoValidatePlutusScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	conwayPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	// Skip if transaction is marked as invalid (phase-2 failure already indicated)
	if !tx.IsValid() {
		return nil
	}

	// Check if there are any redeemers
	witnesses := tx.Witnesses()
	if witnesses == nil {
		return nil
	}
	redeemers := witnesses.Redeemers()
	if redeemers == nil {
		return nil
	}

	// Count redeemers to see if we have any scripts to execute
	redeemerCount := 0
	for range redeemers.Iter() {
		redeemerCount++
	}
	if redeemerCount == 0 {
		return nil
	}

	// Resolve all inputs (regular + reference) for building script context
	inputsResolved, refInputsResolved, err := script.ResolveTxInputs(tx, ls)
	if err != nil {
		return err
	}
	resolvedInputs := script.ConcatResolvedInputs(
		inputsResolved,
		refInputsResolved,
	)
	resolvedInputsMap := make(
		map[string]common.Utxo,
		len(inputsResolved)+len(refInputsResolved),
	)
	for i, input := range tx.Inputs() {
		resolvedInputsMap[input.String()] = inputsResolved[i]
	}
	for i, refInput := range tx.ReferenceInputs() {
		resolvedInputsMap[refInput.String()] = refInputsResolved[i]
	}

	// Build TxInfo lazily based on script version
	var txInfoV1 script.TxInfoV1
	var txInfoV2 script.TxInfoV2
	var txInfoV3 script.TxInfoV3
	var txInfoV1Built, txInfoV2Built, txInfoV3Built bool

	// Collect all available scripts (witness scripts + reference scripts)
	availableScripts := script.AvailablePlutusScripts(tx, resolvedInputs)

	// Get sorted inputs for redeemer index mapping.
	// The Cardano ledger spec requires spend redeemer indices to
	// reference positions in the canonically sorted input list.
	inputs := script.SortInputs(tx.Inputs())
	assetMint := tx.AssetMint()
	if assetMint == nil {
		assetMint = &common.MultiAsset[common.MultiAssetTypeMint]{}
	}
	withdrawals := tx.Withdrawals()
	votes := tx.VotingProcedures()
	proposalProcedures := tx.ProposalProcedures()
	certificates := tx.Certificates()

	// Build witness datums map for datum lookup
	plutusData := witnesses.PlutusData()
	witnessDatums := make(map[common.Blake2b256]*common.Datum)
	for i := range plutusData {
		datum := plutusData[i]
		witnessDatums[datum.Hash()] = &datum
	}

	// Execute each redeemer's script
	for redeemerKey, redeemerValue := range redeemers.Iter() {
		// Build script purpose for this redeemer
		purpose, err := script.BuildScriptPurpose(
			redeemerKey,
			resolvedInputsMap,
			inputs,
			*assetMint,
			certificates,
			withdrawals,
			votes,
			proposalProcedures,
			witnessDatums,
		)
		if err != nil {
			// Redeemer doesn't match any valid purpose (index out of bounds, etc.)
			return ExtraRedeemerError{RedeemerKey: redeemerKey}
		}

		// Check if the purpose actually requires a script
		// If not, this redeemer is "extra" (mismatched)
		switch p := purpose.(type) {
		case script.ScriptPurposeSpending:
			// For spending purposes, verify the input is at a script address
			if p.Input.Output != nil {
				addr := p.Input.Output.Address()
				if (addr.Type() & common.AddressTypeScriptBit) == 0 {
					// Input is at a key address, not a script address
					return ExtraRedeemerError{RedeemerKey: redeemerKey}
				}
			}
		case script.ScriptPurposeCertifying:
			// For certifying purposes, check if the certificate has a script credential
			// ScriptHash() returns empty hash for key credentials
			if p.ScriptHash() == (common.ScriptHash{}) {
				return ExtraRedeemerError{RedeemerKey: redeemerKey}
			}
		case script.ScriptPurposeRewarding:
			// For rewarding purposes, check if the credential is a script
			if p.StakeCredential.CredType != common.CredentialTypeScriptHash {
				return ExtraRedeemerError{RedeemerKey: redeemerKey}
			}
		case script.ScriptPurposeProposing:
			// For proposing purposes, check if the proposal has a policy script
			// If not (empty ScriptHash), this redeemer is "extra"
			if p.ScriptHash() == (common.ScriptHash{}) {
				return ExtraRedeemerError{RedeemerKey: redeemerKey}
			}
		}

		// Find the script for this purpose
		scriptHash := purpose.ScriptHash()
		plutusScript, ok := availableScripts[scriptHash]
		if !ok {
			// Missing script should be caught by MissingScriptWitnesses
			continue
		}

		// Get datum for V1/V2 scripts (spending purpose only)
		var datum data.PlutusData
		var spendInput common.TransactionInput
		if spendPurpose, ok := purpose.(script.ScriptPurposeSpending); ok {
			if spendPurpose.Datum != nil {
				datum = spendPurpose.Datum
			}
			spendInput = spendPurpose.Input.Id
		}

		// Execute based on script version
		var execErr error
		switch s := plutusScript.(type) {
		case common.PlutusV4Script:
			return common.PlutusScriptValidationUnsupportedError{Era: EraNameConway}
		case common.PlutusV3Script:
			// Build V3 TxInfo lazily
			if !txInfoV3Built {
				var err error
				txInfoV3, err = script.NewTxInfoV3FromTransaction(ls, tx, resolvedInputs)
				if err != nil {
					return ScriptContextConstructionError{Err: err}
				}
				txInfoV3Built = true
			}
			// Build V3 context
			redeemer := script.Redeemer{
				Tag:     redeemerKey.Tag,
				Index:   redeemerKey.Index,
				Data:    redeemerValue.Data.Data,
				ExUnits: redeemerValue.ExUnits,
			}
			ctx := script.NewScriptContextV3(txInfoV3, redeemer, purpose)
			ctxData := ctx.ToPlutusData()
			evalContext, err := cek.NewEvalContext(
				lang.LanguageVersionV3,
				cek.ProtoVersion{
					Major: conwayPparams.ProtocolVersion.Major,
					Minor: conwayPparams.ProtocolVersion.Minor,
				},
				conwayPparams.CostModels[2],
			)
			if err != nil {
				return fmt.Errorf("build evaluation context: %w", err)
			}
			_, execErr = s.Evaluate(ctxData, redeemerValue.ExUnits, evalContext)
		case common.PlutusV2Script:
			// V2 scripts require a datum for spending purposes
			if _, isSpend := purpose.(script.ScriptPurposeSpending); isSpend && datum == nil {
				return MissingDatumForSpendingScriptError{
					ScriptHash: scriptHash,
					Input:      spendInput,
				}
			}
			// Build V2 TxInfo lazily
			if !txInfoV2Built {
				var err error
				txInfoV2, err = script.NewTxInfoV2FromTransaction(ls, tx, resolvedInputs)
				if err != nil {
					return ScriptContextConstructionError{Err: err}
				}
				txInfoV2Built = true
			}
			// Build V1V2 context
			ctx := script.NewScriptContextV1V2(txInfoV2, purpose)
			ctxData := ctx.ToPlutusData()
			evalContext, err := cek.NewEvalContext(
				lang.LanguageVersionV2,
				cek.ProtoVersion{
					Major: conwayPparams.ProtocolVersion.Major,
					Minor: conwayPparams.ProtocolVersion.Minor,
				},
				conwayPparams.CostModels[1],
			)
			if err != nil {
				return fmt.Errorf("build evaluation context: %w", err)
			}
			_, execErr = s.Evaluate(datum, redeemerValue.Data.Data, ctxData, redeemerValue.ExUnits, evalContext)
		case common.PlutusV1Script:
			// V1 scripts require a datum for spending purposes
			if _, isSpend := purpose.(script.ScriptPurposeSpending); isSpend && datum == nil {
				return MissingDatumForSpendingScriptError{
					ScriptHash: scriptHash,
					Input:      spendInput,
				}
			}
			// Build V1 TxInfo lazily
			if !txInfoV1Built {
				var err error
				txInfoV1, err = script.NewTxInfoV1FromTransaction(ls, tx, resolvedInputs)
				if err != nil {
					return ScriptContextConstructionError{Err: err}
				}
				txInfoV1Built = true
			}
			// Build V1V2 context
			ctx := script.NewScriptContextV1V2(txInfoV1, purpose)
			ctxData := ctx.ToPlutusData()
			evalContext, err := cek.NewEvalContext(
				lang.LanguageVersionV1,
				cek.ProtoVersion{
					Major: conwayPparams.ProtocolVersion.Major,
					Minor: conwayPparams.ProtocolVersion.Minor,
				},
				conwayPparams.CostModels[0],
			)
			if err != nil {
				return fmt.Errorf("build evaluation context: %w", err)
			}
			_, execErr = s.Evaluate(datum, redeemerValue.Data.Data, ctxData, redeemerValue.ExUnits, evalContext)
		default:
			continue
		}

		if execErr != nil {
			return PlutusScriptFailedError{
				ScriptHash: scriptHash,
				Tag:        redeemerKey.Tag,
				Index:      redeemerKey.Index,
				Err:        execErr,
			}
		}
	}

	return nil
}

// UtxoValidateNativeScripts evaluates native scripts in the transaction.
// Native scripts (timelock scripts) are evaluated based on:
// - Signatures present in the transaction
// - Transaction's validity interval
// This is phase-1 validation for native scripts.
func UtxoValidateNativeScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	witnesses := tx.Witnesses()
	nativeScripts, err := common.NativeScriptsForValidation(tx, ls)
	if err != nil {
		return err
	}
	if len(nativeScripts) == 0 {
		return nil
	}

	// Collect key hashes from VKey witnesses
	keyHashes := make(map[common.Blake2b224]bool)
	if witnesses != nil {
		for _, vkw := range witnesses.Vkey() {
			// VKey witnesses contain the public key, we need its hash
			keyHash := common.Blake2b224Hash(vkw.Vkey)
			keyHashes[keyHash] = true
		}
		// Also collect key hashes from bootstrap witnesses (Byron-era)
		for _, bw := range witnesses.Bootstrap() {
			keyHash := common.Blake2b224Hash(bw.PublicKey)
			keyHashes[keyHash] = true
		}
	}

	// Get transaction validity interval
	validityStart := tx.ValidityIntervalStart()
	validityEnd, validityEndPresent := common.TransactionValidityIntervalUpperBound(
		tx,
	)
	if !validityEndPresent {
		validityEnd = ^uint64(0) // Max uint64 if not set
	}

	// Evaluate each native script
	for _, nscript := range nativeScripts {
		scriptHash := nscript.Hash()
		if !nscript.Evaluate(slot, validityStart, validityEnd, keyHashes) {
			return NativeScriptFailedError{ScriptHash: scriptHash}
		}
	}

	return nil
}

// UtxoValidateDelegation validates delegation certificates against ledger state.
// It checks:
// - Pool registration status for stake delegations
// - Stake credential registration for non-registration delegations
//
// The function tracks in-transaction registrations to handle cases where
// registration and delegation are in the same transaction.
func UtxoValidateDelegation(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// Track credential registration state changes within this transaction.
	// The bool records both registrations and deregistrations so later
	// certificates observe the state produced by earlier certificates.
	type stakeCredentialKey struct {
		credType uint
		hash     common.Blake2b224
	}
	stakeKey := func(cred common.Credential) stakeCredentialKey {
		return stakeCredentialKey{
			credType: cred.CredType,
			hash:     cred.Credential,
		}
	}
	inTxStakeState := make(map[stakeCredentialKey]bool)
	inTxPoolRegs := make(map[common.PoolKeyHash]bool)
	inTxDRepRegs := make(map[common.Blake2b224]bool)
	// Track VRF keys seen in this transaction (for PV11+ duplicate detection)
	inTxVrfKeys := make(map[common.Blake2b256]common.PoolKeyHash)

	// Helper to check if stake credential is registered (in state or in-tx)
	isStakeRegistered := func(cred common.Credential) bool {
		if registered, ok := inTxStakeState[stakeKey(cred)]; ok {
			return registered
		}
		return ls.IsStakeCredentialRegistered(cred)
	}

	registerStakeCredential := func(cred common.Credential) error {
		if isStakeRegistered(cred) {
			return StakeCredentialAlreadyRegisteredError{
				Credential: cred,
			}
		}
		inTxStakeState[stakeKey(cred)] = true
		return nil
	}

	// Helper to check if pool is registered (in state or in-tx)
	isPoolRegistered := func(poolKeyHash common.PoolKeyHash) bool {
		return ls.IsPoolRegistered(poolKeyHash) || inTxPoolRegs[poolKeyHash]
	}

	// Helper to check if DRep is registered (in state or in-tx).
	// Returns true for special DRep types (Abstain, NoConfidence) as they
	// don't need registration. Any DRep type other than key hash, script
	// hash, Abstain, or NoConfidence is rejected outright via
	// InvalidDRepTypeError -- this closes a gap where a programmatically
	// constructed (non-CBOR-decoded) common.Drep could carry an
	// out-of-range Type value and be treated as an unregistered key/script
	// hash DRep instead of being rejected.
	isDRepRegistered := func(drep common.Drep) (bool, error) {
		switch drep.Type {
		case common.DrepTypeAbstain, common.DrepTypeNoConfidence:
			// Special DRep types don't require registration
			return true, nil
		case common.DrepTypeAddrKeyHash, common.DrepTypeScriptHash:
			// For key hash and script hash types, check registration
			if len(drep.Credential) != 28 {
				return false, nil
			}
			var credHash common.Blake2b224
			copy(credHash[:], drep.Credential)
			// Check in-tx registrations first
			if inTxDRepRegs[credHash] {
				return true, nil
			}
			// Check ledger state
			reg, err := ls.DRepRegistration(credHash)
			return err == nil && reg != nil, nil
		default:
			return false, InvalidDRepTypeError{DrepType: drep.Type}
		}
	}

	// Helper to convert Drep type to credential type. Only ever invoked
	// after isDRepRegistered has confirmed the type is one of
	// DrepTypeAddrKeyHash/DrepTypeScriptHash (any other type returns an
	// error from isDRepRegistered before this is reached), so the default
	// case below is unreachable and does not fall back silently.
	drepTypeToCredType := func(drepType int) (uint, error) {
		switch drepType {
		case common.DrepTypeAddrKeyHash:
			return common.CredentialTypeAddrKeyHash, nil
		case common.DrepTypeScriptHash:
			return common.CredentialTypeScriptHash, nil
		default:
			return 0, InvalidDRepTypeError{DrepType: drepType}
		}
	}

	for _, cert := range tx.Certificates() {
		switch c := cert.(type) {
		// Track registrations for in-tx state
		case *common.RegistrationCertificate:
			if err := registerStakeCredential(c.StakeCredential); err != nil {
				return err
			}

		case *common.StakeRegistrationCertificate:
			if err := registerStakeCredential(c.StakeCredential); err != nil {
				return err
			}

		case *common.PoolRegistrationCertificate:
			inTxPoolRegs[c.Operator] = true
			// PV11+: Validate VRF key uniqueness for pool registrations
			conwayPp, ok := pp.(*ConwayProtocolParameters)
			if ok && common.IsProtocolVersionAtLeast(conwayPp.ProtocolVersion.Major, 0, common.ProtocolVersionVanRossem) {
				// Check for in-tx VRF key duplicates first
				if existingPoolId, exists := inTxVrfKeys[c.VrfKeyHash]; exists {
					// Allow same pool to re-register with same VRF key
					if existingPoolId != c.Operator {
						return DuplicateVrfKeyError{
							VrfKeyHash:     c.VrfKeyHash,
							NewPoolId:      c.Operator,
							ExistingPoolId: existingPoolId,
						}
					}
				}
				// Check against ledger state
				if err := PoolValidateVrfKeyUniqueness(c, conwayPp.ProtocolVersion.Major, ls); err != nil {
					return err
				}
				// Track this VRF key for subsequent pool registrations in this tx
				inTxVrfKeys[c.VrfKeyHash] = c.Operator
			}

		case *common.RegistrationDrepCertificate:
			inTxDRepRegs[c.DrepCredential.Credential] = true

		// Track deregistrations for in-tx state
		case *common.StakeDeregistrationCertificate:
			inTxStakeState[stakeKey(c.StakeCredential)] = false

		case *common.DeregistrationCertificate:
			inTxStakeState[stakeKey(c.StakeCredential)] = false

		case *common.PoolRetirementCertificate:
			delete(inTxPoolRegs, c.PoolKeyHash)

		case *common.DeregistrationDrepCertificate:
			delete(inTxDRepRegs, c.DrepCredential.Credential)

		// Check delegations
		case *common.StakeDelegationCertificate:
			// Check if pool is registered
			if !isPoolRegistered(c.PoolKeyHash) {
				return DelegateToUnregisteredPoolError{PoolKeyHash: c.PoolKeyHash}
			}
			// Check if stake credential is registered
			if c.StakeCredential != nil && !isStakeRegistered(*c.StakeCredential) {
				return DelegateUnregisteredStakeCredentialError{Credential: *c.StakeCredential}
			}

		case *common.VoteDelegationCertificate:
			// Check if stake credential is registered
			if !isStakeRegistered(c.StakeCredential) {
				return DelegateUnregisteredStakeCredentialError{Credential: c.StakeCredential}
			}
			// Check if target DRep is registered (except for Abstain/NoConfidence)
			drepRegistered, err := isDRepRegistered(c.Drep)
			if err != nil {
				return err
			}
			if !drepRegistered {
				credType, err := drepTypeToCredType(c.Drep.Type)
				if err != nil {
					return err
				}
				return DelegateVoteToUnregisteredDRepError{DRepCredential: common.Credential{
					CredType:   credType,
					Credential: common.NewBlake2b224(c.Drep.Credential),
				}}
			}

		case *common.StakeVoteDelegationCertificate:
			// Check if pool is registered
			if !isPoolRegistered(c.PoolKeyHash) {
				return DelegateToUnregisteredPoolError{PoolKeyHash: c.PoolKeyHash}
			}
			// Check if stake credential is registered
			if !isStakeRegistered(c.StakeCredential) {
				return DelegateUnregisteredStakeCredentialError{Credential: c.StakeCredential}
			}
			// Check if target DRep is registered (except for Abstain/NoConfidence)
			drepRegistered, err := isDRepRegistered(c.Drep)
			if err != nil {
				return err
			}
			if !drepRegistered {
				credType, err := drepTypeToCredType(c.Drep.Type)
				if err != nil {
					return err
				}
				return DelegateVoteToUnregisteredDRepError{DRepCredential: common.Credential{
					CredType:   credType,
					Credential: common.NewBlake2b224(c.Drep.Credential),
				}}
			}

		case *common.StakeRegistrationDelegationCertificate:
			if err := registerStakeCredential(c.StakeCredential); err != nil {
				return err
			}
			// Check if pool is registered
			if !isPoolRegistered(c.PoolKeyHash) {
				return DelegateToUnregisteredPoolError{PoolKeyHash: c.PoolKeyHash}
			}

		case *common.VoteRegistrationDelegationCertificate:
			if err := registerStakeCredential(c.StakeCredential); err != nil {
				return err
			}
			// Check if target DRep is registered (except for Abstain/NoConfidence)
			drepRegistered, err := isDRepRegistered(c.Drep)
			if err != nil {
				return err
			}
			if !drepRegistered {
				credType, err := drepTypeToCredType(c.Drep.Type)
				if err != nil {
					return err
				}
				return DelegateVoteToUnregisteredDRepError{DRepCredential: common.Credential{
					CredType:   credType,
					Credential: common.NewBlake2b224(c.Drep.Credential),
				}}
			}

		case *common.StakeVoteRegistrationDelegationCertificate:
			if err := registerStakeCredential(c.StakeCredential); err != nil {
				return err
			}
			// Check if pool is registered
			if !isPoolRegistered(c.PoolKeyHash) {
				return DelegateToUnregisteredPoolError{PoolKeyHash: c.PoolKeyHash}
			}
			// Check if target DRep is registered (except for Abstain/NoConfidence)
			drepRegistered, err := isDRepRegistered(c.Drep)
			if err != nil {
				return err
			}
			if !drepRegistered {
				credType, err := drepTypeToCredType(c.Drep.Type)
				if err != nil {
					return err
				}
				return DelegateVoteToUnregisteredDRepError{DRepCredential: common.Credential{
					CredType:   credType,
					Credential: common.NewBlake2b224(c.Drep.Credential),
				}}
			}
		}
	}
	return nil
}

// UtxoValidateWithdrawals validates withdrawals against ledger state.
// For phase-2 invalid transactions (IsValid=false), withdrawal validation is
// skipped since their effects are reverted and only collateral rules apply.
// PV10 and PV11 also require each key-hash stake credential present in the
// withdrawal map to have a DRep vote delegation, including zero-amount
// withdrawals. Script-hash stake credentials are exempt. PV12 removes the
// requirement per CIP-181.
func UtxoValidateWithdrawals(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if !tx.IsValid() {
		return nil
	}
	if err := shelley.UtxoValidateWithdrawals(tx, slot, ls, pp); err != nil {
		return err
	}
	withdrawals := tx.Withdrawals()
	if len(withdrawals) == 0 {
		return nil
	}
	versionedPparams, ok := pp.(interface {
		ProtocolMajorVersion() uint
	})
	if !ok {
		return nil
	}
	protocolMajor := versionedPparams.ProtocolMajorVersion()
	if protocolMajor < common.ProtocolVersionPlomin ||
		protocolMajor >= common.ProtocolVersionDijkstra {
		return nil
	}
	var delegationState common.DRepDelegationState
	for addr := range withdrawals {
		credential, ok := addr.StakeCredential()
		if !ok || credential.CredType != common.CredentialTypeAddrKeyHash {
			continue
		}
		if delegationState == nil {
			var ok bool
			delegationState, ok = ls.(common.DRepDelegationState)
			if !ok {
				return DRepDelegationStateUnavailableError{}
			}
		}
		delegation, err := delegationState.DRepDelegation(credential)
		if err != nil {
			return err
		}
		if delegation == nil {
			return WithdrawalNotDelegatedToDRepError{
				RewardAddress: *addr,
			}
		}
	}
	return nil
}

func UtxoValidateCommitteeCertificates(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	members, err := ls.CommitteeMembers()
	hasCommitteeState := err == nil && len(members) > 0

	for _, cert := range tx.Certificates() {
		switch c := cert.(type) {
		case *common.AuthCommitteeHotCertificate:
			coldHash := c.ColdCredential.Credential
			member, err := ls.CommitteeMember(coldHash)
			if err != nil {
				return CommitteeMemberLookupError{Credential: coldHash, Err: err}
			}
			if member == nil && hasCommitteeState {
				return NotCommitteeMemberError{Credential: coldHash, Operation: "authorize hot key"}
			}
			if member != nil && member.Resigned {
				return ResignedCommitteeMemberHotKeyError{ColdKey: coldHash}
			}

		case *common.ResignCommitteeColdCertificate:
			coldHash := c.ColdCredential.Credential
			member, err := ls.CommitteeMember(coldHash)
			if err != nil {
				return CommitteeMemberLookupError{Credential: coldHash, Err: err}
			}
			if member == nil && hasCommitteeState {
				return NotCommitteeMemberError{Credential: coldHash, Operation: "resign"}
			}
		}
	}
	return nil
}

// PoolValidateVrfKeyUniqueness ensures no two pools use the same VRF key.
// Enforced only for Protocol Version 11+.
func PoolValidateVrfKeyUniqueness(
	cert *common.PoolRegistrationCertificate,
	protocolMajor uint,
	ls common.LedgerState,
) error {
	if !common.IsProtocolVersionAtLeast(
		protocolMajor,
		0,
		common.ProtocolVersionVanRossem,
	) {
		return nil
	}
	inUse, existingPoolId, err := ls.IsVrfKeyInUse(cert.VrfKeyHash)
	if err != nil {
		return err
	}
	if !inUse || existingPoolId == cert.Operator {
		return nil
	}
	return DuplicateVrfKeyError{
		VrfKeyHash:     cert.VrfKeyHash,
		NewPoolId:      cert.Operator,
		ExistingPoolId: existingPoolId,
	}
}

// UtxoValidateUnknownGovActionIds rejects voting procedures that reference a
// governance action id that neither the ledger state records nor the
// transaction under validation proposes
// (ConwayGovPredFailure.GovActionsDoNotExist). This is the rule referenced
// by the "unknown action ID is handled by other validation rules" comment
// in UtxoValidateCCVotingRestrictions.
//
// An action proposed by the transaction under validation counts as existing:
// cardano-ledger folds the transaction's proposals into the proposal set
// before checking its votes, so a transaction that proposes an action and
// votes on it is not voting on an unknown action. Rejecting it here would
// also make the same-transaction resolution in
// UtxoValidateStakePoolVotingRestrictions unreachable, since this rule runs
// first in UtxoValidationRules.
func UtxoValidateUnknownGovActionIds(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	resolver := govActionResolver{tx: tx, ls: ls}
	var unknown []common.GovActionId
	for _, actionVotes := range tx.VotingProcedures() {
		for actionId := range actionVotes {
			if actionId == nil {
				// A nil action id cannot be looked up in the ledger
				// state and is therefore treated the same as a
				// reference to a nonexistent action, matching
				// UtxoValidateCCVotingRestrictions's convention of
				// erroring rather than silently skipping.
				unknown = append(unknown, common.GovActionId{})
				continue
			}
			if !resolver.exists(*actionId) {
				unknown = append(unknown, *actionId)
			}
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	// Map iteration order is non-deterministic; sort before constructing
	// the error so the message is reproducible across runs.
	slices.SortFunc(unknown, func(a, b common.GovActionId) int {
		if c := bytes.Compare(a.TransactionId[:], b.TransactionId[:]); c != 0 {
			return c
		}
		switch {
		case a.GovActionIdx < b.GovActionIdx:
			return -1
		case a.GovActionIdx > b.GovActionIdx:
			return 1
		default:
			return 0
		}
	})
	return UnknownGovActionIdError{ActionIds: unknown}
}

// UtxoValidateUnknownVoters rejects votes cast by a voter that does not
// exist in the ledger state: an unregistered DRep, an unregistered stake
// pool, or a credential that is not currently authorized as a committee hot
// key (ConwayGovPredFailure.VotersDoNotExist).
func UtxoValidateUnknownVoters(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	votes := tx.VotingProcedures()
	if len(votes) == 0 {
		return nil
	}

	var members []common.CommitteeMember
	var membersLoaded bool

	for voter := range votes {
		if voter == nil {
			continue
		}
		switch voter.Type {
		case common.VoterTypeDRepKeyHash, common.VoterTypeDRepScriptHash:
			credHash := common.Blake2b224(voter.Hash)
			reg, err := ls.DRepRegistration(credHash)
			if err != nil {
				return err
			}
			if reg == nil {
				return UnknownVoterError{Voter: *voter}
			}

		case common.VoterTypeStakingPoolKeyHash:
			if !ls.IsPoolRegistered(common.PoolKeyHash(voter.Hash)) {
				return UnknownVoterError{Voter: *voter}
			}

		case common.VoterTypeConstitutionalCommitteeHotKeyHash,
			common.VoterTypeConstitutionalCommitteeHotScriptHash:
			if !membersLoaded {
				var err error
				members, err = ls.CommitteeMembers()
				if err != nil {
					return err
				}
				membersLoaded = true
			}
			// If the ledger state reports no committee members at all,
			// treat committee state as unmodeled (consistent with
			// UtxoValidateCommitteeCertificates) rather than rejecting
			// every CC vote as unknown.
			if len(members) == 0 {
				continue
			}
			hotHash := common.Blake2b224(voter.Hash)
			found := false
			for _, member := range members {
				// NOTE: this does not check member.ExpiryEpoch. There is
				// no current-epoch accessor on LedgerState to compare it
				// against, so an expired-but-not-yet-resigned committee
				// member's hot key is treated as a valid voter here. This
				// is the same class of LedgerState-surface limitation
				// disclosed via NOTE comments elsewhere in this file
				// (e.g. UtxoValidateProposalAncestry, UtxoValidateHardForkCanFollow).
				if member.HotKey != nil && *member.HotKey == hotHash &&
					!member.Resigned {
					found = true
					break
				}
			}
			if !found {
				return UnknownVoterError{Voter: *voter}
			}

		default:
			// Voter.Type is decoded from CBOR with no range check, so
			// values outside the five defined VoterType* constants are
			// possible on the wire. Reject them here rather than silently
			// falling through unvalidated, since no other rule in
			// UtxoValidationRules checks voter type validity.
			return UnknownVoterError{Voter: *voter}
		}
	}
	return nil
}

// UtxoValidateVotingOnExpiredGovAction rejects votes cast on a governance
// action whose expiry slot has already passed
// (ConwayGovPredFailure.VotingOnExpiredGovAction). A nil action id is
// rejected here directly (rather than silently skipped) so this rule does
// not depend on running after UtxoValidateUnknownGovActionIds in the
// pipeline for correctness, matching UtxoValidateCCVotingRestrictions's
// convention of erroring on a nil action id.
func UtxoValidateVotingOnExpiredGovAction(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	for voter, actionVotes := range tx.VotingProcedures() {
		if voter == nil {
			continue
		}
		for actionId := range actionVotes {
			if actionId == nil {
				return UnknownGovActionIdError{
					ActionIds: []common.GovActionId{{}},
				}
			}
			actionState, err := ls.GovActionById(*actionId)
			if err != nil || actionState == nil {
				continue
			}
			// ExpirySlot is optional in the LedgerState contract: a
			// state provider that does not model gov-action expiry
			// leaves it zero. Treat that as "expiry not modeled"
			// rather than "expired at slot 0", which would reject
			// every vote at any slot > 0.
			if actionState.ExpirySlot == 0 {
				continue
			}
			if slot > actionState.ExpirySlot {
				return VotingOnExpiredGovActionError{
					Voter:      *voter,
					ActionId:   *actionId,
					ExpirySlot: actionState.ExpirySlot,
					Slot:       slot,
				}
			}
		}
	}
	return nil
}

// UtxoValidateBootstrapVotingRestrictions enforces the Conway bootstrap-phase
// (PV9) voting restrictions (ConwayGovPredFailure.DisallowedVotesDuringBootstrap):
// DReps may only vote on InfoAction, and all other voter types may only vote
// on bootstrap-eligible actions (ParameterChange, HardForkInitiation,
// InfoAction). A nil action id is rejected directly here (matching
// UtxoValidateCCVotingRestrictions's convention) rather than silently
// skipped.
//
// The action type is resolved from the transaction's own proposals when the
// vote names an action that transaction proposes, so a same-transaction
// propose-and-vote does not escape the restriction.
func UtxoValidateBootstrapVotingRestrictions(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if !isInConwayBootstrapPhase(pp) {
		return nil
	}
	isBootstrapAction := func(actionType common.GovActionType) bool {
		switch actionType {
		case common.GovActionTypeParameterChange,
			common.GovActionTypeHardForkInitiation,
			common.GovActionTypeInfo:
			return true
		case common.GovActionTypeTreasuryWithdrawal,
			common.GovActionTypeNoConfidence,
			common.GovActionTypeUpdateCommittee,
			common.GovActionTypeNewConstitution:
			return false
		default:
			return false
		}
	}
	resolver := govActionResolver{tx: tx, ls: ls}
	for voter, actionVotes := range tx.VotingProcedures() {
		if voter == nil {
			continue
		}
		for actionId := range actionVotes {
			if actionId == nil {
				return BootstrapVotingRestrictionError{
					VoterId:     voter.Hash,
					ActionId:    common.GovActionId{},
					Restriction: "nil action ID in voting procedures",
				}
			}
			actionType, _, ok := resolver.resolve(*actionId)
			if !ok {
				continue
			}
			var allowed bool
			switch voter.Type {
			case common.VoterTypeDRepKeyHash, common.VoterTypeDRepScriptHash:
				allowed = actionType == common.GovActionTypeInfo
			default:
				allowed = isBootstrapAction(actionType)
			}
			if !allowed {
				return BootstrapVotingRestrictionError{
					VoterId:  voter.Hash,
					ActionId: *actionId,
					Restriction: fmt.Sprintf(
						"voter type %d cannot vote on action type %d during bootstrap phase",
						voter.Type,
						actionType,
					),
				}
			}
		}
	}
	return nil
}

// UtxoValidateStakePoolVotingRestrictions validates stake pool (SPO) voting
// restrictions per isStakePoolVotingAllowed and
// votingStakePoolThresholdInternal in cardano-ledger
// (eras/conway/impl/src/Cardano/Ledger/Conway/Governance/Internal.hs lines
// 353-405 at commit 08773e9a8f911f67209560a4e401369cbb21a0cb): SPOs may
// never vote on NewConstitution or TreasuryWithdrawal actions, and may vote
// on a ParameterChange only when at least one of the parameters it modifies
// belongs to the security group (see
// ConwayProtocolParameterUpdate.SecurityGroupFields). A nil action id is
// rejected directly here (matching UtxoValidateCCVotingRestrictions's
// convention) rather than silently skipped.
//
// Classifying a ParameterChange requires the proposed parameter update. It
// is read from the transaction itself when the action is proposed by the
// same transaction, otherwise from common.GovActionState.Action. A ledger
// state that does not record the action contents leaves the parameter-group
// restriction unenforced rather than guessed at.
func UtxoValidateStakePoolVotingRestrictions(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	votes := tx.VotingProcedures()
	if len(votes) == 0 {
		return nil
	}
	resolver := govActionResolver{tx: tx, ls: ls}
	for voter, actionVotes := range votes {
		if voter == nil || voter.Type != common.VoterTypeStakingPoolKeyHash {
			continue
		}
		for actionId := range actionVotes {
			if actionId == nil {
				return StakePoolVotingRestrictionError{
					VoterId:     common.PoolKeyHash(voter.Hash),
					ActionId:    common.GovActionId{},
					Restriction: "nil action ID in voting procedures",
				}
			}
			actionType, action, ok := resolver.resolve(*actionId)
			if !ok {
				continue
			}
			var restriction string
			switch actionType {
			case common.GovActionTypeNewConstitution:
				restriction = "stake pools cannot vote on NewConstitution"
			case common.GovActionTypeTreasuryWithdrawal:
				restriction = "stake pools cannot vote on TreasuryWithdrawal"
			case common.GovActionTypeParameterChange:
				paramChange, ok := action.(common.ParameterChangeGovAction)
				if !ok {
					// The action contents are not available, so the
					// parameter groups it modifies are unknown.
					continue
				}
				if len(paramChange.SecurityGroupFields()) > 0 {
					continue
				}
				restriction = "stake pools cannot vote on a parameter change " +
					"that does not modify security group parameters"
			case common.GovActionTypeHardForkInitiation,
				common.GovActionTypeNoConfidence,
				common.GovActionTypeUpdateCommittee,
				common.GovActionTypeInfo:
				continue
			default:
				continue
			}
			return StakePoolVotingRestrictionError{
				VoterId:     common.PoolKeyHash(voter.Hash),
				ActionId:    *actionId,
				Restriction: restriction,
			}
		}
	}
	return nil
}

// UtxoValidateCCVotingRestrictions validates CC voting restrictions per cardano-ledger spec.
// Constitutional Committee members cannot vote on NoConfidence or UpdateCommittee actions.
// Enforced at ledger level for PV11+ (ProtocolVersionVanRossem).
//
// The action type is resolved from the transaction's own proposals when the
// vote names an action that transaction proposes, so a same-transaction
// propose-and-vote does not escape the restriction.
func UtxoValidateCCVotingRestrictions(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	conwayPp, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	if !common.IsProtocolVersionAtLeast(
		conwayPp.ProtocolVersion.Major, 0, common.ProtocolVersionVanRossem,
	) {
		return nil // Pre-PV11: checked by mempool sanitizer only
	}

	votes := tx.VotingProcedures()
	if len(votes) == 0 {
		return nil
	}

	resolver := govActionResolver{tx: tx, ls: ls}
	for voter, actionVotes := range votes {
		// Only check CC voters
		if voter.Type != common.VoterTypeConstitutionalCommitteeHotKeyHash &&
			voter.Type != common.VoterTypeConstitutionalCommitteeHotScriptHash {
			continue
		}

		for actionId := range actionVotes {
			// Guard against nil action ID (malformed transaction)
			if actionId == nil {
				return CCVotingRestrictionError{
					VoterId:     voter.Hash,
					ActionId:    common.GovActionId{}, // zero value for nil action ID
					Restriction: "nil action ID in voting procedures",
				}
			}

			// Resolve the action type from the transaction's own proposals
			// or, failing that, from governance state. An action neither
			// names is reported by UtxoValidateUnknownGovActionIds.
			actionType, _, ok := resolver.resolve(*actionId)
			if !ok {
				continue
			}

			// CC members cannot vote on NoConfidence or UpdateCommittee
			if actionType == common.GovActionTypeNoConfidence ||
				actionType == common.GovActionTypeUpdateCommittee {
				restriction := "CC cannot vote on NoConfidence"
				if actionType == common.GovActionTypeUpdateCommittee {
					restriction = "CC cannot vote on UpdateCommittee"
				}
				return CCVotingRestrictionError{
					VoterId:     voter.Hash,
					ActionId:    *actionId,
					Restriction: restriction,
				}
			}
		}
	}

	return nil
}

// UtxoValidateMalformedReferenceScripts checks that any reference scripts in
// transaction outputs are well-formed and can be deserialized.
func UtxoValidateMalformedReferenceScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	var malformedHashes []common.ScriptHash

	for _, output := range tx.Outputs() {
		scriptRef := output.ScriptRef()
		if scriptRef == nil {
			continue
		}

		// Check if the script can be decoded properly
		var innerScript []byte
		if _, ok := common.PlutusScriptVersion(scriptRef); !ok {
			// Native scripts don't need UPLC validation
			continue
		}

		// Decode the outer CBOR wrapper to get the actual script bytes.
		if _, err := cbor.Decode(scriptRef.RawScriptBytes(), &innerScript); err != nil {
			malformedHashes = append(malformedHashes, scriptRef.Hash())
			continue
		}
		// Try to decode as UPLC program.
		if _, err := syn.Decode[syn.DeBruijn](innerScript); err != nil {
			malformedHashes = append(malformedHashes, scriptRef.Hash())
		}
	}

	if len(malformedHashes) > 0 {
		return common.MalformedReferenceScriptsError{
			ScriptHashes: malformedHashes,
		}
	}
	return nil
}
