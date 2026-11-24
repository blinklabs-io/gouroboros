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

package conway_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// The tables in this file restate the reference GOV rules as data and check
// every cell, rather than a sample of cells, against the gOuroboros rules
// that implement them. The expected values come from cardano-ledger
// eras/conway/impl/src/Cardano/Ledger/Conway, not from the code under test.

// govModelAction is one row of the voting table: an action kind as the
// voting rules distinguish it. ParameterChange is split in two because stake
// pools may vote only on a change that touches the security group.
type govModelAction struct {
	name   string
	typ    common.GovActionType
	action common.GovAction
}

func govModelActions() []govModelAction {
	maxBlockBodySize := uint(90112)
	keyDeposit := uint(2_000_000)
	return []govModelAction{
		{"NoConfidence", common.GovActionTypeNoConfidence, &common.NoConfidenceGovAction{}},
		{"UpdateCommittee", common.GovActionTypeUpdateCommittee, &common.UpdateCommitteeGovAction{Quorum: modelQuorum()}},
		{"NewConstitution", common.GovActionTypeNewConstitution, &common.NewConstitutionGovAction{}},
		{"HardForkInitiation", common.GovActionTypeHardForkInitiation, &common.HardForkInitiationGovAction{}},
		{"ParameterChangeSecurity", common.GovActionTypeParameterChange, &conway.ConwayParameterChangeGovAction{
			ParamUpdate: conway.ConwayProtocolParameterUpdate{
				MaxBlockBodySize: &maxBlockBodySize,
			},
		}},
		{"ParameterChangeOther", common.GovActionTypeParameterChange, &conway.ConwayParameterChangeGovAction{
			ParamUpdate: conway.ConwayProtocolParameterUpdate{
				KeyDeposit: &keyDeposit,
			},
		}},
		{"TreasuryWithdrawals", common.GovActionTypeTreasuryWithdrawal, &common.TreasuryWithdrawalGovAction{}},
		{"InfoAction", common.GovActionTypeInfo, &common.InfoGovAction{}},
	}
}

// govModelVoteAllowed is the reference verdict for one vote.
//
// Outside bootstrap it is isVotingAllowed of the voter's threshold
// (Governance/Internal.hs 350-545): committees may not vote on NoConfidence
// or UpdateCommittee; DReps may vote on everything; stake pools may not vote
// on NewConstitution or TreasuryWithdrawals, nor on a ParameterChange that
// leaves the security group alone. During bootstrap checkBootstrapVotes
// (Rules/Gov.hs 378-391) also applies: DReps may vote only on InfoAction and
// the others only on ParameterChange, HardForkInitiation and InfoAction.
// Both checks run, so a vote passes only when both allow it.
func govModelVoteAllowed(
	voterType uint8,
	action string,
	bootstrap bool,
) bool {
	var allowed bool
	switch voterType {
	case common.VoterTypeConstitutionalCommitteeHotKeyHash,
		common.VoterTypeConstitutionalCommitteeHotScriptHash:
		allowed = action != "NoConfidence" && action != "UpdateCommittee"
	case common.VoterTypeDRepKeyHash, common.VoterTypeDRepScriptHash:
		allowed = true
	case common.VoterTypeStakingPoolKeyHash:
		allowed = action != "NewConstitution" &&
			action != "TreasuryWithdrawals" &&
			action != "ParameterChangeOther"
	}
	if !bootstrap {
		return allowed
	}
	switch voterType {
	case common.VoterTypeDRepKeyHash, common.VoterTypeDRepScriptHash:
		return allowed && action == "InfoAction"
	default:
		bootstrapAction := action == "ParameterChangeSecurity" ||
			action == "ParameterChangeOther" ||
			action == "HardForkInitiation" ||
			action == "InfoAction"
		return allowed && bootstrapAction
	}
}

