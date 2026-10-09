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
	"bytes"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixedLengthByteStringDecode(t *testing.T) {
	tests := []struct {
		name   string
		size   int
		decode func([]byte) error
	}{
		{
			name: "blake2b-256",
			size: Blake2b256Size,
			decode: func(data []byte) error {
				var value Blake2b256
				_, err := cbor.Decode(data, &value)
				return err
			},
		},
		{
			name: "blake2b-224",
			size: Blake2b224Size,
			decode: func(data []byte) error {
				var value Blake2b224
				_, err := cbor.Decode(data, &value)
				return err
			},
		},
		{
			name: "blake2b-160",
			size: Blake2b160Size,
			decode: func(data []byte) error {
				var value Blake2b160
				_, err := cbor.Decode(data, &value)
				return err
			},
		},
		{
			name: "pool ID",
			size: Blake2b224Size,
			decode: func(data []byte) error {
				var value PoolId
				_, err := cbor.Decode(data, &value)
				return err
			},
		},
		{
			name: "issuer verification key",
			size: Blake2b256Size,
			decode: func(data []byte) error {
				var value IssuerVkey
				_, err := cbor.Decode(data, &value)
				return err
			},
		},
	}
	for _, test := range tests {
		for _, delta := range []int{-1, 0, 1} {
			wantErr := delta != 0
			t.Run(test.name+map[int]string{-1: "/short", 0: "/exact", 1: "/long"}[delta], func(t *testing.T) {
				wire, err := cbor.Encode(bytes.Repeat([]byte{0xaa}, test.size+delta))
				require.NoError(t, err)
				if wantErr {
					assert.Error(t, test.decode(wire))
				} else {
					assert.NoError(t, test.decode(wire))
				}
			})
		}
	}
}

