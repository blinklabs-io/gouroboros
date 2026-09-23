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

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestValueConservationFoldsStakeDepositsSequentially(t *testing.T) {
	const (
		inputAmount    = uint64(100_000_000)
		historical     = uint64(2_000_000)
		current        = uint64(3_000_000)
		correctRefunds = historical + current
	)
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	for _, credType := range []uint{common.CredentialTypeAddrKeyHash, common.CredentialTypeScriptHash} {
		credential := common.Credential{CredType: credType}
		credential.Credential[0] = 0x42
		state := certificateDepositLedgerState{
			LedgerState: mockledger.NewLedgerStateBuilder().
				WithUtxos([]common.Utxo{{
					Id:     input,
					Output: shelley.ShelleyTransactionOutput{OutputAmount: inputAmount},
				}}).
				WithStakeRegistrations([]common.StakeRegistrationCertificate{{StakeCredential: credential}}).
				Build(),
			deposits: map[certificateDepositCredentialKey]uint64{{
				credType: credential.CredType,
				hash:     credential.Credential,
			}: historical},
		}
		certificates := []common.CertificateWrapper{
			{Type: uint(common.CertificateTypeStakeDeregistration), Certificate: &common.StakeDeregistrationCertificate{StakeCredential: credential}},
			{Type: uint(common.CertificateTypeStakeRegistration), Certificate: &common.StakeRegistrationCertificate{StakeCredential: credential}},
			{Type: uint(common.CertificateTypeStakeDeregistration), Certificate: &common.StakeDeregistrationCertificate{StakeCredential: credential}},
		}
		makeTx := func(output uint64) *conway.ConwayTransaction {
			return &conway.ConwayTransaction{
				TxIsValid: true,
				Body: conway.ConwayTransactionBody{
					TxInputs:       conway.NewConwayTransactionInputSet([]shelley.ShelleyTransactionInput{input}),
					TxOutputs:      []babbage.BabbageTransactionOutput{{OutputAmount: mary.MaryTransactionOutputValue{Amount: output}}},
					TxCertificates: certificates,
				},
			}
		}
		params := &conway.ConwayProtocolParameters{KeyDeposit: uint(current)}
		require.NoError(t, conway.UtxoValidateValueNotConservedUtxo(
			makeTx(inputAmount+correctRefunds-current), 0, state, params,
		))
		require.Error(t, conway.UtxoValidateValueNotConservedUtxo(
			makeTx(inputAmount+2*historical-current), 0, state, params,
		))
	}
}

