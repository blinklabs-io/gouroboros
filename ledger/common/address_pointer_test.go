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

package common

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"testing"
)

// pointerAddressBytes builds a mainnet type-4 address from a raw pointer
// encoding, bypassing AddressPayloadPointer.encode so a test can supply a
// pointer the encoder would never produce.
func pointerAddressBytes(pointer []byte) []byte {
	header := byte(
		(AddressTypeKeyPointer << 4) | (AddressNetworkMainnet & AddressHeaderNetworkMask),
	)
	out := make([]byte, 0, 1+AddressHashSize+len(pointer))
	out = append(out, header)
	out = append(out, bytes.Repeat([]byte{0xAB}, AddressHashSize)...)
	return append(out, pointer...)
}

// varUint encodes val as a big-endian 7-bit variable-length natural using the
// minimum number of groups.
func varUint(val uint64) []byte {
	out := []byte{byte(val & 0x7F)}
	for val >>= 7; val > 0; val >>= 7 {
		out = append([]byte{byte(val&0x7F) | 0x80}, out...)
	}
	return out
}

// varUintPadded encodes val in exactly groups groups, padding with leading
// zero groups. The reference decoder accepts such encodings below its group
// cap, but they do not round-trip.
func varUintPadded(val uint64, groups int) []byte {
	out := make([]byte, groups)
	for i := groups - 1; i >= 0; i-- {
		out[i] = byte(val & 0x7F)
		if i != groups-1 {
			out[i] |= 0x80
		}
		val >>= 7
	}
	return out
}

func TestAddressPointerComponentBounds(t *testing.T) {
	tests := []struct {
		name    string
		pointer []byte
		wantErr error
		want    AddressPayloadPointer
	}{
		{
			name: "CIP-0019 pointer vector",
			pointer: bytes.Join(
				[][]byte{varUint(2498243), varUint(27), varUint(3)},
				nil,
			),
			want: AddressPayloadPointer{
				Slot:      2498243,
				TxIndex:   27,
				CertIndex: 3,
			},
		},
		{
			name: "maximum in-range components",
			pointer: bytes.Join([][]byte{
				varUint(math.MaxUint32),
				varUint(math.MaxUint16),
				varUint(math.MaxUint16),
			}, nil),
			want: AddressPayloadPointer{
				Slot:      math.MaxUint32,
				TxIndex:   math.MaxUint16,
				CertIndex: math.MaxUint16,
			},
		},
		{
			name: "slot one past Word32",
			pointer: bytes.Join(
				[][]byte{varUint(math.MaxUint32 + 1), varUint(0), varUint(0)},
				nil,
			),
			wantErr: ErrAddressPointerOutOfRange,
		},
		{
			name: "transaction index one past Word16",
			pointer: bytes.Join(
				[][]byte{varUint(0), varUint(math.MaxUint16 + 1), varUint(0)},
				nil,
			),
			wantErr: ErrAddressPointerOutOfRange,
		},
		{
			name: "certificate index one past Word16",
			pointer: bytes.Join(
				[][]byte{varUint(0), varUint(0), varUint(math.MaxUint16 + 1)},
				nil,
			),
			wantErr: ErrAddressPointerOutOfRange,
		},
		{
			// Six groups holding the value 1. The value fits Word32, so only a
			// group cap rejects it, matching the reference's "too many bytes."
			name: "slot group count past the Word32 cap",
			pointer: bytes.Join(
				[][]byte{varUintPadded(1, 6), varUint(0), varUint(0)},
				nil,
			),
			wantErr: ErrAddressPointerOutOfRange,
		},
		{
			name: "transaction index group count past the Word16 cap",
			pointer: bytes.Join(
				[][]byte{varUint(0), varUintPadded(1, 4), varUint(0)},
				nil,
			),
			wantErr: ErrAddressPointerOutOfRange,
		},
		{
			// Fifteen groups. A uint64 accumulator shifts the leading groups
			// away and yields zero, which is the silent narrowing this bound
			// exists to catch.
			name: "slot wider than the uint64 accumulator",
			pointer: bytes.Join(
				[][]byte{varUintPadded(0, 15), varUint(0), varUint(0)},
				nil,
			),
			wantErr: ErrAddressPointerOutOfRange,
		},
		{
			name: "slot padded with a leading zero group",
			pointer: bytes.Join(
				[][]byte{varUintPadded(5, 2), varUint(0), varUint(0)},
				nil,
			),
			wantErr: ErrAddressPointerNotCanonical,
		},
		{
			name:    "continuation bits never terminate",
			pointer: bytes.Repeat([]byte{0x80}, 4),
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name:    "certificate index truncated",
			pointer: bytes.Join([][]byte{varUint(1), varUint(1)}, nil),
			wantErr: io.ErrUnexpectedEOF,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			addrBytes := pointerAddressBytes(test.pointer)
			addr, err := NewAddressFromBytes(addrBytes)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("got error %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, ok := addr.StakingPayload().(AddressPayloadPointer)
			if !ok {
				t.Fatalf("staking payload is %T, want AddressPayloadPointer", addr.StakingPayload())
			}
			if got != test.want {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}
			roundTripped, err := addr.Bytes()
			if err != nil {
				t.Fatalf("unexpected error re-encoding: %v", err)
			}
			if !bytes.Equal(roundTripped, addrBytes) {
				t.Fatalf(
					"round-trip produced %x, want %x",
					roundTripped,
					addrBytes,
				)
			}
		})
	}
}

