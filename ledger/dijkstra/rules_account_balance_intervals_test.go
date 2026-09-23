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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func dijkstraIntervalBounds(
	lower *uint64,
	upper *uint64,
) *DijkstraAccountBalanceInterval {
	return &DijkstraAccountBalanceInterval{
		LowerBound: lower,
		UpperBound: upper,
	}
}

func dijkstraIntervalExact(amount uint64) *DijkstraAccountBalanceInterval {
	return &DijkstraAccountBalanceInterval{Exact: &amount}
}

func dijkstraIntervalAddress(t *testing.T, fill byte) *common.Address {
	return dijkstraIntervalAddressOnNetwork(t, fill, common.AddressNetworkTestnet)
}

func dijkstraIntervalAddressOnNetwork(
	t *testing.T,
	fill byte,
	network uint8,
) *common.Address {
	t.Helper()
	var hash common.CredentialHash
	copy(hash[:], bytes.Repeat([]byte{fill}, len(hash)))
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: hash,
	}
	address, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey,
		network,
		nil,
		credential.Credential[:],
	)
	require.NoError(t, err)
	return &address
}

func dijkstraRewardAddressForCredential(
	t *testing.T,
	credential common.Credential,
) *common.Address {
	t.Helper()
	address, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey,
		common.AddressNetworkTestnet,
		nil,
		credential.Credential[:],
	)
	require.NoError(t, err)
	return &address
}

func dijkstraIntervalCredential(t *testing.T, address *common.Address) common.Credential {
	t.Helper()
	credential, err := address.RewardAccountCredential()
	require.NoError(t, err)
	return credential
}

func dijkstraIntervalKey(t *testing.T, address *common.Address) cbor.ByteString {
	t.Helper()
	raw, err := address.Bytes()
	require.NoError(t, err)
	return cbor.NewByteString(raw)
}

// TestDijkstraAccountBalanceIntervalsAgainstLedgerState covers the two checks
// cardano-ledger's validateAccountBalanceIntervals makes that the wire shape
// can express: the account must be registered, and its balance must satisfy
// the asserted interval.
func TestDijkstraAccountBalanceIntervalsAgainstLedgerState(t *testing.T) {
	registered := dijkstraIntervalAddress(t, 0x11)
	unregistered := dijkstraIntervalAddress(t, 0x22)
	registeredCredential := dijkstraIntervalCredential(t, registered)
	unregisteredCredential := dijkstraIntervalCredential(t, unregistered)
	const balance = uint64(5_000_000)
	ls := mockledger.NewLedgerStateBuilder().
		WithRewardAccountCredentialBalance(registeredCredential, balance).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleAccountBalanceIntervals)

	newTx := func(
		intervals DijkstraAccountBalanceIntervals,
	) *DijkstraTransaction {
		return &DijkstraTransaction{
			Body:      DijkstraTransactionBody{TxBalanceIntervals: intervals},
			TxIsValid: true,
		}
	}

	require.NoError(t, rule(newTx(DijkstraAccountBalanceIntervals{
		dijkstraIntervalKey(t, registered): dijkstraIntervalExact(balance),
	}), 0, ls, pp))

	var outsideErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(
		t,
		rule(newTx(DijkstraAccountBalanceIntervals{
			dijkstraIntervalKey(t, registered): dijkstraIntervalExact(balance + 1),
		}), 0, ls, pp),
		&outsideErr,
	)
	require.Len(t, outsideErr.Mismatches, 1)
	require.Equal(t, registeredCredential, outsideErr.Mismatches[0].Credential)
	require.Equal(t, balance, outsideErr.Mismatches[0].Balance)

	var missingErr MissingAccountsInBalanceIntervalsError
	require.ErrorAs(
		t,
		rule(newTx(DijkstraAccountBalanceIntervals{
			dijkstraIntervalKey(t, unregistered): dijkstraIntervalExact(balance),
		}), 0, ls, pp),
		&missingErr,
	)
	require.Equal(t, []common.Credential{unregisteredCredential}, missingErr.Credentials)

	// The lower bound is inclusive and the upper bound exclusive, so a
	// balance equal to the upper bound is outside the interval.
	lower := balance
	upper := balance + 1
	require.NoError(t, rule(newTx(DijkstraAccountBalanceIntervals{
		dijkstraIntervalKey(t, registered): dijkstraIntervalBounds(&lower, &upper),
	}), 0, ls, pp))

	atUpper := balance
	var boundErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(
		t,
		rule(newTx(DijkstraAccountBalanceIntervals{
			dijkstraIntervalKey(t, registered): dijkstraIntervalBounds(nil, &atUpper),
		}), 0, ls, pp),
		&boundErr,
	)
	require.Len(t, boundErr.Mismatches, 1)
}

