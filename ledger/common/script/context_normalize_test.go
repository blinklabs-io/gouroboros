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
const normalizeSpendTxHex = "84a200d9010281825820aed81b6b1ab9b79dddd7114092cc1ea21d86eb89a77c7987902ee8b64e46599400021a00030d40a204d9010281d87982582011111111111111111111111111111111111111111111111111111111111111110105a182000082d879825820111111111111111111111111111111111111111111111111111111111111111101821a002dc6c01a3b9aca00f5f6"

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
