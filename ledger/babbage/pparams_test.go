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

package babbage_test

import (
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func TestBabbageProtocolParamsUpdate(t *testing.T) {
	testDefs := []struct {
		startParams    babbage.BabbageProtocolParameters
		updateCbor     string
		expectedParams babbage.BabbageProtocolParameters
	}{
		{
			startParams: babbage.BabbageProtocolParameters{
				ProtocolMajor: 7,
			},
			updateCbor: "a10e820800",
			expectedParams: babbage.BabbageProtocolParameters{
				ProtocolMajor: 8,
			},
		},
		{
			startParams: babbage.BabbageProtocolParameters{
				MaxBlockBodySize: 1,
				MaxTxExUnits: common.ExUnits{
					Memory: 1,
					Steps:  1,
				},
			},
			updateCbor: "a2021a0001200014821a00aba9501b00000002540be400",
			expectedParams: babbage.BabbageProtocolParameters{
				MaxBlockBodySize: 73728,
				MaxTxExUnits: common.ExUnits{
					Memory: 11250000,
					Steps:  10000000000,
				},
			},
		},
		{
			startParams: babbage.BabbageProtocolParameters{},
			updateCbor:  "a112a20098a61a0003236119032c01011903e819023b00011903e8195e7104011903e818201a0001ca761928eb041959d818641959d818641959d818641959d818641959d818641959d81864186418641959d81864194c5118201a0002acfa182019b551041a000363151901ff00011a00015c3518201a000797751936f404021a0002ff941a0006ea7818dc0001011903e8196ff604021a0003bd081a00034ec5183e011a00102e0f19312a011a00032e801901a5011a0002da781903e819cf06011a00013a34182019a8f118201903e818201a00013aac0119e143041903e80a1a00030219189c011a00030219189c011a0003207c1901d9011a000330001901ff0119ccf3182019fd40182019ffd5182019581e18201940b318201a00012adf18201a0002ff941a0006ea7818dc0001011a00010f92192da7000119eabb18201a0002ff941a0006ea7818dc0001011a0002ff941a0006ea7818dc0001011a000c504e197712041a001d6af61a0001425b041a00040c660004001a00014fab18201a0003236119032c010119a0de18201a00033d7618201979f41820197fb8182019a95d1820197df718201995aa18201a009063b91903fd0a0198af1a0003236119032c01011903e819023b00011903e8195e7104011903e818201a0001ca761928eb041959d818641959d818641959d818641959d818641959d818641959d81864186418641959d81864194c5118201a0002acfa182019b551041a000363151901ff00011a00015c3518201a000797751936f404021a0002ff941a0006ea7818dc0001011903e8196ff604021a0003bd081a00034ec5183e011a00102e0f19312a011a00032e801901a5011a0002da781903e819cf06011a00013a34182019a8f118201903e818201a00013aac0119e143041903e80a1a00030219189c011a00030219189c011a0003207c1901d9011a000330001901ff0119ccf3182019fd40182019ffd5182019581e18201940b318201a00012adf18201a0002ff941a0006ea7818dc0001011a00010f92192da7000119eabb18201a0002ff941a0006ea7818dc0001011a0002ff941a0006ea7818dc0001011a0011b22c1a0005fdde00021a000c504e197712041a001d6af61a0001425b041a00040c660004001a00014fab18201a0003236119032c010119a0de18201a00033d7618201979f41820197fb8182019a95d1820197df718201995aa18201b00000004a817c8001b00000004a817c8001a009063b91903fd0a1b00000004a817c800001b00000004a817c800",
			expectedParams: babbage.BabbageProtocolParameters{
				CostModels: map[uint][]int64{
					0: {
						0x32361,
						0x32c,
						0x1,
						0x1,
						0x3e8,
						0x23b,
						0x0,
						0x1,
						0x3e8,
						0x5e71,
						0x4,
						0x1,
						0x3e8,
						0x20,
						0x1ca76,
						0x28eb,
						0x4,
						0x59d8,
						0x64,
						0x59d8,
						0x64,
						0x59d8,
						0x64,
						0x59d8,
						0x64,
						0x59d8,
						0x64,
						0x59d8,
						0x64,
						0x64,
						0x64,
						0x59d8,
						0x64,
						0x4c51,
						0x20,
						0x2acfa,
						0x20,
						0xb551,
						0x4,
						0x36315,
						0x1ff,
						0x0,
						0x1,
						0x15c35,
						0x20,
						0x79775,
						0x36f4,
						0x4,
						0x2,
						0x2ff94,
						0x6ea78,
						0xdc,
						0x0,
						0x1,
						0x1,
						0x3e8,
						0x6ff6,
						0x4,
						0x2,
						0x3bd08,
						0x34ec5,
						0x3e,
						0x1,
						0x102e0f,
						0x312a,
						0x1,
						0x32e80,
						0x1a5,
						0x1,
						0x2da78,
						0x3e8,
						0xcf06,
						0x1,
						0x13a34,
						0x20,
						0xa8f1,
						0x20,
						0x3e8,
						0x20,
						0x13aac,
						0x1,
						0xe143,
						0x4,
						0x3e8,
						0xa,
						0x30219,
						0x9c,
						0x1,
						0x30219,
						0x9c,
						0x1,
						0x3207c,
						0x1d9,
						0x1,
						0x33000,
						0x1ff,
						0x1,
						0xccf3,
						0x20,
						0xfd40,
						0x20,
						0xffd5,
						0x20,
						0x581e,
						0x20,
						0x40b3,
						0x20,
						0x12adf,
						0x20,
						0x2ff94,
						0x6ea78,
						0xdc,
						0x0,
						0x1,
						0x1,
						0x10f92,
						0x2da7,
						0x0,
						0x1,
						0xeabb,
						0x20,
						0x2ff94,
						0x6ea78,
						0xdc,
						0x0,
						0x1,
						0x1,
						0x2ff94,
						0x6ea78,
						0xdc,
						0x0,
						0x1,
						0x1,
						0xc504e,
						0x7712,
						0x4,
						0x1d6af6,
						0x1425b,
						0x4,
						0x40c66,
						0x0,
						0x4,
						0x0,
						0x14fab,
						0x20,
						0x32361,
						0x32c,
						0x1,
						0x1,
						0xa0de,
						0x20,
						0x33d76,
						0x20,
						0x79f4,
						0x20,
						0x7fb8,
						0x20,
						0xa95d,
						0x20,
						0x7df7,
						0x20,
						0x95aa,
						0x20,
						0x9063b9,
						0x3fd,
						0xa,
					},
					1: {
						0x32361,
						0x32c,
						0x1,
						0x1,
						0x3e8,
						0x23b,
						0x0,
						0x1,
						0x3e8,
						0x5e71,
						0x4,
						0x1,
						0x3e8,
						0x20,
						0x1ca76,
						0x28eb,
						0x4,
						0x59d8,
						0x64,
						0x59d8,
						0x64,
						0x59d8,
						0x64,
						0x59d8,
						0x64,
						0x59d8,
						0x64,
						0x59d8,
						0x64,
						0x64,
						0x64,
						0x59d8,
						0x64,
						0x4c51,
						0x20,
						0x2acfa,
						0x20,
						0xb551,
						0x4,
						0x36315,
						0x1ff,
						0x0,
						0x1,
						0x15c35,
						0x20,
						0x79775,
						0x36f4,
						0x4,
						0x2,
						0x2ff94,
						0x6ea78,
						0xdc,
						0x0,
						0x1,
						0x1,
						0x3e8,
						0x6ff6,
						0x4,
						0x2,
						0x3bd08,
						0x34ec5,
						0x3e,
						0x1,
						0x102e0f,
						0x312a,
						0x1,
						0x32e80,
						0x1a5,
						0x1,
						0x2da78,
						0x3e8,
						0xcf06,
						0x1,
						0x13a34,
						0x20,
						0xa8f1,
						0x20,
						0x3e8,
						0x20,
						0x13aac,
						0x1,
						0xe143,
						0x4,
						0x3e8,
						0xa,
						0x30219,
						0x9c,
						0x1,
						0x30219,
						0x9c,
						0x1,
						0x3207c,
						0x1d9,
						0x1,
						0x33000,
						0x1ff,
						0x1,
						0xccf3,
						0x20,
						0xfd40,
						0x20,
						0xffd5,
						0x20,
						0x581e,
						0x20,
						0x40b3,
						0x20,
						0x12adf,
						0x20,
						0x2ff94,
						0x6ea78,
						0xdc,
						0x0,
						0x1,
						0x1,
						0x10f92,
						0x2da7,
						0x0,
						0x1,
						0xeabb,
						0x20,
						0x2ff94,
						0x6ea78,
						0xdc,
						0x0,
						0x1,
						0x1,
						0x2ff94,
						0x6ea78,
						0xdc,
						0x0,
						0x1,
						0x1,
						0x11b22c,
						0x5fdde,
						0x0,
						0x2,
						0xc504e,
						0x7712,
						0x4,
						0x1d6af6,
						0x1425b,
						0x4,
						0x40c66,
						0x0,
						0x4,
						0x0,
						0x14fab,
						0x20,
						0x32361,
						0x32c,
						0x1,
						0x1,
						0xa0de,
						0x20,
						0x33d76,
						0x20,
						0x79f4,
						0x20,
						0x7fb8,
						0x20,
						0xa95d,
						0x20,
						0x7df7,
						0x20,
						0x95aa,
						0x20,
						0x4a817c800,
						0x4a817c800,
						0x9063b9,
						0x3fd,
						0xa,
						0x4a817c800,
						0x0,
						0x4a817c800,
					},
				},
			},
		},
	}
	for _, testDef := range testDefs {
		cborBytes, err := hex.DecodeString(testDef.updateCbor)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var tmpUpdate babbage.BabbageProtocolParameterUpdate
		if _, err := cbor.Decode(cborBytes, &tmpUpdate); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpParams := testDef.startParams
		tmpParams.Update(&tmpUpdate)
		if !reflect.DeepEqual(tmpParams, testDef.expectedParams) {
			t.Fatalf(
				"did not get expected params:\n     got: %#v\n  wanted: %#v",
				tmpParams,
				testDef.expectedParams,
			)
		}
	}
}

func TestBabbageUtxorpc(t *testing.T) {
	testDefs := []struct {
		startParams     babbage.BabbageProtocolParameters
		expectedUtxorpc *cardano.PParams
	}{
		{
			startParams: babbage.BabbageProtocolParameters{
				AdaPerUtxoByte:       44,
				MaxTxSize:            16384,
				MinFeeA:              500,
				MinFeeB:              2,
				MaxBlockBodySize:     65536,
				MaxBlockHeaderSize:   1024,
				KeyDeposit:           2000,
				PoolDeposit:          500000,
				MaxEpoch:             2160,
				NOpt:                 100,
				A0:                   &cbor.Rat{Rat: big.NewRat(1, 2)},
				Rho:                  &cbor.Rat{Rat: big.NewRat(3, 4)},
				Tau:                  &cbor.Rat{Rat: big.NewRat(5, 6)},
				MinPoolCost:          340000000,
				ProtocolMajor:        8,
				ProtocolMinor:        0,
				MaxValueSize:         1024,
				CollateralPercentage: 150,
				MaxCollateralInputs:  5,
				ExecutionCosts: common.ExUnitPrice{
					MemPrice:  &cbor.Rat{Rat: big.NewRat(1, 2)},
					StepPrice: &cbor.Rat{Rat: big.NewRat(2, 3)},
				},
				MaxTxExUnits: common.ExUnits{
					Memory: 1000000,
					Steps:  200000,
				},
				MaxBlockExUnits: common.ExUnits{
					Memory: 5000000,
					Steps:  1000000,
				},
				CostModels: map[uint][]int64{
					1: {100, 200, 300},
					2: {400, 500, 600},
					3: {700, 800, 900},
				},
			},
			expectedUtxorpc: &cardano.PParams{
				CoinsPerUtxoByte:         44,
				MaxTxSize:                16384,
				MinFeeCoefficient:        500,
				MinFeeConstant:           2,
				MaxBlockBodySize:         65536,
				MaxBlockHeaderSize:       1024,
				StakeKeyDeposit:          2000,
				PoolDeposit:              500000,
				PoolRetirementEpochBound: 2160,
				DesiredNumberOfPools:     100,
				PoolInfluence: &cardano.RationalNumber{
					Numerator:   int32(1),
					Denominator: uint32(2),
				},
				MonetaryExpansion: &cardano.RationalNumber{
					Numerator:   int32(3),
					Denominator: uint32(4),
				},
				TreasuryExpansion: &cardano.RationalNumber{
					Numerator:   int32(5),
					Denominator: uint32(6),
				},
				MinPoolCost: 340000000,
				ProtocolVersion: &cardano.ProtocolVersion{
					Major: 8,
					Minor: 0,
				},
				MaxValueSize:         1024,
				CollateralPercentage: 150,
				MaxCollateralInputs:  5,
				CostModels: &cardano.CostModels{
					PlutusV1: &cardano.CostModel{
						Values: []int64{100, 200, 300},
					},
					PlutusV2: &cardano.CostModel{
						Values: []int64{400, 500, 600},
					},
					PlutusV3: &cardano.CostModel{
						Values: []int64{700, 800, 900},
					},
				},
				Prices: &cardano.ExPrices{
					Memory: &cardano.RationalNumber{
						Numerator:   int32(1),
						Denominator: uint32(2),
					},
					Steps: &cardano.RationalNumber{
						Numerator:   int32(2),
						Denominator: uint32(3),
					},
				},
				MaxExecutionUnitsPerTransaction: &cardano.ExUnits{
					Memory: 1000000,
					Steps:  200000,
				},
				MaxExecutionUnitsPerBlock: &cardano.ExUnits{
					Memory: 5000000,
					Steps:  1000000,
				},
			},
		},
	}

	for _, testDef := range testDefs {
		result, err := testDef.startParams.Utxorpc()
		if err != nil {
			t.Fatalf("Utxorpc() failed: %v", err)
		}
		if !reflect.DeepEqual(result, testDef.expectedUtxorpc) {
			t.Fatalf(
				"Utxorpc() test failed:\nExpected: %#v\nGot: %#v",
				testDef.expectedUtxorpc,
				result,
			)
		}
	}
}

// Unit test for BabbageTransactionInput.Utxorpc()
func TestBabbageTransactionInput_Utxorpc(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		2,
	)

	got, err := input.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() failed: %v", err)
	}
	want := &cardano.TxInput{
		TxHash:      input.Id().Bytes(),
		OutputIndex: input.Index(),
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf(
			"BabbageTransactionInput.Utxorpc() mismatch\\nGot: %+v\\nWant: %+v",
			got,
			want,
		)
	}
}