func TestAddressPointerRoundTrip(t *testing.T) {
	slots := []uint64{
		0,
		1,
		127,
		128,
		2498243,
		math.MaxUint16,
		math.MaxUint32 - 1,
		math.MaxUint32,
	}
	indexes := []uint64{0, 1, 127, 128, 16383, 16384, math.MaxUint16 - 1, math.MaxUint16}
	for _, slot := range slots {
		for _, txIndex := range indexes {
			for _, certIndex := range indexes {
				pointer := bytes.Join([][]byte{
					varUint(slot),
					varUint(txIndex),
					varUint(certIndex),
				}, nil)
				addrBytes := pointerAddressBytes(pointer)
				addr, err := NewAddressFromBytes(addrBytes)
				if err != nil {
					t.Fatalf(
						"pointer (%d, %d, %d): unexpected error: %v",
						slot, txIndex, certIndex, err,
					)
				}
				got, ok := addr.StakingPayload().(AddressPayloadPointer)
				if !ok {
					t.Fatalf("staking payload is %T", addr.StakingPayload())
				}
				want := AddressPayloadPointer{
					Slot:      slot,
					TxIndex:   txIndex,
					CertIndex: certIndex,
				}
				if got != want {
					t.Fatalf("got %+v, want %+v", got, want)
				}
				roundTripped, err := addr.Bytes()
				if err != nil {
					t.Fatalf("unexpected error re-encoding: %v", err)
				}
				if !bytes.Equal(roundTripped, addrBytes) {
					t.Fatalf(
						"pointer (%d, %d, %d): round-trip produced %x, want %x",
						slot, txIndex, certIndex, roundTripped, addrBytes,
					)
				}
			}
		}
	}
}

// A pre-Babbage output may carry a pointer the chain already accepted, so that
// path normalizes rather than rejecting, matching mkPtrNormalized.
func TestAddressPointerLenientNormalizes(t *testing.T) {
	pointer := bytes.Join([][]byte{
		varUint(math.MaxUint32 + 1),
		varUint(9),
		varUint(4),
	}, nil)
	addrBytes := pointerAddressBytes(pointer)
	if _, err := NewAddressFromBytes(addrBytes); !errors.Is(
		err,
		ErrAddressPointerOutOfRange,
	) {
		t.Fatalf("strict decode returned %v, want %v", err, ErrAddressPointerOutOfRange)
	}
	addr, err := NewAddressFromBytesLenient(addrBytes)
	if err != nil {
		t.Fatalf("lenient decode failed: %v", err)
	}
	got, ok := addr.StakingPayload().(AddressPayloadPointer)
	if !ok {
		t.Fatalf("staking payload is %T", addr.StakingPayload())
	}
	if got != (AddressPayloadPointer{}) {
		t.Fatalf("got %+v, want all components zero", got)
	}
}

// The only pointer address in this repository's fixtures is the stake
// reference in cardano-ledger's Dijkstra test transaction
// (ledger/dijkstra/testdata/cardano_ledger_dijkstra_w30_tx.hex).
func TestAddressPointerExistingFixture(t *testing.T) {
	addrBytes, err := hex.DecodeString(
		"40cb9358529df4729c3246a2a033cb9821abbfd16de4888005904abc410a0000",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	addr, err := NewAddressFromBytes(addrBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := addr.StakingPayload().(AddressPayloadPointer)
	if !ok {
		t.Fatalf("staking payload is %T", addr.StakingPayload())
	}
	want := AddressPayloadPointer{Slot: 10, TxIndex: 0, CertIndex: 0}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	roundTripped, err := addr.Bytes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(roundTripped, addrBytes) {
		t.Fatalf("round-trip produced %x, want %x", roundTripped, addrBytes)
	}
}

func FuzzAddressPointerRoundTrip(f *testing.F) {
	f.Add([]byte{0x00, 0x00, 0x00})
	f.Add([]byte{0x81, 0x98, 0xBD, 0x43, 0x1B, 0x03})
	f.Add([]byte{0x8F, 0xFF, 0xFF, 0xFF, 0x7F, 0x83, 0xFF, 0x7F, 0x83, 0xFF, 0x7F})
	f.Add(bytes.Repeat([]byte{0x80}, 15))
	f.Add([]byte{0x80, 0x05, 0x00, 0x00})

	f.Fuzz(func(t *testing.T, pointer []byte) {
		addrBytes := pointerAddressBytes(pointer)
		addr, err := NewAddressFromBytes(addrBytes)
		if err != nil {
			return
		}
		payload, ok := addr.StakingPayload().(AddressPayloadPointer)
		if !ok {
			t.Fatalf("staking payload is %T", addr.StakingPayload())
		}
		if payload.Slot > math.MaxUint32 ||
			payload.TxIndex > math.MaxUint16 ||
			payload.CertIndex > math.MaxUint16 {
			t.Fatalf("accepted out-of-range pointer %+v", payload)
		}
		roundTripped, err := addr.Bytes()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(roundTripped, addrBytes) {
			t.Fatalf("round-trip produced %x, want %x", roundTripped, addrBytes)
		}
	})
}

// Past ten 7-bit groups a uint64 accumulator drops the leading groups, so the
// lenient path reports the component as unrepresentable instead of using the
// wrapped value.
func TestAddressPointerLenientRejectsUint64Overflow(t *testing.T) {
	slot := append(
		bytes.Repeat([]byte{0xFF}, 5),
		varUintPadded(0, 10)...,
	)
	pointer := bytes.Join([][]byte{slot, varUint(9), varUint(4)}, nil)
	addr, err := NewAddressFromBytesLenient(pointerAddressBytes(pointer))
	if err != nil {
		t.Fatalf("lenient decode failed: %v", err)
	}
	got, ok := addr.StakingPayload().(AddressPayloadPointer)
	if !ok {
		t.Fatalf("staking payload is %T", addr.StakingPayload())
	}
	if got != (AddressPayloadPointer{}) {
		t.Fatalf("got %+v, want all components zero", got)
	}
}
