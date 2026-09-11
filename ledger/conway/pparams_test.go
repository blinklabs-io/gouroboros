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
	"encoding/hex"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func testPlutusInteger(v int64) data.PlutusData {
	return data.NewInteger(big.NewInt(v))
}

func TestConwayProtocolParamsUpdate(t *testing.T) {
	testDefs := []struct {
		startParams    conway.ConwayProtocolParameters
		updateCbor     string
		expectedParams conway.ConwayProtocolParameters
	}{
		{
			startParams: conway.ConwayProtocolParameters{
				ProtocolVersion: common.ProtocolParametersProtocolVersion{
					Major: 8,
				},
			},
			updateCbor: "a10e820900",
			expectedParams: conway.ConwayProtocolParameters{
				ProtocolVersion: common.ProtocolParametersProtocolVersion{
					Major: 9,
				},
			},
		},
		{
			startParams: conway.ConwayProtocolParameters{
				MaxBlockBodySize: 1,
				MaxTxExUnits: common.ExUnits{
					Memory: 1,
					Steps:  1,
				},
			},
			updateCbor: "a2021a0001200014821a00aba9501b00000002540be400",
			expectedParams: conway.ConwayProtocolParameters{
				MaxBlockBodySize: 73728,
				MaxTxExUnits: common.ExUnits{
					Memory: 11250000,
					Steps:  10000000000,
				},
			},
		},
		{
			startParams: conway.ConwayProtocolParameters{},
			updateCbor:  "a112a20098a61a0003236119032c01011903e819023b00011903e8195e7104011903e818201a0001ca761928eb041959d818641959d818641959d818641959d818641959d818641959d81864186418641959d81864194c5118201a0002acfa182019b551041a000363151901ff00011a00015c3518201a000797751936f404021a0002ff941a0006ea7818dc0001011903e8196ff604021a0003bd081a00034ec5183e011a00102e0f19312a011a00032e801901a5011a0002da781903e819cf06011a00013a34182019a8f118201903e818201a00013aac0119e143041903e80a1a00030219189c011a00030219189c011a0003207c1901d9011a000330001901ff0119ccf3182019fd40182019ffd5182019581e18201940b318201a00012adf18201a0002ff941a0006ea7818dc0001011a00010f92192da7000119eabb18201a0002ff941a0006ea7818dc0001011a0002ff941a0006ea7818dc0001011a000c504e197712041a001d6af61a0001425b041a00040c660004001a00014fab18201a0003236119032c010119a0de18201a00033d7618201979f41820197fb8182019a95d1820197df718201995aa18201a009063b91903fd0a0198af1a0003236119032c01011903e819023b00011903e8195e7104011903e818201a0001ca761928eb041959d818641959d818641959d818641959d818641959d818641959d81864186418641959d81864194c5118201a0002acfa182019b551041a000363151901ff00011a00015c3518201a000797751936f404021a0002ff941a0006ea7818dc0001011903e8196ff604021a0003bd081a00034ec5183e011a00102e0f19312a011a00032e801901a5011a0002da781903e819cf06011a00013a34182019a8f118201903e818201a00013aac0119e143041903e80a1a00030219189c011a00030219189c011a0003207c1901d9011a000330001901ff0119ccf3182019fd40182019ffd5182019581e18201940b318201a00012adf18201a0002ff941a0006ea7818dc0001011a00010f92192da7000119eabb18201a0002ff941a0006ea7818dc0001011a0002ff941a0006ea7818dc0001011a0011b22c1a0005fdde00021a000c504e197712041a001d6af61a0001425b041a00040c660004001a00014fab18201a0003236119032c010119a0de18201a00033d7618201979f41820197fb8182019a95d1820197df718201995aa18201b00000004a817c8001b00000004a817c8001a009063b91903fd0a1b00000004a817c800001b00000004a817c800",
			expectedParams: conway.ConwayProtocolParameters{
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
		var tmpUpdate conway.ConwayProtocolParameterUpdate
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

// TestConwayProtocolParameterUpdate_CostModelLengthForwardCompat asserts that
// a ConwayProtocolParameterUpdate whose Plutus cost-model arrays are longer
// than today's PV10 baselines round-trips through CBOR without truncation.
// PV11 (vanRossem) and later hard forks may extend cost models with new
// builtins; gouroboros must accept the longer arrays.
func TestConwayProtocolParameterUpdate_CostModelLengthForwardCompat(
	t *testing.T,
) {
	const v1Len, v2Len, v3Len = 200, 220, 350
	v1 := make([]int64, v1Len)
	v2 := make([]int64, v2Len)
	v3 := make([]int64, v3Len)
	for i := range v1 {
		v1[i] = int64(i + 1)
	}
	for i := range v2 {
		v2[i] = int64(1000 + i)
	}
	for i := range v3 {
		v3[i] = int64(1_000_000 - i)
	}

	upd := conway.ConwayProtocolParameterUpdate{
		CostModels: map[uint][]int64{0: v1, 1: v2, 2: v3},
	}

	cborData, err := cbor.Encode(upd)
	assert.NoError(t, err)
	assert.NotEmpty(t, cborData)

	var decoded conway.ConwayProtocolParameterUpdate
	_, err = cbor.Decode(cborData, &decoded)
	assert.NoError(t, err)

	assert.Len(t, decoded.CostModels[0], v1Len)
	assert.Len(t, decoded.CostModels[1], v2Len)
	assert.Len(t, decoded.CostModels[2], v3Len)
	assert.Equal(t, v1, decoded.CostModels[0])
	assert.Equal(t, v2, decoded.CostModels[1])
	assert.Equal(t, v3, decoded.CostModels[2])
}

// TestConwayProtocolParameters_UpdateAcceptsLongerCostModel asserts that
// merging a ConwayProtocolParameterUpdate over existing protocol parameters
// preserves the entire incoming cost-model slice, even when it is longer than
// the existing one. Guards against silent truncation when PV11+ extends the
// PlutusV3 cost model.
func TestConwayProtocolParameters_UpdateAcceptsLongerCostModel(t *testing.T) {
	const newV3Len = 350
	newV3 := make([]int64, newV3Len)
	for i := range newV3 {
		newV3[i] = int64(i)
	}

	base := &conway.ConwayProtocolParameters{
		CostModels: map[uint][]int64{2: {1, 2, 3}},
	}
	upd := &conway.ConwayProtocolParameterUpdate{
		CostModels: map[uint][]int64{2: newV3},
	}

	base.Update(upd)

	assert.Len(t, base.CostModels[2], newV3Len)
	assert.Equal(t, newV3, base.CostModels[2])
}

// TestConwayUpdateFromGenesis_PlutusV3CostModelLengthForwardCompat asserts
// that loading a ConwayGenesis whose PlutusV3CostModel is longer than today's
// PV10 baseline copies the full slice into protocol parameters. Cardano
// genesis files for PV11+ are expected to ship extended PlutusV3 cost models.
func TestConwayUpdateFromGenesis_PlutusV3CostModelLengthForwardCompat(
	t *testing.T,
) {
	const v3Len = 350
	v3 := make([]int64, v3Len)
	for i := range v3 {
		v3[i] = int64(2_000_000 + i)
	}
	g := &conway.ConwayGenesis{PlutusV3CostModel: v3}

	p := &conway.ConwayProtocolParameters{}
	if err := p.UpdateFromGenesis(g); err != nil {
		t.Fatalf("UpdateFromGenesis: %v", err)
	}

	assert.Len(t, p.CostModels[2], v3Len)
	assert.Equal(t, v3, p.CostModels[2])
}

func TestConwayProtocolParameters_UpdateCopiesFields(t *testing.T) {
	base := &conway.ConwayProtocolParameters{}
	upd := &conway.ConwayProtocolParameterUpdate{}
	a := uint(11)
	mt := uint(2222)
	ada := uint64(7777)
	upd.MinFeeA = &a
	upd.MaxTxSize = &mt
	upd.AdaPerUtxoByte = &ada
	upd.CostModels = map[uint][]int64{0: {9}, 2: {8, 7}}

	base.Update(upd)
	if base.MinFeeA != 11 {
		t.Fatalf("MinFeeA not updated: got %d", base.MinFeeA)
	}
	if base.MaxTxSize != 2222 {
		t.Fatalf("MaxTxSize not updated: got %d", base.MaxTxSize)
	}
	if base.AdaPerUtxoByte != 7777 {
		t.Fatalf("AdaPerUtxoByte not updated: got %d", base.AdaPerUtxoByte)
	}
	if !reflect.DeepEqual(base.CostModels, upd.CostModels) {
		t.Fatalf(
			"CostModels not updated: got %+v want %+v",
			base.CostModels,
			upd.CostModels,
		)
	}
}

func TestConwayProtocolParamsUpdateFromGenesis(t *testing.T) {
	testDefs := []struct {
		startParams    conway.ConwayProtocolParameters
		genesisJson    string
		expectedParams conway.ConwayProtocolParameters
	}{
		{
			startParams: conway.ConwayProtocolParameters{
				MaxBlockBodySize: 1,
			},
			genesisJson: `{"govActionDeposit": 100000000000}`,
			expectedParams: conway.ConwayProtocolParameters{
				MaxBlockBodySize: 1,
				GovActionDeposit: 100000000000,
			},
		},
	}
	for _, testDef := range testDefs {
		tmpGenesis, err := conway.NewConwayGenesisFromReader(
			strings.NewReader(testDef.genesisJson),
		)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpParams := testDef.startParams
		if err := tmpParams.UpdateFromGenesis(&tmpGenesis); err != nil {
			t.Fatalf("unexpected error updating pparams from genesis: %s", err)
		}
		if !reflect.DeepEqual(tmpParams, testDef.expectedParams) {
			t.Fatalf(
				"did not get expected params:\n     got: %#v\n  wanted: %#v",
				tmpParams,
				testDef.expectedParams,
			)
		}
	}
}
func TestUtxorpc(t *testing.T) {
	// Define test cases
	testDefs := []struct {
		startParams     conway.ConwayProtocolParameters
		expectedUtxorpc *utxorpc.PParams
	}{
		{
			startParams: conway.ConwayProtocolParameters{
				AdaPerUtxoByte:     44,
				MaxTxSize:          16384,
				MinFeeA:            500,
				MinFeeB:            2,
				MaxBlockBodySize:   65536,
				MaxBlockHeaderSize: 1024,
				KeyDeposit:         2000,
				PoolDeposit:        500000,
				MaxEpoch:           2160,
				NOpt:               100,
				A0:                 &cbor.Rat{Rat: big.NewRat(1, 2)},
				Rho:                &cbor.Rat{Rat: big.NewRat(3, 4)},
				Tau:                &cbor.Rat{Rat: big.NewRat(5, 6)},
				MinPoolCost:        340000000,
				ProtocolVersion: common.ProtocolParametersProtocolVersion{
					Major: 8,
					Minor: 0,
				},
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
				// Keys follow the real cardano-ledger wire convention:
				// 0-indexed (0=PlutusV1, 1=PlutusV2, 2=PlutusV3).
				CostModels: map[uint][]int64{
					0: {100, 200, 300},
					1: {400, 500, 600},
					2: {700, 800, 900},
				},
			},
			expectedUtxorpc: &utxorpc.PParams{
				CoinsPerUtxoByte: &utxorpc.BigInt{
					BigInt: &utxorpc.BigInt_Int{Int: 44},
				},
				MaxTxSize: 16384,
				MinFeeCoefficient: &utxorpc.BigInt{
					BigInt: &utxorpc.BigInt_Int{Int: 500},
				},
				MinFeeConstant: &utxorpc.BigInt{
					BigInt: &utxorpc.BigInt_Int{Int: 2},
				},
				MaxBlockBodySize:   65536,
				MaxBlockHeaderSize: 1024,
				StakeKeyDeposit: &utxorpc.BigInt{
					BigInt: &utxorpc.BigInt_Int{Int: 2000},
				},
				PoolDeposit: &utxorpc.BigInt{
					BigInt: &utxorpc.BigInt_Int{Int: 500000},
				},
				PoolRetirementEpochBound: 2160,
				DesiredNumberOfPools:     100,
				PoolInfluence: &utxorpc.RationalNumber{
					Numerator:   int32(1),
					Denominator: uint32(2),
				},
				MonetaryExpansion: &utxorpc.RationalNumber{
					Numerator:   int32(3),
					Denominator: uint32(4),
				},
				TreasuryExpansion: &utxorpc.RationalNumber{
					Numerator:   int32(5),
					Denominator: uint32(6),
				},
				MinPoolCost: &utxorpc.BigInt{
					BigInt: &utxorpc.BigInt_Int{Int: 340000000},
				},
				ProtocolVersion: &utxorpc.ProtocolVersion{
					Major: 8,
					Minor: 0,
				},
				MaxValueSize:         1024,
				CollateralPercentage: 150,
				MaxCollateralInputs:  5,
				CostModels: &utxorpc.CostModels{
					PlutusV1: &utxorpc.CostModel{
						Values: []int64{100, 200, 300},
					},
					PlutusV2: &utxorpc.CostModel{
						Values: []int64{400, 500, 600},
					},
					PlutusV3: &utxorpc.CostModel{
						Values: []int64{700, 800, 900},
					},
				},
				Prices: &utxorpc.ExPrices{
					Memory: &utxorpc.RationalNumber{
						Numerator:   int32(1),
						Denominator: uint32(2),
					},
					Steps: &utxorpc.RationalNumber{
						Numerator:   int32(2),
						Denominator: uint32(3),
					},
				},
				MaxExecutionUnitsPerTransaction: &utxorpc.ExUnits{
					Memory: 1000000,
					Steps:  200000,
				},
				MaxExecutionUnitsPerBlock: &utxorpc.ExUnits{
					Memory: 5000000,
					Steps:  1000000,
				},
				// GovActionDeposit/DRepDeposit are plain uint64 fields (not
				// pointers) on ConwayProtocolParameters, so
				// common.ToUtxorpcBigInt always returns a non-nil BigInt,
				// even for the zero value left unset by this test case.
				GovernanceActionDeposit: &utxorpc.BigInt{
					BigInt: &utxorpc.BigInt_Int{Int: 0},
				},
				DrepDeposit: &utxorpc.BigInt{
					BigInt: &utxorpc.BigInt_Int{Int: 0},
				},
			},
		},
	}

	for _, testDef := range testDefs {
		result, err := testDef.startParams.Utxorpc()
		if err != nil {
			t.Fatalf("Utxorpc() conversion failed")
		}
		// Compare the result with the expected value
		if !reflect.DeepEqual(result, testDef.expectedUtxorpc) {
			t.Fatalf(
				"Utxorpc() test failed:\nExpected: %#v\nGot: %#v",
				testDef.expectedUtxorpc,
				result,
			)
		}
	}
}

// TestConwayUtxorpc_GovernanceFields is the regression test for the
// node-parity audit finding that Utxorpc() silently omitted every Conway
// governance parameter: GovActionDeposit, DRepDeposit, voting thresholds,
// committee settings, governance-action validity period, and the
// reference-script fee rational. Before this fix, changing e.g.
// GovActionDeposit produced an identical, empty node-parity diff because
// none of these fields ever reached the returned *utxorpc.PParams. Each
// field below is given a distinct value (and, for the voting-threshold
// lists, distinct per-position values) so a mapping mistake -- a dropped
// field or a misordered threshold list -- makes this test fail rather than
// passing by coincidence.
func TestConwayUtxorpc_GovernanceFields(t *testing.T) {
	rat := func(num, den int64) cbor.Rat {
		return cbor.Rat{Rat: big.NewRat(num, den)}
	}
	params := conway.ConwayProtocolParameters{
		// Fields required by Utxorpc()'s existing sanity checks.
		A0:  &cbor.Rat{Rat: big.NewRat(1, 2)},
		Rho: &cbor.Rat{Rat: big.NewRat(3, 4)},
		Tau: &cbor.Rat{Rat: big.NewRat(5, 6)},
		ExecutionCosts: common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(1, 2)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(2, 3)},
		},
		// Newly-mapped governance fields under test, each a distinct value.
		MinFeeRefScriptCostPerByte: &cbor.Rat{Rat: big.NewRat(15, 2)},
		PoolVotingThresholds: conway.PoolVotingThresholds{
			MotionNoConfidence:    rat(1, 11),
			CommitteeNormal:       rat(2, 11),
			CommitteeNoConfidence: rat(3, 11),
			HardForkInitiation:    rat(4, 11),
			PpSecurityGroup:       rat(5, 11),
		},
		DRepVotingThresholds: conway.DRepVotingThresholds{
			MotionNoConfidence:    rat(1, 13),
			CommitteeNormal:       rat(2, 13),
			CommitteeNoConfidence: rat(3, 13),
			UpdateToConstitution:  rat(4, 13),
			HardForkInitiation:    rat(5, 13),
			PpNetworkGroup:        rat(6, 13),
			PpEconomicGroup:       rat(7, 13),
			PpTechnicalGroup:      rat(8, 13),
			PpGovGroup:            rat(9, 13),
			TreasuryWithdrawal:    rat(10, 13),
		},
		MinCommitteeSize:        7,
		CommitteeTermLimit:      365,
		GovActionValidityPeriod: 30000,
		// Matches the exact scenario from the node-parity audit: a changed
		// GovActionDeposit must produce a different Utxorpc() value.
		GovActionDeposit:     100000000001,
		DRepDeposit:          500000000,
		DRepInactivityPeriod: 20,
	}

	result, err := params.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() conversion failed: %v", err)
	}

	assert.Equal(t,
		&utxorpc.RationalNumber{Numerator: 15, Denominator: 2},
		result.MinFeeScriptRefCostPerByte,
		"MinFeeScriptRefCostPerByte",
	)
	assert.Equal(t,
		&utxorpc.VotingThresholds{
			Thresholds: []*utxorpc.RationalNumber{
				{Numerator: 1, Denominator: 11},
				{Numerator: 2, Denominator: 11},
				{Numerator: 3, Denominator: 11},
				{Numerator: 4, Denominator: 11},
				{Numerator: 5, Denominator: 11},
			},
		},
		result.PoolVotingThresholds,
		"PoolVotingThresholds",
	)
	assert.Equal(t,
		&utxorpc.VotingThresholds{
			Thresholds: []*utxorpc.RationalNumber{
				{Numerator: 1, Denominator: 13},
				{Numerator: 2, Denominator: 13},
				{Numerator: 3, Denominator: 13},
				{Numerator: 4, Denominator: 13},
				{Numerator: 5, Denominator: 13},
				{Numerator: 6, Denominator: 13},
				{Numerator: 7, Denominator: 13},
				{Numerator: 8, Denominator: 13},
				{Numerator: 9, Denominator: 13},
				{Numerator: 10, Denominator: 13},
			},
		},
		result.DrepVotingThresholds,
		"DrepVotingThresholds",
	)
	assert.Equal(t, uint32(7), result.MinCommitteeSize, "MinCommitteeSize")
	assert.Equal(
		t,
		uint64(365),
		result.CommitteeTermLimit,
		"CommitteeTermLimit",
	)
	assert.Equal(
		t,
		uint64(30000),
		result.GovernanceActionValidityPeriod,
		"GovernanceActionValidityPeriod",
	)
	assert.Equal(t,
		common.ToUtxorpcBigInt(100000000001),
		result.GovernanceActionDeposit,
		"GovernanceActionDeposit",
	)
	assert.Equal(t,
		common.ToUtxorpcBigInt(500000000),
		result.DrepDeposit,
		"DrepDeposit",
	)
	assert.Equal(
		t,
		uint64(20),
		result.DrepInactivityPeriod,
		"DrepInactivityPeriod",
	)

	// Changing GovActionDeposit must change the converted value -- this is
	// the exact audit scenario (100000000000 -> 100000000001 previously
	// produced an identical, empty diff).
	changed := params
	changed.GovActionDeposit = 100000000000
	changedResult, err := changed.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() conversion failed: %v", err)
	}
	assert.NotEqual(
		t,
		result.GovernanceActionDeposit,
		changedResult.GovernanceActionDeposit,
		"GovActionDeposit change must be visible in Utxorpc() output",
	)
}

