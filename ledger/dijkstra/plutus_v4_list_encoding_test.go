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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

// Plutus V4 encodes its single-constructor ledger types as Data lists rather
// than as constructor 0: ScriptContext, TxInfo, TxInInfo, TxOutRef, TxOut,
// Address, POSIXTimeRange, GovernanceActionId, ProtocolVersion, Constitution,
// Rational and ProposalProcedure.

func TestDijkstraInputsV4ListEncoding(t *testing.T) {
	txId := bytes.Repeat([]byte{0x11}, common.Blake2b256Size)
	input := shelley.ShelleyTransactionInput{
		TxId:        common.NewBlake2b256(txId),
		OutputIndex: 3,
	}
	paymentHash := bytes.Repeat([]byte{0x22}, common.AddressHashSize)
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		paymentHash,
		nil,
	)
	require.NoError(t, err)
	output := &babbage.BabbageTransactionOutput{
		OutputAddress: address,
	}
	output.OutputAmount.Amount = 5

	inputs, err := dijkstraInputsV4(
		[]common.TransactionInput{input},
		[]common.Utxo{{Id: input, Output: output}},
	)
	require.NoError(t, err)
	require.Len(t, inputs, 1)
	txInInfo := requireDijkstraV4List(t, inputs[0], 2)
	outRef := requireDijkstraV4List(t, txInInfo.Items[0], 2)
	requireDijkstraV4Bytes(t, outRef.Items[0], txId)
	requireDijkstraV4Integer(t, outRef.Items[1], 3)
	txOut := requireDijkstraV4List(t, txInInfo.Items[1], 4)
	addressData := requireDijkstraV4List(t, txOut.Items[0], 2)
	payment := requireDijkstraV4Constr(t, addressData.Items[0], 0, 1)
	requireDijkstraV4Bytes(t, payment.Fields[0], paymentHash)
	requireDijkstraV4Constr(t, addressData.Items[1], 1, 0)
	requireDijkstraV4Map(t, txOut.Items[1], 1)
	requireDijkstraV4Constr(t, txOut.Items[2], 0, 0)
	requireDijkstraV4Constr(t, txOut.Items[3], 1, 0)

	spending := script.ScriptPurposeSpending{
		Input: common.Utxo{Id: input, Output: output},
	}
	purpose, info, err := dijkstraPurposeV4(
		spending,
		common.RedeemerKey{Tag: common.RedeemerTagSpend},
	)
	require.NoError(t, err)
	purposeData := requireDijkstraV4Constr(t, purpose, 1, 2)
	requireDijkstraV4List(t, purposeData.Fields[1], 2)
	infoData := requireDijkstraV4Constr(t, info, 1, 2)
	requireDijkstraV4List(t, infoData.Fields[0], 2)
}

func testDijkstraV4GovActionId(t *testing.T) *common.GovActionId {
	t.Helper()
	id := &common.GovActionId{GovActionIdx: 4}
	copy(id.TransactionId[:], bytes.Repeat([]byte{0x33}, 32))
	return id
}

func requireDijkstraV4GovActionId(
	t *testing.T,
	value data.PlutusData,
	id *common.GovActionId,
) {
	t.Helper()
	just := requireDijkstraV4Constr(t, value, 0, 1)
	encoded := requireDijkstraV4List(t, just.Fields[0], 2)
	requireDijkstraV4Bytes(t, encoded.Items[0], id.TransactionId[:])
	requireDijkstraV4Integer(t, encoded.Items[1], int64(id.GovActionIdx))
}

