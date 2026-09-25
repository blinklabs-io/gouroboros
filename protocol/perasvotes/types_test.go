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

package perasvotes

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func testVote() Vote {
	return Vote{
		VoterID:     bytes.Repeat([]byte{1}, 32),
		VotingRound: 12,
		BlockHash:   bytes.Repeat([]byte{2}, 32),
		VotingProof: VRFCert{
			Output: []byte{3, 4},
			Proof:  bytes.Repeat([]byte{5}, 80),
		},
		VotingWeight: 600,
		KESPeriod:    99,
		KESVKey:      bytes.Repeat([]byte{6}, 32),
		KESSignature: bytes.Repeat([]byte{7}, 448),
	}
}

func TestVoteCBORRoundTrip(t *testing.T) {
	t.Parallel()

	want := testVote()
	encoded, err := cbor.Encode(want)
	require.NoError(t, err)
	var got Vote
	_, err = cbor.Decode(encoded, &got)
	require.NoError(t, err)
	require.Equal(t, want.VoterID, got.VoterID)
	require.Equal(t, want.VotingRound, got.VotingRound)
	require.Equal(t, want.BlockHash, got.BlockHash)
	require.Equal(t, want.VotingProof, got.VotingProof)
	require.Equal(t, want.VotingWeight, got.VotingWeight)
	require.Equal(t, want.KESPeriod, got.KESPeriod)
	require.Equal(t, want.KESVKey, got.KESVKey)
	require.Equal(t, want.KESSignature, got.KESSignature)
	reencoded, err := cbor.Encode(got)
	require.NoError(t, err)
	require.Equal(t, encoded, reencoded)
}

func TestVoterCertUsesVoteCBORShape(t *testing.T) {
	t.Parallel()

	encoded, err := cbor.Encode(testVote())
	require.NoError(t, err)
	var got VoterCert
	_, err = cbor.Decode(encoded, &got)
	require.NoError(t, err)
	reencoded, err := cbor.Encode(got)
	require.NoError(t, err)
	require.Equal(t, encoded, reencoded)
}

func TestVoteCertPreservesOpaqueCBOR(t *testing.T) {
	t.Parallel()

	for _, encoded := range [][]byte{
		{0x42, 0xaa, 0xbb},
		{0x82, 0x01, 0x02},
	} {
		var got VoteCert
		_, err := cbor.Decode(encoded, &got)
		require.NoError(t, err)
		reencoded, err := cbor.Encode(got)
		require.NoError(t, err)
		require.Equal(t, encoded, reencoded)
	}
}

func TestVoteRejectsInvalidFixedSizeFields(t *testing.T) {
	t.Parallel()

	vote := testVote()
	vote.KESSignature = vote.KESSignature[:len(vote.KESSignature)-1]
	_, err := cbor.Encode(vote)
	require.ErrorContains(t, err, "KES signature must be 448 bytes")
}
