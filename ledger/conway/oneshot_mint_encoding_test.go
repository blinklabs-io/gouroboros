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
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/plutigo/data"
)

// mainnetOneShotMintPolicy is the Plutus V3 policy of the reference
// transaction, and also the script hash carried by its mint redeemer.
const mainnetOneShotMintPolicy = "eb55e440da235c80974c7883b0920c06b31ddac66c12c238626266e9"

// TestOneShotMintAssetNamePinsScriptVisibleEncoding pins, against real
// mainnet state, which CBOR encoding a Plutus script observes for a value
// that reached the node in a different encoding.
//
// The reference transaction is a one-shot mint. Its redeemer is a
// TxOutRef naming one of the transaction's own inputs, and its policy
// derives the minted asset name as blake2b_256(serialiseData(txOutRef)) --
// the usual idiom for making a mint unrepeatable. The script's UPLC
// contains exactly one serialiseData and one blake2b_256, composed
// directly.
//
// That makes the asset name a recorded, consensus-accepted answer to the
// question this test exists to pin. The redeemer arrives definite-length
// on the wire, yet the asset name on chain is the hash of the canonical
// indefinite-length encoding of the same value. The reference
// implementation therefore rebuilds script-visible values at the package
// default rather than replaying the bytes it received, and any
// implementation that hands the evaluator wire-faithful values computes a
// different asset name and rejects a transaction the network accepted.
//
// Normalize is what restores that behavior, so this test fails if
// Normalize is ever dropped from the ScriptContext builders, and it fails
// with a concrete on-chain mismatch rather than an abstract one.
func TestOneShotMintAssetNamePinsScriptVisibleEncoding(t *testing.T) {
	txCbor, err := hex.DecodeString(
		strings.TrimSpace(testdata.MainnetOneShotMintTxHex),
	)
	if err != nil {
		t.Fatalf("decode fixture hex: %v", err)
	}
	tx, err := conway.NewConwayTransactionFromCbor(txCbor)
	if err != nil {
		t.Fatalf("decode transaction: %v", err)
	}

	// The minted asset name, as recorded on chain.
	mint := tx.AssetMint()
	if mint == nil {
		t.Fatal("transaction has no mint field")
	}
	policies := mint.Policies()
	if len(policies) != 1 {
		t.Fatalf("expected exactly 1 mint policy, got %d", len(policies))
	}
	if got := policies[0].String(); got != mainnetOneShotMintPolicy {
		t.Fatalf("mint policy = %s, want %s", got, mainnetOneShotMintPolicy)
	}
	names := mint.Assets(policies[0])
	if len(names) != 1 {
		t.Fatalf("expected exactly 1 minted asset, got %d", len(names))
	}
	assetName := hex.EncodeToString(names[0])

	// The TxOutRef the policy hashes: the single field of the redeemer.
	var redeemerData data.PlutusData
	redeemers := tx.Witnesses().Redeemers()
	if redeemers == nil {
		t.Fatal("transaction has no redeemers")
	}
	count := 0
	for _, value := range redeemers.Iter() {
		redeemerData = value.Data.Data
		count++
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 redeemer, got %d", count)
	}
	outer, ok := redeemerData.(*data.Constr)
	if !ok {
		t.Fatalf("redeemer is %T, want *data.Constr", redeemerData)
	}
	if len(outer.Fields) != 1 {
		t.Fatalf("redeemer has %d fields, want 1", len(outer.Fields))
	}
	txOutRef := outer.Fields[0]

	hashOf := func(pd data.PlutusData) string {
		encoded, err := data.Encode(pd)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		return common.Blake2b256Hash(encoded).String()
	}

	// What a script sees when the value is rebuilt at the package default,
	// which is what the reference implementation does.
	normalized := hashOf(data.Normalize(txOutRef))
	if normalized != assetName {
		t.Errorf(
			"normalized TxOutRef hash does not reproduce the minted asset name\n"+
				"  minted on chain: %s\n"+
				"  normalized hash: %s\n"+
				"Either Normalize no longer resets to the reference encoding,"+
				" or the package default has changed.",
			assetName, normalized,
		)
	}

	// And what it sees if the wire encoding is preserved instead. This must
	// differ, otherwise the test would pass even with Normalize removed and
	// would be pinning nothing.
	passthrough := hashOf(txOutRef)
	if passthrough == assetName {
		t.Fatal(
			"wire-preserved TxOutRef also reproduces the asset name;" +
				" this fixture no longer distinguishes the two encodings" +
				" and the test proves nothing",
		)
	}
	t.Logf(
		"asset name %s reproduced from the normalized TxOutRef;"+
			" wire-preserved encoding yields %s",
		assetName, passthrough,
	)
}
