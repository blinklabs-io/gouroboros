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

package ledger_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
	"github.com/stretchr/testify/require"
)

func dijkstraBlockLimitWitnesses(
	exUnits common.ExUnits,
) dijkstra.DijkstraTransactionWitnessSet {
	return dijkstra.DijkstraTransactionWitnessSet{
		WsRedeemers: dijkstra.DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagGuarding, Index: 0}: {
					Data: common.Datum{
						Data: data.NewInteger(big.NewInt(0)),
					},
					ExUnits: exUnits,
				},
			},
		},
	}
}

func dijkstraBlockLimitExUnitsTx(
	topLevel common.ExUnits,
	subtransaction common.ExUnits,
) dijkstra.DijkstraTransaction {
	return dijkstra.DijkstraTransaction{
		Body: dijkstra.DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]dijkstra.DijkstraSubTransaction{{
					WitnessSet: dijkstraBlockLimitWitnesses(subtransaction),
				}},
				true,
			),
		},
		WitnessSet: dijkstraBlockLimitWitnesses(topLevel),
		TxIsValid:  true,
	}
}

func buildDijkstraLimitsTestBlock(
	t *testing.T,
	txs []dijkstra.DijkstraTransaction,
) ledger.Block {
	t.Helper()
	headerCborBytes, err := hex.DecodeString(blockLimitsTestHeaderHex)
	require.NoError(t, err)
	header, err := ledger.NewBlockHeaderFromCbor(
		ledger.BlockTypeDijkstra,
		headerCborBytes,
	)
	require.NoError(t, err)
	dijkstraHeader, ok := header.(*dijkstra.DijkstraBlockHeader)
	require.True(t, ok)

	crafted := &dijkstra.DijkstraBlock{
		BlockHeader: dijkstraHeader,
		BlockBody: dijkstra.DijkstraBlockBody{
			Transactions: txs,
		},
	}
	blockCbor, err := cbor.Encode(crafted)
	require.NoError(t, err)
	decoded, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeDijkstra,
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	return decoded
}

func TestVerifyBlockDijkstraExUnitsIncludesEveryTransactionLevel(
	t *testing.T,
) {
	block := buildDijkstraLimitsTestBlock(
		t,
		[]dijkstra.DijkstraTransaction{
			dijkstraBlockLimitExUnitsTx(
				common.ExUnits{Memory: 5, Steps: 7},
				common.ExUnits{Memory: 11, Steps: 13},
			),
			dijkstraBlockLimitExUnitsTx(
				common.ExUnits{Memory: 17, Steps: 19},
				common.ExUnits{Memory: 23, Steps: 29},
			),
		},
	)
	wantTotal := common.ExUnits{Memory: 56, Steps: 68}
	tests := []struct {
		name      string
		max       common.ExUnits
		wantError bool
	}{
		{name: "below limit", max: common.ExUnits{Memory: 57, Steps: 69}},
		{name: "at limit", max: wantTotal},
		{
			name:      "over limit",
			max:       common.ExUnits{Memory: 55, Steps: 67},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pp := &dijkstra.DijkstraProtocolParameters{
				ConwayProtocolParameters: conway.ConwayProtocolParameters{
					MaxBlockExUnits: test.max,
				},
			}
			valid, _, _, _, err := ledger.VerifyBlock(
				block,
				blockLimitsTestEta0Hex,
				blockLimitsTestSlotsPerKesPeriod,
				common.VerifyConfig{
					SkipBodyHashValidation:    true,
					SkipTransactionValidation: true,
					SkipStakePoolValidation:   true,
					ProtocolParameters:        pp,
				},
			)
			if !test.wantError {
				require.NoError(t, err)
				require.True(t, valid)
				return
			}
			require.False(t, valid)
			var target common.BlockExUnitsTooBigError
			require.ErrorAs(t, err, &target)
			require.Equal(t, wantTotal, target.TotalExUnits)
		})
	}
}

func dijkstraBlockLimitScript(t *testing.T) common.PlutusV4Script {
	t.Helper()
	flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{
		Version: lang.LanguageVersionV4,
		Term:    &syn.Error{},
	})
	require.NoError(t, err)
	wrapper, err := cbor.Encode(flat)
	require.NoError(t, err)
	return common.PlutusV4Script(wrapper)
}

