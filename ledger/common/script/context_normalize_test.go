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
	"encoding/hex"
	"testing"
	"time"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
)

// normalizeParentTxHex funds the spending transaction below with a single
// output at an enterprise script address.
const normalizeParentTxHex = "84a300d90102818258200000000000000000000000000000000000000000000000000000000000000000000181a300581d7022f3deb8008f5843205e0bd52f912bd3ee546238c95cfeae94bf7edd011a001e8480028200582082586e7a118c931d0cbe21f6f2361823d0f8577ad0fca2550dddddd3467955ff021a00030d40a0f5f6"

// normalizeSpendTxHex spends that output. Its witness datum and its spending
// redeemer both carry the same Constr encoded with a DEFINITE-length field
// list, which is legal on the wire and is what this transaction was built
// with.
const normalizeSpendTxHex = "84a300d9010281825820aed81b6b1ab9b79dddd7114092cc1ea21d86eb89a77c7987902ee8b64e465994000180021a00030d40a204d9010281d87982582011111111111111111111111111111111111111111111111111111111111111110105a182000082d879825820111111111111111111111111111111111111111111111111111111111111111101821a002dc6c01a3b9aca00f5f6"

// normalizeCanonicalData is the same value as it appears on the wire above,
// re-encoded the way the Plutus reference implementation encodes Data: a
// non-empty field list is INDEFINITE-length. A script that calls
// serialiseData on a value observes these bytes, so rendering the wire form
// into the context instead makes every hash and byte comparison over that
// value disagree with cardano-node.
const normalizeCanonicalData = "d8799f5820111111111111111111111111111111111111111111111111111111111111111101ff"

func normalizeFixture(t *testing.T) (lcommon.Transaction, []lcommon.Utxo) {
	t.Helper()
	parentBytes, err := hex.DecodeString(normalizeParentTxHex)
	if err != nil {
		t.Fatalf("decode parent transaction hex: %v", err)
	}
	parentTx, err := conway.NewConwayTransactionFromCbor(parentBytes)
	if err != nil {
		t.Fatalf("parse parent transaction: %v", err)
	}
	spendBytes, err := hex.DecodeString(normalizeSpendTxHex)
	if err != nil {
		t.Fatalf("decode spending transaction hex: %v", err)
	}
	spendTx, err := conway.NewConwayTransactionFromCbor(spendBytes)
	if err != nil {
		t.Fatalf("parse spending transaction: %v", err)
	}
	input := shelley.NewShelleyTransactionInput(parentTx.Hash().String(), 0)
	return spendTx, []lcommon.Utxo{
		{Id: &input, Output: parentTx.Outputs()[0]},
	}
}

var normalizeSlotState = mockSlotState{
	SlotLength: 1 * time.Second,
	ZeroTime:   time.UnixMilli(1596059091000),
	ZeroSlot:   4492800,
}

// TestTxInfoRedeemerDataNormalized pins the redeemer data a script observes
// against cardano-ledger, which rebuilds every script-visible value rather
// than carrying the wire encoding into the context.
func TestTxInfoRedeemerDataNormalized(t *testing.T) {
	t.Parallel()
	spendTx, resolvedInputs := normalizeFixture(t)
	txInfo, err := script.NewTxInfoV3FromTransaction(
		normalizeSlotState,
		spendTx,
		resolvedInputs,
	)
	if err != nil {
		t.Fatalf("build TxInfoV3: %v", err)
	}
	if len(txInfo.Redeemers) != 1 {
		t.Fatalf("redeemers = %d, want 1", len(txInfo.Redeemers))
	}
	encoded, err := data.Encode(txInfo.Redeemers[0].Value.Data)
	if err != nil {
		t.Fatalf("encode redeemer data: %v", err)
	}
	if got := hex.EncodeToString(encoded); got != normalizeCanonicalData {
		t.Errorf(
			"redeemer data encodes to %s, want %s",
			got,
			normalizeCanonicalData,
		)
	}
}

// TestTxInfoDataNormalized pins txInfoData the same way.
func TestTxInfoDataNormalized(t *testing.T) {
	t.Parallel()
	spendTx, resolvedInputs := normalizeFixture(t)
	txInfo, err := script.NewTxInfoV3FromTransaction(
		normalizeSlotState,
		spendTx,
		resolvedInputs,
	)
	if err != nil {
		t.Fatalf("build TxInfoV3: %v", err)
	}
	if len(txInfo.Data) != 1 {
		t.Fatalf("txInfoData entries = %d, want 1", len(txInfo.Data))
	}
	encoded, err := data.Encode(txInfo.Data[0].Value)
	if err != nil {
		t.Fatalf("encode datum: %v", err)
	}
	if got := hex.EncodeToString(encoded); got != normalizeCanonicalData {
		t.Errorf(
			"txInfoData value encodes to %s, want %s",
			got,
			normalizeCanonicalData,
		)
	}
}

