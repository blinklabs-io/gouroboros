// Copyright 2025 Blink Labs Software
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

package script

import (
	"bytes"
	"math/big"
	"slices"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
)

type ScriptContext interface {
	isScriptContext()
	ToPlutusData() data.PlutusData
}

type ScriptContextV1V2 struct {
	TxInfo TxInfo
	// Purpose ScriptPurpose
}

func (ScriptContextV1V2) isScriptContext() {}

func (s ScriptContextV1V2) ToPlutusData() data.PlutusData {
	// TODO
	return nil
}

type ScriptContextV3 struct {
	TxInfo   TxInfo
	Redeemer Redeemer
	Purpose  ScriptInfo
}

func (ScriptContextV3) isScriptContext() {}

func (s ScriptContextV3) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		0,
		s.TxInfo.ToPlutusData(),
		s.Redeemer.ToPlutusData(),
		s.Purpose.ToPlutusData(),
	)
}

func NewScriptContextV3(
	txInfo TxInfo,
	redeemer Redeemer,
	purpose ScriptInfo,
) ScriptContext {
	return ScriptContextV3{
		TxInfo:   txInfo,
		Redeemer: redeemer,
		Purpose:  purpose,
	}
}

type TxInfo interface {
	isTxInfo()
	ToPlutusData() data.PlutusData
}

type TxInfoV1 struct {
	Inputs       []lcommon.Utxo
	Outputs      []lcommon.Utxo
	Fee          uint64
	Mint         lcommon.MultiAsset[lcommon.MultiAssetTypeMint]
	Certificates []lcommon.Certificate
	Withdrawals  KeyValuePairs[*lcommon.Address, Coin]
	ValidRange   TimeRange
	Signatories  []lcommon.Blake2b224
	Data         KeyValuePairs[lcommon.Blake2b256, data.PlutusData]
	Redeemers    KeyValuePairs[ScriptInfo, Redeemer]
	Id           lcommon.Blake2b256
}

func (TxInfoV1) isTxInfo() {}

func (t TxInfoV1) ToPlutusData() data.PlutusData {
	// TODO
	return nil
}

type TxInfoV2 struct {
	Inputs          []lcommon.Utxo
	ReferenceInputs []lcommon.Utxo
	Outputs         []lcommon.Utxo
	Fee             uint64
	Mint            lcommon.MultiAsset[lcommon.MultiAssetTypeMint]
	Certificates    []lcommon.Certificate
	Withdrawals     KeyValuePairs[*lcommon.Address, Coin]
	ValidRange      TimeRange
	Signatories     []lcommon.Blake2b224
	Redeemers       KeyValuePairs[ScriptInfo, Redeemer]
	Data            KeyValuePairs[lcommon.Blake2b256, data.PlutusData]
	Id              lcommon.Blake2b256
}

func (TxInfoV2) isTxInfo() {}

func (t TxInfoV2) ToPlutusData() data.PlutusData {
	// TODO
	return nil
}

type TxInfoV3 struct {
	Inputs                []ResolvedInput
	ReferenceInputs       []ResolvedInput
	Outputs               []lcommon.TransactionOutput
	Fee                   uint64
	Mint                  lcommon.MultiAsset[lcommon.MultiAssetTypeMint]
	Certificates          []lcommon.Certificate
	Withdrawals           map[*lcommon.Address]uint64
	ValidRange            TimeRange
	Signatories           []lcommon.Blake2b224
	Redeemers             KeyValuePairs[ScriptInfo, Redeemer]
	Data                  KeyValuePairs[lcommon.Blake2b256, data.PlutusData]
	Id                    lcommon.Blake2b256
	Votes                 KeyValuePairs[lcommon.Voter, KeyValuePairs[lcommon.GovActionId, lcommon.VotingProcedure]]
	ProposalProcedures    []lcommon.ProposalProcedure
	CurrentTreasuryAmount Option[Coin]
	TreasuryDonation      Option[PositiveCoin]
}

func (TxInfoV3) isTxInfo() {}

