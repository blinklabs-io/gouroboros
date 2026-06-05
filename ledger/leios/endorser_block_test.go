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

package leios

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestLeiosEndorserBlockRoundTrip(t *testing.T) {
	block := LeiosEndorserBlock{
		TransactionReferences: []LeiosTransactionReference{
			{
				TransactionHash: common.NewBlake2b256([]byte{0x01}),
				TransactionSize: 42,
			},
			{
				TransactionHash: common.NewBlake2b256([]byte{0x02}),
				TransactionSize: 65535,
			},
		},
	}

	blockCbor, err := cbor.Encode(&block)
	require.NoError(t, err)

	decoded, err := NewLeiosEndorserBlockFromCbor(blockCbor)
	require.NoError(t, err)
	require.Equal(t, block.TransactionReferences, decoded.TransactionReferences)
	require.Equal(t, blockCbor, decoded.Cbor())

	encodedAgain, err := cbor.Encode(decoded)
	require.NoError(t, err)
	require.Equal(t, blockCbor, encodedAgain)
}

func TestLeiosEndorserBlockRejectsDuplicateReferences(t *testing.T) {
	hash := common.NewBlake2b256([]byte{0x01})
	block := LeiosEndorserBlock{
		TransactionReferences: []LeiosTransactionReference{
			{TransactionHash: hash, TransactionSize: 1},
			{TransactionHash: hash, TransactionSize: 2},
		},
	}

	_, err := cbor.Encode(&block)
	require.ErrorContains(t, err, "duplicate")
}

func TestLeiosEndorserBlockRejectsEmptyReferences(t *testing.T) {
	block := LeiosEndorserBlock{}

	_, err := cbor.Encode(&block)
	require.ErrorContains(t, err, "at least one transaction reference")

	refMap, err := cbor.Encode(map[common.Blake2b256]uint16{})
	require.NoError(t, err)
	blockCbor, err := cbor.Encode([]any{cbor.RawMessage(refMap)})
	require.NoError(t, err)

	_, err = NewLeiosEndorserBlockFromCbor(blockCbor)
	require.ErrorContains(t, err, "at least one transaction reference")
}

func TestLeiosEndorserBlockRejectsZeroTxSize(t *testing.T) {
	block := LeiosEndorserBlock{
		TransactionReferences: []LeiosTransactionReference{
			{
				TransactionHash: common.NewBlake2b256([]byte{0x01}),
				TransactionSize: 0,
			},
		},
	}

	_, err := cbor.Encode(&block)
	require.ErrorContains(t, err, "zero size")

	refMap, err := cbor.Encode(map[common.Blake2b256]uint16{
		common.NewBlake2b256([]byte{0x01}): 0,
	})
	require.NoError(t, err)
	blockCbor, err := cbor.Encode([]any{cbor.RawMessage(refMap)})
	require.NoError(t, err)

	_, err = NewLeiosEndorserBlockFromCbor(blockCbor)
	require.ErrorContains(t, err, "zero size")
}

func TestLeiosEndorserBlockRejectsWrongEnvelopeSize(t *testing.T) {
	blockCbor, err := cbor.Encode([]any{})
	require.NoError(t, err)

	_, err = NewLeiosEndorserBlockFromCbor(blockCbor)
	require.ErrorContains(t, err, "expected 1 component")
}

func TestLeiosEndorserBlockRejectsTrailingBytes(t *testing.T) {
	block := LeiosEndorserBlock{
		TransactionReferences: []LeiosTransactionReference{
			{
				TransactionHash: common.NewBlake2b256([]byte{0x01}),
				TransactionSize: 42,
			},
		},
	}
	blockCbor, err := cbor.Encode(&block)
	require.NoError(t, err)
	blockCbor = append(blockCbor, 0xff)

	_, err = NewLeiosEndorserBlockFromCbor(blockCbor)
	require.ErrorContains(t, err, "trailing bytes")

	var decoded LeiosEndorserBlock
	err = decoded.UnmarshalCBOR(blockCbor)
	require.ErrorContains(t, err, "trailing bytes")
}

func TestLeiosEndorserBlockRejectsOversizedTxSize(t *testing.T) {
	hash := common.NewBlake2b256([]byte{0x01})
	refMap, err := cbor.Encode(map[common.Blake2b256]uint64{
		hash: 65536,
	})
	require.NoError(t, err)
	blockCbor, err := cbor.Encode([]any{cbor.RawMessage(refMap)})
	require.NoError(t, err)

	_, err = NewLeiosEndorserBlockFromCbor(blockCbor)
	require.ErrorContains(t, err, "exceeds uint16")
}
