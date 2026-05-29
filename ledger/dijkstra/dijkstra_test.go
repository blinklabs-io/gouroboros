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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

func minimalTxBody() map[uint]any {
	return map[uint]any{
		0: []any{},
		1: []any{},
		2: uint64(0),
	}
}

func minimalWitnessSet() map[uint]any {
	return map[uint]any{}
}

func minimalTxParts() []any {
	return []any{minimalTxBody(), minimalWitnessSet(), nil}
}

func minimalBlockBodyParts(invalidTxs []uint) []any {
	if invalidTxs == nil {
		invalidTxs = []uint{}
	}
	return []any{
		[]any{minimalTxBody()},
		[]any{minimalWitnessSet()},
		map[uint]any{},
		invalidTxs,
		nil,
		nil,
	}
}

func TestDijkstraEra(t *testing.T) {
	require.Equal(t, uint8(EraIdDijkstra), EraDijkstra.Id)
	require.Equal(t, EraNameDijkstra, EraDijkstra.Name)
	require.Equal(t, EraDijkstra, common.EraById(EraIdDijkstra))
}

func TestDijkstraTypeConstants(t *testing.T) {
	require.Equal(t, 8, BlockTypeDijkstra)
	require.Equal(t, 7, BlockHeaderTypeDijkstra)
	require.Equal(t, 7, TxTypeDijkstra)
}

func TestDijkstraTransactionDecodesThreePartTx(t *testing.T) {
	txCbor, err := cbor.Encode(minimalTxParts())
	require.NoError(t, err)

	tx, err := NewDijkstraTransactionFromCbor(txCbor)
	require.NoError(t, err)
	require.True(t, tx.IsValid())
	require.Equal(t, TxTypeDijkstra, tx.Type())
	require.Equal(t, txCbor, tx.Cbor())
}

func TestDijkstraTransactionAllowsOnlyTrueIsValidForMempool(t *testing.T) {
	parts := minimalTxParts()
	withTrue := []any{parts[0], parts[1], true, parts[2]}
	txCbor, err := cbor.Encode(withTrue)
	require.NoError(t, err)

	tx, err := NewDijkstraTransactionFromCbor(txCbor)
	require.NoError(t, err)
	require.True(t, tx.IsValid())

	withFalse := []any{parts[0], parts[1], false, parts[2]}
	txCbor, err = cbor.Encode(withFalse)
	require.NoError(t, err)

	_, err = NewDijkstraTransactionFromCbor(txCbor)
	require.ErrorContains(t, err, "is_valid=false")
}

func TestDijkstraTransactionRejectsOversizedCbor(t *testing.T) {
	txCbor := make([]byte, MaxTxSize+1)

	_, err := NewDijkstraTransactionFromCbor(txCbor)
	require.ErrorContains(t, err, "MaxTxSize")
}

func TestDijkstraBlockBodyRejectsMismatchedBodiesAndWitnesses(t *testing.T) {
	bodyCbor, err := cbor.Encode([]any{
		[]any{minimalTxBody()},
		[]any{},
		map[uint]any{},
		[]uint{},
		nil,
		nil,
	})
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	err = blockBody.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "different number of transaction bodies")
}

func TestDijkstraBlockBodyAppliesInvalidTransactionIndices(t *testing.T) {
	bodyCbor, err := cbor.Encode(minimalBlockBodyParts([]uint{0}))
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	require.NoError(t, blockBody.UnmarshalCBOR(bodyCbor))
	require.Len(t, blockBody.Transactions, 1)
	require.False(t, blockBody.Transactions[0].IsValid())
}

func TestDijkstraBlockBodyRejectsInvalidTransactionIndexOutOfRange(t *testing.T) {
	bodyCbor, err := cbor.Encode(minimalBlockBodyParts([]uint{1}))
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	err = blockBody.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "outside transaction list length")
}

