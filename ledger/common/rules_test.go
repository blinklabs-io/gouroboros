// Copyright 2026 Blink Labs Software

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

package common_test

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestValidateRequiredVKeyWitnesses_Common(t *testing.T) {
	tx := mockledger.NewTransactionBuilder()
	if err := common.ValidateRequiredVKeyWitnesses(tx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequiredVKeyWitnessesCertificateAndVoter(t *testing.T) {
	cred := common.Credential{CredType: common.CredentialTypeAddrKeyHash}
	cred.Credential[0] = 0x42
	cert := &common.DeregistrationCertificate{StakeCredential: cred}
	tx := mockledger.NewTransactionBuilder().WithCertificates(cert)
	if err := common.ValidateRequiredVKeyWitnesses(tx); err == nil {
		t.Fatal("expected missing witness for certificate credential")
	}

	vkey := []byte{0x01, 0x02, 0x03}
	witness := mockledger.NewMockTransactionWitnessSet().
		WithVkeyWitnesses(common.VkeyWitness{Vkey: vkey})
	// Use the actual hash represented by the witness so the positive path
	// proves the certificate requirement is satisfiable, not merely detected.
	cred.Credential = common.Blake2b224Hash(vkey)
	tx = mockledger.NewTransactionBuilder().
		WithCertificates(&common.DeregistrationCertificate{StakeCredential: cred}).
		WithWitnesses(witness)
	if err := common.ValidateRequiredVKeyWitnesses(tx); err != nil {
		t.Fatalf("expected certificate witness to satisfy requirement: %v", err)
	}

	voterHash := common.Blake2b224Hash([]byte{0x09, 0x08, 0x07})
	voter := &common.Voter{Type: common.VoterTypeDRepKeyHash, Hash: voterHash}
	tx = mockledger.NewTransactionBuilder().WithVotingProcedures(
		common.VotingProcedures{
			voter: {},
		},
	)
	if err := common.ValidateRequiredVKeyWitnesses(tx); err == nil {
		t.Fatal("expected missing witness for key-hash voter")
	}
}

func TestValidateRequiredVKeyWitnessesExplicitRegistration(t *testing.T) {
	vkey := []byte("explicit-registration-key")
	key := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash(vkey),
	}
	explicit := &common.RegistrationCertificate{
		StakeCredential: key,
		Amount:          0,
	}
	// Types 0 and 7 both decode to ConwayRegCert, whose
	// getVKeyWitnessTxCert returns Nothing, so neither form requires the
	// credential's signature.
	tx := mockledger.NewTransactionBuilder().WithCertificates(explicit)
	require.NoError(
		t,
		conway.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil),
	)
	tx.WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
			common.VkeyWitness{Vkey: vkey},
		),
	)
	require.NoError(
		t,
		conway.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil),
	)

	legacy := &common.StakeRegistrationCertificate{StakeCredential: key}
	require.NoError(t, conway.UtxoValidateRequiredVKeyWitnesses(
		mockledger.NewTransactionBuilder().WithCertificates(legacy),
		0,
		nil,
		nil,
	))
}

func TestValidateScriptWitnessesExplicitRegistration(t *testing.T) {
	nativeCbor, err := cbor.Encode([]any{uint64(1), []any{}})
	require.NoError(t, err)
	var nativeScript common.NativeScript
	_, err = cbor.Decode(nativeCbor, &nativeScript)
	require.NoError(t, err)
	credential := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: common.Blake2b224(nativeScript.Hash()),
	}
	tx := mockledger.NewTransactionBuilder().WithCertificates(
		&common.RegistrationCertificate{
			StakeCredential: credential,
			Amount:          0,
		},
	)
	ledgerState := mockledger.NewLedgerStateBuilder().Build()
	// Registration authorizes nothing, so its script credential creates no
	// script purpose and needs no witness. Supplying the script anyway is
	// accepted rather than extraneous; conformance vectors carry that shape.
	require.NoError(
		t,
		conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil),
	)
	tx.WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().
			WithNativeScripts(nativeScript),
	)
	require.NoError(
		t,
		conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil),
	)

	legacy := mockledger.NewTransactionBuilder().WithCertificates(
		&common.StakeRegistrationCertificate{StakeCredential: credential},
	)
	require.NoError(
		t,
		conway.UtxoValidateScriptWitnesses(legacy, 0, ledgerState, nil),
	)
}

func testAuthorizationNativeScript(
	t *testing.T,
	vkey []byte,
) common.NativeScript {
	t.Helper()
	scriptCbor, err := cbor.Encode([]any{
		uint64(0),
		common.Blake2b224Hash(vkey).Bytes(),
	})
	require.NoError(t, err)
	var script common.NativeScript
	_, err = cbor.Decode(scriptCbor, &script)
	require.NoError(t, err)
	return script
}

func testAuthorizationVkeyWitness(vkey []byte) common.VkeyWitness {
	return common.VkeyWitness{Vkey: vkey}
}