// TestConwayUtxorpc_GovernanceFields_UnsetVotingThresholdsOmitted verifies
// that a ConwayProtocolParameters value with unset (zero-value)
// PoolVotingThresholds/DRepVotingThresholds/MinFeeRefScriptCostPerByte -- as
// happens when a value is constructed directly rather than decoded from a
// full on-chain protocol-parameters value or genesis -- leaves the
// corresponding utxorpc fields nil instead of emitting a spurious all-zero
// VotingThresholds list, which would misrepresent a real 0/0 rational as a
// present-but-degenerate threshold.
func TestConwayUtxorpc_GovernanceFields_UnsetVotingThresholdsOmitted(
	t *testing.T,
) {
	params := conway.ConwayProtocolParameters{
		A0:  &cbor.Rat{Rat: big.NewRat(1, 2)},
		Rho: &cbor.Rat{Rat: big.NewRat(3, 4)},
		Tau: &cbor.Rat{Rat: big.NewRat(5, 6)},
		ExecutionCosts: common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(1, 2)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(2, 3)},
		},
	}
	result, err := params.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() conversion failed: %v", err)
	}
	assert.Nil(t, result.MinFeeScriptRefCostPerByte)
	assert.Nil(t, result.PoolVotingThresholds)
	assert.Nil(t, result.DrepVotingThresholds)
}