// TestScriptPurposeSpendingDatumNormalized pins the datum handed to a V1 or V2
// spending script, and the one rendered into a V3 ScriptInfo.
func TestScriptPurposeSpendingDatumNormalized(t *testing.T) {
	t.Parallel()
	spendTx, resolvedInputs := normalizeFixture(t)
	txInfo, err := script.NewTxInfoV3FromTransaction(
		normalizeSlotState,
		spendTx,
		resolvedInputs,
	)
	if err != nil {
		t.Fatalf("build TxInfoV3: %v", err)
	}
	purpose, ok := txInfo.Redeemers[0].Key.(script.ScriptPurposeSpending)
	if !ok {
		t.Fatalf("purpose = %T, want ScriptPurposeSpending", txInfo.Redeemers[0].Key)
	}
	if purpose.Datum == nil {
		t.Fatal("spending purpose carries no datum")
	}
	encoded, err := data.Encode(purpose.Datum)
	if err != nil {
		t.Fatalf("encode datum: %v", err)
	}
	if got := hex.EncodeToString(encoded); got != normalizeCanonicalData {
		t.Errorf(
			"spending datum encodes to %s, want %s",
			got,
			normalizeCanonicalData,
		)
	}
}

// normalizeInlineParentTxHex funds the transaction below with an output whose
// INLINE datum is encoded with a definite-length field list.
const normalizeInlineParentTxHex = "84a300d90102818258200000000000000000000000000000000000000000000000000000000000000000000181a300581d7022f3deb8008f5843205e0bd52f912bd3ee546238c95cfeae94bf7edd011a001e8480028201d8185826d879825820111111111111111111111111111111111111111111111111111111111111111101021a00030d40a0f5f6"

const normalizeInlineSpendTxHex = "84a300d9010281825820472a316de1210312f40cf925d96e4fd1af9933cc04c60265542d90722aa4b2b2000180021a00030d40a105a182000082d879825820111111111111111111111111111111111111111111111111111111111111111101821a002dc6c01a3b9aca00f5f6"

// TestTxInfoV2InputDatumNormalized covers the V1/V2 rendering path, which does
// not go through the era TxOut ToPlutusData at all: WithZeroAdaAsset renders a
// resolved input, and it forwarded the inline datum with its wire encoding
// intact. A PlutusV2 script calling serialiseData on that datum saw different
// bytes than cardano-node.
func TestTxInfoV2InputDatumNormalized(t *testing.T) {
	t.Parallel()
	parentBytes, err := hex.DecodeString(normalizeInlineParentTxHex)
	if err != nil {
		t.Fatalf("decode parent transaction hex: %v", err)
	}
	parentTx, err := conway.NewConwayTransactionFromCbor(parentBytes)
	if err != nil {
		t.Fatalf("parse parent transaction: %v", err)
	}
	spendBytes, err := hex.DecodeString(normalizeInlineSpendTxHex)
	if err != nil {
		t.Fatalf("decode spending transaction hex: %v", err)
	}
	spendTx, err := conway.NewConwayTransactionFromCbor(spendBytes)
	if err != nil {
		t.Fatalf("parse spending transaction: %v", err)
	}
	input := shelley.NewShelleyTransactionInput(parentTx.Hash().String(), 0)
	resolvedInputs := []lcommon.Utxo{
		{Id: &input, Output: parentTx.Outputs()[0]},
	}
	if parentTx.Outputs()[0].Datum() == nil {
		t.Fatal("fixture output must carry an inline datum")
	}

	txInfo, err := script.NewTxInfoV2FromTransaction(
		normalizeSlotState,
		spendTx,
		resolvedInputs,
		false,
	)
	if err != nil {
		t.Fatalf("build TxInfoV2: %v", err)
	}

	// txInfoInputs is field 0; each entry is Constr 0 [TxOutRef, TxOut], and a
	// TxOut is Constr 0 [address, value, datumOption, ...]. An inline datum is
	// datumOption Constr 2 [datum].
	rendered, ok := txInfo.ToPlutusData().(*data.Constr)
	if !ok {
		t.Fatalf("TxInfoV2 did not render as a Constr")
	}
	inputs, ok := rendered.Fields[0].(*data.List)
	if !ok || len(inputs.Items) != 1 {
		t.Fatalf("txInfoInputs did not render as a one-item list")
	}
	resolved, ok := inputs.Items[0].(*data.Constr)
	if !ok {
		t.Fatalf("resolved input did not render as a Constr")
	}
	txOut, ok := resolved.Fields[1].(*data.Constr)
	if !ok {
		t.Fatalf("TxOut did not render as a Constr")
	}
	datumOption, ok := txOut.Fields[2].(*data.Constr)
	if !ok || datumOption.Tag == nil || datumOption.Tag.Int64() != 2 {
		t.Fatalf("datum option is not an inline datum: %v", txOut.Fields[2])
	}
	encoded, err := data.Encode(datumOption.Fields[0])
	if err != nil {
		t.Fatalf("encode inline datum: %v", err)
	}
	if got := hex.EncodeToString(encoded); got != normalizeCanonicalData {
		t.Errorf(
			"inline datum encodes to %s, want %s",
			got,
			normalizeCanonicalData,
		)
	}
}
