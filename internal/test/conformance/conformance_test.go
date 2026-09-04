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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/ouroboros-mock/conformance"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

// committeeStateManager preserves conformance coverage on the
// released ouroboros-mock graph while allowing newer exact committee providers
// to exercise their full credential-aware behavior unchanged.
type committeeStateManager struct {
	*conformance.MockStateManager
}

func newCommitteeStateManager() *committeeStateManager {
	return &committeeStateManager{
		MockStateManager: conformance.NewMockStateManager(),
	}
}

func (m *committeeStateManager) GetStateProvider() conformance.StateProvider {
	provider := m.MockStateManager.GetStateProvider()
	if _, ok := provider.(common.CommitteeCredentialState); ok {
		return provider
	}
	legacy, ok := provider.(*mockledger.MockLedgerState)
	if !ok {
		// Unknown providers remain unadapted so production validation fails
		// closed instead of treating absent committee state as authoritative.
		return provider
	}
	return &legacyCommitteeStateProvider{
		MockLedgerState: legacy,
		governanceState: m.GetGovernanceState(),
	}
}

// legacyCommitteeStateProvider adapts the released v0.17.0 hash-only mock
// state. The released vectors predate typed committee identity. Newer providers
// implement CommitteeCredentialState directly and never use this fallback.
type legacyCommitteeStateProvider struct {
	*mockledger.MockLedgerState
	governanceState *conformance.GovernanceState
}

func (p *legacyCommitteeStateProvider) CommitteeStateAvailable() (bool, error) {
	// MockLedgerState always initializes committee state, including the
	// authoritative-empty case.
	return true, nil
}

func (p *legacyCommitteeStateProvider) CommitteeCredentialMember(
	credential common.Credential,
) (*common.CommitteeMember, error) {
	// The released store is keyed by hash alone and cannot represent a script
	// member, so answering for a script credential would alias it onto a
	// key-hash member with the same hash. That is the identity aliasing
	// CommitteeCredentialState exists to prevent, so report not a member.
	if credential.CredType != common.CredentialTypeAddrKeyHash {
		return nil, nil
	}
	member, err := p.CommitteeMember(credential.Credential)
	if err != nil || member != nil || p.governanceState == nil {
		return member, err
	}
	if memberInfo := p.governanceState.GetCommitteeMember(
		credential.Credential,
	); memberInfo != nil {
		return legacyCommitteeMember(memberInfo), nil
	}
	for _, proposal := range p.governanceState.Proposals {
		if proposal == nil {
			continue
		}
		if expiry, ok := proposal.ProposedMembers[credential.Credential]; ok {
			return &common.CommitteeMember{
				ColdKey:     credential.Credential,
				ExpiryEpoch: expiry,
			}, nil
		}
	}
	return nil, nil
}

func (p *legacyCommitteeStateProvider) CommitteeHotCredentialMember(
	credential common.Credential,
) (*common.CommitteeMember, error) {
	// Hot credentials carry the same hash-only limitation as cold ones.
	if credential.CredType != common.CredentialTypeAddrKeyHash {
		return nil, nil
	}
	if p.governanceState != nil {
		for coldHash, hotHash := range p.governanceState.HotKeyAuthorizations {
			if hotHash != credential.Credential {
				continue
			}
			member, err := p.CommitteeCredentialMember(common.Credential{
				CredType:   common.CredentialTypeAddrKeyHash,
				Credential: coldHash,
			})
			if err != nil || member == nil {
				return member, err
			}
			hotKey := hotHash
			member.HotKey = &hotKey
			return member, nil
		}
	}
	members, err := p.CommitteeMembers()
	if err != nil {
		return nil, err
	}
	for idx := range members {
		if members[idx].HotKey != nil &&
			*members[idx].HotKey == credential.Credential {
			return &members[idx], nil
		}
	}
	return nil, nil
}

func legacyCommitteeMember(
	member *conformance.CommitteeMemberInfo,
) *common.CommitteeMember {
	if member == nil {
		return nil
	}
	return &common.CommitteeMember{
		ColdKey:     member.ColdKey,
		HotKey:      member.HotKey,
		ExpiryEpoch: member.ExpiryEpoch,
		Resigned:    member.Resigned,
	}
}

