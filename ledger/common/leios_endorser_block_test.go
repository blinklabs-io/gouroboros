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
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestLeiosEndorserBlockRoundTrip(t *testing.T) {
	block := LeiosEndorserBlock{
		TransactionReferences: []LeiosTransactionReference{
			{
				TransactionHash: NewBlake2b256([]byte{0x01}),
				TransactionSize: 42,
			},
			{
				TransactionHash: NewBlake2b256([]byte{0x02}),
				TransactionSize: 70000,
			},
			{
				TransactionHash: NewBlake2b256([]byte{0x03}),
				TransactionSize: math.MaxUint32,
			},
		},
	}

	blockCbor, err := cbor.Encode(&block)
	require.NoError(t, err)
	require.Equal(
		t,
		byte(cbor.CborTypeMap|3),
		blockCbor[0],
		"an endorser block is a bare definite map",
	)

	decoded, err := NewLeiosEndorserBlockFromCbor(blockCbor)
	require.NoError(t, err)
	require.Equal(t, block.TransactionReferences, decoded.TransactionReferences)
	require.Equal(t, blockCbor, decoded.Cbor())

	encodedAgain, err := cbor.Encode(decoded)
	require.NoError(t, err)
	require.Equal(t, blockCbor, encodedAgain)
}

// The reference encoder writes encodeMapLen n followed by each
// (bytes32, word32) pair in endorser-block order, with no array wrapper.
func TestLeiosEndorserBlockEncodingMatchesReference(t *testing.T) {
	hash := make([]byte, Blake2b256Size)
	hash[0] = 0xaa
	block := LeiosEndorserBlock{
		TransactionReferences: []LeiosTransactionReference{
			{TransactionHash: NewBlake2b256(hash), TransactionSize: 300},
		},
	}
	want := append([]byte{0xa1, 0x58, 0x20}, hash...)
	want = append(want, 0x19, 0x01, 0x2c)

	got, err := block.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.Equal(t, Blake2b256Hash(want), block.Hash())

	decoded, err := NewLeiosEndorserBlockFromCbor(want)
	require.NoError(t, err)
	require.Equal(t, Blake2b256Hash(want), decoded.Hash())
}

// The reference memoises the bytes an endorser block arrived as and hashes
// those, so a block decoded from wider heads hashes as its wire bytes, not as
// its canonical re-encoding.
func TestLeiosEndorserBlockHashUsesReceivedBytes(t *testing.T) {
	t.Parallel()
	hash := make([]byte, Blake2b256Size)
	hash[0] = 0xaa
	canonical := append([]byte{0xa1, 0x58, 0x20}, hash...)
	canonical = append(canonical, 0x18, 0x26)
	tests := map[string][]byte{
		"canonical": canonical,
		"size with a four-byte argument": append(
			append([]byte{0xa1, 0x58, 0x20}, hash...),
			0x1a, 0x00, 0x00, 0x00, 0x26,
		),
		"map length with a one-byte argument": append(
			append([]byte{0xb8, 0x01, 0x58, 0x20}, hash...),
			0x18, 0x26,
		),
		"hash length with a two-byte argument": append(
			append([]byte{0xa1, 0x59, 0x00, 0x20}, hash...),
			0x18, 0x26,
		),
		"indefinite map": append(
			append([]byte{0xbf, 0x58, 0x20}, hash...),
			0x18, 0x26, 0xff,
		),
	}
	for name, wire := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			decoded, err := NewLeiosEndorserBlockFromCbor(wire)
			require.NoError(t, err)
			require.Equal(t, Blake2b256Hash(wire), decoded.Hash())
			encoded, err := decoded.MarshalCBOR()
			require.NoError(t, err)
			require.Equal(t, wire, encoded)
		})
	}
	wide, err := NewLeiosEndorserBlockFromCbor(
		tests["map length with a one-byte argument"],
	)
	require.NoError(t, err)
	require.NotEqual(t, Blake2b256Hash(canonical), wide.Hash())
}

// A block built from references carries no received bytes and hashes its
// canonical encoding.
func TestLeiosEndorserBlockHashWithoutReceivedBytesIsCanonical(t *testing.T) {
	t.Parallel()
	block := LeiosEndorserBlock{
		TransactionReferences: []LeiosTransactionReference{
			{TransactionHash: NewBlake2b256([]byte{0x01}), TransactionSize: 38},
		},
	}
	raw, err := block.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, Blake2b256Hash(raw), block.Hash())
	require.Equal(t, Blake2b256{}, LeiosEndorserBlock{}.Hash())
}

