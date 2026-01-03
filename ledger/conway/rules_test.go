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

package conway_test

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test_ledger "github.com/blinklabs-io/gouroboros/internal/test/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
)

func TestUtxoValidateWitnessRules_Conway(t *testing.T) {
	// Required vkey witnesses
	t.Run("no required signers", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		err := conway.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("missing vkey witness", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		required := common.Blake2b224Hash([]byte{})
		tx.Body.TxRequiredSigners = cbor.NewSetType(
			[]common.Blake2b224{required},
			false,
		)
		err := conway.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for missing vkey witnesses")
		}
		assert.IsType(t, conway.MissingVKeyWitnessesError{}, err)
	})

	t.Run("mismatched vkey", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		required := common.Blake2b224Hash([]byte{})
		tx.Body.TxRequiredSigners = cbor.NewSetType(
			[]common.Blake2b224{required},
			false,
		)
		tx.WitnessSet.VkeyWitnesses = cbor.NewSetType(
			[]common.VkeyWitness{{Vkey: []byte{0x01, 0x02, 0x03}}},
			false,
		)
		err := conway.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for mismatched vkey witness")
		}
		assert.IsType(t, conway.MissingRequiredVKeyWitnessForSignerError{}, err)
	})

	t.Run("matching vkey", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		required := common.Blake2b224Hash([]byte{})
		tx.Body.TxRequiredSigners = cbor.NewSetType(
			[]common.Blake2b224{required},
			false,
		)
		tx.WitnessSet.VkeyWitnesses = cbor.NewSetType(
			[]common.VkeyWitness{{Vkey: []byte{}}},
			false,
		)
		err := conway.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	// Redeemer/script witness checks (Plutus V1+V2+V3)
	t.Run("no script/redeemer", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		err := conway.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("script hash present but no redeemer", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		tx.Body.TxScriptDataHash = new(common.Blake2b256)
		err := conway.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf(
				"expected error for missing redeemers with script data hash",
			)
		}
		assert.IsType(t, conway.MissingRedeemersForScriptDataHashError{}, err)
	})

	t.Run("redeemer without script", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		tx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: {
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		}
		err := conway.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for redeemer without script")
		}
		assert.IsType(t, conway.MissingPlutusScriptWitnessesError{}, err)
	})

	t.Run("plutus v1 script without redeemer", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		tx.WitnessSet.WsPlutusV1Scripts = cbor.NewSetType(
			[]common.PlutusV1Script{{}},
			false,
		)
		err := conway.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for Plutus v1 script without redeemer")
		}
		assert.IsType(t, conway.ExtraneousPlutusScriptWitnessesError{}, err)
	})

	t.Run("plutus v2 script without redeemer", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		tx.WitnessSet.WsPlutusV2Scripts = cbor.NewSetType(
			[]common.PlutusV2Script{{}},
			false,
		)
		err := conway.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for Plutus v2 script without redeemer")
		}
		assert.IsType(t, conway.ExtraneousPlutusScriptWitnessesError{}, err)
	})

	t.Run("plutus v3 script without redeemer", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		tx.WitnessSet.WsPlutusV3Scripts = cbor.NewSetType(
			[]common.PlutusV3Script{{}},
			false,
		)
		err := conway.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for Plutus v3 script without redeemer")
		}
		assert.IsType(t, conway.ExtraneousPlutusScriptWitnessesError{}, err)
	})

	t.Run("both redeemer and plutus v1 script", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		tx.WitnessSet.WsPlutusV1Scripts = cbor.NewSetType(
			[]common.PlutusV1Script{{}},
			false,
		)
		tx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: {
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		}
		err := conway.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("both redeemer and plutus v2 script", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		tx.WitnessSet.WsPlutusV2Scripts = cbor.NewSetType(
			[]common.PlutusV2Script{{}},
			false,
		)
		tx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: {
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		}
		err := conway.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("both redeemer and plutus v3 script", func(t *testing.T) {
		tx := &conway.ConwayTransaction{}
		tx.WitnessSet.WsPlutusV3Scripts = cbor.NewSetType(
			[]common.PlutusV3Script{{}},
			false,
		)
		tx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: {
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		}
		err := conway.UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})
}

func TestUtxoValidateOutsideValidityIntervalUtxo(t *testing.T) {
	var testSlot uint64 = 555666777
	var testZeroSlot uint64 = 0
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxValidityIntervalStart: testSlot,
		},
	}
	testLedgerState := &test_ledger.MockLedgerState{}
	testProtocolParams := &conway.ConwayProtocolParameters{}
	var testBeforeSlot uint64 = 555666700
	var testAfterSlot uint64 = 555666799
	// Test helper function
	testRun := func(t *testing.T, name string, testSlot uint64, validateFunc func(*testing.T, error)) {
		t.Run(
			name,
			func(t *testing.T) {
				err := conway.UtxoValidateOutsideValidityIntervalUtxo(
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
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				// Non-empty input set
				[]shelley.ShelleyTransactionInput{
					{},
				},
			),
		},
	}
	testLedgerState := &test_ledger.MockLedgerState{}
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{}
	// Non-empty
	t.Run(
		"non-empty input set",
		func(t *testing.T) {
			err := conway.UtxoValidateInputSetEmptyUtxo(
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
			err := conway.UtxoValidateInputSetEmptyUtxo(
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
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxFee: 0, // Set to 0 to calculate minFee
		},
	}
	testTx.SetCbor(testTxCbor)
	testProtocolParams := &conway.ConwayProtocolParameters{
		MinFeeA: 7,
		MinFeeB: 53,
	}
	// Calculate minFee dynamically
	minFee, err := conway.MinFeeTx(testTx, testProtocolParams)
	if err != nil {
		t.Fatalf("failed to calculate minFee: %v", err)
	}
	var testExactFee uint64 = minFee
	var testBelowFee uint64 = minFee - 1
	var testAboveFee uint64 = minFee + 1
	testLedgerState := &test_ledger.MockLedgerState{}
	testSlot := uint64(0)
	// Test helper function
	testRun := func(t *testing.T, name string, testFee uint64, validateFunc func(*testing.T, error)) {
		t.Run(
			name,
			func(t *testing.T) {
				tmpTestTx := testTx
				tmpTestTx.Body.TxFee = testFee
				err := conway.UtxoValidateFeeTooSmallUtxo(
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
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{},
	}
	utxos := []common.Utxo{{Id: testGoodInput}}
	testLedgerState := test_ledger.NewMockLedgerStateWithUtxos(utxos)

	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{}
	// Good input
	t.Run(
		"good input",
		func(t *testing.T) {
			testTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{testGoodInput},
			)
			err := conway.UtxoValidateBadInputsUtxo(
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
			testTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{testBadInput},
			)
			err := conway.UtxoValidateBadInputsUtxo(
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
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxOutputs: []babbage.BabbageTransactionOutput{
				{
					OutputAmount: mary.MaryTransactionOutputValue{
						Amount: 123456,
					},
				},
			},
		},
	}
	testLedgerState := &test_ledger.MockLedgerState{
		NetworkIdVal: common.AddressNetworkMainnet,
	}
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{}
	// Correct network
	t.Run(
		"correct network",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAddress = testCorrectNetworkAddr
			err := conway.UtxoValidateBadInputsUtxo(
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
			err := conway.UtxoValidateWrongNetwork(
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
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{},
		},
	}
	testLedgerState := &test_ledger.MockLedgerState{
		NetworkIdVal: common.AddressNetworkMainnet,
	}
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{}
	// Correct network
	t.Run(
		"correct network",
		func(t *testing.T) {
			testTx.Body.TxWithdrawals[&testCorrectNetworkAddr] = 123456
			err := conway.UtxoValidateWrongNetworkWithdrawal(
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
			err := conway.UtxoValidateWrongNetworkWithdrawal(
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
	var testDepositAmount uint64 = 1_500_000
	testOutputExactAmount := testInputAmount - testFee
	testOutputUnderAmount := testOutputExactAmount - 999
	testOutputOverAmount := testOutputExactAmount + 999
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
				},
			),
			TxOutputs: []babbage.BabbageTransactionOutput{
				// Empty placeholder output
				{},
			},
			TxFee: testFee,
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
	testLedgerState := test_ledger.NewMockLedgerStateWithUtxos(utxos)
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{
		KeyDeposit: uint(testStakeDeposit),
	}
	// Exact amount
	t.Run(
		"exact amount",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputExactAmount
			err := conway.UtxoValidateValueNotConservedUtxo(
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
			err := conway.UtxoValidateValueNotConservedUtxo(
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
					Type: uint(common.CertificateTypeStakeDeregistration),
					Certificate: &common.StakeDeregistrationCertificate{
						StakeCredential: common.Credential{},
					},
				},
			}
			err := conway.UtxoValidateValueNotConservedUtxo(
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
			err := conway.UtxoValidateValueNotConservedUtxo(
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
			err := conway.UtxoValidateValueNotConservedUtxo(
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
	// CIP-0094 Registration certificate with valid deposit
	t.Run(
		"registration certificate valid deposit",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputExactAmount - testDepositAmount // Subtract deposit from output
			testTx.Body.TxCertificates = []common.CertificateWrapper{
				{
					Type: uint(common.CertificateTypeRegistration),
					Certificate: &common.RegistrationCertificate{
						StakeCredential: common.Credential{},
						Amount:          int64(testDepositAmount),
					},
				},
			}
			err := conway.UtxoValidateValueNotConservedUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateValueNotConservedUtxo should succeed with valid registration deposit\n  got error: %v",
					err,
				)
			}
		},
	)
	// CIP-0094 Registration certificate with invalid deposit (zero)
	t.Run(
		"registration certificate invalid deposit zero",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputExactAmount
			testTx.Body.TxCertificates = []common.CertificateWrapper{
				{
					Type: uint(common.CertificateTypeRegistration),
					Certificate: &common.RegistrationCertificate{
						StakeCredential: common.Credential{},
						Amount:          0,
					},
				},
			}
			err := conway.UtxoValidateValueNotConservedUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateValueNotConservedUtxo should fail with zero registration deposit",
				)
				return
			}
			testErrType := shelley.InvalidCertificateDepositError{}
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
	// CIP-0094 Deregistration certificate with valid refund
	t.Run(
		"deregistration certificate valid refund",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputExactAmount + testDepositAmount // Add refund to output
			testTx.Body.TxCertificates = []common.CertificateWrapper{
				{
					Type: uint(common.CertificateTypeDeregistration),
					Certificate: &common.DeregistrationCertificate{
						StakeCredential: common.Credential{},
						Amount:          int64(testDepositAmount),
					},
				},
			}
			err := conway.UtxoValidateValueNotConservedUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateValueNotConservedUtxo should succeed with valid deregistration refund\n  got error: %v",
					err,
				)
			}
		},
	)
	// CIP-0094 Deregistration certificate with invalid refund (zero)
	t.Run(
		"deregistration certificate invalid refund zero",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputExactAmount
			testTx.Body.TxCertificates = []common.CertificateWrapper{
				{
					Type: uint(common.CertificateTypeDeregistration),
					Certificate: &common.DeregistrationCertificate{
						StakeCredential: common.Credential{},
						Amount:          0,
					},
				},
			}
			err := conway.UtxoValidateValueNotConservedUtxo(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateValueNotConservedUtxo should fail with zero deregistration refund",
				)
				return
			}
			testErrType := shelley.InvalidCertificateDepositError{}
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
	// Minting
	t.Run(
		"minting",
		func(t *testing.T) {
			mintData := map[common.Blake2b224]map[cbor.ByteString]int64{
				{}: {cbor.ByteString{}: 7000000},
			}
			mint := common.NewMultiAsset[common.MultiAssetTypeMint](mintData)
			mintTx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxInputs: conway.NewConwayTransactionInputSet(
						[]shelley.ShelleyTransactionInput{},
					),
					TxOutputs: []babbage.BabbageTransactionOutput{
						{
							OutputAmount: mary.MaryTransactionOutputValue{
								Amount: 7000000,
							},
						},
					},
					TxFee:  0,
					TxMint: &mint,
				},
			}
			err := conway.UtxoValidateValueNotConservedUtxo(
				mintTx,
				testSlot,
				&test_ledger.MockLedgerState{},
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateValueNotConservedUtxo should succeed with minting\n  got error: %v",
					err,
				)
			}
		},
	)
}

func TestUtxoValidateOutputTooSmallUtxo(t *testing.T) {
	var testOutputAmountGood uint64 = 1234567
	var testOutputAmountBad uint64 = 123
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxOutputs: []babbage.BabbageTransactionOutput{
				// Empty placeholder output
				{},
			},
		},
	}
	testLedgerState := &test_ledger.MockLedgerState{}
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{
		AdaPerUtxoByte: 50,
	}
	// Good
	t.Run(
		"sufficient coin",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount.Amount = testOutputAmountGood
			err := conway.UtxoValidateOutputTooSmallUtxo(
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
			err := conway.UtxoValidateOutputTooSmallUtxo(
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
	tmpBadMultiAsset := common.NewMultiAsset(
		tmpBadAssets,
	)
	var testOutputValueBad = mary.MaryTransactionOutputValue{
		Amount: 1234567,
		Assets: &tmpBadMultiAsset,
	}
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxOutputs: []babbage.BabbageTransactionOutput{
				// Empty placeholder output
				{},
			},
		},
	}
	testLedgerState := &test_ledger.MockLedgerState{}
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{
		MaxValueSize: 4000,
	}
	// Good
	t.Run(
		"not too large",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount = testOutputValueGood
			err := conway.UtxoValidateOutputTooBigUtxo(
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
			err := conway.UtxoValidateOutputTooBigUtxo(
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
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxOutputs: []babbage.BabbageTransactionOutput{
				// Empty placeholder
				{},
			},
		},
	}
	testLedgerState := &test_ledger.MockLedgerState{}
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{}
	// Good
	t.Run(
		"Shelley address",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAddress = testGoodAddr
			err := conway.UtxoValidateOutputBootAddrAttrsTooBig(
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
			err := conway.UtxoValidateOutputBootAddrAttrsTooBig(
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
	testTx := &conway.ConwayTransaction{}
	testLedgerState := &test_ledger.MockLedgerState{}
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{}
	// Transaction under limit
	t.Run(
		"transaction is under limit",
		func(t *testing.T) {
			testProtocolParams.MaxTxSize = testMaxTxSizeLarge
			err := conway.UtxoValidateMaxTxSizeUtxo(
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
			err := conway.UtxoValidateMaxTxSizeUtxo(
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
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxFee: testFee,
		},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsRedeemers: conway.ConwayRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					// Placeholder entry
					{}: {},
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
	testLedgerState := test_ledger.NewMockLedgerStateWithUtxos(utxos)
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{
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
			err := conway.UtxoValidateInsufficientCollateral(
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
			err := conway.UtxoValidateInsufficientCollateral(
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
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxTotalCollateral: testCollateralAmount,
		},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsRedeemers: conway.ConwayRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					// Placeholder entry
					{}: {},
				},
			},
		},
	}
	tmpMultiAsset := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]uint64{
			common.Blake2b224Hash([]byte("abcd")): {
				cbor.NewByteString([]byte("efgh")): 123,
			},
		},
	)
	tmpZeroMultiAsset := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]uint64{
			common.Blake2b224Hash([]byte("abcd")): {
				cbor.NewByteString([]byte("efgh")): 0,
			},
		},
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
			Output: babbage.BabbageTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: testCollateralAmount,
					Assets: &tmpMultiAsset,
				},
			},
		},
		{
			Id: shelley.NewShelleyTransactionInput(testInputTxId, 2),
			Output: babbage.BabbageTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: testCollateralAmount,
					Assets: &tmpZeroMultiAsset,
				},
			},
		},
	}
	testLedgerState := test_ledger.NewMockLedgerStateWithUtxos(utxos)
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{}
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
			err := conway.UtxoValidateCollateralContainsNonAda(
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
			err := conway.UtxoValidateCollateralContainsNonAda(
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
	// Coin and assets with return
	t.Run(
		"coin and assets with return",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
					shelley.NewShelleyTransactionInput(testInputTxId, 1),
				},
				false,
			)
			testTx.Body.TxCollateralReturn = &babbage.BabbageTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: testCollateralAmount,
					Assets: &tmpMultiAsset,
				},
			}
			err := conway.UtxoValidateCollateralContainsNonAda(
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
	// Coin and zero assets with return
	t.Run(
		"coin and zero assets with return",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 2),
				},
				false,
			)
			testTx.Body.TxCollateralReturn = &babbage.BabbageTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: testCollateralAmount,
				},
			}
			err := conway.UtxoValidateCollateralContainsNonAda(
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
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			WsRedeemers: conway.ConwayRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					// Placeholder entry
					{}: {},
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
	testLedgerState := test_ledger.NewMockLedgerStateWithUtxos(utxos)
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{}
	// No collateral
	t.Run(
		"no collateral",
		func(t *testing.T) {
			err := conway.UtxoValidateNoCollateralInputs(
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
			err := conway.UtxoValidateNoCollateralInputs(
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
	testRedeemerSmall := common.RedeemerValue{
		ExUnits: common.ExUnits{
			Memory: 1_000_000,
			Steps:  2_000,
		},
	}
	testRedeemerLarge := common.RedeemerValue{
		ExUnits: common.ExUnits{
			Memory: 1_000_000_000,
			Steps:  2_000_000,
		},
	}
	testTx := &conway.ConwayTransaction{
		WitnessSet: conway.ConwayTransactionWitnessSet{},
	}
	testLedgerState := &test_ledger.MockLedgerState{}
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{
		MaxTxExUnits: common.ExUnits{
			Memory: 5_000_000,
			Steps:  5_000,
		},
	}
	// Ex-units too large
	t.Run(
		"ExUnits too large",
		func(t *testing.T) {
			testTx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{}: testRedeemerLarge,
				},
			}
			err := conway.UtxoValidateExUnitsTooBigUtxo(
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
			testTx.WitnessSet.WsRedeemers = conway.ConwayRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{}: testRedeemerSmall,
				},
			}
			err := conway.UtxoValidateExUnitsTooBigUtxo(
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

func TestUtxoValidateDisjointRefInputs(t *testing.T) {
	// Test validation for reference inputs (CIP-0031)
	testInputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{},
	}
	testLedgerState := &test_ledger.MockLedgerState{}
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{}
	// Non-disjoint ref inputs
	t.Run(
		"non-disjoint ref inputs",
		func(t *testing.T) {
			testTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
				},
			)
			testTx.Body.TxReferenceInputs = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
				},
				false,
			)
			err := conway.UtxoValidateDisjointRefInputs(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateDisjointRefInputs should fail when inputs and ref inputs are duplicated",
				)
				return
			}
			testErrType := conway.NonDisjointRefInputsError{}
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
	// Disjoint ref inputs
	t.Run(
		"disjoint ref inputs",
		func(t *testing.T) {
			testTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
				},
			)
			testTx.Body.TxReferenceInputs = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 1),
				},
				false,
			)
			err := conway.UtxoValidateDisjointRefInputs(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateDisjointRefInputs should succeed when inputs and ref inputs are not duplicated\n  got error: %v",
					err,
				)
			}
		},
	)
}

