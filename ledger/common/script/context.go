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

package script

import (
	"bytes"
	"cmp"
	"fmt"
	"math/big"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
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
	// Deprecated: ProtocolMajor has no effect. The txInfoMint rendering matches
	// cardano-ledger at every protocol version, so nothing reads this field. It
	// is retained only so existing callers that assign it keep compiling, and
	// will be removed in a future release.
	ProtocolMajor uint
}

func (TxInfoV1) isTxInfo() {}

// mintWithZeroAda renders txInfoMint for PlutusV1/V2 as cardano-ledger does:
// transMintValue m = transCoinToValue zero <> transMultiAsset m. That prepends a
// zero-lovelace ada entry ({"":{"":0}}), even when nothing is minted.
//
// This applies at every protocol version. cardano-ledger's transMintValue takes
// no protocol version and has no branch, and Alonzo, Babbage and Conway all
// route their V1/V2 txInfo through that one function. Gating it on PV10 made
// every pre-Plomin script that inspects txInfoMint see a mint field one entry
// short, so its execution cost diverged from the node that produced the block.
//
// PlutusV3 differs: Conway defines its own transMintValue as
// PV3.UnsafeMintValue . PV1.getValue . transMultiAsset, dropping the ada entry
// because V3 uses the MintValue representation. TxInfoV3 therefore keeps
// t.Mint.ToPlutusData().
func mintWithZeroAda(
	mint lcommon.MultiAsset[lcommon.MultiAssetTypeMint],
) data.PlutusData {
	mintData := mint.ToPlutusData()
	mintMap, ok := mintData.(*data.Map)
	if !ok {
		return mintData
	}
	adaEntry := [2]data.PlutusData{
		data.NewByteString(nil),
		data.NewMap([][2]data.PlutusData{
			{data.NewByteString(nil), data.NewInteger(big.NewInt(0))},
		}),
	}
	pairs := make([][2]data.PlutusData, 0, 1+len(mintMap.Pairs))
	pairs = append(pairs, adaEntry)
	pairs = append(pairs, mintMap.Pairs...)
	return data.NewMap(pairs)
}

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
		mintWithZeroAda(t.Mint),
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
	validityRange, err := validityRangeInfo(slotState, tx)
	if err != nil {
		return TxInfoV1{}, err
	}
	assetMint := tx.AssetMint()
	if assetMint == nil {
		assetMint = &lcommon.MultiAsset[lcommon.MultiAssetTypeMint]{}
	}
	inputs := SortInputs(tx.Inputs())
	withdrawals := withdrawalsInfo(tx.Withdrawals())
	witnessDatums := buildWitnessDatums(tx.Witnesses())
	certs := tx.Certificates()
	redeemers, err := redeemersInfo(
		tx.Witnesses(),
		scriptPurposeBuilder(
			resolvedInputs,
			inputs,
			*assetMint,
			certs,
			withdrawals,
			nil,
			nil,
			witnessDatums,
		),
	)
	if err != nil {
		return TxInfoV1{}, err
	}
	tmpData := dataInfo(tx.Witnesses())
	ret := TxInfoV1{
		Inputs:       expandInputs(inputs, resolvedInputs),
		Outputs:      collapseOutputs(tx.Produced()),
		Fee:          tx.Fee(),
		Mint:         *assetMint,
		ValidRange:   validityRange,
		Certificates: certs,
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
	// Deprecated: ProtocolMajor has no effect. The txInfoMint rendering matches
	// cardano-ledger at every protocol version, so nothing reads this field. It
	// is retained only so existing callers that assign it keep compiling, and
	// will be removed in a future release.
	ProtocolMajor uint
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
		mintWithZeroAda(t.Mint),
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
	validityRange, err := validityRangeInfo(slotState, tx)
	if err != nil {
		return TxInfoV2{}, err
	}
	assetMint := tx.AssetMint()
	if assetMint == nil {
		assetMint = &lcommon.MultiAsset[lcommon.MultiAssetTypeMint]{}
	}
	inputs := SortInputs(tx.Inputs())
	withdrawals := withdrawalsInfo(tx.Withdrawals())
	witnessDatums := buildWitnessDatums(tx.Witnesses())
	certs := tx.Certificates()
	redeemers, err := redeemersInfo(
		tx.Witnesses(),
		scriptPurposeBuilder(
			resolvedInputs,
			inputs,
			*assetMint,
			certs,
			withdrawals,
			nil, // votes
			nil, // proposalProcedures
			witnessDatums,
		),
	)
	if err != nil {
		return TxInfoV2{}, err
	}
	tmpData := dataInfo(tx.Witnesses())
	ret := TxInfoV2{
		Inputs: expandInputs(inputs, resolvedInputs),
		ReferenceInputs: expandInputs(
			SortInputs(tx.ReferenceInputs()),
			resolvedInputs,
		),
		Outputs:      collapseOutputs(tx.Produced()),
		Fee:          tx.Fee(),
		Mint:         *assetMint,
		ValidRange:   validityRange,
		Certificates: certs,
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
	validityRange, err := validityRangeInfo(slotState, tx)
	if err != nil {
		return TxInfoV3{}, err
	}
	assetMint := tx.AssetMint()
	if assetMint == nil {
		assetMint = &lcommon.MultiAsset[lcommon.MultiAssetTypeMint]{}
	}
	inputs := SortInputs(tx.Inputs())
	withdrawals := withdrawalsInfo(tx.Withdrawals())
	votes := votingInfo(tx.VotingProcedures())
	proposalProcedures := tx.ProposalProcedures()
	witnessDatums := buildWitnessDatums(tx.Witnesses())
	redeemers, err := redeemersInfo(
		tx.Witnesses(),
		scriptPurposeBuilder(
			resolvedInputs,
			inputs,
			*assetMint,
			tx.Certificates(),
			withdrawals,
			votes,
			proposalProcedures,
			witnessDatums,
		),
	)
	if err != nil {
		return TxInfoV3{}, err
	}
	tmpData := dataInfo(tx.Witnesses())
	ret := TxInfoV3{
		Inputs: expandInputs(inputs, resolvedInputs),
		ReferenceInputs: expandInputs(
			SortInputs(tx.ReferenceInputs()),
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
	lowerBound        uint64
	upperBound        uint64
	lowerBoundPresent bool
	upperBoundPresent bool
}

func (t TimeRange) ToPlutusData() data.PlutusData {
	bound := func(
		value uint64,
		present bool,
		isLower bool,
		closed bool,
	) data.PlutusData {
		if present {
			return data.NewConstr(
				0,
				data.NewConstr(
					1,
					data.NewInteger(
						new(big.Int).SetUint64(value),
					),
				),
				toPlutusData(closed),
			)
		} else {
			var constrType uint64
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
		bound(
			t.lowerBound,
			t.lowerBoundPresent,
			true,
			true,
		),
		bound(
			t.upperBound,
			t.upperBoundPresent,
			false,
			// cardano-ledger uses `to` (closed upper) for an
			// upper-only interval, but `strictUpperBound` when
			// both bounds are present.
			!t.lowerBoundPresent,
		),
	)
}

// SortInputs returns a sorted copy of the given inputs, ordered by
// (TxId, Index) in ascending byte order. This matches the canonical
// ordering required by the Cardano ledger spec for redeemer index
// mapping.
func SortInputs(inputs []lcommon.TransactionInput) []lcommon.TransactionInput {
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
	ret := make([]lcommon.RedeemerKey, 0)
	for key := range redeemers.Iter() {
		ret = append(ret, key)
	}
	slices.SortFunc(ret, func(a, b lcommon.RedeemerKey) int {
		if a.Tag != b.Tag {
			return cmp.Compare(a.Tag, b.Tag)
		}
		return cmp.Compare(a.Index, b.Index)
	})
	return ret
}

func validityRangeInfo(
	slotState lcommon.SlotState,
	tx lcommon.Transaction,
) (TimeRange, error) {
	var ret TimeRange
	startSlot := tx.ValidityIntervalStart()
	endSlot, upperBoundPresent := lcommon.TransactionValidityIntervalUpperBound(
		tx,
	)
	ret.lowerBoundPresent = validityLowerBoundPresent(tx)
	ret.upperBoundPresent = upperBoundPresent
	if ret.lowerBoundPresent {
		startTime, err := slotState.SlotToTime(startSlot)
		if err != nil {
			return ret, err
		}
		ret.lowerBound = uint64(startTime.UnixMilli()) // nolint:gosec
	}
	if ret.upperBoundPresent {
		endTime, err := slotState.SlotToTime(endSlot)
		if err != nil {
			return ret, err
		}
		ret.upperBound = uint64(endTime.UnixMilli()) // nolint:gosec
	}
	return ret, nil
}

func validityLowerBoundPresent(tx lcommon.Transaction) bool {
	startPresent := tx.ValidityIntervalStart() > 0
	txCbor := tx.Cbor()
	if len(txCbor) == 0 {
		return startPresent
	}
	var txFields []cbor.RawMessage
	if _, err := cbor.Decode(txCbor, &txFields); err != nil ||
		len(txFields) == 0 {
		return startPresent
	}
	var bodyFields map[uint]cbor.RawMessage
	if _, err := cbor.Decode(txFields[0], &bodyFields); err != nil {
		return startPresent
	}
	_, startPresent = bodyFields[8]
	return startPresent
}

func withdrawalsInfo(
	withdrawals map[*lcommon.Address]*big.Int,
) KeyValuePairs[*lcommon.Address, *big.Int] {
	ret := make(KeyValuePairs[*lcommon.Address, *big.Int], 0, len(withdrawals))
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
	// Note: Bytes() errors are ignored here because Address.Bytes() only fails
	// for malformed Byron addresses during CBOR encoding. In practice, addresses
	// in valid transactions will always serialize successfully. If both fail,
	// bytes.Compare(nil, nil) returns 0, preserving original order for that pair.
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
	// Deduplicate by datum hash - same hash means same datum
	seen := make(map[lcommon.DatumHash]struct{})
	for _, datum := range witnessSet.PlutusData() {
		hash := datum.Hash()
		if _, found := seen[hash]; found {
			continue // Skip duplicates
		}
		seen[hash] = struct{}{}
		ret = append(
			ret,
			KeyValuePair[lcommon.DatumHash, data.PlutusData]{
				Key:   hash,
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

func buildWitnessDatums(
	witnessSet lcommon.TransactionWitnessSet,
) map[lcommon.Blake2b256]*lcommon.Datum {
	witnessDatums := make(map[lcommon.Blake2b256]*lcommon.Datum)
	if witnessSet == nil {
		return witnessDatums
	}
	plutusData := witnessSet.PlutusData()
	for i := range plutusData {
		datum := plutusData[i]
		witnessDatums[datum.Hash()] = &datum
	}
	return witnessDatums
}

func redeemersInfo(
	witnessSet lcommon.TransactionWitnessSet,
	toScriptPurpose toScriptPurposeFunc,
) (KeyValuePairs[ScriptPurpose, Redeemer], error) {
	if witnessSet == nil {
		return KeyValuePairs[ScriptPurpose, Redeemer]{}, nil
	}
	redeemers := witnessSet.Redeemers()
	if redeemers == nil {
		return KeyValuePairs[ScriptPurpose, Redeemer]{}, nil
	}
	redeemerKeys := sortedRedeemerKeys(redeemers)
	ret := make(KeyValuePairs[ScriptPurpose, Redeemer], 0, len(redeemerKeys))
	for _, key := range redeemerKeys {
		redeemerValue := redeemers.Value(uint(key.Index), key.Tag)
		purpose, err := toScriptPurpose(key)
		if err != nil {
			return nil, err
		}
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
	return ret, nil
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
	ret := make(KeyValuePairs[*lcommon.Voter, KeyValuePairs[*lcommon.GovActionId, lcommon.VotingProcedure]], 0, len(votingProcedures))
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
			Option[*big.Int]{Value: big.NewInt(c.Amount)}.ToPlutusData(),
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
			Option[*big.Int]{Value: big.NewInt(c.Amount)}.ToPlutusData(),
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
	default:
		panic(
			fmt.Sprintf(
				"unsupported certificate type: %T",
				c,
			),
		)
	}
}
