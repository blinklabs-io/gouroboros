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
	"encoding/hex"
	"math/big"
	"net"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ntc(version uint16) uint16 {
	return version + protocol.ProtocolVersionNtCOffset
}

func spacedHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	require.NoError(t, err)
	return b
}

var (
	testPoolIdHex = "1cb2c9bb54fb097fba58542c5adcf3a7d094db95e8b9c4e3d0bcbf1d"
	testVrfHex    = "c0d1f9b040d2f6fd7fc8775d24753d6db4b0fb6846b76f0d" +
		"8f4b8e8b96ed7d9a"
	testCredAHex = "00000000000000000000000000000000000000000000000000000001"
	testCredBHex = "00000000000000000000000000000000000000000000000000000002"
)

// TestRcQueryEncodings pins the request bytes of the Shelley queries the
// node answers from version 20 on, as encodeShelleyQuery writes them.
func TestRcQueryEncodings(t *testing.T) {
	poolId := ledger.PoolId(ledger.NewBlake2b224(spacedHex(t, testPoolIdHex)))
	cases := []struct {
		name  string
		query any
		want  string
		decTo any
	}{
		{
			name: "stake_pool_default_vote",
			query: &ShelleyStakePoolDefaultVoteQuery{
				Type:   QueryTypeShelleyStakePoolDefaultVote,
				PoolId: poolId,
			},
			want:  "82 18 23 58 1c " + testPoolIdHex,
			decTo: &ShelleyStakePoolDefaultVoteQuery{},
		},
		{
			name: "stake_distribution2",
			query: &ShelleyStakeDistribution2Query{
				simpleQueryBase{Type: QueryTypeShelleyStakeDistribution2},
			},
			want:  "81 18 25",
			decTo: &ShelleyStakeDistribution2Query{},
		},
		{
			name: "max_major_protocol_version",
			query: &ShelleyMaxMajorProtocolVersionQuery{
				simpleQueryBase{Type: QueryTypeShelleyMaxMajorProtocolVersion},
			},
			want:  "81 18 26",
			decTo: &ShelleyMaxMajorProtocolVersionQuery{},
		},
		{
			name: "drep_delegations",
			query: &ShelleyDRepDelegationsQuery{
				Type: QueryTypeShelleyDRepDelegations,
				DReps: cbor.NewSetType(
					[]lcommon.Drep{{
						Type:       lcommon.DrepTypeAddrKeyHash,
						Credential: spacedHex(t, testCredAHex),
					}},
					true,
				),
			},
			want:  "82 18 27 d9 0102 81 82 00 58 1c " + testCredAHex,
			decTo: &ShelleyDRepDelegationsQuery{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := cbor.Encode(tc.query)
			require.NoError(t, err)
			assert.Equal(
				t,
				strings.ReplaceAll(tc.want, " ", ""),
				hex.EncodeToString(data),
			)
			decoded, err := decodeShelleyQuery(data)
			require.NoError(t, err)
			assert.IsType(t, tc.decTo, decoded)
		})
	}
}

func TestDRepDelegationsQueryDecodesFilter(t *testing.T) {
	data := spacedHex(
		t,
		"82 18 27 d9 0102 82 82 00 58 1c "+testCredAHex+" 81 02",
	)
	decoded, err := decodeShelleyQuery(data)
	require.NoError(t, err)
	q, ok := decoded.(*ShelleyDRepDelegationsQuery)
	require.True(t, ok)
	dreps := q.DReps.Items()
	require.Len(t, dreps, 2)
	assert.Equal(t, lcommon.DrepTypeAddrKeyHash, dreps[0].Type)
	assert.Equal(t, lcommon.DrepTypeAbstain, dreps[1].Type)
}

func TestStakePoolDefaultVoteResult(t *testing.T) {
	for _, tc := range []struct {
		hex  string
		want DefaultVote
	}{
		{"8100", DefaultVoteNo},
		{"8101", DefaultVoteAbstain},
		{"8102", DefaultVoteNoConfidence},
	} {
		var result StakePoolDefaultVoteResult
		_, err := cbor.Decode(spacedHex(t, tc.hex), &result)
		require.NoError(t, err, tc.hex)
		assert.Equal(t, tc.want, result.Vote)
		data, err := cbor.Encode(&result)
		require.NoError(t, err)
		assert.Equal(t, tc.hex, hex.EncodeToString(data))
	}
	var result StakePoolDefaultVoteResult
	_, err := cbor.Decode(spacedHex(t, "8103"), &result)
	require.Error(t, err, "the ledger has no fourth default vote")
}

