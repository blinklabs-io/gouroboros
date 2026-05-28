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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func leiosTestSignature(fill byte) []byte {
	ret := make([]byte, LeiosBlsSignatureSize)
	for i := range ret {
		ret[i] = fill
	}
	return ret
}

func TestLeiosVoteCborRoundTrip(t *testing.T) {
	vote := LeiosVote{
		SlotNo:            12345,
		EndorserBlockHash: NewBlake2b256([]byte{0x01, 0x02, 0x03}),
		VoterId:           42,
		VoteSignature:     leiosTestSignature(0xaa),
	}

	encoded, err := cbor.Encode(vote)
	require.NoError(t, err)

	var decoded LeiosVote
	_, err = cbor.Decode(encoded, &decoded)
	require.NoError(t, err)
	assert.Equal(t, vote.SlotNo, decoded.SlotNo)
	assert.Equal(t, vote.EndorserBlockHash, decoded.EndorserBlockHash)
	assert.Equal(t, vote.VoterId, decoded.VoterId)
	assert.Equal(t, vote.VoteSignature, decoded.VoteSignature)
	assert.Equal(t, encoded, decoded.Cbor())

	reencoded, err := cbor.Encode(decoded)
	require.NoError(t, err)
	assert.Equal(t, encoded, reencoded)
}

func TestLeiosVoteCddlVector(t *testing.T) {
	hash := NewBlake2b256([]byte{0x01, 0x02, 0x03})
	signature := leiosTestSignature(0xaa)
	raw, err := cbor.Encode([]any{
		uint64(12345),
		hash,
		uint64(42),
		signature,
	})
	require.NoError(t, err)
	require.Equal(t, byte(0x84), raw[0])

	var decoded LeiosVote
	_, err = cbor.Decode(raw, &decoded)
	require.NoError(t, err)
	assert.Equal(t, uint64(12345), decoded.SlotNo)
	assert.Equal(t, hash, decoded.EndorserBlockHash)
	assert.Equal(t, uint64(42), decoded.VoterId)
	assert.Equal(t, signature, decoded.VoteSignature)
	assert.Equal(t, raw, decoded.Cbor())

	reencoded, err := cbor.Encode(decoded)
	require.NoError(t, err)
	assert.Equal(t, raw, reencoded)
}

