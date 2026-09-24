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

package alonzo_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"math"
	"math/big"
	"testing"

	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUtxoValidateOutsideValidityIntervalUtxo(t *testing.T) {
	var testSlot uint64 = 555666777
	var testZeroSlot uint64 = 0
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxValidityIntervalStart: testSlot,
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testProtocolParams := &alonzo.AlonzoProtocolParameters{}
	var testBeforeSlot uint64 = 555666700
	var testAfterSlot uint64 = 555666799
	// Test helper function
	testRun := func(t *testing.T, name string, testSlot uint64, validateFunc func(*testing.T, error)) {
		t.Run(
			name,
			func(t *testing.T) {
				err := alonzo.UtxoValidateOutsideValidityIntervalUtxo(
					testTx,
					testSlot,
					testLedgerState,
					testProtocolParams,
				)
				validateFunc(t, err)
			},
		)
	}
	// Slot after validity interval start
	testRun(
		t,
		"slot after validity interval start",
		testAfterSlot,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateOutsideValidityIntervalUtxo should succeed when provided a slot (%d) after the specified validity interval start (%d)\n  got error: %v",
					testAfterSlot,
					testTx.ValidityIntervalStart(),
					err,
				)
			}
		},
	)
	// Slot equal to validity interval start
	testRun(
		t,
		"slot equal to validity interval start",
		testSlot,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateOutsideValidityIntervalUtxo should succeed when provided a slot (%d) equal to the specified validity interval start (%d)\n  got error: %v",
					testSlot,
					testTx.ValidityIntervalStart(),
					err,
				)
			}
		},
	)
	// Slot before validity interval start
	testRun(
		t,
		"slot before validity interval start",
		testBeforeSlot,
		func(t *testing.T, err error) {
			if err == nil {
				t.Errorf(
					"UtxoValidateOutsideValidityIntervalUtxo should fail when provided a slot (%d) before the specified validity interval start (%d)",
					testBeforeSlot,
					testTx.ValidityIntervalStart(),
				)
				return
			}
			testErrType := allegra.OutsideValidityIntervalUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
	// Zero TTL
	testTx.Body.TxValidityIntervalStart = testZeroSlot
	testRun(
		t,
		"zero validity interval start",
		testSlot,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateOutsideValidityIntervalUtxo should succeed when provided a zero validity interval start\n  got error: %v",
					err,
				)
			}
		},
	)
}

func TestUtxoValidateTransactionNetworkId(t *testing.T) {
	ledgerState := mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		Build()
	validate := func(tx *alonzo.AlonzoTransaction) error {
		return alonzo.UtxoValidateTransactionNetworkId(tx, 0, ledgerState, &alonzo.AlonzoProtocolParameters{})
	}

	t.Run("absent identifier is accepted", func(t *testing.T) {
		assert.NoError(t, validate(&alonzo.AlonzoTransaction{}))
	})
	t.Run("matching identifier is accepted", func(t *testing.T) {
		assert.NoError(t, validate(&alonzo.AlonzoTransaction{Body: alonzo.AlonzoTransactionBody{NetworkId: 1}}))
	})
	t.Run("mismatched identifier is rejected", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		tx.Body.SetNetworkIdPresence(true)
		err := validate(tx)
		require.Error(t, err)
		assert.IsType(t, common.WrongTransactionNetworkIdError{}, err)
	})
	t.Run("explicit testnet identifier decoded from CBOR is present", func(t *testing.T) {
		bodyCbor, err := cbor.Encode(map[uint]any{15: uint8(0)})
		require.NoError(t, err)
		var body alonzo.AlonzoTransactionBody
		require.NoError(t, body.UnmarshalCBOR(bodyCbor))
		err = validate(&alonzo.AlonzoTransaction{Body: body})
		require.Error(t, err)
		assert.IsType(t, common.WrongTransactionNetworkIdError{}, err)
	})
	t.Run("invalid identifier is rejected", func(t *testing.T) {
		err := validate(&alonzo.AlonzoTransaction{Body: alonzo.AlonzoTransactionBody{NetworkId: 2}})
		require.Error(t, err)
		assert.IsType(t, common.WrongTransactionNetworkIdError{}, err)
	})
}

func TestUtxoValidateInputSetEmptyUtxo(t *testing.T) {
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxInputs: shelley.NewShelleyTransactionInputSet(
				// Non-empty input set
				[]shelley.ShelleyTransactionInput{
					{},
				},
			),
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{}
	// Non-empty
	t.Run(
		"non-empty input set",
		func(t *testing.T) {
			err := alonzo.UtxoValidateInputSetEmptyUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateInputSetEmptyUtxo should succeed when provided a non-empty input set\n  got error: %v",
					err,
				)
			}
		},
	)
	// Empty
	testTx.Body.TxInputs.SetItems(nil)
	t.Run(
		"empty input set",
		func(t *testing.T) {
			err := alonzo.UtxoValidateInputSetEmptyUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateInputSetEmptyUtxo should fail when provided an empty input set\n  got error: %v",
					err,
				)
				return
			}
			testErrType := shelley.InputSetEmptyUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
}

