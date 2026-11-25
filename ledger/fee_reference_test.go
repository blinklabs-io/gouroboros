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
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The transactions and protocol parameters below are cardano-ledger's own
// golden files (see testdata/cardano-ledger/README.md). The reference ships no
// expected fee, size or minimum-coin values for them, so every expectation is
// computed here from the raw CBOR with the reference formula named at each
// test, never from gOuroboros's own result.

const ledgerGoldenDir = "testdata/cardano-ledger"

func readLedgerGolden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(ledgerGoldenDir, name))
	require.NoError(t, err)
	return data
}

// goldenPParams holds the fields of a golden pparams.json that the fee and
// minimum-coin formulas read. Numbers stay as json.Number so a decimal price
// becomes the exact rational of its printed digits.
type goldenPParams struct {
	TxFeePerByte       uint64 `json:"txFeePerByte"`
	TxFeeFixed         uint64 `json:"txFeeFixed"`
	MinUTxOValue       uint64 `json:"minUTxOValue"`
	UtxoCostPerByte    uint64 `json:"utxoCostPerByte"`
	ExecutionUnitPrice struct {
		PriceMemory json.Number `json:"priceMemory"`
		PrMem       json.Number `json:"prMem"`
		PriceSteps  json.Number `json:"priceSteps"`
	} `json:"executionUnitPrices"`
	MinFeeRefScriptCostPerByte json.Number `json:"minFeeRefScriptCostPerByte"`
}

func readGoldenPParams(t *testing.T, era string) goldenPParams {
	t.Helper()
	var ret goldenPParams
	require.NoError(
		t,
		json.Unmarshal(readLedgerGolden(t, era+"-pparams.json"), &ret),
	)
	return ret
}

func goldenRat(t *testing.T, n json.Number) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(n.String())
	require.True(t, ok, "golden number %q", n)
	return r
}

func (g goldenPParams) prices(t *testing.T) (*big.Rat, *big.Rat) {
	mem := g.ExecutionUnitPrice.PriceMemory
	if mem == "" {
		mem = g.ExecutionUnitPrice.PrMem
	}
	return goldenRat(t, mem), goldenRat(t, g.ExecutionUnitPrice.PriceSteps)
}

func (g goldenPParams) exUnitPrice(t *testing.T) common.ExUnitPrice {
	mem, steps := g.prices(t)
	return common.ExUnitPrice{
		MemPrice:  &cbor.Rat{Rat: mem},
		StepPrice: &cbor.Rat{Rat: steps},
	}
}

// goldenTxParts splits a golden transaction into its top-level components
// without going through gOuroboros's transaction decoders.
func goldenTxParts(t *testing.T, txCbor []byte) []cbor.RawMessage {
	t.Helper()
	var parts []cbor.RawMessage
	_, err := cbor.Decode(txCbor, &parts)
	require.NoError(t, err)
	if len(parts) < 3 {
		t.Fatalf("transaction has %d components, want at least 3", len(parts))
	}
	return parts
}

// referenceTxSize is sizeTxF. Every era from Shelley to Conway serialises
// the body, witnesses and auxiliary data under a one-byte, three-element
// array header, whatever framing was received, and Alonzo to Conway leave
// IsValid out (toCBORForSizeComputation in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Tx.hs and
// eras/alonzo/impl/src/Cardano/Ledger/Alonzo/Tx.hs).
func referenceTxSize(t *testing.T, txCbor []byte) int {
	t.Helper()
	parts := goldenTxParts(t, txCbor)
	switch len(parts) {
	case 3:
		return 1 + len(parts[0]) + len(parts[1]) + len(parts[2])
	case 4:
		return 1 + len(parts[0]) + len(parts[1]) + len(parts[3])
	}
	t.Fatalf("unexpected transaction arity %d", len(parts))
	return 0
}