func TestLeiosVoteRejectsInvalidSignatureLength(t *testing.T) {
	vote := LeiosVote{
		SlotNo:            12345,
		EndorserBlockHash: NewBlake2b256([]byte{0x01}),
		VoterId:           42,
		VoteSignature:     []byte{0xaa},
	}
	encoded, err := cbor.Encode([]any{
		vote.SlotNo,
		vote.EndorserBlockHash,
		vote.VoterId,
		vote.VoteSignature,
	})
	require.NoError(t, err)

	var decoded LeiosVote
	_, err = cbor.Decode(encoded, &decoded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VoteSignature")
}

func TestLeiosEbCertificateCborRoundTrip(t *testing.T) {
	cert := LeiosEbCertificate{
		SlotNo:              99,
		EndorserBlockHash:   NewBlake2b256([]byte{0x11, 0x22, 0x33}),
		Signers:             []byte{0x80, 0x40},
		AggregatedSignature: leiosTestSignature(0xbb),
	}

	encoded, err := cbor.Encode(cert)
	require.NoError(t, err)

	var decoded LeiosEbCertificate
	_, err = cbor.Decode(encoded, &decoded)
	require.NoError(t, err)
	require.NoError(t, decoded.Validate(10))
	assert.Equal(t, cert.SlotNo, decoded.SlotNo)
	assert.Equal(t, cert.EndorserBlockHash, decoded.EndorserBlockHash)
	assert.Equal(t, cert.Signers, decoded.Signers)
	assert.Equal(t, cert.AggregatedSignature, decoded.AggregatedSignature)
	assert.True(t, decoded.Signer(0))
	assert.True(t, decoded.Signer(9))
	assert.False(t, decoded.Signer(8))

	reencoded, err := cbor.Encode(&decoded)
	require.NoError(t, err)
	assert.Equal(t, encoded, reencoded)
}

func TestLeiosEbCertificateSetAggregatedSignatureClearsCachedCbor(t *testing.T) {
	cert := LeiosEbCertificate{
		SlotNo:              99,
		EndorserBlockHash:   NewBlake2b256([]byte{0x11, 0x22, 0x33}),
		Signers:             []byte{0x80},
		AggregatedSignature: leiosTestSignature(0xbb),
	}
	encoded, err := cbor.Encode(cert)
	require.NoError(t, err)

	var decoded LeiosEbCertificate
	_, err = cbor.Decode(encoded, &decoded)
	require.NoError(t, err)
	require.NotEmpty(t, decoded.Cbor())

	newSig := leiosTestSignature(0xcc)
	require.NoError(t, decoded.SetAggregatedSignature(newSig))
	require.Empty(t, decoded.Cbor())

	reencoded, err := cbor.Encode(&decoded)
	require.NoError(t, err)
	assert.NotEqual(t, encoded, reencoded)

	var redecoded LeiosEbCertificate
	_, err = cbor.Decode(reencoded, &redecoded)
	require.NoError(t, err)
	assert.Equal(t, newSig, redecoded.AggregatedSignature)
}

func TestLeiosEbCertificatePayloadVector(t *testing.T) {
	hash := NewBlake2b256([]byte{0x11, 0x22, 0x33})
	signers := []byte{0x80}
	signature := leiosTestSignature(0xbb)
	raw, err := cbor.Encode([]any{
		uint64(99),
		hash,
		signers,
		signature,
	})
	require.NoError(t, err)
	require.Equal(t, byte(0x84), raw[0])

	var decoded LeiosEbCertificate
	_, err = cbor.Decode(raw, &decoded)
	require.NoError(t, err)
	require.NoError(t, decoded.Validate(1))
	assert.Equal(t, uint64(99), decoded.SlotNo)
	assert.Equal(t, hash, decoded.EndorserBlockHash)
	assert.Equal(t, signers, decoded.Signers)
	assert.Equal(t, signature, decoded.AggregatedSignature)
	assert.True(t, decoded.Signer(0))
	assert.False(t, decoded.Signer(1))
	assert.Equal(t, raw, decoded.Cbor())

	reencoded, err := cbor.Encode(decoded)
	require.NoError(t, err)
	assert.Equal(t, raw, reencoded)
}

func TestLeiosEbCertificateEmptySignerBitfield(t *testing.T) {
	cert := LeiosEbCertificate{
		SlotNo:              99,
		EndorserBlockHash:   NewBlake2b256([]byte{0x11}),
		Signers:             []byte{},
		AggregatedSignature: leiosTestSignature(0xbb),
	}

	encoded, err := cbor.Encode(cert)
	require.NoError(t, err)

	var decoded LeiosEbCertificate
	_, err = cbor.Decode(encoded, &decoded)
	require.NoError(t, err)
	require.NoError(t, decoded.Validate(0))
}

func TestLeiosEbCertificateMultiByteSignerBitfield(t *testing.T) {
	cert := LeiosEbCertificate{
		SlotNo:              99,
		EndorserBlockHash:   NewBlake2b256([]byte{0x11}),
		Signers:             []byte{0x80, 0x20, 0x01},
		AggregatedSignature: leiosTestSignature(0xbb),
	}

	require.NoError(t, cert.Validate(24))
	assert.True(t, cert.Signer(0))
	assert.True(t, cert.Signer(10))
	assert.True(t, cert.Signer(23))
	assert.False(t, cert.Signer(24))
}

func TestLeiosEbCertificateRejectsInvalidSignatureLength(t *testing.T) {
	encoded, err := cbor.Encode([]any{
		uint64(99),
		NewBlake2b256([]byte{0x11}),
		[]byte{0x80},
		[]byte{0xbb},
	})
	require.NoError(t, err)

	var decoded LeiosEbCertificate
	_, err = cbor.Decode(encoded, &decoded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AggregatedSignature")
}

func TestValidateLeiosSignerBitfield(t *testing.T) {
	tests := []struct {
		name          string
		signers       []byte
		committeeSize uint64
		wantErr       string
	}{
		{
			name:          "empty for zero committee",
			signers:       []byte{},
			committeeSize: 0,
		},
		{
			name:          "one byte for one committee member",
			signers:       []byte{0x80},
			committeeSize: 1,
		},
		{
			name:          "two bytes for nine committee members",
			signers:       []byte{0x01, 0x80},
			committeeSize: 9,
		},
		{
			name:          "wrong length",
			signers:       []byte{},
			committeeSize: 1,
			wantErr:       "must be 1 bytes",
		},
		{
			name:          "unused bits set",
			signers:       []byte{0x81},
			committeeSize: 1,
			wantErr:       "unused bits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLeiosSignerBitfield(tt.signers, tt.committeeSize)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