func TestUtxoValidateFeeTooSmallUtxo(t *testing.T) {
	// NOTE: this is length 4, but body size will be used
	testTxCbor, _ := hex.DecodeString("abcdef01")
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxFee: 0, // Set to 0 to calculate minFee
		},
	}
	testTx.SetCbor(testTxCbor)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		MinFeeA: 7,
		MinFeeB: 53,
	}
	// Calculate minFee dynamically
	minFee, err := alonzo.MinFeeTx(testTx, testProtocolParams)
	if err != nil {
		t.Fatalf("failed to calculate minFee: %v", err)
	}
	var testExactFee uint64 = minFee
	var testBelowFee uint64 = minFee - 1
	var testAboveFee uint64 = minFee + 1
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testSlot := uint64(0)
	// Test helper function
	testRun := func(t *testing.T, name string, testFee uint64, validateFunc func(*testing.T, error)) {
		t.Run(
			name,
			func(t *testing.T) {
				tmpTestTx := testTx
				tmpTestTx.Body.TxFee = testFee
				err := alonzo.UtxoValidateFeeTooSmallUtxo(
					tmpTestTx,
					testSlot,
					testLedgerState,
					testProtocolParams,
				)
				validateFunc(t, err)
			},
		)
	}
	// Fee too low
	testRun(
		t,
		"fee too low",
		testBelowFee,
		func(t *testing.T, err error) {
			if err == nil {
				t.Errorf(
					"UtxoValidateFeeTooSmallUtxo should fail when provided too low of a fee",
				)
				return
			}
			testErrType := shelley.FeeTooSmallUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)

		},
	)
	// Exact fee
	testRun(
		t,
		"exact fee",
		testExactFee,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateFeeTooSmallUtxo should succeed when provided an exact fee\n  got error: %v",
					err,
				)
			}
		},
	)
	// Above min fee
	testRun(
		t,
		"above min fee",
		testAboveFee,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateFeeTooSmallUtxo should succeed when provided above the min fee\n  got error: %v",
					err,
				)
			}
		},
	)
}

func TestUtxoValidateBadInputsUtxo(t *testing.T) {
	testInputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	testGoodInput := shelley.NewShelleyTransactionInput(
		testInputTxId,
		0,
	)
	testBadInput := shelley.NewShelleyTransactionInput(
		testInputTxId,
		1,
	)
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{},
	}
	utxos := []common.Utxo{
		{Id: testGoodInput},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{}
	// Good input
	t.Run(
		"good input",
		func(t *testing.T) {
			testTx.Body.TxInputs = shelley.NewShelleyTransactionInputSet(
				[]shelley.ShelleyTransactionInput{testGoodInput},
			)
			err := alonzo.UtxoValidateBadInputsUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateBadInputsUtxo should succeed when provided a good input\n  got error: %v",
					err,
				)
			}
		},
	)
	// Bad input
	t.Run(
		"bad input",
		func(t *testing.T) {
			testTx.Body.TxInputs = shelley.NewShelleyTransactionInputSet(
				[]shelley.ShelleyTransactionInput{testBadInput},
			)
			err := alonzo.UtxoValidateBadInputsUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateBadInputsUtxo should fail when provided a bad input",
				)
				return
			}
			testErrType := shelley.BadInputsUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
}

func TestUtxoValidateWrongNetwork(t *testing.T) {
	testCorrectNetworkAddr, _ := common.NewAddress(
		"addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd",
	)
	testWrongNetworkAddr, _ := common.NewAddress(
		"addr_test1qqx80sj9nwxdnglmzdl95v2k40d9422au0klwav8jz2dj985v0wma0mza32f8z6pv2jmkn7cen50f9vn9jmp7dd0njcqqpce07",
	)
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxOutputs: []alonzo.AlonzoTransactionOutput{
				{
					OutputAmount: mary.MaryTransactionOutputValue{
						Amount: 123456,
					},
				},
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{}
	// Correct network
	t.Run(
		"correct network",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAddress = testCorrectNetworkAddr
			err := alonzo.UtxoValidateBadInputsUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateWrongNetwork should succeed when provided an address with the correct network ID\n  got error: %v",
					err,
				)
			}
		},
	)
	// Wrong network
	t.Run(
		"wrong network",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAddress = testWrongNetworkAddr
			err := alonzo.UtxoValidateWrongNetwork(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateWrongNetwork should fail when provided an address with the wrong network ID",
				)
				return
			}
			testErrType := shelley.WrongNetworkError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
}

func TestUtxoValidateWrongNetworkWithdrawal(t *testing.T) {
	testCorrectNetworkAddr, _ := common.NewAddress(
		"stake1uyehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gh6ffgw",
	)
	testWrongNetworkAddr, _ := common.NewAddress(
		"stake_test1uqehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gssrtvn",
	)
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{}
	// Correct network
	t.Run(
		"correct network",
		func(t *testing.T) {
			testTx.Body.TxWithdrawals[&testCorrectNetworkAddr] = 123456
			err := alonzo.UtxoValidateWrongNetworkWithdrawal(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateWrongNetworkWithdrawal should succeed when provided an address with the correct network ID\n  got error: %v",
					err,
				)
			}
		},
	)
	// Wrong network
	t.Run(
		"wrong network",
		func(t *testing.T) {
			testTx.Body.TxWithdrawals[&testWrongNetworkAddr] = 123456
			err := alonzo.UtxoValidateWrongNetworkWithdrawal(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateWrongNetworkWIthdrawal should fail when provided an address with the wrong network ID",
				)
				return
			}
			testErrType := shelley.WrongNetworkWithdrawalError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
}

