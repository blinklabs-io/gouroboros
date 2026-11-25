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

package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// govSeqKind is one governance action a transaction in the sequence takes.
type govSeqKind struct {
	op   string
	cold int // index into govSeqColds for "auth" and "resign"
}

// govSeqState is the reference's governance state after a prefix of the
// block, applied transaction by transaction as LEDGERS does (cardano-ledger
// eras/conway/impl/src/Cardano/Ledger/Conway/Rules/Ledgers.hs).
type govSeqState struct {
	drep     bool
	deleg    bool
	proposed bool // an UpdateCommittee proposal names cold credential 1
	props    bool
	hot      [2]bool
	resigned [2]bool
}

func (s govSeqState) member(cold int) bool {
	return cold == 0 || (cold == 1 && s.proposed)
}

// pre is the precondition each action's reference rule checks against the
// state the block's earlier transactions left: GOVCERT for DRep and
// committee certificates (Conway/Rules/GovCert.hs 190-270), DELEG for a vote
// delegation from protocol version 10 (Conway/Rules/Deleg.hs), GOV for
// votes (Conway/Rules/Gov.hs).
func (s govSeqState) pre(k govSeqKind) bool {
	switch k.op {
	case "regDRep":
		return !s.drep
	case "unregDRep", "delegVote":
		return s.drep
	case "voteDRep":
		return s.props && s.drep
	case "voteCC":
		for i := range s.hot {
			if s.hot[i] && !s.resigned[i] {
				return s.props
			}
		}
		return false
	case "auth", "resign":
		return s.member(k.cold) && !s.resigned[k.cold]
	}
	return true
}

func (s govSeqState) apply(k govSeqKind) govSeqState {
	switch k.op {
	case "regDRep":
		s.drep = true
	case "unregDRep":
		// GOVCERT ConwayUnRegDRep clears every delegation to the DRep.
		s.drep, s.deleg = false, false
	case "delegVote":
		s.deleg = true
	case "proposeUC":
		s.proposed, s.props = true, true
	case "proposeInfo":
		s.props = true
	case "auth":
		s.hot[k.cold] = true
	case "resign":
		s.hot[k.cold], s.resigned[k.cold] = false, true
	}
	return s
}

var (
	govSeqDRep  = blockTestCredential(0x81)
	govSeqStake = blockTestCredential(0x82)
	govSeqHot   = blockTestCredential(0x83)
	govSeqColds = [2]common.Credential{
		blockTestCredential(0x84), // seated at the start, hot key authorized
		blockTestCredential(0x85), // named only by an in-block proposal
	}
)

func govSeqKinds() []govSeqKind {
	kinds := []govSeqKind{
		{op: "regDRep"}, {op: "unregDRep"}, {op: "delegVote"},
		{op: "proposeUC"}, {op: "proposeInfo"}, {op: "voteCC"},
		{op: "voteDRep"},
	}
	for i := range govSeqColds {
		kinds = append(kinds,
			govSeqKind{op: "auth", cold: i},
			govSeqKind{op: "resign", cold: i})
	}
	return kinds
}

func govSeqTx(
	t *testing.T,
	id byte,
	k govSeqKind,
	valid bool,
) *mockledger.MockTransaction {
	tx := blockTestTx(t, id, valid)
	switch k.op {
	case "regDRep":
		tx.WithCertificates(&common.RegistrationDrepCertificate{
			DrepCredential: govSeqDRep, Amount: 500,
		})
	case "unregDRep":
		tx.WithCertificates(&common.DeregistrationDrepCertificate{
			DrepCredential: govSeqDRep, Amount: 500,
		})
	case "delegVote":
		tx.WithCertificates(&common.VoteDelegationCertificate{
			StakeCredential: govSeqStake,
			Drep: common.Drep{
				Type:       common.DrepTypeAddrKeyHash,
				Credential: govSeqDRep.Credential[:],
			},
		})
	case "proposeUC":
		cold := govSeqColds[1]
		tx.WithProposalProcedures(conway.ConwayProposalProcedure{
			PPGovAction: conway.ConwayGovAction{
				Action: &common.UpdateCommitteeGovAction{
					CredEpochs: map[*common.Credential]uint64{&cold: 90},
				},
			},
		})
	case "proposeInfo":
		tx.WithProposalProcedures(conway.ConwayProposalProcedure{
			PPGovAction: conway.ConwayGovAction{Action: &common.InfoGovAction{}},
		})
	case "auth":
		tx.WithCertificates(&common.AuthCommitteeHotCertificate{
			ColdCredential: govSeqColds[k.cold], HotCredential: govSeqHot,
		})
	case "resign":
		tx.WithCertificates(&common.ResignCommitteeColdCertificate{
			ColdCredential: govSeqColds[k.cold],
		})
	}
	// Votes change nothing a later transaction in the block can see;
	// their rules are checked against the view below.
	return tx
}

