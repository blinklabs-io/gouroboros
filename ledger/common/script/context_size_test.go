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

package script_test

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
	"github.com/stretchr/testify/require"
)

// The reference implementation applies no node budget to the script context:
// the ledger supplies it as an evaluation argument, and the context's size
// follows the outputs an earlier transaction created, not the size of the
// transaction under validation. Each referenced output may carry an inline
// datum almost as large as the maximum transaction size, so a small
// transaction that references enough of them builds a context of more than a
// million data nodes.
const (
	largeContextRefInputs  = 70
	largeContextDatumNodes = 16_000
)

// largeContextDatum builds an inline datum holding n nodes.
func largeContextDatum(n int) *lcommon.Datum {
	items := make([]data.PlutusData, n)
	for i := range items {
		items[i] = data.NewInteger(big.NewInt(1))
	}
	return &lcommon.Datum{Data: data.NewList(items...)}
}

// largeContextOutputCbor builds the wire form of a post-Alonzo output carrying
// an inline datum.
func largeContextOutputCbor(
	t *testing.T,
	addr lcommon.Address,
	datum *lcommon.Datum,
) []byte {
	t.Helper()
	datumCbor, err := data.Encode(datum.Data)
	require.NoError(t, err)
	addrCbor, err := cbor.Encode(addr)
	require.NoError(t, err)
	var buf bytes.Buffer
	buf.WriteByte(0xa3) // map(3)
	buf.WriteByte(0x00)
	buf.Write(addrCbor)
	buf.WriteByte(0x01)
	coin, err := cbor.Encode(uint64(2_000_000))
	require.NoError(t, err)
	buf.Write(coin)
	buf.Write([]byte{0x02, 0x82, 0x01, 0xd8, 0x18}) // [1, tag24(bytes)]
	wrapped, err := cbor.Encode(datumCbor)
	require.NoError(t, err)
	buf.Write(wrapped)
	return buf.Bytes()
}

// largeContextNodeCount counts the nodes of a PlutusData value, matching the
// per-node accounting the evaluator applies to an applied term.
func largeContextNodeCount(pd data.PlutusData) int {
	count := 0
	var walk func(data.PlutusData)
	walk = func(v data.PlutusData) {
		count++
		switch d := v.(type) {
		case *data.Constr:
			for _, f := range d.Fields {
				walk(f)
			}
		case *data.List:
			for _, item := range d.Items {
				walk(item)
			}
		case *data.Map:
			for _, pair := range d.Pairs {
				walk(pair[0])
				walk(pair[1])
			}
		}
	}
	walk(pd)
	return count
}

// largeContextTx builds a transaction that references n outputs, each carrying
// a datum of datumNodes nodes, and returns those outputs as resolved inputs.
func largeContextTx(
	t *testing.T,
	n int,
	datumNodes int,
) (*conway.ConwayTransaction, []lcommon.Utxo) {
	t.Helper()
	addr, err := lcommon.NewAddressFromParts(
		lcommon.AddressTypeKeyNone,
		lcommon.AddressNetworkTestnet,
		make([]byte, lcommon.AddressHashSize),
		nil,
	)
	require.NoError(t, err)
	datum := largeContextDatum(datumNodes)
	outCbor := largeContextOutputCbor(t, addr, datum)

	refs := make([]shelley.ShelleyTransactionInput, 0, n)
	utxos := make([]lcommon.Utxo, 0, n)
	for i := range n {
		in := shelley.MustNewShelleyTransactionInput(
			fmt.Sprintf("%064x", i+1),
			0,
		)
		refs = append(refs, in)
		out, err := babbage.NewBabbageTransactionOutputFromCbor(outCbor)
		require.NoError(t, err)
		utxos = append(utxos, lcommon.Utxo{Id: in, Output: out})
	}
	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(nil),
			TxReferenceInputs: cbor.NewSetType(
				refs,
				false,
			),
		},
	}
	return tx, utxos
}

// TestPlutusV3LargeLedgerContextEvaluates pins that a script context which the
// ledger accepts still evaluates when its node count exceeds the evaluator's
// per-term node budget. Such a context is not attacker-authored script code,
// so capping the applied term rejects valid phase-2 evaluations.
func TestPlutusV3LargeLedgerContextEvaluates(t *testing.T) {
	tx, utxos := largeContextTx(
		t,
		largeContextRefInputs,
		largeContextDatumNodes,
	)
	info, err := script.NewTxInfoV3FromTransaction(
		preprodSlotState,
		tx,
		utxos,
		10,
	)
	require.NoError(t, err)
	contextData := info.ToPlutusData()
	require.Greater(
		t,
		largeContextNodeCount(contextData),
		1_000_000,
		"the script context must exceed the evaluator's per-term node budget",
	)

	// A minimal V3 script: one lambda returning unit, ignoring its context.
	flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{
		Version: lang.LanguageVersion{1, 0, 0},
		Term: &syn.Lambda[syn.DeBruijn]{
			Body: &syn.Constant{Con: &syn.Unit{}},
		},
	})
	require.NoError(t, err)
	wrapper, err := cbor.Encode(flat)
	require.NoError(t, err)
	plutusScript := lcommon.PlutusV3Script(wrapper)

	params := make(
		[]int64,
		len(lang.GetParamNamesForVersion(lang.LanguageVersionV3)),
	)
	for i := range params {
		params[i] = 1
	}
	evalContext, err := lcommon.PooledEvalContext(
		lang.LanguageVersionV3,
		910002,
		params,
	)
	require.NoError(t, err)

	_, err = plutusScript.EvaluateContext(
		context.Background(),
		contextData,
		lcommon.ExUnits{Steps: 10_000_000, Memory: 10_000_000},
		evalContext,
	)
	require.NoError(t, err)
}
