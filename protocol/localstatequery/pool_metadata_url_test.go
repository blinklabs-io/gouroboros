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
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

const queryPoolMetadataURLMaxBytes = 128

// The fixture types below mirror the wire shape of the pool parameter query
// results without reusing the types under test, so a decoder change cannot
// silently move the fixture with it.
type fixturePoolMetadata struct {
	cbor.StructAsArray
	Url          string
	MetadataHash ledger.Blake2b256
}

type fixturePoolParams struct {
	cbor.StructAsArray
	Operator      ledger.Blake2b224
	VrfKeyHash    ledger.Blake2b256
	Pledge        uint64
	Cost          uint64
	Margin        *cbor.Rat
	RewardAccount ledger.Address
	PoolOwners    []ledger.Blake2b224
	Relays        []ledger.PoolRelay
	PoolMetadata  *fixturePoolMetadata
}

type fixtureStakePoolParamsResult struct {
	cbor.StructAsArray
	Results map[ledger.PoolId]fixturePoolParams
}

type fixturePoolStateResult struct {
	cbor.StructAsArray
	PState   map[ledger.Blake2b224]*fixturePoolParams
	FState   map[ledger.Blake2b224]*fixturePoolParams
	Retiring map[ledger.Blake2b224]uint64
	Deposits map[ledger.Blake2b224]uint64
}

func testPoolId() ledger.PoolId {
	var poolId ledger.PoolId
	for i := range poolId {
		poolId[i] = 0x01
	}
	return poolId
}

func testPoolOperator() ledger.Blake2b224 {
	return ledger.Blake2b224(testPoolId())
}

func testPoolMetadataHash() ledger.Blake2b256 {
	var hash ledger.Blake2b256
	for i := range hash {
		hash[i] = 0x02
	}
	return hash
}

func testPoolMetadataHashBytes() []byte {
	hash := testPoolMetadataHash()
	return hash[:]
}

// testRewardAccount builds a mainnet reward address, which the Address
// decoder requires to be a 29-byte payload.
func testRewardAccount(t *testing.T) ledger.Address {
	t.Helper()
	addrBytes := append([]byte{0xe1}, testPoolOperator().Bytes()...)
	addr, err := lcommon.NewAddressFromBytes(addrBytes)
	require.NoError(t, err)
	return addr
}

func testPoolURL(totalBytes int) string {
	const prefix = "https://example.com/"
	return prefix + strings.Repeat("a", totalBytes-len(prefix))
}

// testPoolParams builds pool registration parameters carrying url. A nil
// metadata pointer is produced by passing an empty url.
func testPoolParams(t *testing.T, url string) fixturePoolParams {
	t.Helper()
	params := fixturePoolParams{
		Operator:      testPoolOperator(),
		VrfKeyHash:    testPoolMetadataHash(),
		Pledge:        1_000_000,
		Cost:          340_000_000,
		Margin:        &cbor.Rat{Rat: big.NewRat(1, 10)},
		RewardAccount: testRewardAccount(t),
		PoolOwners:    []ledger.Blake2b224{testPoolOperator()},
		Relays:        []ledger.PoolRelay{},
	}
	if url != "" {
		params.PoolMetadata = &fixturePoolMetadata{
			Url:          url,
			MetadataHash: testPoolMetadataHash(),
		}
	}
	return params
}

// decodePoolStateResult decodes a pool state result the way Client.GetPoolState
// does, so the test proves the library reaches the PoolStateParams decoder
// through the map value rather than calling it directly.
func decodePoolStateResult(data []byte) (PoolStateResult, error) {
	var result PoolStateResult
	_, err := cbor.Decode(data, &result)
	return result, err
}

func stakePoolParamsCBOR(t *testing.T, url string) []byte {
	t.Helper()
	encoded, err := cbor.Encode(fixtureStakePoolParamsResult{
		Results: map[ledger.PoolId]fixturePoolParams{
			testPoolId(): testPoolParams(t, url),
		},
	})
	require.NoError(t, err)
	return encoded
}

func stakePoolParamsCBORMixed(t *testing.T) []byte {
	t.Helper()
	badPool := testPoolId()
	badPool[0] = 0x02
	encoded, err := cbor.Encode(fixtureStakePoolParamsResult{
		Results: map[ledger.PoolId]fixturePoolParams{
			testPoolId(): testPoolParams(t, "https://valid.example/"),
			badPool:      testPoolParams(t, testPoolURL(129)),
		},
	})
	require.NoError(t, err)
	return encoded
}

