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

package dijkstra

import (
	"errors"
	"maps"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
)

// PlutusRedeemerEvaluation is the execution cost of one Plutus redeemer of a
// Dijkstra transaction.
type PlutusRedeemerEvaluation struct {
	// SubTransactionIndex is the position of the sub-transaction carrying the
	// redeemer, or nil for a redeemer of the top-level transaction.
	SubTransactionIndex *uint32
	Key                 common.RedeemerKey
	ExUnits             common.ExUnits
}

// EvaluatePlutusScripts runs the Plutus script of every redeemer at every
// level of tx, with budget as each script's execution limit, and returns the
// execution units each one consumed.
//
// It applies the structural checks and runs the scripts exactly as
// UtxoValidatePlutusScripts does, except that the declared units and the
// IsValid flag are ignored. Results are ordered by level, sub-transactions
// first in body order and the top-level transaction last, then by redeemer
// tag and index.
func EvaluatePlutusScripts(
	tx *DijkstraTransaction,
	ls common.LedgerState,
	pp common.ProtocolParameters,
	budget common.ExUnits,
) ([]PlutusRedeemerEvaluation, error) {
	if tx == nil {
		return nil, errors.New("nil Dijkstra transaction")
	}
	params, err := conwayPparams(pp)
	if err != nil {
		return nil, err
	}
	levels, available, err := dijkstraScriptLevels(tx, ls)
	if err != nil {
		return nil, err
	}
	if err := validateDijkstraPlutusScriptLevels(
		levels,
		available,
	); err != nil {
		return nil, err
	}
	return evaluateDijkstraPlutusLevels(levels, available, ls, params, &budget)
}

// evaluateDijkstraPlutusLevels runs every level's Plutus redeemers: V1-V3
// redeemers other than guards through the Conway rule, top-level V1-V3
// guards with guarding redeemers omitted from TxInfo, and V4 redeemers with
// the Dijkstra V4 context. A nil budget limits each script to its declared
// units.
func evaluateDijkstraPlutusLevels(
	levels []dijkstraScriptLevel,
	available map[common.ScriptHash]common.Script,
	ls common.LedgerState,
	pp *conway.ConwayProtocolParameters,
	budget *common.ExUnits,
) ([]PlutusRedeemerEvaluation, error) {
	var ret []PlutusRedeemerEvaluation
	for _, level := range levels {
		v4Keys, err := dijkstraPlutusV4RedeemerKeys(level, available)
		if err != nil {
			return nil, err
		}
		used, err := conway.EvaluatePlutusScripts(
			transactionWithoutRedeemers{
				Transaction: transactionWithAvailablePlutusScripts{
					Transaction: level.tx,
					available:   available,
				},
				excluded: v4Keys,
				guarding: true,
			},
			ls,
			pp,
			budget,
		)
		if err != nil {
			return nil, err
		}
		if level.subTxIndex == nil {
			guards, err := validateGuardingPlutusScripts(
				level.tx,
				ls,
				pp,
				available,
				level.resolved,
				budget,
			)
			if err != nil {
				return nil, err
			}
			maps.Copy(used, guards)
		}
		v4, err := validateDijkstraPlutusV4Scripts(
			level,
			pp,
			available,
			v4Keys,
			budget,
		)
		if err != nil {
			return nil, err
		}
		maps.Copy(used, v4)
		wits := level.tx.Witnesses()
		if wits == nil || wits.Redeemers() == nil {
			continue
		}
		for key := range wits.Redeemers().Iter() {
			units, ok := used[key]
			if !ok {
				continue
			}
			ret = append(ret, PlutusRedeemerEvaluation{
				SubTransactionIndex: level.subTxIndex,
				Key:                 key,
				ExUnits:             units,
			})
		}
	}
	return ret, nil
}
