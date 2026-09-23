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
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/plutigo/data"
)

// dijkstraPlutusV4Context builds the Plutus V4 ScriptContext defined by the
// Dijkstra ledger. V4 deliberately has a different TxInfo, ScriptPurpose and
// ScriptInfo shape from Conway's V3 context.
func dijkstraPlutusV4Context(
	level dijkstraScriptLevel,
	purpose script.ScriptPurpose,
	key common.RedeemerKey,
	value common.RedeemerValue,
) (data.PlutusData, error) {
	txInfo, err := dijkstraTxInfoV4(level)
	if err != nil {
		return nil, err
	}
	purposeData, scriptInfo, err := dijkstraPurposeV4(purpose, key)
	if err != nil {
		return nil, err
	}
	_ = purposeData // The purpose is represented in txInfoRedeemers.
	return data.NewConstr(
		0,
		txInfo,
		data.Normalize(value.Data.Data),
		scriptInfo,
		data.NewByteString(purpose.ScriptHash().Bytes()),
	), nil
}

func dijkstraTxInfoV4(level dijkstraScriptLevel) (data.PlutusData, error) {
	base, err := script.NewTxInfoV3FromTransaction(
		level.slotState,
		transactionWithoutGuardingRedeemers{Transaction: level.tx},
		level.resolved,
	)
	if err != nil {
		return nil, err
	}
	inputs, err := dijkstraInputsV4(
		script.SortInputs(level.tx.Inputs()),
		level.view.ResolvedInputs,
	)
	if err != nil {
		return nil, err
	}
	referenceInputs, err := dijkstraInputsV4(
		script.SortInputs(level.tx.ReferenceInputs()),
		level.view.ResolvedReferenceInputs,
	)
	if err != nil {
		return nil, err
	}
	outputs := make([]data.PlutusData, 0, len(level.tx.Outputs()))
	for _, output := range level.tx.Outputs() {
		translated, err := dijkstraOutputV4(output)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, translated)
	}
	certificates := make([]data.PlutusData, 0, len(level.tx.Certificates()))
	for _, certificate := range level.tx.Certificates() {
		translated, err := dijkstraCertificateV4(certificate)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, translated)
	}
	redeemers, err := dijkstraRedeemersV4(level)
	if err != nil {
		return nil, err
	}
	withdrawals, err := dijkstraWithdrawalsV4(level.tx.Withdrawals())
	if err != nil {
		return nil, err
	}
	directDeposits, balanceIntervals, guards, requiredGuards, err := dijkstraBodyFieldsV4(
		level.tx.body,
	)
	if err != nil {
		return nil, err
	}
	validRange, err := dijkstraPOSIXTimeRangeV4(level)
	if err != nil {
		return nil, err
	}

	mint := level.tx.AssetMint()
	if mint == nil {
		mint = &common.MultiAsset[common.MultiAssetTypeMint]{}
	}
	donation := level.tx.Donation()
	if donation == nil {
		donation = new(big.Int)
	}

	return data.NewConstr(
		0,
		data.NewByteString(level.tx.Id().Bytes()),
		data.NewConstr(1),
		data.NewList(inputs...),
		data.NewList(referenceInputs...),
		data.NewList(outputs...),
		mint.ToPlutusData(),
		data.NewList(certificates...),
		withdrawals,
		directDeposits,
		balanceIntervals,
		validRange,
		guards,
		requiredGuards,
		redeemers,
		base.Data.ToPlutusData(),
		base.Votes.ToPlutusData(),
		dijkstraPlutusDataList(level.tx.ProposalProcedures()),
		base.CurrentTreasuryAmount.ToPlutusData(),
		data.NewInteger(donation),
	), nil
}

func dijkstraPOSIXTimeRangeV4(
	level dijkstraScriptLevel,
) (data.PlutusData, error) {
	var lower *big.Int
	if dijkstraValidityLowerBoundPresentV4(level.tx.body) {
		value, err := level.slotState.SlotToTime(
			level.tx.ValidityIntervalStart(),
		)
		if err != nil {
			return nil, err
		}
		lower = big.NewInt(value.UnixMilli())
	}
	var upper *big.Int
	if slot, present := level.tx.ValidityIntervalUpperBound(); present {
		value, err := level.slotState.SlotToTime(slot)
		if err != nil {
			return nil, err
		}
		upper = big.NewInt(value.UnixMilli())
	}
	return dijkstraPOSIXTimeRangeDataV4(lower, upper), nil
}

