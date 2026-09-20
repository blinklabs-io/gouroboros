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
// zero groups. decodeVariableLengthWord16 and decodeVariableLengthWord32 test
// the first byte's spare high bits only while decoding their final group, and
// 0b10000000 passes that test, so the reference accepts these encodings up to
// its group cap.
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

// canonicalPointer is the encoding AddressPayloadPointer.encode produces for a
// decoded pointer. Bytes() re-encodes rather than replaying the input, which is
// what fromCborBothAddr does for every StakeRefPtr at every decoder version by
// setting cAddrNormalized to "compactAddr addr".
func canonicalPointer(p AddressPayloadPointer) []byte {
	return bytes.Join([][]byte{
		varUint(p.Slot),
		varUint(p.TxIndex),
		varUint(p.CertIndex),
	}, nil)
}

func TestAddressPointerComponentBounds(t *testing.T) {
	tests := []struct {
		name string
		// pointer is the raw encoding appended to the payment credential.
		pointer []byte
		// want is the pointer every era below decoder version 9 decodes to.
		want AddressPayloadPointer
		// wantStrictReject is whether decodePtr, used from decoder version 9,
		// would have refused the encoding.
		wantStrictReject bool
		wantErr          error
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
			// Two groups holding 5. decodeVariableLengthWord32 reaches its
			// first-byte check only in the fifth group, so a short padded
			// encoding is never checked at all and decodes to 5 upstream.
			name: "slot padded with a leading zero group",
			pointer: bytes.Join(
				[][]byte{varUintPadded(5, 2), varUint(0), varUint(0)},
				nil,
			),
			want: AddressPayloadPointer{Slot: 5},
		},
		{
			// Every component padded to exactly its group cap, so the
			// reference's first-byte check does run. It requires the spare
			// high bits to be clear, which 0b10000000 satisfies.
			name: "components padded to their group caps",
			pointer: bytes.Join([][]byte{
				varUintPadded(5, 5),
				varUintPadded(7, 3),
				varUintPadded(9, 3),
			}, nil),
			want: AddressPayloadPointer{Slot: 5, TxIndex: 7, CertIndex: 9},
		},
		{
			name: "slot one past Word32",
			pointer: bytes.Join(
				[][]byte{varUint(math.MaxUint32 + 1), varUint(0), varUint(0)},
				nil,
			),
			wantStrictReject: true,
		},
		{
			name: "transaction index one past Word16",
			pointer: bytes.Join(
				[][]byte{varUint(0), varUint(math.MaxUint16 + 1), varUint(0)},
				nil,
			),
			wantStrictReject: true,
		},
		{
			name: "certificate index one past Word16",
			pointer: bytes.Join(
				[][]byte{varUint(0), varUint(0), varUint(math.MaxUint16 + 1)},
				nil,
			),
			wantStrictReject: true,
		},
		{
			// Six groups holding 1. decodeVariableLengthWord32 fails with
			// "too many bytes." on the sixth, while decodeVariableLengthWord64
			// has no cap and yields 1.
			name: "slot group count past the Word32 cap",
			pointer: bytes.Join(
				[][]byte{varUintPadded(1, 6), varUint(0), varUint(0)},
				nil,
			),
			want:             AddressPayloadPointer{Slot: 1},
			wantStrictReject: true,
		},
		{
			name: "transaction index group count past the Word16 cap",
			pointer: bytes.Join(
				[][]byte{varUint(0), varUintPadded(1, 4), varUint(0)},
				nil,
			),
			want:             AddressPayloadPointer{TxIndex: 1},
			wantStrictReject: true,
		},
		{
			name: "slot wider than the uint64 accumulator",
			pointer: bytes.Join(
				[][]byte{varUintPadded(0, 15), varUint(0), varUint(0)},
				nil,
			),
			wantStrictReject: true,
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
			// Pointer handling no longer depends on the trailing-byte rule, so
			// both constructors must agree for every input.
			for name, decode := range map[string]func([]byte) (Address, error){
				"NewAddressFromBytes":        NewAddressFromBytes,
				"NewAddressFromBytesLenient": NewAddressFromBytesLenient,
			} {
				addr, err := decode(addrBytes)
				if test.wantErr != nil {
					if !errors.Is(err, test.wantErr) {
						t.Fatalf("%s: got error %v, want %v", name, err, test.wantErr)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s: unexpected error: %v", name, err)
				}
				got, ok := addr.StakingPayload().(AddressPayloadPointer)
				if !ok {
					t.Fatalf(
						"%s: staking payload is %T, want AddressPayloadPointer",
						name,
						addr.StakingPayload(),
					)
				}
				if got != test.want {
					t.Fatalf("%s: got %+v, want %+v", name, got, test.want)
				}
				err = CheckAddressPointerInRange(addr)
				if test.wantStrictReject {
					if !errors.Is(err, ErrAddressPointerOutOfRange) {
						t.Fatalf(
							"%s: strict check returned %v, want %v",
							name,
							err,
							ErrAddressPointerOutOfRange,
						)
					}
				} else if err != nil {
					t.Fatalf("%s: strict check rejected in-range pointer: %v", name, err)
				}
				roundTripped, err := addr.Bytes()
				if err != nil {
					t.Fatalf("%s: unexpected error re-encoding: %v", name, err)
				}
				want := pointerAddressBytes(canonicalPointer(test.want))
				if !bytes.Equal(roundTripped, want) {
					t.Fatalf(
						"%s: round-trip produced %x, want %x",
						name,
						roundTripped,
						want,
					)
				}
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

// Decoding normalizes an out-of-range pointer in every era, because every
// decoder version below 9 reaches decodePtrLenient and mkPtrNormalized. Only
// CheckAddressPointerInRange, which decoder version 9 onward applies, rejects
// it.
func TestAddressPointerNormalizesOutOfRange(t *testing.T) {
	pointer := bytes.Join([][]byte{
		varUint(math.MaxUint32 + 1),
		varUint(9),
		varUint(4),
	}, nil)
	addrBytes := pointerAddressBytes(pointer)
	for name, decode := range map[string]func([]byte) (Address, error){
		"NewAddressFromBytes":        NewAddressFromBytes,
		"NewAddressFromBytesLenient": NewAddressFromBytesLenient,
	} {
		addr, err := decode(addrBytes)
		if err != nil {
			t.Fatalf("%s: decode failed: %v", name, err)
		}
		got, ok := addr.StakingPayload().(AddressPayloadPointer)
		if !ok {
			t.Fatalf("%s: staking payload is %T", name, addr.StakingPayload())
		}
		if got != (AddressPayloadPointer{}) {
			t.Fatalf("%s: got %+v, want all components zero", name, got)
		}
		if err := CheckAddressPointerInRange(addr); !errors.Is(
			err,
			ErrAddressPointerOutOfRange,
		) {
			t.Fatalf(
				"%s: strict check returned %v, want %v",
				name,
				err,
				ErrAddressPointerOutOfRange,
			)
		}
	}
}

// decodeVariableLengthWord64 is "fix (decode7BitVarLength name buf) 0", so it
// has no group cap and its accumulator wraps. Eleven groups whose leading group
// is shifted past bit 63 leave an in-range value, and the lenient decoders that
// Shelley through Babbage use must produce that wrapped value rather than
// normalizing to zero.
func TestAddressPointerWrapsPastUint64(t *testing.T) {
	// 1 followed by nine empty groups is shifted 70 bits and disappears; the
	// final group supplies the 5 that remains.
	slot := append(
		append([]byte{0x81}, bytes.Repeat([]byte{0x80}, 9)...),
		0x05,
	)
	pointer := bytes.Join([][]byte{slot, varUint(1), varUint(2)}, nil)
	addr, err := NewAddressFromBytesLenient(pointerAddressBytes(pointer))
	if err != nil {
		t.Fatalf("lenient decode failed: %v", err)
	}
	got, ok := addr.StakingPayload().(AddressPayloadPointer)
	if !ok {
		t.Fatalf("staking payload is %T", addr.StakingPayload())
	}
	want := AddressPayloadPointer{Slot: 5, TxIndex: 1, CertIndex: 2}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// The same encoding is past decodePtr's five-group cap, so decoder version
	// 9 onward still refuses it.
	if err := CheckAddressPointerInRange(addr); !errors.Is(
		err,
		ErrAddressPointerOutOfRange,
	) {
		t.Fatalf(
			"strict check returned %v, want %v",
			err,
			ErrAddressPointerOutOfRange,
		)
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
	f.Add([]byte{
		0x81, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x05,
		0x01, 0x02,
	})

	f.Fuzz(func(t *testing.T, pointer []byte) {
		addr, err := NewAddressFromBytes(pointerAddressBytes(pointer))
		if err != nil {
			return
		}
		payload, ok := addr.StakingPayload().(AddressPayloadPointer)
		if !ok {
			t.Fatalf("staking payload is %T", addr.StakingPayload())
		}
		// mkPtrNormalized guarantees the decoded pointer fits Ptr, whatever the
		// encoding held.
		if payload.Slot > math.MaxUint32 ||
			payload.TxIndex > math.MaxUint16 ||
			payload.CertIndex > math.MaxUint16 {
			t.Fatalf("accepted out-of-range pointer %+v", payload)
		}
		// Bytes() emits the canonical encoding rather than the input, so the
		// invariant is that re-decoding it is a fixed point.
		encoded, err := addr.Bytes()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := pointerAddressBytes(canonicalPointer(payload))
		if !bytes.Equal(encoded, want) {
			t.Fatalf("re-encoded %x, want the canonical %x", encoded, want)
		}
		reDecoded, err := NewAddressFromBytes(encoded)
		if err != nil {
			t.Fatalf("canonical encoding failed to decode: %v", err)
		}
		if reDecoded.StakingPayload() != AddressPayload(payload) {
			t.Fatalf(
				"re-decode produced %+v, want %+v",
				reDecoded.StakingPayload(),
				payload,
			)
		}
		// A canonical encoding is always inside decodePtr's rule, so the
		// stricter eras accept what the lenient ones re-emit.
		if err := CheckAddressPointerInRange(reDecoded); err != nil {
			t.Fatalf("canonical encoding rejected by the strict check: %v", err)
		}
	})
}

// Fifteen groups whose every significant bit is shifted past bit 63 leave
// zero, which is the value decodeVariableLengthWord64 produces for the same
// input.
func TestAddressPointerWrapsToZero(t *testing.T) {
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
	want := AddressPayloadPointer{TxIndex: 9, CertIndex: 4}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if err := CheckAddressPointerInRange(addr); !errors.Is(
		err,
		ErrAddressPointerOutOfRange,
	) {
		t.Fatalf(
			"strict check returned %v, want %v",
			err,
			ErrAddressPointerOutOfRange,
		)
	}
}
