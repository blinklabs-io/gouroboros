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
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"

	"github.com/blinklabs-io/plutigo/data"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

// TransactionToUtxorpc converts a full transaction, adding what a body alone
// cannot supply: the witness set, auxiliary data, the redeemers attached to
// the items they apply to, and the success flag from the validity marker.
//
// Redeemers for voting and proposing purposes are not attached: the UTxO-RPC
// messages for votes and proposals carry no redeemer field.
func TransactionToUtxorpc(tx Transaction) (*utxorpc.Tx, error) {
	ret, err := TransactionBodyToUtxorpc(tx)
	if err != nil {
		return nil, err
	}
	ret.Successful = tx.IsValid()
	witnesses, err := witnessSetToUtxorpc(tx.Witnesses())
	if err != nil {
		return nil, fmt.Errorf("witness set: %w", err)
	}
	ret.Witnesses = witnesses
	aux, err := auxDataToUtxorpc(tx.Metadata(), tx.AuxiliaryData())
	if err != nil {
		return nil, fmt.Errorf("auxiliary data: %w", err)
	}
	ret.Auxiliary = aux
	if err := attachRedeemers(tx, ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func witnessSetToUtxorpc(
	ws TransactionWitnessSet,
) (*utxorpc.WitnessSet, error) {
	if ws == nil {
		return nil, nil
	}
	ret := &utxorpc.WitnessSet{}
	for _, w := range ws.Vkey() {
		ret.Vkeywitness = append(
			ret.Vkeywitness,
			&utxorpc.VKeyWitness{Vkey: w.Vkey, Signature: w.Signature},
		)
	}
	var scripts []Script
	for _, s := range ws.NativeScripts() {
		scripts = append(scripts, s)
	}
	for _, s := range ws.PlutusV1Scripts() {
		scripts = append(scripts, s)
	}
	for _, s := range ws.PlutusV2Scripts() {
		scripts = append(scripts, s)
	}
	for _, s := range ws.PlutusV3Scripts() {
		scripts = append(scripts, s)
	}
	if v4, ok := ws.(TransactionWitnessSetWithPlutusV4); ok {
		for _, s := range v4.PlutusV4Scripts() {
			scripts = append(scripts, s)
		}
	}
	for _, s := range scripts {
		script, err := scriptToUtxorpc(s)
		if err != nil {
			return nil, err
		}
		ret.Script = append(ret.Script, script)
	}
	for _, d := range ws.PlutusData() {
		datum, err := plutusDataToUtxorpc(d.Data)
		if err != nil {
			return nil, err
		}
		ret.PlutusDatums = append(ret.PlutusDatums, datum)
	}
	return ret, nil
}

func scriptToUtxorpc(script Script) (*utxorpc.Script, error) {
	switch s := script.(type) {
	case NativeScript:
		native, err := nativeScriptToUtxorpc(s.Item())
		if err != nil {
			return nil, err
		}
		return &utxorpc.Script{
			Script: &utxorpc.Script_Native{Native: native},
		}, nil
	case PlutusV1Script:
		return &utxorpc.Script{
			Script: &utxorpc.Script_PlutusV1{PlutusV1: s.RawScriptBytes()},
		}, nil
	case PlutusV2Script:
		return &utxorpc.Script{
			Script: &utxorpc.Script_PlutusV2{PlutusV2: s.RawScriptBytes()},
		}, nil
	case PlutusV3Script:
		return &utxorpc.Script{
			Script: &utxorpc.Script_PlutusV3{PlutusV3: s.RawScriptBytes()},
		}, nil
	case PlutusV4Script:
		return &utxorpc.Script{
			Script: &utxorpc.Script_PlutusV4{PlutusV4: s.RawScriptBytes()},
		}, nil
	}
	return nil, fmt.Errorf("unsupported script type %T", script)
}

func nativeScriptsToUtxorpc(
	scripts []NativeScript,
) ([]*utxorpc.NativeScript, error) {
	ret := make([]*utxorpc.NativeScript, 0, len(scripts))
	for _, s := range scripts {
		native, err := nativeScriptToUtxorpc(s.Item())
		if err != nil {
			return nil, err
		}
		ret = append(ret, native)
	}
	return ret, nil
}

func nativeScriptToUtxorpc(item any) (*utxorpc.NativeScript, error) {
	ret := &utxorpc.NativeScript{}
	switch s := item.(type) {
	case *NativeScriptPubkey:
		ret.NativeScript = &utxorpc.NativeScript_ScriptPubkey{
			ScriptPubkey: s.Hash,
		}
	case *NativeScriptAll:
		items, err := nativeScriptsToUtxorpc(s.Scripts)
		if err != nil {
			return nil, err
		}
		ret.NativeScript = &utxorpc.NativeScript_ScriptAll{
			ScriptAll: &utxorpc.NativeScriptList{Items: items},
		}
	case *NativeScriptAny:
		items, err := nativeScriptsToUtxorpc(s.Scripts)
		if err != nil {
			return nil, err
		}
		ret.NativeScript = &utxorpc.NativeScript_ScriptAny{
			ScriptAny: &utxorpc.NativeScriptList{Items: items},
		}
	case *NativeScriptNofK:
		// The wire type is int64 and the UTxO-RPC field is uint32. A threshold
		// of zero or less is always met and one above uint32 never is, as no
		// transaction can carry that many scripts, so clamping to the field's
		// range preserves the script's meaning.
		items, err := nativeScriptsToUtxorpc(s.Scripts)
		if err != nil {
			return nil, err
		}
		ret.NativeScript = &utxorpc.NativeScript_ScriptNOfK{
			ScriptNOfK: &utxorpc.ScriptNOfK{
				K:       uint32(min(max(s.N, 0), math.MaxUint32)), // #nosec G115 -- clamped
				Scripts: items,
			},
		}
	case *NativeScriptInvalidBefore:
		ret.NativeScript = &utxorpc.NativeScript_InvalidBefore{
			InvalidBefore: s.Slot,
		}
	case *NativeScriptInvalidHereafter:
		ret.NativeScript = &utxorpc.NativeScript_InvalidHereafter{
			InvalidHereafter: s.Slot,
		}
	default:
		return nil, fmt.Errorf("unsupported native script %T", item)
	}
	return ret, nil
}

// plutusConstrTag maps a constructor index to the CBOR tag UTxO-RPC carries
// in Constr.Tag, plus the explicit index used by the general form (tag 102).
func plutusConstrTag(
	index *big.Int,
) (tag uint32, anyConstructor uint64, err error) {
	switch {
	case index.Sign() < 0 || !index.IsUint64():
		return 0, 0, fmt.Errorf("invalid constructor index %s", index)
	case index.Uint64() <= 6:
		return 121 + uint32(index.Uint64()), 0, nil // #nosec G115
	case index.Uint64() <= 127:
		return 1280 + uint32(index.Uint64()) - 7, 0, nil // #nosec G115
	}
	return 102, index.Uint64(), nil
}

func plutusDataToUtxorpc(pd data.PlutusData) (*utxorpc.PlutusData, error) {
	switch v := pd.(type) {
	case *data.Constr:
		tag, anyConstructor, err := plutusConstrTag(v.Tag)
		if err != nil {
			return nil, err
		}
		fields, err := plutusDataListToUtxorpc(v.Fields)
		if err != nil {
			return nil, err
		}
		return &utxorpc.PlutusData{
			PlutusData: &utxorpc.PlutusData_Constr{
				Constr: &utxorpc.Constr{
					Tag:            tag,
					AnyConstructor: anyConstructor,
					Fields:         fields,
				},
			},
		}, nil
	case *data.Map:
		pairs := make([]*utxorpc.PlutusDataPair, 0, len(v.Pairs))
		for _, pair := range v.Pairs {
			key, err := plutusDataToUtxorpc(pair[0])
			if err != nil {
				return nil, err
			}
			value, err := plutusDataToUtxorpc(pair[1])
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, &utxorpc.PlutusDataPair{Key: key, Value: value})
		}
		return &utxorpc.PlutusData{
			PlutusData: &utxorpc.PlutusData_Map{
				Map: &utxorpc.PlutusDataMap{Pairs: pairs},
			},
		}, nil
	case *data.Integer:
		return &utxorpc.PlutusData{
			PlutusData: &utxorpc.PlutusData_BigInt{
				BigInt: BigIntToUtxorpcBigInt(v.Inner),
			},
		}, nil
	case *data.ByteString:
		return &utxorpc.PlutusData{
			PlutusData: &utxorpc.PlutusData_BoundedBytes{
				BoundedBytes: v.Inner,
			},
		}, nil
	case *data.List:
		items, err := plutusDataListToUtxorpc(v.Items)
		if err != nil {
			return nil, err
		}
		return &utxorpc.PlutusData{
			PlutusData: &utxorpc.PlutusData_Array{
				Array: &utxorpc.PlutusDataArray{Items: items},
			},
		}, nil
	}
	return nil, fmt.Errorf("unsupported plutus data %T", pd)
}

func plutusDataListToUtxorpc(
	items []data.PlutusData,
) ([]*utxorpc.PlutusData, error) {
	ret := make([]*utxorpc.PlutusData, 0, len(items))
	for _, item := range items {
		converted, err := plutusDataToUtxorpc(item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, converted)
	}
	return ret, nil
}

func auxDataToUtxorpc(
	metadata TransactionMetadatum,
	aux AuxiliaryData,
) (*utxorpc.AuxData, error) {
	ret := &utxorpc.AuxData{}
	if metadata != nil {
		m, ok := metadata.(MetaMap)
		if !ok {
			return nil, fmt.Errorf("unexpected metadata root %T", metadata)
		}
		for _, pair := range m.Pairs {
			label, ok := pair.Key.(MetaInt)
			if !ok || label.Value == nil || !label.Value.IsUint64() {
				return nil, errors.New("metadata label is not a uint64")
			}
			value, err := metadatumToUtxorpc(pair.Value)
			if err != nil {
				return nil, err
			}
			ret.Metadata = append(
				ret.Metadata,
				&utxorpc.Metadata{Label: label.Value.Uint64(), Value: value},
			)
		}
	}
	if aux != nil {
		var scripts []Script
		native, err := aux.NativeScripts()
		if err != nil {
			return nil, err
		}
		for _, s := range native {
			scripts = append(scripts, s)
		}
		v1, err := aux.PlutusV1Scripts()
		if err != nil {
			return nil, err
		}
		for _, s := range v1 {
			scripts = append(scripts, s)
		}
		v2, err := aux.PlutusV2Scripts()
		if err != nil {
			return nil, err
		}
		for _, s := range v2 {
			scripts = append(scripts, s)
		}
		v3, err := aux.PlutusV3Scripts()
		if err != nil {
			return nil, err
		}
		for _, s := range v3 {
			scripts = append(scripts, s)
		}
		v4, err := aux.PlutusV4Scripts()
		if err != nil {
			return nil, err
		}
		for _, s := range v4 {
			scripts = append(scripts, s)
		}
		for _, s := range scripts {
			script, err := scriptToUtxorpc(s)
			if err != nil {
				return nil, err
			}
			ret.Scripts = append(ret.Scripts, script)
		}
	}
	if len(ret.GetMetadata()) == 0 && len(ret.GetScripts()) == 0 {
		return nil, nil
	}
	return ret, nil
}

func metadatumToUtxorpc(m TransactionMetadatum) (*utxorpc.Metadatum, error) {
	switch v := m.(type) {
	case MetaInt:
		if v.Value == nil {
			return nil, errors.New("metadatum integer is unset")
		}
		return &utxorpc.Metadatum{
			Metadatum: &utxorpc.Metadatum_Int{Int: metadatumInt64(v.Value)},
		}, nil
	case MetaBytes:
		return &utxorpc.Metadatum{
			Metadatum: &utxorpc.Metadatum_Bytes{Bytes: v.Value},
		}, nil
	case MetaText:
		return &utxorpc.Metadatum{
			Metadatum: &utxorpc.Metadatum_Text{Text: v.Value},
		}, nil
	case MetaList:
		items := make([]*utxorpc.Metadatum, 0, len(v.Items))
		for _, item := range v.Items {
			converted, err := metadatumToUtxorpc(item)
			if err != nil {
				return nil, err
			}
			items = append(items, converted)
		}
		return &utxorpc.Metadatum{
			Metadatum: &utxorpc.Metadatum_Array{
				Array: &utxorpc.MetadatumArray{Items: items},
			},
		}, nil
	case MetaMap:
		pairs := make([]*utxorpc.MetadatumPair, 0, len(v.Pairs))
		for _, pair := range v.Pairs {
			key, err := metadatumToUtxorpc(pair.Key)
			if err != nil {
				return nil, err
			}
			value, err := metadatumToUtxorpc(pair.Value)
			if err != nil {
				return nil, err
			}
			pairs = append(
				pairs,
				&utxorpc.MetadatumPair{Key: key, Value: value},
			)
		}
		return &utxorpc.Metadatum{
			Metadatum: &utxorpc.Metadatum_Map{
				Map: &utxorpc.MetadatumMap{Pairs: pairs},
			},
		}, nil
	}
	return nil, fmt.Errorf("unsupported metadatum %T", m)
}

// metadatumInt64 carries a metadatum integer in the int64 UTxO-RPC field.
// The wire range is -2^64..2^64-1, so a value outside int64 keeps its low 64
// bits in two's complement, as the pallas UTxO-RPC mapper does: a uint64
// value is recovered by reading the field as uint64. Failing instead would
// make a valid transaction, and the block holding it, unconvertible.
func metadatumInt64(v *big.Int) int64 {
	if v.IsInt64() {
		return v.Int64()
	}
	low := new(big.Int).And(v, new(big.Int).SetUint64(math.MaxUint64))
	return int64(low.Uint64()) // #nosec G115 -- two's complement by design
}

func redeemerPurposeToUtxorpc(tag RedeemerTag) utxorpc.RedeemerPurpose {
	switch tag {
	case RedeemerTagSpend:
		return utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_SPEND
	case RedeemerTagMint:
		return utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_MINT
	case RedeemerTagCert:
		return utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_CERT
	case RedeemerTagReward:
		return utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_REWARD
	case RedeemerTagVoting:
		return utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_VOTE
	case RedeemerTagProposing:
		return utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_PROPOSE
	case RedeemerTagGuarding:
		// UTxO-RPC has no guard purpose.
	}
	return utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_UNSPECIFIED
}

// attachRedeemers sets each redeemer on the input, mint policy, withdrawal or
// certificate it applies to. A redeemer index addresses the lexicographically
// sorted inputs, the sorted mint policies, the withdrawals in the order
// withdrawalsToUtxorpc emits, and the certificates in listed order.
func attachRedeemers(tx Transaction, ret *utxorpc.Tx) error {
	witnesses := tx.Witnesses()
	if witnesses == nil || witnesses.Redeemers() == nil {
		return nil
	}
	inputs := tx.Inputs()
	sortedInputs := make([]int, len(inputs))
	for i := range sortedInputs {
		sortedInputs[i] = i
	}
	slices.SortStableFunc(sortedInputs, func(a, b int) int {
		if c := bytes.Compare(
			inputs[a].Id().Bytes(),
			inputs[b].Id().Bytes(),
		); c != 0 {
			return c
		}
		return cmp.Compare(inputs[a].Index(), inputs[b].Index())
	})
	for key, value := range witnesses.Redeemers().Iter() {
		payload, err := plutusDataToUtxorpc(value.Data.Data)
		if err != nil {
			return fmt.Errorf("redeemer payload: %w", err)
		}
		if value.ExUnits.Memory < 0 || value.ExUnits.Steps < 0 {
			return errors.New("redeemer execution units are negative")
		}
		redeemer := &utxorpc.Redeemer{
			Purpose: redeemerPurposeToUtxorpc(key.Tag),
			Payload: payload,
			Index:   key.Index,
			ExUnits: &utxorpc.ExUnits{
				Memory: uint64(value.ExUnits.Memory),
				Steps:  uint64(value.ExUnits.Steps),
			},
			OriginalCbor: value.Data.Cbor(),
		}
		// Compared as uint64: int(key.Index) is negative on 32-bit builds
		// for indexes above the int32 range.
		idx := uint64(key.Index)
		inRange := func(n int) bool { return idx < uint64(n) } // #nosec G115 -- n is a length
		switch key.Tag {
		case RedeemerTagSpend:
			if inRange(len(sortedInputs)) {
				ret.Inputs[sortedInputs[idx]].Redeemer = redeemer
			}
		case RedeemerTagMint:
			if inRange(len(ret.GetMint())) {
				ret.Mint[idx].Redeemer = redeemer
			}
		case RedeemerTagReward:
			if inRange(len(ret.GetWithdrawals())) {
				ret.Withdrawals[idx].Redeemer = redeemer
			}
		case RedeemerTagCert:
			if inRange(len(ret.GetCertificates())) {
				ret.Certificates[idx].Redeemer = redeemer
			}
		case RedeemerTagVoting, RedeemerTagProposing, RedeemerTagGuarding:
			// No redeemer field on votes, proposals or guards.
		}
	}
	return nil
}

func withdrawalsToUtxorpc(
	withdrawals map[*Address]*big.Int,
) ([]*utxorpc.Withdrawal, error) {
	// Reward redeemer indexes address this order, which is cardano-ledger's
	// and puts script credentials before key credentials, unlike the bytes.
	ret := make([]*utxorpc.Withdrawal, 0, len(withdrawals))
	for _, addr := range SortRewardAccountAddresses(withdrawals) {
		account, err := addr.Bytes()
		if err != nil {
			return nil, fmt.Errorf("withdrawal reward account: %w", err)
		}
		ret = append(ret, &utxorpc.Withdrawal{
			RewardAccount: account,
			Coin:          BigIntToUtxorpcBigInt(withdrawals[addr]),
		})
	}
	return ret, nil
}

func mintToUtxorpc(mint *MultiAsset[MultiAssetTypeMint]) []*utxorpc.Multiasset {
	if mint == nil {
		return nil
	}
	policies := mint.Policies()
	slices.SortFunc(policies, func(a, b Blake2b224) int {
		return bytes.Compare(a.Bytes(), b.Bytes())
	})
	ret := make([]*utxorpc.Multiasset, 0, len(policies))
	for _, policy := range policies {
		ma := &utxorpc.Multiasset{PolicyId: policy.Bytes()}
		names := mint.Assets(policy)
		slices.SortFunc(names, bytes.Compare)
		for _, name := range names {
			ma.Assets = append(ma.Assets, &utxorpc.Asset{
				Name: name,
				Quantity: &utxorpc.Asset_MintCoin{
					MintCoin: BigIntToUtxorpcBigInt(mint.Asset(policy, name)),
				},
			})
		}
		ret = append(ret, ma)
	}
	return ret
}

func collateralToUtxorpc(tx TransactionBody) (*utxorpc.Collateral, error) {
	inputs := tx.Collateral()
	returnOutput := tx.CollateralReturn()
	total := tx.TotalCollateral()
	if len(inputs) == 0 && returnOutput == nil &&
		!TransactionTotalCollateralPresent(tx) {
		return nil, nil
	}
	ret := &utxorpc.Collateral{}
	for _, i := range inputs {
		input, err := i.Utxorpc()
		if err != nil {
			return nil, err
		}
		ret.Collateral = append(ret.Collateral, input)
	}
	if returnOutput != nil {
		output, err := returnOutput.Utxorpc()
		if err != nil {
			return nil, err
		}
		ret.CollateralReturn = output
	}
	if total != nil {
		ret.TotalCollateral = BigIntToUtxorpcBigInt(total)
	}
	return ret, nil
}

func proposalsToUtxorpc(
	proposals []ProposalProcedure,
) ([]*utxorpc.GovernanceActionProposal, error) {
	ret := make([]*utxorpc.GovernanceActionProposal, 0, len(proposals))
	for _, p := range proposals {
		account := p.RewardAccount()
		accountBytes, err := account.Bytes()
		if err != nil {
			return nil, fmt.Errorf("proposal reward account: %w", err)
		}
		action, err := govActionToUtxorpc(p.GovAction())
		if err != nil {
			return nil, err
		}
		anchor := p.Anchor()
		ret = append(ret, &utxorpc.GovernanceActionProposal{
			Deposit:       ToUtxorpcBigInt(p.Deposit()),
			RewardAccount: accountBytes,
			GovAction:     action,
			Anchor: &utxorpc.Anchor{
				Url:         anchor.Url,
				ContentHash: anchor.DataHash[:],
			},
		})
	}
	return ret, nil
}

func govActionIdToUtxorpc(id *GovActionId) *utxorpc.GovernanceActionId {
	if id == nil {
		return nil
	}
	return &utxorpc.GovernanceActionId{
		TransactionId:         id.TransactionId[:],
		GovernanceActionIndex: id.GovActionIdx,
	}
}

// parameterChangeUtxorpc is implemented by the era parameter change actions,
// whose parameter update types live outside this package.
type parameterChangeUtxorpc interface {
	ProtocolParamUpdateUtxorpc() (*utxorpc.PParams, error)
}

func govActionToUtxorpc(action GovAction) (*utxorpc.GovernanceAction, error) {
	if action == nil {
		return nil, errors.New("unsupported governance action <nil>")
	}
	actionValue := reflect.ValueOf(action)
	if actionValue.Kind() == reflect.Pointer && actionValue.IsNil() {
		return nil, errors.New("unsupported governance action <nil>")
	}
	ret := &utxorpc.GovernanceAction{}
	switch a := action.(type) {
	case ParameterChangeGovAction:
		change := &utxorpc.ParameterChangeAction{
			GovActionId: govActionIdToUtxorpc(a.PreviousGovActionId()),
		}
		if p, ok := action.(GovActionWithPolicy); ok {
			change.PolicyHash = p.GetPolicyHash()
		}
		if p, ok := action.(parameterChangeUtxorpc); ok {
			update, err := p.ProtocolParamUpdateUtxorpc()
			if err != nil {
				return nil, fmt.Errorf("parameter change update: %w", err)
			}
			change.ProtocolParamUpdate = update
		}
		ret.GovernanceAction = &utxorpc.GovernanceAction_ParameterChangeAction{
			ParameterChangeAction: change,
		}
	case *HardForkInitiationGovAction:
		if uint64(a.ProtocolVersion.Major) > math.MaxUint32 ||
			uint64(a.ProtocolVersion.Minor) > math.MaxUint32 {
			return nil, errors.New("protocol version exceeds uint32 range")
		}
		ret.GovernanceAction = &utxorpc.GovernanceAction_HardForkInitiationAction{
			HardForkInitiationAction: &utxorpc.HardForkInitiationAction{
				GovActionId: govActionIdToUtxorpc(a.ActionId),
				ProtocolVersion: &utxorpc.ProtocolVersion{
					Major: uint32(a.ProtocolVersion.Major), // #nosec G115
					Minor: uint32(a.ProtocolVersion.Minor), // #nosec G115
				},
			},
		}
	case *TreasuryWithdrawalGovAction:
		withdrawals := make([]*utxorpc.WithdrawalAmount, 0, len(a.Withdrawals))
		for _, addr := range SortRewardAccountAddresses(a.Withdrawals) {
			account, err := addr.Bytes()
			if err != nil {
				return nil, fmt.Errorf("treasury withdrawal account: %w", err)
			}
			withdrawals = append(withdrawals, &utxorpc.WithdrawalAmount{
				RewardAccount: account,
				Coin:          ToUtxorpcBigInt(a.Withdrawals[addr]),
			})
		}
		ret.GovernanceAction = &utxorpc.GovernanceAction_TreasuryWithdrawalsAction{
			TreasuryWithdrawalsAction: &utxorpc.TreasuryWithdrawalsAction{
				Withdrawals: withdrawals,
				PolicyHash:  a.PolicyHash,
			},
		}
	case *NoConfidenceGovAction:
		ret.GovernanceAction = &utxorpc.GovernanceAction_NoConfidenceAction{
			NoConfidenceAction: &utxorpc.NoConfidenceAction{
				GovActionId: govActionIdToUtxorpc(a.ActionId),
			},
		}
	case *UpdateCommitteeGovAction:
		update, err := updateCommitteeToUtxorpc(a)
		if err != nil {
			return nil, err
		}
		ret.GovernanceAction = &utxorpc.GovernanceAction_UpdateCommitteeAction{
			UpdateCommitteeAction: update,
		}
	case *NewConstitutionGovAction:
		ret.GovernanceAction = &utxorpc.GovernanceAction_NewConstitutionAction{
			NewConstitutionAction: &utxorpc.NewConstitutionAction{
				GovActionId: govActionIdToUtxorpc(a.ActionId),
				Constitution: &utxorpc.Constitution{
					Anchor: &utxorpc.Anchor{
						Url:         a.Constitution.Anchor.Url,
						ContentHash: a.Constitution.Anchor.DataHash[:],
					},
					Hash: a.Constitution.ScriptHash,
				},
			},
		}
	case *InfoGovAction:
		ret.GovernanceAction = &utxorpc.GovernanceAction_InfoAction{
			InfoAction: uint32(GovActionTypeInfo),
		}
	default:
		return nil, fmt.Errorf("unsupported governance action %T", action)
	}
	return ret, nil
}

func updateCommitteeToUtxorpc(
	a *UpdateCommitteeGovAction,
) (*utxorpc.UpdateCommitteeAction, error) {
	ret := &utxorpc.UpdateCommitteeAction{
		GovActionId: govActionIdToUtxorpc(a.ActionId),
	}
	for i := range a.Credentials {
		cred, err := a.Credentials[i].Utxorpc()
		if err != nil {
			return nil, err
		}
		ret.RemoveCommitteeCredentials = append(
			ret.RemoveCommitteeCredentials,
			cred,
		)
	}
	for credential, epoch := range a.CredEpochs {
		cred, err := credential.Utxorpc()
		if err != nil {
			return nil, err
		}
		// The wire epoch is uint64 and the field uint32. An expiry past
		// uint32 is never reached, so saturating keeps its meaning.
		ret.NewCommitteeCredentials = append(
			ret.NewCommitteeCredentials,
			&utxorpc.NewCommitteeCredentials{
				CommitteeColdCredential: cred,
				ExpiresEpoch: uint32(
					min(epoch, math.MaxUint32),
				), // #nosec G115 -- clamped
			},
		)
	}
	slices.SortFunc(
		ret.GetNewCommitteeCredentials(),
		func(x, y *utxorpc.NewCommitteeCredentials) int {
			return bytes.Compare(
				credentialBytes(x.GetCommitteeColdCredential()),
				credentialBytes(y.GetCommitteeColdCredential()),
			)
		},
	)
	if a.Quorum.Rat != nil {
		quorum, err := ToUtxorpcRationalNumber(a.Quorum.Rat)
		if err != nil {
			return nil, err
		}
		ret.NewCommitteeThreshold = quorum
	}
	return ret, nil
}

// credentialBytes gives a sort key in cardano-ledger's credential order,
// script hash before key hash.
func credentialBytes(c *utxorpc.StakeCredential) []byte {
	if c.GetScriptHash() != nil {
		return append([]byte{0}, c.GetScriptHash()...)
	}
	return append([]byte{1}, c.GetAddrKeyHash()...)
}

// ToUtxorpcRationalNumber converts a rational to the int32/uint32 pair
// UTxO-RPC carries, rejecting values that would wrap.
func ToUtxorpcRationalNumber(r *big.Rat) (*utxorpc.RationalNumber, error) {
	if r == nil {
		return nil, errors.New("rational number is unset")
	}
	if r.Num().Cmp(big.NewInt(math.MinInt32)) < 0 ||
		r.Num().Cmp(big.NewInt(math.MaxInt32)) > 0 ||
		r.Denom().Sign() < 0 ||
		r.Denom().Cmp(new(big.Int).SetUint64(math.MaxUint32)) > 0 {
		return nil, errors.New("invalid rational number values")
	}
	return &utxorpc.RationalNumber{
		Numerator:   int32(r.Num().Int64()),    // #nosec G115
		Denominator: uint32(r.Denom().Int64()), // #nosec G115
	}, nil
}