func poolStateCBOR(t *testing.T, url string) []byte {
	t.Helper()
	params := testPoolParams(t, url)
	encoded, err := cbor.Encode(fixturePoolStateResult{
		PState: map[ledger.Blake2b224]*fixturePoolParams{
			testPoolOperator(): &params,
		},
		FState:   map[ledger.Blake2b224]*fixturePoolParams{},
		Retiring: map[ledger.Blake2b224]uint64{},
		Deposits: map[ledger.Blake2b224]uint64{},
	})
	require.NoError(t, err)
	return encoded
}

// TestStakePoolParamsResultURLBound covers the CDDL rule
// url = text .size (0 .. 128) on the GetStakePoolParams result, whose inline
// metadata struct bypasses common.PoolMetadata.UnmarshalCBOR.
func TestStakePoolParamsResultURLBound(t *testing.T) {
	atLimit := testPoolURL(queryPoolMetadataURLMaxBytes)
	require.Len(t, atLimit, queryPoolMetadataURLMaxBytes)

	var accepted StakePoolParamsResult
	require.NoError(
		t,
		accepted.UnmarshalCBOR(stakePoolParamsCBOR(t, atLimit)),
	)
	params, ok := accepted.Results[testPoolId()]
	require.True(t, ok)
	require.NotNil(t, params.PoolMetadata)
	require.Equal(t, atLimit, params.PoolMetadata.Url)
	require.Equal(
		t,
		testPoolMetadataHashBytes(),
		[]byte(params.PoolMetadata.MetadataHash[:]),
	)

	overLimit := testPoolURL(queryPoolMetadataURLMaxBytes + 1)
	require.Len(t, overLimit, queryPoolMetadataURLMaxBytes+1)

	var filtered StakePoolParamsResult
	require.NoError(t, filtered.UnmarshalCBOR(stakePoolParamsCBORMixed(t)))
	params, ok = filtered.Results[testPoolId()]
	require.True(t, ok)
	require.Equal(t, "https://valid.example/", params.PoolMetadata.Url)
	badPool := testPoolId()
	badPool[0] = 0x02
	_, ok = filtered.Results[badPool]
	require.False(t, ok)
}

// TestPoolStateResultURLBound covers the same rule on the GetPoolState result,
// which reaches the inline metadata struct through PoolStateParams.
func TestPoolStateResultURLBound(t *testing.T) {
	atLimit := testPoolURL(queryPoolMetadataURLMaxBytes)

	accepted, err := decodePoolStateResult(poolStateCBOR(t, atLimit))
	require.NoError(t, err)
	params, ok := accepted.PState[testPoolOperator()]
	require.True(t, ok)
	require.NotNil(t, params)
	if params == nil {
		t.Fatal("expected pool state params")
	}
	require.NotNil(t, params.PoolMetadata)
	require.Equal(t, atLimit, params.PoolMetadata.Url)
	require.Equal(
		t,
		testPoolMetadataHashBytes(),
		[]byte(params.PoolMetadata.MetadataHash[:]),
	)

	overLimit := testPoolURL(queryPoolMetadataURLMaxBytes + 1)

	_, err = decodePoolStateResult(poolStateCBOR(t, overLimit))
	require.ErrorIs(t, err, lcommon.ErrPoolMetadataURLTooLong)
	require.Contains(t, err.Error(), "pstate pool "+strings.Repeat("01", len(testPoolOperator())))
}

// TestPoolStateParamsURLBound exercises the PoolStateParams decoder on its
// own, so the PoolStateResult test cannot pass through a shared code path
// while this one is unreachable.
func TestPoolStateParamsURLBound(t *testing.T) {
	encode := func(url string) []byte {
		encoded, err := cbor.Encode(testPoolParams(t, url))
		require.NoError(t, err)
		return encoded
	}

	atLimit := testPoolURL(queryPoolMetadataURLMaxBytes)
	var accepted PoolStateParams
	require.NoError(t, accepted.UnmarshalCBOR(encode(atLimit)))
	require.NotNil(t, accepted.PoolMetadata)
	require.Equal(t, atLimit, accepted.PoolMetadata.Url)

	var rejected PoolStateParams
	err := rejected.UnmarshalCBOR(encode(testPoolURL(queryPoolMetadataURLMaxBytes + 1)))
	require.Error(t, err)
	require.ErrorIs(t, err, lcommon.ErrPoolMetadataURLTooLong)
}