func dijkstraValidityLowerBoundPresentV4(body common.TransactionBody) bool {
	present := body.ValidityIntervalStart() > 0
	if len(body.Cbor()) == 0 {
		return present
	}
	var fields map[uint]cbor.RawMessage
	if _, err := cbor.Decode(body.Cbor(), &fields); err != nil {
		return present
	}
	_, present = fields[8]
	return present
}

func dijkstraPOSIXTimeRangeDataV4(
	lower *big.Int,
	upper *big.Int,
) data.PlutusData {
	optional := func(value *big.Int) data.PlutusData {
		if value == nil {
			return data.NewConstr(1)
		}
		return data.NewConstr(0, data.NewInteger(value))
	}
	return data.NewConstr(0, optional(lower), optional(upper))
}

func dijkstraInputsV4(
	inputs []common.TransactionInput,
	resolved []common.Utxo,
) ([]data.PlutusData, error) {
	byID := make(map[string]common.Utxo, len(resolved))
	for _, utxo := range resolved {
		if utxo.Id != nil {
			byID[utxo.Id.String()] = utxo
		}
	}
	ret := make([]data.PlutusData, 0, len(inputs))
	for _, input := range inputs {
		utxo, ok := byID[input.String()]
		if !ok || utxo.Output == nil {
			return nil, common.InputResolutionError{
				Input: input,
				Err:   errors.New("resolved input is missing"),
			}
		}
		output, err := dijkstraOutputV4(utxo.Output)
		if err != nil {
			return nil, err
		}
		ret = append(ret, data.NewConstr(
			0,
			dijkstraTxOutRefV4(input),
			output,
		))
	}
	return ret, nil
}

func dijkstraTxOutRefV4(input common.TransactionInput) data.PlutusData {
	return data.NewConstr(
		0,
		data.NewByteString(input.Id().Bytes()),
		data.NewInteger(new(big.Int).SetUint64(uint64(input.Index()))),
	)
}

func dijkstraOutputV4(
	output common.TransactionOutput,
) (data.PlutusData, error) {
	if output == nil {
		return nil, errors.New("nil transaction output in Plutus V4 context")
	}
	address, err := dijkstraAddressV4(output.Address())
	if err != nil {
		return nil, err
	}
	valuePairs := make([][2]data.PlutusData, 0, 2)
	amount := output.Amount()
	if amount == nil {
		amount = new(big.Int)
	}
	valuePairs = append(valuePairs, [2]data.PlutusData{
		data.NewByteString(nil),
		data.NewMap([][2]data.PlutusData{{
			data.NewByteString(nil), data.NewInteger(amount),
		}}),
	})
	if assets := output.Assets(); assets != nil {
		assetData, ok := assets.ToPlutusData().(*data.Map)
		if !ok {
			return nil, errors.New("multi-asset value did not encode as a map")
		}
		valuePairs = append(valuePairs, assetData.Pairs...)
	}

	var datumData data.PlutusData
	switch {
	case output.Datum() != nil:
		datumData = data.NewConstr(2, data.Normalize(output.Datum().Data))
	case output.DatumHash() != nil:
		datumData = data.NewConstr(
			1,
			data.NewByteString(output.DatumHash().Bytes()),
		)
	default:
		datumData = data.NewConstr(0)
	}
	var referenceScript data.PlutusData
	if output.ScriptRef() == nil {
		referenceScript = data.NewConstr(1)
	} else {
		referenceScript = data.NewConstr(
			0,
			data.NewByteString(output.ScriptRef().Hash().Bytes()),
		)
	}
	return data.NewConstr(
		0,
		address,
		data.NewMap(valuePairs),
		datumData,
		referenceScript,
	), nil
}

func dijkstraAddressV4(address common.Address) (data.PlutusData, error) {
	if address.Type() == common.AddressTypeByron {
		return nil, errors.New(
			"byron output is not supported in Plutus V4 context",
		)
	}
	if address.Type() == common.AddressTypeKeyPointer ||
		address.Type() == common.AddressTypeScriptPointer {
		return nil, errors.New(
			"pointer output is not supported in Plutus V4 context",
		)
	}
	var payment common.Credential
	switch payload := address.PayloadPayload().(type) {
	case common.AddressPayloadKeyHash:
		payment.CredType = common.CredentialTypeAddrKeyHash
		payment.Credential = payload.Hash
	case common.AddressPayloadScriptHash:
		payment.CredType = common.CredentialTypeScriptHash
		payment.Credential = common.Blake2b224(payload.Hash)
	default:
		return nil, errors.New("transaction output has no payment credential")
	}
	stake := data.NewConstr(1)
	if credential, ok := address.StakeCredential(); ok {
		stake = data.NewConstr(0, credential.ToPlutusData())
	}
	return data.NewConstr(0, payment.ToPlutusData(), stake), nil
}