// TestConwayUtxorpc_MinFeeRefScriptCostPerByteNilEmbeddedRatDoesNotPanic is
// the regression test for a review finding on
// blinklabs-io/gouroboros#2292: MinFeeRefScriptCostPerByte is a *cbor.Rat,
// and the nil check in Utxorpc()'s original validation guard only covered
// that outer pointer -- a non-nil *cbor.Rat whose embedded *big.Rat is
// itself nil (the same "unset" representation
// TestConwayUtxorpc_GovernanceFields_UnsetVotingThresholdsOmitted's
// zero-value fields use, just wrapped in a non-nil pointer instead of a
// nil one) reached a call to Num() on a nil *big.Rat and panicked, before
// the nil-safe ratPtrToUtxorpcRationalNumber conversion ever got a chance
// to correctly return nil for it.
func TestConwayUtxorpc_MinFeeRefScriptCostPerByteNilEmbeddedRatDoesNotPanic(
	t *testing.T,
) {
	params := conway.ConwayProtocolParameters{
		A0:  &cbor.Rat{Rat: big.NewRat(1, 2)},
		Rho: &cbor.Rat{Rat: big.NewRat(3, 4)},
		Tau: &cbor.Rat{Rat: big.NewRat(5, 6)},
		ExecutionCosts: common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(1, 2)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(2, 3)},
		},
		// Non-nil *cbor.Rat, nil embedded *big.Rat -- the panic-inducing
		// shape, distinct from a nil MinFeeRefScriptCostPerByte pointer
		// entirely (already covered by the unset-fields test above).
		MinFeeRefScriptCostPerByte: &cbor.Rat{},
	}

	require.NotPanics(t, func() {
		result, err := params.Utxorpc()
		require.NoError(t, err)
		assert.Nil(t, result.MinFeeScriptRefCostPerByte)
	})
}

