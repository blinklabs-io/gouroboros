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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

// The transactions below carry no inputs at all, so the only script purpose
// each one defines is the non-spending purpose under test. That isolates the
// gap issue #2250 records: ValidateRequiredRedeemers used to build its
// required set from RedeemerTagSpend alone, while cardano-ledger's
// hasExactSetOfRedeemers
// (eras/alonzo/impl/src/Cardano/Ledger/Alonzo/Rules/Utxow.hs) derives it from
// every purpose in scriptsNeeded -- getConwayScriptsNeeded
// (eras/conway/impl/src/Cardano/Ledger/Conway/UTxO.hs) covers spending,
// withdrawing, certifying, minting, voting and proposing.
//
// No upstream fixture exercises a script purpose missing its redeemer: the
// cardano-ledger, cardano-node and ouroboros-consensus fixtures mirrored under
// ouroboros-mock are protocol-parameter, genesis and golden-serialization
// vectors, none of which carry a witnessed-but-unredeemed Plutus purpose. The
// transactions here are constructed, with the collection each redeemer index
// names taken from the Haskell source cited on each case.

// purposeTestScript is the Plutus script every purpose below resolves to. It
// is supplied as an explicit witness, so ValidateScriptWitnesses is satisfied
// and only the redeemer requirement is under test.
func purposeTestScript() common.PlutusV3Script {
	return common.PlutusV3Script{0xaa, 0xbb, 0xcc}
}

func purposeTestWitnessSet(
	redeemers map[common.RedeemerKey]common.RedeemerValue,
) conway.ConwayTransactionWitnessSet {
	ws := conway.ConwayTransactionWitnessSet{
		WsPlutusV3Scripts: cbor.NewSetType(
			[]common.PlutusV3Script{purposeTestScript()},
			true,
		),
	}
	if redeemers != nil {
		ws.WsRedeemers = conway.ConwayRedeemers{Redeemers: redeemers}
	}
	return ws
}

func redeemerAt(
	tag common.RedeemerTag,
	index uint32,
) map[common.RedeemerKey]common.RedeemerValue {
	return map[common.RedeemerKey]common.RedeemerValue{
		{Tag: tag, Index: index}: {
			ExUnits: common.ExUnits{Steps: 1, Memory: 1},
		},
	}
}

// mintOfScript mints one asset under the test script's hash as its policy id,
// so getMintingScriptsNeeded's policy set
// (eras/alonzo/impl/src/Cardano/Ledger/Alonzo/UTxO.hs) contains exactly that
// script hash at index 0.
func mintOfScript() *common.MultiAsset[common.MultiAssetTypeMint] {
	mint := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeMint{
			common.Blake2b224(purposeTestScript().Hash()): {
				cbor.NewByteString([]byte("tok")): big.NewInt(1),
			},
		},
	)
	return &mint
}

func scriptStakeCredential() common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: common.Blake2b224(purposeTestScript().Hash()),
	}
}

// scriptRewardAddress builds the reward account whose credential is the test
// script, the shape getWithdrawingScriptsNeeded reads through
// credScriptHash.
func scriptRewardAddress(t *testing.T) *common.Address {
	t.Helper()
	addr, err := common.NewAddressFromParts(
		common.AddressTypeNoneScript,
		common.AddressNetworkTestnet,
		nil,
		purposeTestScript().Hash().Bytes(),
	)
	require.NoError(t, err)
	return &addr
}

func scriptDRepVoter() *common.Voter {
	voter := &common.Voter{Type: common.VoterTypeDRepScriptHash}
	copy(voter.Hash[:], purposeTestScript().Hash().Bytes())
	return voter
}

// scriptGuardrailsProposal is a parameter-change proposal carrying the test
// script as its guardrails policy, the only shape
// getConwayScriptsNeeded's getProposalScriptHash recognizes besides a
// treasury withdrawal.
func scriptGuardrailsProposal() conway.ConwayProposalProcedure {
	return conway.ConwayProposalProcedure{
		PPGovAction: conway.ConwayGovAction{
			Action: &conway.ConwayParameterChangeGovAction{
				Type:       0,
				PolicyHash: purposeTestScript().Hash().Bytes(),
			},
		},
	}
}