func dijkstraRedeemersV4(level dijkstraScriptLevel) (data.PlutusData, error) {
	wits := level.tx.Witnesses()
	if wits == nil || wits.Redeemers() == nil {
		return data.NewMap(nil), nil
	}
	pairs := make([][2]data.PlutusData, 0)
	for key, value := range wits.Redeemers().Iter() {
		purpose, err := dijkstraPurposeForKey(level, key)
		if err != nil {
			return nil, err
		}
		purposeData, _, err := dijkstraPurposeV4(purpose, key)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, [2]data.PlutusData{purposeData, data.Normalize(value.Data.Data)})
	}
	return data.NewMap(pairs), nil
}

func dijkstraPurposeForKey(
	level dijkstraScriptLevel,
	key common.RedeemerKey,
) (script.ScriptPurpose, error) {
	if key.Tag == common.RedeemerTagGuarding {
		purpose, ok := dijkstraGuardingPurpose(level.tx, key)
		if !ok {
			return nil, script.UnmatchedRedeemerError{RedeemerKey: key}
		}
		return purpose, nil
	}
	resolved := make(
		map[string]common.Utxo,
		len(level.view.ResolvedInputs),
	)
	for _, utxo := range level.view.ResolvedInputs {
		if utxo.Id != nil {
			resolved[utxo.Id.String()] = utxo
		}
	}
	mint := level.tx.AssetMint()
	if mint == nil {
		mint = &common.MultiAsset[common.MultiAssetTypeMint]{}
	}
	witnessDatums := make(map[common.Blake2b256]*common.Datum)
	if witnesses := level.tx.Witnesses(); witnesses != nil {
		for _, item := range witnesses.PlutusData() {
			datum := item
			witnessDatums[datum.Hash()] = &datum
		}
	}
	return script.BuildScriptPurpose(
		key,
		resolved,
		script.SortInputs(level.tx.Inputs()),
		*mint,
		level.tx.Certificates(),
		level.tx.Withdrawals(),
		level.tx.VotingProcedures(),
		level.tx.ProposalProcedures(),
		witnessDatums,
	)
}

func dijkstraPurposeV4(
	purpose script.ScriptPurpose,
	key common.RedeemerKey,
) (data.PlutusData, data.PlutusData, error) {
	hash := data.NewByteString(purpose.ScriptHash().Bytes())
	index := data.NewInteger(new(big.Int).SetUint64(uint64(key.Index)))
	switch p := purpose.(type) {
	case script.ScriptPurposeMinting:
		policy := data.NewByteString(p.PolicyId.Bytes())
		return data.NewConstr(0, hash, policy), data.NewConstr(0, policy), nil
	case script.ScriptPurposeSpending:
		input := dijkstraTxOutRefV4(p.Input.Id)
		datumOption := data.NewConstr(1)
		if p.Datum != nil {
			datumOption = data.NewConstr(0, p.Datum)
		}
		return data.NewConstr(1, hash, input),
			data.NewConstr(1, input, datumOption), nil
	case script.ScriptPurposeRewarding:
		credential := p.StakeCredential.ToPlutusData()
		return data.NewConstr(2, hash, credential),
			data.NewConstr(2, credential), nil
	case script.ScriptPurposeCertifying:
		certificate, err := dijkstraCertificateV4(p.Certificate)
		if err != nil {
			return nil, nil, err
		}
		return data.NewConstr(3, hash, index, certificate),
			data.NewConstr(3, index, certificate), nil
	case script.ScriptPurposeVoting:
		voter := p.Voter.ToPlutusData()
		return data.NewConstr(4, hash, voter), data.NewConstr(4, voter), nil
	case script.ScriptPurposeProposing:
		proposal, ok := p.ProposalProcedure.(interface {
			ToPlutusData() data.PlutusData
		})
		if !ok {
			return nil, nil, fmt.Errorf(
				"proposal procedure %T cannot be encoded for Plutus V4",
				p.ProposalProcedure,
			)
		}
		proposalData := proposal.ToPlutusData()
		return data.NewConstr(5, hash, index, proposalData),
			data.NewConstr(5, index, proposalData), nil
	case script.ScriptPurposeGuarding:
		return data.NewConstr(6, hash, index),
			data.NewConstr(6, index, data.NewConstr(1)), nil
	default:
		return nil, nil, fmt.Errorf("unsupported Plutus V4 purpose %T", purpose)
	}
}

