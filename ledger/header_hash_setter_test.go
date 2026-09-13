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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

type headerHashTestValue interface {
	Hash() common.Blake2b256
	SetCbor([]byte)
	SetCborReference([]byte)
}

func TestHeaderHashSettersResetCache(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		newHeader  func() headerHashTestValue
		copyHeader func(headerHashTestValue) headerHashTestValue
		prefix     []byte
	}{
		{
			name: "shelley",
			newHeader: func() headerHashTestValue {
				return &shelley.ShelleyBlockHeader{}
			},
			copyHeader: func(h headerHashTestValue) headerHashTestValue {
				copy := *h.(*shelley.ShelleyBlockHeader)
				return &copy
			},
		},
		{
			name: "babbage",
			newHeader: func() headerHashTestValue {
				return &babbage.BabbageBlockHeader{}
			},
			copyHeader: func(h headerHashTestValue) headerHashTestValue {
				copy := *h.(*babbage.BabbageBlockHeader)
				return &copy
			},
		},
		{
			name: "byron main",
			newHeader: func() headerHashTestValue {
				return &byron.ByronMainBlockHeader{}
			},
			copyHeader: func(h headerHashTestValue) headerHashTestValue {
				copy := *h.(*byron.ByronMainBlockHeader)
				return &copy
			},
			prefix: []byte{0x82, byron.BlockTypeByronMain},
		},
		{
			name: "byron ebb",
			newHeader: func() headerHashTestValue {
				return &byron.ByronEpochBoundaryBlockHeader{}
			},
			copyHeader: func(h headerHashTestValue) headerHashTestValue {
				copy := *h.(*byron.ByronEpochBoundaryBlockHeader)
				return &copy
			},
			prefix: []byte{0x82, byron.BlockTypeByronEbb},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, setter := range []struct {
				name string
				set  func(headerHashTestValue, []byte)
			}{
				{
					name: "SetCbor",
					set:  func(h headerHashTestValue, data []byte) { h.SetCbor(data) },
				},
				{
					name: "SetCborReference",
					set: func(h headerHashTestValue, data []byte) {
						h.SetCborReference(data)
					},
				},
			} {
				t.Run(setter.name, func(t *testing.T) {
					t.Parallel()
					first := []byte{0x01, 0x02, 0x03}
					second := []byte{0xa0, 0xbb, 0xcc, 0xdd}
					header := test.newHeader()
					setter.set(header, first)
					firstHash := headerHash(test.prefix, first)
					require.Equal(t, firstHash, header.Hash())

					copyHeader := test.copyHeader(header)
					require.Equal(t, firstHash, copyHeader.Hash())

					setter.set(header, second)
					require.Equal(t, headerHash(test.prefix, second), header.Hash())
					require.Equal(t, firstHash, copyHeader.Hash())

					clearedCopy := test.copyHeader(header)
					setter.set(header, nil)
					require.Equal(t, headerHash(test.prefix, nil), header.Hash())
					require.Equal(t, headerHash(test.prefix, second), clearedCopy.Hash())
				})
			}
		})
	}
}

func headerHash(prefix, cborData []byte) common.Blake2b256 {
	data := append(append([]byte(nil), prefix...), cborData...)
	return common.Blake2b256Hash(data)
}
