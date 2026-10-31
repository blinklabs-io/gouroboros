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

package ledger

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failureWithFields returns a definite list of count one-byte items whose
// first item is tag. Count stays under the rejection item budget so the
// budget pre-pass accepts it and only the shape check can refuse it.
func failureWithFields(tag byte, count int) []byte {
	data := []byte{
		0x9a,
		byte(count >> 24),
		byte(count >> 16),
		byte(count >> 8),
		byte(count),
	}
	data = append(data, tag)
	return append(data, make([]byte, count-1)...)
}

// applyTxErrorWith wraps one failure in the ShelleyTxValidationError envelope
// for era.
func applyTxErrorWith(era byte, failure []byte) []byte {
	data := []byte{0x81, 0x82, era, 0x81}
	return append(data, failure...)
}

func allocatedBy(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

const failureFieldsAllocLimit = 1 << 20

func TestFailureFieldsRejectedBeforeAllocation(t *testing.T) {
	// Not t.Parallel: reads the process-wide allocation counter.
	const count = 131000
	utxowErr := func(era uint8) func([]byte) error {
		return func(d []byte) error {
			return (&UtxowFailure{era: era}).UnmarshalCBOR(d)
		}
	}
	apply := func(era uint8) func([]byte) error {
		return func(d []byte) error {
			return (&ApplyTxError{era: era}).UnmarshalCBOR(d)
		}
	}
	// Each case is a decoder that sizes a slice from a recognized,
	// fixed-shape failure list, fed that list at its own level.
	cases := map[string]struct {
		decode  func([]byte) error
		payload []byte
	}{
		"utxow shelley": {utxowErr(EraIdShelley), failureWithFields(0, count)},
		"utxow alonzo":  {utxowErr(EraIdAlonzo), failureWithFields(0, count)},
		"utxow babbage": {utxowErr(EraIdBabbage), failureWithFields(1, count)},
		"utxow conway":  {utxowErr(EraIdConway), failureWithFields(0, count)},
		"utxow dijkstra": {
			utxowErr(EraIdDijkstra),
			failureWithFields(0, count),
		},
		"shelley utxow failure": {
			func(d []byte) error {
				return (&ShelleyUtxowFailure{}).UnmarshalCBOR(d)
			},
			failureWithFields(0, count),
		},
		"alonzo utxow failure": {
			func(d []byte) error {
				return (&AlonzoUtxowFailure{}).UnmarshalCBOR(d)
			},
			failureWithFields(0, count),
		},
		"babbage utxo failure": {
			func(d []byte) error {
				return (&BabbageUtxoFailure{}).UnmarshalCBOR(d)
			},
			failureWithFields(1, count),
		},
		"conway utxow failure": {
			func(d []byte) error {
				return (&ConwayUtxowFailure{}).UnmarshalCBOR(d)
			},
			failureWithFields(0, count),
		},
		"apply conway utxow": {
			apply(EraIdConway),
			append([]byte{0x81}, failureWithFields(1, count)...),
		},
		"apply shelley utxow": {
			apply(EraIdShelley),
			append([]byte{0x81}, failureWithFields(0, count)...),
		},
		"apply dijkstra mempool": {
			apply(EraIdDijkstra),
			append([]byte{0x81}, failureWithFields(1, count)...),
		},
		"apply dijkstra ledger": {
			apply(EraIdDijkstra),
			append(
				[]byte{0x81, 0x82, 0x01},
				failureWithFields(1, count)...,
			),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var err error
			allocated := allocatedBy(func() { err = tc.decode(tc.payload) })
			assert.Less(
				t,
				allocated,
				uint64(failureFieldsAllocLimit),
				"oversized failure must be refused before its fields are sized",
			)
			require.ErrorContains(t, err, "fields exceeds the maximum")
		})
	}
}

func TestFailureFieldsHeaderOnlyRejected(t *testing.T) {
	// Not t.Parallel: reads the process-wide allocation counter.
	cases := map[string][]byte{
		"definite 64-bit count": append(
			[]byte{0x9b},
			bytes.Repeat([]byte{0xff}, 8)...,
		),
		"definite 32-bit count": {0x9a, 0xff, 0xff, 0xff, 0xff},
		"indefinite, no break": append(
			[]byte{0x9f, 0x01},
			bytes.Repeat([]byte{0x00}, 200000)...,
		),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			var err error
			allocated := allocatedBy(func() {
				err = (&UtxowFailure{era: EraIdConway}).UnmarshalCBOR(data)
			})
			require.Error(t, err)
			assert.Less(t, allocated, uint64(failureFieldsAllocLimit))
		})
	}
}