func dijkstraWithdrawalsV4(
	withdrawals map[*common.Address]*big.Int,
) (data.PlutusData, error) {
	sortedAddresses := script.SortWithdrawalAddresses(withdrawals)
	pairs := make([][2]data.PlutusData, len(sortedAddresses))
	for idx, address := range sortedAddresses {
		credential, err := address.RewardAccountCredential()
		if err != nil {
			return nil, err
		}
		pairs[idx] = [2]data.PlutusData{
			credential.ToPlutusData(), data.NewInteger(withdrawals[address]),
		}
	}
	return data.NewMap(pairs), nil
}

func dijkstraBodyFieldsV4(body common.TransactionBody) (
	directDeposits data.PlutusData,
	balanceIntervals data.PlutusData,
	guards data.PlutusData,
	requiredGuards data.PlutusData,
	err error,
) {
	balanceIntervals = data.NewMap(nil)
	guards = data.NewList()
	requiredGuards = data.NewMap(nil)
	var deposits map[cbor.ByteString]uint64
	var intervals DijkstraAccountBalanceIntervals
	var guardSet *DijkstraGuards
	var required DijkstraRequiredTopLevelGuards
	switch b := body.(type) {
	case *DijkstraTransactionBody:
		deposits = b.TxDirectDeposits
		intervals = b.TxBalanceIntervals
		guardSet = b.TxGuards
	case *DijkstraSubTransactionBody:
		deposits = b.TxDirectDeposits
		intervals = b.TxAccountBalanceIntervals
		guardSet = b.TxGuards
		required = b.TxRequiredTopLevelGuards
	default:
		return nil, nil, nil, nil, fmt.Errorf(
			"unexpected Dijkstra body type %T",
			body,
		)
	}
	directDeposits, err = dijkstraDirectDepositsV4(deposits)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if len(intervals) > 0 {
		balanceIntervals, err = dijkstraAccountBalanceIntervalsV4(intervals)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}
	if guardSet != nil {
		items := make([]data.PlutusData, len(guardSet.Credentials))
		for idx := range guardSet.Credentials {
			items[idx] = guardSet.Credentials[idx].ToPlutusData()
		}
		guards = data.NewList(items...)
	}
	if len(required) > 0 {
		requiredGuards, err = dijkstraRequiredTopLevelGuardsV4(required)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}
	return directDeposits, balanceIntervals, guards, requiredGuards, nil
}

// sortedDijkstraCredentials returns the keys of a credential-keyed map in
// deterministic (CredType, then hash bytes) order, matching
// dijkstraSortCredentialKeys, so Plutus V4 map encodings are reproducible.
func sortedDijkstraCredentials[V any](
	values map[*common.Credential]V,
) []*common.Credential {
	credentials := make([]*common.Credential, 0, len(values))
	for credential := range values {
		credentials = append(credentials, credential)
	}
	slices.SortFunc(credentials, func(a, b *common.Credential) int {
		if a.CredType != b.CredType {
			return int(a.CredType) - int(b.CredType)
		}
		return bytes.Compare(a.Credential.Bytes(), b.Credential.Bytes())
	})
	return credentials
}

func dijkstraAccountBalanceIntervalsV4(
	intervals DijkstraAccountBalanceIntervals,
) (data.PlutusData, error) {
	rawIntervals := map[*common.Credential]*DijkstraAccountBalanceInterval(
		intervals,
	)
	if err := validateDijkstraCredentialMapKeys(
		rawIntervals,
		"account balance intervals",
	); err != nil {
		return nil, err
	}
	credentials := sortedDijkstraCredentials(rawIntervals)
	pairs := make([][2]data.PlutusData, 0, len(credentials))
	for _, credential := range credentials {
		interval := intervals[credential]
		if interval == nil {
			return nil, errors.New(
				"account balance intervals contains a nil interval",
			)
		}
		if err := validateDijkstraAccountBalanceInterval(interval); err != nil {
			return nil, err
		}
		pairs = append(pairs, [2]data.PlutusData{
			credential.ToPlutusData(),
			dijkstraAccountBalanceIntervalV4(interval),
		})
	}
	return data.NewMap(pairs), nil
}

