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
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

const (
	dijkstraCurrentTreasuryRuleIndex = 0
	dijkstraIsValidFlagRuleIndex     = 12
	dijkstraInputSetEmptyRuleIndex   = 23
)

func dijkstraTreasuryValue(value uint64) *uint64 {
	return &value
}

func decodeDijkstraTreasuryBody(
	t *testing.T,
	treasuryValue *uint64,
) DijkstraTransactionBody {
	t.Helper()
	bodyFields := map[uint]any{
		0: cbor.NewSetType([]shelley.ShelleyTransactionInput{}, true),
		1: []DijkstraTransactionOutput{},
		2: uint64(0),
	}
	if treasuryValue != nil {
		bodyFields[21] = *treasuryValue
	}
	bodyCbor, err := cbor.Encode(bodyFields)
	require.NoError(t, err)
	var body DijkstraTransactionBody
	require.NoError(t, body.UnmarshalCBOR(bodyCbor))
	reencoded, err := cbor.Encode(&body)
	require.NoError(t, err)
	require.Equal(t, bodyCbor, reencoded)
	return body
}

func TestDijkstraCurrentTreasuryValuePresence(t *testing.T) {
	absent := decodeDijkstraTreasuryBody(t, nil)
	require.False(t, absent.CurrentTreasuryValuePresent())
	require.Nil(t, absent.CurrentTreasuryValue())

	zero := decodeDijkstraTreasuryBody(t, dijkstraTreasuryValue(0))
	require.True(t, zero.CurrentTreasuryValuePresent())
	require.Zero(t, zero.CurrentTreasuryValue().Sign())

	nonzero := decodeDijkstraTreasuryBody(t, dijkstraTreasuryValue(42))
	require.True(t, nonzero.CurrentTreasuryValuePresent())
	require.Equal(t, big.NewInt(42), nonzero.CurrentTreasuryValue())

	constructed := DijkstraTransactionBody{}
	require.Nil(t, constructed.CurrentTreasuryValue())
	constructed.SetCurrentTreasuryValuePresence(true)
	encoded, err := cbor.Encode(&constructed)
	require.NoError(t, err)
	var encodedFields map[uint]cbor.RawMessage
	_, err = cbor.Decode(encoded, &encodedFields)
	require.NoError(t, err)
	encodedZero, ok := encodedFields[21]
	require.True(t, ok)
	var value uint64
	_, err = cbor.Decode(encodedZero, &value)
	require.NoError(t, err)
	require.Zero(t, value)

	subBodyCbor, err := cbor.Encode(map[uint]any{21: uint64(0)})
	require.NoError(t, err)
	var subBody DijkstraSubTransactionBody
	require.NoError(t, subBody.UnmarshalCBOR(subBodyCbor))
	require.True(t, subBody.CurrentTreasuryValuePresent())
	require.Zero(t, subBody.CurrentTreasuryValue().Sign())
}

