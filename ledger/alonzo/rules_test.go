// Copyright 2025 Blink Labs Software
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
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test "github.com/blinklabs-io/gouroboros/internal/test/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"

	"github.com/stretchr/testify/assert"
)

func TestUtxoValidateOutsideValidityIntervalUtxo(t *testing.T) {
	var testSlot uint64 = 555666777
	var testZeroSlot uint64 = 0
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxValidityIntervalStart: testSlot,
		},
	}
	testLedgerState := test.MockLedgerState{}
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
	testLedgerState := test.MockLedgerState{}
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
	var testExactFee uint64 = 74
	var testBelowFee uint64 = 73
	var testAboveFee uint64 = 75
	// NOTE: this is length 4, but 3 will be used in the calculations
	testTxCbor, _ := hex.DecodeString("abcdef01")
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxFee: testExactFee,
		},
	}
	testTx.SetCbor(testTxCbor)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		MinFeeA: 7,
		MinFeeB: 53,
	}
	testLedgerState := test.MockLedgerState{}
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
	testLedgerState := test.MockLedgerState{
		MockUtxos: []common.Utxo{
			{
				Id: testGoodInput,
			},
		},
	}
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
	testLedgerState := test.MockLedgerState{
		MockNetworkId: common.AddressNetworkMainnet,
	}
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
		"addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd",
	)
	testWrongNetworkAddr, _ := common.NewAddress(
		"addr_test1qqx80sj9nwxdnglmzdl95v2k40d9422au0klwav8jz2dj985v0wma0mza32f8z6pv2jmkn7cen50f9vn9jmp7dd0njcqqpce07",
	)
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{},
		},
	}
	testLedgerState := test.MockLedgerState{
		MockNetworkId: common.AddressNetworkMainnet,
	}
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
	testLedgerState := test.MockLedgerState{
		MockUtxos: []common.Utxo{
			{
				Id: shelley.NewShelleyTransactionInput(testInputTxId, 0),
				Output: shelley.ShelleyTransactionOutput{
					OutputAmount: testInputAmount,
				},
			},
		},
	}
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
					Type: common.CertificateTypeStakeRegistration,
					Certificate: &common.StakeRegistrationCertificate{
						StakeRegistration: common.Credential{},
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
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputExactAmount + testStakeDeposit
			testTx.Body.TxCertificates = []common.CertificateWrapper{
				{
					Type: common.CertificateTypeStakeRegistration,
					Certificate: &common.StakeDeregistrationCertificate{
						StakeDeregistration: common.Credential{},
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
	testLedgerState := test.MockLedgerState{}
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		MinUtxoValue: 100000,
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
	var tmpBadAssets = map[common.Blake2b224]map[cbor.ByteString]uint64{}
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
		tmpBadAssets[common.NewBlake2b224(tmpPolicyId)] = map[cbor.ByteString]uint64{
			cbor.NewByteString(tmpAssetName): 1,
		}
	}
	tmpBadMultiAsset := common.NewMultiAsset[common.MultiAssetTypeOutput](
		tmpBadAssets,
	)
	var testOutputValueBad = mary.MaryTransactionOutputValue{
		Amount: 1234567,
		Assets: &tmpBadMultiAsset,
	}
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{
			TxOutputs: []alonzo.AlonzoTransactionOutput{
				// Empty placeholder output
				{},
			},
		},
	}
	testLedgerState := test.MockLedgerState{}
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
	testLedgerState := test.MockLedgerState{}
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
	testLedgerState := test.MockLedgerState{}
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
			WsRedeemers: []alonzo.AlonzoRedeemer{
				// Placeholder entry
				{},
			},
		},
	}
	testLedgerState := test.MockLedgerState{
		MockUtxos: []common.Utxo{
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
		},
	}
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		CollateralPercentage: 150,
	}
	// Insufficient collateral
	t.Run(
		"insufficient collateral",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType[shelley.ShelleyTransactionInput](
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
			testTx.Body.TxCollateral = cbor.NewSetType[shelley.ShelleyTransactionInput](
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

func TestUtxoValidateCollateralContainsNonAda(t *testing.T) {
	testInputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	var testCollateralAmount uint64 = 100000
	testTx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{},
		WitnessSet: alonzo.AlonzoTransactionWitnessSet{
			WsRedeemers: []alonzo.AlonzoRedeemer{
				// Placeholder entry
				{},
			},
		},
	}
	tmpMultiAsset := common.NewMultiAsset[common.MultiAssetTypeOutput](
		map[common.Blake2b224]map[cbor.ByteString]uint64{},
	)
	testLedgerState := test.MockLedgerState{
		MockUtxos: []common.Utxo{
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
		},
	}
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{}
	// Coin and assets
	t.Run(
		"coin and assets",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType[shelley.ShelleyTransactionInput](
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
			testTx.Body.TxCollateral = cbor.NewSetType[shelley.ShelleyTransactionInput](
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
			WsRedeemers: []alonzo.AlonzoRedeemer{
				// Placeholder entry
				{},
			},
		},
	}
	testLedgerState := test.MockLedgerState{
		MockUtxos: []common.Utxo{
			{
				Id: shelley.NewShelleyTransactionInput(testInputTxId, 0),
				Output: shelley.ShelleyTransactionOutput{
					OutputAmount: testCollateralAmount,
				},
			},
		},
	}
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
			testTx.Body.TxCollateral = cbor.NewSetType[shelley.ShelleyTransactionInput](
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
	testLedgerState := test.MockLedgerState{}
	testSlot := uint64(0)
	testProtocolParams := &alonzo.AlonzoProtocolParameters{
		MaxTxExUnits: common.ExUnits{
			Memory: 5_000_000,
			Steps:  5_000,
		},
	}
	// Ex-units too large
	t.Run(
		"ExUnits too large",
		func(t *testing.T) {
			testTx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
				testRedeemerLarge,
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
				testRedeemerSmall,
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
}
