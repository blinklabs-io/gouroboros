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

package byron

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// The expected MainToSign bytes come from cardano-sl's binaryTest @MainToSign
// golden in chain/test/golden/bi/block/MainToSign at immutable commit
// 1499214d93767b703b9599369a431e67d83f10a2.
func TestSimpleSignatureAgainstCardanoSLMainToSignGolden(t *testing.T) {
	fixture, err := os.ReadFile("testdata/cardano-sl-main-to-sign.hex")
	require.NoError(t, err)
	referenceToSign, err := hex.DecodeString(
		strings.TrimSpace(string(fixture)),
	)
	require.NoError(t, err)

	var fields []any
	_, err = cbor.Decode(referenceToSign, &fields)
	require.NoError(t, err)
	require.Len(t, fields, 5)
	encoded, err := cbor.Encode(fields)
	require.NoError(t, err)
	require.Equal(
		t,
		referenceToSign,
		encoded,
		"upstream golden must be canonical",
	)

	config := testByronConfig()
	validator := NewHeaderValidator(config)
	privateKey := ed25519.NewKeyFromSeed(
		bytes.Repeat([]byte{0x42}, ed25519.SeedSize),
	)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	issuerVerificationKey := append(
		append([]byte(nil), publicKey...),
		bytes.Repeat([]byte{0x43}, 32)...,
	)
	domainSeparated, err := validator.domainSeparateMainBlock(
		referenceToSign,
	)
	require.NoError(t, err)
	signature := ed25519.Sign(privateKey, domainSeparated)

	// Assemble a block header around the upstream MainToSign fields. The
	// expected bytes remain the independently sourced Haskell golden above.
	headerCbor, err := cbor.Encode([]any{
		config.ProtocolMagic,
		fields[0],
		fields[1],
		[]any{
			fields[2],
			issuerVerificationKey,
			fields[3],
			[]any{uint64(0), signature},
		},
		fields[4],
	})
	require.NoError(t, err)
	input := &ValidateHeaderInput{
		IssuerPubKey:   publicKey,
		HeaderCbor:     headerCbor,
		BlockSignature: signature,
	}

	got, err := validator.buildToSign(input)
	require.NoError(t, err)
	require.Equal(t, referenceToSign, got)
	require.NoError(t, validator.validateBlockSignature(input))
}