func TestDijkstraBlockMarshalUsesSevenItemEnvelope(t *testing.T) {
	block := DijkstraBlock{
		BlockBody: DijkstraBlockBody{
			TransactionBodies: []DijkstraTransactionBody{
				{TxFee: 1},
			},
			TransactionWitnessSets: []DijkstraTransactionWitnessSet{{}},
			InvalidTransactions:    []uint{},
			LeiosCertificate:       &DijkstraLeiosCertificate{},
			PerasCertificate:       &DijkstraPerasCertificate{},
		},
	}

	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	var raw []cbor.RawMessage
	_, err = cbor.Decode(blockCbor, &raw)
	require.NoError(t, err)
	require.Len(t, raw, 7)
	// When a Leios cert is present, the prototype encodes empty transaction
	// components and resolves the EB closure through LeiosDB.
	require.Equal(t, []byte{0x80}, []byte(raw[1]))
	require.Equal(t, []byte{0x80}, []byte(raw[2]))
	require.Equal(t, []byte{0xa0}, []byte(raw[3]))
	require.Equal(t, []byte{0x80}, []byte(raw[4]))
	require.Equal(t, []byte{0x80}, []byte(raw[5]))
	require.Equal(t, []byte{0x80}, []byte(raw[6]))
}

func TestDijkstraBlockBodyHashIncludesLeiosAndPerasCertSlots(t *testing.T) {
	withoutCerts := DijkstraBlockBody{}
	withCerts := DijkstraBlockBody{
		LeiosCertificate: &DijkstraLeiosCertificate{},
		PerasCertificate: &DijkstraPerasCertificate{},
	}

	require.NotEqual(t, withoutCerts.Hash(), withCerts.Hash())
}

func TestDijkstraBlockRoundTripWithBodyHash(t *testing.T) {
	blockBody := DijkstraBlockBody{
		TransactionBodies:      []DijkstraTransactionBody{},
		TransactionWitnessSets: []DijkstraTransactionWitnessSet{},
		InvalidTransactions:    []uint{},
	}
	block := DijkstraBlock{
		BlockHeader: &DijkstraBlockHeader{
			BabbageBlockHeader: babbage.BabbageBlockHeader{
				Body: babbage.BabbageBlockHeaderBody{
					BlockBodyHash: blockBody.Hash(),
					VrfKey:        make([]byte, 32),
					VrfResult: common.VrfResult{
						Output: []byte{},
						Proof:  make([]byte, 80),
					},
					OpCert: babbage.BabbageOpCert{
						HotVkey:   make([]byte, 32),
						Signature: make([]byte, 64),
					},
					ProtoVersion: babbage.BabbageProtoVersion{
						Major: MinProtocolVersionDijkstra,
					},
				},
				Signature: make([]byte, 448),
			},
		},
		BlockBody: blockBody,
	}

	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	decoded, err := NewDijkstraBlockFromCbor(blockCbor)
	require.NoError(t, err)
	require.Equal(t, blockBody.Hash(), decoded.BlockBodyHash())
	require.Empty(t, decoded.Transactions())

	var raw []cbor.RawMessage
	_, err = cbor.Decode(blockCbor, &raw)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	require.NotEmpty(t, decoded.BlockHeader.Cbor())
	require.Equal(t, []byte(raw[0]), decoded.BlockHeader.Cbor())
}

func TestIsCborNullOnlyAcceptsEncodedNull(t *testing.T) {
	require.True(t, isCborNull(cbor.RawMessage{0xf6}))
	require.False(t, isCborNull(nil))
	require.False(t, isCborNull(cbor.RawMessage{}))
}

