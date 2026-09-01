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
	"bytes"
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func conwayRuleIndex(
	t *testing.T,
	id common.UtxoValidationRuleId,
) int {
	t.Helper()
	for idx, descriptor := range conway.UtxoValidationRuleDescriptors() {
		if descriptor.Id == id {
			return idx
		}
	}
	t.Fatalf("Conway validation rule %q is not registered", id)
	return -1
}

func conwayTreasuryValue(value uint64) *uint64 {
	return &value
}

func conwayTreasuryScriptInput(
	t *testing.T,
	scriptValue common.Script,
	index int,
) (shelley.ShelleyTransactionInput, common.Utxo) {
	t.Helper()
	input := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		index,
	)
	address, err := common.NewAddressFromParts(
		common.AddressTypeScriptKey,
		common.AddressNetworkTestnet,
		scriptValue.Hash().Bytes(),
		bytes.Repeat([]byte{0x55}, common.AddressHashSize),
	)
	require.NoError(t, err)
	return input, common.Utxo{
		Id: input,
		Output: babbage.BabbageTransactionOutput{
			OutputAddress: address,
			OutputAmount: mary.MaryTransactionOutputValue{
				Amount: 2_000_000,
			},
		},
	}
}

func decodeConwayTreasuryBody(
	t *testing.T,
	treasuryValue *uint64,
) conway.ConwayTransactionBody {
	t.Helper()
	bodyFields := map[uint]any{
		0: cbor.NewSetType([]shelley.ShelleyTransactionInput{}, true),
		1: []babbage.BabbageTransactionOutput{},
		2: uint64(0),
	}
	if treasuryValue != nil {
		bodyFields[21] = int64(*treasuryValue)
	}
	bodyCbor, err := cbor.Encode(bodyFields)
	require.NoError(t, err)
	var body conway.ConwayTransactionBody
	require.NoError(t, body.UnmarshalCBOR(bodyCbor))
	reencoded, err := cbor.Encode(&body)
	require.NoError(t, err)
	require.Equal(t, bodyCbor, reencoded)
	return body
}

func TestConwayCurrentTreasuryValuePresence(t *testing.T) {
	absent := decodeConwayTreasuryBody(t, nil)
	require.False(t, absent.CurrentTreasuryValuePresent())
	require.Nil(t, absent.CurrentTreasuryValue())

	zero := decodeConwayTreasuryBody(t, conwayTreasuryValue(0))
	require.True(t, zero.CurrentTreasuryValuePresent())
	require.Zero(t, zero.CurrentTreasuryValue().Sign())

	nonzero := decodeConwayTreasuryBody(t, conwayTreasuryValue(42))
	require.True(t, nonzero.CurrentTreasuryValuePresent())
	require.Equal(t, big.NewInt(42), nonzero.CurrentTreasuryValue())

	constructed := conway.ConwayTransactionBody{}
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
}

