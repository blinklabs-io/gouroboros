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

// Positions of the V4 TxInfo fields that TopTxInfoSimplified aggregates.
const (
	dijkstraTxInfoV4Id = iota
	_
	dijkstraTxInfoV4Inputs
	dijkstraTxInfoV4ReferenceInputs
	dijkstraTxInfoV4Outputs
	_
	dijkstraTxInfoV4TxCerts
	_
	_
	_
	_
	dijkstraTxInfoV4Guards
	_
	dijkstraTxInfoV4Redeemers
	dijkstraTxInfoV4Data
	_
	dijkstraTxInfoV4ProposalProcedures
	dijkstraTxInfoV4CurrentTreasuryAmount
	dijkstraTxInfoV4TreasuryDonation
	dijkstraTxInfoV4FieldCount
)

// dijkstraTopTxInfoV4 builds the TopTxInfo a top-level guard script receives:
// every sub-transaction's TxInfo, the datums the sub-transactions and the top
// level supply for guardHash in their required top-level guards, the top
// level's starting account balance intervals, and TopTxInfoSimplified, an
// aggregate over the sub-transactions followed by the top level.
func dijkstraTopTxInfoV4(
	top dijkstraScriptLevel,
	guardHash common.Blake2b224,
) (data.PlutusData, error) {
	batch := append(slices.Clone(top.subLevels), top)
	txInfos := make([]*data.List, len(batch))
	for idx, level := range batch {
		txInfo, err := level.cachedTxInfoV4()
		if err != nil {
			return nil, err
		}
		list, ok := txInfo.(*data.List)
		if !ok || len(list.Items) != dijkstraTxInfoV4FieldCount {
			return nil, errors.New("malformed Plutus V4 TxInfo")
		}
		txInfos[idx] = list
	}
	subTxInfos := make([]data.PlutusData, len(top.subLevels))
	for idx := range top.subLevels {
		subTxInfos[idx] = txInfos[idx]
	}
	guardDatums, err := dijkstraTopTxInfoGuardDatumsV4(batch, guardHash)
	if err != nil {
		return nil, err
	}
	startingIntervals := data.PlutusData(data.NewMap(nil))
	if body, ok := top.tx.body.(*DijkstraTransactionBody); ok &&
		len(body.TxStartingBalanceIntervals) > 0 {
		startingIntervals, err = dijkstraAccountBalanceIntervalsV4(
			body.TxStartingBalanceIntervals,
		)
		if err != nil {
			return nil, err
		}
	}
	simplified, err := dijkstraTopTxInfoSimplifiedV4(top, batch, txInfos)
	if err != nil {
		return nil, err
	}
	return data.NewList(
		data.NewList(subTxInfos...),
		guardDatums,
		startingIntervals,
		simplified,
	), nil
}

func dijkstraTopTxInfoGuardDatumsV4(
	batch []dijkstraScriptLevel,
	guardHash common.Blake2b224,
) (data.PlutusData, error) {
	type txDatum struct {
		id    common.Blake2b256
		datum data.PlutusData
	}
	datums := make([]txDatum, 0, len(batch))
	for _, level := range batch {
		_, required, err := dijkstraLevelBodyFieldsV4(level.tx.body)
		if err != nil {
			return nil, err
		}
		for credential, datum := range required {
			if credential == nil ||
				credential.CredType != common.CredentialTypeScriptHash ||
				credential.Credential != guardHash || datum == nil {
				continue
			}
			datums = append(datums, txDatum{
				id:    level.tx.Id(),
				datum: data.Normalize(datum.Data),
			})
		}
	}
	slices.SortFunc(datums, func(a, b txDatum) int {
		return bytes.Compare(a.id[:], b.id[:])
	})
	pairs := make([][2]data.PlutusData, len(datums))
	for idx, entry := range datums {
		pairs[idx] = [2]data.PlutusData{
			data.NewByteString(entry.id.Bytes()),
			entry.datum,
		}
	}
	return data.NewMap(pairs), nil
}

