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

package shelley_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shelleyTxWithRequired embeds a ShelleyTransaction but overrides RequiredSigners
// to allow testing witness rules which don't exist in base Shelley body.
type shelleyTxWithRequired struct {
	shelley.ShelleyTransaction
	req []common.Blake2b224
}

func (t shelleyTxWithRequired) RequiredSigners() []common.Blake2b224 { return t.req }

type rewardBalanceErrorLedgerState struct {
	common.LedgerState
	err error
}

func (s rewardBalanceErrorLedgerState) RewardAccountBalance(
	common.Credential,
) (*uint64, error) {
	return nil, s.err
}

func TestUtxoValidateWitnessRules_Shelley(t *testing.T) {
	t.Run("no required signers", func(t *testing.T) {
		tx := &shelley.ShelleyTransaction{}
		err := shelley.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("missing vkey witness", func(t *testing.T) {
		required := common.Blake2b224Hash([]byte{})
		tx := &shelleyTxWithRequired{req: []common.Blake2b224{required}}
		err := shelley.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for missing vkey witnesses")
		}
		assert.IsType(t, shelley.MissingVKeyWitnessesError{}, err)
	})

	t.Run("mismatched vkey", func(t *testing.T) {
		required := common.Blake2b224Hash([]byte{})
		tx := &shelleyTxWithRequired{req: []common.Blake2b224{required}}
		tx.WitnessSet.VkeyWitnesses = []common.VkeyWitness{
			{Vkey: []byte{0x01, 0x02, 0x03}},
		}
		err := shelley.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		if err == nil {
			t.Fatalf("expected error for mismatched vkey witness")
		}
		assert.IsType(
			t,
			shelley.MissingRequiredVKeyWitnessForSignerError{},
			err,
		)
	})

	t.Run("matching vkey", func(t *testing.T) {
		required := common.Blake2b224Hash([]byte{})
		tx := &shelleyTxWithRequired{req: []common.Blake2b224{required}}
		tx.WitnessSet.VkeyWitnesses = []common.VkeyWitness{{Vkey: []byte{}}}
		err := shelley.UtxoValidateRequiredVKeyWitnesses(tx, 0, nil, nil)
		assert.NoError(t, err)
	})
}

func TestUtxoValidateTimeToLive(t *testing.T) {
	var testSlot uint64 = 555666777
	var testZeroSlot uint64 = 0
	testTx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
			Ttl: testSlot,
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testProtocolParams := &shelley.ShelleyProtocolParameters{}
	var testBeforeSlot uint64 = 555666700
	var testAfterSlot uint64 = 555666799
	// Test helper function
	testRun := func(t *testing.T, name string, testTipSlot uint64, validateFunc func(*testing.T, error)) {
		t.Run(
			name,
			func(t *testing.T) {
				err := shelley.UtxoValidateTimeToLive(
					testTx,
					testTipSlot,
					testLedgerState,
					testProtocolParams,
				)
				validateFunc(t, err)
			},
		)
	}
	// Slot before TTL
	testRun(
		t,
		"slot before TTL",
		testBeforeSlot,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateTimeToLive should succeed when provided a tip slot (%d) before the specified TTL (%d)\n  got error: %v",
					testBeforeSlot,
					testTx.TTL(),
					err,
				)
			}
		},
	)
	// Slot equal to TTL
	testRun(
		t,
		"slot equal to TTL",
		testSlot,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateTimeToLive should succeed when provided a tip slot (%d) equal to the specified TTL (%d)\n  got error: %v",
					testSlot,
					testTx.TTL(),
					err,
				)
			}
		},
	)
	// Slot after TTL
	testRun(
		t,
		"slot after TTL",
		testAfterSlot,
		func(t *testing.T, err error) {
			if err == nil {
				t.Errorf(
					"UtxoValidateTimeToLive should fail when provided a tip slot (%d) after the specified TTL (%d)",
					testAfterSlot,
					testTx.TTL(),
				)
				return
			}
			testErrType := shelley.ExpiredUtxoError{}
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
	testTx.Body.Ttl = testZeroSlot
	testRun(
		t,
		"zero TTL",
		testZeroSlot,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateTimeToLive should succeed when provided a zero TTL\n  got error: %v",
					err,
				)
			}
		},
	)
}

