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

	"github.com/blinklabs-io/ouroboros-mock/conformance"
)

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

	sm := conformance.NewMockStateManager()
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

	sm := conformance.NewMockStateManager()
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