func TestUtxoValidateCollateralEqBalance(t *testing.T) {
	testInputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	var testInputAmount uint64 = 20_000_000
	var testTotalCollateral uint64 = 5_000_000
	var testCollateralReturnAmountGood uint64 = 15_000_000
	var testCollateralReturnAmountBad uint64 = 16_000_000
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxTotalCollateral: testTotalCollateral,
			TxCollateral: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
				},
				false,
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
	testLedgerState := test_ledger.NewMockLedgerStateWithUtxos(utxos)
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{}
	// Too much collateral return
	t.Run(
		"too much collateral return",
		func(t *testing.T) {
			testTx.Body.TxCollateralReturn = &babbage.BabbageTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: testCollateralReturnAmountBad,
				},
			}
			err := conway.UtxoValidateCollateralEqBalance(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateCollateralEqBalance should fail when collateral doesn't equal balance",
				)
				return
			}
			testErrType := babbage.IncorrectTotalCollateralFieldError{}
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
	// Collateral equals balance
	t.Run(
		"collateral equals balance",
		func(t *testing.T) {
			testTx.Body.TxCollateralReturn = &babbage.BabbageTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: testCollateralReturnAmountGood,
				},
			}
			err := conway.UtxoValidateCollateralEqBalance(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateCollateralEqBalance should succeed when collateral equals balance\n  got error: %v",
					err,
				)
			}
		},
	)
}

