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

package conformance

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/ouroboros-mock/conformance"
)

// rulesOnlyOutcome is the result of replaying one vector with gOuroboros as
// the only validator.
type rulesOnlyOutcome struct {
	path, title string
	family      string
	// mismatch is empty when every transaction event matched the
	// vector's expected outcome.
	mismatch string
	// rejected counts the expected-failure transactions gOuroboros rejected.
	rejected int
}

// rulesOnlyStateManager adds the governance views the full Conway rule list
// reads (purpose roots and the constitution's guardrails script) on top of
// the mock state manager. Both are read from the mock's governance state, so
// they follow the vector's enactments.
type rulesOnlyStateManager struct {
	epochAwareStateManager
	// actions holds each applied proposal's governance action by id: the
	// mock's governance-action view carries only the action type.
	actions map[common.GovActionId]common.GovAction
}

func (m rulesOnlyStateManager) GetStateProvider() conformance.StateProvider {
	return rulesOnlyStateProvider{
		epochAwareStateProvider: m.epochAwareStateManager.GetStateProvider().(epochAwareStateProvider),
		actions:                 m.actions,
	}
}

func (m rulesOnlyStateManager) ApplyTransaction(tx common.Transaction, slot uint64) error {
	if err := m.epochAwareStateManager.ApplyTransaction(tx, slot); err != nil {
		return err
	}
	if !tx.IsValid() {
		return nil
	}
	txId := tx.Hash()
	for i, proposal := range tx.ProposalProcedures() {
		if proposal == nil {
			continue
		}
		id, err := common.NewGovActionId(txId.Bytes(), uint32(i)) // #nosec G115 -- proposal count is bounded by tx size
		if err != nil {
			return err
		}
		m.actions[id] = proposal.GovAction()
	}
	return nil
}

type rulesOnlyStateProvider struct {
	epochAwareStateProvider
	actions map[common.GovActionId]common.GovAction
}

func (s rulesOnlyStateProvider) StakeCredentialDeposit(
	credential common.Credential,
) (*uint64, error) {
	state, ok := s.StateProvider.(common.StakeCredentialDepositState)
	if !ok {
		return nil, fmt.Errorf("mock state has no stake credential deposits")
	}
	return state.StakeCredentialDeposit(credential)
}

// GovActionById adds the proposed action and an expiry slot to the mock's
// view. Vector transaction slots are markers, not a timeline: the vectors
// advance epochs explicitly. The expiry slot therefore encodes the caller's
// epoch comparison (the reference refuses a vote once the current epoch is
// past gasExpiresAfter): every slot is at or before it while the action is
// live, and every slot after the first is past it once it has expired.
func (s rulesOnlyStateProvider) GovActionById(
	id common.GovActionId,
) (*common.GovActionState, error) {
	gs := s.stateManager.GetGovernanceState()
	if gs == nil {
		return nil, fmt.Errorf("conformance governance state is unavailable")
	}
	proposal := gs.GetProposal(fmt.Sprintf("%x#%d", id.TransactionId, id.GovActionIdx))
	if proposal == nil {
		return nil, nil
	}
	state := &common.GovActionState{
		ActionId:   id,
		ActionType: proposal.ActionType,
		ExpirySlot: ^uint64(0),
		Action:     s.actions[id],
	}
	if state.Action == nil {
		state.Action = govActionFromInitialState(proposal)
	}
	if gs.CurrentEpoch > proposal.ExpiresAfter {
		state.ExpirySlot = 1
	}
	return state, nil
}

func (s rulesOnlyStateProvider) Constitution() (*common.Constitution, error) {
	gs := s.stateManager.GetGovernanceState()
	if gs == nil || gs.Constitution == nil {
		return nil, nil
	}
	c := &common.Constitution{}
	if len(gs.Constitution.PolicyHash) > 0 {
		c.ScriptHash = append([]byte(nil), gs.Constitution.PolicyHash...)
	}
	return c, nil
}

func (s rulesOnlyStateProvider) GovPurposeRoots() (*common.GovPurposeRoots, error) {
	gs := s.stateManager.GetGovernanceState()
	if gs == nil {
		return nil, fmt.Errorf("conformance governance state is unavailable")
	}
	conv := func(id *string) (*common.GovActionId, error) {
		if id == nil {
			return nil, nil
		}
		hash, idx, ok := strings.Cut(*id, "#")
		if !ok {
			return nil, fmt.Errorf("malformed governance action id %q", *id)
		}
		txId, err := hex.DecodeString(hash)
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseUint(idx, 10, 32)
		if err != nil {
			return nil, err
		}
		gid, err := common.NewGovActionId(txId, uint32(n))
		if err != nil {
			return nil, err
		}
		return &gid, nil
	}
	var roots common.GovPurposeRoots
	var err error
	if roots.PParamUpdate, err = conv(gs.Roots.ProtocolParameters); err != nil {
		return nil, err
	}
	if roots.HardFork, err = conv(gs.Roots.HardFork); err != nil {
		return nil, err
	}
	if roots.Committee, err = conv(gs.Roots.ConstitutionalCommittee); err != nil {
		return nil, err
	}
	if roots.Constitution, err = conv(gs.Roots.Constitution); err != nil {
		return nil, err
	}
	return &roots, nil
}

