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

// Tests that every era's maxTxSize rule measures a transaction the same way
// the fee rule does. They live in package ledger_test rather than in each
// era's own package because the rule is duplicated per era and the point of
// the test is that all of them agree.

package ledger_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMaxTxSize uint = 16384

// segmentedTxCbor returns a transaction CBOR blob of exactly size bytes whose
// first byte is a four-element array header.
//
// That is the shape gouroboros reconstructs for an Alonzo-and-later
// transaction read out of a block: the block stores bodies, witness sets,
// IsValid flags and auxiliary data in four parallel arrays, and rebuilding a
// standalone transaction re-attaches the IsValid byte that was never part of
// the transaction the chain measured.
func segmentedTxCbor(size int) []byte {
	b := make([]byte, size)
	b[0] = 0x84 // array(4)
	return b
}

// unsegmentedTxCbor is the pre-Alonzo shape: three elements, no IsValid flag,
// so nothing is re-attached and nothing may be subtracted.
func unsegmentedTxCbor(size int) []byte {
	b := make([]byte, size)
	b[0] = 0x83 // array(3)
	return b
}

type maxTxSizeEra struct {
	name     string
	newTx    func(cborData []byte) common.Transaction
	pparams  func(maxTxSize uint) common.ProtocolParameters
	validate func(common.Transaction, uint64, common.LedgerState, common.ProtocolParameters) error
	// blockTxCarriesIsValid is true for the eras whose in-block representation
	// keeps IsValid in a separate parallel array, so reconstructing a
	// standalone transaction re-attaches it as a fourth element.
	//
	// Dijkstra is false. Its transactions are three components in a block —
	// newDijkstraTransactionFromCborComponents rejects a four-component
	// transaction outright with "dijkstra transactions in blocks cannot
	// include is_valid" — so nothing is re-attached and nothing is
	// subtracted. It accepts four components only for a standalone
	// transaction, which is not what maxTxSize validates.
	blockTxCarriesIsValid bool
}

func maxTxSizeEras() []maxTxSizeEra {
	return []maxTxSizeEra{
		{
			name: "shelley",
			newTx: func(c []byte) common.Transaction {
				tx := &shelley.ShelleyTransaction{}
				tx.SetCbor(c)
				return tx
			},
			pparams: func(m uint) common.ProtocolParameters {
				return &shelley.ShelleyProtocolParameters{MaxTxSize: m}
			},
			validate:              shelley.UtxoValidateMaxTxSizeUtxo,
			blockTxCarriesIsValid: false,
		},
		{
			name: "allegra",
			newTx: func(c []byte) common.Transaction {
				tx := &allegra.AllegraTransaction{}
				tx.SetCbor(c)
				return tx
			},
			pparams: func(m uint) common.ProtocolParameters {
				return &allegra.AllegraProtocolParameters{MaxTxSize: m}
			},
			validate:              allegra.UtxoValidateMaxTxSizeUtxo,
			blockTxCarriesIsValid: false,
		},
		{
			name: "mary",
			newTx: func(c []byte) common.Transaction {
				tx := &mary.MaryTransaction{}
				tx.SetCbor(c)
				return tx
			},
			pparams: func(m uint) common.ProtocolParameters {
				return &mary.MaryProtocolParameters{MaxTxSize: m}
			},
			validate:              mary.UtxoValidateMaxTxSizeUtxo,
			blockTxCarriesIsValid: false,
		},
		{
			name: "alonzo",
			newTx: func(c []byte) common.Transaction {
				tx := &alonzo.AlonzoTransaction{}
				tx.SetCbor(c)
				return tx
			},
			pparams: func(m uint) common.ProtocolParameters {
				return &alonzo.AlonzoProtocolParameters{MaxTxSize: m}
			},
			validate:              alonzo.UtxoValidateMaxTxSizeUtxo,
			blockTxCarriesIsValid: true,
		},
		{
			name: "babbage",
			newTx: func(c []byte) common.Transaction {
				tx := &babbage.BabbageTransaction{}
				tx.SetCbor(c)
				return tx
			},
			pparams: func(m uint) common.ProtocolParameters {
				return &babbage.BabbageProtocolParameters{MaxTxSize: m}
			},
			validate:              babbage.UtxoValidateMaxTxSizeUtxo,
			blockTxCarriesIsValid: true,
		},
		{
			name: "conway",
			newTx: func(c []byte) common.Transaction {
				tx := &conway.ConwayTransaction{}
				tx.SetCbor(c)
				return tx
			},
			pparams: func(m uint) common.ProtocolParameters {
				return &conway.ConwayProtocolParameters{MaxTxSize: m}
			},
			validate:              conway.UtxoValidateMaxTxSizeUtxo,
			blockTxCarriesIsValid: true,
		},
		{
			name: "dijkstra",
			newTx: func(c []byte) common.Transaction {
				tx := &dijkstra.DijkstraTransaction{}
				tx.SetCbor(c)
				return tx
			},
			pparams: func(m uint) common.ProtocolParameters {
				pp := &dijkstra.DijkstraProtocolParameters{}
				pp.MaxTxSize = m
				return pp
			},
			validate:              dijkstra.UtxoValidateMaxTxSizeUtxo,
			blockTxCarriesIsValid: false,
		},
	}
}

