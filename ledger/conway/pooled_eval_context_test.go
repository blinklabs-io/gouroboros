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
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

// oneshotMintEvaluation returns the mainnet one-shot mint transaction's
// PlutusV3 minting script and the script context its mint redeemer runs with.
func oneshotMintEvaluation(
	t *testing.T,
) (lcommon.PlutusV3Script, data.PlutusData) {
	t.Helper()
	raw, err := hex.DecodeString(
		strings.TrimSpace(testdata.MainnetOneShotMintTxHex),
	)
	require.NoError(t, err)
	var tx conway.ConwayTransaction
	_, err = cbor.Decode(raw, &tx)
	require.NoError(t, err)
	v3Scripts := tx.Witnesses().PlutusV3Scripts()
	require.Len(t, v3Scripts, 1)
	txInfo, err := script.NewTxInfoV3FromTransaction(
		mainnetSlotState{},
		&tx,
		resolvedInputsForTx(&tx),
		lcommon.ProtocolVersionConway,
	)
	require.NoError(t, err)
	policies := tx.AssetMint().Policies()
	require.Len(t, policies, 1)
	redeemers := tx.Witnesses().Redeemers()
	require.NotNil(t, redeemers)
	for k, v := range redeemers.Iter() {
		if k.Tag != lcommon.RedeemerTagMint {
			continue
		}
		ctx := script.NewScriptContextV3(
			txInfo,
			script.Redeemer{
				Tag:     k.Tag,
				Index:   k.Index,
				Data:    v.Data.Data,
				ExUnits: v.ExUnits,
			},
			script.ScriptPurposeMinting{PolicyId: policies[0]},
		)
		return v3Scripts[0], ctx.ToPlutusData()
	}
	t.Fatal("no mint redeemer found")
	return nil, nil
}

// TestPooledEvalContextConcurrentCostModels: goroutines evaluate one real
// script under two cost models at once, each fetching its pooled EvalContext
// per evaluation, and every evaluation consumes exactly what a lone evaluation
// under its own cost model consumes. A context or Machine shared across cost
// models, or a Machine run by two evaluations at once, shows as a different
// budget or a -race report.
func TestPooledEvalContextConcurrentCostModels(t *testing.T) {
	plutusScript, scriptContext := oneshotMintEvaluation(t)
	// A protocol major no other test uses, so this test's tuples have their
	// own cache entries and Machine pools.
	const protoMajor = 930000
	doubled := make([]int64, len(testdata.Epoch653PlutusV3CostModel))
	for i, p := range testdata.Epoch653PlutusV3CostModel {
		doubled[i] = 2 * p
	}
	costModels := [][]int64{testdata.Epoch653PlutusV3CostModel, doubled}
	budget := lcommon.ExUnits{Memory: 1_000_000_000, Steps: 1_000_000_000_000}

	evaluate := func(costModel []int64) (lcommon.ExUnits, error) {
		evalContext, err := lcommon.PooledEvalContext(
			lang.LanguageVersionV3,
			protoMajor,
			costModel,
		)
		if err != nil {
			return lcommon.ExUnits{}, err
		}
		return plutusScript.Evaluate(scriptContext, budget, evalContext)
	}
	want := make([]lcommon.ExUnits, len(costModels))
	for i, costModel := range costModels {
		used, err := evaluate(costModel)
		require.NoError(t, err)
		want[i] = used
	}
	require.NotEqual(t, want[0], want[1],
		"the two cost models must charge differently for this test to"+
			" tell their contexts apart")

	const goroutines = 8
	const iterations = 10
	errs := make(chan error, goroutines*iterations)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			<-start
			for i := range iterations {
				which := (g + i) % len(costModels)
				got, err := evaluate(costModels[which])
				if err != nil {
					errs <- err
					continue
				}
				if got != want[which] {
					errs <- fmt.Errorf(
						"cost model %d: consumed %+v, want %+v",
						which,
						got,
						want[which],
					)
				}
			}
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
