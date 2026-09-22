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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

const hashFixtureURL = "https://example.com/pool.json"

// The fixture types below mirror the wire shape of the pool parameter query
// results without reusing the types under test, and hold the metadata hash as
// a plain byte slice so a fixture can carry any hash length the reference
// ledger allows.
type hashFixtureMetadata struct {
	cbor.StructAsArray
	Url          string
	MetadataHash []byte
}

type hashFixtureParams struct {
	cbor.StructAsArray
	Operator      ledger.Blake2b224
	VrfKeyHash    ledger.Blake2b256
	Pledge        uint64
	Cost          uint64
	Margin        *cbor.Rat
	RewardAccount ledger.Address
	PoolOwners    []ledger.Blake2b224
	Relays        []ledger.PoolRelay
	PoolMetadata  *hashFixtureMetadata
}

type hashFixtureStakePoolParamsResult struct {
	cbor.StructAsArray
	Results map[ledger.PoolId]hashFixtureParams
}

type hashFixturePoolStateResult struct {
	cbor.StructAsArray
	PState   map[ledger.Blake2b224]*hashFixtureParams
	FState   map[ledger.Blake2b224]*hashFixtureParams
	Retiring map[ledger.Blake2b224]uint64
	Deposits map[ledger.Blake2b224]uint64
}

func hashFixturePoolId() ledger.PoolId {
	var poolId ledger.PoolId
	for i := range poolId {
		poolId[i] = 0x03
	}
	return poolId
}

func hashFixtureOperator() ledger.Blake2b224 {
	return ledger.Blake2b224(hashFixturePoolId())
}

func hashFixtureVrfKeyHash() ledger.Blake2b256 {
	var hash ledger.Blake2b256
	for i := range hash {
		hash[i] = 0x04
	}
	return hash
}

// hashFixtureRewardAccount builds a mainnet reward address, which the Address
// decoder requires to be a 29-byte payload.
func hashFixtureRewardAccount(t *testing.T) ledger.Address {
	t.Helper()
	addrBytes := append([]byte{0xe1}, hashFixtureOperator().Bytes()...)
	addr, err := lcommon.NewAddressFromBytes(addrBytes)
	require.NoError(t, err)
	return addr
}

// hashFixtureMetadataHash builds a metadata hash of the requested byte length.
func hashFixtureMetadataHash(length int) []byte {
	hash := make([]byte, length)
	for i := range hash {
		hash[i] = byte(0x10 + i%0x70)
	}
	return hash
}

func hashFixturePoolParams(t *testing.T, hash []byte) hashFixtureParams {
	t.Helper()
	return hashFixtureParams{
		Operator:      hashFixtureOperator(),
		VrfKeyHash:    hashFixtureVrfKeyHash(),
		Pledge:        1_000_000,
		Cost:          340_000_000,
		Margin:        &cbor.Rat{Rat: big.NewRat(1, 10)},
		RewardAccount: hashFixtureRewardAccount(t),
		PoolOwners:    []ledger.Blake2b224{hashFixtureOperator()},
		Relays:        []ledger.PoolRelay{},
		PoolMetadata: &hashFixtureMetadata{
			Url:          hashFixtureURL,
			MetadataHash: hash,
		},
	}
}

func hashFixtureStakePoolParamsCBOR(t *testing.T, hash []byte) []byte {
	t.Helper()
	encoded, err := cbor.Encode(hashFixtureStakePoolParamsResult{
		Results: map[ledger.PoolId]hashFixtureParams{
			hashFixturePoolId(): hashFixturePoolParams(t, hash),
		},
	})
	require.NoError(t, err)
	return encoded
}

func hashFixturePoolStateParamsCBOR(t *testing.T, hash []byte) []byte {
	t.Helper()
	encoded, err := cbor.Encode(hashFixturePoolParams(t, hash))
	require.NoError(t, err)
	return encoded
}

func hashFixturePoolStateResultCBOR(t *testing.T, hash []byte) []byte {
	t.Helper()
	params := hashFixturePoolParams(t, hash)
	encoded, err := cbor.Encode(hashFixturePoolStateResult{
		PState: map[ledger.Blake2b224]*hashFixtureParams{
			hashFixtureOperator(): &params,
		},
		FState:   map[ledger.Blake2b224]*hashFixtureParams{},
		Retiring: map[ledger.Blake2b224]uint64{},
		Deposits: map[ledger.Blake2b224]uint64{},
	})
	require.NoError(t, err)
	return encoded
}

// poolMetadataHashLengths covers the lengths the reference ledger accepts: the
// POOL rule predicate PoolMedataHashTooBig rejects only hashes longer than 32
// bytes, and only from protocol version 5, so a registration recorded in
// ledger state can carry any of these.
var poolMetadataHashLengths = []int{2, 28, 31, 32, 33}

// TestStakePoolParamsResultDecodesAnyMetadataHashLength covers the
// GetStakePoolParams result path, whose inline metadata struct must not
// impose the fixed 32-byte width that ledger.Blake2b256 enforces.
func TestStakePoolParamsResultDecodesAnyMetadataHashLength(t *testing.T) {
	for _, length := range poolMetadataHashLengths {
		hash := hashFixtureMetadataHash(length)
		data := hashFixtureStakePoolParamsCBOR(t, hash)
		var result StakePoolParamsResult
		_, err := cbor.Decode(data, &result)
		require.NoError(t, err, "hash length %d", length)
		params, ok := result.Results[hashFixturePoolId()]
		require.True(t, ok, "hash length %d", length)
		require.NotNil(t, params.PoolMetadata, "hash length %d", length)
		require.Equal(
			t,
			lcommon.PoolMetadataHash(hash),
			params.PoolMetadata.MetadataHash,
			"hash length %d",
			length,
		)
		require.Equal(t, hashFixtureURL, params.PoolMetadata.Url)
	}
}

