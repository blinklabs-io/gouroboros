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

package byron_test

import (
	"bytes"
	"encoding/hex"
	"hash/crc32"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
)

// TestByronTransactionOutputPreservesUnknownAddressAttributes covers the path
// the divergence actually reaches: a Byron address is decoded from every Byron
// transaction output, so an attribute key this decoder does not interpret
// failed the whole containing block.
func TestByronTransactionOutputPreservesUnknownAddressAttributes(t *testing.T) {
	// The address root of the mainnet Byron address the block fixture carries
	root, err := hex.DecodeString(
		"5d5e698eba3dd9452add99a1af9461beb0ba61b8bece26e7399878dd",
	)
	if err != nil {
		t.Fatalf("bad test hash: %v", err)
	}
	// {2: <network magic>, 3: <unknown>}: the attribute map is
	// Map Word8 LByteString, and every key the updater does not consume is
	// retained (cardano-ledger
	// eras/byron/ledger/impl/src/Cardano/Chain/Common/Attributes.hs).
	attrCbor, err := hex.DecodeString("a202410203450102030405")
	if err != nil {
		t.Fatalf("bad test attribute hex: %v", err)
	}
	payload, err := cbor.Encode(
		[]any{root, cbor.RawMessage(attrCbor), uint64(0)},
	)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	addrBytes, err := cbor.Encode(
		[]any{
			cbor.Tag{Number: 24, Content: payload},
			crc32.ChecksumIEEE(payload),
		},
	)
	if err != nil {
		t.Fatalf("encode address: %v", err)
	}
	outputBytes, err := cbor.Encode(
		[]any{cbor.RawMessage(addrBytes), uint64(1000000)},
	)
	if err != nil {
		t.Fatalf("encode output: %v", err)
	}

	var output byron.ByronTransactionOutput
	if _, err := cbor.Decode(outputBytes, &output); err != nil {
		t.Fatalf("decode Byron transaction output: %v", err)
	}
	attrs := output.OutputAddress.ByronAttr()
	if !bytes.Equal(attrs.Unparsed[3], []byte{0x01, 0x02, 0x03, 0x04, 0x05}) {
		t.Fatalf("unparsed attribute 3: got %x", attrs.Unparsed[3])
	}
	if attrs.Network == nil || *attrs.Network != 2 {
		t.Fatalf("network magic: got %v", attrs.Network)
	}
	roundTrip, err := output.OutputAddress.Bytes()
	if err != nil {
		t.Fatalf("re-encode address: %v", err)
	}
	if !bytes.Equal(roundTrip, addrBytes) {
		t.Fatalf("round trip: got %x, want %x", roundTrip, addrBytes)
	}
}