func TestInvalidTransactionValueConservationUsesProtocolAndRecordedDeposits(t *testing.T) {
	const inputAmount = uint64(100_000_000)
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	credential := common.Credential{CredType: common.CredentialTypeAddrKeyHash}
	credential.Credential[0] = 0x42
	drepCredential := common.Credential{CredType: common.CredentialTypeScriptHash}
	drepCredential.Credential[0] = 0x24
	const (
		keyDeposit      = uint64(2_000_000)
		recordedStake   = uint64(1_500_000)
		drepDeposit     = uint64(3_000_000)
		recordedDRep    = uint64(2_500_000)
		proposalDeposit = uint64(4_000_000)
	)
	recordedDRepValue := recordedDRep
	baseState := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{{Id: input, Output: shelley.ShelleyTransactionOutput{OutputAmount: inputAmount}}}).
		WithStakeRegistrations([]common.StakeRegistrationCertificate{{StakeCredential: credential}}).
		WithDRepRegistrations([]common.DRepRegistration{{Credential: drepCredential, Deposit: &recordedDRepValue}}).
		Build()
	state := certificateDepositLedgerState{
		LedgerState: baseState,
		deposits: map[certificateDepositCredentialKey]uint64{{
			credType: credential.CredType,
			hash:     credential.Credential,
		}: recordedStake},
	}
	params := &conway.ConwayProtocolParameters{
		KeyDeposit:       uint(keyDeposit),
		DRepDeposit:      drepDeposit,
		GovActionDeposit: proposalDeposit,
	}
	tests := []struct {
		name        string
		certificate common.Certificate
		proposal    bool
		deposit     uint64
		refund      bool
	}{
		{"legacy stake registration", &common.StakeRegistrationCertificate{StakeCredential: credential}, false, keyDeposit, false},
		{"explicit stake registration", &common.RegistrationCertificate{StakeCredential: credential, Amount: int64(keyDeposit)}, false, keyDeposit, false},
		{"stake registration delegation", &common.StakeRegistrationDelegationCertificate{StakeCredential: credential, Amount: int64(keyDeposit)}, false, keyDeposit, false},
		{"stake vote registration delegation", &common.StakeVoteRegistrationDelegationCertificate{StakeCredential: credential, Amount: int64(keyDeposit)}, false, keyDeposit, false},
		{"vote registration delegation", &common.VoteRegistrationDelegationCertificate{StakeCredential: credential, Amount: int64(keyDeposit)}, false, keyDeposit, false},
		{"DRep registration", &common.RegistrationDrepCertificate{DrepCredential: drepCredential, Amount: int64(drepDeposit)}, false, drepDeposit, false},
		{"explicit stake refund", &common.DeregistrationCertificate{StakeCredential: credential, Amount: int64(recordedStake)}, false, recordedStake, true},
		{"DRep refund", &common.DeregistrationDrepCertificate{DrepCredential: drepCredential, Amount: int64(recordedDRep)}, false, recordedDRep, true},
		{"proposal deposit", nil, true, proposalDeposit, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outputAmount := inputAmount - test.deposit
			if test.refund {
				outputAmount = inputAmount + test.deposit
			}
			makeTx := func(output uint64) *conway.ConwayTransaction {
				body := conway.ConwayTransactionBody{
					TxInputs:  conway.NewConwayTransactionInputSet([]shelley.ShelleyTransactionInput{input}),
					TxOutputs: []babbage.BabbageTransactionOutput{{OutputAmount: mary.MaryTransactionOutputValue{Amount: output}}},
				}
				if test.certificate != nil {
					body.TxCertificates = []common.CertificateWrapper{{Type: uint(test.certificate.Type()), Certificate: test.certificate}}
				}
				if test.proposal {
					body.TxProposalProcedures = []conway.ConwayProposalProcedure{{PPDeposit: test.deposit}}
				}
				return &conway.ConwayTransaction{Body: body, TxIsValid: false}
			}
			require.NoError(t, conway.UtxoValidateValueNotConservedUtxo(makeTx(outputAmount), 0, state, params))
			wrongBodyTx := makeTx(outputAmount)
			if test.proposal {
				wrongBodyTx.Body.TxProposalProcedures[0].PPDeposit = 1
			} else {
				switch certificate := wrongBodyTx.Body.TxCertificates[0].Certificate.(type) {
				case *common.RegistrationCertificate:
					certificate.Amount = 1
				case *common.StakeRegistrationDelegationCertificate:
					certificate.Amount = 1
				case *common.StakeVoteRegistrationDelegationCertificate:
					certificate.Amount = 1
				case *common.VoteRegistrationDelegationCertificate:
					certificate.Amount = 1
				case *common.RegistrationDrepCertificate:
					certificate.Amount = 1
				case *common.DeregistrationCertificate:
					certificate.Amount = 1
				case *common.DeregistrationDrepCertificate:
					certificate.Amount = 1
				}
			}
			if _, legacyRegistration := test.certificate.(*common.StakeRegistrationCertificate); !legacyRegistration && !test.proposal {
				require.Error(t, conway.UtxoValidateValueNotConservedUtxo(wrongBodyTx, 0, state, params))
			} else if test.proposal {
				require.Error(t, conway.UtxoValidateValueNotConservedUtxo(wrongBodyTx, 0, state, params))
			}
			wrongOutput := outputAmount
			if test.refund {
				wrongOutput++
			} else {
				wrongOutput--
			}
			require.Error(t, conway.UtxoValidateValueNotConservedUtxo(makeTx(wrongOutput), 0, state, params))
		})
	}
}

func TestValueConservationAllowsZeroDRepDeposits(t *testing.T) {
	const inputAmount = uint64(100_000_000)
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	credential := common.Credential{CredType: common.CredentialTypeAddrKeyHash}
	credential.Credential[0] = 0x42
	zero := uint64(0)
	base := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{{Id: input, Output: shelley.ShelleyTransactionOutput{OutputAmount: inputAmount}}}).Build()
	registered := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{{Id: input, Output: shelley.ShelleyTransactionOutput{OutputAmount: inputAmount}}}).
		WithDRepRegistrations([]common.DRepRegistration{{Credential: credential, Deposit: &zero}}).Build()
	makeTx := func(cert common.Certificate) *conway.ConwayTransaction {
		return &conway.ConwayTransaction{
			TxIsValid: true,
			Body: conway.ConwayTransactionBody{
				TxInputs:  conway.NewConwayTransactionInputSet([]shelley.ShelleyTransactionInput{input}),
				TxOutputs: []babbage.BabbageTransactionOutput{{OutputAmount: mary.MaryTransactionOutputValue{Amount: inputAmount}}},
				TxCertificates: []common.CertificateWrapper{{
					Type: uint(cert.Type()), Certificate: cert,
				}},
			},
		}
	}
	params := &conway.ConwayProtocolParameters{DRepDeposit: 0}
	require.NoError(t, conway.UtxoValidateValueNotConservedUtxo(
		makeTx(&common.RegistrationDrepCertificate{DrepCredential: credential, Amount: 0}), 0, base, params,
	))
	require.NoError(t, conway.UtxoValidateValueNotConservedUtxo(
		makeTx(&common.DeregistrationDrepCertificate{DrepCredential: credential, Amount: 0}), 0, registered, params,
	))
}
