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
	TxInfo  TxInfo
	Purpose ScriptPurpose
}

func (ScriptContextV1V2) isScriptContext() {}

func (s ScriptContextV1V2) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		0,
		s.TxInfo.ToPlutusData(),
		WithWrappedTransactionId{
			s.Purpose,
		}.ToPlutusData(),
	)
}

func NewScriptContextV1V2(
	txInfo TxInfo,
	purpose ScriptPurpose,
) ScriptContext {
	return ScriptContextV1V2{
		TxInfo:  txInfo,
		Purpose: purpose,
	}
}

type ScriptContextV3 struct {
	TxInfo     TxInfo
	Redeemer   Redeemer
	ScriptInfo ScriptInfo
}

func (ScriptContextV3) isScriptContext() {}

func (s ScriptContextV3) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		0,
		s.TxInfo.ToPlutusData(),
		s.Redeemer.ToPlutusData(),
		s.ScriptInfo.ToPlutusData(),
	)
}

func NewScriptContextV3(
	txInfo TxInfo,
	redeemer Redeemer,
	purpose ScriptPurpose,
) ScriptContext {
	return ScriptContextV3{
		TxInfo:     txInfo,
		Redeemer:   redeemer,
		ScriptInfo: purpose.ToScriptInfo(),
	}
}

type TxInfo interface {
	isTxInfo()
	ToPlutusData() data.PlutusData
}

type TxInfoV1 struct {
	Inputs       []ResolvedInput
	Outputs      []lcommon.TransactionOutput
	Fee          *big.Int
	Mint         lcommon.MultiAsset[lcommon.MultiAssetTypeMint]
	Certificates []lcommon.Certificate
	Withdrawals  Pairs[*lcommon.Address, *big.Int]
	ValidRange   TimeRange
	Signatories  []lcommon.Blake2b224
	Data         KeyValuePairs[lcommon.Blake2b256, data.PlutusData]
	Redeemers    KeyValuePairs[ScriptPurpose, Redeemer]
	Id           lcommon.Blake2b256
}

func (TxInfoV1) isTxInfo() {}

func (t TxInfoV1) ToPlutusData() data.PlutusData {
	tmpDataItems := make([]data.PlutusData, len(t.Data))
	for i, item := range t.Data {
		tmpDataItems[i] = data.NewConstr(
			0,
			item.Key.ToPlutusData(),
			item.Value,
		)
	}
	return data.NewConstr(
		0,
		WithOptionDatum{
			WithZeroAdaAsset{
				WithWrappedTransactionId{
					t.Inputs,
				},
			},
		}.ToPlutusData(),
		WithOptionDatum{
			WithZeroAdaAsset{
				t.Outputs,
			},
		}.ToPlutusData(),
		WithZeroAdaAsset{
			Value{
				CoinBigInt: t.Fee,
			},
		}.ToPlutusData(),
		WithZeroAdaAsset{
			Value{
				AssetsMint: &t.Mint,
			},
		}.ToPlutusData(),
		WithPartialCertificates{
			t.Certificates,
		}.ToPlutusData(),
		WithWrappedStakeCredential{
			t.Withdrawals,
		}.ToPlutusData(),
		t.ValidRange.ToPlutusData(),
		toPlutusData(t.Signatories),
		data.NewList(
			tmpDataItems...,
		),
		data.NewConstr(
			0,
			data.NewByteString(t.Id.Bytes()),
		),
	)
}

func NewTxInfoV1FromTransaction(
	slotState lcommon.SlotState,
	tx lcommon.Transaction,
	resolvedInputs []lcommon.Utxo,
) (TxInfoV1, error) {
	validityRange, err := validityRangeInfo(
		slotState,
		tx.ValidityIntervalStart(),
		tx.TTL(),
	)
	if err != nil {
		return TxInfoV1{}, err
	}
	assetMint := tx.AssetMint()
	if assetMint == nil {
		assetMint = &lcommon.MultiAsset[lcommon.MultiAssetTypeMint]{}
	}
	inputs := sortInputs(tx.Inputs())
	withdrawals := withdrawalsInfo(tx.Withdrawals())
	redeemers := redeemersInfo(
		tx.Witnesses(),
		scriptPurposeBuilder(
			resolvedInputs,
			inputs,
			*assetMint,
			tx.Certificates(),
			withdrawals,
			nil,
			nil,
		),
	)
	tmpData := dataInfo(tx.Witnesses())
	ret := TxInfoV1{
		Inputs:       expandInputs(inputs, resolvedInputs),
		Outputs:      collapseOutputs(tx.Produced()),
		Fee:          tx.Fee(),
		Mint:         *assetMint,
		ValidRange:   validityRange,
		Certificates: tx.Certificates(),
		Withdrawals:  withdrawals.ToPairs(),
		Signatories:  signatoriesInfo(tx.RequiredSigners()),
		Redeemers:    redeemers,
		Data:         tmpData,
		Id:           tx.Id(),
	}
	return ret, nil
}

