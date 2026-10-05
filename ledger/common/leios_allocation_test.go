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
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLeiosMapHeaderDoesNotAllocateClaimedReferences(t *testing.T) {
	data := []byte{0xba, 0, 2, 0, 0}
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			var block LeiosEndorserBlock
			err := block.UnmarshalCBOR(data)
			if err == nil {
				b.Fatal("truncated references map accepted")
			}
		}
	})
	t.Logf(
		"five encoded bytes: %d allocated bytes/op",
		result.AllocedBytesPerOp(),
	)
	require.Less(
		t,
		result.AllocedBytesPerOp(),
		int64(32*1024),
		"a map header without entries must not allocate its claimed 131072 references",
	)
}

func TestLeiosReferencesFitEncodedInput(t *testing.T) {
	t.Parallel()
	const count = 10000
	data := []byte{0xb9, 0x27, 0x10}
	for i := range uint32(count) {
		data = append(data, 0x58, 0x20)
		hash := make([]byte, Blake2b256Size)
		binary.BigEndian.PutUint32(hash[len(hash)-4:], i)
		data = append(data, hash...)
		data = append(data, 1)
	}
	for _, wire := range [][]byte{data, append([]byte{0x81}, data...)} {
		block, err := NewLeiosEndorserBlockFromCbor(wire)
		require.NoError(t, err)
		require.Len(t, block.TransactionReferences, count)
		require.Equal(
			t,
			uint16(1),
			block.TransactionReferences[count-1].TransactionSize,
		)
		require.Equal(t, wire, block.Cbor())
	}
	for _, wire := range [][]byte{
		{0xa1}, {0xa1, 0x58, 0x20}, {0xb9, 0x27, 0x10},
	} {
		_, err := NewLeiosEndorserBlockFromCbor(wire)
		require.Error(t, err)
	}
}