func TestCertificateAuthorizationCompleteness(t *testing.T) {
	type credentialCertificateCase struct {
		name            string
		certificateType common.CertificateType
		requiresWitness bool
		certificate     func(common.Credential) common.Certificate
	}
	cases := []credentialCertificateCase{
		{
			name:            "legacy stake registration",
			certificateType: common.CertificateTypeStakeRegistration,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.StakeRegistrationCertificate{
					CertType:        uint(common.CertificateTypeStakeRegistration),
					StakeCredential: credential,
				}
			},
		},
		{
			name:            "stake deregistration",
			certificateType: common.CertificateTypeStakeDeregistration,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.StakeDeregistrationCertificate{
					CertType:        uint(common.CertificateTypeStakeDeregistration),
					StakeCredential: credential,
				}
			},
		},
		{
			name:            "stake delegation",
			certificateType: common.CertificateTypeStakeDelegation,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.StakeDelegationCertificate{
					CertType:        uint(common.CertificateTypeStakeDelegation),
					StakeCredential: &credential,
				}
			},
		},
		{
			// ConwayRegCert covers types 0 and 7 and authorizes neither.
			name:            "explicit registration",
			certificateType: common.CertificateTypeRegistration,
			requiresWitness: false,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.RegistrationCertificate{
					CertType:        uint(common.CertificateTypeRegistration),
					StakeCredential: credential,
				}
			},
		},
		{
			name:            "explicit deregistration",
			certificateType: common.CertificateTypeDeregistration,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.DeregistrationCertificate{
					CertType:        uint(common.CertificateTypeDeregistration),
					StakeCredential: credential,
				}
			},
		},
		{
			name:            "vote delegation",
			certificateType: common.CertificateTypeVoteDelegation,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.VoteDelegationCertificate{
					CertType:        uint(common.CertificateTypeVoteDelegation),
					StakeCredential: credential,
				}
			},
		},
		{
			name:            "stake vote delegation",
			certificateType: common.CertificateTypeStakeVoteDelegation,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.StakeVoteDelegationCertificate{
					CertType:        uint(common.CertificateTypeStakeVoteDelegation),
					StakeCredential: credential,
				}
			},
		},
		{
			name:            "stake registration delegation",
			certificateType: common.CertificateTypeStakeRegistrationDelegation,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.StakeRegistrationDelegationCertificate{
					CertType: uint(
						common.CertificateTypeStakeRegistrationDelegation,
					),
					StakeCredential: credential,
				}
			},
		},
		{
			name:            "vote registration delegation",
			certificateType: common.CertificateTypeVoteRegistrationDelegation,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.VoteRegistrationDelegationCertificate{
					CertType: uint(
						common.CertificateTypeVoteRegistrationDelegation,
					),
					StakeCredential: credential,
				}
			},
		},
		{
			name:            "stake vote registration delegation",
			certificateType: common.CertificateTypeStakeVoteRegistrationDelegation,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.StakeVoteRegistrationDelegationCertificate{
					CertType: uint(
						common.CertificateTypeStakeVoteRegistrationDelegation,
					),
					StakeCredential: credential,
				}
			},
		},
		{
			name:            "committee hot authorization",
			certificateType: common.CertificateTypeAuthCommitteeHot,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.AuthCommitteeHotCertificate{
					CertType:       uint(common.CertificateTypeAuthCommitteeHot),
					ColdCredential: credential,
				}
			},
		},
		{
			name:            "committee cold resignation",
			certificateType: common.CertificateTypeResignCommitteeCold,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.ResignCommitteeColdCertificate{
					CertType:       uint(common.CertificateTypeResignCommitteeCold),
					ColdCredential: credential,
				}
			},
		},
		{
			name:            "DRep registration",
			certificateType: common.CertificateTypeRegistrationDrep,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.RegistrationDrepCertificate{
					CertType:       uint(common.CertificateTypeRegistrationDrep),
					DrepCredential: credential,
				}
			},
		},
		{
			name:            "DRep deregistration",
			certificateType: common.CertificateTypeDeregistrationDrep,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.DeregistrationDrepCertificate{
					CertType:       uint(common.CertificateTypeDeregistrationDrep),
					DrepCredential: credential,
				}
			},
		},
		{
			name:            "DRep update",
			certificateType: common.CertificateTypeUpdateDrep,
			requiresWitness: true,
			certificate: func(credential common.Credential) common.Certificate {
				return &common.UpdateDrepCertificate{
					CertType:       uint(common.CertificateTypeUpdateDrep),
					DrepCredential: credential,
				}
			},
		},
	}

	covered := make(map[common.CertificateType]struct{}, len(cases)+4)
	ledgerState := mockledger.NewLedgerStateBuilder().Build()
	for _, testCase := range cases {
		covered[testCase.certificateType] = struct{}{}
		t.Run(testCase.name+"/key", func(t *testing.T) {
			vkey := []byte("certificate-author-key-" + testCase.name)
			credential := common.Credential{
				CredType:   common.CredentialTypeAddrKeyHash,
				Credential: common.Blake2b224Hash(vkey),
			}
			tx := mockledger.NewTransactionBuilder().WithCertificates(
				testCase.certificate(credential),
			).WithWitnesses(
				mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
					testAuthorizationVkeyWitness([]byte("unrelated-key")),
				),
			)
			err := conway.UtxoValidateRequiredVKeyWitnesses(tx, 0, ledgerState, nil)
			if testCase.requiresWitness {
				require.ErrorAs(
					t,
					err,
					&common.MissingRequiredVKeyWitnessForSignerError{},
				)
			} else {
				require.NoError(t, err)
			}
			tx.WithWitnesses(
				mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
					testAuthorizationVkeyWitness(vkey),
				),
			)
			require.NoError(
				t,
				conway.UtxoValidateRequiredVKeyWitnesses(tx, 0, ledgerState, nil),
			)
		})

		t.Run(testCase.name+"/script", func(t *testing.T) {
			vkey := []byte("certificate-script-key-" + testCase.name)
			native := testAuthorizationNativeScript(t, vkey)
			credential := common.Credential{
				CredType:   common.CredentialTypeScriptHash,
				Credential: common.Blake2b224(native.Hash()),
			}
			tx := mockledger.NewTransactionBuilder().WithCertificates(
				testCase.certificate(credential),
			)
			err := conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil)
			if testCase.requiresWitness {
				require.ErrorAs(t, err, &common.MissingScriptWitnessesError{})
			} else {
				require.NoError(t, err)
			}
			tx.WithWitnesses(
				mockledger.NewMockTransactionWitnessSet().
					WithNativeScripts(native).
					WithVkeyWitnesses(testAuthorizationVkeyWitness(vkey)),
			)
			err = conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil)
			// A certificate that authorizes nothing creates no script purpose,
			// so its script is optional: not required, and accepted when
			// supplied rather than reported as extraneous.
			require.NoError(t, err)
			if !testCase.requiresWitness {
				return
			}
			require.NoError(
				t,
				conway.UtxoValidateNativeScripts(tx, 0, ledgerState, nil),
			)
		})
	}

	covered[common.CertificateTypePoolRegistration] = struct{}{}
	covered[common.CertificateTypePoolRetirement] = struct{}{}
	covered[common.CertificateTypeGenesisKeyDelegation] = struct{}{}
	covered[common.CertificateTypeMoveInstantaneousRewards] = struct{}{}
	for certType := common.CertificateTypeStakeRegistration; certType <= common.CertificateTypeUpdateDrep; certType++ {
		require.Contains(t, covered, certType, "certificate type %d", certType)
	}
}