func TestMaxMajorProtocolVersionResult(t *testing.T) {
	var result MaxMajorProtocolVersionResult
	_, err := cbor.Decode(spacedHex(t, "810c"), &result)
	require.NoError(t, err)
	assert.Equal(t, uint64(12), result.Version)
}

// TestStakeDistribution2Result covers the ledger PoolDistr record, including
// a pool entry that carries the optional fourth BLS-key field and a stake
// fraction written without tag 30.
func TestStakeDistribution2Result(t *testing.T) {
	const blsKey = "82 41 aa 41 bb"
	wire := spacedHex(t, "81 82 a2"+
		" 58 1c "+testPoolIdHex+
		" 83 d8 1e 82 01 03 1a 05 f5 e1 00 58 20 "+testVrfHex+
		" 58 1c "+strings.Repeat("ab", 28)+
		" 84 82 01 04 1a 0b eb c2 00 58 20 "+testVrfHex+" 81 "+blsKey+
		" 1b 00 00 00 02 54 0b e4 00")
	var result StakeDistribution2Result
	_, err := cbor.Decode(wire, &result)
	require.NoError(t, err)
	distr := result.Distribution
	assert.Equal(t, uint64(10000000000), distr.TotalActiveStake)
	require.Len(t, distr.Pools, 2)
	first := distr.Pools[ledger.PoolId(
		ledger.NewBlake2b224(spacedHex(t, testPoolIdHex)),
	)]
	assert.Zero(t, first.StakeFraction.Cmp(big.NewRat(1, 3)))
	assert.Equal(t, uint64(100000000), first.TotalPoolStake)
	assert.Equal(t, testVrfHex, hex.EncodeToString(first.VrfHash[:]))
	assert.Nil(t, first.BlsKey)
	second := distr.Pools[ledger.PoolId(
		ledger.NewBlake2b224(spacedHex(t, strings.Repeat("ab", 28))),
	)]
	assert.Zero(t, second.StakeFraction.Cmp(big.NewRat(1, 4)))
	assert.Equal(
		t,
		strings.ReplaceAll(blsKey, " ", ""),
		hex.EncodeToString(second.BlsKey),
	)

	// The entry without a key keeps three fields and the one with a key four.
	data, err := cbor.Encode(&second)
	require.NoError(t, err)
	assert.Equal(t, byte(0x84), data[0])
	data, err = cbor.Encode(&first)
	require.NoError(t, err)
	assert.Equal(t, byte(0x83), data[0])

	var bad IndividualPoolStake
	_, err = cbor.Decode(spacedHex(t, "82 01 02"), &bad)
	require.Error(t, err)
}

// TestDRepDelegationsResult decodes Map DRep (Set (Credential Staking)) and
// re-encodes it in the order a node writes it.
func TestDRepDelegationsResult(t *testing.T) {
	wire := spacedHex(t, "81 a2"+
		" 82 00 58 1c "+testCredAHex+
		" d9 0102 82 82 00 58 1c "+testCredAHex+" 82 01 58 1c "+testCredBHex+
		" 81 02 d9 0102 80")
	var result DRepDelegationsResult
	_, err := cbor.Decode(wire, &result)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, lcommon.DrepTypeAddrKeyHash, result[0].Drep.Type)
	require.Len(t, result[0].Delegators, 2)
	assert.Equal(t, uint(0), result[0].Delegators[0].CredType)
	assert.Equal(t, uint(1), result[0].Delegators[1].CredType)
	assert.Equal(t, lcommon.DrepTypeAbstain, result[1].Drep.Type)
	assert.Empty(t, result[1].Delegators)
	data, err := cbor.Encode(result)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(wire, data), "re-encoded %x", data)

	dup := spacedHex(t, "81 a2 81 02 d9 0102 80 81 02 d9 0102 80")
	_, err = cbor.Decode(dup, &result)
	require.ErrorContains(t, err, "duplicate DRep")
}