type TxInfoV2 struct {
	Inputs          []ResolvedInput
	ReferenceInputs []ResolvedInput
	Outputs         []lcommon.TransactionOutput
	Fee             *big.Int
	Mint            lcommon.MultiAsset[lcommon.MultiAssetTypeMint]
	Certificates    []lcommon.Certificate
	Withdrawals     KeyValuePairs[*lcommon.Address, *big.Int]
	ValidRange      TimeRange
	Signatories     []lcommon.Blake2b224
	Redeemers       KeyValuePairs[ScriptPurpose, Redeemer]
	Data            KeyValuePairs[lcommon.Blake2b256, data.PlutusData]
	Id              lcommon.Blake2b256
}

func (TxInfoV2) isTxInfo() {}

func (t TxInfoV2) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		0,
		WithZeroAdaAsset{
			WithWrappedTransactionId{
				t.Inputs,
			},
		}.ToPlutusData(),
		WithZeroAdaAsset{
			WithWrappedTransactionId{
				t.ReferenceInputs,
			},
		}.ToPlutusData(),
		WithZeroAdaAsset{
			t.Outputs,
		}.ToPlutusData(),
		WithZeroAdaAsset{
			Value{
				CoinBigInt: t.Fee,
			},
		}.ToPlutusData(),
		WithZeroAdaAsset{
			Value{
				AssetsMint: &t.Mint,
			},
		}.ToPlutusData(),
		WithPartialCertificates{
			t.Certificates,
		}.ToPlutusData(),
		WithWrappedStakeCredential{
			t.Withdrawals,
		}.ToPlutusData(),
		t.ValidRange.ToPlutusData(),
		toPlutusData(t.Signatories),
		WithWrappedTransactionId{
			t.Redeemers,
		}.ToPlutusData(),
		t.Data.ToPlutusData(),
		data.NewConstr(
			0,
			data.NewByteString(t.Id.Bytes()),
		),
	)
}

func NewTxInfoV2FromTransaction(
	slotState lcommon.SlotState,
	tx lcommon.Transaction,
	resolvedInputs []lcommon.Utxo,
) (TxInfoV2, error) {
	validityRange, err := validityRangeInfo(
		slotState,
		tx.ValidityIntervalStart(),
		tx.TTL(),
	)
	if err != nil {
		return TxInfoV2{}, err
	}
	assetMint := tx.AssetMint()
	if assetMint == nil {
		assetMint = &lcommon.MultiAsset[lcommon.MultiAssetTypeMint]{}
	}
	inputs := sortInputs(tx.Inputs())
	withdrawals := withdrawalsInfo(tx.Withdrawals())
	redeemers := redeemersInfo(
		tx.Witnesses(),
		scriptPurposeBuilder(
			resolvedInputs,
			inputs,
			*assetMint,
			tx.Certificates(),
			withdrawals,
			nil, // votes
			nil, // proposalProcedures,
		),
	)
	tmpData := dataInfo(tx.Witnesses())
	ret := TxInfoV2{
		Inputs: expandInputs(inputs, resolvedInputs),
		ReferenceInputs: expandInputs(
			sortInputs(tx.ReferenceInputs()),
			resolvedInputs,
		),
		Outputs:      collapseOutputs(tx.Produced()),
		Fee:          tx.Fee(),
		Mint:         *assetMint,
		ValidRange:   validityRange,
		Certificates: tx.Certificates(),
		Withdrawals:  withdrawals,
		Signatories:  signatoriesInfo(tx.RequiredSigners()),
		Redeemers:    redeemers,
		Data:         tmpData,
		Id:           tx.Id(),
	}
	return ret, nil
}