func TestPoolAndGenesisCertificateAuthorization(t *testing.T) {
	operatorVkey := []byte("pool-operator")
	ownerOneVkey := []byte("pool-owner-one")
	ownerTwoVkey := []byte("pool-owner-two")
	authors := []struct {
		name string
		vkey []byte
	}{
		{name: "operator", vkey: operatorVkey},
		{name: "owner one", vkey: ownerOneVkey},
		{name: "owner two", vkey: ownerTwoVkey},
	}
	pool := &common.PoolRegistrationCertificate{
		CertType: uint(common.CertificateTypePoolRegistration),
		Operator: common.PoolKeyHash(common.Blake2b224Hash(operatorVkey)),
		PoolOwners: []common.AddrKeyHash{
			common.AddrKeyHash(common.Blake2b224Hash(ownerOneVkey)),
			common.AddrKeyHash(common.Blake2b224Hash(ownerTwoVkey)),
		},
	}
	for omitted := range authors {
		t.Run("pool registration missing "+authors[omitted].name, func(t *testing.T) {
			witnesses := mockledger.NewMockTransactionWitnessSet()
			for index, author := range authors {
				if index != omitted {
					witnesses.WithVkeyWitnesses(
						testAuthorizationVkeyWitness(author.vkey),
					)
				}
			}
			tx := mockledger.NewTransactionBuilder().
				WithCertificates(pool).
				WithWitnesses(witnesses)
			var missing common.MissingRequiredVKeyWitnessForSignerError
			require.ErrorAs(t, common.ValidateRequiredVKeyWitnesses(tx), &missing)
			require.Equal(t, common.Blake2b224Hash(authors[omitted].vkey), missing.Signer)
		})
	}
	allWitnesses := mockledger.NewMockTransactionWitnessSet()
	for _, author := range authors {
		allWitnesses.WithVkeyWitnesses(testAuthorizationVkeyWitness(author.vkey))
	}
	require.NoError(t, common.ValidateRequiredVKeyWitnesses(
		mockledger.NewTransactionBuilder().
			WithCertificates(pool).
			WithWitnesses(allWitnesses),
	))

	t.Run("pool retirement requires pool key", func(t *testing.T) {
		cert := &common.PoolRetirementCertificate{
			CertType:    uint(common.CertificateTypePoolRetirement),
			PoolKeyHash: common.PoolKeyHash(common.Blake2b224Hash(operatorVkey)),
		}
		tx := mockledger.NewTransactionBuilder().WithCertificates(cert)
		require.Error(t, common.ValidateRequiredVKeyWitnesses(tx))
		tx.WithWitnesses(
			mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
				testAuthorizationVkeyWitness(operatorVkey),
			),
		)
		require.NoError(t, common.ValidateRequiredVKeyWitnesses(tx))
	})

	t.Run("genesis source key authorizes delegation", func(t *testing.T) {
		genesisVkey := []byte("genesis-source-key")
		delegateVkey := []byte("genesis-delegate-target")
		cert := &common.GenesisKeyDelegationCertificate{
			CertType:            uint(common.CertificateTypeGenesisKeyDelegation),
			GenesisHash:         common.Blake2b224Hash(genesisVkey).Bytes(),
			GenesisDelegateHash: common.Blake2b224Hash(delegateVkey).Bytes(),
		}
		tx := mockledger.NewTransactionBuilder().WithCertificates(cert)
		require.Error(t, common.ValidateRequiredVKeyWitnesses(tx))
		tx.WithWitnesses(
			mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
				testAuthorizationVkeyWitness(delegateVkey),
			),
		)
		require.Error(t, common.ValidateRequiredVKeyWitnesses(tx))
		tx.WithWitnesses(
			mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
				testAuthorizationVkeyWitness(genesisVkey),
			),
		)
		require.NoError(t, common.ValidateRequiredVKeyWitnesses(tx))
	})

	t.Run("MIR has no field-level author", func(t *testing.T) {
		// MIR authorization is a stateful genesis-delegate quorum enforced by
		// ValidateMIRGenesisQuorum, not a certificate credential. See
		// TestValidateMIRGenesisQuorum.
		cert := &common.MoveInstantaneousRewardsCertificate{
			CertType: uint(common.CertificateTypeMoveInstantaneousRewards),
		}
		require.NoError(t, common.ValidateRequiredVKeyWitnesses(
			mockledger.NewTransactionBuilder().WithCertificates(cert),
		))
	})
}