// TestConwayUtxorpc_VotingThresholdOutOfRangeRejected is the regression
// test for a review finding on blinklabs-io/gouroboros#2292:
// ratToUtxorpcRationalNumber cast a threshold's numerator/denominator to
// int32/uint32 unconditionally, unlike every sibling rational field in this
// file (A0, Rho, Tau, the execution-cost prices), which reject an
// out-of-range value with an error rather than silently wrapping it during
// the cast. An out-of-range voting threshold must fail the same way.
func TestConwayUtxorpc_VotingThresholdOutOfRangeRejected(t *testing.T) {
	base := conway.ConwayProtocolParameters{
		A0:  &cbor.Rat{Rat: big.NewRat(1, 2)},
		Rho: &cbor.Rat{Rat: big.NewRat(3, 4)},
		Tau: &cbor.Rat{Rat: big.NewRat(5, 6)},
		ExecutionCosts: common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(1, 2)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(2, 3)},
		},
	}
	outOfRange := cbor.Rat{
		Rat: big.NewRat(int64(math.MaxInt32)+1, 1),
	}

	t.Run("pool voting threshold", func(t *testing.T) {
		params := base
		params.PoolVotingThresholds = conway.PoolVotingThresholds{
			MotionNoConfidence: outOfRange,
		}
		_, err := params.Utxorpc()
		require.Error(t, err)
	})

	t.Run("drep voting threshold", func(t *testing.T) {
		params := base
		params.DRepVotingThresholds = conway.DRepVotingThresholds{
			MotionNoConfidence: outOfRange,
		}
		_, err := params.Utxorpc()
		require.Error(t, err)
	})
}