// TestDijkstraAccountBalanceIntervalsCoverSubTransactions checks sub-level
// and top-level assertions, including that only changed credentials are
// skipped when gouroboros cannot thread account state between levels.
func TestDijkstraAccountBalanceIntervalsCoverSubTransactions(t *testing.T) {
	credential := dijkstraIntervalAddress(t, 0x11)
	otherCredential := dijkstraIntervalAddress(t, 0x22)
	credentialHash := dijkstraIntervalCredential(t, credential)
	otherCredentialHash := dijkstraIntervalCredential(t, otherCredential)
	const balance = uint64(5_000_000)
	ls := mockledger.NewLedgerStateBuilder().
		WithRewardAccountCredentialBalance(credentialHash, balance).
		WithRewardAccountCredentialBalance(otherCredentialHash, balance).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleAccountBalanceIntervals)

	wrong := DijkstraAccountBalanceIntervals{
		dijkstraIntervalKey(t, credential): dijkstraIntervalExact(balance + 1),
	}
	right := DijkstraAccountBalanceIntervals{
		dijkstraIntervalKey(t, credential): dijkstraIntervalExact(balance),
	}
	unrelatedWrong := DijkstraAccountBalanceIntervals{
		dijkstraIntervalKey(t, otherCredential): dijkstraIntervalExact(balance + 1),
	}
	address := credential

	newTx := func(
		subBody DijkstraSubTransactionBody,
		topIntervals DijkstraAccountBalanceIntervals,
	) *DijkstraTransaction {
		tx := dijkstraSingleSubTx(DijkstraSubTransaction{Body: subBody})
		tx.Body.TxBalanceIntervals = topIntervals
		return tx
	}

	// A sub-transaction body's key 26 is checked.
	var subErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(
		t,
		rule(newTx(
			DijkstraSubTransactionBody{TxAccountBalanceIntervals: wrong},
			nil,
		), 0, ls, pp),
		&subErr,
	)
	require.Len(t, subErr.Mismatches, 1)
	require.NoError(t, rule(newTx(
		DijkstraSubTransactionBody{TxAccountBalanceIntervals: right},
		nil,
	), 0, ls, pp))

	// The top level's own key 26 is checked when no earlier level moves an
	// account balance.
	var topErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(
		t,
		rule(newTx(DijkstraSubTransactionBody{}, wrong), 0, ls, pp),
		&topErr,
	)
	require.Len(t, topErr.Mismatches, 1)

	// Earlier account transitions are reflected in the later top-level check.
	var changedErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(t, rule(newTx(
		DijkstraSubTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{address: balance},
		},
		wrong,
	), 0, ls, pp), &changedErr)
	var unrelatedErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(t, rule(newTx(
		DijkstraSubTransactionBody{
			TxDirectDeposits: DijkstraDirectDeposits{
				dijkstraIntervalKey(t, dijkstraDepositAccount(t, 0x11)): 1,
			},
		},
		unrelatedWrong,
	), 0, ls, pp), &unrelatedErr)
	require.Len(t, unrelatedErr.Mismatches, 1)
	require.ErrorAs(t, rule(newTx(
		DijkstraSubTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{address: balance},
		},
		unrelatedWrong,
	), 0, ls, pp), &unrelatedErr)
	require.NoError(t, rule(newTx(
		DijkstraSubTransactionBody{
			TxDirectDeposits: DijkstraDirectDeposits{
				dijkstraIntervalKey(t, dijkstraDepositAccount(t, 0x11)): 1,
			},
		},
		wrong,
	), 0, ls, pp))
}