func TestConwayCurrentTreasuryValueProductionRules(t *testing.T) {
	providerErr := errors.New("treasury provider failed")
	tests := []struct {
		name             string
		treasuryValue    *uint64
		isValid          bool
		ledgerValue      uint64
		ledgerErr        error
		wantProviderCall int
		wantRuleId       common.UtxoValidationRuleId
		checkError       func(*testing.T, error)
	}{
		{
			name:        "absent",
			isValid:     true,
			ledgerValue: 42,
			wantRuleId:  common.UtxoValidationRuleInputSetEmpty,
			checkError: func(t *testing.T, err error) {
				var target shelley.InputSetEmptyUtxoError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:             "equal",
			treasuryValue:    conwayTreasuryValue(42),
			isValid:          true,
			ledgerValue:      42,
			wantProviderCall: 1,
			wantRuleId:       common.UtxoValidationRuleInputSetEmpty,
			checkError: func(t *testing.T, err error) {
				var target shelley.InputSetEmptyUtxoError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:             "unequal",
			treasuryValue:    conwayTreasuryValue(41),
			isValid:          true,
			ledgerValue:      42,
			wantProviderCall: 1,
			wantRuleId:       common.UtxoValidationRuleCurrentTreasuryValue,
			checkError: func(t *testing.T, err error) {
				var target common.CurrentTreasuryValueMismatchError
				require.ErrorAs(t, err, &target)
				require.Equal(t, big.NewInt(41), target.Supplied)
				require.Equal(t, uint64(42), target.Expected)
			},
		},
		{
			name:             "present zero is unequal",
			treasuryValue:    conwayTreasuryValue(0),
			isValid:          true,
			ledgerValue:      42,
			wantProviderCall: 1,
			wantRuleId:       common.UtxoValidationRuleCurrentTreasuryValue,
			checkError: func(t *testing.T, err error) {
				var target common.CurrentTreasuryValueMismatchError
				require.ErrorAs(t, err, &target)
				require.Zero(t, target.Supplied.Sign())
				require.Equal(t, uint64(42), target.Expected)
			},
		},
		{
			name:             "provider error",
			treasuryValue:    conwayTreasuryValue(42),
			isValid:          true,
			ledgerErr:        providerErr,
			wantProviderCall: 1,
			wantRuleId:       common.UtxoValidationRuleCurrentTreasuryValue,
			checkError: func(t *testing.T, err error) {
				var target common.TreasuryValueQueryError
				require.ErrorAs(t, err, &target)
				require.ErrorIs(t, err, providerErr)
			},
		},
		{
			name:          "phase-2 invalid skips provider",
			treasuryValue: conwayTreasuryValue(41),
			ledgerErr:     providerErr,
			wantRuleId:    common.UtxoValidationRuleIsValidFlag,
			checkError: func(t *testing.T, err error) {
				var target common.InvalidIsValidFlagError
				require.ErrorAs(t, err, &target)
				require.NotErrorIs(t, err, providerErr)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := decodeConwayTreasuryBody(t, test.treasuryValue)
			tx := &conway.ConwayTransaction{
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
				&conway.ConwayProtocolParameters{},
				conway.UtxoValidationRules,
			)
			test.checkError(t, err)
			require.Equal(t, test.wantProviderCall, providerCalls)
			var validationErr *common.ValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(
				t,
				conwayRuleIndex(t, test.wantRuleId),
				validationErr.Details["rule_index"],
			)
		})
	}
}

func TestConwayCurrentTreasuryValueNilLedgerState(t *testing.T) {
	body := decodeConwayTreasuryBody(t, conwayTreasuryValue(42))
	tx := &conway.ConwayTransaction{
		Body:      body,
		TxIsValid: true,
	}
	var typedNilState *mockledger.MockLedgerState
	tests := []struct {
		name  string
		state common.LedgerState
	}{
		{name: "nil interface"},
		{name: "typed nil", state: typedNilState},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() {
				err = common.VerifyTransaction(
					tx,
					0,
					test.state,
					&conway.ConwayProtocolParameters{},
					conway.UtxoValidationRules,
				)
			})
			var target common.TreasuryValueQueryError
			require.ErrorAs(t, err, &target)
			var unavailable common.TreasuryValueProviderUnavailableError
			require.ErrorAs(t, err, &unavailable)
		})
	}
}

func TestConwayCurrentTreasuryValuePrecedesMetadata(t *testing.T) {
	body := decodeConwayTreasuryBody(t, conwayTreasuryValue(41))
	body.TxAuxDataHash = &common.Blake2b256{}
	tx := &conway.ConwayTransaction{
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
		&conway.ConwayProtocolParameters{},
		conway.UtxoValidationRules,
	)
	var target common.CurrentTreasuryValueMismatchError
	require.ErrorAs(t, err, &target)
	var validationErr *common.ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.NotNil(t, validationErr)
	require.Equal(
		t,
		conwayRuleIndex(t, common.UtxoValidationRuleCurrentTreasuryValue),
		validationErr.Details["rule_index"],
	)
}

func conwayTreasuryMint(scriptValue common.Script) *common.MultiAsset[common.MultiAssetTypeMint] {
	mint := common.NewMultiAsset[common.MultiAssetTypeMint](
		map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeMint{
			scriptValue.Hash(): {
				cbor.NewByteString([]byte("asset")): big.NewInt(1),
			},
		},
	)
	return &mint
}

func conwayTreasuryWitnessSet(
	t *testing.T,
	scriptValue common.Script,
) conway.ConwayTransactionWitnessSet {
	t.Helper()
	switch s := scriptValue.(type) {
	case common.PlutusV1Script:
		return conway.ConwayTransactionWitnessSet{
			WsPlutusV1Scripts: cbor.NewSetType([]common.PlutusV1Script{s}, true),
		}
	case common.PlutusV2Script:
		return conway.ConwayTransactionWitnessSet{
			WsPlutusV2Scripts: cbor.NewSetType([]common.PlutusV2Script{s}, true),
		}
	case common.PlutusV3Script:
		return conway.ConwayTransactionWitnessSet{
			WsPlutusV3Scripts: cbor.NewSetType([]common.PlutusV3Script{s}, true),
		}
	default:
		t.Fatalf("unsupported test script type %T", scriptValue)
		return conway.ConwayTransactionWitnessSet{}
	}
}