// Unit test for ConwayTransactionBody.Utxorpc()
func TestConwayTransactionBody_Utxorpc(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		1,
	)
	var inputSet conway.ConwayTransactionInputSet
	inputSet.SetItems([]shelley.ShelleyTransactionInput{input})

	address := common.Address{}
	output := babbage.BabbageTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 5000},
	}
	txCollateral := cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	txTotalCollateral := uint64(200)
	txReferenceInputs := cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	txAuxDataHash := &common.Blake2b256{0xde, 0xad, 0xbe, 0xef}
	txValidityIntervalStart := uint64(4000)
	var signer common.Blake2b224
	copy(signer[:], []byte{0xab, 0xcd, 0xef})
	txRequiredSigners := cbor.NewSetType(
		[]common.Blake2b224{signer},
		false,
	)
	txScriptDataHash := &common.Blake2b256{0xba, 0xad, 0xf0, 0x0d}
	txMint := &common.MultiAsset[common.MultiAssetTypeMint]{}

	body := conway.ConwayTransactionBody{
		TxInputs:                inputSet,
		TxOutputs:               []babbage.BabbageTransactionOutput{output},
		TxFee:                   1000,
		Ttl:                     5000,
		TxCollateral:            txCollateral,
		TxTotalCollateral:       txTotalCollateral,
		TxReferenceInputs:       txReferenceInputs,
		TxAuxDataHash:           txAuxDataHash,
		TxValidityIntervalStart: txValidityIntervalStart,
		TxRequiredSigners:       txRequiredSigners,
		TxScriptDataHash:        txScriptDataHash,
		TxMint:                  txMint,
	}

	got, err := body.Utxorpc()
	if err != nil {
		t.Fatalf(
			"Could not convert the transaction body to utxorpc format %v",
			err,
		)
	}

	if got.Fee.GetInt() != 1000 {
		t.Errorf("Fee mismatch: got %d, want 1000", got.Fee.GetInt())
	}
	if len(got.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(got.Inputs))
	}
	if len(got.Outputs) != 1 {
		t.Fatalf("Expected 1 output, got %d", len(got.Outputs))
	}
	coin := got.Outputs[0].Coin
	if bigInt := coin.GetBigUInt(); bigInt != nil {
		coinValue := new(big.Int).SetBytes(bigInt).Uint64()
		if coinValue != uint64(5000) {
			t.Errorf(
				"Output coin mismatch: got %d, want 5000",
				coinValue,
			)
		}
	} else if coin.GetInt() != 5000 {
		t.Errorf("Output coin mismatch: got %d, want 5000", coin.GetInt())
	}
	if len(got.Hash) == 0 {
		t.Error("Expected non-empty transaction hash")
	}
}