func TestVoterAuthorizationCompleteness(t *testing.T) {
	ledgerState := mockledger.NewLedgerStateBuilder().Build()
	voterTypes := []struct {
		name       string
		voterType  uint8
		scriptForm bool
	}{
		{"committee hot key", common.VoterTypeConstitutionalCommitteeHotKeyHash, false},
		{"committee hot script", common.VoterTypeConstitutionalCommitteeHotScriptHash, true},
		{"DRep key", common.VoterTypeDRepKeyHash, false},
		{"DRep script", common.VoterTypeDRepScriptHash, true},
		{"stake pool key", common.VoterTypeStakingPoolKeyHash, false},
	}
	for _, testCase := range voterTypes {
		t.Run(testCase.name, func(t *testing.T) {
			vkey := []byte("voter-author-" + testCase.name)
			voter := &common.Voter{Type: testCase.voterType}
			if testCase.scriptForm {
				native := testAuthorizationNativeScript(t, vkey)
				nativeHash := native.Hash()
				copy(voter.Hash[:], nativeHash[:])
				tx := mockledger.NewTransactionBuilder().WithVotingProcedures(
					common.VotingProcedures{voter: {}},
				)
				require.ErrorAs(
					t,
					conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil),
					&common.MissingScriptWitnessesError{},
				)
				tx.WithWitnesses(
					mockledger.NewMockTransactionWitnessSet().
						WithNativeScripts(native).
						WithVkeyWitnesses(testAuthorizationVkeyWitness(vkey)),
				)
				require.NoError(
					t,
					conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil),
				)
				require.NoError(
					t,
					conway.UtxoValidateNativeScripts(tx, 0, ledgerState, nil),
				)
				return
			}
			voterHash := common.Blake2b224Hash(vkey)
			copy(voter.Hash[:], voterHash[:])
			tx := mockledger.NewTransactionBuilder().WithVotingProcedures(
				common.VotingProcedures{voter: {}},
			).WithWitnesses(
				mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
					testAuthorizationVkeyWitness([]byte("unrelated-voter-key")),
				),
			)
			require.ErrorAs(
				t,
				common.ValidateRequiredVKeyWitnesses(tx),
				&common.MissingRequiredVKeyWitnessForSignerError{},
			)
			tx.WithWitnesses(
				mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
					testAuthorizationVkeyWitness(vkey),
				),
			)
			require.NoError(t, common.ValidateRequiredVKeyWitnesses(tx))
		})
	}
}

func TestScriptAuthorizationRequiresExactRedeemerPurpose(t *testing.T) {
	plutus := common.PlutusV3Script([]byte{0x41, 0x00})
	scriptRefCbor, err := cbor.Encode(&common.ScriptRef{
		Type:   common.ScriptRefTypePlutusV3,
		Script: plutus,
	})
	require.NoError(t, err)
	const address = "addr_test1vqg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zygxrcya6"
	utxo, err := mockledger.NewUtxoBuilder().
		WithTxId(bytes.Repeat([]byte{0x20}, 32)).
		WithIndex(0).
		WithAddress(address).
		WithLovelace(2_000_000).
		WithScriptRef(scriptRefCbor).
		Build()
	require.NoError(t, err)
	ledgerState := mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(common.TransactionInput) (common.Utxo, error) {
			return utxo, nil
		},
	).Build()
	credential := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: common.Blake2b224(plutus.Hash()),
	}
	certificates := []common.Certificate{
		&common.DeregistrationCertificate{StakeCredential: credential},
		&common.UpdateDrepCertificate{DrepCredential: credential},
	}
	redeemers := conway.ConwayRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{
		{Tag: common.RedeemerTagCert, Index: 0}: {},
	}}
	witnesses := mockledger.NewMockTransactionWitnessSet().WithRedeemers(redeemers)
	tx := mockledger.NewTransactionBuilder().
		WithCertificates(certificates...).
		WithReferenceInputs(utxo.Id).
		WithWitnesses(witnesses)
	var missing common.MissingRedeemerForScriptError
	require.ErrorAs(
		t,
		conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil),
		&missing,
	)
	require.Equal(
		t,
		common.RedeemerKey{Tag: common.RedeemerTagCert, Index: 1},
		missing.RedeemerKey,
	)
	redeemers.Redeemers[common.RedeemerKey{
		Tag: common.RedeemerTagCert, Index: 1,
	}] = common.RedeemerValue{}
	require.NoError(
		t,
		conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil),
	)
}

func TestVoterScriptAuthorizationRequiresExactRedeemerPurpose(t *testing.T) {
	plutus := common.PlutusV3Script([]byte{0x41, 0x01})
	scriptHash := plutus.Hash()
	committeeScript := &common.Voter{
		Type: common.VoterTypeConstitutionalCommitteeHotScriptHash,
	}
	copy(committeeScript.Hash[:], scriptHash[:])
	committeeKey := &common.Voter{
		Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
	}
	committeeKey.Hash[0] = 0x42
	drepScript := &common.Voter{Type: common.VoterTypeDRepScriptHash}
	copy(drepScript.Hash[:], scriptHash[:])
	redeemers := conway.ConwayRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{
		{Tag: common.RedeemerTagVoting, Index: 0}: {},
	}}
	tx := mockledger.NewTransactionBuilder().
		WithVotingProcedures(common.VotingProcedures{
			committeeScript: {},
			committeeKey:    {},
			drepScript:      {},
		}).
		WithWitnesses(
			mockledger.NewMockTransactionWitnessSet().
				WithPlutusV3Scripts(plutus).
				WithRedeemers(redeemers),
		)
	ledgerState := mockledger.NewLedgerStateBuilder().Build()
	var missing common.MissingRedeemerForScriptError
	require.ErrorAs(
		t,
		conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil),
		&missing,
	)
	require.Equal(
		t,
		common.RedeemerKey{Tag: common.RedeemerTagVoting, Index: 2},
		missing.RedeemerKey,
	)
	redeemers.Redeemers[common.RedeemerKey{
		Tag: common.RedeemerTagVoting, Index: 2,
	}] = common.RedeemerValue{}
	require.NoError(
		t,
		conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil),
	)
}

func TestNativeScriptAuthorizationRejectsRedeemer(t *testing.T) {
	vkey := []byte("native-purpose-key")
	native := testAuthorizationNativeScript(t, vkey)
	credential := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: common.Blake2b224(native.Hash()),
	}
	redeemers := conway.ConwayRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{
		{Tag: common.RedeemerTagCert, Index: 0}: {},
	}}
	tx := mockledger.NewTransactionBuilder().
		WithCertificates(&common.DeregistrationCertificate{StakeCredential: credential}).
		WithWitnesses(
			mockledger.NewMockTransactionWitnessSet().
				WithNativeScripts(native).
				WithRedeemers(redeemers),
		)
	var extra common.ExtraneousRedeemerError
	require.ErrorAs(
		t,
		conway.UtxoValidateScriptWitnesses(
			tx,
			0,
			mockledger.NewLedgerStateBuilder().Build(),
			nil,
		),
		&extra,
	)
	require.Equal(
		t,
		common.RedeemerKey{Tag: common.RedeemerTagCert, Index: 0},
		extra.RedeemerKey,
	)
}

