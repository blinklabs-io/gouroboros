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

package byron_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestMainBlockBodyHashCheckedValid(t *testing.T) {
	merkleRoot := bytes.Repeat([]byte{0x3c}, common.Blake2b256Size)
	header := &byron.ByronMainBlockHeader{
		BodyProof: []any{
			[]any{
				uint64(2),
				merkleRoot,
				bytes.Repeat([]byte{0x4d}, common.Blake2b256Size),
			},
			[]any{uint64(3), bytes.Repeat([]byte{0x00}, 32)},
			bytes.Repeat([]byte{0x01}, common.Blake2b256Size),
			bytes.Repeat([]byte{0x02}, common.Blake2b256Size),
		},
	}

	hash, err := header.BlockBodyHashChecked()
	require.NoError(t, err)
	require.Equal(t, merkleRoot, hash.Bytes())
	require.Equal(t, hash, header.BlockBodyHash())
}

func TestMainBlockBodyHashCheckedMalformed(t *testing.T) {
	testCases := []struct {
		name      string
		bodyProof any
	}{
		{
			name:      "nil proof",
			bodyProof: nil,
		},
		{
			name:      "proof is not an array",
			bodyProof: uint64(7),
		},
		{
			name:      "empty array",
			bodyProof: []any{},
		},
		{
			name:      "tx proof is not an array",
			bodyProof: []any{uint64(0), nil, nil, nil},
		},
		{
			name:      "tx proof too short",
			bodyProof: []any{[]any{uint64(0)}, nil, nil, nil},
		},
		{
			name: "merkle root is not bytes",
			bodyProof: []any{
				[]any{uint64(0), "root", nil}, nil, nil, nil,
			},
		},
		{
			name: "merkle root truncated",
			bodyProof: []any{
				[]any{uint64(0), bytes.Repeat([]byte{0x01}, 16), nil},
				nil, nil, nil,
			},
		},
		{
			name:      "raw bytes of the wrong length",
			bodyProof: bytes.Repeat([]byte{0x01}, 16),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			header := &byron.ByronMainBlockHeader{
				BodyProof: testCase.bodyProof,
			}
			hash, err := header.BlockBodyHashChecked()
			require.ErrorIs(t, err, byron.ErrMalformedBodyProof)
			require.Equal(t, common.Blake2b256{}, hash)
			// BlockBodyHash still stands in a zero hash for the
			// common.BlockHeader interface, which has no way to report this.
			require.Equal(t, common.Blake2b256{}, header.BlockBodyHash())
		})
	}
}

func TestEBBBodyHashCheckedValid(t *testing.T) {
	bodyHash := bytes.Repeat([]byte{0x7e}, common.Blake2b256Size)
	header := &byron.ByronEpochBoundaryBlockHeader{BodyProof: bodyHash}

	hash, err := header.BlockBodyHashChecked()
	require.NoError(t, err)
	require.Equal(t, bodyHash, hash.Bytes())
	require.Equal(t, hash, header.BlockBodyHash())
}

func TestEBBBodyHashCheckedMalformed(t *testing.T) {
	testCases := []struct {
		name      string
		bodyProof any
	}{
		{name: "nil proof", bodyProof: nil},
		{name: "proof is an array", bodyProof: []any{uint64(0)}},
		{
			name:      "proof truncated",
			bodyProof: bytes.Repeat([]byte{0x01}, 31),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			header := &byron.ByronEpochBoundaryBlockHeader{
				BodyProof: testCase.bodyProof,
			}
			hash, err := header.BlockBodyHashChecked()
			require.ErrorIs(t, err, byron.ErrMalformedBodyProof)
			require.Equal(t, common.Blake2b256{}, hash)
			require.Equal(t, common.Blake2b256{}, header.BlockBodyHash())
		})
	}
}

// TestEBBValidateBodyProofMalformed pins the distinction the checked
// accessor exists for: a body proof that cannot be parsed is reported as
// malformed, not as a mismatch against the zero hash that standing one in
// would otherwise produce.
func TestEBBValidateBodyProofMalformed(t *testing.T) {
	block := &byron.ByronEpochBoundaryBlock{
		BlockHeader: &byron.ByronEpochBoundaryBlockHeader{
			BodyProof: []any{uint64(0)},
		},
	}
	err := block.ValidateBodyProof()
	require.ErrorIs(t, err, byron.ErrMalformedBodyProof)
	require.NotErrorIs(t, err, byron.ErrBodyProofMismatch)
}

func TestBlockBodyHashCheckedNilReceivers(t *testing.T) {
	var mainBlock *byron.ByronMainBlock
	_, err := mainBlock.BlockBodyHashChecked()
	require.ErrorIs(t, err, byron.ErrMalformedBodyProof)

	var ebb *byron.ByronEpochBoundaryBlock
	_, err = ebb.BlockBodyHashChecked()
	require.ErrorIs(t, err, byron.ErrMalformedBodyProof)

	_, err = (&byron.ByronMainBlock{}).BlockBodyHashChecked()
	require.ErrorIs(t, err, byron.ErrMalformedBodyProof)
}