// Unit test for ConwayTransaction.Utxorpc()
func TestConwayTransaction_Utxorpc(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		0,
	)
	var inputSet conway.ConwayTransactionInputSet
	inputSet.SetItems([]shelley.ShelleyTransactionInput{input})

	address := common.Address{}
	output := babbage.BabbageTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 5000},
	}

	txCollateral := cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	txTotalCollateral := uint64(200)
	txReferenceInputs := cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{input},
		false,
	)
	txAuxDataHash := &common.Blake2b256{0xde, 0xad, 0xbe, 0xef}
	txValidityIntervalStart := uint64(4000)
	var signer common.Blake2b224
	copy(signer[:], []byte{0xab, 0xcd, 0xef})
	txRequiredSigners := cbor.NewSetType(
		[]common.Blake2b224{signer},
		false,
	)
	txScriptDataHash := &common.Blake2b256{0xba, 0xad, 0xf0, 0x0d}
	txMint := &common.MultiAsset[common.MultiAssetTypeMint]{}

	body := conway.ConwayTransactionBody{
		TxInputs:                inputSet,
		TxOutputs:               []babbage.BabbageTransactionOutput{output},
		TxFee:                   1000,
		Ttl:                     5000,
		TxCollateral:            txCollateral,
		TxTotalCollateral:       txTotalCollateral,
		TxReferenceInputs:       txReferenceInputs,
		TxAuxDataHash:           txAuxDataHash,
		TxValidityIntervalStart: txValidityIntervalStart,
		TxRequiredSigners:       txRequiredSigners,
		TxScriptDataHash:        txScriptDataHash,
		TxMint:                  txMint,
	}

	tx := &conway.ConwayTransaction{
		Body:      body,
		TxIsValid: true,
	}

	got, err := tx.Utxorpc()
	if err != nil {
		t.Fatalf("Could not convert transaction to utxorpc format: %v", err)
	}

	if got.Fee.GetInt() != 1000 {
		t.Errorf(
			"Transaction fee mismatch: got %d, want 1000",
			got.Fee.GetInt(),
		)
	}
	if len(got.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(got.Inputs))
	}
	if len(got.Outputs) != 1 {
		t.Fatalf("Expected 1 output, got %d", len(got.Outputs))
	}
	coin := got.Outputs[0].Coin
	if bigInt := coin.GetBigUInt(); bigInt != nil {
		coinValue := new(big.Int).SetBytes(bigInt).Uint64()
		if coinValue != uint64(5000) {
			t.Errorf(
				"Output coin mismatch: got %d, want 5000",
				coinValue,
			)
		}
	} else if coin.GetInt() != 5000 {
		t.Errorf("Output coin mismatch: got %d, want 5000", coin.GetInt())
	}
	if len(got.Hash) == 0 {
		t.Error("Expected non-empty transaction hash")
	}
}
func TestConwayProtocolParameters_CborRoundTrip(t *testing.T) {
	// Test CBOR round-trip for Conway protocol parameters (CIP-0009)
	params := &conway.ConwayProtocolParameters{
		MinFeeA:            44,
		MinFeeB:            155381,
		MaxBlockBodySize:   90112,
		MaxTxSize:          16384,
		MaxBlockHeaderSize: 1100,
		KeyDeposit:         2000000,
		PoolDeposit:        500000000,
		MaxEpoch:           18,
		NOpt:               150,
		A0:                 &cbor.Rat{Rat: big.NewRat(1, 2)},
		Rho:                &cbor.Rat{Rat: big.NewRat(3, 100000)},
		Tau:                &cbor.Rat{Rat: big.NewRat(1, 20)},
		ProtocolVersion: common.ProtocolParametersProtocolVersion{
			Major: 9,
			Minor: 0,
		},
		MinPoolCost:    340000000,
		AdaPerUtxoByte: 4310,
		CostModels:     map[uint][]int64{0: {1, 2, 3}},
		ExecutionCosts: common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(577, 10000)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(721, 10000000)},
		},
		MaxTxExUnits: common.ExUnits{
			Memory: 14000000,
			Steps:  10000000000,
		},
		MaxBlockExUnits: common.ExUnits{
			Memory: 62000000,
			Steps:  40000000000,
		},
		MaxValueSize:         5000,
		CollateralPercentage: 150,
		MaxCollateralInputs:  3,
		PoolVotingThresholds: conway.PoolVotingThresholds{
			MotionNoConfidence:    cbor.Rat{Rat: big.NewRat(51, 100)},
			CommitteeNormal:       cbor.Rat{Rat: big.NewRat(51, 100)},
			CommitteeNoConfidence: cbor.Rat{Rat: big.NewRat(51, 100)},
			HardForkInitiation:    cbor.Rat{Rat: big.NewRat(51, 100)},
			PpSecurityGroup:       cbor.Rat{Rat: big.NewRat(51, 100)},
		},
		DRepVotingThresholds: conway.DRepVotingThresholds{
			MotionNoConfidence:    cbor.Rat{Rat: big.NewRat(51, 100)},
			CommitteeNormal:       cbor.Rat{Rat: big.NewRat(51, 100)},
			CommitteeNoConfidence: cbor.Rat{Rat: big.NewRat(51, 100)},
			UpdateToConstitution:  cbor.Rat{Rat: big.NewRat(51, 100)},
			HardForkInitiation:    cbor.Rat{Rat: big.NewRat(51, 100)},
			PpNetworkGroup:        cbor.Rat{Rat: big.NewRat(51, 100)},
			PpEconomicGroup:       cbor.Rat{Rat: big.NewRat(51, 100)},
			PpTechnicalGroup:      cbor.Rat{Rat: big.NewRat(51, 100)},
			PpGovGroup:            cbor.Rat{Rat: big.NewRat(51, 100)},
			TreasuryWithdrawal:    cbor.Rat{Rat: big.NewRat(51, 100)},
		},
		MinCommitteeSize:           7,
		CommitteeTermLimit:         200,
		GovActionValidityPeriod:    1209600,
		GovActionDeposit:           1000000000,
		DRepDeposit:                500000000,
		DRepInactivityPeriod:       60,
		MinFeeRefScriptCostPerByte: &cbor.Rat{Rat: big.NewRat(15, 1)},
	}

	// Encode to CBOR
	cborData, err := cbor.Encode(params)
	assert.NoError(t, err)
	assert.NotEmpty(t, cborData)

	// Decode back from CBOR
	var decoded conway.ConwayProtocolParameters
	_, err = cbor.Decode(cborData, &decoded)
	assert.NoError(t, err)

	// Verify all fields are preserved - compare dereferenced params with decoded value
	// Note: We compare individual fields because reflect.DeepEqual can have issues
	// with big.Rat internal representations after CBOR round-trip
	assert.Equal(t, params.MinFeeA, decoded.MinFeeA)
	assert.Equal(t, params.MinFeeB, decoded.MinFeeB)
	assert.Equal(t, params.MaxBlockBodySize, decoded.MaxBlockBodySize)
	assert.Equal(t, params.MaxTxSize, decoded.MaxTxSize)
	assert.Equal(t, params.MaxBlockHeaderSize, decoded.MaxBlockHeaderSize)
	assert.Equal(t, params.KeyDeposit, decoded.KeyDeposit)
	assert.Equal(t, params.PoolDeposit, decoded.PoolDeposit)
	assert.Equal(t, params.MaxEpoch, decoded.MaxEpoch)
	assert.Equal(t, params.NOpt, decoded.NOpt)
	assert.Equal(t, params.A0.Rat.RatString(), decoded.A0.Rat.RatString())
	assert.Equal(t, params.Rho.Rat.RatString(), decoded.Rho.Rat.RatString())
	assert.Equal(t, params.Tau.Rat.RatString(), decoded.Tau.Rat.RatString())
	assert.Equal(t, params.ProtocolVersion.Major, decoded.ProtocolVersion.Major)
	assert.Equal(t, params.ProtocolVersion.Minor, decoded.ProtocolVersion.Minor)
	assert.Equal(t, params.MinPoolCost, decoded.MinPoolCost)
	assert.Equal(t, params.AdaPerUtxoByte, decoded.AdaPerUtxoByte)
	assert.Equal(t, params.CostModels, decoded.CostModels)
	assert.Equal(
		t,
		params.ExecutionCosts.MemPrice.Rat.RatString(),
		decoded.ExecutionCosts.MemPrice.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.ExecutionCosts.StepPrice.Rat.RatString(),
		decoded.ExecutionCosts.StepPrice.Rat.RatString(),
	)
	assert.Equal(t, params.MaxTxExUnits, decoded.MaxTxExUnits)
	assert.Equal(t, params.MaxBlockExUnits, decoded.MaxBlockExUnits)
	assert.Equal(t, params.MaxValueSize, decoded.MaxValueSize)
	assert.Equal(t, params.CollateralPercentage, decoded.CollateralPercentage)
	assert.Equal(t, params.MaxCollateralInputs, decoded.MaxCollateralInputs)
	// Pool voting thresholds
	assert.Equal(
		t,
		params.PoolVotingThresholds.MotionNoConfidence.Rat.RatString(),
		decoded.PoolVotingThresholds.MotionNoConfidence.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.PoolVotingThresholds.CommitteeNormal.Rat.RatString(),
		decoded.PoolVotingThresholds.CommitteeNormal.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.PoolVotingThresholds.CommitteeNoConfidence.Rat.RatString(),
		decoded.PoolVotingThresholds.CommitteeNoConfidence.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.PoolVotingThresholds.HardForkInitiation.Rat.RatString(),
		decoded.PoolVotingThresholds.HardForkInitiation.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.PoolVotingThresholds.PpSecurityGroup.Rat.RatString(),
		decoded.PoolVotingThresholds.PpSecurityGroup.Rat.RatString(),
	)
	// DRep voting thresholds
	assert.Equal(
		t,
		params.DRepVotingThresholds.MotionNoConfidence.Rat.RatString(),
		decoded.DRepVotingThresholds.MotionNoConfidence.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.DRepVotingThresholds.CommitteeNormal.Rat.RatString(),
		decoded.DRepVotingThresholds.CommitteeNormal.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.DRepVotingThresholds.CommitteeNoConfidence.Rat.RatString(),
		decoded.DRepVotingThresholds.CommitteeNoConfidence.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.DRepVotingThresholds.UpdateToConstitution.Rat.RatString(),
		decoded.DRepVotingThresholds.UpdateToConstitution.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.DRepVotingThresholds.HardForkInitiation.Rat.RatString(),
		decoded.DRepVotingThresholds.HardForkInitiation.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.DRepVotingThresholds.PpNetworkGroup.Rat.RatString(),
		decoded.DRepVotingThresholds.PpNetworkGroup.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.DRepVotingThresholds.PpEconomicGroup.Rat.RatString(),
		decoded.DRepVotingThresholds.PpEconomicGroup.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.DRepVotingThresholds.PpTechnicalGroup.Rat.RatString(),
		decoded.DRepVotingThresholds.PpTechnicalGroup.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.DRepVotingThresholds.PpGovGroup.Rat.RatString(),
		decoded.DRepVotingThresholds.PpGovGroup.Rat.RatString(),
	)
	assert.Equal(
		t,
		params.DRepVotingThresholds.TreasuryWithdrawal.Rat.RatString(),
		decoded.DRepVotingThresholds.TreasuryWithdrawal.Rat.RatString(),
	)
	// Governance parameters
	assert.Equal(t, params.MinCommitteeSize, decoded.MinCommitteeSize)
	assert.Equal(t, params.CommitteeTermLimit, decoded.CommitteeTermLimit)
	assert.Equal(
		t,
		params.GovActionValidityPeriod,
		decoded.GovActionValidityPeriod,
	)
	assert.Equal(t, params.GovActionDeposit, decoded.GovActionDeposit)
	assert.Equal(t, params.DRepDeposit, decoded.DRepDeposit)
	assert.Equal(t, params.DRepInactivityPeriod, decoded.DRepInactivityPeriod)
	assert.Equal(
		t,
		params.MinFeeRefScriptCostPerByte.Rat.RatString(),
		decoded.MinFeeRefScriptCostPerByte.Rat.RatString(),
	)
}

