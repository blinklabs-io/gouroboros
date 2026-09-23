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
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/ouroboros-mock/conformance"
)

type epochAwareStateManager struct {
	conformance.StateManager
}

func (m epochAwareStateManager) GetStateSnapshot() *conformance.StateSnapshot {
	if provider, ok := m.StateManager.(conformance.StateSnapshotProvider); ok {
		return provider.GetStateSnapshot()
	}
	return nil
}

func (m epochAwareStateManager) GetStateProvider() conformance.StateProvider {
	return epochAwareStateProvider{
		StateProvider: m.StateManager.GetStateProvider(),
		stateManager:  m.StateManager,
	}
}

type epochAwareStateProvider struct {
	conformance.StateProvider
	stateManager conformance.StateManager
}

func (s epochAwareStateProvider) EpochForSlot(uint64) (uint64, error) {
	state := s.stateManager.GetGovernanceState()
	if state == nil {
		return 0, fmt.Errorf("conformance governance state is unavailable")
	}
	return state.CurrentEpoch, nil
}

func (s epochAwareStateProvider) CommitteeStateAvailable() (bool, error) {
	state, ok := s.StateProvider.(common.CommitteeCredentialState)
	if !ok {
		return false, nil
	}
	return state.CommitteeStateAvailable()
}

func (s epochAwareStateProvider) CommitteeCredentialMember(
	credential common.Credential,
) (*common.CommitteeMember, error) {
	state, ok := s.StateProvider.(common.CommitteeCredentialState)
	if !ok {
		return nil, nil
	}
	return state.CommitteeCredentialMember(credential)
}

func (s epochAwareStateProvider) CommitteeHotCredentialMember(
	credential common.Credential,
) (*common.CommitteeMember, error) {
	state, ok := s.StateProvider.(common.CommitteeCredentialState)
	if !ok {
		return nil, nil
	}
	return state.CommitteeHotCredentialMember(credential)
}

func (s epochAwareStateProvider) DRepDelegation(
	credential common.Credential,
) (*common.Drep, error) {
	state, ok := s.StateProvider.(common.DRepDelegationState)
	if !ok {
		return nil, nil
	}
	return state.DRepDelegation(credential)
}

// TestStateProviderExposesCommitteeCredentials pins the committee capability
// the vector runs below depend on.
//
// ouroboros-mock v0.19.0 makes the mock state provider implement
// common.CommitteeCredentialState, so committee membership resolves by typed
// credential. Earlier releases exposed committee state keyed by hash alone,
// which cannot distinguish a script member from a key-hash member sharing that
// hash, and this package carried a local adapter to bridge the gap. The
// adapter is gone; if a future mock stops implementing the interface the
// harness would silently answer committee lookups from hash-only state, so
// fail here instead of reporting a vector count that no longer covers
// committee credential identity.
func TestStateProviderExposesCommitteeCredentials(t *testing.T) {
	provider := conformance.NewMockStateManager().GetStateProvider()
	if _, ok := provider.(common.CommitteeCredentialState); !ok {
		t.Fatalf(
			"mock state provider %T does not implement common.CommitteeCredentialState",
			provider,
		)
	}
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

	sm := epochAwareStateManager{StateManager: conformance.NewMockStateManager()}
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

	sm := epochAwareStateManager{StateManager: conformance.NewMockStateManager()}
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