func TestFailureFieldsOversizedRejectedThroughShelleyEntry(t *testing.T) {
	// Not t.Parallel: reads the process-wide allocation counter.
	const count = 131000
	// Tag 1 reaches a UTXOW (Conway) or MEMPOOL (Dijkstra) failure whose
	// field list is fixed-shape.
	for _, era := range []uint8{EraIdConway, EraIdDijkstra} {
		data := applyTxErrorWith(era, failureWithFields(1, count))
		var err error
		allocated := allocatedBy(func() {
			_, err = NewShelleyTxValidationErrorFromCbor(data)
		})
		assert.Less(
			t,
			allocated,
			uint64(failureFieldsAllocLimit),
			"era %d",
			era,
		)
		require.ErrorContains(
			t,
			err,
			"fields exceeds the maximum",
			"era %d",
			era,
		)
	}
}

func TestApplyTxErrorKeepsUnknownFailureWithManyFields(t *testing.T) {
	t.Parallel()
	failure := failureWithFields(20, 100)
	apply := &ApplyTxError{era: EraIdConway}
	require.NoError(t, apply.UnmarshalCBOR(append([]byte{0x81}, failure...)))
	require.Len(t, apply.Failures, 1)
	unknown, ok := apply.Failures[0].(*UnknownApplyTxFailureError)
	require.True(t, ok)
	require.Equal(t, 20, unknown.FailureType)
	require.Equal(t, failure, unknown.Cbor)
}

func TestFailureFieldsAcceptsRealShapes(t *testing.T) {
	t.Parallel()
	fields, err := decodeFailureFields([]byte{0x83, 0x01, 0x02, 0x03}, "x")
	require.NoError(t, err)
	require.Len(t, fields, 3)
	fields, err = decodeFailureFields([]byte{0x9f, 0x01, 0x02, 0xff}, "x")
	require.NoError(t, err)
	require.Len(t, fields, 2)
	_, err = decodeFailureFields([]byte{0x01}, "x")
	require.Error(t, err)
}

func TestApplyTxErrorUnknownFailureNotSizedFromFieldCount(t *testing.T) {
	// Not t.Parallel: reads the process-wide allocation counter.
	failure := failureWithFields(20, 131000)
	data := append([]byte{0x81}, failure...)
	apply := &ApplyTxError{era: EraIdConway}
	var err error
	allocated := allocatedBy(func() { err = apply.UnmarshalCBOR(data) })
	require.NoError(t, err)
	require.Len(t, apply.Failures, 1)
	assert.Less(t, allocated, uint64(failureFieldsAllocLimit))
}

func TestFailureFieldsOversizedUnknownConstructorPreserved(t *testing.T) {
	// Not t.Parallel: reads the process-wide allocation counter.
	// Tag 23 is unrecognized by every decoder below. An unrecognized
	// constructor has no known shape to bound, so it is preserved raw
	// whatever its field count, without sizing a slice from that count.
	const unknownTag = 23
	utxow := func(era uint8) func([]byte) (error, error) {
		return func(d []byte) (error, error) {
			f := &UtxowFailure{era: era}
			err := f.UnmarshalCBOR(d)
			return f.Err, err
		}
	}
	cases := map[string]func([]byte) (error, error){
		"utxow shelley":  utxow(EraIdShelley),
		"utxow alonzo":   utxow(EraIdAlonzo),
		"utxow babbage":  utxow(EraIdBabbage),
		"utxow conway":   utxow(EraIdConway),
		"utxow dijkstra": utxow(EraIdDijkstra),
		"utxow unknown era": func(d []byte) (error, error) {
			f := &UtxowFailure{era: 0x7f}
			err := f.UnmarshalCBOR(d)
			return f.Err, err
		},
		"shelley utxow failure": func(d []byte) (error, error) {
			f := &ShelleyUtxowFailure{}
			err := f.UnmarshalCBOR(d)
			return f.Err, err
		},
		"alonzo utxow failure": func(d []byte) (error, error) {
			f := &AlonzoUtxowFailure{}
			err := f.UnmarshalCBOR(d)
			return f.Err, err
		},
		"babbage utxo failure": func(d []byte) (error, error) {
			f := &BabbageUtxoFailure{}
			err := f.UnmarshalCBOR(d)
			return f.Err, err
		},
		"conway utxow failure": func(d []byte) (error, error) {
			f := &ConwayUtxowFailure{}
			err := f.UnmarshalCBOR(d)
			return f.Err, err
		},
	}
	for name, decode := range cases {
		for _, count := range []int{maxFailureFields + 1, 131000} {
			t.Run(fmt.Sprintf("%s/%d", name, count), func(t *testing.T) {
				data := failureWithFields(unknownTag, count)
				var got, err error
				allocated := allocatedBy(func() { got, err = decode(data) })
				require.NoError(t, err)
				assert.Less(t, allocated, uint64(failureFieldsAllocLimit))
				var raw []byte
				switch u := got.(type) {
				case *UnknownUtxowFailureError:
					require.Equal(t, unknownTag, u.FailureType)
					raw = u.Cbor
				case *UnknownUtxoFailureError:
					require.Equal(t, unknownTag, u.FailureType)
					raw = u.Cbor
				default:
					t.Fatalf("got %T, want an unknown failure", got)
				}
				require.Equal(t, data, raw)
			})
		}
	}
}

