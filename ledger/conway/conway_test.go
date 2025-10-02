// Copyright 2025 Blink Labs Software
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

package conway

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestConwayRedeemersIter(t *testing.T) {
	testRedeemers := ConwayRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{
				Tag:   common.RedeemerTagMint,
				Index: 2,
			}: {
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  2222,
				},
			},
			{
				Tag:   common.RedeemerTagMint,
				Index: 0,
			}: {
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  0,
				},
			},
			{
				Tag:   common.RedeemerTagSpend,
				Index: 4,
			}: {
				ExUnits: common.ExUnits{
					Memory: 0,
					Steps:  4444,
				},
			},
		},
	}
	expectedOrder := []struct {
		Key   common.RedeemerKey
		Value common.RedeemerValue
	}{
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagSpend,
				Index: 4,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 0,
					Steps:  4444,
				},
			},
		},
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagMint,
				Index: 0,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  0,
				},
			},
		},
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagMint,
				Index: 2,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  2222,
				},
			},
		},
	}
	iterIdx := 0
	for key, val := range testRedeemers.Iter() {
		expected := expectedOrder[iterIdx]
		if !reflect.DeepEqual(key, expected.Key) {
			t.Fatalf(
				"did not get expected key: got %#v, wanted %#v",
				key,
				expected.Key,
			)
		}
		if !reflect.DeepEqual(val, expected.Value) {
			t.Fatalf(
				"did not get expected value: got %#v, wanted %#v",
				val,
				expected.Value,
			)
		}
		iterIdx++
	}
}

// Transaction taken from https://cexplorer.io/tx/6e6e15e39da0b8283b6c6d10b88b29adcac12e67edcf502c84cd2adb38a68880
// HASH: 6e6e15e39da0b8283b6c6d10b88b29adcac12e67edcf502c84cd2adb38a68880

const conwayTxHex = `84a40081825820b267f34cbf10a995565482742f328537ba4ccc767ea3ec99d4a443b5c71ae70101018182584c82d818584283581c4473be071285b7c1ac840cfaef878d95e078bf67fb859d587d488128a101581e581c5981b261ab5ccd11065b4b22cb3d6ba5938430480014d42742714dfb001a103e5cad1a0afcf06d021a00028ab9031a0b532b80a10081825820ca3554458c006d1ef04f33c13688b06c032d7c80adaf2967a01f1159c8058c65584016ce288ac2aca9ec373f3f84fd3a37c762e0ab244f68b2fd1c59525f24bbd5afc82ccbc67fa3eeeaaa7df2e212eae795b48e7803e6c7ec49b6b14bde6b84550af5f6`

func TestConwayTx_CborRoundTrip_UsingCborEncode(t *testing.T) {
	txHex := strings.TrimSpace(conwayTxHex)
	orig, err := hex.DecodeString(txHex)
	if err != nil {
		t.Fatalf("failed to decode Conway tx hex into CBOR bytes: %v", err)
	}

	var tx ConwayTransaction
	if err := tx.UnmarshalCBOR(orig); err != nil {
		t.Fatalf("failed to unmarshal CBOR into ConwayTransaction: %v", err)
	}

	enc, err := cbor.Encode(tx)
	if err != nil {
		t.Fatalf("failed to encode ConwayTransaction via cbor.Encode: %v", err)
	}
	if len(enc) == 0 {
		t.Fatal("encoded tx CBOR is empty")
	}

	if !bytes.Equal(orig, enc) {
		t.Errorf("CBOR round-trip mismatch\noriginal: %x\nencoded : %x", orig, enc)

		i := -1
		for j := 0; j < len(orig) && j < len(enc); j++ {
			if orig[j] != enc[j] {
				i = j
				break
			}
		}
		if i != -1 {
			t.Logf("first diff at byte %d: orig=0x%02x enc=0x%02x", i, orig[i], enc[i])
		} else {
			t.Logf("length mismatch: orig=%d enc=%d", len(orig), len(enc))
		}
	}
}

func TestConwayTx_Utxorpc(t *testing.T) {
	txHex := strings.TrimSpace(conwayTxHex)
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		t.Fatalf("failed to decode Conway tx hex: %v", err)
	}

	tx, err := NewConwayTransactionFromCbor(txBytes)
	if err != nil {
		t.Fatalf("failed to parse Conway tx: %v", err)
	}

	utxoTx, err := tx.Utxorpc()
	if err != nil {
		t.Fatalf("failed to convert Conway tx to utxorpc: %v", err)
	}

	expHash := tx.Hash().Bytes()
	if !equalBytes(utxoTx.Hash, expHash) {
		t.Errorf("tx hash mismatch\nexpected: %x\nactual  : %x", expHash, utxoTx.Hash)
	}

	if got, want := len(utxoTx.Inputs), len(tx.Inputs()); got != want {
		t.Errorf("inputs count mismatch: got %d want %d", got, want)
	}
	if got, want := len(utxoTx.Outputs), len(tx.Outputs()); got != want {
		t.Errorf("outputs count mismatch: got %d want %d", got, want)
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
