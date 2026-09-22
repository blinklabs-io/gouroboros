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

func dijkstraIntervalCredential(fill byte) *common.Credential {
	var hash common.CredentialHash
	copy(hash[:], bytes.Repeat([]byte{fill}, len(hash)))
	return &common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: hash,
	}
}

// TestDijkstraAccountBalanceIntervalsAgainstLedgerState covers the two checks
// cardano-ledger's validateAccountBalanceIntervals makes that the wire shape
// can express: the account must be registered, and its balance must satisfy
// the asserted interval.
func TestDijkstraAccountBalanceIntervalsAgainstLedgerState(t *testing.T) {
	registered := dijkstraIntervalCredential(0x11)
	unregistered := dijkstraIntervalCredential(0x22)
	const balance = uint64(5_000_000)
	ls := mockledger.NewLedgerStateBuilder().
		WithRewardAccountCredentialBalance(*registered, balance).
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
		registered: dijkstraIntervalExact(balance),
	}), 0, ls, pp))

	var outsideErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(
		t,
		rule(newTx(DijkstraAccountBalanceIntervals{
			registered: dijkstraIntervalExact(balance + 1),
		}), 0, ls, pp),
		&outsideErr,
	)
	require.Len(t, outsideErr.Mismatches, 1)
	require.Equal(t, *registered, outsideErr.Mismatches[0].Credential)
	require.Equal(t, balance, outsideErr.Mismatches[0].Balance)

	var missingErr MissingAccountsInBalanceIntervalsError
	require.ErrorAs(
		t,
		rule(newTx(DijkstraAccountBalanceIntervals{
			unregistered: dijkstraIntervalExact(balance),
		}), 0, ls, pp),
		&missingErr,
	)
	require.Equal(t, []common.Credential{*unregistered}, missingErr.Credentials)

	// The lower bound is inclusive and the upper bound exclusive, so a
	// balance equal to the upper bound is outside the interval.
	lower := balance
	upper := balance + 1
	require.NoError(t, rule(newTx(DijkstraAccountBalanceIntervals{
		registered: dijkstraIntervalBounds(&lower, &upper),
	}), 0, ls, pp))

	atUpper := balance
	var boundErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(
		t,
		rule(newTx(DijkstraAccountBalanceIntervals{
			registered: dijkstraIntervalBounds(nil, &atUpper),
		}), 0, ls, pp),
		&boundErr,
	)
	require.Len(t, boundErr.Mismatches, 1)
}

// TestDijkstraAccountBalanceIntervalsCoverSubTransactions checks sub-level
// and top-level assertions, including that only changed credentials are
// skipped when gouroboros cannot thread account state between levels.
func TestDijkstraAccountBalanceIntervalsCoverSubTransactions(t *testing.T) {
	credential := dijkstraIntervalCredential(0x11)
	otherCredential := dijkstraIntervalCredential(0x22)
	const balance = uint64(5_000_000)
	ls := mockledger.NewLedgerStateBuilder().
		WithRewardAccountCredentialBalance(*credential, balance).
		WithRewardAccountCredentialBalance(*otherCredential, balance).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleAccountBalanceIntervals)

	wrong := DijkstraAccountBalanceIntervals{
		credential: dijkstraIntervalExact(balance + 1),
	}
	right := DijkstraAccountBalanceIntervals{
		credential: dijkstraIntervalExact(balance),
	}
	unrelatedWrong := DijkstraAccountBalanceIntervals{
		otherCredential: dijkstraIntervalExact(balance + 1),
	}
	address, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey,
		common.AddressNetworkTestnet,
		nil,
		credential.Credential[:],
	)
	require.NoError(t, err)

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

	// A sub-transaction withdrawal or direct deposit leaves only that
	// credential unchecked at the later top-level assertion.
	require.NoError(t, rule(newTx(
		DijkstraSubTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{&address: balance},
		},
		wrong,
	), 0, ls, pp))
	var unrelatedErr BalancesOutsideAccountBalanceIntervalsError
	require.ErrorAs(t, rule(newTx(
		DijkstraSubTransactionBody{
			TxDirectDeposits: map[cbor.ByteString]uint64{
				dijkstraDepositAccount(0x11): 1,
			},
		},
		unrelatedWrong,
	), 0, ls, pp), &unrelatedErr)
	require.Len(t, unrelatedErr.Mismatches, 1)
	require.ErrorAs(t, rule(newTx(
		DijkstraSubTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{&address: balance},
		},
		unrelatedWrong,
	), 0, ls, pp), &unrelatedErr)
	require.NoError(t, rule(newTx(
		DijkstraSubTransactionBody{
			TxDirectDeposits: map[cbor.ByteString]uint64{
				dijkstraDepositAccount(0x11): 1,
			},
		},
		wrong,
	), 0, ls, pp))
}
