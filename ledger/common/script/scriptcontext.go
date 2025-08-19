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
	"math/big"
	"slices"
	"strings"

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
		// TODO: signatories
		toPlutusData([]any{}),
		t.Redeemers.ToPlutusData(),
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
	inputs := sortInputs(tx.Consumed())
	redeemers := redeemersInfo(
		tx.Witnesses(),
		scriptPurposeBuilder(
			inputs,
			*assetMint,
			// TODO: certificates
			tx.Withdrawals(),
			// TODO: proposal procedures
			// TODO: votes
		),
	)
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
		// TODO: Signatories
		Redeemers: redeemers,
		// TODO: Data
		Id: tx.Hash(),
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

type ScriptInfo interface {
	isScriptInfo()
	ToPlutusData
}

type ScriptInfoMinting struct {
	PolicyId lcommon.Blake2b224
}

func (ScriptInfoMinting) isScriptInfo() {}

func (s ScriptInfoMinting) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		0,
		data.NewByteString(s.PolicyId.Bytes()),
	)
}

type ScriptInfoSpending struct {
	Input lcommon.TransactionInput
	Datum data.PlutusData
}

func (ScriptInfoSpending) isScriptInfo() {}

func (s ScriptInfoSpending) ToPlutusData() data.PlutusData {
	if s.Datum == nil {
		return data.NewConstr(
			1,
			s.Input.ToPlutusData(),
		)
	}
	return data.NewConstr(
		1,
		s.Input.ToPlutusData(),
		data.NewConstr(
			0,
			s.Datum,
		),
	)
}

type ScriptInfoRewarding struct {
	StakeCredential lcommon.Credential
}

func (ScriptInfoRewarding) isScriptInfo() {}

func (s ScriptInfoRewarding) ToPlutusData() data.PlutusData {
	// TODO
	return nil
}

type ScriptInfoCertifying struct {
	Size        uint64
	Certificate lcommon.Certificate
}

func (ScriptInfoCertifying) isScriptInfo() {}

func (s ScriptInfoCertifying) ToPlutusData() data.PlutusData {
	// TODO
	return nil
}

type ScriptInfoVoting struct {
	Voter lcommon.Voter
}

func (ScriptInfoVoting) isScriptInfo() {}

func (s ScriptInfoVoting) ToPlutusData() data.PlutusData {
	// TODO
	return nil
}

type ScriptInfoProposing struct {
	Size              uint64
	ProposalProcedure lcommon.ProposalProcedure
}

func (ScriptInfoProposing) isScriptInfo() {}

func (s ScriptInfoProposing) ToPlutusData() data.PlutusData {
	// TODO
	return nil
}

func sortInputs(inputs []lcommon.TransactionInput) []lcommon.TransactionInput {
	ret := make([]lcommon.TransactionInput, len(inputs))
	copy(ret, inputs)
	slices.SortFunc(
		ret,
		func(a, b lcommon.TransactionInput) int {
			// Compare TX ID
			x := strings.Compare(a.Id().String(), b.Id().String())
			if x != 0 {
				return x
			}
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

func redeemersInfo(
	witnessSet lcommon.TransactionWitnessSet,
	toScriptPurpose toScriptPurposeFunc,
) KeyValuePairs[ScriptInfo, Redeemer] {
	var ret KeyValuePairs[ScriptInfo, Redeemer]
	redeemers := witnessSet.Redeemers()
	redeemerKeys := sortedRedeemerKeys(redeemers)
	for _, key := range redeemerKeys {
		redeemerValue := redeemers.Value(uint(key.Index), key.Tag)
		datum := redeemerValue.Data.Data
		purpose := toScriptPurpose(key, datum)
		ret = append(
			ret,
			KeyValuePair[ScriptInfo, Redeemer]{
				Key: purpose,
				Value: Redeemer{
					Tag:     key.Tag,
					Index:   key.Index,
					Data:    datum,
					ExUnits: redeemerValue.ExUnits,
				},
			},
		)
	}
	return ret
}

type toScriptPurposeFunc func(lcommon.RedeemerKey, data.PlutusData) ScriptInfo

// scriptPurposeBuilder creates a reusable function preloaded with information about a particular transaction
func scriptPurposeBuilder(
	inputs []lcommon.TransactionInput,
	mint lcommon.MultiAsset[lcommon.MultiAssetTypeMint],
	// TODO: certificates
	withdrawals map[*lcommon.Address]uint64,
	// TODO: proposal procedures
	// TODO: votes
) toScriptPurposeFunc {
	return func(redeemerKey lcommon.RedeemerKey, datum data.PlutusData) ScriptInfo {
		// TODO: implement additional redeemer tags
		// https://github.com/aiken-lang/aiken/blob/af4e04b91e54dbba3340de03fc9e65a90f24a93b/crates/uplc/src/tx/script_context.rs#L771-L826
		switch redeemerKey.Tag {
		case lcommon.RedeemerTagSpend:
			return ScriptInfoSpending{
				Input: inputs[redeemerKey.Index],
				Datum: datum,
			}
		case lcommon.RedeemerTagMint:
			// TODO: fix this to work for more than one minted policy
			mintPolicies := mint.Policies()
			return ScriptInfoMinting{
				PolicyId: mintPolicies[0],
			}
		case lcommon.RedeemerTagCert:
			return nil
		case lcommon.RedeemerTagReward:
			return nil
		case lcommon.RedeemerTagVoting:
			return nil
		case lcommon.RedeemerTagProposing:
			return nil
		}
		return nil
	}
}