// The reference decodes each hash with decodeBytes and each size with
// decodeWord32, neither of which accepts an indefinite-length byte string or
// a tagged value.
func TestLeiosEndorserBlockRejectsNonDefiniteEntries(t *testing.T) {
	hash := make([]byte, Blake2b256Size)
	tests := map[string][]byte{
		"indefinite-length hash": append(
			append([]byte{0xa1, 0x5f, 0x58, 0x20}, hash...),
			0xff, 0x18, 0x26,
		),
		"tagged hash": append(
			append([]byte{0xa1, 0xd8, 0x18, 0x58, 0x20}, hash...),
			0x18, 0x26,
		),
		"bignum size": append(
			append([]byte{0xa1, 0x58, 0x20}, hash...),
			0xc2, 0x41, 0x26,
		),
		"negative size": append(
			append([]byte{0xa1, 0x58, 0x20}, hash...),
			0x20,
		),
	}
	for name, wire := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewLeiosEndorserBlockFromCbor(wire)
			require.Error(t, err)
		})
	}
}

func TestLeiosEndorserBlockRejectsArrayWrapper(t *testing.T) {
	block := LeiosEndorserBlock{
		TransactionReferences: []LeiosTransactionReference{
			{
				TransactionHash: NewBlake2b256([]byte{0x01}),
				TransactionSize: 42,
			},
		},
	}
	refMap, err := block.MarshalCBOR()
	require.NoError(t, err)
	wrapped, err := cbor.Encode([]any{cbor.RawMessage(refMap)})
	require.NoError(t, err)

	_, err = NewLeiosEndorserBlockFromCbor(wrapped)
	require.ErrorContains(t, err, "expected map")
}

func leiosIndefiniteMapWire(entries ...[2]byte) []byte {
	wire := []byte{0xbf}
	for _, e := range entries {
		hash := make([]byte, Blake2b256Size)
		hash[0] = e[0]
		wire = append(wire, 0x58, 0x20)
		wire = append(wire, hash...)
		wire = append(wire, 0x18, e[1])
	}
	return wire
}

func TestLeiosEndorserBlockAcceptsIndefiniteMap(t *testing.T) {
	t.Parallel()
	wire := append(leiosIndefiniteMapWire([2]byte{1, 5}, [2]byte{2, 6}), 0xff)
	block, err := NewLeiosEndorserBlockFromCbor(wire)
	require.NoError(t, err)
	require.Len(t, block.TransactionReferences, 2)
	require.Equal(t, uint32(5), block.TransactionReferences[0].TransactionSize)
	require.Equal(t, uint32(6), block.TransactionReferences[1].TransactionSize)
	require.NoError(t, block.Validate())
}

func TestLeiosEndorserBlockRejectsMalformedIndefiniteMap(t *testing.T) {
	t.Parallel()
	one := leiosIndefiniteMapWire([2]byte{1, 5})
	cut := one[:len(one)-2]
	tests := map[string][]byte{
		"missing break":  one,
		"empty":          {0xbf, 0xff},
		"only header":    {0xbf},
		"trailing bytes": append(append([]byte{}, one...), 0xff, 0x00),
		"duplicate": append(
			leiosIndefiniteMapWire([2]byte{1, 5}, [2]byte{1, 6}),
			0xff,
		),
		"break before value": append(append([]byte{}, cut...), 0xff),
		"indefinite hash":    {0xbf, 0x5f, 0x40, 0xff, 0x01, 0xff},
		"non-uint size":      append(append([]byte{}, cut...), 0x40, 0xff),
		"oversized size": append(
			append([]byte{}, cut...),
			0x1b, 0, 0, 0, 1, 0, 0, 0, 0, 0xff,
		),
	}
	for name, wire := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := NewLeiosEndorserBlockFromCbor(wire)
			require.Error(t, err)
			// cbor.Decode refuses ill-formed CBOR before UnmarshalCBOR
			// runs, so call it directly to reach the decoder's own checks.
			var block LeiosEndorserBlock
			require.Error(t, block.UnmarshalCBOR(wire))
		})
	}
	_, err := NewLeiosEndorserBlockFromCbor(
		append(append([]byte{}, one...), 0xff, 0x00),
	)
	require.ErrorContains(t, err, "trailing bytes")
	_, err = NewLeiosEndorserBlockFromCbor([]byte{0xbf, 0xff})
	require.ErrorContains(t, err, "at least one transaction reference")
}