// referenceTotalExUnits is getTotalExUnits: the sum of every redeemer's
// budget, read from witness-set key 5 in either the array or the Conway map
// encoding.
func referenceTotalExUnits(t *testing.T, txCbor []byte) (uint64, uint64) {
	t.Helper()
	parts := goldenTxParts(t, txCbor)
	var wits map[uint64]cbor.RawMessage
	_, err := cbor.Decode(parts[1], &wits)
	require.NoError(t, err)
	raw, ok := wits[5]
	if !ok {
		return 0, 0
	}
	var mem, steps uint64
	add := func(exUnits cbor.RawMessage) {
		var units [2]uint64
		_, err := cbor.Decode(exUnits, &units)
		require.NoError(t, err)
		mem += units[0]
		steps += units[1]
	}
	dec, err := cbor.NewStreamDecoder(raw)
	require.NoError(t, err)
	if raw[0]>>5 == 4 {
		var entries [][]cbor.RawMessage
		_, err := cbor.Decode(raw, &entries)
		require.NoError(t, err)
		for _, entry := range entries {
			require.Len(t, entry, 4)
			add(entry[3])
		}
		return mem, steps
	}
	count, _, _, err := dec.DecodeMapHeader()
	require.NoError(t, err)
	for range count {
		_, _, err := dec.Skip()
		require.NoError(t, err)
		var value []cbor.RawMessage
		_, _, err = dec.Decode(&value)
		require.NoError(t, err)
		require.Len(t, value, 2)
		add(value[1])
	}
	return mem, steps
}

// referenceMinFee is alonzoMinFeeTx (eras/alonzo/impl/src/Cardano/Ledger/
// Alonzo/Tx.hs): size * txFeePerByte + txFeeFixed + txscriptfee, where
// txscriptfee (libs/cardano-ledger-core/src/Cardano/Ledger/Plutus/
// ExUnits.hs) rounds the priced total budget up once. Shelley to Mary have no
// script fee.
func referenceMinFee(
	t *testing.T,
	txCbor []byte,
	pp goldenPParams,
	withScriptFee bool,
) uint64 {
	t.Helper()
	fee := new(big.Int).SetUint64(pp.TxFeePerByte)
	fee.Mul(fee, big.NewInt(int64(referenceTxSize(t, txCbor))))
	fee.Add(fee, new(big.Int).SetUint64(pp.TxFeeFixed))
	if withScriptFee {
		mem, steps := referenceTotalExUnits(t, txCbor)
		memPrice, stepPrice := pp.prices(t)
		script := new(big.Rat).Mul(
			new(big.Rat).SetInt(new(big.Int).SetUint64(mem)),
			memPrice,
		)
		script.Add(script, new(big.Rat).Mul(
			new(big.Rat).SetInt(new(big.Int).SetUint64(steps)),
			stepPrice,
		))
		ceil := new(big.Int).Quo(script.Num(), script.Denom())
		if new(big.Int).Rem(script.Num(), script.Denom()).Sign() != 0 {
			ceil.Add(ceil, big.NewInt(1))
		}
		fee.Add(fee, ceil)
	}
	require.True(t, fee.IsUint64())
	return fee.Uint64()
}

func goldenOutputs(t *testing.T, txCbor []byte) []cbor.RawMessage {
	t.Helper()
	parts := goldenTxParts(t, txCbor)
	var body map[uint64]cbor.RawMessage
	_, err := cbor.Decode(parts[0], &body)
	require.NoError(t, err)
	var outputs []cbor.RawMessage
	_, err = cbor.Decode(body[1], &outputs)
	require.NoError(t, err)
	return outputs
}

