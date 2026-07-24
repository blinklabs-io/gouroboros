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
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// This is the MsgQuery shape emitted by cardano-cli for
// query stake-address-info:
//
//	MsgQuery(Block(Shelley(era=6, GetStakeDelegDeposits({key credential})))))
//
// Before query 22 was registered in shelleyQueryTypes, decoding this message
// failed and the node-to-client connection closed without returning a result.
func TestDecodeStakeDelegDepositsQuery(t *testing.T) {
	const payloadHex = "82038200820082068216d90102818200581c" +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload, err := hex.DecodeString(payloadHex)
	require.NoError(t, err)

	msg, err := NewMsgFromCbor(MessageTypeQuery, payload)
	require.NoError(t, err)
	msgQuery, ok := msg.(*MsgQuery)
	require.True(t, ok)
	blockQuery, ok := msgQuery.Query.Query.(*BlockQuery)
	require.True(t, ok)
	shelleyQuery, ok := blockQuery.Query.(*ShelleyQuery)
	require.True(t, ok)
	require.Equal(t, uint(6), shelleyQuery.Era)
	query, ok := shelleyQuery.Query.(*ShelleyStakeDelegDepositsQuery)
	require.True(t, ok)
	require.Equal(t, QueryTypeShelleyStakeDelegDeposits, query.Type)
	require.Len(t, query.Creds.Items(), 1)
	require.Equal(t, uint64(0), query.Creds.Items()[0].Tag)
	require.Equal(
		t,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		hex.EncodeToString(query.Creds.Items()[0].Bytes[:]),
	)
}

func TestDecodeGetProposalsQueryWithFilter(t *testing.T) {
	const payloadHex = "820382008200820682181fd9010280"
	payload, err := hex.DecodeString(payloadHex)
	require.NoError(t, err)

	msg, err := NewMsgFromCbor(MessageTypeQuery, payload)
	require.NoError(t, err)
	msgQuery := msg.(*MsgQuery)
	blockQuery := msgQuery.Query.Query.(*BlockQuery)
	shelleyQuery := blockQuery.Query.(*ShelleyQuery)
	query, ok := shelleyQuery.Query.(*ShelleyGetProposalsQuery)
	require.True(t, ok)
	require.Equal(t, QueryTypeShelleyGetProposals, query.Type)
	require.Empty(t, query.ActionIds.Items())
}

func TestDecodeFilteredDelegationsAndRewardAccountsResult(t *testing.T) {
	payload, err := hex.DecodeString("8182a0a0")
	require.NoError(t, err)

	var result FilteredDelegationsAndRewardAccountsResult
	_, err = cbor.Decode(payload, &result)
	require.NoError(t, err)
	require.Empty(t, result.Delegations)
	require.Empty(t, result.Rewards)
}

func TestDecodeStakeAddressInfoEraResults(t *testing.T) {
	var deposits StakeDelegDepositsResult
	_, err := cbor.Decode([]byte{0x81, 0xa0}, &deposits)
	require.NoError(t, err)
	require.Empty(t, deposits)

	var delegatees FilteredVoteDelegateesResult
	_, err = cbor.Decode([]byte{0x81, 0xa0}, &delegatees)
	require.NoError(t, err)
	require.Empty(t, delegatees)

	var proposals ProposalsResult
	_, err = cbor.Decode([]byte{0x81, 0x80}, &proposals)
	require.NoError(t, err)
	require.Empty(t, proposals)
}