func dijkstraBlockLimitRefScriptOutput(
	t *testing.T,
	id byte,
	script common.Script,
) dijkstra.DijkstraTransactionOutput {
	t.Helper()
	credential := make([]byte, common.Blake2b224Size)
	credential[0] = id
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		credential,
		nil,
	)
	require.NoError(t, err)
	return dijkstra.DijkstraTransactionOutput{
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: address,
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV4,
				Script: script,
			},
		},
	}
}

func dijkstraBlockLimitRefScriptTx(
	t *testing.T,
	id byte,
	script common.Script,
) (dijkstra.DijkstraTransaction, common.Utxo) {
	t.Helper()
	txId := make([]byte, common.Blake2b256Size)
	txId[0] = id
	input := shelley.NewShelleyTransactionInput(hex.EncodeToString(txId), 0)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = id
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		common.Blake2b224Hash(publicKey).Bytes(),
		nil,
	)
	require.NoError(t, err)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: address,
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1},
		},
	}
	tx := dijkstra.DijkstraTransaction{
		Body: dijkstra.DijkstraTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{input},
			),
			TxFee: 1,
			TxSubTransactions: cbor.NewSetType(
				[]dijkstra.DijkstraSubTransaction{{
					Body: dijkstra.DijkstraSubTransactionBody{
						TxOutputs: []dijkstra.DijkstraTransactionOutput{
							dijkstraBlockLimitRefScriptOutput(t, id, script),
						},
					},
				}},
				true,
			),
		},
		TxIsValid: true,
	}
	bodyCbor, err := cbor.Encode(&tx.Body)
	require.NoError(t, err)
	tx.Body.SetCborReference(bodyCbor)
	txHash := tx.Hash()
	tx.WitnessSet.VkeyWitnesses = cbor.NewSetType(
		[]common.VkeyWitness{{
			Vkey:      publicKey,
			Signature: ed25519.Sign(privateKey, txHash[:]),
		}},
		true,
	)
	return tx, utxo
}

func TestVerifyBlockDijkstraReferenceScriptSizeIncludesSubtransactions(
	t *testing.T,
) {
	script := dijkstraBlockLimitScript(t)
	txA, utxoA := dijkstraBlockLimitRefScriptTx(t, 1, script)
	txB, utxoB := dijkstraBlockLimitRefScriptTx(t, 2, script)
	block := buildDijkstraLimitsTestBlock(
		t,
		[]dijkstra.DijkstraTransaction{txA, txB},
	)
	state := mockledger.NewLedgerStateBuilder().WithUtxos(
		[]common.Utxo{utxoA, utxoB},
	).Build()
	perTxSize := uint32(len(script.RawScriptBytes()))
	blockSize := uint32(2) * perTxSize
	tests := []struct {
		name      string
		max       uint32
		wantError bool
	}{
		{name: "at limit", max: blockSize},
		{name: "over limit", max: blockSize - 1, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pp := &dijkstra.DijkstraProtocolParameters{
				ConwayProtocolParameters: conway.ConwayProtocolParameters{
					ProtocolVersion: common.ProtocolParametersProtocolVersion{
						Major: common.ProtocolVersionDijkstra,
					},
					MaxTxSize:       16 * 1024,
					MaxValueSize:    5_000,
					MaxTxExUnits:    common.ExUnits{},
					MaxBlockExUnits: common.ExUnits{},
				},
				MaxRefScriptSizePerTx:    perTxSize,
				MaxRefScriptSizePerBlock: test.max,
			}
			valid, _, _, _, err := ledger.VerifyBlock(
				block,
				blockLimitsTestEta0Hex,
				blockLimitsTestSlotsPerKesPeriod,
				common.VerifyConfig{
					SkipBodyHashValidation:  true,
					SkipStakePoolValidation: true,
					LedgerState:             state,
					ProtocolParameters:      pp,
				},
			)
			if !test.wantError {
				require.NoError(t, err)
				require.True(t, valid)
				return
			}
			require.False(t, valid)
			var target common.RefScriptSizePerBlockTooLargeError
			require.ErrorAs(t, err, &target)
			require.Equal(t, uint64(blockSize), target.BlockSize)
		})
	}
}
