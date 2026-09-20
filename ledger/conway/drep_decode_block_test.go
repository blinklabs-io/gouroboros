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

package conway_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

// conwayBlockWithVoteDelegation splices a delegation_to_drep_cert carrying
// drepCbor into a real preview transaction and wraps the result in a block.
// Splicing a known good transaction keeps every other decode check satisfied,
// so a block that still decodes can only have accepted the DRep.
func conwayBlockWithVoteDelegation(
	t *testing.T,
	drepCbor []byte,
	isValid bool,
) []byte {
	t.Helper()

	txCbor := previewPoolRegistrationTxCbor(t)
	var txComponents []cbor.RawMessage
	_, err := cbor.Decode(txCbor, &txComponents)
	require.NoError(t, err)
	require.Len(t, txComponents, 4)

	var body map[uint64]cbor.RawMessage
	_, err = cbor.Decode(txComponents[0], &body)
	require.NoError(t, err)

	stakeCredential, err := cbor.Encode(
		[]any{0, bytes.Repeat([]byte{0x11}, common.Blake2b224Size)},
	)
	require.NoError(t, err)
	certificate, err := cbor.Encode([]any{
		9, // delegation_to_drep_cert
		cbor.RawMessage(stakeCredential),
		cbor.RawMessage(drepCbor),
	})
	require.NoError(t, err)
	certificates, err := cbor.Encode(
		[]any{cbor.RawMessage(certificate)},
	)
	require.NoError(t, err)
	body[4] = certificates

	bodyCbor, err := cbor.Encode(body)
	require.NoError(t, err)
	splicedTx, err := cbor.Encode([]any{
		cbor.RawMessage(bodyCbor),
		cbor.RawMessage(txComponents[1]),
		isValid,
		cbor.RawMessage(txComponents[3]),
	})
	require.NoError(t, err)
	return syntheticConwayBlockWithTransaction(t, splicedTx)
}

// A block carrying a vote delegation to a DRep whose credential is not 28
// bytes must be refused at decode. cardano-ledger decodes the credential
// through a fixed-size codec, so a node that accepts the block diverges from
// consensus; four DelegateVoteToUnregisteredDRep sites in this package would
// otherwise report the credential zero-padded to 28 bytes.
//
// The isValid=false rows matter because a phase-2-invalid transaction skips
// several validation rules. Decode is not one of them.
func TestConwayBlockRejectsWrongLengthDrepCredential(t *testing.T) {
	t.Parallel()

	shortCredential, err := cbor.Encode(
		[]any{common.DrepTypeAddrKeyHash, []byte{0x01, 0x02}},
	)
	require.NoError(t, err)
	longCredential, err := cbor.Encode(
		[]any{
			common.DrepTypeScriptHash,
			bytes.Repeat([]byte{0xcd}, common.Blake2b224Size+1),
		},
	)
	require.NoError(t, err)

	testCases := []struct {
		name     string
		drepCbor []byte
		isValid  bool
	}{
		{"2-byte key hash, isValid true", shortCredential, true},
		{"2-byte key hash, isValid false", shortCredential, false},
		{"29-byte script hash, isValid true", longCredential, true},
		{"29-byte script hash, isValid false", longCredential, false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			blockCbor := conwayBlockWithVoteDelegation(
				t,
				testCase.drepCbor,
				testCase.isValid,
			)
			_, err := conway.NewConwayBlockFromCbor(
				blockCbor,
				common.VerifyConfig{SkipBodyHashValidation: true},
			)
			require.Error(
				t,
				err,
				"block decoded with a DRep credential that is not 28 bytes",
			)
		})
	}
}

// The rejection above must not come from rejecting every spliced block.
func TestConwayBlockAcceptsHash28DrepCredential(t *testing.T) {
	t.Parallel()

	credential := bytes.Repeat([]byte{0xab}, common.Blake2b224Size)
	drepCbor, err := cbor.Encode(
		[]any{common.DrepTypeAddrKeyHash, credential},
	)
	require.NoError(t, err)

	block, err := conway.NewConwayBlockFromCbor(
		conwayBlockWithVoteDelegation(t, drepCbor, true),
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	txs := block.Transactions()
	require.Len(t, txs, 1)
	certificates := txs[0].Certificates()
	require.Len(t, certificates, 1)
	cert, ok := certificates[0].(*common.VoteDelegationCertificate)
	require.True(t, ok, "certificate type %T", certificates[0])
	require.Equal(t, common.DrepTypeAddrKeyHash, cert.Drep.Type)
	require.Equal(t, credential, cert.Drep.Credential)
}