// govActionFromInitialState rebuilds the parts of a proposal from the
// vector's initial state that the Conway rules read: a parameter change's
// update (for the security-group check) and a hard fork's version (for the
// successor check). Proposals made inside the vector keep their decoded
// action instead.
func govActionFromInitialState(p *conformance.ProposalState) common.GovAction {
	switch p.ActionType {
	case common.GovActionTypeParameterChange:
		if p.ParameterUpdate == nil {
			return nil
		}
		return &conway.ConwayParameterChangeGovAction{
			Type:        uint(common.GovActionTypeParameterChange),
			ParamUpdate: *p.ParameterUpdate,
		}
	case common.GovActionTypeHardForkInitiation:
		if p.ProtocolVersion == nil {
			return nil
		}
		action := &common.HardForkInitiationGovAction{
			Type: uint(common.GovActionTypeHardForkInitiation),
		}
		action.ProtocolVersion.Major = p.ProtocolVersion.Major
		action.ProtocolVersion.Minor = p.ProtocolVersion.Minor
		return action
	default:
		return nil
	}
}

// runVectorRulesOnly replays a Blueprint vector through
// common.VerifyTransaction and the vector's gOuroboros rule list alone. The
// ouroboros-mock harness runs its own phase-1 validator before gOuroboros, so
// a transaction that validator rejects never reaches a gOuroboros rule; this
// replay drops that validator so each expected-failure transaction is
// rejected by gOuroboros or not at all. The mock state manager still holds
// the ledger state between events. Vectors with rollback events are skipped
// (ok=false); final-state comparison is not part of this replay.
func runVectorRulesOnly(
	loader *conformance.PParamsLoader,
	path string,
) (rulesOnlyOutcome, bool, error) {
	vector, err := conformance.DecodeTestVector(path)
	if err != nil {
		return rulesOnlyOutcome{}, false, err
	}
	out := rulesOnlyOutcome{
		path:   path,
		title:  vector.Title,
		family: coverageKeyForResult(conformance.VectorResult{Path: path, Title: vector.Title}).RuleFamily,
	}
	for _, event := range vector.Events {
		if event.Type == conformance.EventTypeRollback {
			return out, false, nil
		}
	}
	initial, err := conformance.ParseInitialState(vector.InitialState)
	if err != nil {
		return out, false, err
	}
	pp, err := loader.LoadForVector(vector, initial)
	if err != nil {
		return out, false, err
	}
	sm := rulesOnlyStateManager{
		epochAwareStateManager: epochAwareStateManager{StateManager: conformance.NewMockStateManager()},
		actions:                map[common.GovActionId]common.GovAction{},
	}
	if err := sm.LoadInitialState(initial, pp); err != nil {
		return out, false, err
	}
	rules := rulesOnlyRulesForVector(vector.Title)
	epoch := initial.CurrentEpoch
	for i, event := range vector.Events {
		switch event.Type {
		case conformance.EventTypePassTick:
			continue
		case conformance.EventTypePassEpoch:
			epoch += event.EpochDelta
			if err := sm.ProcessEpochBoundary(epoch); err != nil {
				return out, false, err
			}
			pp = sm.GetProtocolParameters()
			continue
		case conformance.EventTypeTransaction:
		default:
			return out, false, fmt.Errorf("unknown event type %d", event.Type)
		}
		tx := &conway.ConwayTransaction{}
		if _, err := cbor.Decode(event.TxBytes, tx); err != nil {
			if event.Success {
				out.mismatch = fmt.Sprintf("tx %d: decode: %v", i, err)
				return out, true, nil
			}
			continue
		}
		verr := common.VerifyTransaction(
			tx,
			event.Slot,
			sm.GetStateProvider(),
			pp,
			rules,
		)
		switch {
		case verr == nil && !event.Success:
			out.mismatch = fmt.Sprintf("tx %d: expected failure, gOuroboros accepted", i)
			return out, true, nil
		case verr != nil && event.Success:
			out.mismatch = fmt.Sprintf("tx %d: expected success, gOuroboros rejected: %v", i, verr)
			return out, true, nil
		case verr != nil:
			out.rejected++
			continue
		}
		if err := sm.ApplyTransaction(tx, event.Slot); err != nil {
			return out, false, err
		}
	}
	return out, true, nil
}