func TestRequiredNativeReferenceScriptIsEvaluated(t *testing.T) {
	vkey := []byte("reference-native-key")
	native := testAuthorizationNativeScript(t, vkey)
	scriptRefCbor, err := cbor.Encode(&common.ScriptRef{
		Type:   common.ScriptRefTypeNativeScript,
		Script: native,
	})
	require.NoError(t, err)
	const address = "addr_test1vqg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zygxrcya6"
	utxo, err := mockledger.NewUtxoBuilder().
		WithTxId(bytes.Repeat([]byte{0x21}, 32)).
		WithIndex(0).
		WithAddress(address).
		WithLovelace(2_000_000).
		WithScriptRef(scriptRefCbor).
		Build()
	require.NoError(t, err)
	ledgerState := mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(common.TransactionInput) (common.Utxo, error) {
			return utxo, nil
		},
	).Build()
	credential := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: common.Blake2b224(native.Hash()),
	}
	tx := mockledger.NewTransactionBuilder().
		WithCertificates(&common.DeregistrationCertificate{StakeCredential: credential}).
		WithReferenceInputs(utxo.Id)
	require.NoError(
		t,
		conway.UtxoValidateScriptWitnesses(tx, 0, ledgerState, nil),
	)
	var failed conway.NativeScriptFailedError
	require.ErrorAs(
		t,
		conway.UtxoValidateNativeScripts(tx, 0, ledgerState, nil),
		&failed,
	)
	tx.WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
			testAuthorizationVkeyWitness(vkey),
		),
	)
	require.NoError(
		t,
		conway.UtxoValidateNativeScripts(tx, 0, ledgerState, nil),
	)
}

func TestMalformedAuthorizationSubjectsFailClosed(t *testing.T) {
	tests := []struct {
		name string
		tx   *mockledger.MockTransaction
	}{
		{
			name: "typed nil certificate",
			tx: mockledger.NewTransactionBuilder().WithCertificates(
				(*common.DeregistrationCertificate)(nil),
			),
		},
		{
			name: "nil stake delegation credential",
			tx: mockledger.NewTransactionBuilder().WithCertificates(
				&common.StakeDelegationCertificate{},
			),
		},
		{
			name: "invalid credential tag",
			tx: mockledger.NewTransactionBuilder().WithCertificates(
				&common.DeregistrationCertificate{StakeCredential: common.Credential{
					CredType: 2,
				}},
			),
		},
		{
			name: "nil voter",
			tx: mockledger.NewTransactionBuilder().WithVotingProcedures(
				common.VotingProcedures{nil: {}},
			),
		},
		{
			name: "invalid voter tag",
			tx: mockledger.NewTransactionBuilder().WithVotingProcedures(
				common.VotingProcedures{&common.Voter{Type: 5}: {}},
			),
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			for _, validation := range []struct {
				name string
				run  func() error
			}{
				{
					name: "key witnesses",
					run: func() error {
						return common.ValidateRequiredVKeyWitnesses(testCase.tx)
					},
				},
				{
					name: "script witnesses",
					run: func() error {
						return common.ValidateScriptWitnesses(
							testCase.tx,
							mockledger.NewLedgerStateBuilder().Build(),
						)
					},
				},
			} {
				t.Run(validation.name, func(t *testing.T) {
					var malformed common.MalformedAuthorizationError
					require.ErrorAs(t, validation.run(), &malformed)
				})
			}
		})
	}
}

