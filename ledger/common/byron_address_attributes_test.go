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

package common_test

import (
	"bytes"
	"encoding/hex"
	"hash/crc32"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// byronAddressHash is the 28 byte root of the mainnet Byron address
// 82d818582483581c5d5e698eba3dd9452add99a1af9461beb0ba61b8bece26e7399878dd
// a1024102001a36d41aba, which is the address the Byron block fixture and
// TestAddressFromBytes use.
const byronAddressHash = "5d5e698eba3dd9452add99a1af9461beb0ba61b8bece26e7399878dd"

// buildByronAddress assembles the wire form of a Byron address:
// [ #6.24(bytes .cbor ([ address_root, addr_attributes, addr_type ])), crc ],
// with the attribute map supplied as raw CBOR so an attribute key this
// decoder does not interpret can be placed in it.
func buildByronAddress(t *testing.T, attrCbor []byte) []byte {
	t.Helper()
	root, err := hex.DecodeString(byronAddressHash)
	if err != nil {
		t.Fatalf("bad test hash: %v", err)
	}
	payload, err := cbor.Encode(
		[]any{root, cbor.RawMessage(attrCbor), uint64(0)},
	)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	addr, err := cbor.Encode(
		[]any{
			cbor.Tag{Number: 24, Content: payload},
			crc32.ChecksumIEEE(payload),
		},
	)
	if err != nil {
		t.Fatalf("encode address: %v", err)
	}
	return addr
}

// TestByronAddressPreservesUnknownAttributes covers the forward compatibility
// decCBORAttributes is built for: every key the updater does not consume is
// retained in attrRemain and written back by encCBORAttributes
// (cardano-ledger eras/byron/ledger/impl/src/Cardano/Chain/Common/Attributes.hs).
// The attribute map is hashed into the address root, so the re-encoding has to
// be byte for byte.
func TestByronAddressPreservesUnknownAttributes(t *testing.T) {
	tests := []struct {
		name         string
		attrHex      string
		wantUnparsed map[uint8][]byte
		wantPayload  []byte
		wantNetwork  *uint32
	}{
		{
			name:         "no attributes",
			attrHex:      "a0",
			wantUnparsed: nil,
		},
		{
			name:         "unknown key alone",
			attrHex:      "a10342c0de",
			wantUnparsed: map[uint8][]byte{3: {0xc0, 0xde}},
		},
		{
			name:    "unknown key alongside a derivation path",
			attrHex: "a2014a1c0102030405060708090342c0de",
			wantPayload: []byte{
				0x1c, 0x01, 0x02, 0x03, 0x04,
				0x05, 0x06, 0x07, 0x08, 0x09,
			},
			wantUnparsed: map[uint8][]byte{3: {0xc0, 0xde}},
		},
		{
			name:         "unknown key alongside a network magic",
			attrHex:      "a2024102034100",
			wantNetwork:  func() *uint32 { v := uint32(2); return &v }(),
			wantUnparsed: map[uint8][]byte{3: {0x00}},
		},
		{
			name:    "several unknown keys",
			attrHex: "a3034201020542030407420506",
			wantUnparsed: map[uint8][]byte{
				3: {0x01, 0x02},
				5: {0x03, 0x04},
				7: {0x05, 0x06},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrCbor, err := hex.DecodeString(tt.attrHex)
			if err != nil {
				t.Fatalf("bad test attribute hex: %v", err)
			}
			addrBytes := buildByronAddress(t, attrCbor)

			var addr common.Address
			if _, err := cbor.Decode(addrBytes, &addr); err != nil {
				t.Fatalf("decode Byron address: %v", err)
			}
			attrs := addr.ByronAttr()
			if len(attrs.Unparsed) != len(tt.wantUnparsed) {
				t.Fatalf(
					"unparsed attributes: got %v, want %v",
					attrs.Unparsed,
					tt.wantUnparsed,
				)
			}
			for key, want := range tt.wantUnparsed {
				if !bytes.Equal(attrs.Unparsed[key], want) {
					t.Fatalf(
						"unparsed attribute %d: got %x, want %x",
						key,
						attrs.Unparsed[key],
						want,
					)
				}
			}
			if !bytes.Equal(attrs.Payload, tt.wantPayload) {
				t.Fatalf(
					"derivation path: got %x, want %x",
					attrs.Payload,
					tt.wantPayload,
				)
			}
			switch {
			case tt.wantNetwork == nil && attrs.Network != nil:
				t.Fatalf("unexpected network magic %d", *attrs.Network)
			case tt.wantNetwork != nil && attrs.Network == nil:
				t.Fatal("missing network magic")
			case tt.wantNetwork != nil && *attrs.Network != *tt.wantNetwork:
				t.Fatalf(
					"network magic: got %d, want %d",
					*attrs.Network,
					*tt.wantNetwork,
				)
			}

			// The address root is a hash over the attribute map, so the
			// re-encoding has to reproduce the input exactly.
			roundTrip, err := addr.Bytes()
			if err != nil {
				t.Fatalf("re-encode Byron address: %v", err)
			}
			if !bytes.Equal(roundTrip, addrBytes) {
				t.Fatalf(
					"round trip: got %x, want %x",
					roundTrip,
					addrBytes,
				)
			}

			// And the base58 form has to survive a parse
			reparsed, err := common.NewAddress(addr.String())
			if err != nil {
				t.Fatalf("reparse Byron address: %v", err)
			}
			reparsedBytes, err := reparsed.Bytes()
			if err != nil {
				t.Fatalf("re-encode reparsed address: %v", err)
			}
			if !bytes.Equal(reparsedBytes, addrBytes) {
				t.Fatalf(
					"base58 round trip: got %x, want %x",
					reparsedBytes,
					addrBytes,
				)
			}
		})
	}
}

// TestByronAddressAttributesRejectMalformedValues is the negative control: the
// attribute map is Map Word8 LByteString, and key 2 is decoded as a Word32, so
// neither a non-bytestring value nor a bad network magic is accepted.
func TestByronAddressAttributesRejectMalformedValues(t *testing.T) {
	for _, tc := range []struct {
		name    string
		attrHex string
	}{
		// value for an unknown key is a text string, not a byte string
		{"unknown key with text value", "a10363616263"},
		// derivation path value is empty, which decodeFull cannot accept
		{"derivation path with empty value", "a10140"},
		// network magic value is not a valid CBOR uint32
		{"network magic with bad value", "a10243616263"},
		// network magic value is empty
		{"network magic with empty value", "a10240"},
		// duplicate attribute key
		{"duplicate key", "a20342010203420304"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attrCbor, err := hex.DecodeString(tc.attrHex)
			if err != nil {
				t.Fatalf("bad test attribute hex: %v", err)
			}
			addrBytes := buildByronAddress(t, attrCbor)
			var addr common.Address
			if _, err := cbor.Decode(addrBytes, &addr); err == nil {
				t.Fatal("expected malformed attributes to be rejected")
			}
		})
	}
}

