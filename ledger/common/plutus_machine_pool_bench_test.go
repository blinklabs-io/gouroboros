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
	"testing"

	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/lang"
)

// BenchmarkEvaluateManyRedeemers evaluates a block's worth of redeemers of one
// PlutusV3 script per op, once with a single caller-built *cek.EvalContext
// reused across them and once with a fresh EvalContext per redeemer.
func BenchmarkEvaluateManyRedeemers(b *testing.B) {
	const redeemers = 64
	script := buildMachinePoolTestV3Script(b)
	params := make(
		[]int64,
		len(lang.GetParamNamesForVersion(lang.LanguageVersionV3)),
	)
	for i := range params {
		params[i] = int64(i + 1)
	}
	newEvalContext := func(b *testing.B) *cek.EvalContext {
		evalContext, err := cek.NewEvalContext(
			lang.LanguageVersionV3,
			cek.ProtoVersion{Major: 10},
			params,
		)
		if err != nil {
			b.Fatal(err)
		}
		return evalContext
	}
	scriptContext := machinePoolTestScriptContext()
	budget := machinePoolTestBudget()

	b.Run("reused-eval-context", func(b *testing.B) {
		evalContext := newEvalContext(b)
		b.ReportAllocs()
		for b.Loop() {
			for range redeemers {
				if _, err := script.Evaluate(
					scriptContext,
					budget,
					evalContext,
				); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("fresh-eval-context", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for range redeemers {
				if _, err := script.Evaluate(
					scriptContext,
					budget,
					newEvalContext(b),
				); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}
