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

package localstatequery

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// Each reply collection must grow only with entries that decode. Sized from
// the peer-declared count, a reply of one-byte entries reserves a whole
// element per byte before the first entry fails.
func TestReplyCollectionsGrowAsEntriesDecode(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		elemSize uintptr
		decode   func([]byte) error
	}{
		{
			name:     "era history",
			data:     oneByteEntries(),
			elemSize: unsafe.Sizeof(EraHistoryResult{}),
			decode: func(data []byte) error {
				var r eraHistoryReply
				_, err := cbor.Decode(data, &r)
				return err
			},
		},
		{
			name: "era history null entries",
			data: concat(
				[]byte{
					0x99,
					governanceAllocEntries >> 8,
					governanceAllocEntries & 0xff,
				},
				bytes.Repeat([]byte{0xf6}, governanceAllocEntries),
			),
			elemSize: unsafe.Sizeof(EraHistoryResult{}),
			decode: func(data []byte) error {
				var r eraHistoryReply
				_, err := cbor.Decode(data, &r)
				return err
			},
		},
		{
			name: "ledger peer snapshot v1 pools",
			// [1, [origin, [...]]]
			data:     oneByteEntries(0x82, 0x01, 0x82, 0x81, 0x00),
			elemSize: unsafe.Sizeof(PoolLedgerPeers{}),
			decode: func(data []byte) error {
				var r LedgerPeerSnapshotResult
				_, err := cbor.Decode(data, &r)
				return err
			},
		},
		{
			name: "ledger peer snapshot v2 pools",
			data: oneByteEntries(
				0x82, 0x02, 0x83, 0x81, 0x00, 0x00,
			),
			elemSize: unsafe.Sizeof(cbor.RawMessage{}),
			decode: func(data []byte) error {
				var r LedgerPeerSnapshotResult
				_, err := cbor.Decode(data, &r)
				return err
			},
		},
		{
			name: "DRep delegators",
			// [{[2]: 258([...])}]: one always-abstain DRep.
			data: oneByteEntries(
				0x81, 0xa1, 0x81, 0x02, 0xd9, 0x01, 0x02,
			),
			elemSize: unsafe.Sizeof(lcommon.Credential{}),
			decode: func(data []byte) error {
				var r DRepDelegationsResult
				_, err := cbor.Decode(data, &r)
				return err
			},
		},
		{
			name:     "stake pools",
			data:     oneByteEntries(0x81, 0xd9, 0x01, 0x02),
			elemSize: unsafe.Sizeof(ledger.PoolId{}),
			decode: func(data []byte) error {
				var r StakePoolsResult
				_, err := cbor.Decode(data, &r)
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.decode(tc.data))
			result := testing.Benchmark(func(b *testing.B) {
				for range b.N {
					if err := tc.decode(tc.data); err == nil {
						b.Fatal("expected decode error")
					}
				}
			})
			t.Logf(
				"%d one-byte entries: %d bytes/op",
				governanceAllocEntries,
				result.AllocedBytesPerOp(),
			)
			require.Less(
				t,
				result.AllocedBytesPerOp(),
				int64(governanceAllocEntries*tc.elemSize/4),
			)
		})
	}
}