// TestMaxTxSizeAcceptsATransactionExactlyAtTheLimit is the regression for the
// Preview replay wedge at slot 27745494 (epoch 321, Conway). Transaction
// ddd3f6b502c45d7c9f60f75c79148b70c0bae6e238aafafa1d24e9b7ed228d85 is exactly
// 16384 bytes on chain and reconstructs to 16385 with the IsValid byte
// re-attached. maxTxSize was 16384, so the node rejected a transaction the
// chain had accepted and stopped following the chain.
//
// It covers every era rather than only the ones that carry an IsValid byte.
//
// All seven eras changed here — six replaced an inline length computation with
// common.TxSize, and Allegra delegates to Shelley — so all seven need proof
// that a transaction exactly on the limit is still accepted. A validation
// change tested only by its rejections is how a false rejection ships.
func TestMaxTxSizeAcceptsATransactionExactlyAtTheLimit(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	for _, era := range maxTxSizeEras() {
		t.Run(era.name, func(t *testing.T) {
			// A transaction is exactly maxTxSize on chain. For an era whose
			// block keeps IsValid in a parallel array, reconstruction re-adds
			// one byte; for the others the on-wire bytes are the measure.
			raw := int(testMaxTxSize)
			body := unsegmentedTxCbor
			if era.blockTxCarriesIsValid {
				raw++
				body = segmentedTxCbor
			}
			tx := era.newTx(body(raw))
			err := era.validate(tx, 0, ls, era.pparams(testMaxTxSize))
			assert.NoError(t, err,
				"a transaction exactly at maxTxSize on chain must be accepted")
		})
	}
}

// TestMaxTxSizeAdjustmentFollowsTheEnvelopeNotTheEra pins that the IsValid byte
// is subtracted because the envelope has four components, not because the era
// is Alonzo or later.
//
// Dijkstra is the case that separates the two rules: it is an Alonzo-or-later
// era whose block transactions are three components —
// newDijkstraTransactionFromCborComponents rejects four outright for a block
// transaction — so an era-driven adjustment would wrongly shorten it.
func TestMaxTxSizeAdjustmentFollowsTheEnvelopeNotTheEra(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	for _, era := range maxTxSizeEras() {
		if !isAlonzoOrLater(era.name) {
			continue
		}
		t.Run(era.name, func(t *testing.T) {
			t.Run("three-component envelope is not adjusted", func(t *testing.T) {
				tx := era.newTx(unsegmentedTxCbor(int(testMaxTxSize) + 1))
				err := era.validate(tx, 0, ls, era.pparams(testMaxTxSize))
				require.Error(t, err,
					"nothing is re-attached to a three-component envelope, "+
						"so one byte over the limit is over the limit")
			})
			t.Run("four-component envelope is adjusted", func(t *testing.T) {
				tx := era.newTx(segmentedTxCbor(int(testMaxTxSize) + 1))
				err := era.validate(tx, 0, ls, era.pparams(testMaxTxSize))
				assert.NoError(t, err,
					"the fourth component is the IsValid byte and is not "+
						"part of the size the chain measured")
			})
		})
	}
}

func isAlonzoOrLater(name string) bool {
	switch name {
	case "alonzo", "babbage", "conway", "dijkstra":
		return true
	default:
		return false
	}
}

// TestMaxTxSizeStillRejectsOversize is the discrimination check. The fix must
// move the boundary by exactly the IsValid byte, not disable the rule.
func TestMaxTxSizeStillRejectsOversize(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	for _, era := range maxTxSizeEras() {
		t.Run(era.name, func(t *testing.T) {
			raw := int(testMaxTxSize) + 1
			if era.blockTxCarriesIsValid {
				// One byte past the limit once the IsValid byte is removed.
				raw = int(testMaxTxSize) + 2
			}
			body := unsegmentedTxCbor
			if era.blockTxCarriesIsValid {
				body = segmentedTxCbor
			}
			tx := era.newTx(body(raw))
			err := era.validate(tx, 0, ls, era.pparams(testMaxTxSize))
			require.Error(t, err, "a genuinely oversize transaction must fail")

			var sizeErr shelley.MaxTxSizeUtxoError
			require.ErrorAs(t, err, &sizeErr)
			assert.Equal(t, testMaxTxSize+1, sizeErr.TxSize,
				"the reported size must be the on-chain measure, not the "+
					"reconstructed length")
		})
	}
}

// TestMaxTxSizePreAlonzoUnchanged pins that the adjustment is confined to the
// eras that actually carry an IsValid flag. A pre-Alonzo transaction one byte
// over the limit is over the limit.
func TestMaxTxSizePreAlonzoUnchanged(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	for _, era := range maxTxSizeEras() {
		if era.blockTxCarriesIsValid {
			continue
		}
		t.Run(era.name, func(t *testing.T) {
			tx := era.newTx(unsegmentedTxCbor(int(testMaxTxSize) + 1))
			err := era.validate(tx, 0, ls, era.pparams(testMaxTxSize))
			require.Error(t, err)
			var sizeErr shelley.MaxTxSizeUtxoError
			require.ErrorAs(t, err, &sizeErr)
			assert.Equal(t, testMaxTxSize+1, sizeErr.TxSize)
		})
	}
}
