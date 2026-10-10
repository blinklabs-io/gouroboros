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

package cbor_test

import (
	"runtime"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// Not t.Parallel: measures process-wide allocation via runtime.MemStats.
func TestDecodeDoesNotCopyInputThroughStreamBuffer(t *testing.T) {
	const payloadSize = 64 * 1024
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i)
	}
	// Byte string with a 4-byte length header, followed by trailing bytes
	// that Decode must leave unread.
	encoded := append([]byte{0x5a, 0x00, 0x01, 0x00, 0x00}, payload...)
	encoded = append(encoded, 0xde, 0xad)

	var dest []byte
	decodeOnce := func() int {
		dest = nil
		n, err := cbor.Decode(encoded, &dest)
		require.NoError(t, err)
		return n
	}
	require.Equal(t, len(encoded)-2, decodeOnce())
	require.Equal(t, payload, dest)

	const runs = 20
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for range runs {
		decodeOnce()
	}
	runtime.ReadMemStats(&after)
	perRun := (after.TotalAlloc - before.TotalAlloc) / runs

	// The destination copy is unavoidable (payloadSize). A streaming
	// decoder adds growing intermediate buffers and a full copy on top.
	require.Less(
		t,
		perRun,
		uint64(payloadSize+payloadSize/2),
		"Decode allocated %d bytes per call for a %d byte payload",
		perRun,
		payloadSize,
	)
}

type retainingUnmarshaler struct {
	data []byte
}

func (r *retainingUnmarshaler) UnmarshalCBOR(data []byte) error {
	r.data = data
	return nil
}

// Decode documents that decoded values may keep references into the input.
func TestDecodeRetainedInputAliasesCallerSlice(t *testing.T) {
	t.Parallel()
	input := []byte{0x82, 0x01, 0x02}
	var dest retainingUnmarshaler
	n, err := cbor.Decode(input, &dest)
	require.NoError(t, err)
	require.Equal(t, len(input), n)
	require.Equal(t, []byte{0x82, 0x01, 0x02}, dest.data)
	input[1] = 0xff
	require.Equal(t, []byte{0x82, 0xff, 0x02}, dest.data)
}

// A *Value destination keeps its own copy, as the Decode doc states.
func TestDecodeValueDoesNotAliasCallerSlice(t *testing.T) {
	t.Parallel()
	input := []byte{0x82, 0x01, 0x02}
	var dest cbor.Value
	_, err := cbor.Decode(input, &dest)
	require.NoError(t, err)
	input[1] = 0xff
	require.Equal(t, []byte{0x82, 0x01, 0x02}, dest.Cbor())
}

func TestDecodeReportsZeroBytesOnError(t *testing.T) {
	t.Parallel()
	// Malformed: array header promises two items, only one present.
	var malformed any
	n, err := cbor.Decode([]byte{0x82, 0x01}, &malformed)
	require.Error(t, err)
	require.Equal(t, 0, n)
	// Well-formed item that does not fit the destination, with trailing data.
	var wrong string
	n, err = cbor.Decode([]byte{0x01, 0x02, 0x03}, &wrong)
	require.Error(t, err)
	require.Equal(t, 0, n)
}