type TxInfoV3 struct {
	Inputs                []ResolvedInput
	ReferenceInputs       []ResolvedInput
	Outputs               []lcommon.TransactionOutput
	Fee                   *big.Int
	Mint                  lcommon.MultiAsset[lcommon.MultiAssetTypeMint]
	Certificates          []lcommon.Certificate
	Withdrawals           KeyValuePairs[*lcommon.Address, *big.Int]
	ValidRange            TimeRange
	Signatories           []lcommon.Blake2b224
	Redeemers             KeyValuePairs[ScriptPurpose, Redeemer]
	Data                  KeyValuePairs[lcommon.Blake2b256, data.PlutusData]
	Id                    lcommon.Blake2b256
	Votes                 KeyValuePairs[*lcommon.Voter, KeyValuePairs[*lcommon.GovActionId, lcommon.VotingProcedure]]
	ProposalProcedures    []lcommon.ProposalProcedure
	CurrentTreasuryAmount Option[*big.Int]
	TreasuryDonation      Option[*big.Int]
}

func (TxInfoV3) isTxInfo() {}

func (t TxInfoV3) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		0,
		toPlutusData(t.Inputs),
		toPlutusData(t.ReferenceInputs),
		toPlutusData(t.Outputs),
		toPlutusData(t.Fee),
		t.Mint.ToPlutusData(),
		certificatesToPlutusData(t.Certificates),
		toPlutusData(t.Withdrawals),
		t.ValidRange.ToPlutusData(),
		toPlutusData(t.Signatories),
		t.Redeemers.ToPlutusData(),
		t.Data.ToPlutusData(),
		data.NewByteString(t.Id.Bytes()),
		t.Votes.ToPlutusData(),
		toPlutusData(t.ProposalProcedures),
		t.CurrentTreasuryAmount.ToPlutusData(),
		t.TreasuryDonation.ToPlutusData(),
	)
}