func TestLeiosEndorserBlockIndefiniteMapCountBound(t *testing.T) {
	t.Parallel()
	wire := []byte{0xbf}
	for i := range LeiosMaxTxsPerEndorserBlock + 1 {
		hash := make([]byte, Blake2b256Size)
		binary.BigEndian.PutUint32(hash[len(hash)-4:], uint32(i)) // #nosec G115
		wire = append(wire, 0x58, 0x20)
		wire = append(wire, hash...)
		wire = append(wire, 0x01)
	}
	wire = append(wire, 0xff)
	_, err := NewLeiosEndorserBlockFromCbor(wire)
	require.ErrorContains(t, err, "not in [1, 17142]")
}

func TestLeiosEndorserBlockRejectsDuplicateReferences(t *testing.T) {
	hash := NewBlake2b256([]byte{0x01})
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

	_, err = NewLeiosEndorserBlockFromCbor([]byte{0xa0})
	require.ErrorContains(t, err, "at least one transaction reference")
}

// The reference sizes are Word32 and its decoder rejects only an empty map.
func TestLeiosEndorserBlockAcceptsZeroTxSize(t *testing.T) {
	t.Parallel()
	block := LeiosEndorserBlock{
		TransactionReferences: []LeiosTransactionReference{
			{TransactionHash: NewBlake2b256([]byte{0x01}), TransactionSize: 0},
		},
	}
	require.NoError(t, block.Validate())
	raw, err := block.MarshalCBOR()
	require.NoError(t, err)

	decoded, err := NewLeiosEndorserBlockFromCbor(raw)
	require.NoError(t, err)
	require.Equal(
		t,
		uint32(0),
		decoded.TransactionReferences[0].TransactionSize,
	)
	require.Equal(t, Blake2b256Hash(raw), decoded.Hash())
}

func TestLeiosEndorserBlockRejectsTrailingBytes(t *testing.T) {
	block := LeiosEndorserBlock{
		TransactionReferences: []LeiosTransactionReference{
			{
				TransactionHash: NewBlake2b256([]byte{0x01}),
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
	hash := NewBlake2b256([]byte{0x01})
	refMap, err := cbor.Encode(map[Blake2b256]uint64{
		hash: math.MaxUint32 + 1,
	})
	require.NoError(t, err)

	_, err = NewLeiosEndorserBlockFromCbor(refMap)
	require.ErrorContains(t, err, "exceeds uint32")
}

func leiosEndorserBlockWire(count int) []byte {
	data := encodeCborMapHeader(uint64(count))
	for i := range count {
		data = append(data, 0x58, 0x20)
		hash := make([]byte, Blake2b256Size)
		binary.BigEndian.PutUint32(hash[len(hash)-4:], uint32(i)) // #nosec G115
		data = append(data, hash...)
		data = append(data, 1)
	}
	return data
}

func TestLeiosEndorserBlockReferenceCountBounds(t *testing.T) {
	t.Parallel()
	block, err := NewLeiosEndorserBlockFromCbor(
		leiosEndorserBlockWire(LeiosMaxTxsPerEndorserBlock),
	)
	require.NoError(t, err)
	require.Len(t, block.TransactionReferences, LeiosMaxTxsPerEndorserBlock)

	_, err = NewLeiosEndorserBlockFromCbor(
		leiosEndorserBlockWire(LeiosMaxTxsPerEndorserBlock + 1),
	)
	require.ErrorContains(t, err, "not in [1, 17142]")

	refs := make([]LeiosTransactionReference, LeiosMaxTxsPerEndorserBlock+1)
	for i := range refs {
		hash := make([]byte, Blake2b256Size)
		binary.BigEndian.PutUint32(hash[len(hash)-4:], uint32(i)) // #nosec G115
		refs[i] = LeiosTransactionReference{
			TransactionHash: NewBlake2b256(hash),
			TransactionSize: 1,
		}
	}
	_, err = LeiosEndorserBlock{TransactionReferences: refs}.MarshalCBOR()
	require.ErrorContains(t, err, "not in [1, 17142]")
}