// TestGovVoterActionTableModel checks every voter type against every action
// kind in the bootstrap phase (PV9), after it (PV10), and from PV11, through
// the three rules that together implement DisallowedVoters and
// DisallowedVotesDuringBootstrap.
func TestGovVoterActionTableModel(t *testing.T) {
	voterTypes := []struct {
		name string
		typ  uint8
	}{
		{"CCKey", common.VoterTypeConstitutionalCommitteeHotKeyHash},
		{"CCScript", common.VoterTypeConstitutionalCommitteeHotScriptHash},
		{"DRepKey", common.VoterTypeDRepKeyHash},
		{"DRepScript", common.VoterTypeDRepScriptHash},
		{"SPO", common.VoterTypeStakingPoolKeyHash},
	}
	phases := []struct {
		name      string
		major     uint
		bootstrap bool
	}{
		{"PV9", common.ProtocolVersionConway, true},
		{"PV10", common.ProtocolVersionPlomin, false},
		{"PV11", common.ProtocolVersionVanRossem, false},
	}

	actions := govModelActions()
	govActions := make(map[string]*common.GovActionState, len(actions))
	ids := make(map[string]common.GovActionId, len(actions))
	for i, a := range actions {
		id := common.GovActionId{
			TransactionId: common.Blake2b256{0x40, byte(i)},
		}
		ids[a.name] = id
		govActions[govActionKey(id)] = &common.GovActionState{
			ActionId:   id,
			ActionType: a.typ,
			Action:     a.action,
		}
	}
	ls := mockledger.NewLedgerStateBuilder().WithGovActions(govActions).Build()

	cells := 0
	for _, phase := range phases {
		pp := mkConwayPp(phase.major, 0)
		for _, voter := range voterTypes {
			for _, a := range actions {
				want := govModelVoteAllowed(voter.typ, a.name, phase.bootstrap)
				name := fmt.Sprintf("%s/%s/%s", phase.name, voter.name, a.name)
				t.Run(name, func(t *testing.T) {
					tx := mkVoteTx(
						common.Voter{Type: voter.typ, Hash: common.Blake2b224{0x01}},
						ids[a.name],
						common.GovVoteYes,
					)
					var errs []error
					for _, rule := range []common.UtxoValidationRuleFunc{
						conway.UtxoValidateBootstrapVotingRestrictions,
						conway.UtxoValidateStakePoolVotingRestrictions,
						conway.UtxoValidateCCVotingRestrictions,
					} {
						if err := rule(tx, 0, ls, pp); err != nil {
							errs = append(errs, err)
						}
					}
					if want {
						require.Empty(t, errs, "reference allows this vote")
					} else {
						require.NotEmpty(t, errs, "reference refuses this vote")
					}
				})
				cells++
			}
		}
	}
	require.Equal(t, len(phases)*len(voterTypes)*len(actions), cells)
}