func TestUtxoValidateValueNotConservedUtxo(t *testing.T) {
	testInputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	var testInputAmount uint64 = 555666777
	var testFee uint64 = 123456
	var testStakeDeposit uint64 = 2_000_000
	testOutputExactAmount := testInputAmount - testFee
	testOutputUnderAmount := testOutputExactAmount - 999
	testOutputOverAmount := testOutputExactAmount + 999
	testTx := &alonzo.AlonzoTransaction{
		TxIsValid: true,
		Body: alonzo.AlonzoTransactionBody{
			TxOutputs: []alonzo.AlonzoTransactionOutput{
				// Empty placeholder output
				{},
			},
			TxFee: testFee,
			TxInputs: shelley.NewShelleyTransactionInputSet(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(
						testInputTxId,
						0,
					),
				},
			),
		},
	}
	utxos := []common.Utxo{
		{
			Id: shelley.NewShelleyTransactionInput(testInputTxId, 0),
			Output: shelley.ShelleyTransactionOutput{
				OutputAmount: testInputAmount,
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		KeyDeposit: uint(testStakeDeposit),
	}
	// Exact amount
	t.Run(
		"exact amount",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputExactAmount
			err := alonzo.UtxoValidateValueNotConservedUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateValueNotConservedUtxo should succeed when inputs and outputs are balanced\n  got error: %v",
					err,
				)
			}
		},
	)
	// Stake registration
	t.Run(
		"stake registration",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputExactAmount - testStakeDeposit
			testTx.Body.TxCertificates = []common.CertificateWrapper{
				{
					Type: uint(common.CertificateTypeStakeRegistration),
					Certificate: &common.StakeRegistrationCertificate{
						StakeCredential: common.Credential{},
					},
				},
			}
			err := alonzo.UtxoValidateValueNotConservedUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateValueNotConservedUtxo should succeed when inputs and outputs are balanced\n  got error: %v",
					err,
				)
			}
		},
	)
	// Stake deregistration
	t.Run(
		"stake deregistration",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputExactAmount
			credential := common.Credential{}
			testTx.Body.TxCertificates = []common.CertificateWrapper{
				{Type: uint(common.CertificateTypeStakeDeregistration), Certificate: &common.StakeDeregistrationCertificate{StakeCredential: credential}},
				{Type: uint(common.CertificateTypeStakeRegistration), Certificate: &common.StakeRegistrationCertificate{StakeCredential: credential}},
				{Type: uint(common.CertificateTypeStakeDeregistration), Certificate: &common.StakeDeregistrationCertificate{StakeCredential: credential}},
			}
			err := alonzo.UtxoValidateValueNotConservedUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateValueNotConservedUtxo should succeed when inputs and outputs are balanced\n  got error: %v",
					err,
				)
			}
		},
	)
	// Output too low
	t.Run(
		"output too low",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputUnderAmount
			err := alonzo.UtxoValidateValueNotConservedUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateValueNotConservedUtxo should fail when the output amount is too low",
				)
				return
			}
			testErrType := shelley.ValueNotConservedUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
	// Output too high
	t.Run(
		"output too high",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputOverAmount
			err := alonzo.UtxoValidateValueNotConservedUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateValueNotConservedUtxo should fail when the output amount is too high",
				)
				return
			}
			testErrType := shelley.ValueNotConservedUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
}

func TestUtxoValidateOutputTooSmallUtxo(t *testing.T) {
	var testOutputAmountGood uint64 = 1234567
	var testOutputAmountBad uint64 = 123
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxOutputs: []alonzo.AlonzoTransactionOutput{
				// Empty placeholder output
				{},
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testSlot := uint64(0)
	// Alonzo gates outputs on utxoEntrySize * coinsPerUTxOWord. The
	// placeholder output is ada-only with no datum hash, so its entry size
	// is 29 words and the minimum is 29 * 34482 = 999978 lovelace.
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		AdaPerUtxoByte: 34482,
	}
	// Good
	t.Run(
		"sufficient coin",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputAmountGood
			err := alonzo.UtxoValidateOutputTooSmallUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateOutputTooSmallUtxo should succeed when outputs have sufficient coin\n  got error: %v",
					err,
				)
			}
		},
	)
	// Bad
	t.Run(
		"insufficient coin",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputAmountBad
			err := alonzo.UtxoValidateOutputTooSmallUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateOutputTooSmallUtxo should fail when the output amount is too low",
				)
				return
			}
			testErrType := shelley.OutputTooSmallUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
}

func TestUtxoValidateOutputTooBigUtxo(t *testing.T) {
	var testOutputValueGood = mary.MaryTransactionOutputValue{
		Amount: 1234567,
	}
	var tmpBadAssets = map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{}
	// Build too-large asset set
	// We create 45 random policy IDs and asset names in order to exceed the max value size (4000 bytes)
	for range 45 {
		tmpPolicyId := make([]byte, 28)
		if _, err := rand.Read(tmpPolicyId); err != nil {
			t.Fatalf("could not read random bytes")
		}
		tmpAssetName := make([]byte, 64)
		if _, err := rand.Read(tmpAssetName); err != nil {
			t.Fatalf("could not read random bytes")
		}
		tmpBadAssets[common.NewBlake2b224(tmpPolicyId)] = map[cbor.ByteString]common.MultiAssetTypeOutput{
			cbor.NewByteString(tmpAssetName): big.NewInt(1),
		}
	}
	tmpBadMultiAsset := common.NewMultiAsset(
		tmpBadAssets,
	)
	var testOutputValueBad = mary.MaryTransactionOutputValue{
		Amount: 1234567,
		Assets: &tmpBadMultiAsset,
	}
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxOutputs: []alonzo.AlonzoTransactionOutput{
				{},
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		MaxValueSize: 4000,
	}
	// Good
	t.Run(
		"not too large",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount = testOutputValueGood
			err := alonzo.UtxoValidateOutputTooBigUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateOutputTooBigUtxo should succeed when outputs are not too large\n  got error: %v",
					err,
				)
			}
		},
	)
	// Bad
	t.Run(
		"too large",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount = testOutputValueBad
			err := alonzo.UtxoValidateOutputTooBigUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateOutputTooBigUtxo should fail when the output value is too large",
				)
				return
			}
			testErrType := mary.OutputTooBigUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
}