func TestDijkstraProtocolParametersRoundTrip(t *testing.T) {
	rat := func(num, denom int64) *cbor.Rat {
		return &cbor.Rat{Rat: big.NewRat(num, denom)}
	}
	ratValue := func(num, denom int64) cbor.Rat {
		return cbor.Rat{Rat: big.NewRat(num, denom)}
	}
	pparams := DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			MinFeeA:    44,
			MaxTxSize:  16384,
			A0:         rat(1, 2),
			Rho:        rat(3, 1000),
			Tau:        rat(1, 5),
			CostModels: map[uint][]int64{3: {1, 2, 3}},
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: 12,
			},
			ExecutionCosts: common.ExUnitPrice{
				MemPrice:  rat(1, 10),
				StepPrice: rat(2, 10),
			},
			MaxTxExUnits:        common.ExUnits{Memory: 1, Steps: 2},
			MaxBlockExUnits:     common.ExUnits{Memory: 3, Steps: 4},
			MaxCollateralInputs: 3,
			PoolVotingThresholds: conway.PoolVotingThresholds{
				MotionNoConfidence:    ratValue(1, 2),
				CommitteeNormal:       ratValue(1, 2),
				CommitteeNoConfidence: ratValue(1, 2),
				HardForkInitiation:    ratValue(1, 2),
				PpSecurityGroup:       ratValue(1, 2),
			},
			DRepVotingThresholds: conway.DRepVotingThresholds{
				MotionNoConfidence:    ratValue(1, 2),
				CommitteeNormal:       ratValue(1, 2),
				CommitteeNoConfidence: ratValue(1, 2),
				UpdateToConstitution:  ratValue(1, 2),
				HardForkInitiation:    ratValue(1, 2),
				PpNetworkGroup:        ratValue(1, 2),
				PpEconomicGroup:       ratValue(1, 2),
				PpTechnicalGroup:      ratValue(1, 2),
				PpGovGroup:            ratValue(1, 2),
				TreasuryWithdrawal:    ratValue(1, 2),
			},
			MinFeeRefScriptCostPerByte: rat(15, 1000),
		},
		MaxRefScriptSizePerBlock: 1000,
		MaxRefScriptSizePerTx:    2000,
		RefScriptCostStride:      16,
		RefScriptCostMultiplier:  rat(2, 1),
	}

	pparamsCbor, err := cbor.Encode(pparams)
	require.NoError(t, err)

	var decoded DijkstraProtocolParameters
	_, err = cbor.Decode(pparamsCbor, &decoded)
	require.NoError(t, err)
	require.Equal(t, uint(44), decoded.MinFeeA)
	require.Equal(t, uint(16384), decoded.MaxTxSize)
	require.Equal(t, uint(3), decoded.MaxCollateralInputs)
	require.Equal(t, uint32(1000), decoded.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(2000), decoded.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(16), decoded.RefScriptCostStride)
	require.Equal(t, 0, decoded.RefScriptCostMultiplier.Cmp(big.NewRat(2, 1)))
}

func TestDijkstraProtocolParametersUpdateNil(t *testing.T) {
	pparams := DijkstraProtocolParameters{
		MaxRefScriptSizePerBlock: 1000,
	}

	require.NotPanics(t, func() {
		pparams.Update(nil)
	})
	require.Equal(t, uint32(1000), pparams.MaxRefScriptSizePerBlock)
}

func TestDijkstraProtocolParameterUpdateDecodesConwayAndDijkstraFields(t *testing.T) {
	updateCbor, err := cbor.Encode(map[uint]any{
		0:  uint(44),
		34: uint32(1000),
		35: uint32(2000),
		36: uint32(16),
		37: cbor.Rat{Rat: big.NewRat(2, 1)},
	})
	require.NoError(t, err)

	var update DijkstraProtocolParameterUpdate
	_, err = cbor.Decode(updateCbor, &update)
	require.NoError(t, err)
	require.Equal(t, updateCbor, update.Cbor())
	require.NotNil(t, update.MinFeeA)
	require.Equal(t, uint(44), *update.MinFeeA)
	require.NotNil(t, update.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(1000), *update.MaxRefScriptSizePerBlock)
	require.NotNil(t, update.RefScriptCostMultiplier)
	require.Equal(t, 0, update.RefScriptCostMultiplier.Cmp(big.NewRat(2, 1)))

	var pparams DijkstraProtocolParameters
	pparams.Update(&update)
	require.Equal(t, uint(44), pparams.MinFeeA)
	require.Equal(t, uint32(1000), pparams.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(2000), pparams.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(16), pparams.RefScriptCostStride)
	require.Equal(t, 0, pparams.RefScriptCostMultiplier.Cmp(big.NewRat(2, 1)))
}

func TestDijkstraTransactionLeiosHashCaches(t *testing.T) {
	tx := &DijkstraTransaction{}
	tx.SetCbor([]byte{0x83, 0xa0, 0xa0, 0xa0})

	first := tx.LeiosHash()
	require.NotNil(t, tx.hash)
	cached := tx.hash

	second := tx.LeiosHash()
	require.Equal(t, first, second)
	require.True(t, cached == tx.hash)
}
