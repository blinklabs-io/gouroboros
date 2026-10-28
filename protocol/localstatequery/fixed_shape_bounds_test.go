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
	"runtime"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oversizedArray is a well-formed definite array of 1000 one-byte items. It
// is far larger than any fixed-shape reply array, so a decoder that sizes a
// slice from it before checking the expected cardinality allocates for it.
func oversizedArray() []byte {
	return append([]byte{0x99, 0x03, 0xe8}, make([]byte, 1000)...)
}

// cat concatenates CBOR fragments.
func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// requireBoundRejected asserts that err came from the array-length bound
// applied before decoding, not from the later cardinality check.
func requireBoundRejected(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum is")
}

func TestFixedShapeRepliesRejectOversizedArraysBeforeDecoding(t *testing.T) {
	t.Parallel()
	big := oversizedArray()
	hash32 := cat([]byte{0x58, 0x20}, make([]byte, 32))
	// Snapshot v2/v3 carry [version, [point, magic, pools]].
	snapshotWithPool := func(version byte, pool []byte) []byte {
		return cat(
			[]byte{0x82, version, 0x83, 0x83, 0x01, 0x00},
			hash32,
			[]byte{0x00, 0x81},
			pool,
		)
	}
	tests := []struct {
		name   string
		decode func() error
	}{
		{"rational", func() error {
			_, err := decodeRational(big)
			return err
		}},
		{"relay access point", func() error {
			var v RelayAccessPoint
			return v.UnmarshalCBOR(big)
		}},
		{"pool ledger peers", func() error {
			var v PoolLedgerPeers
			return v.UnmarshalCBOR(big)
		}},
		{"pool ledger peers detail", func() error {
			var v PoolLedgerPeersDetail
			return v.UnmarshalCBOR(big)
		}},
		{"snapshot outer", func() error {
			var v LedgerPeerSnapshotResult
			return v.UnmarshalCBOR(big)
		}},
		{"snapshot v1 inner", func() error {
			var v LedgerPeerSnapshotResult
			return v.UnmarshalCBOR(cat([]byte{0x82, 0x01}, big))
		}},
		{"snapshot v2 inner", func() error {
			var v LedgerPeerSnapshotResult
			return v.UnmarshalCBOR(cat([]byte{0x82, 0x02}, big))
		}},
		{"snapshot v2 pool", func() error {
			var v LedgerPeerSnapshotResult
			return v.UnmarshalCBOR(snapshotWithPool(0x02, big))
		}},
		{"snapshot v3 pool", func() error {
			var v LedgerPeerSnapshotResult
			return v.UnmarshalCBOR(snapshotWithPool(0x03, big))
		}},
		{"snapshot point", func() error {
			_, _, err := decodeSnapshotPoint(big)
			return err
		}},
		{"IPv4 octets", func() error {
			_, err := decodeIPWords(big, 4, 8)
			return err
		}},
		{"IPv6 words", func() error {
			_, err := decodeIPWords(big, 8, 16)
			return err
		}},
		{"genesis config", func() error {
			var v GenesisConfigResult
			return v.UnmarshalCBOR(big)
		}},
		{"genesis rational", func() error {
			var v genesisRational
			return v.UnmarshalCBOR(big)
		}},
		{"individual pool stake", func() error {
			var v IndividualPoolStake
			return v.UnmarshalCBOR(big)
		}},
		{"individual pool stake BLS key", func() error {
			var v IndividualPoolStake
			return v.UnmarshalCBOR(cat(
				[]byte{0x84, 0xd8, 0x1e, 0x82, 0x01, 0x02, 0x01},
				hash32,
				big,
			))
		}},
		{"ledger peer snapshot query", func() error {
			var v ShelleyGetLedgerPeerSnapshotQuery
			return v.UnmarshalCBOR(big)
		}},
		{"committee wrapper", func() error {
			g := GovStateResult{Committee: big}
			_, err := g.DecodeCommittee()
			return err
		}},
		{"DRep state anchor", func() error {
			// [expiry, anchor, deposit, delegators] with an oversized anchor.
			var v DRepStateEntry
			return v.UnmarshalCBOR(cat(
				[]byte{0x84, 0x00},
				big,
				[]byte{0x00, 0xd9, 0x01, 0x02, 0x80},
			))
		}},
		{"DRep keyed result wrapper", func() error {
			return decodeDRepKeyedResult(
				big,
				"DRep test",
				func(*cbor.StreamDecoder, lcommon.Drep) error { return nil },
			)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireBoundRejected(t, tt.decode())
		})
	}
}

// TestDistributionRepliesRejectOversizedRationalBeforeDecoding feeds each
// pool's stake fraction as a million-item array that a fixed-shape check
// rejects: the fraction itself as an indefinite-length array, which declares
// no count, or a two-item fraction whose numerator is the array. A decoder
// that builds those items before checking them allocates about 16 MB from a
// 1 MB reply.
func TestDistributionRepliesRejectOversizedRationalBeforeDecoding(
	t *testing.T,
) {
	poolID := cat([]byte{0x58, 0x1c}, make([]byte, 28))
	vrfHash := cat([]byte{0x58, 0x20}, make([]byte, 32))
	million := make([]byte, 1_000_000)
	fractions := map[string][]byte{
		"indefinite fraction": cat(
			[]byte{0xd8, 0x1e, 0x9f},
			million,
			[]byte{0xff},
		),
		"array numerator": cat(
			[]byte{0xd8, 0x1e, 0x82, 0x9a, 0x00, 0x0f, 0x42, 0x40},
			million,
			[]byte{0x01},
		),
	}
	results := map[string]func() any{
		"pool distribution":  func() any { return &PoolDistrResult{} },
		"stake distribution": func() any { return &StakeDistributionResult{} },
	}
	for fractionName, stakeFraction := range fractions {
		// [ { pool_id => [ stake_fraction, vrf_hash ] } ]
		reply := cat(
			[]byte{0x81, 0xa1},
			poolID,
			[]byte{0x82},
			stakeFraction,
			vrfHash,
		)
		for resultName, newResult := range results {
			t.Run(resultName+"/"+fractionName, func(t *testing.T) {
				result := newResult()
				var before, after runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&before)
				_, err := cbor.Decode(reply, result)
				runtime.ReadMemStats(&after)

				require.ErrorContains(t, err, "cbor.Rat")
				require.Less(
					t,
					after.TotalAlloc-before.TotalAlloc,
					uint64(4<<20),
				)
			})
		}
	}
}