func dijkstraAccountBalanceIntervalV4(
	interval *DijkstraAccountBalanceInterval,
) data.PlutusData {
	switch {
	case interval.Exact != nil:
		return data.NewConstr(
			3,
			data.NewInteger(new(big.Int).SetUint64(*interval.Exact)),
		)
	case interval.LowerBound != nil && interval.UpperBound != nil:
		return data.NewConstr(
			2,
			data.NewInteger(new(big.Int).SetUint64(*interval.LowerBound)),
			data.NewInteger(new(big.Int).SetUint64(*interval.UpperBound)),
		)
	case interval.LowerBound != nil:
		return data.NewConstr(
			0,
			data.NewInteger(new(big.Int).SetUint64(*interval.LowerBound)),
		)
	case interval.UpperBound != nil:
		return data.NewConstr(
			1,
			data.NewInteger(new(big.Int).SetUint64(*interval.UpperBound)),
		)
	default:
		// Unreachable for a decoded value: DijkstraAccountBalanceInterval's
		// own UnmarshalCBOR rejects an interval with every bound nil. Only a
		// manually-constructed zero-value interval can reach this case.
		return data.NewConstr(1, data.NewInteger(new(big.Int)))
	}
}

func dijkstraRequiredTopLevelGuardsV4(
	required DijkstraRequiredTopLevelGuards,
) (data.PlutusData, error) {
	rawRequired := map[*common.Credential]*common.Datum(required)
	if err := validateDijkstraCredentialMapKeys(
		rawRequired,
		"required top-level guards",
	); err != nil {
		return nil, err
	}
	credentials := sortedDijkstraCredentials(rawRequired)
	pairs := make([][2]data.PlutusData, 0, len(credentials))
	for _, credential := range credentials {
		value := data.NewConstr(1)
		datum := required[credential]
		if err := validateDijkstraRequiredTopLevelGuardDatum(datum); err != nil {
			return nil, err
		}
		if datum != nil {
			// Normalize: datum.Data comes straight off cbor.Decode, so it
			// still carries the wire's definite/indefinite encoding. Every
			// other script-visible boundary in this file normalizes; this
			// one must too, or a required top-level guard datum reaches a
			// PlutusV4 script with non-canonical bytes.
			value = data.NewConstr(0, data.Normalize(datum.Data))
		}
		pairs = append(pairs, [2]data.PlutusData{
			credential.ToPlutusData(), value,
		})
	}
	return data.NewMap(pairs), nil
}

func dijkstraDirectDepositsV4(
	deposits map[cbor.ByteString]uint64,
) (data.PlutusData, error) {
	type entry struct {
		account []byte
		amount  uint64
	}
	entries := make([]entry, 0, len(deposits))
	for account, amount := range deposits {
		entries = append(
			entries,
			entry{account: account.Bytes(), amount: amount},
		)
	}
	slices.SortFunc(entries, func(a, b entry) int {
		return bytes.Compare(a.account, b.account)
	})
	pairs := make([][2]data.PlutusData, len(entries))
	for idx, item := range entries {
		address, err := common.NewAddressFromBytes(item.account)
		if err != nil {
			return nil, fmt.Errorf("decode direct-deposit account: %w", err)
		}
		credential, err := address.RewardAccountCredential()
		if err != nil {
			return nil, err
		}
		pairs[idx] = [2]data.PlutusData{
			credential.ToPlutusData(),
			data.NewInteger(new(big.Int).SetUint64(item.amount)),
		}
	}
	return data.NewMap(pairs), nil
}

type dijkstraCredentialKey struct {
	Type uint
	Hash common.Blake2b224
}

func (k *dijkstraCredentialKey) UnmarshalCBOR(raw []byte) error {
	var credential common.Credential
	if _, err := cbor.Decode(raw, &credential); err != nil {
		return err
	}
	k.Type = credential.CredType
	k.Hash = credential.Credential
	return nil
}

func (k dijkstraCredentialKey) credential() *common.Credential {
	return &common.Credential{CredType: k.Type, Credential: k.Hash}
}

