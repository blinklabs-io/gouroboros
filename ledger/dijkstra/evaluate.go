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
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
)

// PlutusRedeemerEvaluation is the execution cost of one Plutus redeemer of a
// Dijkstra transaction.
type PlutusRedeemerEvaluation struct {
	// SubTransactionIndex is the position of the sub-transaction carrying the
	// redeemer, or nil for a redeemer of the top-level transaction.
	SubTransactionIndex *uint32
	Key                 common.RedeemerKey
	ScriptHash          common.ScriptHash
	ExUnits             common.ExUnits
}

// EvaluatePlutusScripts runs the Plutus script of every redeemer at every
// level of tx, with budget as each script's execution limit, and returns the
// execution units each one consumed.
//
// Scripts run against the same script contexts UtxoValidatePlutusScripts
// builds, so a redeemer declaring the returned units passes validation. The
// declared units and the IsValid flag are ignored. Results are ordered by
// level, sub-transactions first in body order and the top-level transaction
// last, then by redeemer tag and index. A redeemer whose script fails returns
// conway.PlutusScriptFailedError.
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
	if err := validateDijkstraPlutusScriptLevels(levels, available); err != nil {
		return nil, err
	}
	var ret []PlutusRedeemerEvaluation
	for _, level := range levels {
		results, err := evaluateDijkstraLevel(
			level,
			ls,
			params,
			available,
			budget,
		)
		if err != nil {
			return nil, err
		}
		ret = append(ret, results...)
	}
	return ret, nil
}

// dijkstraLevelEvaluator mirrors the three script-execution paths of
// UtxoValidatePlutusScripts for one level: Plutus V4 redeemers with the
// Dijkstra V4 context, top-level V1-V3 guarding redeemers with a TxInfo that
// omits guarding redeemers, and every other V1-V3 redeemer through the
// Conway path, whose TxInfo omits guarding and V4 redeemers.
type dijkstraLevelEvaluator struct {
	level     dijkstraScriptLevel
	ls        common.LedgerState
	pp        *conway.ConwayProtocolParameters
	available map[common.ScriptHash]common.Script
	budget    common.ExUnits
	v4Keys    map[common.RedeemerKey]struct{}

	conwayTx       common.Transaction
	conwayInfo     txInfoCache
	guardingTx     common.Transaction
	guardingInfo   txInfoCache
	resolvedByName map[string]common.Utxo
}

type txInfoCache struct {
	v1      *script.TxInfoV1
	v2      *script.TxInfoV2
	v3      *script.TxInfoV3
	v3Error error
}

func evaluateDijkstraLevel(
	level dijkstraScriptLevel,
	ls common.LedgerState,
	pp *conway.ConwayProtocolParameters,
	available map[common.ScriptHash]common.Script,
	budget common.ExUnits,
) ([]PlutusRedeemerEvaluation, error) {
	wits := level.tx.Witnesses()
	if wits == nil || wits.Redeemers() == nil {
		return nil, nil
	}
	v4Keys, err := dijkstraPlutusV4RedeemerKeys(level, available)
	if err != nil {
		return nil, err
	}
	e := &dijkstraLevelEvaluator{
		level:     level,
		ls:        ls,
		pp:        pp,
		available: available,
		budget:    budget,
		v4Keys:    v4Keys,
		conwayTx: transactionWithoutRedeemers{
			Transaction: transactionWithAvailablePlutusScripts{
				Transaction: level.tx,
				available:   available,
			},
			excluded: v4Keys,
			guarding: true,
		},
		guardingTx: transactionWithoutGuardingRedeemers{
			Transaction: level.tx,
		},
	}
	var ret []PlutusRedeemerEvaluation
	for key, value := range wits.Redeemers().Iter() {
		hash, used, ran, err := e.evaluate(key, value)
		if err != nil {
			return nil, err
		}
		if !ran {
			continue
		}
		ret = append(ret, PlutusRedeemerEvaluation{
			SubTransactionIndex: level.subTxIndex,
			Key:                 key,
			ScriptHash:          hash,
			ExUnits:             used,
		})
	}
	return ret, nil
}