// TestGovProposalAncestryTableModel checks every action kind that names a
// predecessor against every kind of predecessor, with and without a purpose
// root, through UtxoValidateProposalAncestry.
//
// The reference accepts a proposal when runProposalsAddAction can attach it
// (Governance/Proposals.hs 305-334): the predecessor equals the purpose's
// root, both possibly absent, or is a live proposal in that purpose's tree.
// NoConfidence and UpdateCommittee share the committee purpose, so either
// may follow the other.
func TestGovProposalAncestryTableModel(t *testing.T) {
	type purposeKind int
	const (
		pparams purposeKind = iota
		hardFork
		committee
		constitution
	)
	kinds := []struct {
		name    string
		purpose purposeKind
		typ     common.GovActionType
		mk      func(prev *common.GovActionId) common.GovAction
	}{
		{"ParameterChange", pparams, common.GovActionTypeParameterChange,
			func(p *common.GovActionId) common.GovAction {
				return &conway.ConwayParameterChangeGovAction{ActionId: p}
			}},
		{"HardForkInitiation", hardFork, common.GovActionTypeHardForkInitiation,
			func(p *common.GovActionId) common.GovAction {
				return &common.HardForkInitiationGovAction{ActionId: p}
			}},
		{"NoConfidence", committee, common.GovActionTypeNoConfidence,
			func(p *common.GovActionId) common.GovAction {
				return &common.NoConfidenceGovAction{ActionId: p}
			}},
		{"UpdateCommittee", committee, common.GovActionTypeUpdateCommittee,
			func(p *common.GovActionId) common.GovAction {
				return &common.UpdateCommitteeGovAction{
					ActionId: p,
					Quorum:   modelQuorum(),
				}
			}},
		{"NewConstitution", constitution, common.GovActionTypeNewConstitution,
			func(p *common.GovActionId) common.GovAction {
				return &common.NewConstitutionGovAction{ActionId: p}
			}},
	}

	// One root and one live proposal per kind. A root is enacted, so the
	// ledger state does not list it among the proposals.
	rootIds := map[purposeKind]*common.GovActionId{}
	liveIds := map[string]common.GovActionId{}
	govActions := map[string]*common.GovActionState{}
	for i, k := range kinds {
		live := common.GovActionId{TransactionId: common.Blake2b256{0x50, byte(i)}}
		liveIds[k.name] = live
		govActions[govActionKey(live)] = &common.GovActionState{
			ActionId:   live,
			ActionType: k.typ,
		}
		if _, ok := rootIds[k.purpose]; !ok {
			root := common.GovActionId{
				TransactionId: common.Blake2b256{0x60, byte(k.purpose)},
			}
			rootIds[k.purpose] = &root
		}
	}
	base := mockledger.NewLedgerStateBuilder().WithGovActions(govActions).Build()
	withRoots := rootedLedgerState{
		LedgerState: base,
		roots: &common.GovPurposeRoots{
			PParamUpdate: rootIds[pparams],
			HardFork:     rootIds[hardFork],
			Committee:    rootIds[committee],
			Constitution: rootIds[constitution],
		},
	}
	noRoots := rootedLedgerState{LedgerState: base, roots: &common.GovPurposeRoots{}}
	unknown := common.GovActionId{TransactionId: common.Blake2b256{0x7f}}
	pp := mkConwayPp(common.ProtocolVersionPlomin, 0)

	cells := 0
	for _, rooted := range []bool{false, true} {
		ls := noRoots
		if rooted {
			ls = withRoots
		}
		for _, k := range kinds {
			type prevCase struct {
				name string
				prev *common.GovActionId
				want bool
			}
			cases := []prevCase{
				// The absent predecessor equals the absent root only.
				{"none", nil, !rooted},
				// The root is not a live proposal, so it is a known
				// predecessor only while it is the root.
				{"root", rootIds[k.purpose], rooted},
				{"unknown", &unknown, false},
			}
			for _, other := range kinds {
				id := liveIds[other.name]
				cases = append(cases, prevCase{
					name: "live" + other.name,
					prev: &id,
					want: other.purpose == k.purpose,
				})
			}
			for _, c := range cases {
				name := fmt.Sprintf("rooted=%t/%s/%s", rooted, k.name, c.name)
				t.Run(name, func(t *testing.T) {
					tx := mkProposalTx(0, common.Address{}, k.mk(c.prev))
					err := conway.UtxoValidateProposalAncestry(tx, 0, ls, pp)
					if c.want {
						require.NoError(t, err)
					} else {
						var ancErr conway.InvalidGovActionAncestorError
						require.ErrorAs(t, err, &ancErr)
					}
				})
				cells++
			}
		}
	}
	require.Equal(t, 2*len(kinds)*(3+len(kinds)), cells)
}

// modelQuorum is a committee quorum for the UpdateCommittee proposals. The
// zero cbor.Rat cannot be encoded, and a transaction body without stored
// CBOR is hashed from its encoding.
func modelQuorum() cbor.Rat {
	return cbor.Rat{Rat: big.NewRat(2, 3)}
}