func TestUtxoValidateInputSetEmptyUtxo(t *testing.T) {
	testTx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
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
	testProtocolParams := &shelley.ShelleyProtocolParameters{}
	// Non-empty
	t.Run(
		"non-empty input set",
		func(t *testing.T) {
			err := shelley.UtxoValidateInputSetEmptyUtxo(
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
			err := shelley.UtxoValidateInputSetEmptyUtxo(
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
	testTxCbor, _ := hex.DecodeString("abcdef")
	testTx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
			TxFee: 0, // Set to 0 to calculate minFee
		},
	}
	testTx.SetCbor(testTxCbor)
	testProtocolParams := &shelley.ShelleyProtocolParameters{
		MinFeeA: 7,
		MinFeeB: 53,
	}
	// Calculate minFee dynamically
	minFee, err := shelley.MinFeeTx(testTx, testProtocolParams)
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
				err := shelley.UtxoValidateFeeTooSmallUtxo(
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
	testTx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{},
	}
	utxos := []common.Utxo{{Id: testGoodInput}}
	testLedgerState := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
	testSlot := uint64(0)
	testProtocolParams := &shelley.ShelleyProtocolParameters{}
	// Good input
	t.Run(
		"good input",
		func(t *testing.T) {
			testTx.Body.TxInputs = shelley.NewShelleyTransactionInputSet(
				[]shelley.ShelleyTransactionInput{testGoodInput},
			)
			err := shelley.UtxoValidateBadInputsUtxo(
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
			err := shelley.UtxoValidateBadInputsUtxo(
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
	testTx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
			TxOutputs: []shelley.ShelleyTransactionOutput{
				{
					OutputAmount: 123456,
				},
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().WithNetworkId(common.AddressNetworkMainnet).Build()
	testSlot := uint64(0)
	testProtocolParams := &shelley.ShelleyProtocolParameters{}
	// Correct network
	t.Run(
		"correct network",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAddress = testCorrectNetworkAddr
			err := shelley.UtxoValidateBadInputsUtxo(
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
			err := shelley.UtxoValidateWrongNetwork(
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
	testTx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().WithNetworkId(common.AddressNetworkMainnet).Build()
	testSlot := uint64(0)
	testProtocolParams := &shelley.ShelleyProtocolParameters{}
	// Correct network
	t.Run(
		"correct network",
		func(t *testing.T) {
			testTx.Body.TxWithdrawals[&testCorrectNetworkAddr] = 123456
			err := shelley.UtxoValidateWrongNetworkWithdrawal(
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
			err := shelley.UtxoValidateWrongNetworkWithdrawal(
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
	testTx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
			TxFee: testFee,
			TxInputs: shelley.NewShelleyTransactionInputSet(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(testInputTxId, 0),
				},
			),
			TxOutputs: []shelley.ShelleyTransactionOutput{
				// Empty placeholder output
				{},
			},
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
	testProtocolParams := &shelley.ShelleyProtocolParameters{
		KeyDeposit: uint(testStakeDeposit),
	}
	// Exact amount
	t.Run(
		"exact amount",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount = testOutputExactAmount
			err := shelley.UtxoValidateValueNotConservedUtxo(
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
			testTx.Body.TxOutputs[0].OutputAmount = testOutputExactAmount - testStakeDeposit
			testTx.Body.TxCertificates = []common.CertificateWrapper{
				{
					Type: uint(common.CertificateTypeStakeRegistration),
					Certificate: &common.StakeRegistrationCertificate{
						StakeCredential: common.Credential{},
					},
				},
			}
			err := shelley.UtxoValidateValueNotConservedUtxo(
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
			testTx.Body.TxOutputs[0].OutputAmount = testOutputExactAmount + testStakeDeposit
			testTx.Body.TxCertificates = []common.CertificateWrapper{
				{
					Type: uint(common.CertificateTypeStakeDeregistration),
					Certificate: &common.StakeDeregistrationCertificate{
						StakeCredential: common.Credential{},
					},
				},
			}
			err := shelley.UtxoValidateValueNotConservedUtxo(
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
			testTx.Body.TxOutputs[0].OutputAmount = testOutputUnderAmount
			err := shelley.UtxoValidateValueNotConservedUtxo(
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
			testTx.Body.TxOutputs[0].OutputAmount = testOutputOverAmount
			err := shelley.UtxoValidateValueNotConservedUtxo(
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

// poolDepositEpochLedgerState adds the optional common.EpochState capability to
// a mock ledger state. PoolRegistrationDepositDue needs the current epoch to
// decide whether a pending retirement has already taken effect.
type poolDepositEpochLedgerState struct {
	common.LedgerState
	epoch uint64
}

func (s poolDepositEpochLedgerState) EpochForSlot(uint64) (uint64, error) {
	return s.epoch, nil
}

var _ common.EpochState = poolDepositEpochLedgerState{}

// shelleyValueNotConservedRule returns the value-not-conserved validator as it
// is actually registered for the era. Resolving it through the descriptors
// rather than naming the function keeps the test on the code path a consumer
// runs, so an unregistered or misidentified rule fails here.
func shelleyValueNotConservedRule(t *testing.T) common.UtxoValidationRuleFunc {
	t.Helper()
	descriptors := shelley.UtxoValidationRuleDescriptors()
	require.Len(t, shelley.UtxoValidationRules, len(descriptors))
	for idx, descriptor := range descriptors {
		if descriptor.Id == common.UtxoValidationRuleValueNotConserved {
			return shelley.UtxoValidationRules[idx]
		}
	}
	t.Fatalf(
		"Shelley validation rule %q is not registered",
		common.UtxoValidationRuleValueNotConserved,
	)
	return nil
}

// TestUtxoValidateValueNotConservedUtxoPoolDeposits drives the registered
// value-not-conserved rule and pins how far the produced value moves for pool
// registration certificates.
//
// The unit tests in ledger/common prove the PoolRegistrationDepositDue
// predicate. These cases prove what the rule does with it. Each case budgets
// the expected number of deposits and requires conservation, then budgets one
// deposit more (and, where a deposit is expected, one less) and requires the
// consumed/produced gap to be exactly one deposit. That bounds the rule from
// both sides: a re-registration after an effective retirement moves the
// produced value by exactly one deposit, and a transaction carrying two
// registration certificates for the same pool is charged exactly once.
//
// The per-transaction dedup is what the two-certificate cases exercise. POOL
// applies certificates in order, and cardano-ledger's
// shelleyTotalDepositsTxBody counts a Set of not-yet-registered pool ids, so a
// second certificate for a pool the first one just registered incurs nothing.
func TestUtxoValidateValueNotConservedUtxoPoolDeposits(t *testing.T) {
	const (
		inputAmount     uint64 = 3_000_000_000
		fee             uint64 = 123_456
		poolDeposit     uint64 = 500_000_000
		retirementEpoch uint64 = 197
	)
	inputTxId := "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	exactAmount := inputAmount - fee
	operator := common.PoolKeyHash(
		common.NewBlake2b224(
			bytes.Repeat([]byte{0x42}, common.Blake2b224Size),
		),
	)
	onRecord := &common.PoolRegistrationCertificate{Operator: operator}
	utxos := []common.Utxo{
		{
			Id: shelley.NewShelleyTransactionInput(inputTxId, 0),
			Output: shelley.ShelleyTransactionOutput{
				OutputAmount: inputAmount,
			},
		},
	}
	pparams := &shelley.ShelleyProtocolParameters{
		PoolDeposit: uint(poolDeposit),
	}

	// newTx budgets budgetedDeposits pool deposits out of the outputs and
	// carries the given number of registration certificates, all for the same
	// operator.
	newTx := func(
		budgetedDeposits uint64,
		registrations int,
	) *shelley.ShelleyTransaction {
		certs := make([]common.CertificateWrapper, 0, registrations)
		for range registrations {
			certs = append(certs, common.CertificateWrapper{
				Type: uint(common.CertificateTypePoolRegistration),
				Certificate: &common.PoolRegistrationCertificate{
					Operator: operator,
				},
			})
		}
		return &shelley.ShelleyTransaction{
			Body: shelley.ShelleyTransactionBody{
				TxFee: fee,
				TxInputs: shelley.NewShelleyTransactionInputSet(
					[]shelley.ShelleyTransactionInput{
						shelley.NewShelleyTransactionInput(inputTxId, 0),
					},
				),
				TxOutputs: []shelley.ShelleyTransactionOutput{
					{
						OutputAmount: exactAmount - budgetedDeposits*poolDeposit,
					},
				},
				TxCertificates: certs,
			},
		}
	}
	newLedgerState := func(
		reg *common.PoolRegistrationCertificate,
		retirement *uint64,
	) common.LedgerState {
		return poolDepositEpochLedgerState{
			LedgerState: mockledger.NewLedgerStateBuilder().
				WithUtxos(utxos).
				WithPoolCurrentState(
					func(common.PoolKeyHash) (*common.PoolRegistrationCertificate, *uint64, error) {
						return reg, retirement, nil
					},
				).Build(),
			epoch: retirementEpoch,
		}
	}
	retiresAt := func(epoch uint64) *uint64 { return &epoch }

	for _, tc := range []struct {
		name string
		// reg and retire are what PoolCurrentState reports for the operator.
		reg    *common.PoolRegistrationCertificate
		retire *uint64
		// registrations is how many registration certificates for that
		// operator the transaction carries.
		registrations int
		// wantDeposits is how many pool deposits the rule must add to the
		// produced value.
		wantDeposits uint64
	}{
		{
			name:          "unregistered pool",
			registrations: 1,
			wantDeposits:  1,
		},
		{
			name:          "live pool is a parameter update",
			reg:           onRecord,
			registrations: 1,
			wantDeposits:  0,
		},
		{
			name:          "retirement still pending is a parameter update",
			reg:           onRecord,
			retire:        retiresAt(retirementEpoch + 1),
			registrations: 1,
			wantDeposits:  0,
		},
		{
			name:          "retirement has taken effect",
			reg:           onRecord,
			retire:        retiresAt(retirementEpoch),
			registrations: 1,
			wantDeposits:  1,
		},
		{
			name:          "two registrations for an unregistered pool",
			registrations: 2,
			wantDeposits:  1,
		},
		{
			name:          "two registrations for a retired pool",
			reg:           onRecord,
			retire:        retiresAt(retirementEpoch),
			registrations: 2,
			wantDeposits:  1,
		},
		{
			name:          "two registrations for a live pool",
			reg:           onRecord,
			registrations: 2,
			wantDeposits:  0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := shelleyValueNotConservedRule(t)
			ls := newLedgerState(tc.reg, tc.retire)
			// Budgeting exactly the expected deposits must conserve value.
			require.NoError(
				t,
				rule(newTx(tc.wantDeposits, tc.registrations), 0, ls, pparams),
				"budgeting %d pool deposit(s) should conserve value",
				tc.wantDeposits,
			)
			// Budgeting one deposit either side must miss by exactly one
			// deposit, which bounds the rule above and below.
			budgets := []uint64{tc.wantDeposits + 1}
			if tc.wantDeposits > 0 {
				budgets = append(budgets, tc.wantDeposits-1)
			}
			for _, budget := range budgets {
				tx := newTx(budget, tc.registrations)
				err := rule(tx, 0, ls, pparams)
				var notConserved shelley.ValueNotConservedUtxoError
				require.ErrorAs(
					t,
					err,
					&notConserved,
					"budgeting %d pool deposit(s) should not conserve value",
					budget,
				)
				// Consumed minus produced isolates the deposit
				// difference. The budget is one deposit either side of
				// wantDeposits, so the gap is one deposit, signed by which
				// side it is on.
				want := new(big.Int).SetUint64(poolDeposit)
				if budget < tc.wantDeposits {
					want.Neg(want)
				}
				gap := new(big.Int).Sub(
					notConserved.Consumed,
					notConserved.Produced,
				)
				require.Equal(
					t,
					want.String(),
					gap.String(),
					"budgeting %d pool deposit(s): the rule added the wrong number of deposits",
					budget,
				)
			}
		})
	}
}

func TestUtxoValidateOutputTooSmallUtxo(t *testing.T) {
	var testOutputAmountGood uint64 = 1234567
	var testOutputAmountBad uint64 = 123
	testTx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
			TxOutputs: []shelley.ShelleyTransactionOutput{
				// Empty placeholder output
				{},
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testSlot := uint64(0)
	testProtocolParams := &shelley.ShelleyProtocolParameters{
		MinUtxoValue: 100000,
	}
	// Good
	t.Run(
		"sufficient coin",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAmount = testOutputAmountGood
			err := shelley.UtxoValidateOutputTooSmallUtxo(
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
			testTx.Body.TxOutputs[0].OutputAmount = testOutputAmountBad
			err := shelley.UtxoValidateOutputTooSmallUtxo(
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
	testTx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
			TxOutputs: []shelley.ShelleyTransactionOutput{
				// Empty placeholder
				{},
			},
		},
	}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testSlot := uint64(0)
	testProtocolParams := &shelley.ShelleyProtocolParameters{}
	// Good
	t.Run(
		"Shelley address",
		func(t *testing.T) {
			testTx.Body.TxOutputs[0].OutputAddress = testGoodAddr
			err := shelley.UtxoValidateOutputBootAddrAttrsTooBig(
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
			err := shelley.UtxoValidateOutputBootAddrAttrsTooBig(
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

func TestUtxoValidateNoDuplicateInputs(t *testing.T) {
	input1, err := mockledger.NewSimpleTransactionInput(
		bytes.Repeat([]byte{0x01}, 32),
		0,
	)
	require.NoError(t, err)
	input2, err := mockledger.NewSimpleTransactionInput(
		bytes.Repeat([]byte{0x02}, 32),
		1,
	)
	require.NoError(t, err)

	t.Run("unique inputs pass", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		tx.WithInputs(input1, input2)
		err := shelley.UtxoValidateNoDuplicateInputs(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("duplicate regular inputs fail", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		tx.WithInputs(input1, input1)
		err := shelley.UtxoValidateNoDuplicateInputs(tx, 0, nil, nil)
		assert.Error(t, err)
		assert.IsType(t, shelley.DuplicateInputError{}, err)
	})

	t.Run("empty inputs pass", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		err := shelley.UtxoValidateNoDuplicateInputs(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("single input passes", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		tx.WithInputs(input1)
		err := shelley.UtxoValidateNoDuplicateInputs(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("duplicate collateral inputs fail", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		tx.WithCollateral(input1, input1)
		err := shelley.UtxoValidateNoDuplicateInputs(tx, 0, nil, nil)
		assert.Error(t, err)
		assert.IsType(t, shelley.DuplicateInputError{}, err)
	})

	t.Run("unique collateral inputs pass", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		tx.WithCollateral(input1, input2)
		err := shelley.UtxoValidateNoDuplicateInputs(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("duplicate reference inputs fail", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		tx.WithReferenceInputs(input1, input1)
		err := shelley.UtxoValidateNoDuplicateInputs(tx, 0, nil, nil)
		assert.Error(t, err)
		assert.IsType(t, shelley.DuplicateInputError{}, err)
	})

	t.Run("unique reference inputs pass", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		tx.WithReferenceInputs(input1, input2)
		err := shelley.UtxoValidateNoDuplicateInputs(tx, 0, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("same input in regular and collateral passes", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		tx.WithInputs(input1)
		tx.WithCollateral(input1)
		err := shelley.UtxoValidateNoDuplicateInputs(tx, 0, nil, nil)
		assert.NoError(t, err)
	})
}

func TestUtxoValidateMaxTxSizeUtxo(t *testing.T) {
	var testMaxTxSizeSmall uint = 2
	var testMaxTxSizeLarge uint = 64 * 1024
	testTx := &shelley.ShelleyTransaction{}
	testLedgerState := mockledger.NewLedgerStateBuilder().Build()
	testSlot := uint64(0)
	testProtocolParams := &shelley.ShelleyProtocolParameters{}
	// Transaction under limit
	t.Run(
		"transaction is under limit",
		func(t *testing.T) {
			testProtocolParams.MaxTxSize = testMaxTxSizeLarge
			err := shelley.UtxoValidateMaxTxSizeUtxo(
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
			err := shelley.UtxoValidateMaxTxSizeUtxo(
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

// makeRewardAddress builds a reward address (type 14 / NoneKey) from a 28-byte key hash
func makeRewardAddress(t *testing.T, keyHash common.Blake2b224) common.Address {
	t.Helper()
	// Reward address: header = (AddressTypeNoneKey << 4) | networkId
	// AddressTypeNoneKey = 0b1110 = 14, mainnet networkId = 1
	addrBytes := make([]byte, 0, 29)
	addrBytes = append(addrBytes, 0xE1) // (14 << 4) | 1
	addrBytes = append(addrBytes, keyHash.Bytes()...)
	addr, err := common.NewAddressFromBytes(addrBytes)
	require.NoError(t, err)
	return addr
}

func TestUtxoValidateWithdrawals(t *testing.T) {
	t.Run("no withdrawals passes", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder()
		ls := mockledger.NewLedgerStateBuilder().Build()
		err := shelley.UtxoValidateWithdrawals(tx, 0, ls, nil)
		assert.NoError(t, err)
	})

	keyHash := common.Blake2b224Hash([]byte("test-stake-key"))
	rewardAddr := makeRewardAddress(t, keyHash)

	for _, tc := range []struct {
		name       string
		balance    uint64
		withdrawal uint64
		wantError  bool
	}{
		{
			name:       "exact reward balance passes",
			balance:    5_000_000,
			withdrawal: 5_000_000,
		},
		{
			name:       "zero balance withdrawal passes",
			balance:    0,
			withdrawal: 0,
		},
		{
			name:       "partial reward balance fails",
			balance:    5_000_000,
			withdrawal: 4_000_000,
			wantError:  true,
		},
		{
			name:       "excessive reward balance fails",
			balance:    5_000_000,
			withdrawal: 6_000_000,
			wantError:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := mockledger.NewTransactionBuilder().
				WithWithdrawals(map[*common.Address]uint64{
					&rewardAddr: tc.withdrawal,
				})
			ls := mockledger.NewLedgerStateBuilder().
				WithRewardAccountBalance(keyHash, tc.balance).
				Build()

			err := shelley.UtxoValidateWithdrawals(tx, 0, ls, nil)
			if !tc.wantError {
				require.NoError(t, err)
				return
			}
			var target shelley.IncorrectWithdrawalAmountError
			require.ErrorAs(t, err, &target)
			assert.Equal(t, rewardAddr, target.RewardAddress)
			assert.Equal(t, tc.balance, target.Balance)
			require.NotNil(t, target.Provided)
			assert.Equal(t, tc.withdrawal, target.Provided.Uint64())
		})
	}

	t.Run("unregistered reward account fails", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder().
			WithWithdrawals(map[*common.Address]uint64{
				&rewardAddr: 1_000_000,
			})

		ls := mockledger.NewLedgerStateBuilder().Build()

		err := shelley.UtxoValidateWithdrawals(tx, 0, ls, nil)
		require.Error(t, err)
		assert.IsType(t, shelley.WithdrawalFromUnregisteredRewardAccountError{}, err)
	})

	t.Run("balance lookup error is propagated", func(t *testing.T) {
		tx := mockledger.NewTransactionBuilder().
			WithWithdrawals(map[*common.Address]uint64{
				&rewardAddr: 1_000_000,
			})
		baseState := mockledger.NewLedgerStateBuilder().
			WithRewardAccountBalance(keyHash, 1_000_000).
			Build()
		lookupErr := errors.New("reward balance lookup failed")
		ls := rewardBalanceErrorLedgerState{
			LedgerState: baseState,
			err:         lookupErr,
		}

		err := shelley.UtxoValidateWithdrawals(tx, 0, ls, nil)
		require.ErrorIs(t, err, lookupErr)
	})
}