func TestConwayProtocolParameterUpdateToPlutusDataCostModels(t *testing.T) {
	update := conway.ConwayProtocolParameterUpdate{
		CostModels: map[uint][]int64{
			2: {8, 7},
			0: {1, 2, 3},
		},
	}
	expected := data.NewMap([][2]data.PlutusData{
		{
			testPlutusInteger(18),
			data.NewMap([][2]data.PlutusData{
				{
					testPlutusInteger(0),
					data.NewList(
						testPlutusInteger(1),
						testPlutusInteger(2),
						testPlutusInteger(3),
					),
				},
				{
					testPlutusInteger(2),
					data.NewList(
						testPlutusInteger(8),
						testPlutusInteger(7),
					),
				},
			}),
		},
	})

	result := update.ToPlutusData()
	assert.True(t, expected.Equal(result), "got %#v", result)
}

func TestConwayProtocolParameterUpdate_BootstrapRestrictedFields(t *testing.T) {
	t.Run("empty update has no restricted fields", func(t *testing.T) {
		u := &conway.ConwayProtocolParameterUpdate{}
		fields := u.BootstrapRestrictedFields()
		if len(fields) != 0 {
			t.Fatalf("want empty, got %v", fields)
		}
	})

	t.Run(
		"DRepDeposit and MinCommitteeSize are restricted",
		func(t *testing.T) {
			dep := uint64(500_000_000)
			size := uint(7)
			u := &conway.ConwayProtocolParameterUpdate{
				DRepDeposit:      &dep,
				MinCommitteeSize: &size,
			}
			fields := u.BootstrapRestrictedFields()
			want := []string{"MinCommitteeSize", "DRepDeposit"}
			if !reflect.DeepEqual(fields, want) {
				t.Fatalf("want %v, got %v", want, fields)
			}
		},
	)

	t.Run("non-restricted fields are ignored", func(t *testing.T) {
		fee := uint(44)
		u := &conway.ConwayProtocolParameterUpdate{MinFeeA: &fee}
		fields := u.BootstrapRestrictedFields()
		if len(fields) != 0 {
			t.Fatalf("want empty for MinFeeA-only update, got %v", fields)
		}
	})
}

