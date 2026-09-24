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

package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestLegacyStakeRefundUsesRecordedDepositAcrossEras(t *testing.T) {
	const inputAmount = uint64(100_000_000)
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	validators := []struct {
		name string
		call func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error
		pp   func(uint) common.ProtocolParameters
	}{
		{"Shelley", shelley.UtxoValidateValueNotConservedUtxo, func(v uint) common.ProtocolParameters { return &shelley.ShelleyProtocolParameters{KeyDeposit: v} }},
		{"Allegra", allegra.UtxoValidateValueNotConservedUtxo, func(v uint) common.ProtocolParameters { return &allegra.AllegraProtocolParameters{KeyDeposit: v} }},
		{"Mary", mary.UtxoValidateValueNotConservedUtxo, func(v uint) common.ProtocolParameters { return &mary.MaryProtocolParameters{KeyDeposit: v} }},
		{"Alonzo", alonzo.UtxoValidateValueNotConservedUtxo, func(v uint) common.ProtocolParameters { return &alonzo.AlonzoProtocolParameters{KeyDeposit: v} }},
		{"Babbage", babbage.UtxoValidateValueNotConservedUtxo, func(v uint) common.ProtocolParameters { return &babbage.BabbageProtocolParameters{KeyDeposit: v} }},
		{"Conway", conway.UtxoValidateValueNotConservedUtxo, func(v uint) common.ProtocolParameters { return &conway.ConwayProtocolParameters{KeyDeposit: v} }},
		{"Dijkstra", dijkstra.UtxoValidateValueNotConservedUtxo, func(v uint) common.ProtocolParameters {
			return &dijkstra.DijkstraProtocolParameters{ConwayProtocolParameters: conway.ConwayProtocolParameters{KeyDeposit: v}}
		}},
	}
	for _, validator := range validators {
		for _, credType := range []uint{common.CredentialTypeAddrKeyHash, common.CredentialTypeScriptHash} {
			for _, deposits := range []struct {
				name     string
				current  uint
				recorded uint64
			}{
				{"key deposit increased", 3_000_000, 2_000_000},
				{"key deposit decreased", 2_000_000, 3_000_000},
			} {
				t.Run(validator.name+"/credential-"+string(rune('0'+credType))+"/"+deposits.name, func(t *testing.T) {
					credential := common.Credential{CredType: credType}
					credential.Credential[0] = 0x42
					state := mockledger.NewLedgerStateBuilder().
						WithUtxos([]common.Utxo{{Id: input, Output: shelley.ShelleyTransactionOutput{OutputAmount: inputAmount}}}).
						WithStakeRegistrations([]common.StakeRegistrationCertificate{{StakeCredential: credential}}).
						WithStakeCredentialDeposits(map[mockledger.RewardAccountKey]uint64{
							mockledger.NewRewardAccountKey(credential): deposits.recorded,
						}).Build()
					output, err := mockledger.NewTransactionOutputBuilder().WithLovelace(inputAmount + deposits.recorded).Build()
					require.NoError(t, err)
					tx, err := mockledger.NewTransactionBuilder().
						WithCertificates(&common.StakeDeregistrationCertificate{StakeCredential: credential}).
						WithInputs(input).
						WithOutputs(output).Build()
					require.NoError(t, err)
					require.NoError(t, validator.call(tx, 0, state, validator.pp(deposits.current)))

					wrongOutput, err := mockledger.NewTransactionOutputBuilder().WithLovelace(inputAmount + uint64(deposits.current)).Build()
					require.NoError(t, err)
					wrongTx, err := mockledger.NewTransactionBuilder().
						WithCertificates(&common.StakeDeregistrationCertificate{StakeCredential: credential}).
						WithInputs(input).
						WithOutputs(wrongOutput).Build()
					require.NoError(t, err)
					require.Error(t, validator.call(wrongTx, 0, state, validator.pp(deposits.current)))
				})
			}
		}
	}
}