func TestDijkstraProposalProceduresV4ListEncoding(t *testing.T) {
	id := testDijkstraV4GovActionId(t)
	stakeHash := bytes.Repeat([]byte{0x44}, common.AddressHashSize)
	rewardAccount, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey,
		common.AddressNetworkTestnet,
		nil,
		stakeHash,
	)
	require.NoError(t, err)
	hardFork := &common.HardForkInitiationGovAction{
		Type:     uint(common.GovActionTypeHardForkInitiation),
		ActionId: id,
	}
	hardFork.ProtocolVersion.Major = 13
	hardFork.ProtocolVersion.Minor = 1
	member := common.Credential{CredType: common.CredentialTypeAddrKeyHash}
	updateCommittee := &common.UpdateCommitteeGovAction{
		Type:       uint(common.GovActionTypeUpdateCommittee),
		ActionId:   id,
		CredEpochs: map[*common.Credential]uint64{&member: 9},
		Quorum:     cbor.Rat{Rat: big.NewRat(2, 3)},
	}
	constitution := &common.NewConstitutionGovAction{
		Type:     uint(common.GovActionTypeNewConstitution),
		ActionId: id,
	}
	constitution.Constitution.ScriptHash = bytes.Repeat([]byte{0x55}, 28)
	noConfidence := &common.NoConfidenceGovAction{
		Type:     uint(common.GovActionTypeNoConfidence),
		ActionId: id,
	}
	proposal := func(action common.GovAction) common.ProposalProcedure {
		return DijkstraProposalProcedure{
			PPDeposit:       100,
			PPRewardAccount: rewardAccount,
			PPGovAction:     DijkstraGovAction{Action: action},
		}
	}

	proposals, err := dijkstraProposalProceduresV4(
		[]common.ProposalProcedure{
			proposal(hardFork),
			proposal(updateCommittee),
			proposal(constitution),
			proposal(noConfidence),
			proposal(&common.InfoGovAction{
				Type: uint(common.GovActionTypeInfo),
			}),
		},
	)
	require.NoError(t, err)
	encoded := requireDijkstraV4List(t, proposals, 5)
	actions := make([]*data.Constr, 0, len(encoded.Items))
	for _, item := range encoded.Items {
		procedure := requireDijkstraV4List(t, item, 3)
		requireDijkstraV4Integer(t, procedure.Items[0], 100)
		reward := requireDijkstraV4Constr(t, procedure.Items[1], 0, 1)
		requireDijkstraV4Bytes(t, reward.Fields[0], stakeHash)
		action, ok := procedure.Items[2].(*data.Constr)
		require.True(t, ok, "expected constructor, got %T", procedure.Items[2])
		actions = append(actions, action)
	}

	requireDijkstraV4Constr(t, actions[0], 1, 2)
	requireDijkstraV4GovActionId(t, actions[0].Fields[0], id)
	version := requireDijkstraV4List(t, actions[0].Fields[1], 2)
	requireDijkstraV4Integer(t, version.Items[0], 13)
	requireDijkstraV4Integer(t, version.Items[1], 1)

	requireDijkstraV4Constr(t, actions[1], 4, 4)
	requireDijkstraV4GovActionId(t, actions[1].Fields[0], id)
	quorum := requireDijkstraV4List(t, actions[1].Fields[3], 2)
	requireDijkstraV4Integer(t, quorum.Items[0], 2)
	requireDijkstraV4Integer(t, quorum.Items[1], 3)

	requireDijkstraV4Constr(t, actions[2], 5, 2)
	requireDijkstraV4GovActionId(t, actions[2].Fields[0], id)
	constitutionData := requireDijkstraV4List(t, actions[2].Fields[1], 1)
	scriptHash := requireDijkstraV4Constr(t, constitutionData.Items[0], 0, 1)
	requireDijkstraV4Bytes(
		t,
		scriptHash.Fields[0],
		constitution.Constitution.ScriptHash,
	)

	requireDijkstraV4Constr(t, actions[3], 3, 1)
	requireDijkstraV4GovActionId(t, actions[3].Fields[0], id)

	requireDijkstraV4Constr(t, actions[4], 6, 0)

	proposing := script.ScriptPurposeProposing{
		ProposalProcedure: proposal(hardFork),
	}
	purpose, info, err := dijkstraPurposeV4(
		proposing,
		common.RedeemerKey{Tag: common.RedeemerTagProposing},
	)
	require.NoError(t, err)
	purposeData := requireDijkstraV4Constr(t, purpose, 5, 3)
	requireDijkstraV4List(t, purposeData.Fields[2], 3)
	infoData := requireDijkstraV4Constr(t, info, 5, 2)
	requireDijkstraV4List(t, infoData.Fields[1], 3)
}

