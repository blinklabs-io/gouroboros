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

package ledger

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

const sequentialTestAddress = "addr_test1qqx80sj9nwxdnglmzdl95v2k40d9422au0klwav8jz2dj985v0wma0mza32f8z6pv2jmkn7cen50f9vn9jmp7dd0njcqqpce07"

var sequentialTestRules = []common.UtxoValidationRuleFunc{
	shelley.UtxoValidateBadInputsUtxo,
}

func sequentialTestInput(
	t *testing.T,
	txId byte,
	index uint32,
) common.TransactionInput {
	t.Helper()
	input, err := mockledger.NewSimpleTransactionInput(
		bytes.Repeat([]byte{txId}, 32),
		index,
	)
	require.NoError(t, err)
	return input
}

func sequentialTestTx(
	t *testing.T,
	txId byte,
	inputs ...common.TransactionInput,
) common.Transaction {
	t.Helper()
	output, err := mockledger.NewSimpleTransactionOutput(
		sequentialTestAddress,
		2_000_000,
	)
	require.NoError(t, err)
	tx, err := mockledger.NewTransactionBuilder().
		WithId(bytes.Repeat([]byte{txId}, 32)).
		WithInputs(inputs...).
		WithOutputs(output).
		WithValid(true).
		Build()
	require.NoError(t, err)
	return tx
}

func sequentialTestState(
	t *testing.T,
	inputs ...common.TransactionInput,
) common.LedgerState {
	t.Helper()
	output, err := mockledger.NewSimpleTransactionOutput(
		sequentialTestAddress,
		5_000_000,
	)
	require.NoError(t, err)
	utxos := make([]common.Utxo, 0, len(inputs))
	for _, input := range inputs {
		utxos = append(utxos, common.Utxo{Id: input, Output: output})
	}
	return mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
}

func requireBadInputsAt(t *testing.T, idx int, err error, want int) {
	t.Helper()
	var badInputs shelley.BadInputsUtxoError
	require.ErrorAs(t, err, &badInputs)
	require.Equal(t, want, idx)
}

func TestVerifyBlockTransactionsAcceptsChainedSpend(t *testing.T) {
	genesis := sequentialTestInput(t, 0x01, 0)
	ls := sequentialTestState(t, genesis)
	first := sequentialTestTx(t, 0xa1, genesis)
	second := sequentialTestTx(t, 0xa2, sequentialTestInput(t, 0xa1, 0))
	_, err := verifyBlockTransactions(
		[]common.Transaction{first, second},
		0,
		ls,
		&mockledger.MockProtocolParamsRules{},
		sequentialTestRules,
	)
	require.NoError(t, err)
}

func TestVerifyBlockTransactionsRejectsDoubleSpend(t *testing.T) {
	genesis := sequentialTestInput(t, 0x01, 0)
	ls := sequentialTestState(t, genesis)
	idx, err := verifyBlockTransactions(
		[]common.Transaction{
			sequentialTestTx(t, 0xa1, genesis),
			sequentialTestTx(t, 0xa2, genesis),
		},
		0,
		ls,
		&mockledger.MockProtocolParamsRules{},
		sequentialTestRules,
	)
	requireBadInputsAt(t, idx, err, 1)
}

// A phase-2-invalid transaction consumes only its collateral and produces
// only its collateral return, at the index after its regular outputs.
func TestVerifyBlockTransactionsPhaseTwoInvalidAppliesCollateralOnly(
	t *testing.T,
) {
	input := shelley.NewShelleyTransactionInput(strings.Repeat("01", 32), 0)
	collateral := shelley.NewShelleyTransactionInput(
		strings.Repeat("02", 32),
		0,
	)
	addr, err := common.NewAddress(sequentialTestAddress)
	require.NoError(t, err)
	output := babbage.BabbageTransactionOutput{
		OutputAddress: addr,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 2_000_000},
	}
	invalid := &babbage.BabbageTransaction{
		Body: babbage.BabbageTransactionBody{
			TxOutputs:          []babbage.BabbageTransactionOutput{output},
			TxCollateralReturn: &output,
			TxCollateral: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{collateral},
				false,
			),
		},
		TxIsValid: false,
	}
	invalid.Body.TxInputs = shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)
	invalidId := invalid.Hash().String()
	ls := sequentialTestState(t, input, collateral)

	for _, test := range []struct {
		name    string
		spend   common.TransactionInput
		wantErr bool
	}{
		{name: "regular input stays unspent", spend: input},
		{
			name:  "collateral return is created",
			spend: shelley.NewShelleyTransactionInput(invalidId, 1),
		},
		{
			name:    "regular output is not created",
			spend:   shelley.NewShelleyTransactionInput(invalidId, 0),
			wantErr: true,
		},
		{name: "collateral is consumed", spend: collateral, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			idx, err := verifyBlockTransactions(
				[]common.Transaction{
					invalid,
					sequentialTestTx(t, 0xb1, test.spend),
				},
				0,
				ls,
				&mockledger.MockProtocolParamsRules{},
				sequentialTestRules,
			)
			if test.wantErr {
				requireBadInputsAt(t, idx, err, 1)
				return
			}
			require.NoError(t, err)
		})
	}
}