// evaluate runs the script of one redeemer. ran is false for a redeemer the
// validation rule does not execute: a V1-V3 guarding redeemer of a
// sub-transaction.
func (e *dijkstraLevelEvaluator) evaluate(
	key common.RedeemerKey,
	value common.RedeemerValue,
) (common.ScriptHash, common.ExUnits, bool, error) {
	if _, ok := e.v4Keys[key]; ok {
		hash, used, err := e.evaluateV4(key, value)
		return hash, used, true, err
	}
	if key.Tag == common.RedeemerTagGuarding {
		if e.level.subTxIndex != nil {
			return common.ScriptHash{}, common.ExUnits{}, false, nil
		}
		purpose, ok := dijkstraGuardingPurpose(e.level.tx, key)
		if !ok {
			return common.ScriptHash{}, common.ExUnits{}, false,
				conway.ExtraRedeemerError{RedeemerKey: key}
		}
		hash, used, err := e.evaluatePreV4(
			e.guardingTx,
			&e.guardingInfo,
			purpose,
			nil,
			key,
			value,
		)
		return hash, used, true, err
	}
	purpose, err := e.conwayPurpose(key)
	if err != nil {
		return common.ScriptHash{}, common.ExUnits{}, false,
			conway.ExtraRedeemerError{RedeemerKey: key}
	}
	var datum data.PlutusData
	if spend, ok := purpose.(script.ScriptPurposeSpending); ok {
		datum = spend.Datum
	}
	hash, used, err := e.evaluatePreV4(
		e.conwayTx,
		&e.conwayInfo,
		purpose,
		datum,
		key,
		value,
	)
	return hash, used, true, err
}

func (e *dijkstraLevelEvaluator) evaluateV4(
	key common.RedeemerKey,
	value common.RedeemerValue,
) (common.ScriptHash, common.ExUnits, error) {
	purpose, err := dijkstraPurposeForKey(e.level, key)
	if err != nil {
		return common.ScriptHash{}, common.ExUnits{},
			conway.ExtraRedeemerError{RedeemerKey: key}
	}
	hash := purpose.ScriptHash()
	candidate, ok := e.available[hash].(common.PlutusV4Script)
	if !ok {
		return hash, common.ExUnits{},
			common.MissingScriptWitnessesError{ScriptHash: hash}
	}
	scriptContext, err := dijkstraPlutusV4Context(e.level, purpose, key, value)
	if err != nil {
		return hash, common.ExUnits{},
			conway.ScriptContextConstructionError{Err: err}
	}
	evalContext, err := e.evalContext(lang.LanguageVersionV4, 3)
	if err != nil {
		return hash, common.ExUnits{}, err
	}
	used, err := candidate.Evaluate(scriptContext, e.budget, evalContext)
	return hash, used, scriptFailed(hash, key, err)
}

// evaluatePreV4 runs a Plutus V1-V3 script. tx is the view whose TxInfo the
// script sees, and datum is the spending datum, if any.
func (e *dijkstraLevelEvaluator) evaluatePreV4(
	tx common.Transaction,
	infos *txInfoCache,
	purpose script.ScriptPurpose,
	datum data.PlutusData,
	key common.RedeemerKey,
	value common.RedeemerValue,
) (common.ScriptHash, common.ExUnits, error) {
	hash := purpose.ScriptHash()
	candidate, ok := e.available[hash]
	if !ok {
		return hash, common.ExUnits{},
			common.MissingScriptWitnessesError{ScriptHash: hash}
	}
	if e.ls == nil {
		return hash, common.ExUnits{}, errors.New(
			"ledger state is required for Dijkstra script evaluation",
		)
	}
	redeemerData := data.Normalize(value.Data.Data)
	major := e.pp.ProtocolVersion.Major
	var used common.ExUnits
	var runErr error
	switch s := candidate.(type) {
	case common.PlutusV3Script:
		info, err := infos.txInfoV3(e.ls, tx, e.level.resolved, major)
		if err != nil {
			return hash, used, err
		}
		redeemer := script.Redeemer{
			Tag:     key.Tag,
			Index:   key.Index,
			Data:    redeemerData,
			ExUnits: value.ExUnits,
		}
		evalContext, err := e.evalContext(lang.LanguageVersionV3, 2)
		if err != nil {
			return hash, used, err
		}
		scriptContext := script.NewScriptContextV3(info, redeemer, purpose)
		used, runErr = s.Evaluate(
			scriptContext.ToPlutusData(),
			e.budget,
			evalContext,
		)
	case common.PlutusV2Script:
		if err := requireSpendingDatum(purpose, datum); err != nil {
			return hash, used, err
		}
		info, err := infos.txInfoV2(e.ls, tx, e.level.resolved, major)
		if err != nil {
			return hash, used, err
		}
		evalContext, err := e.evalContext(lang.LanguageVersionV2, 1)
		if err != nil {
			return hash, used, err
		}
		scriptContext := script.NewScriptContextV1V2(info, purpose)
		used, runErr = s.Evaluate(
			datum,
			redeemerData,
			scriptContext.ToPlutusData(),
			e.budget,
			evalContext,
		)
	case common.PlutusV1Script:
		if err := requireSpendingDatum(purpose, datum); err != nil {
			return hash, used, err
		}
		info, err := infos.txInfoV1(e.ls, tx, e.level.resolved, major)
		if err != nil {
			return hash, used, err
		}
		evalContext, err := e.evalContext(lang.LanguageVersionV1, 0)
		if err != nil {
			return hash, used, err
		}
		scriptContext := script.NewScriptContextV1V2(info, purpose)
		used, runErr = s.Evaluate(
			datum,
			redeemerData,
			scriptContext.ToPlutusData(),
			e.budget,
			evalContext,
		)
	default:
		return hash, used, conway.ExtraRedeemerError{RedeemerKey: key}
	}
	return hash, used, scriptFailed(hash, key, runErr)
}

