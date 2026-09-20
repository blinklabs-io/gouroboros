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
	"github.com/stretchr/testify/require"
)

func encodeEbCertificate(t *testing.T, signersLen int) []byte {
	t.Helper()
	raw, err := cbor.Encode([]any{
		uint64(1),
		make([]byte, Blake2b256Size),
		make([]byte, signersLen),
		make([]byte, LeiosBlsSignatureSize),
	})
	require.NoError(t, err)
	return raw
}

func TestLeiosEbCertificateRejectsOversizedSigners(t *testing.T) {
	var cert LeiosEbCertificate
	err := cert.UnmarshalCBOR(
		encodeEbCertificate(t, MaxLeiosSignerBitfieldSize+1),
	)
	require.Error(t, err)
	var target *LeiosSignerBitfieldTooLargeError
	require.ErrorAs(t, err, &target)
	require.Equal(t, MaxLeiosSignerBitfieldSize+1, target.Size)
	require.Equal(t, MaxLeiosSignerBitfieldSize, target.Max)
}

func TestLeiosEbCertificateAcceptsMaximumSigners(t *testing.T) {
	var cert LeiosEbCertificate
	require.NoError(
		t,
		cert.UnmarshalCBOR(encodeEbCertificate(t, MaxLeiosSignerBitfieldSize)),
	)
	require.Len(t, cert.Signers, MaxLeiosSignerBitfieldSize)
}

func TestMaxLeiosSignerBitfieldSizeMatchesMaxCommittee(t *testing.T) {
	require.Equal(
		t,
		uint64(MaxLeiosSignerBitfieldSize),
		LeiosSignerBitfieldSize(MaxLeiosCommitteeSize),
	)
}
