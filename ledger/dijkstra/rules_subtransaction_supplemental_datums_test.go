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
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

// musashiSubTxRefInputDatumTxHex is tx 12 (c07f2887...1f2d) of the Musashi
// endorser block 255ea6c1...c2a7. Its top-level witness set carries the datum
// 42, whose hash is only on the UTxO of sub-transaction 0's reference input
// a64abf69...d542#0. The Haskell nodes accepted the block that certifies it.
// The bytes are the block_transaction form taken from the endorser block.
const musashiSubTxRefInputDatumTxHex = "84a500d9010281825820fa05c5450944ee231c5cd86659f5d3bb2382b1732253956be52588c95aa80e7902018182581d60b8a4330fff8dcb6dc2ab5973fd2ef3fc3cd9fcb344d9e9400dc77d6b1a003567e0021a0007a1200b582046b7a61a02a263786a6caa08ba28f512939dbd24f705663f693e4ee0d24818ac178283a300d9010281825820a64abf693cc5594ff04bcc0706d541c354b298b28b2fc6b3feafaed52daed5420112d9010281825820a64abf693cc5594ff04bcc0706d541c354b298b28b2fc6b3feafaed52daed54200018182581d60a630d3e3b042229653440745adccc2917403bd4afae173f8829c75821a0023577fa100d90102818258205b061a5f95a13c46ccac6d4a8ca2fee164cdb5faa2c28d497e1491379f4b600b584036214afada01474a3d87a9150c14b51e4ee25454ac8ca8f5389432a1344b8fdf7a5a19917a3095bcf5433539c17b286177e22c26bc8468a61c19db669a4ba809f683a200d9010281825820fa05c5450944ee231c5cd86659f5d3bb2382b1732253956be52588c95aa80e7901018182581d60a581e5a858e8fb79dc77c86ed4eb7c2cdc6fed9735f360fa1c6ab2681a003d0900a100d90102818258202506fe658448e02d348c2578ca76d2df85cb2f93b92921bd3d373359adb0da7e58405d059fcd70eb88006fab7561188628e2c6fa398b20119bab9a35e4f9e81d1aa9d498f8c17707a4276c66f81f1c3af49eef11133494b2b0b7e1fe4de06a950a05f6a200d90102818258204799ca373b3fa873cba90f47ee251e2042fb831ffcb6c2b692796968d667945558406c15e402978916c3574d1639b87f8a7ef46a24273d7b5af879439f044dbe2743037dac6706b14c13cf3a2dd204cf3e36aa5a8ddb903343bb2927f8aeacd3320704d9010281182af6f5"

const musashiSubTxRefInputDatumTxId = "c07f2887f90a38a9158b242848129d699a9f12e45bfe81679bc76e7f5f021f2d"

func supplementalDatumTestKeyOutput(
	t *testing.T,
	datumHash *common.Blake2b256,
) *alonzo.AlonzoTransactionOutput {
	t.Helper()
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		bytes.Repeat([]byte{0x44}, common.AddressHashSize),
		nil,
	)
	require.NoError(t, err)
	return &alonzo.AlonzoTransactionOutput{
		OutputAddress:   address,
		OutputAmount:    mary.MaryTransactionOutputValue{Amount: 2_000_000},
		OutputDatumHash: datumHash,
	}
}

// cardano-ledger getDijkstraSupplementalDataHashes (Dijkstra/UTxO.hs) allows a
// top-level witness datum whose hash is on a sub-transaction's reference
// input UTxO.
func TestSupplementalDatumsAllowsSubtransactionReferenceInputDatumOnMusashiTx(
	t *testing.T,
) {
	txCbor, err := hex.DecodeString(musashiSubTxRefInputDatumTxHex)
	require.NoError(t, err)
	tx, err := newDijkstraBlockTransactionFromCbor(txCbor)
	require.NoError(t, err)
	require.Equal(t, musashiSubTxRefInputDatumTxId, tx.Hash().String())
	subTxs := tx.Body.TxSubTransactions.Items()
	require.Len(t, subTxs, 2)
	refInputs := subTxs[0].Body.ReferenceInputs()
	require.Len(t, refInputs, 1)
	witnessDatums := tx.WitnessSet.PlutusData()
	require.Len(t, witnessDatums, 1)
	datumHash := witnessDatums[0].Hash()
	require.Equal(
		t,
		"9e1199a988ba72ffd6e9c269cadb3b53b5f360ff99f112d9b2ee30c4d74ad88b",
		datumHash.String(),
	)

	stateWith := func(hash *common.Blake2b256) common.LedgerState {
		return mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{
			Id:     refInputs[0],
			Output: supplementalDatumTestKeyOutput(t, hash),
		}}).Build()
	}

	require.NoError(t, UtxoValidateSupplementalDatums(
		tx, 0, stateWith(&datumHash), dijkstraGuardTestPParams(),
	))

	// Negative: the reference input UTxO carries no datum hash, so nothing
	// on any level justifies the witness datum.
	err = UtxoValidateSupplementalDatums(
		tx, 0, stateWith(nil), dijkstraGuardTestPParams(),
	)
	var notAllowed conway.NotAllowedSupplementalDatumsError
	require.ErrorAs(t, err, &notAllowed)
	require.Equal(t, []common.Blake2b256{datumHash}, notAllowed.DatumHashes)

	other := common.Blake2b256{0x01}
	err = UtxoValidateSupplementalDatums(
		tx, 0, stateWith(&other), dijkstraGuardTestPParams(),
	)
	require.ErrorAs(t, err, &notAllowed)
	require.Equal(t, []common.Blake2b256{datumHash}, notAllowed.DatumHashes)
}