func TestUtxoValidateOutputBootAddrAttrsTooBig(t *testing.T) {
	testGoodAddr, _ := common.NewAddress(
		"addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd",
	)
	// Generate random pubkey
	testBadAddrPubkey := make([]byte, 28)
	if _, err := rand.Read(testBadAddrPubkey); err != nil {
		t.Fatalf("could not read random bytes")
	}
	// Generate random large attribute payload
	testBadAddrAttrPayload := make([]byte, 100)
	if _, err := rand.Read(testBadAddrAttrPayload); err != nil {
		t.Fatalf("could not read random bytes")
	}
	testBadAddr, _ := common.NewByronAddressFromParts(
		common.ByronAddressTypePubkey,
		testBadAddrPubkey,
		common.ByronAddressAttributes{
			Payload: testBadAddrAttrPayload,
		},
	)
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxOutputs: []alonzo.AlonzoTransactionOutput{
				// Empty placeholder
				{},
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{}
	// Good
	t.Run(
		"Shelley address",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAddress = testGoodAddr
			err := alonzo.UtxoValidateOutputBootAddrAttrsTooBig(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateOutputBootAddrAttrsTooBig should succeed when outputs have sufficient coin\n  got error: %v",
					err,
				)
			}
		},
	)
	// Bad
	t.Run(
		"Byron address with large attribute payload",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAddress = testBadAddr
			err := alonzo.UtxoValidateOutputBootAddrAttrsTooBig(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateOutputBootAddrAttrsTooBig should fail when the output address has large Byron attributes payload",
				)
				return
			}
			testErrType := shelley.OutputBootAddrAttrsTooBigError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
}

func TestUtxoValidateMaxTxSizeUtxo(t *testing.T) {
	var testMaxTxSizeSmall uint = 2
	var testMaxTxSizeLarge uint = 64 * 1024
	testTx := &alonzo.AlonzoTransaction{}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{}
	// Transaction under limit
	t.Run(
		"transaction is under limit",
		func(t *testing.T) {
			testProtocolParams.MaxTxSize = testMaxTxSizeLarge
			err := alonzo.UtxoValidateMaxTxSizeUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateMaxTxSizeUtxo should succeed when the TX size is under the limit\n  got error: %v",
					err,
				)
			}
		},
	)
	// Transaction too large
	t.Run(
		"transaction is too large",
		func(t *testing.T) {
			testProtocolParams.MaxTxSize = testMaxTxSizeSmall
			err := alonzo.UtxoValidateMaxTxSizeUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateMaxTxSizeUtxo should fail when the TX size is too large",
				)
				return
			}
			testErrType := shelley.MaxTxSizeUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
}

func TestUtxoValidateInsufficientCollateral(t *testing.T) {
	testInputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	var testFee uint64 = 123456
	var testCollateralAmount1 uint64 = 100000
	var testCollateralAmount2 uint64 = 200000
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxFee: testFee,
		},
		WitnessSet: alonzo.AlonzoTransactionWitnessSet{
			WsRedeemers: alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{
					// Placeholder entry
					{},
				},
			},
		},
	}
	utxos := []common.Utxo{
		{
			Id: shelley.NewShelleyTransactionInput(testInputTxId, 0),
			Output: shelley.ShelleyTransactionOutput{
				OutputAmount: testCollateralAmount1,
			},
		},
		{
			Id: shelley.NewShelleyTransactionInput(testInputTxId, 1),
			Output: shelley.ShelleyTransactionOutput{
				OutputAmount: testCollateralAmount2,
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		CollateralPercentage: 150,
	}
	// Insufficient collateral
	t.Run(
		"insufficient collateral",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
				},
				false,
			)
			err := alonzo.UtxoValidateInsufficientCollateral(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateInsufficientCollateral should fail when insufficient collateral is provided",
				)
				return
			}
			testErrType := alonzo.InsufficientCollateralError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
	// Sufficient collateral
	t.Run(
		"sufficient collateral",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
					shelley.NewShelleyTransactionInput(testInputTxId, 1),
				},
				false,
			)
			err := alonzo.UtxoValidateInsufficientCollateral(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateInsufficientCollateral should succeed when sufficient collateral is provided\n  got error: %v",
					err,
				)
			}
		},
	)
}

func TestUtxoValidateInsufficientCollateralRejectsUnresolvedInput(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22",
		0,
	)
	tx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxCollateral: cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false),
		},
		WitnessSet: alonzo.AlonzoTransactionWitnessSet{
			WsRedeemers: alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{{}},
			},
		},
	}
	state := mockledger.NewLedgerStateBuilder().Build()
	err := alonzo.UtxoValidateInsufficientCollateral(
		tx,
		0,
		state,
		&alonzo.AlonzoProtocolParameters{},
	)
	var resolutionErr common.InputResolutionError
	assert.ErrorAs(t, err, &resolutionErr)
}