// TestValidateRequiredRedeemersNonSpendingPurposes is issue #2250's core
// case, one subtest per script purpose that had no required-redeemer check:
// the Plutus script is witnessed, the purpose needs it, and no redeemer names
// it. Every one of these transactions was accepted before the fix, leaving
// the script unexecuted.
func TestValidateRequiredRedeemersNonSpendingPurposes(t *testing.T) {
	s := purposeTestScript()
	ls := ledgerStateWithUtxos()

	for _, tc := range []struct {
		name string
		tag  common.RedeemerTag
		body conway.ConwayTransactionBody
	}{
		{
			name: "minting",
			tag:  common.RedeemerTagMint,
			body: conway.ConwayTransactionBody{TxMint: mintOfScript()},
		},
		{
			name: "certifying",
			tag:  common.RedeemerTagCert,
			body: conway.ConwayTransactionBody{
				TxCertificates: []common.CertificateWrapper{
					{
						Type: 1,
						Certificate: &common.StakeDeregistrationCertificate{
							StakeCredential: scriptStakeCredential(),
						},
					},
				},
			},
		},
		{
			name: "rewarding",
			tag:  common.RedeemerTagReward,
			body: conway.ConwayTransactionBody{
				TxWithdrawals: map[*common.Address]uint64{
					scriptRewardAddress(t): 42,
				},
			},
		},
		{
			name: "voting",
			tag:  common.RedeemerTagVoting,
			body: conway.ConwayTransactionBody{
				TxVotingProcedures: common.VotingProcedures{
					scriptDRepVoter(): map[*common.GovActionId]common.VotingProcedure{},
				},
			},
		},
		{
			name: "proposing",
			tag:  common.RedeemerTagProposing,
			body: conway.ConwayTransactionBody{
				TxProposalProcedures: []conway.ConwayProposalProcedure{
					scriptGuardrailsProposal(),
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			missingTx := &conway.ConwayTransaction{
				Body:       tc.body,
				WitnessSet: purposeTestWitnessSet(nil),
				TxIsValid:  true,
			}
			err := script.ValidateRequiredRedeemers(missingTx, ls)
			var missingErr common.MissingRedeemerForScriptError
			require.ErrorAs(t, err, &missingErr)
			require.Equal(t, s.Hash(), missingErr.ScriptHash)
			require.Equal(t, tc.tag, missingErr.Tag)
			require.Equal(t, uint32(0), missingErr.Index)

			// Negative control: the same transaction carrying the redeemer
			// its purpose names must still pass, so the check is requiring a
			// redeemer rather than rejecting the purpose.
			validTx := &conway.ConwayTransaction{
				Body:       tc.body,
				WitnessSet: purposeTestWitnessSet(redeemerAt(tc.tag, 0)),
				TxIsValid:  true,
			}
			require.NoError(t, script.ValidateRequiredRedeemers(validTx, ls))
		})
	}
}

// TestValidateRequiredRedeemersNativeScriptPurposeNeedsNone is the boundary
// of the widening: a native script backing the same minting policy requires
// no redeemer. hasExactSetOfRedeemers drops native scripts from
// redeemersNeeded with `not (isNativeScript script)`, so requiring one here
// would reject a transaction cardano-ledger accepts.
func TestValidateRequiredRedeemersNativeScriptPurposeNeedsNone(t *testing.T) {
	native := &common.NativeScript{}
	require.NoError(t, native.UnmarshalCBOR(nativeScriptAllCbor()))
	mint := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeMint{
			common.Blake2b224(native.Hash()): {
				cbor.NewByteString([]byte("tok")): big.NewInt(1),
			},
		},
	)
	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{TxMint: &mint},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsNativeScripts: cbor.NewSetType(
				[]common.NativeScript{*native},
				true,
			),
		},
		TxIsValid: true,
	}
	require.NoError(
		t,
		script.ValidateRequiredRedeemers(tx, ledgerStateWithUtxos()),
	)
}

// nativeScriptAllCbor is `ScriptAll []`, the smallest native script: CBOR
// array [1, []].
func nativeScriptAllCbor() []byte {
	return []byte{0x82, 0x01, 0x80}
}

// TestValidateRequiredRedeemersUnavailableScriptPurposeSkipped pins that a
// purpose whose script is not provided at all stays
// ValidateScriptWitnesses's failure to report. hasExactSetOfRedeemers only
// considers purposes with `Just script <- [Map.lookup sh scriptsProvided]`,
// so reporting a missing redeemer here would mask a missing script.
func TestValidateRequiredRedeemersUnavailableScriptPurposeSkipped(t *testing.T) {
	// The witness set provides an unrelated script, so Available is non-empty
	// and the walk actually runs, but nothing supplies the mint policy.
	other := common.PlutusV3Script{0x11, 0x22}
	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{TxMint: mintOfScript()},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsPlutusV3Scripts: cbor.NewSetType(
				[]common.PlutusV3Script{other},
				true,
			),
		},
		TxIsValid: true,
	}
	require.NoError(
		t,
		script.ValidateRequiredRedeemers(tx, ledgerStateWithUtxos()),
	)
}