func (e *dijkstraLevelEvaluator) evalContext(
	version lang.LanguageVersion,
	costModel uint,
) (*cek.EvalContext, error) {
	evalContext, err := common.PooledEvalContext(
		version,
		e.pp.ProtocolVersion.Major,
		e.pp.CostModels[costModel],
	)
	if err != nil {
		return nil, fmt.Errorf("build evaluation context: %w", err)
	}
	return evalContext, nil
}

// conwayPurpose builds the purpose of a non-guarding V1-V3 redeemer the way
// conway.UtxoValidatePlutusScripts does for the level.
func (e *dijkstraLevelEvaluator) conwayPurpose(
	key common.RedeemerKey,
) (script.ScriptPurpose, error) {
	tx := e.conwayTx
	if e.resolvedByName == nil {
		e.resolvedByName = make(map[string]common.Utxo, len(e.level.resolved))
		for _, utxo := range e.level.resolved {
			if utxo.Id != nil {
				e.resolvedByName[utxo.Id.String()] = utxo
			}
		}
	}
	mint := tx.AssetMint()
	if mint == nil {
		mint = &common.MultiAsset[common.MultiAssetTypeMint]{}
	}
	witnessDatums := make(map[common.Blake2b256]*common.Datum)
	if wits := tx.Witnesses(); wits != nil {
		for _, item := range wits.PlutusData() {
			datum := item
			witnessDatums[datum.Hash()] = &datum
		}
	}
	return script.BuildScriptPurpose(
		key,
		e.resolvedByName,
		script.SortInputs(tx.Inputs()),
		*mint,
		tx.Certificates(),
		tx.Withdrawals(),
		tx.VotingProcedures(),
		tx.ProposalProcedures(),
		witnessDatums,
		e.pp.ProtocolVersion.Major,
	)
}

func requireSpendingDatum(
	purpose script.ScriptPurpose,
	datum data.PlutusData,
) error {
	spend, ok := purpose.(script.ScriptPurposeSpending)
	if !ok || datum != nil {
		return nil
	}
	return conway.MissingDatumForSpendingScriptError{
		ScriptHash: purpose.ScriptHash(),
		Input:      spend.Input.Id,
	}
}

func scriptFailed(
	hash common.ScriptHash,
	key common.RedeemerKey,
	err error,
) error {
	if err == nil {
		return nil
	}
	return conway.PlutusScriptFailedError{
		ScriptHash: hash,
		Tag:        key.Tag,
		Index:      key.Index,
		Err:        err,
	}
}

func (c *txInfoCache) txInfoV3(
	ls common.LedgerState,
	tx common.Transaction,
	resolved []common.Utxo,
	major uint,
) (script.TxInfoV3, error) {
	if c.v3 != nil {
		return *c.v3, nil
	}
	if c.v3Error != nil {
		return script.TxInfoV3{}, c.v3Error
	}
	if err := script.ValidatePlutusV3ReferenceInputs(tx, major); err != nil {
		c.v3Error = conway.ScriptContextConstructionError{Err: err}
		return script.TxInfoV3{}, c.v3Error
	}
	info, err := script.NewTxInfoV3FromTransaction(ls, tx, resolved, major)
	if err != nil {
		c.v3Error = conway.ScriptContextConstructionError{Err: err}
		return script.TxInfoV3{}, c.v3Error
	}
	c.v3 = &info
	return info, nil
}

func (c *txInfoCache) txInfoV2(
	ls common.LedgerState,
	tx common.Transaction,
	resolved []common.Utxo,
	major uint,
) (script.TxInfoV2, error) {
	if c.v2 != nil {
		return *c.v2, nil
	}
	info, err := script.NewTxInfoV2FromTransaction(
		ls,
		tx,
		resolved,
		script.StrictValidityUpperBoundForTransaction(tx),
		major,
	)
	if err != nil {
		return script.TxInfoV2{}, conway.ScriptContextConstructionError{
			Err: err,
		}
	}
	c.v2 = &info
	return info, nil
}

func (c *txInfoCache) txInfoV1(
	ls common.LedgerState,
	tx common.Transaction,
	resolved []common.Utxo,
	major uint,
) (script.TxInfoV1, error) {
	if c.v1 != nil {
		return *c.v1, nil
	}
	info, err := script.NewTxInfoV1FromTransaction(
		ls,
		tx,
		resolved,
		script.StrictValidityUpperBoundForTransaction(tx),
		major,
	)
	if err != nil {
		return script.TxInfoV1{}, conway.ScriptContextConstructionError{
			Err: err,
		}
	}
	c.v1 = &info
	return info, nil
}