func TestUtxoValidateInsufficientCollateralRejectsNilOutput(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22",
		0,
	)
	tx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxCollateral: cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false),
		},
		WitnessSet: alonzo.AlonzoTransactionWitnessSet{
			WsRedeemers: alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{{}},
			},
		},
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, nil
		},
	).Build()
	err := alonzo.UtxoValidateInsufficientCollateral(
		tx,
		0,
		state,
		&alonzo.AlonzoProtocolParameters{},
	)
	var resolutionErr common.InputResolutionError
	assert.ErrorAs(t, err, &resolutionErr)
}

func TestUtxoValidateCollateralContainsNonAda(t *testing.T) {
	testInputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	var testCollateralAmount uint64 = 100000
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{},
		WitnessSet: alonzo.AlonzoTransactionWitnessSet{
			WsRedeemers: alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{
					// Placeholder entry
					{},
				},
			},
		},
	}
	tmpMultiAsset := common.NewMultiAsset[common.MultiAssetTypeOutput](
		map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{},
	)
	utxos := []common.Utxo{
		{
			Id: shelley.NewShelleyTransactionInput(testInputTxId, 0),
			Output: shelley.ShelleyTransactionOutput{
				OutputAmount: testCollateralAmount,
			},
		},
		{
			Id: shelley.NewShelleyTransactionInput(testInputTxId, 1),
			Output: alonzo.AlonzoTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: testCollateralAmount,
					Assets: &tmpMultiAsset,
				},
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{}
	// Coin and assets
	t.Run(
		"coin and assets",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
					shelley.NewShelleyTransactionInput(testInputTxId, 1),
				},
				false,
			)
			err := alonzo.UtxoValidateCollateralContainsNonAda(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateCollateralContainsNonAda should fail when collateral with assets is provided",
				)
				return
			}
			testErrType := alonzo.CollateralContainsNonAdaError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
	// Coin only
	t.Run(
		"coin only",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
				},
				false,
			)
			err := alonzo.UtxoValidateCollateralContainsNonAda(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateCollateralContainsNonAda should succeed when collateral with only coin is provided\n  got error: %v",
					err,
				)
			}
		},
	)
}

func TestUtxoValidateNoCollateralInputs(t *testing.T) {
	testInputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	var testCollateralAmount uint64 = 100000
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{},
		WitnessSet: alonzo.AlonzoTransactionWitnessSet{
			WsRedeemers: alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{
					// Placeholder entry
					{},
				},
			},
		},
	}
	utxos := []common.Utxo{
		{
			Id: shelley.NewShelleyTransactionInput(testInputTxId, 0),
			Output: shelley.ShelleyTransactionOutput{
				OutputAmount: testCollateralAmount,
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{}
	// No collateral
	t.Run(
		"no collateral",
		func(t *testing.T) {
			err := alonzo.UtxoValidateNoCollateralInputs(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateNoCollateralInputs should fail when no collateral is provided",
				)
				return
			}
			testErrType := alonzo.NoCollateralInputsError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
	// Collateral
	t.Run(
		"collateral",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
				},
				false,
			)
			err := alonzo.UtxoValidateNoCollateralInputs(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateNoCollateralInputs should succeed when collateral is provided\n  got error: %v",
					err,
				)
			}
		},
	)
}

func TestUtxoValidateExUnitsTooBigUtxo(t *testing.T) {
	testRedeemerSmall := alonzo.AlonzoRedeemer{
		ExUnits: common.ExUnits{
			Memory: 1_000_000,
			Steps:  2_000,
		},
	}
	testRedeemerLarge := alonzo.AlonzoRedeemer{
		ExUnits: common.ExUnits{
			Memory: 1_000_000_000,
			Steps:  2_000_000,
		},
	}
	testTx := &alonzo.AlonzoTransaction{
		WitnessSet: alonzo.AlonzoTransactionWitnessSet{},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		MaxTxExUnits: common.ExUnits{
			Memory: 5_000_000,
			Steps:  5_000,
		},
	}
	// A duplicated key contributes its budget once, as it does in the map the
	// ledger holds. Preview transaction 3ace3bc7f4c5 at slot 12925989 carries
	// the same (mint, 0) redeemer six times; summing the raw list rejected a
	// transaction the network accepted (blinklabs-io/dingo#3875). That block is
	// Babbage, but the rule is duplicated per era and the Alonzo copy summed
	// the raw list the same way.
	t.Run(
		"duplicate redeemer key counts once",
		func(t *testing.T) {
			dup := alonzo.AlonzoRedeemer{
				Tag:   common.RedeemerTagMint,
				Index: 0,
				ExUnits: common.ExUnits{
					Memory: 3_000_000,
					Steps:  3_000,
				},
			}
			testTx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{dup, dup, dup, dup, dup, dup},
			}
			// One copy is inside the 5,000,000 / 5,000 cap and two already
			// exceed it, so only a collapse to exactly one entry passes: an
			// over-count of any size, not just all six, fails here.
			if err := alonzo.UtxoValidateExUnitsTooBigUtxo(
				testTx, testSlot, testLedgerState, testProtocolParams,
			); err != nil {
				t.Errorf(
					"six copies of one redeemer must count once, got: %v", err,
				)
			}
		},
	)

	// Ex-units too large
	t.Run(
		"ExUnits too large",
		func(t *testing.T) {
			testTx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{testRedeemerLarge},
			}
			err := alonzo.UtxoValidateExUnitsTooBigUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateExUnitsTooBigUtxo should fail when no redeemer ExUnits are too large",
				)
				return
			}
			testErrType := alonzo.ExUnitsTooBigUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
	// Ex-units under limit
	t.Run(
		"ExUnits under limit",
		func(t *testing.T) {
			testTx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{testRedeemerSmall},
			}
			err := alonzo.UtxoValidateExUnitsTooBigUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateExUnitsTooBigUtxo should succeed when redeemer ExUnits are under the limit\n  got error: %v",
					err,
				)
			}
		},
	)
	// Ex-units overflow
	t.Run(
		"ExUnits overflow",
		func(t *testing.T) {
			// Distinct keys: this case is about addition overflowing, and
			// two entries sharing a key would collapse to one before the
			// sum ever happens.
			testTx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{
					{
						Index: 0,
						ExUnits: common.ExUnits{
							Memory: math.MaxInt64 - 10,
							Steps:  math.MaxInt64 - 10,
						},
					},
					{
						Index: 1,
						ExUnits: common.ExUnits{
							Memory: 100,
							Steps:  100,
						},
					},
				},
			}
			testProtocolParams.MaxTxExUnits = common.ExUnits{
				Memory: math.MaxInt64,
				Steps:  math.MaxInt64,
			}
			err := alonzo.UtxoValidateExUnitsTooBigUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateExUnitsTooBigUtxo should fail when ExUnits summation overflows",
				)
				return
			}
			testErrType := alonzo.ExUnitsTooBigUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
}

