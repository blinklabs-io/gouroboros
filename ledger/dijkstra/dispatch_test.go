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

package dijkstra

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestDijkstraGovActionAcceptsListLengthEncodings(t *testing.T) {
	canonical, err := cbor.Encode(&DijkstraGovAction{
		Action: &common.InfoGovAction{Type: uint(common.GovActionTypeInfo)},
	})
	require.NoError(t, err)
	for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
		t.Run(encoding.Name, func(t *testing.T) {
			var decoded DijkstraGovAction
			require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
			require.IsType(t, &common.InfoGovAction{}, decoded.Action)
		})
	}
}

func TestDijkstraGovActionAcceptsIndefiniteArray(t *testing.T) {
	var decoded DijkstraGovAction
	require.NoError(
		t,
		decoded.UnmarshalCBOR([]byte{0x9f, byte(common.GovActionTypeInfo), 0xff}),
	)
	require.IsType(t, &common.InfoGovAction{}, decoded.Action)
}

func TestDijkstraProposalProcedureAcceptsIndefiniteArray(t *testing.T) {
	address, err := common.NewAddress(
		"stake_test1uqehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gssrtvn",
	)
	require.NoError(t, err)
	anchor, err := common.NewGovAnchor(
		"https://example.com/proposal.json",
		make([]byte, common.Blake2b256Size),
	)
	require.NoError(t, err)
	procedure := DijkstraProposalProcedure{
		PPDeposit:       1,
		PPRewardAccount: address,
		PPGovAction: DijkstraGovAction{Action: &common.InfoGovAction{
			Type: uint(common.GovActionTypeInfo),
		}},
		PPAnchor: anchor,
	}
	encoded, err := cbor.Encode(procedure)
	require.NoError(t, err)
	length, headerLength, indefinite := cbor.ArrayInfo(encoded)
	require.GreaterOrEqual(t, length, 0)
	require.False(t, indefinite)
	encoded = append([]byte{0x9f}, encoded[headerLength:]...)
	encoded = append(encoded, 0xff)

	var decoded DijkstraProposalProcedure
	require.NoError(t, decoded.UnmarshalCBOR(encoded))
}