// TestShelleyQueryVersionGates pins the node-to-client versions that carry
// each query whose availability changed after version 19.
func TestShelleyQueryVersionGates(t *testing.T) {
	cases := []struct {
		queryType int
		supported []uint16
		refused   []uint16
	}{
		{
			QueryTypeShelleyProposedProtocolParamsUpdates,
			[]uint16{19},
			[]uint16{20, 23},
		},
		{QueryTypeShelleyStakeDistribution, []uint16{20}, []uint16{21, 23}},
		{QueryTypeShelleyPoolDistr, []uint16{20}, []uint16{21, 23}},
		{QueryTypeShelleyStakePoolDefaultVote, []uint16{20, 23}, []uint16{19}},
		{QueryTypeShelleyStakeDistribution2, []uint16{21, 23}, []uint16{20}},
		{
			QueryTypeShelleyMaxMajorProtocolVersion,
			[]uint16{21, 23},
			[]uint16{20},
		},
		{QueryTypeShelleyDRepDelegations, []uint16{23}, []uint16{21, 22}},
		{QueryTypeShelleyEpochNo, []uint16{9, 23}, nil},
	}
	for _, tc := range cases {
		for _, v := range tc.supported {
			assert.True(t, shelleyQuerySupported(tc.queryType, ntc(v)),
				"query %d at version %d", tc.queryType, v)
		}
		for _, v := range tc.refused {
			assert.False(t, shelleyQuerySupported(tc.queryType, ntc(v)),
				"query %d at version %d", tc.queryType, v)
		}
	}
}

// TestClientRefusesQueriesOutsideVersion checks each gated client call fails
// before any wire activity on a version that does not carry its query.
func TestClientRefusesQueriesOutsideVersion(t *testing.T) {
	calls := []struct {
		name    string
		version uint16
		call    func(c *Client) error
	}{
		{"proposed_pparams_updates_v20", 20, func(c *Client) error {
			_, err := c.GetProposedProtocolParamsUpdates()
			return err
		}},
		{"stake_distribution_v21", 21, func(c *Client) error {
			_, err := c.GetStakeDistribution()
			return err
		}},
		{"pool_distr_v23", 23, func(c *Client) error {
			_, err := c.GetPoolDistr(nil)
			return err
		}},
		{"stake_pool_default_vote_v19", 19, func(c *Client) error {
			_, err := c.GetStakePoolDefaultVote(ledger.PoolId{})
			return err
		}},
		{"stake_distribution2_v20", 20, func(c *Client) error {
			_, err := c.GetStakeDistribution2()
			return err
		}},
		{"max_major_protocol_version_v20", 20, func(c *Client) error {
			_, err := c.GetMaxMajorProtocolVersion()
			return err
		}},
		{"drep_delegations_v22", 22, func(c *Client) error {
			_, err := c.GetDRepDelegations(nil)
			return err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			c := newClientWithVersion(ntc(tc.version))
			require.ErrorIs(t, tc.call(c), ErrQueryUnsupportedVersion)
		})
	}
}

// TestServerRefusesQueriesOutsideVersion checks the server rejects a query
// the negotiated version does not carry before the callback sees it.
func TestServerRefusesQueriesOutsideVersion(t *testing.T) {
	called := false
	cfg := NewConfig(
		WithQueryFunc(func(_ CallbackContext, _ QueryWrapper) (any, error) {
			called = true
			return nil, nil
		}),
	)
	s := NewServer(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Version: ntc(23),
	}, &cfg)
	msg := NewMsgQuery(&BlockQuery{
		Query: &ShelleyQuery{
			Era: ledger.EraIdConway,
			Query: &ShelleyStakeDistributionQuery{
				simpleQueryBase{Type: QueryTypeShelleyStakeDistribution},
			},
		},
	})
	require.ErrorIs(t, s.handleQuery(msg), ErrQueryUnsupportedVersion)
	assert.False(t, called, "callback ran for a query version 23 lacks")
}

func TestIndividualPoolStakeTagsFraction(t *testing.T) {
	s := IndividualPoolStake{
		StakeFraction: &cbor.Rat{Rat: big.NewRat(1, 3)},
	}
	data, err := cbor.Encode(&s)
	require.NoError(t, err)
	// cardano-ledger tags a Rational with 30 from protocol version 9.
	assert.Equal(t, "83d81e820103", hex.EncodeToString(data[:6]))
}