// The Conway withdrawal rule reads reward-account registration, balance and,
// at protocol version 10, the account's DRep delegation, all of which an
// earlier transaction's withdrawal or certificate can change.
func TestVerifyBlockTransactionsAppliesEarlierCertificatesAndWithdrawals(
	t *testing.T,
) {
	stake := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224(bytes.Repeat([]byte{0x11}, 28)),
	}
	drepCred := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224(bytes.Repeat([]byte{0x22}, 28)),
	}
	drep := common.Drep{
		Type:       common.DrepTypeAddrKeyHash,
		Credential: drepCred.Credential.Bytes(),
	}
	rewardAddr, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey,
		common.AddressNetworkTestnet,
		nil,
		stake.Credential.Bytes(),
	)
	require.NoError(t, err)
	withdraw := func(id byte, amount uint64) common.Transaction {
		tx := sequentialTestTx(
			t,
			id,
			sequentialTestInput(t, id, 0),
		).(*mockledger.MockTransaction)
		return tx.WithWithdrawals(
			map[*common.Address]uint64{&rewardAddr: amount},
		)
	}
	certify := func(id byte, certs ...common.Certificate) common.Transaction {
		tx := sequentialTestTx(
			t,
			id,
			sequentialTestInput(t, id, 0),
		).(*mockledger.MockTransaction)
		return tx.WithCertificates(certs...)
	}
	pp := &conway.ConwayProtocolParameters{}
	pp.ProtocolVersion.Major = common.ProtocolVersionPlomin
	rules := []common.UtxoValidationRuleFunc{conway.UtxoValidateWithdrawals}

	for _, test := range []struct {
		name       string
		registered bool
		delegated  bool
		txs        []common.Transaction
		wantErr    func(error) bool
	}{
		{
			name:       "vote delegation enables a later withdrawal",
			registered: true,
			txs: []common.Transaction{
				certify(0xc1, &common.VoteDelegationCertificate{
					StakeCredential: stake,
					Drep:            drep,
				}),
				withdraw(0xc2, 1_000),
			},
		},
		{
			name: "stake registration enables a later withdrawal",
			txs: []common.Transaction{
				certify(0xc1, &common.VoteRegistrationDelegationCertificate{
					StakeCredential: stake,
					Drep:            drep,
					Amount:          2_000_000,
				}),
				withdraw(0xc2, 0),
			},
		},
		{
			name:       "an earlier withdrawal drains the account",
			registered: true,
			delegated:  true,
			txs: []common.Transaction{
				withdraw(0xc1, 1_000),
				withdraw(0xc2, 1_000),
			},
			wantErr: func(err error) bool {
				var target shelley.IncorrectWithdrawalAmountError
				return errors.As(err, &target)
			},
		},
		{
			name:       "DRep deregistration clears the delegation",
			registered: true,
			delegated:  true,
			txs: []common.Transaction{
				certify(0xc1, &common.DeregistrationDrepCertificate{
					DrepCredential: drepCred,
				}),
				withdraw(0xc2, 1_000),
			},
			wantErr: func(err error) bool {
				var target conway.WithdrawalNotDelegatedToDRepError
				return errors.As(err, &target)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := mockledger.NewLedgerStateBuilder().
				WithDRepDelegation(func(cred common.Credential) (*common.Drep, error) {
					if test.delegated && cred.CredType == stake.CredType &&
						cred.Credential == stake.Credential {
						return &drep, nil
					}
					return nil, nil
				})
			if test.registered {
				builder = builder.WithRewardAccountCredentialBalance(
					stake,
					1_000,
				)
			}
			idx, err := verifyBlockTransactions(
				test.txs,
				0,
				builder.Build(),
				pp,
				rules,
			)
			if test.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.True(t, test.wantErr(err), "unexpected error: %v", err)
			require.Equal(t, 1, idx)
		})
	}
}

// From Dijkstra a withdrawal may take part of the balance; the rest stays
// available to a later transaction in the block.
func TestVerifyBlockTransactionsKeepsBalanceAfterPartialWithdrawal(
	t *testing.T,
) {
	stake := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224(bytes.Repeat([]byte{0x11}, 28)),
	}
	rewardAddr, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey,
		common.AddressNetworkTestnet,
		nil,
		stake.Credential.Bytes(),
	)
	require.NoError(t, err)
	withdraw := func(id byte, amount uint64) common.Transaction {
		tx := sequentialTestTx(
			t,
			id,
			sequentialTestInput(t, id, 0),
		).(*mockledger.MockTransaction)
		return tx.WithWithdrawals(
			map[*common.Address]uint64{&rewardAddr: amount},
		)
	}
	pp := &conway.ConwayProtocolParameters{}
	pp.ProtocolVersion.Major = common.ProtocolVersionDijkstra
	// The Dijkstra amount rule resolves inputs to look for Plutus V1/V2
	// scripts.
	output, err := mockledger.NewSimpleTransactionOutput(
		sequentialTestAddress,
		5_000_000,
	)
	require.NoError(t, err)
	var utxos []common.Utxo
	for _, id := range []byte{0xd1, 0xd2} {
		utxos = append(utxos, common.Utxo{
			Id:     sequentialTestInput(t, id, 0),
			Output: output,
		})
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxos(utxos).
		WithRewardAccountCredentialBalance(stake, 1_000).
		Build()
	rules := []common.UtxoValidationRuleFunc{conway.UtxoValidateWithdrawals}

	_, err = verifyBlockTransactions(
		[]common.Transaction{withdraw(0xd1, 400), withdraw(0xd2, 600)},
		0,
		ls,
		pp,
		rules,
	)
	require.NoError(t, err)

	idx, err := verifyBlockTransactions(
		[]common.Transaction{withdraw(0xd1, 400), withdraw(0xd2, 601)},
		0,
		ls,
		pp,
		rules,
	)
	var amountErr shelley.IncorrectWithdrawalAmountError
	require.ErrorAs(t, err, &amountErr)
	require.Equal(t, 1, idx)
}