func TestUtxoValidateWitnessRules_Alonzo(t *testing.T) {
	// Required vkey witnesses
	t.Run("no required signers", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		err := alonzo.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("missing vkey witness", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		required := common.Blake2b224Hash([]byte{})
		tx.Body.TxRequiredSigners = cbor.NewSetType(
			[]common.Blake2b224{required},
			false,
		)
		err := alonzo.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for missing vkey witnesses")
		}
		assert.IsType(t, alonzo.MissingVKeyWitnessesError{}, err)
	})

	t.Run("mismatched vkey", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		required := common.Blake2b224Hash([]byte{})
		tx.Body.TxRequiredSigners = cbor.NewSetType(
			[]common.Blake2b224{required},
			false,
		)
		tx.WitnessSet.VkeyWitnesses = []common.VkeyWitness{
			{Vkey: []byte{0x01, 0x02, 0x03}},
		}
		err := alonzo.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for mismatched vkey witness")
		}
		assert.IsType(t, alonzo.MissingRequiredVKeyWitnessForSignerError{}, err)
	})

	t.Run("matching vkey", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		required := common.Blake2b224Hash([]byte{})
		tx.Body.TxRequiredSigners = cbor.NewSetType(
			[]common.Blake2b224{required},
			false,
		)
		tx.WitnessSet.VkeyWitnesses = []common.VkeyWitness{{Vkey: []byte{}}}
		err := alonzo.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	// Redeemer/script witness checks
	t.Run("no witness set", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		err := alonzo.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("script hash present but no redeemer", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		tx.Body.TxScriptDataHash = new(common.Blake2b256)
		err := alonzo.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf(
				"expected error for missing redeemers with script data hash",
			)
		}
		assert.IsType(t, alonzo.MissingRedeemersForScriptDataHashError{}, err)
	})

	t.Run("redeemers present but no scripts", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{ExUnits: common.ExUnits{Steps: 1, Memory: 1}},
			},
		}
		err := alonzo.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for redeemer without script")
		}
		assert.IsType(t, alonzo.MissingPlutusScriptWitnessesError{}, err)
	})

	t.Run("scripts present but no redeemers", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		tx.WitnessSet.WsPlutusV1Scripts = []common.PlutusV1Script{{}}
		err := alonzo.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for scripts without redeemer")
		}
		assert.IsType(t, alonzo.ExtraneousPlutusScriptWitnessesError{}, err)
	})

	t.Run("both redeemer and script present", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		tx.WitnessSet.WsPlutusV1Scripts = []common.PlutusV1Script{{}}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{ExUnits: common.ExUnits{Steps: 1, Memory: 1}},
			},
		}
		err := alonzo.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})
}

func TestUtxoValidatePlutusScriptsUnsupported_Alonzo(t *testing.T) {
	t.Run("valid tx with redeemer fails closed", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{
			TxIsValid: true,
		}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{
					Tag:     common.RedeemerTagSpend,
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		}

		err := alonzo.UtxoValidatePlutusScripts(tx, 0, nil, nil)
		assert.Error(t, err)
		assert.IsType(
			t,
			alonzo.PlutusScriptValidationUnsupportedError{},
			err,
		)
	})

	t.Run("invalid tx with redeemer is phase-1 valid", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{
					Tag:     common.RedeemerTagSpend,
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		}

		err := alonzo.UtxoValidatePlutusScripts(tx, 0, nil, nil)
		assert.NoError(t, err)
	})
}

