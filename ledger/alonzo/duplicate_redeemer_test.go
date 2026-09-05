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

package alonzo_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func decodeRedeemers(t *testing.T, raw string) alonzo.AlonzoRedeemers {
	t.Helper()
	b, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode fixture hex: %v", err)
	}
	var ret alonzo.AlonzoRedeemers
	if err := ret.UnmarshalCBOR(b); err != nil {
		t.Fatalf("decode redeemers: %v", err)
	}
	return ret
}

// TestAlonzoRedeemersDuplicateKeyLastWins reproduces the redeemer-list shape
// from the Preview transaction that wedged a Dingo replay at slot 12924758
// (blinklabs-io/dingo#3875). It carries six (mint, 0) entries with data
// 1, 1, 2, 2, 3, 3; execution-unit values are minimized because they do not
// affect key resolution.
func TestAlonzoRedeemersDuplicateKeyLastWins(t *testing.T) {
	const raw = "8684010001820101840100018201018401000282010184010002820101" +
		"8401000382010184010003820101"
	r := decodeRedeemers(t, raw)
	if len(r.Redeemers) != 6 {
		t.Fatalf("fixture redeemer count = %d, want 6", len(r.Redeemers))
	}

	got := r.Value(0, common.RedeemerTagMint)
	if got.Data.Data == nil {
		t.Fatal("Value did not resolve (mint, 0)")
	}
	if gotData := hex.EncodeToString(got.Data.Cbor()); gotData != "03" {
		t.Errorf("Value data = %s, want last entry 03", gotData)
	}

	count := 0
	for key, value := range r.Iter() {
		count++
		if key.Tag != common.RedeemerTagMint || key.Index != 0 {
			t.Errorf("Iter key = (%d, %d), want (%d, 0)", key.Tag, key.Index, common.RedeemerTagMint)
		}
		if gotData := hex.EncodeToString(value.Data.Cbor()); gotData != "03" {
			t.Errorf("Iter data = %s, want last entry 03", gotData)
		}
	}
	if count != 1 {
		t.Errorf("Iter count = %d, want one value for duplicated key", count)
	}
}

// TestAlonzoRedeemersDuplicateKeyPreservesDistinctKeys is a control proving
// that collapsing one duplicate does not discard or reorder distinct keys.
func TestAlonzoRedeemersDuplicateKeyPreservesDistinctKeys(t *testing.T) {
	// [(mint, 0, 1), (spend, 2, 4), (mint, 0, 3)]
	const raw = "83840100018201018400020482020384010003820405"
	r := decodeRedeemers(t, raw)

	type observed struct {
		key  common.RedeemerKey
		data string
	}
	var got []observed
	for key, value := range r.Iter() {
		got = append(got, observed{
			key:  key,
			data: hex.EncodeToString(value.Data.Cbor()),
		})
	}
	want := []observed{
		{key: common.RedeemerKey{Tag: common.RedeemerTagSpend, Index: 2}, data: "04"},
		{key: common.RedeemerKey{Tag: common.RedeemerTagMint, Index: 0}, data: "03"},
	}
	if len(got) != len(want) {
		t.Fatalf("Iter count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Iter[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