func TestDijkstraTxInfoV4VotesListEncodeGovActionIds(t *testing.T) {
	id := testDijkstraV4GovActionId(t)
	voter := &common.Voter{Type: common.VoterTypeDRepKeyHash}
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxVotingProcedures: common.VotingProcedures{
				voter: {id: common.VotingProcedure{Vote: 1}},
			},
		},
		TxIsValid: true,
	}
	txInfoData, err := dijkstraTxInfoV4(dijkstraV4TestLevel(t, tx))
	require.NoError(t, err)
	txInfo := requireDijkstraV4List(t, txInfoData, 19)
	votes := requireDijkstraV4Map(t, txInfo.Items[15], 1)
	require.True(t, voter.ToPlutusData().Equal(votes.Pairs[0][0]))
	byAction := requireDijkstraV4Map(t, votes.Pairs[0][1], 1)
	actionId := requireDijkstraV4List(t, byAction.Pairs[0][0], 2)
	requireDijkstraV4Bytes(t, actionId.Items[0], id.TransactionId[:])
	requireDijkstraV4Integer(t, actionId.Items[1], int64(id.GovActionIdx))
	requireDijkstraV4Constr(t, byAction.Pairs[0][1], 1, 0)
}

// TopTxInfoSimplified sums withdrawals from the same account across levels,
// and a later level's vote on an action replaces an earlier one.
func TestDijkstraTopTxInfoV4MergesBatch(t *testing.T) {
	stakeHash := bytes.Repeat([]byte{0x66}, common.AddressHashSize)
	newRewardAccount := func() *common.Address {
		address, err := common.NewAddressFromParts(
			common.AddressTypeNoneKey,
			common.AddressNetworkTestnet,
			nil,
			stakeHash,
		)
		require.NoError(t, err)
		return &address
	}
	id := testDijkstraV4GovActionId(t)
	newVoter := func() *common.Voter {
		return &common.Voter{Type: common.VoterTypeDRepKeyHash}
	}
	sub := DijkstraSubTransactionBody{
		TxWithdrawals: map[*common.Address]uint64{newRewardAccount(): 4},
		TxVotingProcedures: common.VotingProcedures{
			newVoter(): {id: common.VotingProcedure{Vote: 1}},
		},
	}
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{newRewardAccount(): 5},
			TxVotingProcedures: common.VotingProcedures{
				newVoter(): {id: common.VotingProcedure{Vote: 0}},
			},
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{{Body: sub}},
				true,
			),
		},
		TxIsValid: true,
	}
	levels, _, err := dijkstraScriptLevels(tx, dijkstraV4TestLedgerState())
	require.NoError(t, err)
	require.Len(t, levels, 2)
	var guardHash common.Blake2b224
	topInfo, err := dijkstraTopTxInfoV4(levels[1], guardHash)
	require.NoError(t, err)
	simplified := requireDijkstraV4List(
		t,
		requireDijkstraV4List(t, topInfo, 4).Items[3],
		18,
	)
	withdrawals := requireDijkstraV4Map(t, simplified.Items[7], 1)
	requireDijkstraV4Integer(t, withdrawals.Pairs[0][1], 9)
	votes := requireDijkstraV4Map(t, simplified.Items[14], 1)
	byAction := requireDijkstraV4Map(t, votes.Pairs[0][1], 1)
	requireDijkstraV4List(t, byAction.Pairs[0][0], 2)
	requireDijkstraV4Constr(t, byAction.Pairs[0][1], 0, 0)
}

func TestDijkstraVotesV4RejectsMalformedActionId(t *testing.T) {
	malformed := data.NewMap([][2]data.PlutusData{{
		data.NewConstr(0),
		data.NewMap([][2]data.PlutusData{{
			data.NewByteString([]byte{0x01}),
			data.NewConstr(0),
		}}),
	}})
	votes, err := dijkstraVotesV4(malformed)
	require.Error(t, err)
	require.Nil(t, votes)
}