func TestDijkstraCurrentTreasuryValueProductionRules(t *testing.T) {
	providerErr := errors.New("treasury provider failed")
	tests := []struct {
		name             string
		treasuryValue    *uint64
		isValid          bool
		ledgerValue      uint64
		ledgerErr        error
		wantProviderCall int
		wantRuleIndex    int
		checkError       func(*testing.T, error)
	}{
		{
			name:          "absent",
			isValid:       true,
			ledgerValue:   42,
			wantRuleIndex: dijkstraInputSetEmptyRuleIndex,
			checkError: func(t *testing.T, err error) {
				var target shelley.InputSetEmptyUtxoError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:             "equal",
			treasuryValue:    dijkstraTreasuryValue(42),
			isValid:          true,
			ledgerValue:      42,
			wantProviderCall: 1,
			wantRuleIndex:    dijkstraInputSetEmptyRuleIndex,
			checkError: func(t *testing.T, err error) {
				var target shelley.InputSetEmptyUtxoError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:             "unequal",
			treasuryValue:    dijkstraTreasuryValue(41),
			isValid:          true,
			ledgerValue:      42,
			wantProviderCall: 1,
			wantRuleIndex:    dijkstraCurrentTreasuryRuleIndex,
			checkError: func(t *testing.T, err error) {
				var target common.CurrentTreasuryValueMismatchError
				require.ErrorAs(t, err, &target)
				require.Equal(t, big.NewInt(41), target.Supplied)
				require.Equal(t, uint64(42), target.Expected)
			},
		},
		{
			name:             "present zero is unequal",
			treasuryValue:    dijkstraTreasuryValue(0),
			isValid:          true,
			ledgerValue:      42,
			wantProviderCall: 1,
			wantRuleIndex:    dijkstraCurrentTreasuryRuleIndex,
			checkError: func(t *testing.T, err error) {
				var target common.CurrentTreasuryValueMismatchError
				require.ErrorAs(t, err, &target)
				require.Zero(t, target.Supplied.Sign())
				require.Equal(t, uint64(42), target.Expected)
			},
		},
		{
			name:             "provider error",
			treasuryValue:    dijkstraTreasuryValue(42),
			isValid:          true,
			ledgerErr:        providerErr,
			wantProviderCall: 1,
			wantRuleIndex:    dijkstraCurrentTreasuryRuleIndex,
			checkError: func(t *testing.T, err error) {
				var target common.TreasuryValueQueryError
				require.ErrorAs(t, err, &target)
				require.ErrorIs(t, err, providerErr)
			},
		},
		{
			name:          "phase-2 invalid skips provider",
			treasuryValue: dijkstraTreasuryValue(41),
			ledgerErr:     providerErr,
			wantRuleIndex: dijkstraIsValidFlagRuleIndex,
			checkError: func(t *testing.T, err error) {
				var target common.InvalidIsValidFlagError
				require.ErrorAs(t, err, &target)
				require.NotErrorIs(t, err, providerErr)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := decodeDijkstraTreasuryBody(t, test.treasuryValue)
			tx := &DijkstraTransaction{
				Body:      body,
				TxIsValid: test.isValid,
			}
			providerCalls := 0
			state := mockledger.NewLedgerStateBuilder().WithTreasuryValue(
				func() (uint64, error) {
					providerCalls++
					return test.ledgerValue, test.ledgerErr
				},
			).Build()
			err := common.VerifyTransaction(
				tx,
				0,
				state,
				&DijkstraProtocolParameters{
					ConwayProtocolParameters: conway.ConwayProtocolParameters{},
				},
				UtxoValidationRules,
			)
			test.checkError(t, err)
			require.Equal(t, test.wantProviderCall, providerCalls)
			var validationErr *common.ValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(
				t,
				test.wantRuleIndex,
				validationErr.Details["rule_index"],
			)
		})
	}
}

func decodeDijkstraTreasurySubTransaction(
	t *testing.T,
	treasuryValue *uint64,
) DijkstraSubTransaction {
	t.Helper()
	bodyFields := map[uint]any{}
	if treasuryValue != nil {
		bodyFields[21] = *treasuryValue
	}
	bodyCbor, err := cbor.Encode(bodyFields)
	require.NoError(t, err)
	var body DijkstraSubTransactionBody
	require.NoError(t, body.UnmarshalCBOR(bodyCbor))
	return DijkstraSubTransaction{Body: body}
}

func TestDijkstraCurrentTreasuryValueValidatesAllSubTransactions(
	t *testing.T,
) {
	t.Run("validates every sub-transaction before top level", func(t *testing.T) {
		body := decodeDijkstraTreasuryBody(t, dijkstraTreasuryValue(43))
		body.TxSubTransactions = cbor.NewSetType(
			[]DijkstraSubTransaction{
				decodeDijkstraTreasurySubTransaction(
					t,
					dijkstraTreasuryValue(42),
				),
				decodeDijkstraTreasurySubTransaction(
					t,
					dijkstraTreasuryValue(41),
				),
			},
			true,
		)
		tx := &DijkstraTransaction{
			Body:      body,
			TxIsValid: true,
		}
		providerCalls := 0
		state := mockledger.NewLedgerStateBuilder().WithTreasuryValue(
			func() (uint64, error) {
				providerCalls++
				return 42, nil
			},
		).Build()
		err := common.VerifyTransaction(
			tx,
			0,
			state,
			&DijkstraProtocolParameters{
				ConwayProtocolParameters: conway.ConwayProtocolParameters{},
			},
			UtxoValidationRules,
		)
		var target common.CurrentTreasuryValueMismatchError
		require.ErrorAs(t, err, &target)
		require.Equal(t, big.NewInt(41), target.Supplied)
		require.Equal(t, 1, providerCalls)
	})

	t.Run("top-level phase-2-invalid skips every sub-transaction", func(t *testing.T) {
		body := decodeDijkstraTreasuryBody(t, nil)
		body.TxSubTransactions = cbor.NewSetType(
			[]DijkstraSubTransaction{
				decodeDijkstraTreasurySubTransaction(
					t,
					dijkstraTreasuryValue(41),
				),
			},
			true,
		)
		tx := &DijkstraTransaction{Body: body}
		providerCalls := 0
		state := mockledger.NewLedgerStateBuilder().WithTreasuryValue(
			func() (uint64, error) {
				providerCalls++
				return 42, nil
			},
		).Build()
		err := common.VerifyTransaction(
			tx,
			0,
			state,
			&DijkstraProtocolParameters{
				ConwayProtocolParameters: conway.ConwayProtocolParameters{},
			},
			UtxoValidationRules,
		)
		var target common.InvalidIsValidFlagError
		require.ErrorAs(t, err, &target)
		require.Equal(t, 0, providerCalls)
	})
}

func TestDijkstraCurrentTreasuryValuePrecedesMetadata(t *testing.T) {
	body := decodeDijkstraTreasuryBody(t, dijkstraTreasuryValue(41))
	body.TxAuxDataHash = &common.Blake2b256{}
	tx := &DijkstraTransaction{
		Body:      body,
		TxIsValid: true,
	}
	state := mockledger.NewLedgerStateBuilder().
		WithTreasuryAmount(42).
		Build()
	err := common.VerifyTransaction(
		tx,
		0,
		state,
		&DijkstraProtocolParameters{
			ConwayProtocolParameters: conway.ConwayProtocolParameters{},
		},
		UtxoValidationRules,
	)
	var target common.CurrentTreasuryValueMismatchError
	require.ErrorAs(t, err, &target)
	var validationErr *common.ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.NotNil(t, validationErr)
	require.Equal(t, 0, validationErr.Details["rule_index"])
}