func dijkstraTopTxInfoSimplifiedV4(
	top dijkstraScriptLevel,
	batch []dijkstraScriptLevel,
	txInfos []*data.List,
) (data.PlutusData, error) {
	concat := func(field int) (data.PlutusData, error) {
		var items []data.PlutusData
		for _, txInfo := range txInfos {
			list, ok := txInfo.Items[field].(*data.List)
			if !ok {
				return nil, fmt.Errorf(
					"plutus V4 TxInfo field %d is not a list",
					field,
				)
			}
			items = append(items, list.Items...)
		}
		return data.NewList(items...), nil
	}
	fields := make([]data.PlutusData, 0, 18)

	ids := make([]data.PlutusData, len(txInfos))
	for idx, txInfo := range txInfos {
		ids[idx] = txInfo.Items[dijkstraTxInfoV4Id]
	}
	fields = append(fields, data.NewList(ids...))
	for _, field := range []int{
		dijkstraTxInfoV4Inputs,
		dijkstraTxInfoV4ReferenceInputs,
		dijkstraTxInfoV4Outputs,
	} {
		items, err := concat(field)
		if err != nil {
			return nil, err
		}
		fields = append(fields, items)
	}
	mints, burns := dijkstraBatchMintsV4(batch)
	fields = append(fields, mints, burns)
	certs, err := concat(dijkstraTxInfoV4TxCerts)
	if err != nil {
		return nil, err
	}
	fields = append(fields, certs)

	withdrawals, err := dijkstraBatchWithdrawalsV4(batch)
	if err != nil {
		return nil, err
	}
	directDeposits, err := dijkstraBatchDirectDepositsV4(batch)
	if err != nil {
		return nil, err
	}
	validRange, err := dijkstraBatchValidRangeV4(top, batch)
	if err != nil {
		return nil, err
	}
	guards, err := concat(dijkstraTxInfoV4Guards)
	if err != nil {
		return nil, err
	}
	requiredGuards, err := dijkstraBatchRequiredGuardsV4(batch)
	if err != nil {
		return nil, err
	}
	redeemerHashes, err := dijkstraBatchRedeemerHashesV4(txInfos)
	if err != nil {
		return nil, err
	}
	datums, err := dijkstraBatchDatumsV4(txInfos)
	if err != nil {
		return nil, err
	}
	fields = append(
		fields,
		withdrawals,
		directDeposits,
		validRange,
		guards,
		requiredGuards,
		redeemerHashes,
		datums,
		dijkstraBatchVotesV4(batch),
	)
	proposals, err := concat(dijkstraTxInfoV4ProposalProcedures)
	if err != nil {
		return nil, err
	}
	fields = append(fields, proposals)

	// The ledger rules guarantee every present treasury amount is equal, so
	// the first one stands for all of them.
	treasury := data.PlutusData(data.NewConstr(1))
	for _, txInfo := range txInfos {
		amount := txInfo.Items[dijkstraTxInfoV4CurrentTreasuryAmount]
		if constr, ok := amount.(*data.Constr); ok && constr.Tag != nil &&
			constr.Tag.Sign() == 0 {
			treasury = amount
			break
		}
	}
	donations := new(big.Int)
	for _, txInfo := range txInfos {
		donation, ok := txInfo.Items[dijkstraTxInfoV4TreasuryDonation].(*data.Integer)
		if !ok {
			return nil, errors.New(
				"plutus V4 TxInfo treasury donation is not an integer",
			)
		}
		donations.Add(donations, donation.Inner)
	}
	fields = append(fields, treasury, data.NewInteger(donations))
	return data.NewList(fields...), nil
}

// dijkstraBatchMintsV4 splits every level's mint into its positive and its
// negative quantities and sums each side across the batch.
func dijkstraBatchMintsV4(
	batch []dijkstraScriptLevel,
) (data.PlutusData, data.PlutusData) {
	mints := make(map[common.Blake2b224]map[cbor.ByteString]*big.Int)
	burns := make(map[common.Blake2b224]map[cbor.ByteString]*big.Int)
	add := func(
		target map[common.Blake2b224]map[cbor.ByteString]*big.Int,
		policy common.Blake2b224,
		name []byte,
		amount *big.Int,
	) {
		assets, ok := target[policy]
		if !ok {
			assets = make(map[cbor.ByteString]*big.Int)
			target[policy] = assets
		}
		key := cbor.NewByteString(name)
		sum, ok := assets[key]
		if !ok {
			sum = new(big.Int)
			assets[key] = sum
		}
		sum.Add(sum, amount)
	}
	for _, level := range batch {
		mint := level.tx.AssetMint()
		if mint == nil {
			continue
		}
		for _, policy := range mint.Policies() {
			for _, name := range mint.Assets(policy) {
				amount := mint.Asset(policy, name)
				switch {
				case amount == nil || amount.Sign() == 0:
				case amount.Sign() > 0:
					add(mints, policy, name, amount)
				default:
					add(burns, policy, name, amount)
				}
			}
		}
	}
	mintValue := common.NewMultiAsset(mints)
	burnValue := common.NewMultiAsset(burns)
	return mintValue.ToPlutusData(), burnValue.ToPlutusData()
}