// TestPoolStateParamsDecodesAnyMetadataHashLength covers PoolStateParams
// directly, so a regression at this site cannot be masked by the
// StakePoolParamsResult tests.
func TestPoolStateParamsDecodesAnyMetadataHashLength(t *testing.T) {
	for _, length := range poolMetadataHashLengths {
		hash := hashFixtureMetadataHash(length)
		data := hashFixturePoolStateParamsCBOR(t, hash)
		var params PoolStateParams
		_, err := cbor.Decode(data, &params)
		require.NoError(t, err, "hash length %d", length)
		require.NotNil(t, params.PoolMetadata, "hash length %d", length)
		require.Equal(
			t,
			lcommon.PoolMetadataHash(hash),
			params.PoolMetadata.MetadataHash,
			"hash length %d",
			length,
		)
		require.Equal(t, hashFixtureURL, params.PoolMetadata.Url)
	}
}

// TestPoolStateResultDecodesShortMetadataHash reaches PoolStateParams through
// the map value the way Client.GetPoolState does.
func TestPoolStateResultDecodesShortMetadataHash(t *testing.T) {
	hash := hashFixtureMetadataHash(2)
	data := hashFixturePoolStateResultCBOR(t, hash)
	var result PoolStateResult
	_, err := cbor.Decode(data, &result)
	require.NoError(t, err)
	params, ok := result.PState[hashFixtureOperator()]
	require.True(t, ok)
	require.NotNil(t, params)
	require.NotNil(t, params.PoolMetadata)
	require.Equal(
		t,
		lcommon.PoolMetadataHash(hash),
		params.PoolMetadata.MetadataHash,
	)
}

// TestStakePoolParamsResultDecodesFieldsUnchanged asserts that widening the
// metadata hash type leaves every other field of a 32-byte result as it was,
// and that the result re-encodes to the bytes it was decoded from.
func TestStakePoolParamsResultDecodesFieldsUnchanged(t *testing.T) {
	hash := hashFixtureMetadataHash(32)
	data := hashFixtureStakePoolParamsCBOR(t, hash)
	var result StakePoolParamsResult
	_, err := cbor.Decode(data, &result)
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	params, ok := result.Results[hashFixturePoolId()]
	require.True(t, ok)
	require.Equal(t, hashFixtureOperator(), params.Operator)
	require.Equal(t, hashFixtureVrfKeyHash(), params.VrfKeyHash)
	require.Equal(t, uint(1_000_000), params.Pledge)
	require.Equal(t, uint(340_000_000), params.FixedCost)
	require.NotNil(t, params.Margin)
	require.Equal(t, big.NewRat(1, 10), params.Margin.Rat)
	require.Equal(
		t,
		hashFixtureRewardAccount(t).String(),
		params.RewardAccount.String(),
	)
	require.Equal(
		t,
		[]ledger.Blake2b224{hashFixtureOperator()},
		params.PoolOwners,
	)
	require.Empty(t, params.Relays)
	require.NotNil(t, params.PoolMetadata)
	require.Equal(t, hashFixtureURL, params.PoolMetadata.Url)
	require.Equal(
		t,
		lcommon.PoolMetadataHash(hash),
		params.PoolMetadata.MetadataHash,
	)
	reencoded, err := cbor.Encode(result)
	require.NoError(t, err)
	require.True(
		t,
		bytes.Equal(data, reencoded),
		"decoded result did not re-encode to the original bytes",
	)
}

// TestPoolStateParamsDecodesFieldsUnchanged is the PoolStateParams half of the
// 32-byte positive case.
func TestPoolStateParamsDecodesFieldsUnchanged(t *testing.T) {
	hash := hashFixtureMetadataHash(32)
	data := hashFixturePoolStateParamsCBOR(t, hash)
	var params PoolStateParams
	_, err := cbor.Decode(data, &params)
	require.NoError(t, err)
	require.Equal(t, hashFixtureOperator(), params.Operator)
	require.Equal(t, hashFixtureVrfKeyHash(), params.VrfKeyHash)
	require.Equal(t, uint64(1_000_000), params.Pledge)
	require.Equal(t, uint64(340_000_000), params.Cost)
	require.NotNil(t, params.Margin)
	require.Equal(t, big.NewRat(1, 10), params.Margin.Rat)
	require.Equal(
		t,
		hashFixtureRewardAccount(t).String(),
		params.RewardAccount.String(),
	)
	require.Equal(
		t,
		[]ledger.Blake2b224{hashFixtureOperator()},
		params.PoolOwners,
	)
	require.Empty(t, params.Relays)
	require.NotNil(t, params.PoolMetadata)
	require.Equal(t, hashFixtureURL, params.PoolMetadata.Url)
	require.Equal(
		t,
		lcommon.PoolMetadataHash(hash),
		params.PoolMetadata.MetadataHash,
	)
	reencoded, err := cbor.Encode(params)
	require.NoError(t, err)
	require.True(
		t,
		bytes.Equal(data, reencoded),
		"decoded params did not re-encode to the original bytes",
	)
}
