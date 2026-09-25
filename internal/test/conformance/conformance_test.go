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

// TestRulesConformanceVectors runs the pinned Cardano Blueprint ledger corpus
// through gouroboros' production validation rules and reports coverage.
func TestRulesConformanceVectors(t *testing.T) {
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
	var (
		failures                         int
		ledgerVectors                    int
		ledgerPassed, ledgerFailed       int
		syntheticPassed, syntheticFailed int
	)
	coverage := conformance.SummarizeCoverage(results)
	for key := range coverage {
		if key.Era == "unknown" || key.RuleFamily == "unknown" {
			t.Errorf(
				"unclassified conformance coverage: era=%s family=%s",
				key.Era,
				key.RuleFamily,
			)
		}
	}
	expected := make(map[conformance.CoverageKey]map[string]int)
	for _, result := range results {
		if !result.Success {
			failures++
		}
		if coverageForResult := conformance.SummarizeCoverage(
			[]conformance.VectorResult{result},
		); len(coverageForResult) > 0 {
			for key := range coverageForResult {
				switch {
				case key.Era == "synthetic" && result.Success:
					syntheticPassed++
				case key.Era == "synthetic":
					syntheticFailed++
				case result.Success:
					ledgerPassed++
				default:
					ledgerFailed++
				}
				if key.Era != "synthetic" {
					ledgerVectors++
				}
				vector, err := conformance.DecodeTestVector(result.Path)
				if err != nil {
					t.Errorf("decode vector %s for coverage report: %v", result.Path, err)
					continue
				}
				for _, event := range vector.Events {
					if event.Type != conformance.EventTypeTransaction {
						continue
					}
					outcome := "accepted"
					if !event.Success {
						outcome = "rejected"
					}
					if expected[key] == nil {
						expected[key] = make(map[string]int)
					}
					expected[key][outcome]++
				}
			}
		}
	}
	if ledgerVectors < 2574 {
		t.Fatalf(
			"pinned Blueprint corpus unexpectedly shrank: got %d ledger vectors, want at least 2574",
			ledgerVectors,
		)
	}

	t.Logf("Cardano Blueprint ledger conformance results:")
	t.Logf("  corpus vectors: %d", ledgerVectors)
	t.Logf("  ledger vectors: passed=%d failed=%d", ledgerPassed, ledgerFailed)
	t.Logf(
		"  synthetic rollback vectors: passed=%d failed=%d",
		syntheticPassed,
		syntheticFailed,
	)
	t.Logf("  coverage by era, rule family, and expected transaction result:")
	for _, key := range conformance.SortedCoverageKeys(coverage) {
		counts := coverage[key]
		t.Logf(
			"  era=%s family=%s vectors=%d passed=%d failed=%d accepted_tx=%d rejected_tx=%d",
			key.Era,
			key.RuleFamily,
			counts.Total,
			counts.Passed,
			counts.Failed,
			expected[key]["accepted"],
			expected[key]["rejected"],
		)
	}

	if failures != 0 {
		for _, result := range results {
			if !result.Success {
				t.Errorf("vector %s failed at event %d: %v", result.Title, result.FailedEvent, result.Error)
			}
		}
		t.Fatalf(
			"%d of %d ledger and synthetic conformance vectors failed",
			failures,
			len(results),
		)
	}
}