func TestReferenceTxSizeOnLedgerGoldens(t *testing.T) {
	cases := []struct {
		era    string
		decode func([]byte) (common.Transaction, error)
	}{
		{"shelley", func(b []byte) (common.Transaction, error) {
			return shelley.NewShelleyTransactionFromCbor(b)
		}},
		{"allegra", func(b []byte) (common.Transaction, error) {
			return allegra.NewAllegraTransactionFromCbor(b)
		}},
		{"mary", func(b []byte) (common.Transaction, error) {
			return mary.NewMaryTransactionFromCbor(b)
		}},
		{"alonzo", func(b []byte) (common.Transaction, error) {
			return alonzo.NewAlonzoTransactionFromCbor(b)
		}},
		{"babbage", func(b []byte) (common.Transaction, error) {
			return babbage.NewBabbageTransactionFromCbor(b)
		}},
		{"conway", func(b []byte) (common.Transaction, error) {
			return conway.NewConwayTransactionFromCbor(b)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.era, func(t *testing.T) {
			txCbor := readLedgerGolden(t, tc.era+"-tx.cbor")
			tx, err := tc.decode(txCbor)
			require.NoError(t, err)
			size, err := common.TxSize(tx)
			require.NoError(t, err)
			assert.Equal(t, referenceTxSize(t, txCbor), size)
		})
	}
}

// TestReferenceMaxTxSizeBoundaryOnLedgerGoldens checks every era's size rule
// at the reference size: validateMaxTxSizeUTxO
// (eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Utxo.hs) accepts
// txsize <= maxTxSize.
func TestReferenceMaxTxSizeBoundaryOnLedgerGoldens(t *testing.T) {
	type rule func(
		common.Transaction, uint64, common.LedgerState,
		common.ProtocolParameters,
	) error
	cases := []struct {
		era    string
		decode func([]byte) (common.Transaction, error)
		pp     func(uint) common.ProtocolParameters
		rule   rule
	}{
		{
			"shelley",
			func(b []byte) (common.Transaction, error) {
				return shelley.NewShelleyTransactionFromCbor(b)
			},
			func(m uint) common.ProtocolParameters {
				return &shelley.ShelleyProtocolParameters{MaxTxSize: m}
			},
			shelley.UtxoValidateMaxTxSizeUtxo,
		},
		{
			"allegra",
			func(b []byte) (common.Transaction, error) {
				return allegra.NewAllegraTransactionFromCbor(b)
			},
			func(m uint) common.ProtocolParameters {
				return &allegra.AllegraProtocolParameters{MaxTxSize: m}
			},
			allegra.UtxoValidateMaxTxSizeUtxo,
		},
		{
			"mary",
			func(b []byte) (common.Transaction, error) {
				return mary.NewMaryTransactionFromCbor(b)
			},
			func(m uint) common.ProtocolParameters {
				return &mary.MaryProtocolParameters{MaxTxSize: m}
			},
			mary.UtxoValidateMaxTxSizeUtxo,
		},
		{
			"alonzo",
			func(b []byte) (common.Transaction, error) {
				return alonzo.NewAlonzoTransactionFromCbor(b)
			},
			func(m uint) common.ProtocolParameters {
				return &alonzo.AlonzoProtocolParameters{MaxTxSize: m}
			},
			alonzo.UtxoValidateMaxTxSizeUtxo,
		},
		{
			"babbage",
			func(b []byte) (common.Transaction, error) {
				return babbage.NewBabbageTransactionFromCbor(b)
			},
			func(m uint) common.ProtocolParameters {
				return &babbage.BabbageProtocolParameters{MaxTxSize: m}
			},
			babbage.UtxoValidateMaxTxSizeUtxo,
		},
		{
			"conway",
			func(b []byte) (common.Transaction, error) {
				return conway.NewConwayTransactionFromCbor(b)
			},
			func(m uint) common.ProtocolParameters {
				return &conway.ConwayProtocolParameters{MaxTxSize: m}
			},
			conway.UtxoValidateMaxTxSizeUtxo,
		},
	}
	for _, tc := range cases {
		t.Run(tc.era, func(t *testing.T) {
			txCbor := readLedgerGolden(t, tc.era+"-tx.cbor")
			tx, err := tc.decode(txCbor)
			require.NoError(t, err)
			size := uint(referenceTxSize(t, txCbor)) // #nosec G115
			require.NoError(t, tc.rule(tx, 0, nil, tc.pp(size)))
			var sizeErr shelley.MaxTxSizeUtxoError
			require.ErrorAs(t, tc.rule(tx, 0, nil, tc.pp(size-1)), &sizeErr)
			assert.Equal(t, size, sizeErr.TxSize)
		})
	}
}

func TestReferenceMinFeeOnLedgerGoldens(t *testing.T) {
	shelleyPP := readGoldenPParams(t, "shelley")
	t.Run("shelley", func(t *testing.T) {
		txCbor := readLedgerGolden(t, "shelley-tx.cbor")
		tx, err := shelley.NewShelleyTransactionFromCbor(txCbor)
		require.NoError(t, err)
		got, err := shelley.MinFeeTx(tx, &shelley.ShelleyProtocolParameters{
			MinFeeA: uint(shelleyPP.TxFeePerByte),
			MinFeeB: uint(shelleyPP.TxFeeFixed),
		})
		require.NoError(t, err)
		assert.Equal(t, referenceMinFee(t, txCbor, shelleyPP, false), got)
	})
	t.Run("allegra", func(t *testing.T) {
		txCbor := readLedgerGolden(t, "allegra-tx.cbor")
		tx, err := allegra.NewAllegraTransactionFromCbor(txCbor)
		require.NoError(t, err)
		got, err := shelley.MinFeeTx(tx, &allegra.AllegraProtocolParameters{
			MinFeeA: uint(shelleyPP.TxFeePerByte),
			MinFeeB: uint(shelleyPP.TxFeeFixed),
		})
		require.NoError(t, err)
		assert.Equal(t, referenceMinFee(t, txCbor, shelleyPP, false), got)
	})
	t.Run("mary", func(t *testing.T) {
		txCbor := readLedgerGolden(t, "mary-tx.cbor")
		tx, err := mary.NewMaryTransactionFromCbor(txCbor)
		require.NoError(t, err)
		got, err := mary.MinFeeTx(tx, &mary.MaryProtocolParameters{
			MinFeeA: uint(shelleyPP.TxFeePerByte),
			MinFeeB: uint(shelleyPP.TxFeeFixed),
		})
		require.NoError(t, err)
		assert.Equal(t, referenceMinFee(t, txCbor, shelleyPP, false), got)
	})
	t.Run("alonzo", func(t *testing.T) {
		pp := readGoldenPParams(t, "alonzo")
		txCbor := readLedgerGolden(t, "alonzo-tx.cbor")
		mem, steps := referenceTotalExUnits(t, txCbor)
		require.NotZero(t, mem+steps, "golden must carry a script budget")
		tx, err := alonzo.NewAlonzoTransactionFromCbor(txCbor)
		require.NoError(t, err)
		got, err := alonzo.MinFeeTx(tx, &alonzo.AlonzoProtocolParameters{
			MinFeeA:        uint(pp.TxFeePerByte),
			MinFeeB:        uint(pp.TxFeeFixed),
			ExecutionCosts: pp.exUnitPrice(t),
		})
		require.NoError(t, err)
		assert.Equal(t, referenceMinFee(t, txCbor, pp, true), got)
	})
	t.Run("babbage", func(t *testing.T) {
		pp := readGoldenPParams(t, "babbage")
		txCbor := readLedgerGolden(t, "babbage-tx.cbor")
		tx, err := babbage.NewBabbageTransactionFromCbor(txCbor)
		require.NoError(t, err)
		params := &babbage.BabbageProtocolParameters{
			MinFeeA:        uint(pp.TxFeePerByte),
			MinFeeB:        uint(pp.TxFeeFixed),
			ExecutionCosts: pp.exUnitPrice(t),
		}
		// The golden step price (about 9.5e18 per step) puts the reference
		// script fee above any Word64 fee field, so the transaction cannot
		// pay it; gOuroboros reports the overflow instead of a number.
		_, err = babbage.MinFeeTx(tx, params)
		require.ErrorContains(t, err, "overflow")
		params.ExecutionCosts = readGoldenPParams(t, "alonzo").exUnitPrice(t)
		pp.ExecutionUnitPrice = readGoldenPParams(
			t,
			"alonzo",
		).ExecutionUnitPrice
		got, err := babbage.MinFeeTx(tx, params)
		require.NoError(t, err)
		assert.Equal(t, referenceMinFee(t, txCbor, pp, true), got)
	})
	t.Run("conway", func(t *testing.T) {
		pp := readGoldenPParams(t, "conway")
		txCbor := readLedgerGolden(t, "conway-tx.cbor")
		tx, err := conway.NewConwayTransactionFromCbor(txCbor)
		require.NoError(t, err)
		refCost := goldenRat(t, pp.MinFeeRefScriptCostPerByte)
		conwayPP := &conway.ConwayProtocolParameters{
			MinFeeA:                    uint(pp.TxFeePerByte),
			MinFeeB:                    uint(pp.TxFeeFixed),
			ExecutionCosts:             pp.exUnitPrice(t),
			MinFeeRefScriptCostPerByte: &cbor.Rat{Rat: refCost},
		}
		base := referenceMinFee(t, txCbor, pp, true)
		got, err := conway.MinFeeTx(tx, conwayPP)
		require.NoError(t, err)
		assert.Equal(t, base, got)
		// getConwayMinFeeTx adds tierRefScriptFee with Conway's fixed
		// multiplier 1.2 and stride 25600 (ppRefScriptCostMultiplierG and
		// ppRefScriptCostStrideG in eras/conway/impl/src/Cardano/Ledger/
		// Conway/PParams.hs).
		for _, refSize := range []uint64{0, 1, 25_599, 25_600, 25_601, 204_800} {
			got, err := conway.MinFeeTxWithRefScriptSize(tx, conwayPP, refSize)
			require.NoError(t, err)
			assert.Equal(
				t,
				base+referenceTierRefScriptFee(
					big.NewRat(6, 5), 25_600, refCost, refSize,
				),
				got,
				"reference script size %d", refSize,
			)
		}
	})
}

// referenceTierRefScriptFee is tierRefScriptFee in
// eras/conway/impl/src/Cardano/Ledger/Conway/Tx.hs: each full stride is
// priced at the current tier price, the price is multiplied after every full
// stride, and the exact rational total is floored once.
func referenceTierRefScriptFee(
	multiplier *big.Rat,
	stride uint64,
	base *big.Rat,
	size uint64,
) uint64 {
	acc := new(big.Rat)
	price := new(big.Rat).Set(base)
	n := size
	for n >= stride {
		acc.Add(acc, new(big.Rat).Mul(
			new(big.Rat).SetInt(new(big.Int).SetUint64(stride)),
			price,
		))
		price.Mul(price, multiplier)
		n -= stride
	}
	acc.Add(acc, new(big.Rat).Mul(
		new(big.Rat).SetInt(new(big.Int).SetUint64(n)),
		price,
	))
	return new(big.Int).Quo(acc.Num(), acc.Denom()).Uint64()
}

// TestReferenceTierRefScriptFeeGolden replays the unit test of
// tierRefScriptFee in eras/conway/impl/test/Main.hs, which ships its expected
// values: multiplier 1.5, stride 25600, base price 15, sizes 0 to 204800 in
// whole strides.
func TestReferenceTierRefScriptFeeGolden(t *testing.T) {
	want := []uint64{
		0, 384000, 960000, 1824000, 3120000, 5064000, 7980000,
		12354000, 18915000,
	}
	for i, expected := range want {
		size := uint64(i) * 25_600 // #nosec G115
		got, err := conway.CalculateRefScriptFee(
			size,
			&cbor.Rat{Rat: big.NewRat(15, 1)},
			25_600,
			&cbor.Rat{Rat: big.NewRat(3, 2)},
		)
		require.NoError(t, err)
		assert.Equal(t, expected, got, "size %d", size)
	}
	// The same file states that a multiplier of 1 makes the fee linear:
	// floor(size * base).
	base := big.NewRat(1234567, 1000)
	for _, size := range []uint64{0, 1, 25_599, 25_600, 77_777} {
		got, err := conway.CalculateRefScriptFee(
			size,
			&cbor.Rat{Rat: base},
			25_600,
			&cbor.Rat{Rat: big.NewRat(1, 1)},
		)
		require.NoError(t, err)
		linear := new(big.Rat).Mul(
			new(big.Rat).SetInt(new(big.Int).SetUint64(size)),
			base,
		)
		assert.Equal(
			t,
			new(big.Int).Quo(linear.Num(), linear.Denom()).Uint64(),
			got,
			"size %d", size,
		)
	}
}

// TestReferenceMinCoinOnLedgerGoldens pins each era's minimum coin per output
// of the golden transactions.
func TestReferenceMinCoinOnLedgerGoldens(t *testing.T) {
	t.Run("shelley", func(t *testing.T) {
		// getMinCoinTxOut pp _ = minUTxOValue
		// (eras/shelley/impl/src/Cardano/Ledger/Shelley/TxOut.hs).
		pp := readGoldenPParams(t, "shelley")
		txCbor := readLedgerGolden(t, "shelley-tx.cbor")
		tx, err := shelley.NewShelleyTransactionFromCbor(txCbor)
		require.NoError(t, err)
		got, err := shelley.MinCoinTxOut(
			tx,
			&shelley.ShelleyProtocolParameters{
				MinUtxoValue: uint(pp.MinUTxOValue),
			},
		)
		require.NoError(t, err)
		assert.Equal(t, pp.MinUTxOValue, got)
	})
	t.Run("mary", func(t *testing.T) {
		// Allegra and Mary flag an output when
		// Val.pointwise (<) v (inject (getMinCoinTxOut pp txOut))
		// (validateOutputTooSmallUTxO in eras/allegra/impl/src/Cardano/
		// Ledger/Allegra/Rules/Utxo.hs). A positive token quantity is never
		// below zero, so an output carrying tokens is never too small,
		// whatever scaledMinDeposit returns; an ada-only output is compared
		// on coin against minUTxOValue.
		txCbor := readLedgerGolden(t, "mary-tx.cbor")
		tx, err := mary.NewMaryTransactionFromCbor(txCbor)
		require.NoError(t, err)
		var maxAdaOnly uint64
		var adaOnly []int
		for i, out := range goldenOutputs(t, txCbor) {
			var coin uint64
			if _, err := cbor.Decode(maryOutputValue(t, out), &coin); err == nil {
				adaOnly = append(adaOnly, i)
				maxAdaOnly = max(maxAdaOnly, coin)
			}
		}
		require.NotEmpty(t, adaOnly)
		require.Less(t, len(adaOnly), len(tx.Outputs()))
		pp := &mary.MaryProtocolParameters{MinUtxoValue: uint(maxAdaOnly)}
		require.NoError(t, mary.UtxoValidateOutputTooSmallUtxo(tx, 0, nil, pp))
		pp.MinUtxoValue = uint(maxAdaOnly + 1)
		err = mary.UtxoValidateOutputTooSmallUtxo(tx, 0, nil, pp)
		var tooSmall shelley.OutputTooSmallUtxoError
		require.ErrorAs(t, err, &tooSmall)
		assert.Len(t, tooSmall.Outputs, len(adaOnly))
		for _, out := range tooSmall.Outputs {
			assert.Nil(t, out.Assets())
		}
	})
	t.Run("alonzo", func(t *testing.T) {
		// getMinCoinTxOut = utxoEntrySize txOut * coinsPerUTxOWord
		// (eras/alonzo/impl/src/Cardano/Ledger/Alonzo/TxOut.hs).
		pp := readGoldenPParams(t, "alonzo")
		txCbor := readLedgerGolden(t, "alonzo-tx.cbor")
		tx, err := alonzo.NewAlonzoTransactionFromCbor(txCbor)
		require.NoError(t, err)
		params := &alonzo.AlonzoProtocolParameters{
			AdaPerUtxoByte: pp.UtxoCostPerByte,
		}
		raw := goldenOutputs(t, txCbor)
		require.Len(t, raw, len(tx.Outputs()))
		for i, out := range tx.Outputs() {
			got, err := alonzo.MinCoinTxOut(out, params)
			require.NoError(t, err)
			assert.Equal(
				t,
				referenceAlonzoUtxoEntrySize(t, raw[i])*pp.UtxoCostPerByte,
				got,
				"output %d", i,
			)
		}
	})
	t.Run("alonzo word boundary", func(t *testing.T) {
		// One policy and one asset fill representationSize to exactly
		// 12 + 28 + len(name) bytes, so an 8-byte name lands on a word
		// boundary (48 bytes, 6 words) and a 9-byte name rolls into the
		// next word. A per-asset or per-policy cost off by one byte moves
		// the 8-byte case across the boundary.
		const coinsPerWord = 4310
		policy := make([]byte, 28)
		addr := append([]byte{0x61}, make([]byte, 28)...)
		for _, tc := range []struct {
			name  string
			words uint64
		}{
			{"12345678", 27 + 6 + 6},
			{"123456789", 27 + 7 + 6},
		} {
			outCbor, err := cbor.Encode([]any{
				addr,
				[]any{
					uint64(1),
					map[cbor.ByteString]map[cbor.ByteString]uint64{
						cbor.NewByteString(policy): {
							cbor.NewByteString([]byte(tc.name)): 1,
						},
					},
				},
			})
			require.NoError(t, err)
			require.Equal(
				t,
				tc.words,
				referenceAlonzoUtxoEntrySize(t, outCbor),
			)
			out, err := alonzo.NewAlonzoTransactionOutputFromCbor(outCbor)
			require.NoError(t, err)
			got, err := alonzo.MinCoinTxOut(
				out,
				&alonzo.AlonzoProtocolParameters{AdaPerUtxoByte: coinsPerWord},
			)
			require.NoError(t, err)
			assert.Equal(t, tc.words*coinsPerWord, got, "name %q", tc.name)
		}
	})
	babbageFamily := []struct {
		era    string
		minCon func(*testing.T, []byte, uint64) []uint64
	}{
		{"babbage", func(t *testing.T, b []byte, cpb uint64) []uint64 {
			tx, err := babbage.NewBabbageTransactionFromCbor(b)
			require.NoError(t, err)
			pp := &babbage.BabbageProtocolParameters{AdaPerUtxoByte: cpb}
			var ret []uint64
			for _, out := range tx.Outputs() {
				v, err := babbage.MinCoinTxOut(out, pp)
				require.NoError(t, err)
				ret = append(ret, v)
			}
			return ret
		}},
		{"conway", func(t *testing.T, b []byte, cpb uint64) []uint64 {
			tx, err := conway.NewConwayTransactionFromCbor(b)
			require.NoError(t, err)
			pp := &conway.ConwayProtocolParameters{AdaPerUtxoByte: cpb}
			var ret []uint64
			for _, out := range tx.Outputs() {
				v, err := conway.MinCoinTxOut(out, pp)
				require.NoError(t, err)
				ret = append(ret, v)
			}
			return ret
		}},
	}
	for _, tc := range babbageFamily {
		t.Run(tc.era, func(t *testing.T) {
			// babbageMinUTxOValue: (160 + serialized output size) *
			// coinsPerUTxOByte (eras/babbage/impl/src/Cardano/Ledger/
			// Babbage/TxOut.hs), measured on the received output bytes.
			pp := readGoldenPParams(t, tc.era)
			txCbor := readLedgerGolden(t, tc.era+"-tx.cbor")
			raw := goldenOutputs(t, txCbor)
			got := tc.minCon(t, txCbor, pp.UtxoCostPerByte)
			require.Len(t, got, len(raw))
			for i, out := range raw {
				assert.Equal(
					t,
					uint64(160+len(out))*pp.UtxoCostPerByte,
					got[i],
					"output %d", i,
				)
			}
		})
	}
}

func maryOutputValue(t *testing.T, out cbor.RawMessage) cbor.RawMessage {
	t.Helper()
	var fields []cbor.RawMessage
	_, err := cbor.Decode(out, &fields)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(fields), 2)
	return fields[1]
}

