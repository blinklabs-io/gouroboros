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

package conway_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

func conwayVotingProcedures(voters ...*common.Voter) common.VotingProcedures {
	procedures := make(common.VotingProcedures, len(voters))
	action := &common.GovActionId{TransactionId: common.Blake2b256{1}}
	for _, voter := range voters {
		procedures[voter] = map[*common.GovActionId]common.VotingProcedure{
			action: {Vote: common.GovVoteYes},
		}
	}
	return procedures
}

func TestConwayExtraneousVotingRedeemersRequireScriptVoters(t *testing.T) {
	for _, voterType := range []struct {
		name   string
		typeID uint8
	}{
		{name: "committee key hash", typeID: common.VoterTypeConstitutionalCommitteeHotKeyHash},
		{name: "DRep key hash", typeID: common.VoterTypeDRepKeyHash},
		{name: "stake-pool key hash", typeID: common.VoterTypeStakingPoolKeyHash},
	} {
		t.Run(voterType.name, func(t *testing.T) {
			mintScript := common.PlutusV1Script{0x41, 0x01}
			voter := &common.Voter{
				Type: voterType.typeID,
				Hash: common.Blake2b224{1},
			}
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxMint:             conwayTreasuryMint(mintScript),
					TxVotingProcedures: conwayVotingProcedures(voter),
				},
				WitnessSet: conway.ConwayTransactionWitnessSet{
					WsPlutusV1Scripts: cbor.NewSetType(
						[]common.PlutusV1Script{mintScript},
						true,
					),
					WsRedeemers: conway.ConwayRedeemers{
						Redeemers: map[common.RedeemerKey]common.RedeemerValue{
							{Tag: common.RedeemerTagMint, Index: 0}:   {},
							{Tag: common.RedeemerTagVoting, Index: 0}: {},
						},
					},
				},
			}
			var extraErr conway.ExtraRedeemerError
			require.ErrorAs(
				t,
				conway.UtxoValidateExtraneousRedeemers(
					tx,
					0,
					nil,
					&conway.ConwayProtocolParameters{},
				),
				&extraErr,
			)
			require.Equal(t, common.RedeemerTagVoting, extraErr.RedeemerKey.Tag)
		})
	}
}

func TestConwayExtraneousVotingRedeemersAcceptScriptVotersAndPreserveIndexes(
	t *testing.T,
) {
	tests := []struct {
		name      string
		voterType uint8
	}{
		{name: "committee script hash", voterType: common.VoterTypeConstitutionalCommitteeHotScriptHash},
		{name: "DRep script hash", voterType: common.VoterTypeDRepScriptHash},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script := common.PlutusV1Script{0x42, byte(test.voterType)}
			voter := &common.Voter{Type: test.voterType, Hash: script.Hash()}
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxVotingProcedures: conwayVotingProcedures(voter),
				},
				WitnessSet: conway.ConwayTransactionWitnessSet{
					WsPlutusV1Scripts: cbor.NewSetType(
						[]common.PlutusV1Script{script},
						true,
					),
					WsRedeemers: conway.ConwayRedeemers{
						Redeemers: map[common.RedeemerKey]common.RedeemerValue{
							{Tag: common.RedeemerTagVoting, Index: 0}: {},
						},
					},
				},
			}
			require.NoError(t, conway.UtxoValidateExtraneousRedeemers(
				tx,
				0,
				nil,
				&conway.ConwayProtocolParameters{},
			))
		})
	}

	t.Run("script voter index follows preceding key voter", func(t *testing.T) {
		script := common.PlutusV1Script{0x43, 0x01}
		keyVoter := &common.Voter{
			Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
			Hash: common.Blake2b224{1},
		}
		scriptVoter := &common.Voter{
			Type: common.VoterTypeDRepScriptHash,
			Hash: script.Hash(),
		}
		tx := &conway.ConwayTransaction{
			Body: conway.ConwayTransactionBody{
				TxVotingProcedures: conwayVotingProcedures(keyVoter, scriptVoter),
			},
			WitnessSet: conway.ConwayTransactionWitnessSet{
				WsPlutusV1Scripts: cbor.NewSetType([]common.PlutusV1Script{script}, true),
				WsRedeemers: conway.ConwayRedeemers{
					Redeemers: map[common.RedeemerKey]common.RedeemerValue{
						{Tag: common.RedeemerTagVoting, Index: 1}: {},
					},
				},
			},
		}
		require.NoError(t, conway.UtxoValidateExtraneousRedeemers(
			tx,
			0,
			nil,
			&conway.ConwayProtocolParameters{},
		))
	})
}
