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

package leiosnotify

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

// Messages the rc-2026w41 Leios prototype node sent to a LeiosNotify client on
// a devnet, as captured from the wire. Each must decode and re-encode to the
// same bytes.
const (
	rcBlockOffer    = "83058218bd5820b7ded070e944f25c20198c06f6304ef1f305fe070d569edf5e9ee69d7b4347191825"
	rcBlockTxsOffer = "82068218a05820c440e4ffbe4c0a9b2581e76bef3caf0645c02baaa2bdbf43ed3068d6ee8f05b2"
	rcVotes         = "8207818358206b5d20fcce0adfee9a9ff3431950463e349c640321e1d875568f8bf1b1a0ff2f0058308ff14b1a198df6d15c21b4506393ebe6a84487ad03c13f51bfb2141ab90d75d6fad964491395619c629fd64929c0882c"
)

func rcVector(t *testing.T, hexData string, msgType uint) ([]byte, any) {
	t.Helper()
	raw, err := hex.DecodeString(strings.TrimSpace(hexData))
	require.NoError(t, err)
	msg, err := NewMsgFromCbor(msgType, raw)
	require.NoError(t, err)
	return raw, msg
}

func TestRcPrototypeBlockAnnouncement(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/rc-2026w41-block-announcement.hex")
	require.NoError(t, err)
	raw, decoded := rcVector(t, string(data), MessageTypeBlockAnnouncement)
	msg := decoded.(*MsgBlockAnnouncement)
	require.Equal(t, uint(dijkstra.BlockHeaderTypeDijkstra), msg.Era)
	header, err := dijkstra.NewDijkstraBlockHeaderFromCbor(msg.BlockHeaderRaw)
	require.NoError(t, err)
	_, _, announces := header.LeiosAnnouncement()
	require.True(t, announces)

	reencoded := NewMsgBlockAnnouncement(msg.Era, msg.BlockHeaderRaw)
	encoded, err := reencoded.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, raw, encoded)
}

func TestRcPrototypeBlockOffers(t *testing.T) {
	t.Parallel()
	raw, decoded := rcVector(t, rcBlockOffer, MessageTypeBlockOffer)
	offer := decoded.(*MsgBlockOffer)
	require.Equal(t, uint64(0xbd), offer.Point.Slot)
	require.Equal(t, uint64(0x25), offer.Size)
	encoded, err := cbor.Encode(NewMsgBlockOffer(offer.Point, offer.Size))
	require.NoError(t, err)
	require.Equal(t, raw, encoded)

	raw, decoded = rcVector(t, rcBlockTxsOffer, MessageTypeBlockTxsOffer)
	txsOffer := decoded.(*MsgBlockTxsOffer)
	require.Equal(t, uint64(0xa0), txsOffer.Point.Slot)
	encoded, err = cbor.Encode(NewMsgBlockTxsOffer(txsOffer.Point))
	require.NoError(t, err)
	require.Equal(t, raw, encoded)
}

func TestRcPrototypeVotes(t *testing.T) {
	t.Parallel()
	raw, decoded := rcVector(t, rcVotes, MessageTypeVotesOffer)
	votes := decoded.(*MsgVotesOffer)
	require.Len(t, votes.PrototypeVotes, 1)
	vote := votes.PrototypeVotes[0]
	require.Equal(
		t,
		"6b5d20fcce0adfee9a9ff3431950463e349c640321e1d875568f8bf1b1a0ff2f",
		vote.AnnouncingRbHash.String(),
	)
	require.Zero(t, vote.VoterId)
	require.Len(t, vote.VoteSignature, 48)

	encoded, err := NewMsgVotesOfferPrototype(votes.PrototypeVotes).MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, raw, encoded)
}