var (
	_ conformance.StateManager               = (*committeeStateManager)(nil)
	_ conformance.RewardAccountBalanceSetter = (*committeeStateManager)(nil)
	_ common.CommitteeCredentialState        = (*legacyCommitteeStateProvider)(nil)
)

type currentEpochStateProvider struct {
	*legacyCommitteeStateProvider
	currentEpoch uint64
}

func (p currentEpochStateProvider) CurrentEpoch() uint64 {
	return p.currentEpoch
}

type currentEpochStateManager struct {
	*committeeStateManager
	currentEpoch uint64
}

func newCurrentEpochStateManager() *currentEpochStateManager {
	return &currentEpochStateManager{
		committeeStateManager: newCommitteeStateManager(),
	}
}

func (m *currentEpochStateManager) LoadInitialState(
	state *conformance.ParsedInitialState,
	pp common.ProtocolParameters,
) error {
	if err := m.committeeStateManager.LoadInitialState(state, pp); err != nil {
		return err
	}
	m.currentEpoch = state.CurrentEpoch
	return nil
}

func (m *currentEpochStateManager) ProcessEpochBoundary(
	newEpoch uint64,
) error {
	if err := m.committeeStateManager.ProcessEpochBoundary(newEpoch); err != nil {
		return err
	}
	m.currentEpoch = newEpoch
	return nil
}

func (m *currentEpochStateManager) GetStateProvider() conformance.StateProvider {
	state, ok := m.committeeStateManager.GetStateProvider().(*legacyCommitteeStateProvider)
	if !ok {
		panic("ouroboros-mock returned an unexpected state provider")
	}
	return currentEpochStateProvider{
		legacyCommitteeStateProvider: state,
		currentEpoch:                 m.currentEpoch,
	}
}

func (m *currentEpochStateManager) Reset() error {
	if err := m.committeeStateManager.Reset(); err != nil {
		return err
	}
	m.currentEpoch = 0
	return nil
}

// TestRulesConformanceVectors runs the Amaru ledger rules conformance test vectors
// using the shared harness from ouroboros-mock/conformance.
//
// The test vectors exercise Conway era ledger rules including:
// - UTxO validation (inputs, outputs, fees, collateral)
// - Certificate processing (stake, pool, DRep, committee)
// - Governance (proposals, voting, enactment)
// - Script execution (native scripts, Plutus V1/V2/V3)
//
// Test vectors are embedded in the ouroboros-mock module and extracted at test time.
func TestRulesConformanceVectors(t *testing.T) {
	testdataRoot, err := conformance.ExtractEmbeddedTestdata(t.TempDir())
	if err != nil {
		t.Fatalf("failed to extract embedded testdata: %v", err)
	}

	sm := newCurrentEpochStateManager()
	harness := conformance.NewHarness(sm, conformance.HarnessConfig{
		TestdataRoot: testdataRoot,
		Debug:        testing.Verbose(),
	})

	harness.RunAllVectors(t)
}

// TestRulesConformanceVectorsWithResults runs the conformance tests and reports
// detailed statistics. This is useful for tracking implementation progress.
func TestRulesConformanceVectorsWithResults(t *testing.T) {
	testdataRoot, err := conformance.ExtractEmbeddedTestdata(t.TempDir())
	if err != nil {
		t.Fatalf("failed to extract embedded testdata: %v", err)
	}

	sm := newCurrentEpochStateManager()
	harness := conformance.NewHarness(sm, conformance.HarnessConfig{
		TestdataRoot: testdataRoot,
		Debug:        false,
	})

	results, err := harness.RunAllVectorsWithResults()
	if err != nil {
		t.Fatalf("failed to run vectors: %v", err)
	}

	var successes, failures int
	for _, result := range results {
		if result.Success {
			successes++
		} else {
			failures++
		}
	}

	t.Logf("Conformance Test Results:")
	t.Logf("  Total vectors: %d", len(results))
	t.Logf("  Passed: %d", successes)
	t.Logf("  Failed: %d", failures)
	t.Logf("  Pass rate: %.1f%%", float64(successes)/float64(len(results))*100)

	if failures > 0 && testing.Verbose() {
		t.Log("First failures:")
		failCount := 0
		for _, result := range results {
			if !result.Success && failCount < 5 {
				t.Logf("  %s: %v", result.Title, result.Error)
				failCount++
			}
		}
		if failures > 5 {
			t.Logf("  ... and %d more failures", failures-5)
		}
	}
}