func TestValidateRedeemerAndScriptWitnesses_Common(t *testing.T) {
	tx := mockledger.NewTransactionBuilder()
	if err := common.ValidateRedeemerAndScriptWitnesses(tx, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEncodeLangViews(t *testing.T) {
	t.Run("encodes_versions_in_shortlex_order", func(t *testing.T) {
		usedVersions := map[uint]struct{}{
			2: {},
			0: {},
			1: {},
		}
		costModels := map[uint][]int64{
			0: {10, 11},
			1: {20, 21},
			2: {30, 31},
		}

		got, err := common.EncodeLangViews(usedVersions, costModels)
		require.NoError(t, err)

		v1List, err := cbor.Encode(cbor.IndefLengthList{int64(10), int64(11)})
		require.NoError(t, err)
		v1Params, err := cbor.Encode(v1List)
		require.NoError(t, err)
		v2Params, err := cbor.Encode([]int64{20, 21})
		require.NoError(t, err)
		v3Params, err := cbor.Encode([]int64{30, 31})
		require.NoError(t, err)

		want := append([]byte{0xa3, 0x01}, v2Params...)
		want = append(want, 0x02)
		want = append(want, v3Params...)
		want = append(want, 0x41, 0x00)
		want = append(want, v1Params...)

		require.Equal(t, want, got)
	})

	t.Run("rejects_unsupported_versions", func(t *testing.T) {
		_, err := common.EncodeLangViews(
			map[uint]struct{}{4: {}},
			map[uint][]int64{4: {1}},
		)
		require.Error(t, err)
	})

	t.Run("rejects_unsupported_versions_without_cost_model", func(t *testing.T) {
		_, err := common.EncodeLangViews(
			map[uint]struct{}{4: {}},
			map[uint][]int64{},
		)
		require.Error(t, err)
	})

	t.Run("rejects_missing_cost_model_for_supported_version", func(t *testing.T) {
		_, err := common.EncodeLangViews(
			map[uint]struct{}{2: {}},
			map[uint][]int64{},
		)
		require.Error(t, err)
	})

	// Forward-compat for PV11 (vanRossem) and later: cost models may grow as
	// new Plutus builtins are added. The langview hash is part of
	// ScriptDataHash, so any silent truncation here would invalidate every
	// Plutus tx after a hard fork. Encode arbitrary-length arrays for V2 and
	// V3 and assert the produced bytes round-trip the full slice.
	t.Run("encodes_longer_cost_models_for_v2_and_v3", func(t *testing.T) {
		v2 := make([]int64, 220)
		v3 := make([]int64, 350)
		for i := range v2 {
			v2[i] = int64(i + 1)
		}
		for i := range v3 {
			v3[i] = int64(i + 100_000)
		}

		got, err := common.EncodeLangViews(
			map[uint]struct{}{1: {}, 2: {}},
			map[uint][]int64{1: v2, 2: v3},
		)
		require.NoError(t, err)

		v2Params, err := cbor.Encode(v2)
		require.NoError(t, err)
		v3Params, err := cbor.Encode(v3)
		require.NoError(t, err)

		// Map header for 2 entries, then tag 0x01 + v2 params, tag 0x02 + v3 params.
		want := append([]byte{0xa2, 0x01}, v2Params...)
		want = append(want, 0x02)
		want = append(want, v3Params...)

		require.Equal(t, want, got)
	})

	t.Run("encodes_v4_language_view", func(t *testing.T) {
		got, err := common.EncodeLangViews(
			map[uint]struct{}{3: {}},
			map[uint][]int64{3: {40, 41}},
		)
		require.NoError(t, err)

		v4Params, err := cbor.Encode([]int64{40, 41})
		require.NoError(t, err)

		want := append([]byte{0xa1, 0x03}, v4Params...)
		require.Equal(t, want, got)
	})
}

func TestTxSizeForFeeDijkstraEnvelope(t *testing.T) {
	txBody := map[uint]any{
		0: []any{},
		1: []any{},
		2: uint64(0),
	}
	witnessSet := map[uint]any{}
	auxData := any(nil)

	threePartCbor, err := cbor.Encode([]any{txBody, witnessSet, auxData})
	require.NoError(t, err)
	threePartTx := mockledger.NewTransactionBuilder().
		WithType(dijkstra.TxTypeDijkstra)
	threePartTx.SetCbor(threePartCbor)

	size, err := common.TxSizeForFee(threePartTx)
	require.NoError(t, err)
	require.Equal(t, len(threePartCbor), size)

	fourPartCbor, err := cbor.Encode([]any{txBody, witnessSet, true, auxData})
	require.NoError(t, err)
	fourPartTx := mockledger.NewTransactionBuilder().
		WithType(dijkstra.TxTypeDijkstra)
	fourPartTx.SetCbor(fourPartCbor)

	size, err = common.TxSizeForFee(fourPartTx)
	require.NoError(t, err)
	require.Equal(t, len(fourPartCbor)-1, size)

	malformedTx := mockledger.NewTransactionBuilder().
		WithType(dijkstra.TxTypeDijkstra)
	malformedCbor := []byte{0xff}
	malformedTx.SetCbor(malformedCbor)

	size, err = common.TxSizeForFee(malformedTx)
	require.NoError(t, err)
	require.Equal(t, len(malformedCbor), size)
}

// Tests for VerifyTransaction moved from verify_rules_test.go
func TestVerifyTransaction(t *testing.T) {
	var tx common.Transaction

	slot := uint64(1000)
	ledgerState := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) { return common.Utxo{}, nil }).
		Build()
	protocolParams := &mockledger.MockProtocolParamsRules{}

	t.Run("all_rules_pass", func(t *testing.T) {
		rules := []common.UtxoValidationRuleFunc{
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
		}

		err := common.VerifyTransaction(
			tx,
			slot,
			ledgerState,
			protocolParams,
			rules,
		)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("first_rule_fails", func(t *testing.T) {
		expectedErr := errors.New("first rule failed")
		rules := []common.UtxoValidationRuleFunc{
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error {
				return expectedErr
			},
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
		}

		err := common.VerifyTransaction(
			tx,
			slot,
			ledgerState,
			protocolParams,
			rules,
		)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var validationErr *common.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("expected ValidationError, got %T", err)
		}
		if validationErr.Cause != expectedErr {
			t.Errorf(
				"expected cause %v, got %v",
				expectedErr,
				validationErr.Cause,
			)
		}
	})

	t.Run("middle_rule_fails", func(t *testing.T) {
		expectedErr := errors.New("middle rule failed")
		rules := []common.UtxoValidationRuleFunc{
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error {
				return expectedErr
			},
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
		}

		err := common.VerifyTransaction(
			tx,
			slot,
			ledgerState,
			protocolParams,
			rules,
		)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var validationErr *common.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("expected ValidationError, got %T", err)
		}
		if validationErr.Cause != expectedErr {
			t.Errorf(
				"expected cause %v, got %v",
				expectedErr,
				validationErr.Cause,
			)
		}
	})

	t.Run("last_rule_fails", func(t *testing.T) {
		expectedErr := errors.New("last rule failed")
		rules := []common.UtxoValidationRuleFunc{
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error { return nil },
			func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error {
				return expectedErr
			},
		}

		err := common.VerifyTransaction(
			tx,
			slot,
			ledgerState,
			protocolParams,
			rules,
		)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var validationErr *common.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("expected ValidationError, got %T", err)
		}
		if validationErr.Cause != expectedErr {
			t.Errorf(
				"expected cause %v, got %v",
				expectedErr,
				validationErr.Cause,
			)
		}
	})

	t.Run("empty_rules", func(t *testing.T) {
		rules := []common.UtxoValidationRuleFunc{}

		err := common.VerifyTransaction(
			tx,
			slot,
			ledgerState,
			protocolParams,
			rules,
		)
		if err != nil {
			t.Errorf("expected no error with empty rules, got %v", err)
		}
	})
}