// TestByronAddressAttributesEncodeAsValue pins the encoding of a
// ByronAddressAttributes value rather than a pointer. The Byron address root
// is a hash over the CBOR of the attribute map, and a consumer recomputing it
// from ByronAttr() encodes the value it is handed, so the unparsed keys have
// to be present there too.
func TestByronAddressAttributesEncodeAsValue(t *testing.T) {
	// {2: <network magic 2>, 3: <unknown>}
	attrCbor, err := hex.DecodeString("a202410203420102")
	if err != nil {
		t.Fatalf("bad test attribute hex: %v", err)
	}
	addrBytes := buildByronAddress(t, attrCbor)
	var addr common.Address
	if _, err := cbor.Decode(addrBytes, &addr); err != nil {
		t.Fatalf("decode Byron address: %v", err)
	}
	encoded, err := cbor.Encode(addr.ByronAttr())
	if err != nil {
		t.Fatalf("encode attributes: %v", err)
	}
	if !bytes.Equal(encoded, attrCbor) {
		t.Fatalf("attribute encoding: got %x, want %x", encoded, attrCbor)
	}
}

func TestByronAddressPreservesNonCanonicalAttributeEncoding(t *testing.T) {
	// The attribute map uses a non-canonical map-length encoding and an
	// indefinite-length byte string. Both forms are accepted by the Byron
	// decoder and are part of the bytes hashed into the address root.
	attrCbor, err := hex.DecodeString("b802035f41c041deff014a1c010203040506070809")
	if err != nil {
		t.Fatalf("bad test attribute hex: %v", err)
	}
	addrBytes := buildByronAddress(t, attrCbor)
	var addr common.Address
	if _, err := cbor.Decode(addrBytes, &addr); err != nil {
		t.Fatalf("decode Byron address: %v", err)
	}
	roundTrip, err := addr.Bytes()
	if err != nil {
		t.Fatalf("re-encode Byron address: %v", err)
	}
	if !bytes.Equal(roundTrip, addrBytes) {
		t.Fatalf("round trip: got %x, want %x", roundTrip, addrBytes)
	}
}

// TestByronAddressAttributesRejectShadowedKey covers the case
// encCBORAttributes panics on: a key that is both interpreted and carried as
// unparsed.
func TestByronAddressAttributesRejectShadowedKey(t *testing.T) {
	attrs := common.ByronAddressAttributes{
		Payload:  []byte{0x41, 0x00},
		Unparsed: map[uint8][]byte{1: {0x41, 0x01}},
	}
	if _, err := attrs.MarshalCBOR(); err == nil {
		t.Fatal("expected a shadowed attribute key to be rejected")
	}
}

func TestDecodedByronAddressAttributesRejectShadowedKey(t *testing.T) {
	attrCbor, err := hex.DecodeString("a202410203420102")
	if err != nil {
		t.Fatalf("bad test attribute hex: %v", err)
	}
	var attrs common.ByronAddressAttributes
	if _, err := cbor.Decode(attrCbor, &attrs); err != nil {
		t.Fatalf("decode attributes: %v", err)
	}
	attrs.Unparsed[1] = []byte{0x41, 0x01}
	if _, err := attrs.MarshalCBOR(); err == nil {
		t.Fatal("expected a shadowed attribute key to be rejected")
	}
}