func TestConwayFeaturesWithPlutusV1V2NilLedgerState(t *testing.T) {
	unresolvedInput := shelley.NewShelleyTransactionInput(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		0,
	)
	tests := []struct {
		name        string
		script      common.Script
		useScript   bool
		wantVersion string
	}{
		{
			name:        "needed PlutusV1 witness",
			script:      common.PlutusV1Script{0x01},
			useScript:   true,
			wantVersion: "PlutusV1",
		},
		{
			name:        "needed PlutusV2 witness",
			script:      common.PlutusV2Script{0x02},
			useScript:   true,
			wantVersion: "PlutusV2",
		},
		{
			name:      "unused PlutusV1 witness",
			script:    common.PlutusV1Script{0x03},
			useScript: false,
		},
		{
			name:      "needed PlutusV3 witness",
			script:    common.PlutusV3Script{0x04},
			useScript: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := conway.ConwayTransactionBody{
				TxInputs: conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{unresolvedInput},
				),
				TxCurrentTreasuryValue: 42,
			}
			if test.useScript {
				body.TxMint = conwayTreasuryMint(test.script)
			}
			tx := &conway.ConwayTransaction{
				Body:       body,
				WitnessSet: conwayTreasuryWitnessSet(t, test.script),
				TxIsValid:  true,
			}
			err := conway.UtxoValidateConwayFeaturesWithPlutusV1V2(
				tx,
				0,
				nil,
				&conway.ConwayProtocolParameters{},
			)
			if test.wantVersion == "" {
				require.NoError(t, err)
				return
			}
			var target conway.CurrentTreasuryValueWithPlutusV1V2Error
			require.ErrorAs(
				t,
				err,
				&target,
				"a needed witness script must be detected without UTxO resolution",
			)
			require.Equal(t, test.wantVersion, target.PlutusVersion)
		})
	}
}

func TestConwayFeaturesWithPlutusV1V2ReferenceScripts(t *testing.T) {
	refInput := shelley.NewShelleyTransactionInput(
		"abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		0,
	)
	tests := []struct {
		name        string
		script      common.Script
		wantVersion string
	}{
		{
			name:        "PlutusV1",
			script:      common.PlutusV1Script{0x01},
			wantVersion: "PlutusV1",
		},
		{
			name:   "PlutusV3 control",
			script: common.PlutusV3Script{0x03},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxCurrentTreasuryValue: 42,
					TxMint:                 conwayTreasuryMint(test.script),
					TxReferenceInputs: cbor.NewSetType(
						[]shelley.ShelleyTransactionInput{refInput},
						true,
					),
				},
				TxIsValid: true,
			}
			err := conway.UtxoValidateConwayFeaturesWithPlutusV1V2(
				tx,
				0,
				nil,
				&conway.ConwayProtocolParameters{},
			)
			var resolutionErr common.ReferenceInputResolutionError
			require.ErrorAs(t, err, &resolutionErr)
			require.ErrorIs(t, err, common.ErrReferenceInputResolution)

			state := mockledger.NewLedgerStateBuilder().WithUtxos(
				[]common.Utxo{
					{
						Id: refInput,
						Output: babbage.BabbageTransactionOutput{
							TxOutScriptRef: &common.ScriptRef{Script: test.script},
						},
					},
				},
			).Build()
			err = conway.UtxoValidateConwayFeaturesWithPlutusV1V2(
				tx,
				0,
				state,
				&conway.ConwayProtocolParameters{},
			)
			if test.wantVersion == "" {
				require.NoError(t, err)
				return
			}
			var featureErr conway.CurrentTreasuryValueWithPlutusV1V2Error
			require.ErrorAs(t, err, &featureErr)
			require.Equal(t, test.wantVersion, featureErr.PlutusVersion)
		})
	}
}

func TestConwayCurrentTreasuryValuePresentZeroPlutusContexts(
	t *testing.T,
) {
	body := decodeConwayTreasuryBody(t, conwayTreasuryValue(0))
	tx := &conway.ConwayTransaction{
		Body:      body,
		TxIsValid: true,
	}
	state := mockledger.NewLedgerStateBuilder().Build()
	t.Run("PlutusV3 preserves Some zero", func(t *testing.T) {
		txInfo, err := script.NewTxInfoV3FromTransaction(state, tx, nil)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(0), txInfo.CurrentTreasuryAmount.Value)
	})

	tests := []struct {
		name        string
		script      common.Script
		witnesses   conway.ConwayTransactionWitnessSet
		wantVersion string
	}{
		{
			name:   "PlutusV1",
			script: common.PlutusV1Script{0x01},
			witnesses: conway.ConwayTransactionWitnessSet{
				WsPlutusV1Scripts: cbor.NewSetType(
					[]common.PlutusV1Script{{0x01}},
					true,
				),
			},
			wantVersion: "PlutusV1",
		},
		{
			name:   "PlutusV2",
			script: common.PlutusV2Script{0x01},
			witnesses: conway.ConwayTransactionWitnessSet{
				WsPlutusV2Scripts: cbor.NewSetType(
					[]common.PlutusV2Script{{0x01}},
					true,
				),
			},
			wantVersion: "PlutusV2",
		},
	}
	for idx, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, utxo := conwayTreasuryScriptInput(t, test.script, idx)
			tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			)
			tx.WitnessSet = test.witnesses
			err := conway.UtxoValidateConwayFeaturesWithPlutusV1V2(
				tx,
				0,
				mockledger.NewLedgerStateBuilder().WithUtxos(
					[]common.Utxo{utxo},
				).Build(),
				&conway.ConwayProtocolParameters{},
			)
			var target conway.CurrentTreasuryValueWithPlutusV1V2Error
			require.ErrorAs(t, err, &target)
			require.Equal(t, test.wantVersion, target.PlutusVersion)
		})
	}
}