func TestUtxoValidateExtraneousRedeemers_Alonzo(t *testing.T) {
	testInput := shelley.NewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000001",
		0,
	)
	testAddr := common.Address{}
	baseBody := func() alonzo.AlonzoTransactionBody {
		return alonzo.AlonzoTransactionBody{
			TxInputs: shelley.NewShelleyTransactionInputSet(
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
				&testAddr: 0,
			},
			TxMint: func() *common.MultiAsset[common.MultiAssetTypeMint] {
				ma := common.NewMultiAsset[common.MultiAssetTypeMint](
					map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeMint{
						common.NewBlake2b224([]byte{1, 2, 3}): {
							cbor.NewByteString([]byte("token")): big.NewInt(1),
						},
					},
				)
				return &ma
			}(),
		}
	}

	t.Run("unknown tag is extraneous", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{Tag: common.RedeemerTag(99)},
			},
		}
		err := alonzo.UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
		require.Error(t, err)
		assert.IsType(t, common.ExtraneousRedeemerError{}, err)
	})

	t.Run("guarding tag is extraneous", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{Tag: common.RedeemerTagGuarding},
			},
		}
		err := alonzo.UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
		require.Error(t, err)
		assert.IsType(t, common.ExtraneousRedeemerError{}, err)
	})

	t.Run("spend index out of range", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{Tag: common.RedeemerTagSpend, Index: 1},
			},
		}
		err := alonzo.UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
		require.Error(t, err)
		assert.IsType(t, common.ExtraneousRedeemerError{}, err)
	})

	t.Run("mint index out of range", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{Tag: common.RedeemerTagMint, Index: 1},
			},
		}
		err := alonzo.UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
		require.Error(t, err)
		assert.IsType(t, common.ExtraneousRedeemerError{}, err)
	})

	t.Run("cert index out of range", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{Tag: common.RedeemerTagCert, Index: 1},
			},
		}
		err := alonzo.UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
		require.Error(t, err)
		assert.IsType(t, common.ExtraneousRedeemerError{}, err)
	})

	t.Run("reward index out of range", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{Tag: common.RedeemerTagReward, Index: 1},
			},
		}
		err := alonzo.UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
		require.Error(t, err)
		assert.IsType(t, common.ExtraneousRedeemerError{}, err)
	})

	t.Run("in-range redeemers for every supported tag pass", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{Body: baseBody()}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{Tag: common.RedeemerTagSpend, Index: 0},
				{Tag: common.RedeemerTagMint, Index: 0},
				{Tag: common.RedeemerTagCert, Index: 0},
				{Tag: common.RedeemerTagReward, Index: 0},
			},
		}
		err := alonzo.UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("no redeemers is valid", func(t *testing.T) {
		tx := &alonzo.AlonzoTransaction{Body: baseBody()}
		err := alonzo.UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
		assert.NoError(t, err)
	})
}

// Alonzo prices a UTxO entry by utxoEntrySize(txOut) * coinsPerUTxOWord, where
// utxoEntrySize is 27 + Val.size(value) + dataHashSize(datumHash) in 8-byte
// words. The expected values below are derived from that formula with the
// mainnet Alonzo lovelacePerUTxOWord of 34482, not from this implementation.
//
// Reference: utxoEntrySize and getMinCoinTxOut in
// eras/alonzo/impl/src/Cardano/Ledger/Alonzo/TxOut.hs; Val.size for MaryValue
// and representationSize in eras/mary/impl/src/Cardano/Ledger/Mary/Value.hs.
func TestAlonzoMinCoinTxOut(t *testing.T) {
	t.Parallel()
	const lovelacePerUtxoWord = 34482
	testDatumHash := common.NewBlake2b256(make([]byte, 32))
	policyOne := common.NewBlake2b224(bytes.Repeat([]byte{0x01}, 28))
	policyTwo := common.NewBlake2b224(bytes.Repeat([]byte{0x02}, 28))
	oneAssetOnePolicy := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
			policyOne: {
				cbor.NewByteString([]byte("token")): big.NewInt(1),
			},
		},
	)
	twoAssetsTwoPolicies := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
			policyOne: {
				cbor.NewByteString([]byte("token")): big.NewInt(1),
			},
			policyTwo: {
				cbor.NewByteString([]byte("other")): big.NewInt(2),
			},
		},
	)
	testCases := []struct {
		name      string
		output    alonzo.AlonzoTransactionOutput
		entrySize uint64
	}{
		{
			// 27 + 2 = 29 words
			name:      "ada only",
			output:    alonzo.AlonzoTransactionOutput{},
			entrySize: 29,
		},
		{
			// 27 + 2 + 10 = 39 words
			name: "ada only with datum hash",
			output: alonzo.AlonzoTransactionOutput{
				OutputDatumHash: &testDatumHash,
			},
			entrySize: 39,
		},
		{
			// representationSize = 1*12 + 1*28 + len("token") = 45,
			// roundupBytesToWords(45) = 6, + repOverhead 6 = 12,
			// so 27 + 12 = 39 words
			name: "one asset one policy",
			output: alonzo.AlonzoTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{
					Assets: &oneAssetOnePolicy,
				},
			},
			entrySize: 39,
		},
		{
			// representationSize = 2*12 + 2*28 + len("token") +
			// len("other") = 90, roundupBytesToWords(90) = 12,
			// + repOverhead 6 = 18, so 27 + 18 = 45 words
			name: "two assets two policies",
			output: alonzo.AlonzoTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{
					Assets: &twoAssetsTwoPolicies,
				},
			},
			entrySize: 45,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			minCoin, err := alonzo.MinCoinTxOut(
				&testCase.output,
				&alonzo.AlonzoProtocolParameters{
					AdaPerUtxoByte: lovelacePerUtxoWord,
					// A non-zero flat Shelley minUTxOValue must not
					// influence the Alonzo result.
					MinUtxoValue: 100000,
				},
			)
			require.NoError(t, err)
			require.Equal(
				t,
				testCase.entrySize*lovelacePerUtxoWord,
				minCoin,
				"min-UTxO must be utxoEntrySize * coinsPerUTxOWord",
			)
		})
	}
}

func TestAlonzoMinCoinTxOutOverflow(t *testing.T) {
	t.Parallel()
	_, err := alonzo.MinCoinTxOut(
		&alonzo.AlonzoTransactionOutput{},
		&alonzo.AlonzoProtocolParameters{
			AdaPerUtxoByte: math.MaxUint64,
		},
	)
	require.ErrorContains(t, err, "overflow")
}

