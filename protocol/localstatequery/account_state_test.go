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

package localstatequery

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// TestShelleyAccountStateQueryBuildShelleyQuery verifies the GetAccountState
// (sub-query 29) leaf wraps through buildShelleyQuery into the full
// BlockQuery -> ShelleyQuery -> era -> [29] envelope cardano-node sends.
func TestShelleyAccountStateQueryBuildShelleyQuery(t *testing.T) {
	const era = 6 // Conway
	q := buildShelleyQuery(era, QueryTypeShelleyAccountState)
	data, err := cbor.Encode(q)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	// [0, [0, [6, [29]]]]  =>  82 00 82 00 82 06 81 18 1d
	want := "82008200820681181d"
	if got := hex.EncodeToString(data); got != want {
		t.Fatalf("built query mismatch:\n  got:  %s\n  want: %s", got, want)
	}
}

// TestAccountStateResultRoundTrip verifies the [ [treasury, reserves] ] wire
// shape, including a negative reserves value (Coin is signed; a misconfigured
// network can drive reserves below zero, as seen against a real cardano-node).
func TestAccountStateResultRoundTrip(t *testing.T) {
	want := AccountStateResult{
		State: AccountState{Treasury: 500_000_000, Reserves: -1234},
	}
	data, err := cbor.Encode(want)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	var got AccountStateResult
	if _, err := cbor.Decode(data, &got); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch:\n  got:  %#v\n  want: %#v", got, want)
	}
}