func TestDijkstraAccountBalanceIntervalsThreadDirectDepositAndStartingState(t *testing.T) {
	address := dijkstraIntervalAddress(t, 0x31)
	credential := dijkstraIntervalCredential(t, address)
	const initial = uint64(100)
	const deposit = uint64(25)
	ls := mockledger.NewLedgerStateBuilder().
		WithRewardAccountCredentialBalance(credential, initial).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleAccountBalanceIntervals)
	key := dijkstraIntervalKey(t, address)
	initialInterval := DijkstraAccountBalanceIntervals{key: dijkstraIntervalExact(initial)}
	finalInterval := DijkstraAccountBalanceIntervals{key: dijkstraIntervalExact(initial + deposit)}

	tx := dijkstraSingleSubTx(DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxDirectDeposits:          DijkstraDirectDeposits{key: deposit},
			TxAccountBalanceIntervals: initialInterval,
		},
	})
	tx.Body.TxBalanceIntervals = finalInterval
	tx.Body.TxStartingBalanceIntervals = initialInterval
	require.NoError(t, rule(tx, 0, ls, pp))

	tx.Body.TxStartingBalanceIntervals = finalInterval
	var startingErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(t, rule(tx, 0, ls, pp), &startingErr)
	require.True(t, startingErr.Starting)

	tx.Body.TxBalanceIntervals = initialInterval
	var finalErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(t, rule(tx, 0, ls, pp), &finalErr)
	require.False(t, finalErr.Starting)
}

func TestDijkstraDirectDepositsRequireCurrentNetworkAndRegisteredAccount(t *testing.T) {
	registered := dijkstraIntervalAddress(t, 0x41)
	credential := dijkstraIntervalCredential(t, registered)
	ls := mockledger.NewLedgerStateBuilder().
		WithRewardAccountCredentialBalance(credential, 0).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleAccountBalanceIntervals)

	wrongNetwork := dijkstraIntervalAddressOnNetwork(
		t,
		0x41,
		common.AddressNetworkMainnet,
	)
	wrongNetworkTx := dijkstraSubUtxoTopLevelTx(nil, nil)
	wrongNetworkTx.Body.TxDirectDeposits = DijkstraDirectDeposits{
		dijkstraIntervalKey(t, wrongNetwork): 1,
	}
	var networkErr WrongNetworkAccountAddressesError
	require.ErrorAs(t, rule(wrongNetworkTx, 0, ls, pp), &networkErr)
	require.Equal(t, "direct deposits", networkErr.Field)

	unregistered := dijkstraIntervalAddress(t, 0x42)
	unregisteredTx := dijkstraSubUtxoTopLevelTx(nil, nil)
	unregisteredTx.Body.TxDirectDeposits = DijkstraDirectDeposits{
		dijkstraIntervalKey(t, unregistered): 1,
	}
	var missingErr DirectDepositAccountsMissingError
	require.ErrorAs(t, rule(unregisteredTx, 0, ls, pp), &missingErr)
	require.Equal(t, []common.Credential{dijkstraIntervalCredential(t, unregistered)}, missingErr.Credentials)

	registeredAfterCertificate := dijkstraIntervalAddress(t, 0x43)
	registeredCredential := dijkstraIntervalCredential(t, registeredAfterCertificate)
	registeredTx := dijkstraSubUtxoTopLevelTx(nil, nil)
	registeredTx.Body.TxCertificates = []common.CertificateWrapper{{
		Type: uint(common.CertificateTypeStakeRegistration),
		Certificate: &common.StakeRegistrationCertificate{
			CertType:        uint(common.CertificateTypeStakeRegistration),
			StakeCredential: registeredCredential,
		},
	}}
	registeredTx.Body.TxDirectDeposits = DijkstraDirectDeposits{
		dijkstraIntervalKey(t, registeredAfterCertificate): 1,
	}
	require.NoError(t, rule(registeredTx, 0, ls, pp))
}