// TestConwayProtocolParameterUpdate_SecurityGroupFields pins the Conway
// security group: the set of protocol parameters annotated 'SecurityGroup in
// the ConwayPParams record of cardano-ledger
// eras/conway/impl/src/Cardano/Ledger/Conway/PParams.hs (lines 644-711 at
// commit 08773e9a8f911f67209560a4e401369cbb21a0cb). Every other parameter is
// 'NoStakePoolGroup, and cppProtocolVersion is HKDNoUpdate (it is not
// changeable by a ParameterChange action at all).
func TestConwayProtocolParameterUpdate_SecurityGroupFields(t *testing.T) {
	t.Run("empty update touches no security parameter", func(t *testing.T) {
		u := &conway.ConwayProtocolParameterUpdate{}
		if fields := u.SecurityGroupFields(); len(fields) != 0 {
			t.Fatalf("want empty, got %v", fields)
		}
	})

	t.Run("every security group parameter is reported", func(t *testing.T) {
		val := uint(1)
		val64 := uint64(1)
		rat := &cbor.Rat{Rat: big.NewRat(1, 2)}
		u := &conway.ConwayProtocolParameterUpdate{
			MinFeeA:                    &val,
			MinFeeB:                    &val,
			MaxBlockBodySize:           &val,
			MaxTxSize:                  &val,
			MaxBlockHeaderSize:         &val,
			AdaPerUtxoByte:             &val64,
			MaxBlockExUnits:            &common.ExUnits{Memory: 1, Steps: 1},
			MaxValueSize:               &val,
			GovActionDeposit:           &val64,
			MinFeeRefScriptCostPerByte: rat,
		}
		want := []string{
			"MinFeeA",
			"MinFeeB",
			"MaxBlockBodySize",
			"MaxTxSize",
			"MaxBlockHeaderSize",
			"AdaPerUtxoByte",
			"MaxBlockExUnits",
			"MaxValueSize",
			"GovActionDeposit",
			"MinFeeRefScriptCostPerByte",
		}
		if fields := u.SecurityGroupFields(); !reflect.DeepEqual(fields, want) {
			t.Fatalf("want %v, got %v", want, fields)
		}
	})

	t.Run("non-security parameters are ignored", func(t *testing.T) {
		val := uint(1)
		val64 := uint64(1)
		rat := &cbor.Rat{Rat: big.NewRat(1, 2)}
		u := &conway.ConwayProtocolParameterUpdate{
			KeyDeposit:  &val,
			PoolDeposit: &val,
			MaxEpoch:    &val,
			NOpt:        &val,
			A0:          rat,
			Rho:         rat,
			Tau:         rat,
			ProtocolVersion: &common.ProtocolParametersProtocolVersion{
				Major: 10,
			},
			MinPoolCost:             &val64,
			CostModels:              map[uint][]int64{0: {1}},
			ExecutionCosts:          &common.ExUnitPrice{},
			MaxTxExUnits:            &common.ExUnits{Memory: 1, Steps: 1},
			CollateralPercentage:    &val,
			MaxCollateralInputs:     &val,
			PoolVotingThresholds:    &conway.PoolVotingThresholds{},
			DRepVotingThresholds:    &conway.DRepVotingThresholds{},
			MinCommitteeSize:        &val,
			CommitteeTermLimit:      &val64,
			GovActionValidityPeriod: &val64,
			DRepDeposit:             &val64,
			DRepInactivityPeriod:    &val64,
		}
		if fields := u.SecurityGroupFields(); len(fields) != 0 {
			t.Fatalf("want empty, got %v", fields)
		}
	})
}
