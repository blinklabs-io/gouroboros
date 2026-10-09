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