func TestStakeRefundUsesEarlierInTransactionRegistrationAcrossEras(t *testing.T) {
	const inputAmount = uint64(100_000_000)
	input := shelley.NewShelleyTransactionInput(
		"d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22",
		0,
	)
	credential := common.Credential{CredType: common.CredentialTypeAddrKeyHash}
	credential.Credential[0] = 0x42
	output, err := mockledger.NewTransactionOutputBuilder().
		WithLovelace(inputAmount + 2_000_000).Build()
	require.NoError(t, err)
	tx, err := mockledger.NewTransactionBuilder().
		WithCertificates(
			&common.StakeDeregistrationCertificate{StakeCredential: credential},
			&common.StakeRegistrationCertificate{StakeCredential: credential},
			&common.StakeDeregistrationCertificate{StakeCredential: credential},
		).
		WithInputs(input).
		WithOutputs(output).
		Build()
	require.NoError(t, err)
	state := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{{
			Id: input,
			Output: shelley.ShelleyTransactionOutput{
				OutputAmount: inputAmount,
			},
		}}).
		WithStakeRegistrations([]common.StakeRegistrationCertificate{{
			StakeCredential: credential,
		}}).
		WithStakeCredentialDeposits(map[mockledger.RewardAccountKey]uint64{
			mockledger.NewRewardAccountKey(credential): 2_000_000,
		}).
		Build()
	validators := []struct {
		name string
		call func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error
		pp   common.ProtocolParameters
	}{
		{"Shelley", shelley.UtxoValidateValueNotConservedUtxo, &shelley.ShelleyProtocolParameters{KeyDeposit: 3_000_000}},
		{"Allegra", allegra.UtxoValidateValueNotConservedUtxo, &allegra.AllegraProtocolParameters{KeyDeposit: 3_000_000}},
		{"Mary", mary.UtxoValidateValueNotConservedUtxo, &mary.MaryProtocolParameters{KeyDeposit: 3_000_000}},
		{"Alonzo", alonzo.UtxoValidateValueNotConservedUtxo, &alonzo.AlonzoProtocolParameters{KeyDeposit: 3_000_000}},
		{"Babbage", babbage.UtxoValidateValueNotConservedUtxo, &babbage.BabbageProtocolParameters{KeyDeposit: 3_000_000}},
	}
	for _, validator := range validators {
		t.Run(validator.name, func(t *testing.T) {
			require.NoError(t, validator.call(tx, 0, state, validator.pp))
			wrongOutput, err := mockledger.NewTransactionOutputBuilder().
				WithLovelace(inputAmount + 1_000_000).Build()
			require.NoError(t, err)
			wrongTx, err := mockledger.NewTransactionBuilder().
				WithCertificates(
					&common.StakeDeregistrationCertificate{StakeCredential: credential},
					&common.StakeRegistrationCertificate{StakeCredential: credential},
					&common.StakeDeregistrationCertificate{StakeCredential: credential},
				).
				WithInputs(input).
				WithOutputs(wrongOutput).
				Build()
			require.NoError(t, err)
			require.Error(t, validator.call(wrongTx, 0, state, validator.pp))
		})
	}
}

func TestPhase2InvalidUnregisteredStakeDeregistrationDoesNotRefundKeyDeposit(t *testing.T) {
	const (
		inputAmount = uint64(100_000_000)
		keyDeposit  = uint(2_000_000)
	)
	input := shelley.NewShelleyTransactionInput(
		"d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22",
		0,
	)
	credential := common.Credential{CredType: common.CredentialTypeAddrKeyHash}
	credential.Credential[0] = 0x42
	state := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{{
			Id:     input,
			Output: shelley.ShelleyTransactionOutput{OutputAmount: inputAmount},
		}}).
		Build()
	for _, validator := range []struct {
		name string
		call func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error
		pp   common.ProtocolParameters
	}{
		{
			name: "Alonzo",
			call: alonzo.UtxoValidateValueNotConservedUtxo,
			pp:   &alonzo.AlonzoProtocolParameters{KeyDeposit: keyDeposit},
		},
		{
			name: "Babbage",
			call: babbage.UtxoValidateValueNotConservedUtxo,
			pp:   &babbage.BabbageProtocolParameters{KeyDeposit: keyDeposit},
		},
	} {
		t.Run(validator.name, func(t *testing.T) {
			buildTx := func(outputAmount uint64) common.Transaction {
				output, err := mockledger.NewTransactionOutputBuilder().
					WithLovelace(outputAmount).Build()
				require.NoError(t, err)
				tx, err := mockledger.NewTransactionBuilder().
					WithCertificates(&common.StakeDeregistrationCertificate{
						StakeCredential: credential,
					}).
					WithInputs(input).
					WithOutputs(output).
					WithValid(false).
					Build()
				require.NoError(t, err)
				return tx
			}
			tx := buildTx(inputAmount)
			require.False(t, tx.IsValid())
			require.Len(t, tx.Certificates(), 1)
			require.NoError(t, validator.call(tx, 0, state, validator.pp))
			require.Error(t, validator.call(buildTx(inputAmount+uint64(keyDeposit)), 0, state, validator.pp))
		})
	}
}
