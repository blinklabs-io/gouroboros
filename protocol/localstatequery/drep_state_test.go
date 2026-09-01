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
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

// drepCredHash is the DRep key hash captured from a real cardano-node
// GetDRepState reply on a Conway devnet (cardano-cli 11.0.0.0).
const drepCredHash = "f857c5ebc4c4c6d8683c8b83a87de825f436acc2fddc48b4f0bae474"

// TestDRepStateResultWireBytes pins the GetDRepState value encoding to the
// exact bytes a real cardano-node sends, so the 4-element shape
// [ expiry, anchor, deposit, delegators ] cannot silently regress to the
// 3-element form that made cardano-cli fail with
// "Size mismatch when decoding Record RecD. Expected 3, but found 4."
func TestDRepStateResultWireBytes(t *testing.T) {
	var hash ledger.Blake2b224
	raw, err := hex.DecodeString(drepCredHash)
	if err != nil {
		t.Fatalf("decode hash: %s", err)
	}
	copy(hash[:], raw)
	cred := StakeCredential{Tag: 0, Bytes: hash}

	result := DRepStateResult{
		cred: DRepStateEntry{
			Expiry:  22,
			Anchor:  nil, // StrictMaybe SNothing -> empty list 0x80
			Deposit: 500_000_000,
			// no delegators -> empty set 0xd9010280
		},
	}
	data, err := cbor.Encode(result)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	// map(1) { [0, hash] : [22, [], 500000000, set()] }
	//   a1 8200 581c <28 hash> 84 16 80 1a1dcd6500 d9010280
	want := "a18200581c" + drepCredHash + "8416801a1dcd6500d9010280"
	if got := hex.EncodeToString(data); got != want {
		t.Fatalf("DRepState wire mismatch (no anchor/delegators):\n  got:  %s\n  want: %s", got, want)
	}
}

// TestDRepStateResultRoundTrip exercises the populated case: an anchor present
// (StrictMaybe SJust -> [anchor]) and a non-empty delegators set, and verifies
// encode/decode round-trips through the gouroboros client type.
func TestDRepStateResultRoundTrip(t *testing.T) {
	var drepHash ledger.Blake2b224
	copy(drepHash[:], mustHex(t, drepCredHash))
	cred := StakeCredential{Tag: 0, Bytes: drepHash}

	var delegHash ledger.Blake2b224
	copy(delegHash[:], mustHex(t, "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c"))
	deleg := StakeCredential{Tag: 1, Bytes: delegHash}

	anchor := lcommon.GovAnchor{Url: "https://drep.example.com"}
	copy(anchor.DataHash[:], mustHex(t,
		"aabbcc00000000000000000000000000000000000000000000000000000000ff"))

	want := DRepStateResult{
		cred: DRepStateEntry{
			Expiry:     100,
			Anchor:     &anchor,
			Deposit:    500_000_000,
			Delegators: []StakeCredential{deleg},
		},
	}
	data, err := cbor.Encode(want)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	var got DRepStateResult
	if _, err := cbor.Decode(data, &got); err != nil {
		t.Fatalf("decode: %s", err)
	}
	entry, ok := got[cred]
	if !ok {
		t.Fatalf("credential key missing after round-trip")
	}
	if entry.Expiry != 100 || entry.Deposit != 500_000_000 {
		t.Fatalf("scalar fields wrong: %+v", entry)
	}
	if entry.Anchor == nil || entry.Anchor.Url != anchor.Url ||
		entry.Anchor.DataHash != anchor.DataHash {
		t.Fatalf("anchor wrong after round-trip: %+v", entry.Anchor)
	}
	if len(entry.Delegators) != 1 || entry.Delegators[0] != deleg {
		t.Fatalf("delegators wrong after round-trip: %+v", entry.Delegators)
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decode hex %q: %s", s, err)
	}
	return b
}