func (t TxInfoV3) ToPlutusData() data.PlutusData {
	tmpRedeemers := make(KeyValuePairs[ScriptInfo, Redeemer], len(t.Redeemers))
	for i, pair := range t.Redeemers {
		tmpRedeemers[i] = KeyValuePair[ScriptInfo, Redeemer]{
			Key:   scriptPurposeStripDatum(pair.Key),
			Value: pair.Value,
		}
	}
	return data.NewConstr(
		0,
		toPlutusData(t.Inputs),
		toPlutusData(t.ReferenceInputs),
		toPlutusData(t.Outputs),
		data.NewInteger(new(big.Int).SetUint64(t.Fee)),
		t.Mint.ToPlutusData(),
		// TODO: certs
		toPlutusData([]any{}),
		toPlutusData(t.Withdrawals),
		t.ValidRange.ToPlutusData(),
		toPlutusData(t.Signatories),
		tmpRedeemers.ToPlutusData(),
		t.Data.ToPlutusData(),
		data.NewByteString(t.Id.Bytes()),
		// TODO: votes
		data.NewMap([][2]data.PlutusData{}),
		// TODO: proposal procedures
		toPlutusData([]any{}),
		// TODO: current treasury amount
		data.NewConstr(1),
		// TODO: treasury donation
		data.NewConstr(1),
	)
}

func NewTxInfoV3FromTransaction(
	tx lcommon.Transaction,
	resolvedInputs []lcommon.Utxo,
) TxInfoV3 {
	assetMint := tx.AssetMint()
	if assetMint == nil {
		assetMint = &lcommon.MultiAsset[lcommon.MultiAssetTypeMint]{}
	}
	inputs := sortInputs(tx.Inputs())
	redeemers := redeemersInfo(
		tx.Witnesses(),
		scriptPurposeBuilder(
			resolvedInputs,
			inputs,
			*assetMint,
			// TODO: certificates
			tx.Withdrawals(),
			// TODO: proposal procedures
			// TODO: votes
		),
	)
	tmpData := dataInfo(tx.Witnesses())
	ret := TxInfoV3{
		Inputs: expandInputs(inputs, resolvedInputs),
		ReferenceInputs: expandInputs(
			sortInputs(tx.ReferenceInputs()),
			resolvedInputs,
		),
		Outputs: collapseOutputs(tx.Produced()),
		Fee:     tx.Fee(),
		Mint:    *assetMint,
		ValidRange: TimeRange{
			tx.TTL(),
			tx.ValidityIntervalStart(),
		},
		Withdrawals: tx.Withdrawals(),
		Signatories: signatoriesInfo(tx.RequiredSigners()),
		Redeemers:   redeemers,
		Data:        tmpData,
		Id:          tx.Hash(),
		// TODO: Votes
		// TODO: ProposalProcedures
		// TODO: CurrentTreasuryAmount
		// TODO: TreasuryDonation
	}
	return ret
}

type TimeRange struct {
	lowerBound uint64
	upperBound uint64
}

func (t TimeRange) ToPlutusData() data.PlutusData {
	bound := func(bound uint64, isLower bool) data.PlutusData {
		if bound > 0 {
			return data.NewConstr(
				0,
				data.NewConstr(
					1,
					data.NewInteger(
						new(big.Int).SetUint64(bound),
					),
				),
				toPlutusData(isLower),
			)
		} else {
			var constrType uint = 0
			if !isLower {
				constrType = 2
			}
			return data.NewConstr(
				0,
				data.NewConstr(constrType),
				// NOTE: Infinite bounds are always exclusive, by convention.
				toPlutusData(true),
			)
		}
	}
	return data.NewConstr(
		0,
		bound(t.lowerBound, true),
		bound(t.upperBound, false),
	)
}