// rulesOnlyRulesForVector returns the full Conway rule list for a Conway
// specification vector, less the fee and transaction-size rules: the vectors
// carry fees and sizes computed from the reference's own encoding. Other
// vectors keep the harness's rule list.
func rulesOnlyRulesForVector(title string) []common.UtxoValidationRuleFunc {
	if !strings.Contains(title, "ConwayImpSpec") {
		return conformance.ValidationRulesForVector(title)
	}
	var descriptors []common.UtxoValidationRuleDescriptor
	for _, d := range conway.UtxoValidationRuleDescriptors() {
		switch d.Id {
		case common.UtxoValidationRuleFeeTooSmall,
			common.UtxoValidationRuleMaxTxSize:
			continue
		}
		descriptors = append(descriptors, d)
	}
	rules, err := common.UtxoValidationRulesFromDescriptors(descriptors)
	if err != nil {
		panic(err)
	}
	return rules
}

func rulesOnlyOutcomes(t *testing.T, keep func(path string) bool) []rulesOnlyOutcome {
	t.Helper()
	root, err := conformance.ExtractEmbeddedTestdata(t.TempDir())
	if err != nil {
		t.Fatalf("extract testdata: %v", err)
	}
	loader := conformance.NewPParamsLoaderFromTestdata(root)
	paths, err := conformance.CollectVectorFiles(filepath.Join(root, "eras"))
	if err != nil {
		t.Fatalf("collect vectors: %v", err)
	}
	var outcomes []rulesOnlyOutcome
	for _, path := range paths {
		if !keep(path) {
			continue
		}
		out, ok, err := runVectorRulesOnly(loader, path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if ok {
			outcomes = append(outcomes, out)
		}
	}
	return outcomes
}

// rulesOnlyExclusions lists vectors this replay cannot judge: a vector with
// this title is excluded only when its mismatch contains the given text.
var rulesOnlyExclusions = map[string]string{
	// The vote is refused because the current epoch (902) is past the
	// action's expiresAfter (901). gOuroboros compares slots against the
	// caller's ExpirySlot, and this vector's transaction is at slot 0, which
	// no expiry slot precedes.
	"Conway.Imp.ConwayImpSpec - Version 10.GOV.Voting.expired gov-actions": "expected failure, gOuroboros accepted",
	// The mock's parsed current epoch (899) predates proposals submitted in
	// epoch 913, so the pool-retirement window (epoch, epoch+eMax] the
	// transaction is checked against is not the reference's.
	"Conway.Imp.ConwayImpSpec - Version 10.RATIFY.Voting.Active voting stake.StakePool.Rewards contribute to active voting stake even in the absence of StakeDistr": "retirement epoch 918 outside allowed range (899, 904]",
	"Conway.Imp.ConwayImpSpec - Version 10.RATIFY.Voting.Active voting stake.DRep.Rewards contribute to active voting stake even in the absence of StakeDistr":      "retirement epoch 918 outside allowed range (899, 904]",
}

// TestRulesOnlyConformanceVectors replays every Blueprint ledger vector with
// gOuroboros as the only validator (see runVectorRulesOnly) and requires the
// expected outcome of every transaction. Conway specification vectors run
// the full Conway rule list, so each GOV predicate failure the reference
// raises (Conway/Rules/Gov.hs) must be raised by a gOuroboros rule.
func TestRulesOnlyConformanceVectors(t *testing.T) {
	outcomes := rulesOnlyOutcomes(t, func(string) bool { return true })
	type counts struct{ ok, rejected, excluded int }
	byFamily := map[string]*counts{}
	var failed int
	for _, o := range outcomes {
		c := byFamily[o.family]
		if c == nil {
			c = &counts{}
			byFamily[o.family] = c
		}
		if want, ok := rulesOnlyExclusions[o.title]; ok && o.mismatch != "" &&
			strings.Contains(o.mismatch, want) {
			c.excluded++
			continue
		}
		if o.mismatch != "" {
			failed++
			t.Errorf("%s: %s", o.title, o.mismatch)
			continue
		}
		c.ok++
		c.rejected += o.rejected
	}
	families := make([]string, 0, len(byFamily))
	for f := range byFamily {
		families = append(families, f)
	}
	slices.Sort(families)
	total := 0
	for _, f := range families {
		c := byFamily[f]
		total += c.ok + c.excluded
		t.Logf("family=%s matched=%d rejected_tx=%d excluded=%d", f, c.ok, c.rejected, c.excluded)
	}
	if total < 2207 {
		t.Fatalf("replayed %d vectors, want at least 2207", total)
	}
	if failed > 0 {
		t.Fatalf("%d vectors did not match their expected outcome", failed)
	}
}