// The collateral rule compares balance * 100 against fee * collateralPercent
// rather than dividing, so the required amount is an exact ceiling. With
// fee 101 and collateralPercentage 150 the requirement is ceil(151.5) = 152,
// and 151 lovelace is one short.
//
// Reference: validateInsufficientCollateral in
// eras/alonzo/impl/src/Cardano/Ledger/Alonzo/Rules/Utxo.hs.
func TestUtxoValidateInsufficientCollateralRoundsUp(t *testing.T) {
	t.Parallel()
	testInputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		CollateralPercentage: 150,
	}
	validate := func(t *testing.T, fee, collateral uint64) error {
		t.Helper()
		tx := &alonzo.AlonzoTransaction{
			Body: alonzo.AlonzoTransactionBody{
				TxFee: fee,
				TxCollateral: cbor.NewSetType(
					[]shelley.ShelleyTransactionInput{
						shelley.NewShelleyTransactionInput(
							testInputTxId,
							0,
						),
					},
					false,
				),
			},
			WitnessSet: alonzo.AlonzoTransactionWitnessSet{
				WsRedeemers: alonzo.AlonzoRedeemers{
					Redeemers: []alonzo.AlonzoRedeemer{{}},
				},
			},
		}
		ls := mockledger.NewLedgerStateBuilder().WithUtxos(
			[]common.Utxo{
				{
					Id: shelley.NewShelleyTransactionInput(testInputTxId, 0),
					Output: shelley.ShelleyTransactionOutput{
						OutputAmount: collateral,
					},
				},
			},
		).Build()
		return alonzo.UtxoValidateInsufficientCollateral(
			tx,
			0,
			ls,
			testProtocolParams,
		)
	}
	t.Run("one lovelace short of the ceiling", func(t *testing.T) {
		t.Parallel()
		err := validate(t, 101, 151)
		var collateralErr alonzo.InsufficientCollateralError
		require.ErrorAs(t, err, &collateralErr)
		require.Equal(t, uint64(152), collateralErr.Required)
		require.Equal(t, uint64(151), collateralErr.Provided)
	})
	t.Run("exactly the ceiling", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validate(t, 101, 152))
	})
	t.Run("exact multiple of 100 is not rounded up", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validate(t, 100, 150))
	})
}

// The guard must sit exactly at the uint64 boundary: the largest product that
// still fits is a valid requirement and has to be returned exactly, while the
// next one up has to be rejected rather than wrapped to a small requirement.
func TestAlonzoMinCoinTxOutBoundary(t *testing.T) {
	t.Parallel()
	txOut := &alonzo.AlonzoTransactionOutput{}
	entrySize, err := alonzo.MinCoinTxOut(
		txOut,
		&alonzo.AlonzoProtocolParameters{AdaPerUtxoByte: 1},
	)
	require.NoError(t, err)
	require.Positive(t, entrySize)
	largest := uint64(math.MaxUint64) / entrySize
	minCoin, err := alonzo.MinCoinTxOut(
		txOut,
		&alonzo.AlonzoProtocolParameters{AdaPerUtxoByte: largest},
	)
	require.NoError(t, err)
	require.Equal(t, largest*entrySize, minCoin)
	_, err = alonzo.MinCoinTxOut(
		txOut,
		&alonzo.AlonzoProtocolParameters{AdaPerUtxoByte: largest + 1},
	)
	require.ErrorContains(t, err, "overflow")
}

func TestMinFeeIncludesDeclaredExecutionUnits(t *testing.T) {
	prices := common.ExUnitPrice{
		MemPrice:  &cbor.Rat{Rat: big.NewRat(1, 2)},
		StepPrice: &cbor.Rat{Rat: big.NewRat(1, 4)},
	}
	tx := &alonzo.AlonzoTransaction{
		WitnessSet: alonzo.AlonzoTransactionWitnessSet{
			WsRedeemers: alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{{
					Tag:     common.RedeemerTagSpend,
					ExUnits: common.ExUnits{Memory: 3},
				}},
			},
		},
	}
	tx.SetCbor([]byte{0x84, 0xa0, 0xa0, 0xf5, 0xf6})
	pp := &alonzo.AlonzoProtocolParameters{
		MinFeeA:        2,
		MinFeeB:        3,
		ExecutionCosts: prices,
	}
	txSize, err := common.TxSizeForFee(tx)
	require.NoError(t, err)
	baseFee, err := common.CalculateMinFee(txSize, pp.MinFeeA, pp.MinFeeB)
	require.NoError(t, err)
	minFee, err := alonzo.MinFeeTx(tx, pp)
	require.NoError(t, err)
	require.Equal(t, baseFee+2, minFee)

	tx.Body.TxFee = baseFee
	state := mockledger.NewLedgerStateBuilder().Build()
	require.ErrorAs(t, alonzo.UtxoValidateFeeTooSmallUtxo(tx, 0, state, pp), &shelley.FeeTooSmallUtxoError{})
	tx.Body.TxFee = minFee
	require.NoError(t, alonzo.UtxoValidateFeeTooSmallUtxo(tx, 0, state, pp))

	tx.WitnessSet = alonzo.AlonzoTransactionWitnessSet{}
	noRedeemerFee, err := alonzo.MinFeeTx(tx, pp)
	require.NoError(t, err)
	require.Equal(t, baseFee, noRedeemerFee)
}