func dijkstraSortCredentialKeys(credentials []dijkstraCredentialKey) {
	slices.SortFunc(credentials, func(a, b dijkstraCredentialKey) int {
		if a.Type != b.Type {
			return int(a.Type) - int(b.Type)
		}
		return bytes.Compare(a.Hash.Bytes(), b.Hash.Bytes())
	})
}

func dijkstraPlutusDataList(values []common.ProposalProcedure) data.PlutusData {
	items := make([]data.PlutusData, 0, len(values))
	for _, value := range values {
		if encodable, ok := value.(interface {
			ToPlutusData() data.PlutusData
		}); ok {
			items = append(items, encodable.ToPlutusData())
		}
	}
	return data.NewList(items...)
}

func dijkstraCertificateV4(
	certificate common.Certificate,
) (data.PlutusData, error) {
	integer := func(value int64) data.PlutusData {
		return data.NewInteger(big.NewInt(value))
	}
	delegateePool := func(pool common.PoolKeyHash) data.PlutusData {
		return data.NewConstr(0, data.NewByteString(pool.Bytes()))
	}
	delegateeDRep := func(drep *common.Drep) data.PlutusData {
		return data.NewConstr(1, drep.ToPlutusData())
	}
	switch cert := certificate.(type) {
	case *common.RegistrationCertificate:
		return data.NewConstr(
			0, cert.StakeCredential.ToPlutusData(), integer(cert.Amount),
		), nil
	case *common.DeregistrationCertificate:
		return data.NewConstr(
			1, cert.StakeCredential.ToPlutusData(), integer(cert.Amount),
		), nil
	case *common.StakeDelegationCertificate:
		return data.NewConstr(
			2, cert.StakeCredential.ToPlutusData(), delegateePool(cert.PoolKeyHash),
		), nil
	case *common.VoteDelegationCertificate:
		return data.NewConstr(
			2, cert.StakeCredential.ToPlutusData(), delegateeDRep(&cert.Drep),
		), nil
	case *common.StakeVoteDelegationCertificate:
		return data.NewConstr(
			2,
			cert.StakeCredential.ToPlutusData(),
			data.NewConstr(
				2,
				data.NewByteString(cert.PoolKeyHash.Bytes()),
				cert.Drep.ToPlutusData(),
			),
		), nil
	case *common.StakeRegistrationDelegationCertificate:
		return data.NewConstr(
			3,
			cert.StakeCredential.ToPlutusData(),
			delegateePool(cert.PoolKeyHash),
			integer(cert.Amount),
		), nil
	case *common.VoteRegistrationDelegationCertificate:
		return data.NewConstr(
			3,
			cert.StakeCredential.ToPlutusData(),
			delegateeDRep(&cert.Drep),
			integer(cert.Amount),
		), nil
	case *common.StakeVoteRegistrationDelegationCertificate:
		return data.NewConstr(
			3,
			cert.StakeCredential.ToPlutusData(),
			data.NewConstr(
				2,
				data.NewByteString(cert.PoolKeyHash.Bytes()),
				cert.Drep.ToPlutusData(),
			),
			integer(cert.Amount),
		), nil
	case *common.RegistrationDrepCertificate:
		return data.NewConstr(
			4, cert.DrepCredential.ToPlutusData(), integer(cert.Amount),
		), nil
	case *common.UpdateDrepCertificate:
		return data.NewConstr(5, cert.DrepCredential.ToPlutusData()), nil
	case *common.DeregistrationDrepCertificate:
		return data.NewConstr(
			6, cert.DrepCredential.ToPlutusData(), integer(cert.Amount),
		), nil
	case *common.PoolRegistrationCertificate:
		return data.NewConstr(
			7,
			data.NewByteString(cert.Operator.Bytes()),
			data.NewByteString(cert.VrfKeyHash.Bytes()),
		), nil
	case *common.PoolRetirementCertificate:
		return data.NewConstr(
			8,
			data.NewByteString(cert.PoolKeyHash.Bytes()),
			data.NewInteger(new(big.Int).SetUint64(cert.Epoch)),
		), nil
	case *common.AuthCommitteeHotCertificate:
		return data.NewConstr(
			9,
			cert.ColdCredential.ToPlutusData(),
			cert.HotCredential.ToPlutusData(),
		), nil
	case *common.ResignCommitteeColdCertificate:
		return data.NewConstr(10, cert.ColdCredential.ToPlutusData()), nil
	default:
		return nil, fmt.Errorf(
			"unsupported certificate type %T in Plutus V4 context",
			certificate,
		)
	}
}