func TestDijkstraUnknownLedgerFailureOversizedPreserved(t *testing.T) {
	// Not t.Parallel: reads the process-wide allocation counter.
	for _, tc := range []struct {
		tag   byte
		count int
	}{
		{5, maxFailureFields + 1},
		{5, 131000},
		{23, maxFailureFields + 1},
		{23, 131000},
	} {
		count := tc.count
		mempool := append(
			[]byte{0x82, 0x01},
			failureWithFields(tc.tag, count)...,
		)
		apply := &ApplyTxError{era: EraIdDijkstra}
		var err error
		allocated := allocatedBy(func() {
			err = apply.UnmarshalCBOR(append([]byte{0x81}, mempool...))
		})
		require.NoError(t, err, "tag %d count %d", tc.tag, count)
		assert.Less(t, allocated, uint64(failureFieldsAllocLimit))
		require.Len(t, apply.Failures, 1)
		unknown, ok := apply.Failures[0].(*UnknownApplyTxFailureError)
		require.True(t, ok, "got %T", apply.Failures[0])
		require.Equal(t, 1, unknown.FailureType)
		require.Equal(t, mempool, []byte(unknown.Cbor))
	}
}

func TestTxSubmitErrorOversizedKnownFailureFallsBackToGeneric(t *testing.T) {
	t.Parallel()
	// Conway LEDGER tag 1 is UtxowFailure, a recognized two-entry shape.
	data := applyTxErrorWith(
		EraIdConway,
		failureWithFields(1, maxFailureFields+1),
	)
	_, err := NewShelleyTxValidationErrorFromCbor(data)
	require.ErrorContains(t, err, "fields exceeds the maximum")
	got, err := NewTxSubmitErrorFromCbor(data)
	require.NoError(t, err)
	generic, ok := got.(*GenericError)
	require.True(t, ok, "got %T", got)
	require.Equal(t, data, generic.Cbor)
}

func TestFailureFieldsOversizedKnownWrapperOfUnknownRejected(t *testing.T) {
	t.Parallel()
	// Alonzo UTXOW tag 0 wraps a Shelley UTXOW failure; the wrapped
	// constructor (tag 23) is unrecognized, but the wrapper is not.
	inBounds := &UtxowFailure{era: EraIdAlonzo}
	require.NoError(t, inBounds.UnmarshalCBOR([]byte{0x82, 0x00, 0x81, 23}))
	wrapper, ok := inBounds.Err.(*ShelleyUtxowFailure)
	require.True(t, ok, "got %T", inBounds.Err)
	require.IsType(t, &UnknownUtxowFailureError{}, wrapper.Err)
	data := append(
		[]byte{0x80 + maxFailureFields + 1, 0x00, 0x81, 23},
		make([]byte, maxFailureFields-1)...,
	)
	err := (&UtxowFailure{era: EraIdAlonzo}).UnmarshalCBOR(data)
	require.ErrorContains(t, err, "fields exceeds the maximum")
}