// Use centralized mocks from ledger/common/mock.go

// mockTxInput implements TransactionInput minimally for constructing
// ReferenceInputResolutionError in tests.
type mockTxInput struct{ id common.Blake2b256 }

func (m *mockTxInput) Id() common.Blake2b256 { return m.id }
func (m *mockTxInput) Index() uint32         { return 0 }

func (m *mockTxInput) String() string                     { return m.id.String() }
func (m *mockTxInput) Utxorpc() (*cardano.TxInput, error) { return nil, nil }
func (m *mockTxInput) ToPlutusData() data.PlutusData      { return nil }

func TestReferenceInputResolutionSentinel(t *testing.T) {
	// Construct ReferenceInputResolutionError using the concrete type defined
	// in common/errors.go. Provide a minimal mock input and inner error.
	inner := errors.New("utxo not found")
	rie := common.ReferenceInputResolutionError{
		Input: &mockTxInput{id: common.Blake2b256{}},
		Err:   inner,
	}
	err := rie

	if !errors.Is(err, common.ErrReferenceInputResolution) {
		t.Fatalf("expected errors.Is to match ErrReferenceInputResolution")
	}

	var out common.ReferenceInputResolutionError
	if !errors.As(err, &out) {
		t.Fatalf(
			"expected errors.As to unwrap to ReferenceInputResolutionError",
		)
	}

	if out.Err == nil || out.Err.Error() != "utxo not found" {
		t.Fatalf("expected inner message 'utxo not found', got %q", out.Err)
	}
}

func TestCalculateMinFee(t *testing.T) {
	t.Run("normal_parameters", func(t *testing.T) {
		// Typical mainnet values: minFeeA=44, minFeeB=155381, bodySize=300
		fee, err := common.CalculateMinFee(300, 44, 155381)
		require.NoError(t, err)
		require.Equal(t, uint64(44*300+155381), fee)
	})

	t.Run("zero_values", func(t *testing.T) {
		fee, err := common.CalculateMinFee(0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(0), fee)
	})

	t.Run("multiplication_overflow", func(t *testing.T) {
		// Choose minFeeA and bodySize whose product exceeds math.MaxUint64.
		// math.MaxUint64 ≈ 1.8e19, so (1<<32+1) * (1<<32+1) > 2^64.
		bigA := uint(1<<32 + 1)
		bigSize := int(1<<32 + 1)
		_, err := common.CalculateMinFee(bigSize, bigA, 0)
		require.Error(t, err)
		require.Contains(t, err.Error(), "overflow")
	})

	t.Run("addition_overflow", func(t *testing.T) {
		// Product fits but adding minFeeB pushes past MaxUint64.
		fee, err := common.CalculateMinFee(1, uint(math.MaxUint64), 0)
		require.NoError(t, err)
		require.Equal(t, uint64(math.MaxUint64), fee)

		_, err = common.CalculateMinFee(1, uint(math.MaxUint64), 1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "overflow")
	})
}

func TestValidateExtraneousRedeemers_Common(t *testing.T) {
	testInput := shelley.NewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000001",
		0,
	)
	votingVoter := &common.Voter{
		Type: common.VoterTypeDRepKeyHash,
		Hash: common.Blake2b224{0x10},
	}
	votingGovActionId := &common.GovActionId{}

	baseBody := func() conway.ConwayTransactionBody {
		return conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{testInput},
			),
			TxCertificates: []common.CertificateWrapper{
				{
					Type: uint(common.CertificateTypeStakeRegistration),
					Certificate: &common.StakeRegistrationCertificate{
						StakeCredential: common.Credential{},
					},
				},
			},
			TxWithdrawals: map[*common.Address]uint64{
				{}: 0,
			},
			TxVotingProcedures: common.VotingProcedures{
				votingVoter: {
					votingGovActionId: common.VotingProcedure{Vote: 1},
				},
			},
			TxProposalProcedures: []conway.ConwayProposalProcedure{{}},
		}
	}

	t.Run("no witnesses is valid", func(t *testing.T) {
		tx := &conway.ConwayTransaction{Body: baseBody()}
		require.NoError(t, common.ValidateExtraneousRedeemers(tx))
	})

	t.Run("unknown tag is extraneous", func(t *testing.T) {
		tx := &conway.ConwayTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTag(99)}: {},
			},
		}
		err := common.ValidateExtraneousRedeemers(tx)
		require.Error(t, err)
		var extraErr common.ExtraneousRedeemerError
		require.ErrorAs(t, err, &extraErr)
	})

	t.Run("guarding tag is extraneous", func(t *testing.T) {
		tx := &conway.ConwayTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagGuarding}: {},
			},
		}
		err := common.ValidateExtraneousRedeemers(tx)
		require.Error(t, err)
		require.ErrorAs(t, err, &common.ExtraneousRedeemerError{})
	})

	t.Run("voting index out of range", func(t *testing.T) {
		tx := &conway.ConwayTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagVoting, Index: 1}: {},
			},
		}
		err := common.ValidateExtraneousRedeemers(tx)
		require.Error(t, err)
		require.ErrorAs(t, err, &common.ExtraneousRedeemerError{})
	})

	t.Run("proposing index out of range", func(t *testing.T) {
		tx := &conway.ConwayTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagProposing, Index: 1}: {},
			},
		}
		err := common.ValidateExtraneousRedeemers(tx)
		require.Error(t, err)
		require.ErrorAs(t, err, &common.ExtraneousRedeemerError{})
	})

	t.Run("in-range redeemers for every tag pass", func(t *testing.T) {
		tx := &conway.ConwayTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}:     {},
				{Tag: common.RedeemerTagCert, Index: 0}:      {},
				{Tag: common.RedeemerTagReward, Index: 0}:    {},
				{Tag: common.RedeemerTagVoting, Index: 0}:    {},
				{Tag: common.RedeemerTagProposing, Index: 0}: {},
			},
		}
		require.NoError(t, common.ValidateExtraneousRedeemers(tx))
	})
}