func TestValidateCBORArrayLength(t *testing.T) {
	for _, test := range []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "definite", data: []byte{0x82, 0x01, 0x02}},
		{name: "indefinite", data: []byte{0x9f, 0x01, 0x02, 0xff}},
		{name: "short", data: []byte{0x81, 0x01}, wantErr: true},
		{name: "long", data: []byte{0x83, 0x01, 0x02, 0x03}, wantErr: true},
		{name: "non-array", data: []byte{0xa0}, wantErr: true},
		{name: "trailing", data: []byte{0x82, 0x01, 0x02, 0x00}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateCBORArrayLength(test.data, 2, "test value")
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExUnitsUnmarshalCBORArrayLength(t *testing.T) {
	valid := []byte{0x82, 0x01, 0x02}
	var units ExUnits
	require.NoError(t, units.UnmarshalCBOR(valid))
	assert.Equal(t, int64(1), units.Memory)
	assert.Equal(t, int64(2), units.Steps)

	var indefinite ExUnits
	require.NoError(t, indefinite.UnmarshalCBOR([]byte{0x9f, 0x01, 0x02, 0xff}))
	assert.Equal(t, units, indefinite)

	var extra ExUnits
	require.Error(t, extra.UnmarshalCBOR([]byte{0x83, 0x01, 0x02, 0x03}))
}

func TestGovActionIdUnmarshalCBORRejectsExtraFields(t *testing.T) {
	encoded, err := cbor.Encode([]any{make([]byte, Blake2b256Size), uint16(1), uint8(0)})
	require.NoError(t, err)
	var id GovActionId
	require.Error(t, id.UnmarshalCBOR(encoded))
}

func TestGovActionIdUnmarshalCBORRejectsIndefiniteArray(t *testing.T) {
	encoded, err := cbor.Encode([]any{make([]byte, Blake2b256Size), uint16(1)})
	require.NoError(t, err)
	encoded[0] = 0x9f
	encoded = append(encoded, 0xff)

	var id GovActionId
	require.ErrorContains(
		t,
		id.UnmarshalCBOR(encoded),
		"definite-length CBOR array",
	)
}

func TestGovAnchorUnmarshalCBORRejectsInvalidUTF8(t *testing.T) {
	encoded, err := cbor.Encode([]any{
		cbor.RawMessage{0x61, 0xff},
		make([]byte, Blake2b256Size),
	})
	require.NoError(t, err)
	var anchor GovAnchor
	require.ErrorContains(t, anchor.UnmarshalCBOR(encoded), "invalid UTF-8")
}

func TestGovAnchorUnmarshalCBORRejectsIndefiniteArray(t *testing.T) {
	encoded, err := cbor.Encode([]any{"https://example.com", make([]byte, Blake2b256Size)})
	require.NoError(t, err)
	encoded[0] = 0x9f
	encoded = append(encoded, 0xff)

	var anchor GovAnchor
	require.ErrorContains(
		t,
		anchor.UnmarshalCBOR(encoded),
		"definite-length CBOR array",
	)
}

func TestFixedHashDecodeRejectsAliasedWireValue(t *testing.T) {
	exact := bytes.Repeat([]byte{0xaa}, Blake2b256Size)
	long := append(bytes.Clone(exact), 0xbb)
	exactWire, err := cbor.Encode(exact)
	require.NoError(t, err)
	longWire, err := cbor.Encode(long)
	require.NoError(t, err)

	var exactHash Blake2b256
	_, err = cbor.Decode(exactWire, &exactHash)
	require.NoError(t, err)
	assert.Equal(t, Blake2b256(exact), exactHash)

	var longHash Blake2b256
	_, err = cbor.Decode(longWire, &longHash)
	assert.Error(t, err)
}

func TestFixedHashDecodeIndefiniteByteString(t *testing.T) {
	for _, size := range []int{31, 32, 33} {
		t.Run(map[int]string{31: "short", 32: "exact", 33: "long"}[size], func(t *testing.T) {
			firstChunkSize := size / 2
			secondChunkSize := size - firstChunkSize
			wire := []byte{0x5f, byte(0x40 + firstChunkSize)}
			wire = append(wire, bytes.Repeat([]byte{0xaa}, firstChunkSize)...)
			wire = append(wire, byte(0x40+secondChunkSize))
			wire = append(wire, bytes.Repeat([]byte{0xbb}, secondChunkSize)...)
			wire = append(wire, 0xff)

			var hash Blake2b256
			_, err := cbor.Decode(wire, &hash)
			assert.Error(t, err)
		})
	}
}

func TestFixedLengthRawArrayFields(t *testing.T) {
	tests := []struct {
		name   string
		size   int
		decode func([]byte) error
	}{
		{
			name: "nonce",
			size: Blake2b256Size,
			decode: func(hash []byte) error {
				wire, err := cbor.Encode([]any{NonceTypeNonce, hash})
				require.NoError(t, err)
				var value Nonce
				_, err = cbor.Decode(wire, &value)
				return err
			},
		},
		{
			name: "voter",
			size: Blake2b224Size,
			decode: func(hash []byte) error {
				wire, err := cbor.Encode([]any{uint8(0), hash})
				require.NoError(t, err)
				var value Voter
				_, err = cbor.Decode(wire, &value)
				return err
			},
		},
		{
			name: "governance anchor",
			size: Blake2b256Size,
			decode: func(hash []byte) error {
				wire, err := cbor.Encode([]any{"https://example.com", hash})
				require.NoError(t, err)
				var value GovAnchor
				_, err = cbor.Decode(wire, &value)
				return err
			},
		},
		{
			name: "governance action ID",
			size: Blake2b256Size,
			decode: func(hash []byte) error {
				wire, err := cbor.Encode([]any{hash, uint32(0)})
				require.NoError(t, err)
				var value GovActionId
				_, err = cbor.Decode(wire, &value)
				return err
			},
		},
	}
	for _, test := range tests {
		for _, delta := range []int{-1, 0, 1} {
			wantErr := delta != 0
			t.Run(test.name+map[int]string{-1: "/short", 0: "/exact", 1: "/long"}[delta], func(t *testing.T) {
				err := test.decode(bytes.Repeat([]byte{0xbb}, test.size+delta))
				if wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	}
}

func TestMultiAssetWireBounds(t *testing.T) {
	tests := []struct {
		name       string
		policySize int
		assetSize  int
	}{
		{name: "short policy ID", policySize: Blake2b224Size - 1, assetSize: 0},
		{name: "long policy ID", policySize: Blake2b224Size + 1, assetSize: 0},
		{name: "maximum asset name", policySize: Blake2b224Size, assetSize: 32},
		{name: "long asset name", policySize: Blake2b224Size, assetSize: 33},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := cbor.Encode(
				map[cbor.ByteString]map[cbor.ByteString]*big.Int{
					cbor.NewByteString(bytes.Repeat([]byte{0x11}, test.policySize)): {
						cbor.NewByteString(bytes.Repeat([]byte{0x22}, test.assetSize)): big.NewInt(1),
					},
				},
			)
			require.NoError(t, err)
			var value MultiAsset[MultiAssetTypeOutput]
			err = value.UnmarshalCBOR(encoded)
			if test.policySize == Blake2b224Size && test.assetSize <= 32 {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestMultiAssetLenientDecodeRejectsLongAssetName(t *testing.T) {
	policy := bytes.Repeat([]byte{0x11}, Blake2b224Size)
	name := bytes.Repeat([]byte{0x22}, 33)
	// Duplicate inner keys force the pre-Conway last-wins decoder.
	wire := append([]byte{0xa1, 0x58, 0x1c}, policy...)
	wire = append(wire, 0xa2, 0x58, 0x21)
	wire = append(wire, name...)
	wire = append(wire, 0x01, 0x58, 0x21)
	wire = append(wire, name...)
	wire = append(wire, 0x02)

	var value MultiAsset[MultiAssetTypeMint]
	assert.Error(t, value.UnmarshalCBOR(wire))
}