// Unit test for BabbageTransactionOutput.Utxorpc()
func TestBabbageTransactionOutput_Utxorpc(t *testing.T) {
	address := common.Address{}
	amount := uint64(8400)

	output := babbage.BabbageTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: amount},
	}

	got, err := output.Utxorpc()
	assert.NoError(t, err)
	addr, err := address.Bytes()
	assert.NoError(t, err)
	want := &cardano.TxOutput{
		Address: addr,
		Coin:    amount,
		Datum: &cardano.Datum{
			Hash: make([]byte, 32),
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf(
			"BabbageTransactionOutput.Utxorpc() mismatch\nGot: %+v\nWant: %+v",
			got,
			want,
		)
	}
}

// Unit test for BabbageTransactionBody.Utxorpc()
func TestBabbageTransactionBody_Utxorpc(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	address := common.Address{}
	output := babbage.BabbageTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 5000},
	}

	body := babbage.BabbageTransactionBody{
		TxInputs:  inputSet,
		TxOutputs: []babbage.BabbageTransactionOutput{output},
		TxFee:     100,
	}

	got, err := body.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() conversion failed: %v", err)
	}
	if got.Fee != 100 {
		t.Errorf("Fee mismatch: got %d, want 100", got.Fee)
	}
	if len(got.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(got.Inputs))
	}
	if len(got.Outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(got.Outputs))
	}
	if got.Outputs[0].Coin != 5000 {
		t.Errorf("Output coin mismatch: got %d, want 5000", got.Outputs[0].Coin)
	}
	if len(got.Hash) == 0 {
		t.Error("Expected non-empty transaction hash")
	}
}

// Unit test for BabbageTransaction.Utxorpc()
func TestBabbageTransaction_Utxorpc(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", 2)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	address := common.Address{}
	output := babbage.BabbageTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 9000},
	}

	body := babbage.BabbageTransactionBody{
		TxInputs:  inputSet,
		TxOutputs: []babbage.BabbageTransactionOutput{output},
		TxFee:     150,
	}
	tx := babbage.BabbageTransaction{
		Body:      body,
		TxIsValid: true,
	}

	got, err := tx.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() failed: %v", err)
	}
	if got.Fee != 150 {
		t.Errorf("Fee mismatch: got %d, want 150", got.Fee)
	}
	if len(got.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(got.Inputs))
	}
	if len(got.Outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(got.Outputs))
	}
	if got.Outputs[0].Coin != 9000 {
		t.Errorf("Output coin mismatch: got %d, want 9000", got.Outputs[0].Coin)
	}
	if len(got.Hash) == 0 {
		t.Error("Expected non-empty transaction hash")
	}
}
