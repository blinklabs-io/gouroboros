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

package ledger_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// mainnetTrailerAddress is a type 6 (payment key, no staking) mainnet address
// carrying one byte past its payload. It is one of the addresses that exist on
// Cardano mainnet with unconsumed trailing bytes
// (IntersectMBO/cardano-ledger#2729).
const mainnetTrailerAddress = "61549b5a20e449a3e394b762705f64b9a26b99013003a2bfdba239967c00"

// testnetTrailerAddress is the same shape on a testnet, which the previous
// mainnet-only trailer whitelist never admitted. fromCborBackwardsBothAddr is
// unconditional on network (cardano-ledger
// libs/cardano-ledger-core/src/Cardano/Ledger/Address.hs).
var testnetTrailerAddress = hex.EncodeToString(
	append(
		append(
			[]byte{byte(common.AddressTypeKeyNone << 4)},
			make([]byte, common.AddressHashSize)...,
		),
		0xFF,
	),
)

func trailerAddressCbor(t *testing.T, addrHex string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(addrHex)
	if err != nil {
		t.Fatalf("bad test address hex: %v", err)
	}
	encoded, err := cbor.Encode(raw)
	if err != nil {
		t.Fatalf("encode address: %v", err)
	}
	return encoded
}

// arrayOutput builds [address, coin], the Shelley through Alonzo output shape.
func arrayOutput(addrCbor []byte) []byte {
	out := []byte{0x82}
	out = append(out, addrCbor...)
	// 1000000 lovelace
	return append(out, 0x1a, 0x00, 0x0f, 0x42, 0x40)
}

// mapOutput builds {0: address, 1: coin}, the Babbage output shape.
func mapOutput(addrCbor []byte) []byte {
	out := []byte{0xa2, 0x00}
	out = append(out, addrCbor...)
	return append(out, 0x01, 0x1a, 0x00, 0x0f, 0x42, 0x40)
}

func decodedAddress(t *testing.T, out common.TransactionOutput) common.Address {
	t.Helper()
	return out.Address()
}

// TestPreBabbageOutputsAcceptTrailingAddressBytes covers decoder versions below
// 7, where fromCborBothAddr selects fromCborBackwardsBothAddr and the trailing
// bytes are cropped rather than rejected.
func TestPreBabbageOutputsAcceptTrailingAddressBytes(t *testing.T) {
	for network, addrHex := range map[string]string{
		"mainnet": mainnetTrailerAddress,
		"testnet": testnetTrailerAddress,
	} {
		addrCbor := trailerAddressCbor(t, addrHex)
		raw, err := hex.DecodeString(addrHex)
		if err != nil {
			t.Fatalf("bad test address hex: %v", err)
		}
		consumed := raw[:len(raw)-1]
		trailer := raw[len(raw)-1:]

		outputBytes := arrayOutput(addrCbor)
		for _, tc := range []struct {
			name   string
			output common.TransactionOutput
		}{
			{"shelley", &shelley.ShelleyTransactionOutput{}},
			{"mary", &mary.MaryTransactionOutput{}},
			{"alonzo", &alonzo.AlonzoTransactionOutput{}},
		} {
			t.Run(tc.name+"/"+network, func(t *testing.T) {
				if _, err := cbor.Decode(outputBytes, tc.output); err != nil {
					t.Fatalf("decode output: %v", err)
				}
				addr := decodedAddress(t, tc.output)
				if !bytes.Equal(addr.TrailingBytes(), trailer) {
					t.Fatalf(
						"trailing bytes: got %x, want %x",
						addr.TrailingBytes(),
						trailer,
					)
				}
				addrBytes, err := addr.Bytes()
				if err != nil {
					t.Fatalf("address bytes: %v", err)
				}
				if !bytes.Equal(addrBytes, consumed) {
					t.Fatalf(
						"address bytes: got %x, want the cropped %x",
						addrBytes,
						consumed,
					)
				}
			})
		}
	}
}

// TestBabbageOnwardOutputsRejectTrailingAddressBytes covers decoder version 7
// and later, where fromCborRigorousBothAddr reaches ensureBufIsConsumed.
func TestBabbageOnwardOutputsRejectTrailingAddressBytes(t *testing.T) {
	addrCbor := trailerAddressCbor(t, mainnetTrailerAddress)
	for _, tc := range []struct {
		name   string
		bytes  []byte
		output common.TransactionOutput
	}{
		{
			"babbage map form",
			mapOutput(addrCbor),
			&babbage.BabbageTransactionOutput{},
		},
		{
			"babbage legacy array form",
			arrayOutput(addrCbor),
			&babbage.BabbageTransactionOutput{},
		},
		{
			"dijkstra map form",
			mapOutput(addrCbor),
			&dijkstra.DijkstraTransactionOutput{},
		},
		{
			"dijkstra array form",
			arrayOutput(addrCbor),
			&dijkstra.DijkstraTransactionOutput{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cbor.Decode(tc.bytes, tc.output)
			if err == nil {
				t.Fatal("expected trailing address bytes to be rejected")
			}
			if !strings.Contains(err.Error(), "unexpected trailing byte") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestBabbageOnwardOutputsAcceptWellFormedAddresses is the negative control
// for the gate above: the same output shapes without a trailer still decode.
func TestBabbageOnwardOutputsAcceptWellFormedAddresses(t *testing.T) {
	addrHex := mainnetTrailerAddress[:len(mainnetTrailerAddress)-2]
	addrCbor := trailerAddressCbor(t, addrHex)
	for _, tc := range []struct {
		name   string
		bytes  []byte
		output common.TransactionOutput
	}{
		{
			"babbage map form",
			mapOutput(addrCbor),
			&babbage.BabbageTransactionOutput{},
		},
		{
			"babbage legacy array form",
			arrayOutput(addrCbor),
			&babbage.BabbageTransactionOutput{},
		},
		{
			"dijkstra map form",
			mapOutput(addrCbor),
			&dijkstra.DijkstraTransactionOutput{},
		},
		{
			"dijkstra array form",
			arrayOutput(addrCbor),
			&dijkstra.DijkstraTransactionOutput{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cbor.Decode(tc.bytes, tc.output); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			addr := decodedAddress(t, tc.output)
			if len(addr.TrailingBytes()) != 0 {
				t.Fatalf(
					"unexpected trailing bytes: %x",
					addr.TrailingBytes(),
				)
			}
		})
	}
}

// TestRewardAccountRejectsTrailingBytes covers decodeAccountAddressT, which
// calls ensureBufIsConsumed with no version gate, so a reward account with
// unconsumed bytes is invalid in every era.
func TestRewardAccountRejectsTrailingBytes(t *testing.T) {
	// Type 14 (no payment, staking key) on mainnet, with one byte past the
	// 28 byte credential
	raw := append(
		append(
			[]byte{byte(common.AddressTypeNoneKey<<4) | common.AddressNetworkMainnet},
			make([]byte, common.AddressHashSize)...,
		),
		0xFF,
	)
	addr, err := common.NewAddressFromBytes(raw)
	if err != nil {
		t.Fatalf("decode reward account: %v", err)
	}
	if _, err := addr.RewardAccountCredential(); err == nil {
		t.Fatal("expected a reward account with trailing bytes to be rejected")
	}
	// Negative control: the same account without the trailer is accepted
	good, err := common.NewAddressFromBytes(raw[:len(raw)-1])
	if err != nil {
		t.Fatalf("decode reward account: %v", err)
	}
	if _, err := good.RewardAccountCredential(); err != nil {
		t.Fatalf("well-formed reward account rejected: %v", err)
	}
}
