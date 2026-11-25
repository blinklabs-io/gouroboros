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

package script_test

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"

	gcbor "github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
)

// The cardano-ledger Conway translation golden covers PlutusV3 votes,
// proposals of every governance action, the current treasury amount and
// every certificate except UpdateDRep, but its generator gives every
// transaction a single Spending redeemer and a non-zero donation. The cases
// below cover the rest. Each expected value is built by hand from
// eras/conway/impl/src/Cardano/Ledger/Conway/TxInfo.hs at cardano-ledger
// 1210d37f (transTxCert, transPlutusPurposeV3, scriptPurposeToScriptInfo,
// transVotingProcedures, transProposal and the txInfoTreasuryDonation case)
// and the ToData splices in plutus-ledger-api PlutusLedgerApi/V3/Contexts.hs
// at plutus bdc12f105a98.

func v3Bytes(b byte, n int) []byte { return bytes.Repeat([]byte{b}, n) }

func v3Int(i int64) data.PlutusData { return data.NewInteger(big.NewInt(i)) }

func TestTxInfoV3ReferenceCases(t *testing.T) {
	scriptHash := v3Bytes(0xaa, 28)
	keyHash := v3Bytes(0xbb, 28)
	spentTxID := v3Bytes(0x01, 32)
	govTxID := v3Bytes(0x03, 32)
	datum := v3Int(42)

	scriptAddr, err := lcommon.NewAddressFromParts(
		lcommon.AddressTypeScriptNone, lcommon.AddressNetworkTestnet, scriptHash, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	scriptReward, err := lcommon.NewAddressFromParts(
		lcommon.AddressTypeNoneScript, lcommon.AddressNetworkTestnet, nil, scriptHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	keyReward, err := lcommon.NewAddressFromParts(
		lcommon.AddressTypeNoneKey, lcommon.AddressNetworkTestnet, nil, keyHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	datumCbor, err := data.Encode(datum)
	if err != nil {
		t.Fatal(err)
	}
	spentOutput, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(scriptAddr.String()).WithLovelace(5).WithDatum(datumCbor).Build()
	if err != nil {
		t.Fatal(err)
	}
	input := shelley.MustNewShelleyTransactionInput(hex.EncodeToString(spentTxID), 0)

	scriptCred := lcommon.Credential{
		CredType:   lcommon.CredentialTypeScriptHash,
		Credential: lcommon.CredentialHash(scriptHash),
	}
	updateDRep := &lcommon.UpdateDrepCertificate{
		CertType:       uint(lcommon.CertificateTypeUpdateDrep),
		DrepCredential: scriptCred,
	}
	voter := &lcommon.Voter{Type: lcommon.VoterTypeDRepScriptHash}
	copy(voter.Hash[:], scriptHash)
	govActionID := &lcommon.GovActionId{GovActionIdx: 7}
	copy(govActionID.TransactionId[:], govTxID)
	proposal := &conway.ConwayProposalProcedure{
		PPDeposit:       100,
		PPRewardAccount: keyReward,
		PPGovAction: conway.ConwayGovAction{
			Type:   uint(lcommon.GovActionTypeInfo),
			Action: &lcommon.InfoGovAction{Type: uint(lcommon.GovActionTypeInfo)},
		},
	}
	mint := lcommon.NewMultiAsset[lcommon.MultiAssetTypeMint](
		map[lcommon.Blake2b224]map[gcbor.ByteString]*big.Int{
			lcommon.NewBlake2b224(scriptHash): {
				gcbor.NewByteString([]byte("a")): big.NewInt(1),
			},
		},
	)
	redeemerTags := []lcommon.RedeemerTag{
		lcommon.RedeemerTagSpend,
		lcommon.RedeemerTagMint,
		lcommon.RedeemerTagCert,
		lcommon.RedeemerTagReward,
		lcommon.RedeemerTagVoting,
		lcommon.RedeemerTagProposing,
	}
	redeemers := conway.ConwayRedeemers{
		Redeemers: map[lcommon.RedeemerKey]lcommon.RedeemerValue{},
	}
	for _, tag := range redeemerTags {
		redeemers.Redeemers[lcommon.RedeemerKey{Tag: tag}] = lcommon.RedeemerValue{
			Data: lcommon.Datum{Data: v3Int(int64(tag))},
		}
	}

	build := func(donation uint64) *mockledger.MockTransaction {
		tx := mockledger.NewTransactionBuilder()
		tx.WithInputs(input)
		tx.WithType(6).
			WithMint(&mint).
			WithCertificates(updateDRep).
			WithWithdrawals(map[*lcommon.Address]uint64{&scriptReward: 5}).
			WithVotingProcedures(lcommon.VotingProcedures{
				voter: {govActionID: {Vote: lcommon.GovVoteYes}},
			}).
			WithProposalProcedures(proposal).
			WithDonation(donation).
			WithWitnesses(mockledger.NewMockTransactionWitnessSet().WithRedeemers(redeemers))
		return tx
	}

	// Expected values, each from the named reference function.
	scriptCredData := data.NewConstr(1, data.NewByteString(scriptHash)) // ScriptCredential
	txOutRef := data.NewConstr(0, data.NewByteString(spentTxID), v3Int(0))
	// transTxCert: UpdateDRepTxCert -> TxCertUpdateDRep (index 5), whose
	// DRepCredential is a newtype over Credential.
	txCert := data.NewConstr(5, scriptCredData)
	// transProposal: ProposalProcedure deposit (PubKeyCredential K) InfoAction.
	proposalData := data.NewConstr(
		0,
		v3Int(100),
		data.NewConstr(0, data.NewByteString(keyHash)),
		data.NewConstr(6),
	)
	// transVoter: DRepVoter (index 1) of a DRepCredential.
	voterData := data.NewConstr(1, scriptCredData)
	// transPlutusPurposeV3, in the order of the ledger redeemer map:
	// ConwayPlutusPurpose's constructors are Spending, Minting, Certifying,
	// Withdrawing, Voting, Proposing (Conway/Scripts.hs) and Map.toList
	// follows their derived Ord.
	purposes := []data.PlutusData{
		data.NewConstr(1, txOutRef),
		data.NewConstr(0, data.NewByteString(scriptHash)),
		data.NewConstr(3, v3Int(0), txCert),
		data.NewConstr(2, scriptCredData),
		data.NewConstr(4, voterData),
		data.NewConstr(5, v3Int(0), proposalData),
	}
	// scriptPurposeToScriptInfo maps each purpose to the ScriptInfo
	// constructor of the same index; SpendingScript adds the spent
	// output's datum as Just (Constr 0).
	scriptInfos := []data.PlutusData{
		data.NewConstr(1, txOutRef, data.NewConstr(0, datum)),
		purposes[1],
		purposes[2],
		purposes[3],
		purposes[4],
		purposes[5],
	}
	redeemerPairs := make([][2]data.PlutusData, len(purposes))
	for i, purpose := range purposes {
		redeemerPairs[i] = [2]data.PlutusData{purpose, v3Int(int64(redeemerTags[i]))}
	}
	votes := data.NewMap([][2]data.PlutusData{{
		voterData,
		data.NewMap([][2]data.PlutusData{{
			data.NewConstr(0, data.NewByteString(govTxID), v3Int(7)),
			data.NewConstr(1), // VoteYes
		}}),
	}})

	for _, tc := range []struct {
		name     string
		donation uint64
		want     data.PlutusData
	}{
		// txInfoTreasuryDonation: Coin 0 -> Nothing, otherwise Just.
		{"zero donation", 0, data.NewConstr(1)},
		{"donation", 9, data.NewConstr(0, v3Int(9))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info, err := script.NewTxInfoV3FromTransaction(
				referenceSlotState{},
				build(tc.donation),
				[]lcommon.Utxo{{Id: input, Output: spentOutput}},
				lcommon.ProtocolVersionConway,
			)
			if err != nil {
				t.Fatal(err)
			}
			fields := info.ToPlutusData().(*data.Constr).Fields()
			for _, check := range []struct {
				name  string
				field int
				want  data.PlutusData
			}{
				{"txInfoTxCerts", 5, data.NewList(txCert)},
				{"txInfoRedeemers", 9, data.NewMap(redeemerPairs)},
				{"txInfoVotes", 12, votes},
				{"txInfoProposalProcedures", 13, data.NewList(proposalData)},
				{"txInfoCurrentTreasuryAmount", 14, data.NewConstr(1)},
				{"txInfoTreasuryDonation", 15, tc.want},
			} {
				if path, exp, act, ok := firstDataDifference(check.name, check.want, fields[check.field]); !ok {
					t.Errorf("%s differs:\nexpected %s\nactual   %s", path, exp, act)
				}
			}
			if len(info.Redeemers) != len(scriptInfos) {
				t.Fatalf("%d redeemers, want %d", len(info.Redeemers), len(scriptInfos))
			}
			for i, pair := range info.Redeemers {
				got := pair.Key.ToScriptInfo().ToPlutusData()
				if path, exp, act, ok := firstDataDifference("ScriptInfo", scriptInfos[i], got); !ok {
					t.Errorf("redeemer %d %s differs:\nexpected %s\nactual   %s", i, path, exp, act)
				}
			}
		})
	}
}