// dijkstraBatchWithdrawalsV4 unions the batch's withdrawals, summing the
// amounts of an account that more than one level withdraws from.
func dijkstraBatchWithdrawalsV4(
	batch []dijkstraScriptLevel,
) (data.PlutusData, error) {
	merged := make(map[*common.Address]*big.Int)
	byKey := make(map[string]*common.Address)
	for _, level := range batch {
		for address, amount := range level.tx.Withdrawals() {
			if address == nil || amount == nil {
				continue
			}
			key, err := address.Bytes()
			if err != nil {
				return nil, err
			}
			existing, ok := byKey[string(key)]
			if !ok {
				existing = address
				byKey[string(key)] = address
				merged[address] = new(big.Int)
			}
			merged[existing].Add(merged[existing], amount)
		}
	}
	return dijkstraWithdrawalsV4(merged)
}

// dijkstraBatchDirectDepositsV4 unions the batch's direct deposits, summing
// the amounts of an account that more than one level deposits to.
func dijkstraBatchDirectDepositsV4(
	batch []dijkstraScriptLevel,
) (data.PlutusData, error) {
	merged := make(map[cbor.ByteString]*big.Int)
	for _, level := range batch {
		deposits, _, err := dijkstraLevelBodyFieldsV4(level.tx.body)
		if err != nil {
			return nil, err
		}
		for address, amount := range deposits {
			sum, ok := merged[address]
			if !ok {
				sum = new(big.Int)
				merged[address] = sum
			}
			sum.Add(sum, new(big.Int).SetUint64(amount))
		}
	}
	addresses := sortedDijkstraAccountAddresses(merged)
	pairs := make([][2]data.PlutusData, len(addresses))
	for idx, addressBytes := range addresses {
		address, err := dijkstraAddressFromKey(addressBytes)
		if err != nil {
			return nil, err
		}
		credential, err := address.RewardAccountCredential()
		if err != nil {
			return nil, err
		}
		pairs[idx] = [2]data.PlutusData{
			credential.ToPlutusData(),
			data.NewInteger(merged[addressBytes]),
		}
	}
	return data.NewMap(pairs), nil
}

// dijkstraBatchValidRangeV4 intersects the batch's validity intervals: the
// latest lower bound and the earliest upper bound that any level sets.
func dijkstraBatchValidRangeV4(
	top dijkstraScriptLevel,
	batch []dijkstraScriptLevel,
) (data.PlutusData, error) {
	var lowerSlot, upperSlot *uint64
	for _, level := range batch {
		if dijkstraValidityLowerBoundPresentV4(level.tx.body) {
			slot := level.tx.ValidityIntervalStart()
			if lowerSlot == nil || slot > *lowerSlot {
				lowerSlot = &slot
			}
		}
		if slot, present := level.tx.ValidityIntervalUpperBound(); present {
			if upperSlot == nil || slot < *upperSlot {
				upperSlot = &slot
			}
		}
	}
	toPOSIX := func(slot *uint64) (*big.Int, error) {
		if slot == nil {
			return nil, nil
		}
		value, err := top.slotState.SlotToTime(*slot)
		if err != nil {
			return nil, err
		}
		return big.NewInt(value.UnixMilli()), nil
	}
	lower, err := toPOSIX(lowerSlot)
	if err != nil {
		return nil, err
	}
	upper, err := toPOSIX(upperSlot)
	if err != nil {
		return nil, err
	}
	return dijkstraPOSIXTimeRangeDataV4(lower, upper), nil
}

// dijkstraBatchRequiredGuardsV4 lists the distinct credentials any level
// requires as top-level guards, in the reference Credential order.
func dijkstraBatchRequiredGuardsV4(
	batch []dijkstraScriptLevel,
) (data.PlutusData, error) {
	type credentialKey struct {
		credType uint
		hash     common.Blake2b224
	}
	unique := make(map[*common.Credential]struct{})
	seen := make(map[credentialKey]struct{})
	for _, level := range batch {
		_, required, err := dijkstraLevelBodyFieldsV4(level.tx.body)
		if err != nil {
			return nil, err
		}
		for credential := range required {
			if credential == nil {
				continue
			}
			key := credentialKey{credential.CredType, credential.Credential}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			unique[credential] = struct{}{}
		}
	}
	credentials := sortedDijkstraCredentials(unique)
	items := make([]data.PlutusData, len(credentials))
	for idx, credential := range credentials {
		items[idx] = credential.ToPlutusData()
	}
	return data.NewList(items...), nil
}