func NewTxInfoV3FromTransaction(
	slotState lcommon.SlotState,
	tx lcommon.Transaction,
	resolvedInputs []lcommon.Utxo,
) (TxInfoV3, error) {
	validityRange, err := validityRangeInfo(
		slotState,
		tx.ValidityIntervalStart(),
		tx.TTL(),
	)
	if err != nil {
		return TxInfoV3{}, err
	}
	assetMint := tx.AssetMint()
	if assetMint == nil {
		assetMint = &lcommon.MultiAsset[lcommon.MultiAssetTypeMint]{}
	}
	inputs := sortInputs(tx.Inputs())
	withdrawals := withdrawalsInfo(tx.Withdrawals())
	votes := votingInfo(tx.VotingProcedures())
	proposalProcedures := tx.ProposalProcedures()
	redeemers := redeemersInfo(
		tx.Witnesses(),
		scriptPurposeBuilder(
			resolvedInputs,
			inputs,
			*assetMint,
			tx.Certificates(),
			withdrawals,
			votes,
			proposalProcedures,
		),
	)
	tmpData := dataInfo(tx.Witnesses())
	ret := TxInfoV3{
		Inputs: expandInputs(inputs, resolvedInputs),
		ReferenceInputs: expandInputs(
			sortInputs(tx.ReferenceInputs()),
			resolvedInputs,
		),
		Outputs:            collapseOutputs(tx.Produced()),
		Fee:                tx.Fee(),
		Mint:               *assetMint,
		ValidRange:         validityRange,
		Certificates:       tx.Certificates(),
		Withdrawals:        withdrawals,
		Signatories:        signatoriesInfo(tx.RequiredSigners()),
		Redeemers:          redeemers,
		Data:               tmpData,
		Id:                 tx.Id(),
		Votes:              votes,
		ProposalProcedures: proposalProcedures,
	}
	if amt := tx.CurrentTreasuryValue(); amt != nil && amt.Sign() > 0 {
		ret.CurrentTreasuryAmount.Value = amt
	}
	if amt := tx.Donation(); amt != nil && amt.Sign() > 0 {
		ret.TreasuryDonation.Value = amt
	}
	return ret, nil
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
	if redeemers == nil {
		return []lcommon.RedeemerKey{}
	}
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

func validityRangeInfo(
	slotState lcommon.SlotState,
	startSlot uint64,
	endSlot uint64,
) (TimeRange, error) {
	var ret TimeRange
	if startSlot > 0 {
		startTime, err := slotState.SlotToTime(startSlot)
		if err != nil {
			return ret, err
		}
		ret.lowerBound = uint64(startTime.UnixMilli()) // nolint:gosec
	}
	if endSlot > 0 {
		endTime, err := slotState.SlotToTime(endSlot)
		if err != nil {
			return ret, err
		}
		ret.upperBound = uint64(endTime.UnixMilli()) // nolint:gosec
	}
	return ret, nil
}

func withdrawalsInfo(
	withdrawals map[*lcommon.Address]*big.Int,
) KeyValuePairs[*lcommon.Address, *big.Int] {
	var ret KeyValuePairs[*lcommon.Address, *big.Int]
	for addr, amt := range withdrawals {
		ret = append(
			ret,
			KeyValuePair[*lcommon.Address, *big.Int]{
				Key:   addr,
				Value: amt,
			},
		)
	}
	// Sort by address bytes
	slices.SortFunc(
		ret,
		func(a, b KeyValuePair[*lcommon.Address, *big.Int]) int {
			aBytes, _ := a.Key.Bytes()
			bBytes, _ := b.Key.Bytes()
			return bytes.Compare(aBytes, bBytes)
		},
	)
	return ret
}

func dataInfo(
	witnessSet lcommon.TransactionWitnessSet,
) KeyValuePairs[lcommon.DatumHash, data.PlutusData] {
	var ret KeyValuePairs[lcommon.DatumHash, data.PlutusData]
	if witnessSet == nil {
		return ret
	}
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
) KeyValuePairs[ScriptPurpose, Redeemer] {
	if witnessSet == nil {
		return KeyValuePairs[ScriptPurpose, Redeemer]{}
	}
	redeemers := witnessSet.Redeemers()
	if redeemers == nil {
		return KeyValuePairs[ScriptPurpose, Redeemer]{}
	}
	redeemerKeys := sortedRedeemerKeys(redeemers)
	ret := make(KeyValuePairs[ScriptPurpose, Redeemer], 0, len(redeemerKeys))
	for _, key := range redeemerKeys {
		redeemerValue := redeemers.Value(uint(key.Index), key.Tag)
		purpose := toScriptPurpose(key)
		ret = append(
			ret,
			KeyValuePair[ScriptPurpose, Redeemer]{
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

func votingInfo(
	votingProcedures lcommon.VotingProcedures,
) KeyValuePairs[*lcommon.Voter, KeyValuePairs[*lcommon.GovActionId, lcommon.VotingProcedure]] {
	var ret KeyValuePairs[*lcommon.Voter, KeyValuePairs[*lcommon.GovActionId, lcommon.VotingProcedure]]
	for voter, voterData := range votingProcedures {
		voterPairs := make(
			KeyValuePairs[*lcommon.GovActionId, lcommon.VotingProcedure],
			0,
			len(votingProcedures),
		)
		for govActionId, votingProcedure := range voterData {
			voterPairs = append(
				voterPairs,
				KeyValuePair[*lcommon.GovActionId, lcommon.VotingProcedure]{
					Key:   govActionId,
					Value: votingProcedure,
				},
			)
		}
		// Sort voter pairs by gov action ID
		slices.SortFunc(
			voterPairs,
			func(a, b KeyValuePair[*lcommon.GovActionId, lcommon.VotingProcedure]) int {
				// Compare TX ID
				x := bytes.Compare(
					a.Key.TransactionId[:],
					b.Key.TransactionId[:],
				)
				if x != 0 {
					return x
				}
				// Compare index
				if a.Key.GovActionIdx < b.Key.GovActionIdx {
					return -1
				} else if a.Key.GovActionIdx > b.Key.GovActionIdx {
					return 1
				}
				return 0
			},
		)
		ret = append(
			ret,
			KeyValuePair[*lcommon.Voter, KeyValuePairs[*lcommon.GovActionId, lcommon.VotingProcedure]]{
				Key:   voter,
				Value: voterPairs,
			},
		)
	}
	// Sort by voter ID
	slices.SortFunc(
		ret,
		func(a, b KeyValuePair[*lcommon.Voter, KeyValuePairs[*lcommon.GovActionId, lcommon.VotingProcedure]]) int {
			voterTag := func(v *lcommon.Voter) int {
				switch v.Type {
				case lcommon.VoterTypeConstitutionalCommitteeHotScriptHash:
					return 0
				case lcommon.VoterTypeConstitutionalCommitteeHotKeyHash:
					return 1
				case lcommon.VoterTypeDRepScriptHash:
					return 2
				case lcommon.VoterTypeDRepKeyHash:
					return 3
				case lcommon.VoterTypeStakingPoolKeyHash:
					return 4
				}
				return -1
			}
			tagA := voterTag(a.Key)
			tagB := voterTag(b.Key)
			if tagA == tagB {
				return bytes.Compare(a.Key.Hash[:], b.Key.Hash[:])
			}
			if tagA < tagB {
				return -1
			}
			return 1
		},
	)
	return ret
}

func certificatesToPlutusData(
	certificates []lcommon.Certificate,
) data.PlutusData {
	tmpCerts := make([]data.PlutusData, len(certificates))
	for idx, cert := range certificates {
		tmpCerts[idx] = certificateToPlutusData(cert)
	}
	return data.NewList(tmpCerts...)
}

func certificateToPlutusData(
	certificate lcommon.Certificate,
) data.PlutusData {
	switch c := certificate.(type) {
	case *lcommon.StakeRegistrationCertificate:
		return data.NewConstr(
			0,
			c.StakeCredential.ToPlutusData(),
			data.NewConstr(1),
		)
	case *lcommon.RegistrationCertificate:
		return data.NewConstr(
			0,
			c.StakeCredential.ToPlutusData(),
			data.NewConstr(1),
		)
	case *lcommon.StakeDeregistrationCertificate:
		return data.NewConstr(
			1,
			c.StakeCredential.ToPlutusData(),
			data.NewConstr(1),
		)
	case *lcommon.DeregistrationCertificate:
		return data.NewConstr(
			1,
			c.StakeCredential.ToPlutusData(),
			data.NewConstr(1),
		)
	case *lcommon.StakeDelegationCertificate:
		return data.NewConstr(
			2,
			c.StakeCredential.ToPlutusData(),
			data.NewConstr(
				0,
				c.PoolKeyHash.ToPlutusData(),
			),
		)
	case *lcommon.VoteDelegationCertificate:
		return data.NewConstr(
			2,
			c.StakeCredential.ToPlutusData(),
			data.NewConstr(
				1,
				c.Drep.ToPlutusData(),
			),
		)
	case *lcommon.StakeVoteDelegationCertificate:
		return data.NewConstr(
			2,
			c.StakeCredential.ToPlutusData(),
			data.NewConstr(
				2,
				toPlutusData(c.PoolKeyHash),
				c.Drep.ToPlutusData(),
			),
		)
	case *lcommon.StakeRegistrationDelegationCertificate:
		return data.NewConstr(
			3,
			c.StakeCredential.ToPlutusData(),
			data.NewConstr(
				0,
				toPlutusData(c.PoolKeyHash),
			),
			data.NewInteger(big.NewInt(c.Amount)),
		)
	case *lcommon.VoteRegistrationDelegationCertificate:
		return data.NewConstr(
			3,
			c.StakeCredential.ToPlutusData(),
			data.NewConstr(
				1,
				c.Drep.ToPlutusData(),
			),
			data.NewInteger(big.NewInt(c.Amount)),
		)
	case *lcommon.StakeVoteRegistrationDelegationCertificate:
		return data.NewConstr(
			3,
			c.StakeCredential.ToPlutusData(),
			data.NewConstr(
				2,
				c.PoolKeyHash.ToPlutusData(),
				c.Drep.ToPlutusData(),
			),
			data.NewInteger(big.NewInt(c.Amount)),
		)
	case *lcommon.RegistrationDrepCertificate:
		return data.NewConstr(
			4,
			c.DrepCredential.ToPlutusData(),
			data.NewInteger(big.NewInt(c.Amount)),
		)
	case *lcommon.UpdateDrepCertificate:
		return data.NewConstr(
			5,
			c.DrepCredential.ToPlutusData(),
		)
	case *lcommon.DeregistrationDrepCertificate:
		return data.NewConstr(
			6,
			c.DrepCredential.ToPlutusData(),
			data.NewInteger(big.NewInt(c.Amount)),
		)
	case *lcommon.PoolRegistrationCertificate:
		return data.NewConstr(
			7,
			toPlutusData(c.Operator),
			toPlutusData(c.VrfKeyHash),
		)
	case *lcommon.PoolRetirementCertificate:
		return data.NewConstr(
			8,
			toPlutusData(c.PoolKeyHash),
			data.NewInteger(new(big.Int).SetUint64(c.Epoch)),
		)
	case *lcommon.AuthCommitteeHotCertificate:
		return data.NewConstr(
			9,
			c.ColdCredential.ToPlutusData(),
			c.HotCredential.ToPlutusData(),
		)
	case *lcommon.ResignCommitteeColdCertificate:
		return data.NewConstr(
			10,
			c.ColdCredential.ToPlutusData(),
		)
	}
	return nil
}