// genesisDelegationLedgerState is a ledger state that can answer the
// genesis-delegation queries required to authorize an MIR certificate.
type genesisDelegationLedgerState struct {
	common.LedgerState
	delegates []common.Blake2b224
	quorum    uint
}

func (s genesisDelegationLedgerState) GenesisDelegateKeyHashes() (
	[]common.Blake2b224,
	error,
) {
	return s.delegates, nil
}

func (s genesisDelegationLedgerState) GenesisUpdateQuorum() (uint, error) {
	return s.quorum, nil
}

func mirGenesisQuorumState(
	quorum uint,
	delegateVkeys ...[]byte,
) genesisDelegationLedgerState {
	delegates := make([]common.Blake2b224, 0, len(delegateVkeys))
	for _, vkey := range delegateVkeys {
		delegates = append(delegates, common.Blake2b224Hash(vkey))
	}
	return genesisDelegationLedgerState{
		LedgerState: mockledger.NewLedgerStateBuilder().Build(),
		delegates:   delegates,
		quorum:      quorum,
	}
}

func mirTransaction(witnessVkeys ...[]byte) common.Transaction {
	cert := &common.MoveInstantaneousRewardsCertificate{
		CertType: uint(common.CertificateTypeMoveInstantaneousRewards),
	}
	tx := mockledger.NewTransactionBuilder().WithCertificates(cert)
	if len(witnessVkeys) > 0 {
		witnesses := make(
			[]common.VkeyWitness,
			0,
			len(witnessVkeys),
		)
		for _, vkey := range witnessVkeys {
			witnesses = append(witnesses, testAuthorizationVkeyWitness(vkey))
		}
		tx.WithWitnesses(
			mockledger.NewMockTransactionWitnessSet().
				WithVkeyWitnesses(witnesses...),
		)
	}
	return tx
}

// MIR certificates name no author in their own fields, so Shelley through
// Babbage authorize them with signatures from a quorum of the currently
// delegated genesis keys.
func TestValidateMIRGenesisQuorum(t *testing.T) {
	delegateA := []byte("genesis-delegate-a")
	delegateB := []byte("genesis-delegate-b")
	delegateC := []byte("genesis-delegate-c")
	retired := []byte("retired-genesis-delegate")
	unrelated := []byte("unrelated-signer")

	t.Run("no MIR certificate is unaffected", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		require.NoError(t, common.ValidateMIRGenesisQuorum(
			tx,
			mockledger.NewLedgerStateBuilder().Build(),
		))
	})

	t.Run("ledger state without the capability fails closed", func(t *testing.T) {
		err := common.ValidateMIRGenesisQuorum(
			mirTransaction(delegateA, delegateB),
			mockledger.NewLedgerStateBuilder().Build(),
		)
		require.ErrorAs(t, err, &common.GenesisDelegationStateUnavailableError{})
	})

	t.Run("nil ledger state fails closed", func(t *testing.T) {
		err := common.ValidateMIRGenesisQuorum(mirTransaction(), nil)
		require.ErrorAs(t, err, &common.GenesisDelegationStateUnavailableError{})
	})

	t.Run("no genesis delegate witnesses is rejected", func(t *testing.T) {
		err := common.ValidateMIRGenesisQuorum(
			mirTransaction(),
			mirGenesisQuorumState(2, delegateA, delegateB, delegateC),
		)
		var quorumErr common.MIRInsufficientGenesisSigsError
		require.ErrorAs(t, err, &quorumErr)
		require.Equal(t, uint(0), quorumErr.Provided)
		require.Equal(t, uint(2), quorumErr.Required)
	})

	t.Run("insufficient delegate witnesses is rejected", func(t *testing.T) {
		err := common.ValidateMIRGenesisQuorum(
			mirTransaction(delegateA),
			mirGenesisQuorumState(2, delegateA, delegateB, delegateC),
		)
		var quorumErr common.MIRInsufficientGenesisSigsError
		require.ErrorAs(t, err, &quorumErr)
		require.Equal(t, uint(1), quorumErr.Provided)
	})

	t.Run("quorum of current delegates is accepted", func(t *testing.T) {
		require.NoError(t, common.ValidateMIRGenesisQuorum(
			mirTransaction(delegateA, delegateB),
			mirGenesisQuorumState(2, delegateA, delegateB, delegateC),
		))
	})

	t.Run("obsolete and unrelated witnesses do not count", func(t *testing.T) {
		// Only a signature from a currently delegated genesis key counts, so a
		// retired delegate and a bystander cannot make up the quorum.
		err := common.ValidateMIRGenesisQuorum(
			mirTransaction(delegateA, retired, unrelated),
			mirGenesisQuorumState(2, delegateA, delegateB, delegateC),
		)
		var quorumErr common.MIRInsufficientGenesisSigsError
		require.ErrorAs(t, err, &quorumErr)
		require.Equal(t, uint(1), quorumErr.Provided)
	})

	t.Run("a repeated delegate counts once", func(t *testing.T) {
		err := common.ValidateMIRGenesisQuorum(
			mirTransaction(delegateA, delegateA),
			mirGenesisQuorumState(2, delegateA, delegateB),
		)
		var quorumErr common.MIRInsufficientGenesisSigsError
		require.ErrorAs(t, err, &quorumErr)
		require.Equal(t, uint(1), quorumErr.Provided)
	})

	t.Run("the Shelley UTXOW rule enforces the quorum", func(t *testing.T) {
		state := mirGenesisQuorumState(2, delegateA, delegateB)
		require.Error(t, shelley.UtxoValidateMIRGenesisQuorum(
			mirTransaction(delegateA),
			0,
			state,
			nil,
		))
		require.NoError(t, shelley.UtxoValidateMIRGenesisQuorum(
			mirTransaction(delegateA, delegateB),
			0,
			state,
			nil,
		))
	})
}