func TestUtxoValidateTooManyCollateralInputs(t *testing.T) {
	testInputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	testTx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{},
	}
	testLedgerState := &test_ledger.MockLedgerState{}
	testSlot := uint64(0)
	testProtocolParams := &conway.ConwayProtocolParameters{
		MaxCollateralInputs: 1,
	}
	// Too many collateral inputs
	t.Run(
		"too many collateral inputs",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
					shelley.NewShelleyTransactionInput(testInputTxId, 1),
				},
				false,
			)
			err := conway.UtxoValidateTooManyCollateralInputs(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err == nil {
				t.Errorf(
					"UtxoValidateTooManyCollateralInputs should fail when too many collateral inputs are provided",
				)
				return
			}
			testErrType := babbage.TooManyCollateralInputsError{}
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
	// Single collateral input
	t.Run(
		"single collateral input",
		func(t *testing.T) {
			testTx.Body.TxCollateral = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
				},
				false,
			)
			err := conway.UtxoValidateTooManyCollateralInputs(
				testTx,
				testSlot,
				testLedgerState,
				testProtocolParams,
			)
			if err != nil {
				t.Errorf(
					"UtxoValidateTooManyCollateralInputs should succeed when the number of collateral inputs is under the limit\n  got error: %v",
					err,
				)
			}
		},
	)
}