// govSeqView reads from the overlay every answer the reference state holds.
func govSeqView(
	t *testing.T,
	state *common.BlockLedgerState,
	proposals []common.GovActionId,
) govSeqState {
	t.Helper()
	var view govSeqState
	reg, err := state.DRepRegistration(govSeqDRep)
	require.NoError(t, err)
	view.drep = reg != nil
	delegations, ok := common.DRepDelegationStateFor(state)
	require.True(t, ok)
	drep, err := delegations.DRepDelegation(govSeqStake)
	require.NoError(t, err)
	view.deleg = drep != nil
	for _, id := range proposals {
		view.props = view.props || state.GovActionExists(id)
	}
	committee, ok := common.CommitteeCredentialStateFor(state)
	require.True(t, ok)
	for i, cold := range govSeqColds {
		member, err := committee.CommitteeCredentialMember(cold)
		require.NoError(t, err)
		if member == nil {
			continue
		}
		if i == 1 {
			view.proposed = true
		}
		view.hot[i] = member.HotKey != nil &&
			*member.HotKey == govSeqHot.Credential
		view.resigned[i] = member.Resigned
	}
	return view
}

// TestBlockLedgerStateGovernanceSequences walks every block of up to three
// transactions drawn from the governance actions above, each valid or
// phase-2-invalid, through BlockLedgerState, and compares its answers after
// every transaction with the reference's sequential application. A block
// stops at the first transaction the reference rejects, as BBODY rejects
// the block there. Committee certificates are judged by the real rule,
// UtxoValidateCommitteeCertificates, against the overlay.
func TestBlockLedgerStateGovernanceSequences(t *testing.T) {
	kinds := govSeqKinds()
	type step struct {
		kind  govSeqKind
		valid bool
	}
	steps := make([]step, 0, 2*len(kinds))
	for _, k := range kinds {
		steps = append(steps, step{k, true}, step{k, false})
	}
	pp := &conway.ConwayProtocolParameters{}
	var walk func(prefix []step)
	cells := 0
	walk = func(prefix []step) {
		cells++
		base := mockledger.NewLedgerStateBuilder().
			WithCommitteeMembers([]common.CommitteeMember{{
				ColdKey:     govSeqColds[0].Credential,
				HotKey:      &govSeqHot.Credential,
				ExpiryEpoch: 90,
			}}).
			Build()
		state := common.NewBlockLedgerState(base)
		ref := govSeqState{hot: [2]bool{true, false}}
		var proposals []common.GovActionId
		for i, s := range prefix {
			tx := govSeqTx(t, byte(0xc0+i), s.kind, s.valid)
			refOk := !s.valid || ref.pre(s.kind)
			goView := govSeqView(t, state, proposals)
			require.Equal(t, ref, goView, "view before tx %d of %v", i, prefix)
			goOk := !s.valid || goView.pre(s.kind)
			if s.kind.op == "auth" || s.kind.op == "resign" {
				goOk = conway.UtxoValidateCommitteeCertificates(
					tx, 0, state, pp,
				) == nil
			}
			require.Equal(t, refOk, goOk, "verdict on tx %d of %v", i, prefix)
			if !refOk {
				return
			}
			require.NoError(t, state.ApplyTransaction(tx, pp))
			if s.valid {
				ref = ref.apply(s.kind)
				if s.kind.op == "proposeUC" || s.kind.op == "proposeInfo" {
					proposals = append(proposals, common.GovActionId{
						TransactionId: tx.Hash(),
					})
				}
			}
			require.Equal(t, ref, govSeqView(t, state, proposals),
				"view after tx %d of %v", i, prefix)
		}
		if len(prefix) == 3 {
			return
		}
		for _, s := range steps {
			walk(append(append([]step(nil), prefix...), s))
		}
	}
	walk(nil)
	t.Logf("%d blocks walked", cells)
}