func TestQueryPoolMetadataURLEncodingBound(t *testing.T) {
	tooLong := testPoolURL(queryPoolMetadataURLMaxBytes + 1)
	decoded, err := decodePoolStateResult(
		poolStateCBOR(t, "https://pool.example/"),
	)
	require.NoError(t, err)
	params, ok := decoded.PState[testPoolOperator()]
	require.True(t, ok)
	require.NotNil(t, params)
	if params == nil {
		t.Fatal("expected pool state params")
	}
	params.PoolMetadata.Url = tooLong
	_, err = cbor.Encode(params)
	require.ErrorIs(t, err, lcommon.ErrPoolMetadataURLTooLong)

	var stakeResult StakePoolParamsResult
	require.NoError(
		t,
		stakeResult.UnmarshalCBOR(stakePoolParamsCBOR(t, "https://pool.example/")),
	)
	poolParams := stakeResult.Results[testPoolId()]
	poolParams.PoolMetadata.Url = tooLong
	stakeResult.Results[testPoolId()] = poolParams
	_, err = cbor.Encode(stakeResult)
	require.ErrorIs(t, err, lcommon.ErrPoolMetadataURLTooLong)
}

// TestPoolMetadataURLBoundMeasuredInBytes checks that the limit counts UTF-8
// bytes rather than runes, matching cardano-ledger's lengthWord8 measure.
func TestPoolMetadataURLBoundMeasuredInBytes(t *testing.T) {
	atLimit := strings.Repeat("é", queryPoolMetadataURLMaxBytes/2)
	require.Len(t, atLimit, queryPoolMetadataURLMaxBytes)
	overLimit := strings.Repeat("é", queryPoolMetadataURLMaxBytes/2+1)

	var acceptedParams StakePoolParamsResult
	require.NoError(
		t,
		acceptedParams.UnmarshalCBOR(stakePoolParamsCBOR(t, atLimit)),
	)
	var rejectedParams StakePoolParamsResult
	require.NoError(
		t,
		rejectedParams.UnmarshalCBOR(stakePoolParamsCBOR(t, overLimit)),
	)
	_, ok := rejectedParams.Results[testPoolId()]
	require.False(t, ok)

	_, err := decodePoolStateResult(poolStateCBOR(t, atLimit))
	require.NoError(t, err)
	_, err = decodePoolStateResult(poolStateCBOR(t, overLimit))
	require.ErrorIs(t, err, lcommon.ErrPoolMetadataURLTooLong)
}

// TestPoolParamsResultsDecodeUnchanged checks that results within the bound,
// including results carrying no metadata at all, decode field for field.
func TestPoolParamsResultsDecodeUnchanged(t *testing.T) {
	url := "https://pool.example.com/metadata.json"

	var stakePoolParams StakePoolParamsResult
	require.NoError(
		t,
		stakePoolParams.UnmarshalCBOR(stakePoolParamsCBOR(t, url)),
	)
	params, ok := stakePoolParams.Results[testPoolId()]
	require.True(t, ok)
	require.Equal(t, testPoolOperator(), params.Operator)
	require.Equal(t, testPoolMetadataHash(), params.VrfKeyHash)
	require.Equal(t, uint(1_000_000), params.Pledge)
	require.Equal(t, uint(340_000_000), params.FixedCost)
	require.NotNil(t, params.Margin)
	require.Equal(t, big.NewRat(1, 10), params.Margin.Rat)
	require.Equal(t, []ledger.Blake2b224{testPoolOperator()}, params.PoolOwners)
	require.NotNil(t, params.PoolMetadata)
	require.Equal(t, url, params.PoolMetadata.Url)

	poolState, err := decodePoolStateResult(poolStateCBOR(t, url))
	require.NoError(t, err)
	statePool, ok := poolState.PState[testPoolOperator()]
	require.True(t, ok)
	require.NotNil(t, statePool)
	require.Equal(t, uint64(1_000_000), statePool.Pledge)
	require.Equal(t, uint64(340_000_000), statePool.Cost)
	require.NotNil(t, statePool.PoolMetadata)
	require.Equal(t, url, statePool.PoolMetadata.Url)

	var noMetadata StakePoolParamsResult
	require.NoError(t, noMetadata.UnmarshalCBOR(stakePoolParamsCBOR(t, "")))
	emptyParams, ok := noMetadata.Results[testPoolId()]
	require.True(t, ok)
	require.Nil(t, emptyParams.PoolMetadata)

	noStateMetadata, err := decodePoolStateResult(poolStateCBOR(t, ""))
	require.NoError(t, err)
	emptyState, ok := noStateMetadata.PState[testPoolOperator()]
	require.True(t, ok)
	require.NotNil(t, emptyState)
	require.Nil(t, emptyState.PoolMetadata)
}