func supplementalDatumTestTx(
	t *testing.T,
	datum common.Datum,
) (*DijkstraTransaction, common.Blake2b256) {
	t.Helper()
	hash := datum.Hash()
	subTx := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxOutputs: []DijkstraTransactionOutput{{
				Output: supplementalDatumTestKeyOutput(t, &hash),
			}},
		},
	}
	return &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{subTx},
				true,
			),
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsPlutusData: cbor.NewSetType([]common.Datum{datum}, true),
		},
		TxIsValid: true,
	}, hash
}

// A sub-transaction's output datum hash also justifies a top-level witness
// datum (allSizedOutputsTxBodyF of the sub-transaction body).
func TestSupplementalDatumsAllowsSubtransactionOutputDatumAtTopLevel(
	t *testing.T,
) {
	datum := common.Datum{Data: data.NewInteger(big.NewInt(7))}
	tx, _ := supplementalDatumTestTx(t, datum)
	require.NoError(t, UtxoValidateSupplementalDatums(
		tx, 0, mockledger.NewLedgerStateBuilder().Build(),
		dijkstraGuardTestPParams(),
	))
}

// Control: a sub-transaction level is checked against its own body only, so
// a datum in a sub-transaction witness set is not justified by the top-level
// body or by a sibling sub-transaction.
func TestSupplementalDatumsSubtransactionLevelUsesOwnBodyOnly(t *testing.T) {
	datum := common.Datum{Data: data.NewInteger(big.NewInt(8))}
	hash := datum.Hash()
	withDatum := DijkstraSubTransaction{
		WitnessSet: DijkstraTransactionWitnessSet{
			WsPlutusData: cbor.NewSetType([]common.Datum{datum}, true),
		},
	}
	sibling := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxOutputs: []DijkstraTransactionOutput{{
				Output: supplementalDatumTestKeyOutput(t, &hash),
			}},
		},
	}
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxOutputs: []DijkstraTransactionOutput{{
				Output: supplementalDatumTestKeyOutput(t, &hash),
			}},
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{withDatum, sibling},
				true,
			),
		},
		TxIsValid: true,
	}
	err := UtxoValidateSupplementalDatums(
		tx, 0, mockledger.NewLedgerStateBuilder().Build(),
		dijkstraGuardTestPParams(),
	)
	var notAllowed conway.NotAllowedSupplementalDatumsError
	require.ErrorAs(t, err, &notAllowed)
	require.Equal(t, []common.Blake2b256{hash}, notAllowed.DatumHashes)
}

// In Plutus legacy mode (the top-level body needs a Plutus V1-V3 script),
// Dijkstra Utxow drops the sub-transactions before missingRequiredDatums, so
// a sub-transaction output no longer justifies a top-level witness datum.
func TestSupplementalDatumsLegacyModeIgnoresSubtransactionBodies(
	t *testing.T,
) {
	v3 := common.PlutusV3Script{0x23}
	scriptInput, scriptUtxo := dijkstraScriptLockedInput(t, v3, 970)
	datum := common.Datum{Data: data.NewInteger(big.NewInt(9))}
	tx, hash := supplementalDatumTestTx(t, datum)
	tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
		[]shelley.ShelleyTransactionInput{scriptInput},
	)
	tx.WitnessSet.WsPlutusV3Scripts = cbor.NewSetType(
		[]common.PlutusV3Script{v3},
		true,
	)
	state := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{scriptUtxo}).
		Build()
	err := UtxoValidateSupplementalDatums(
		tx, 0, state, dijkstraGuardTestPParams(),
	)
	var notAllowed conway.NotAllowedSupplementalDatumsError
	require.ErrorAs(t, err, &notAllowed)
	require.Equal(t, []common.Blake2b256{hash}, notAllowed.DatumHashes)
}

// Legacy mode is decided by the top-level body's own needed scripts
// (dsattPlutusLegacyMode in cardano-ledger Dijkstra.hs). A Plutus V3 script
// needed only by a sub-transaction does not switch it on.
func TestSupplementalDatumsSubtransactionPlutusDoesNotEnableLegacyMode(
	t *testing.T,
) {
	v3 := common.PlutusV3Script{0x24}
	scriptInput, scriptUtxo := dijkstraScriptLockedInput(t, v3, 971)
	datum := common.Datum{Data: data.NewInteger(big.NewInt(10))}
	tx, _ := supplementalDatumTestTx(t, datum)
	subTxs := tx.Body.TxSubTransactions.Items()
	subTxs[0].Body.TxInputs = conway.NewConwayTransactionInputSet(
		[]shelley.ShelleyTransactionInput{scriptInput},
	)
	subTxs[0].WitnessSet.WsPlutusV3Scripts = cbor.NewSetType(
		[]common.PlutusV3Script{v3},
		true,
	)
	tx.Body.TxSubTransactions = cbor.NewSetType(subTxs, true)
	state := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{scriptUtxo}).
		Build()
	require.NoError(t, UtxoValidateSupplementalDatums(
		tx, 0, state, dijkstraGuardTestPParams(),
	))
}