func sortInputs(inputs []lcommon.TransactionInput) []lcommon.TransactionInput {
	ret := make([]lcommon.TransactionInput, len(inputs))
	copy(ret, inputs)
	slices.SortFunc(
		ret,
		func(a, b lcommon.TransactionInput) int {
			// Compare TX ID
			x := bytes.Compare(a.Id().Bytes(), b.Id().Bytes())
			if x != 0 {
				return x
			}
			// Compare index
			if a.Index() < b.Index() {
				return -1
			} else if a.Index() > b.Index() {
				return 1
			}
			return 0
		},
	)
	return ret
}

func expandInputs(
	inputs []lcommon.TransactionInput,
	resolvedInputs []lcommon.Utxo,
) []ResolvedInput {
	ret := make([]ResolvedInput, len(inputs))
	for i, input := range inputs {
		for _, resolvedInput := range resolvedInputs {
			if input.String() == resolvedInput.Id.String() {
				ret[i] = ResolvedInput(resolvedInput)
				break
			}
		}
	}
	return ret
}

func collapseOutputs(outputs []lcommon.Utxo) []lcommon.TransactionOutput {
	ret := make([]lcommon.TransactionOutput, len(outputs))
	for i, item := range outputs {
		ret[i] = item.Output
	}
	return ret
}

func sortedRedeemerKeys(
	redeemers lcommon.TransactionWitnessRedeemers,
) []lcommon.RedeemerKey {
	tags := []lcommon.RedeemerTag{
		lcommon.RedeemerTagSpend,
		lcommon.RedeemerTagMint,
		lcommon.RedeemerTagCert,
		lcommon.RedeemerTagReward,
		lcommon.RedeemerTagVoting,
		lcommon.RedeemerTagProposing,
	}
	ret := make([]lcommon.RedeemerKey, 0)
	for _, tag := range tags {
		idxs := redeemers.Indexes(tag)
		slices.Sort(idxs)
		for _, idx := range idxs {
			ret = append(
				ret,
				lcommon.RedeemerKey{
					Tag: tag,
					// nolint:gosec
					Index: uint32(idx),
				},
			)
		}
	}
	return ret
}

func dataInfo(
	witnessSet lcommon.TransactionWitnessSet,
) KeyValuePairs[lcommon.DatumHash, data.PlutusData] {
	var ret KeyValuePairs[lcommon.DatumHash, data.PlutusData]
	for _, datum := range witnessSet.PlutusData() {
		ret = append(
			ret,
			KeyValuePair[lcommon.DatumHash, data.PlutusData]{
				Key:   datum.Hash(),
				Value: datum.Data,
			},
		)
	}
	// Sort by datum hash
	slices.SortFunc(
		ret,
		func(a, b KeyValuePair[lcommon.DatumHash, data.PlutusData]) int {
			return bytes.Compare(a.Key.Bytes(), b.Key.Bytes())
		},
	)
	return ret
}

func redeemersInfo(
	witnessSet lcommon.TransactionWitnessSet,
	toScriptPurpose toScriptPurposeFunc,
) KeyValuePairs[ScriptInfo, Redeemer] {
	var ret KeyValuePairs[ScriptInfo, Redeemer]
	redeemers := witnessSet.Redeemers()
	redeemerKeys := sortedRedeemerKeys(redeemers)
	for _, key := range redeemerKeys {
		redeemerValue := redeemers.Value(uint(key.Index), key.Tag)
		purpose := toScriptPurpose(key)
		ret = append(
			ret,
			KeyValuePair[ScriptInfo, Redeemer]{
				Key: purpose,
				Value: Redeemer{
					Tag:     key.Tag,
					Index:   key.Index,
					Data:    redeemerValue.Data.Data,
					ExUnits: redeemerValue.ExUnits,
				},
			},
		)
	}
	return ret
}

func signatoriesInfo(
	requiredSigners []lcommon.Blake2b224,
) []lcommon.Blake2b224 {
	tmp := make([]lcommon.Blake2b224, len(requiredSigners))
	copy(tmp, requiredSigners)
	slices.SortFunc(
		tmp,
		func(a, b lcommon.Blake2b224) int {
			return bytes.Compare(a.Bytes(), b.Bytes())
		},
	)
	return tmp
}