// referenceAlonzoUtxoEntrySize is utxoEntrySize
// (eras/alonzo/impl/src/Cardano/Ledger/Alonzo/TxOut.hs): 27 words, plus
// Val.size of the value, plus 10 for a datum hash. Val.size is 2 for an
// ada-only value and otherwise roundupBytesToWords(representationSize) + 6
// (eras/mary/impl/src/Cardano/Ledger/Mary/Value.hs), where
// representationSize = 12 per asset + 28 per distinct policy + the bytes of
// each distinct asset name across all policies.
func referenceAlonzoUtxoEntrySize(t *testing.T, out cbor.RawMessage) uint64 {
	t.Helper()
	var fields []cbor.RawMessage
	_, err := cbor.Decode(out, &fields)
	require.NoError(t, err)
	if len(fields) < 2 {
		t.Fatalf("output has %d fields, want at least 2", len(fields))
	}
	size := uint64(27)
	if len(fields) == 3 {
		size += 10
	}
	var coin uint64
	if _, err := cbor.Decode(fields[1], &coin); err == nil {
		return size + 2
	}
	var value []cbor.RawMessage
	_, err = cbor.Decode(fields[1], &value)
	require.NoError(t, err)
	if len(value) != 2 {
		t.Fatalf("value has %d elements, want 2", len(value))
	}
	var assets map[cbor.ByteString]map[cbor.ByteString]uint64
	_, err = cbor.Decode(value[1], &assets)
	require.NoError(t, err)
	numAssets, names := 0, map[string]struct{}{}
	for _, byName := range assets {
		for name, quantity := range byName {
			require.NotZero(t, quantity)
			numAssets++
			names[string(name.Bytes())] = struct{}{}
		}
	}
	repSize := 12*numAssets + 28*len(assets)
	for name := range names {
		repSize += len(name)
	}
	return size + uint64((repSize+7)/8+6) // #nosec G115
}