// dijkstraBatchRedeemerHashesV4 lists the distinct script hashes of every
// level's redeemer purposes, in ascending byte order. Every V4 ScriptPurpose
// constructor carries the script hash as its first field.
func dijkstraBatchRedeemerHashesV4(
	txInfos []*data.List,
) (data.PlutusData, error) {
	var hashes [][]byte
	for _, txInfo := range txInfos {
		redeemers, ok := txInfo.Items[dijkstraTxInfoV4Redeemers].(*data.Map)
		if !ok {
			return nil, errors.New("plutus V4 TxInfo redeemers is not a map")
		}
		for _, pair := range redeemers.Pairs {
			purpose, ok := pair[0].(*data.Constr)
			if !ok || len(purpose.Fields) == 0 {
				return nil, errors.New("malformed Plutus V4 script purpose")
			}
			hash, ok := purpose.Fields[0].(*data.ByteString)
			if !ok {
				return nil, errors.New(
					"malformed Plutus V4 script purpose hash",
				)
			}
			hashes = append(hashes, hash.Inner)
		}
	}
	slices.SortFunc(hashes, bytes.Compare)
	hashes = slices.CompactFunc(hashes, bytes.Equal)
	items := make([]data.PlutusData, len(hashes))
	for idx, hash := range hashes {
		items[idx] = data.NewByteString(hash)
	}
	return data.NewList(items...), nil
}

// dijkstraBatchDatumsV4 unions every level's witness datums. A datum hash
// determines its datum, so a hash seen twice is kept once.
func dijkstraBatchDatumsV4(txInfos []*data.List) (data.PlutusData, error) {
	var pairs [][2]data.PlutusData
	seen := make(map[string]struct{})
	for _, txInfo := range txInfos {
		datums, ok := txInfo.Items[dijkstraTxInfoV4Data].(*data.Map)
		if !ok {
			return nil, errors.New("plutus V4 TxInfo data is not a map")
		}
		for _, pair := range datums.Pairs {
			hash, ok := pair[0].(*data.ByteString)
			if !ok {
				return nil, errors.New("malformed Plutus V4 datum hash")
			}
			if _, ok := seen[string(hash.Inner)]; ok {
				continue
			}
			seen[string(hash.Inner)] = struct{}{}
			pairs = append(pairs, pair)
		}
	}
	slices.SortFunc(pairs, func(a, b [2]data.PlutusData) int {
		return bytes.Compare(
			a[0].(*data.ByteString).Inner,
			b[0].(*data.ByteString).Inner,
		)
	})
	return data.NewMap(pairs), nil
}

// dijkstraBatchVotesV4 unions the batch's votes. For a voter that votes in
// more than one level, a later level's vote on an action replaces an earlier
// one, as the reference VotingProcedures semigroup does.
func dijkstraBatchVotesV4(batch []dijkstraScriptLevel) data.PlutusData {
	type voterKey struct {
		voterType uint8
		hash      [28]byte
	}
	type actionKey struct {
		txId  common.Blake2b256
		index uint32
	}
	voters := make(map[voterKey]*common.Voter)
	votes := make(
		map[voterKey]map[actionKey]common.VotingProcedure,
	)
	actions := make(map[actionKey]*common.GovActionId)
	for _, level := range batch {
		for voter, byAction := range level.tx.VotingProcedures() {
			if voter == nil {
				continue
			}
			vKey := voterKey{voter.Type, voter.Hash}
			voterVotes, ok := votes[vKey]
			if !ok {
				voters[vKey] = voter
				voterVotes = make(map[actionKey]common.VotingProcedure)
				votes[vKey] = voterVotes
			}
			for actionId, procedure := range byAction {
				if actionId == nil {
					continue
				}
				aKey := actionKey{
					actionId.TransactionId,
					actionId.GovActionIdx,
				}
				if _, ok := actions[aKey]; !ok {
					actions[aKey] = actionId
				}
				voterVotes[aKey] = procedure
			}
		}
	}
	merged := make(common.VotingProcedures, len(voters))
	for vKey, voter := range voters {
		byAction := make(
			map[*common.GovActionId]common.VotingProcedure,
			len(votes[vKey]),
		)
		for aKey, procedure := range votes[vKey] {
			byAction[actions[aKey]] = procedure
		}
		merged[voter] = byAction
	}
	return dijkstraVotesV4(script.VotingProceduresToPlutusData(merged))
}

// dijkstraLevelBodyFieldsV4 returns a level's direct deposits and required
// top-level guards.
func dijkstraLevelBodyFieldsV4(body common.TransactionBody) (
	DijkstraDirectDeposits,
	DijkstraRequiredTopLevelGuards,
	error,
) {
	switch b := body.(type) {
	case *DijkstraTransactionBody:
		return b.TxDirectDeposits, b.TxRequiredTopLevelGuards, nil
	case *DijkstraSubTransactionBody:
		return b.TxDirectDeposits, b.TxRequiredTopLevelGuards, nil
	default:
		return nil, nil, fmt.Errorf("unexpected Dijkstra body type %T", body)
	}
}
