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
	"errors"
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// pointerVarUint encodes val as a big-endian 7-bit variable-length natural.
func pointerVarUint(val uint64) []byte {
	out := []byte{byte(val & 0x7F)}
	for val >>= 7; val > 0; val >>= 7 {
		out = append([]byte{byte(val&0x7F) | 0x80}, out...)
	}
	return out
}

// pointerOutputAddress builds the CBOR for a mainnet type 4 address whose stake
// reference is the given pointer.
func pointerOutputAddress(slot, txIndex, certIndex uint64) []byte {
	raw := append(
		[]byte{
			byte(
				common.AddressTypeKeyPointer<<4,
			) | common.AddressNetworkMainnet,
		},
		make([]byte, common.AddressHashSize)...,
	)
	raw = append(raw, pointerVarUint(slot)...)
	raw = append(raw, pointerVarUint(txIndex)...)
	raw = append(raw, pointerVarUint(certIndex)...)
	return mustEncodeCbor(raw)
}

// conwayBody wraps an output in the smallest Conway transaction body that
// decodes: inputs, outputs and fee.
func conwayBody(outputBytes []byte) []byte {
	return mustEncodeCbor(map[int]any{
		0: []any{},
		1: []any{cbor.RawMessage(outputBytes)},
		2: uint64(0),
	})
}

func decodedPointer(t *testing.T, addr common.Address) common.AddressPayloadPointer {
	t.Helper()
	ptr, ok := addr.StakingPayload().(common.AddressPayloadPointer)
	if !ok {
		t.Fatalf("staking payload is %T, want a pointer", addr.StakingPayload())
	}
	return ptr
}

// TestPreConwayOutputsNormalizeOutOfRangePointers covers every decoder version
// below 9. Versions below 7 reach decodePtrLenient through
// fromCborBackwardsBothAddr and versions 7 and 8, which are Babbage, reach it
// through fromCborRigorousBothAddr True, so all of them accept a pointer that
// does not fit Ptr and normalize it to all zeros through mkPtrNormalized.
func TestPreConwayOutputsNormalizeOutOfRangePointers(t *testing.T) {
	addrCbor := pointerOutputAddress(math.MaxUint32+1, 9, 4)
	for _, tc := range []struct {
		name   string
		bytes  []byte
		output common.TransactionOutput
	}{
		{"shelley", arrayOutput(addrCbor), &shelley.ShelleyTransactionOutput{}},
		{"mary", arrayOutput(addrCbor), &mary.MaryTransactionOutput{}},
		{"alonzo", arrayOutput(addrCbor), &alonzo.AlonzoTransactionOutput{}},
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cbor.Decode(tc.bytes, tc.output); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			got := decodedPointer(t, tc.output.Address())
			if got != (common.AddressPayloadPointer{}) {
				t.Fatalf("got %+v, want all components zero", got)
			}
		})
	}
}

// TestConwayOnwardOutputsRejectOutOfRangePointers covers decoder version 9 and
// later, where fromCborBothAddr passes False to fromCborRigorousBothAddr and
// decodeStakeReference uses decodePtr, which fails instead of normalizing.
func TestConwayOnwardOutputsRejectOutOfRangePointers(t *testing.T) {
	addrCbor := pointerOutputAddress(math.MaxUint32+1, 9, 4)
	for _, tc := range []struct {
		name   string
		bytes  []byte
		target any
	}{
		{
			"conway body",
			conwayBody(mapOutput(addrCbor)),
			&conway.ConwayTransactionBody{},
		},
		{
			"conway body with a legacy array output",
			conwayBody(arrayOutput(addrCbor)),
			&conway.ConwayTransactionBody{},
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
			_, err := cbor.Decode(tc.bytes, tc.target)
			if !errors.Is(err, common.ErrAddressPointerOutOfRange) {
				t.Fatalf(
					"got error %v, want %v",
					err,
					common.ErrAddressPointerOutOfRange,
				)
			}
		})
	}
}

// TestConwayOnwardOutputsAcceptPaddedPointers is the negative control for the
// gate above. decodeVariableLengthWord16 and decodeVariableLengthWord32 test
// the first byte's spare high bits only while decoding their final group, and
// 0b10000000 passes that test, so decodePtr accepts a padded in-range encoding
// at every decoder version. Rejecting one would stall a sync from genesis.
func TestConwayOnwardOutputsAcceptPaddedPointers(t *testing.T) {
	// The CIP-0019 pointer (2498243, 27, 3) with each component padded by one
	// leading zero group.
	raw := append(
		[]byte{
			byte(
				common.AddressTypeKeyPointer<<4,
			) | common.AddressNetworkMainnet,
		},
		make([]byte, common.AddressHashSize)...,
	)
	raw = append(raw, 0x80, 0x81, 0x98, 0xBD, 0x43, 0x80, 0x1B, 0x80, 0x03)
	addrCbor := mustEncodeCbor(raw)
	want := common.AddressPayloadPointer{
		Slot:      2498243,
		TxIndex:   27,
		CertIndex: 3,
	}

	var body conway.ConwayTransactionBody
	if _, err := cbor.Decode(conwayBody(mapOutput(addrCbor)), &body); err != nil {
		t.Fatalf("conway body: %v", err)
	}
	outputs := body.Outputs()
	if len(outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(outputs))
	}
	if got := decodedPointer(t, outputs[0].Address()); got != want {
		t.Fatalf("conway body: got %+v, want %+v", got, want)
	}

	var output dijkstra.DijkstraTransactionOutput
	if _, err := cbor.Decode(mapOutput(addrCbor), &output); err != nil {
		t.Fatalf("dijkstra output: %v", err)
	}
	if got := decodedPointer(t, output.Output.Address()); got != want {
		t.Fatalf("dijkstra output: got %+v, want %+v", got, want)
	}
}
