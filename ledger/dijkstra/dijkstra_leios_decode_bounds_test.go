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
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func testLeiosCertificate(t *testing.T, signersLen int) *DijkstraLeiosCertificate {
	t.Helper()
	return &DijkstraLeiosCertificate{
		Signers:             make([]byte, signersLen),
		AggregatedSignature: make([]byte, common.LeiosBlsSignatureSize),
	}
}

func encodeLeiosCertificate(t *testing.T, signersLen int) []byte {
	t.Helper()
	raw, err := cbor.Encode([]any{
		make([]byte, signersLen),
		make([]byte, common.LeiosBlsSignatureSize),
	})
	require.NoError(t, err)
	return raw
}

// The Dijkstra CDDL bounds the Leios signers bitfield at
// `bytes .size (0 .. 8192)`, which cardano-ledger derives from
// maxLeiosCertSignersBytes. A larger bitfield addresses seats that no
// committee can contain.
func TestDijkstraLeiosCertificateRejectsOversizedSigners(t *testing.T) {
	certCbor := encodeLeiosCertificate(
		t,
		common.MaxLeiosSignerBitfieldSize+1,
	)
	var cert DijkstraLeiosCertificate
	err := cert.UnmarshalCBOR(certCbor)
	require.Error(t, err)
	var target *common.LeiosSignerBitfieldTooLargeError
	require.ErrorAs(t, err, &target)
	require.Equal(t, common.MaxLeiosSignerBitfieldSize+1, target.Size)
	require.Equal(t, common.MaxLeiosSignerBitfieldSize, target.Max)
}

func TestDijkstraLeiosCertificateAcceptsMaximumSigners(t *testing.T) {
	certCbor := encodeLeiosCertificate(t, common.MaxLeiosSignerBitfieldSize)
	var cert DijkstraLeiosCertificate
	require.NoError(t, cert.UnmarshalCBOR(certCbor))
	require.Len(t, cert.Signers, common.MaxLeiosSignerBitfieldSize)
}

func TestDijkstraLeiosCertificateAcceptsEmptySigners(t *testing.T) {
	certCbor := encodeLeiosCertificate(t, 0)
	var cert DijkstraLeiosCertificate
	require.NoError(t, cert.UnmarshalCBOR(certCbor))
	require.Empty(t, cert.Signers)
}

// CIP-0164: "RB' contains either a certificate for the EB announced in RB, or
// a list of transactions forming a valid extension of RB."
func TestDijkstraBlockBodyRejectsCertifiedBodyWithTransactions(t *testing.T) {
	body := DijkstraBlockBody{
		Transactions: []DijkstraTransaction{
			{Body: DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
		},
		LeiosCertificate: testLeiosCertificate(t, 1),
	}
	bodyCbor, err := body.MarshalCBOR()
	require.NoError(t, err)

	var decoded DijkstraBlockBody
	err = decoded.UnmarshalCBOR(bodyCbor)
	require.Error(t, err)
	var target *LeiosCertifiedBlockTransactionsError
	require.ErrorAs(t, err, &target)
	require.Equal(t, 1, target.TransactionCount)
}

// The pre-respin four-element body form carries the same Leios semantics.
func TestDijkstraLegacyBlockBodyRejectsCertifiedBodyWithTransactions(
	t *testing.T,
) {
	legacyCbor, err := cbor.Encode([]any{
		[]uint64{},
		[]any{minimalTxParts()},
		cbor.RawMessage(encodeLeiosCertificate(t, 1)),
		nil,
	})
	require.NoError(t, err)

	var decoded DijkstraBlockBody
	err = decoded.UnmarshalCBOR(legacyCbor)
	require.Error(t, err)
	var target *LeiosCertifiedBlockTransactionsError
	require.ErrorAs(t, err, &target)
	require.Equal(t, 1, target.TransactionCount)
}

// An empty transaction list alongside a certificate is the legal certified
// block shape, so the rejection is not unconditional.
func TestDijkstraBlockBodyAcceptsCertifiedBodyWithoutTransactions(t *testing.T) {
	body := DijkstraBlockBody{
		LeiosCertificate: testLeiosCertificate(t, 2),
	}
	bodyCbor, err := body.MarshalCBOR()
	require.NoError(t, err)

	var decoded DijkstraBlockBody
	require.NoError(t, decoded.UnmarshalCBOR(bodyCbor))
	require.Empty(t, decoded.Transactions)
	require.NotNil(t, decoded.LeiosCertificate)
	require.Len(t, decoded.LeiosCertificate.Signers, 2)
}

// The exclusion is not mutual: an uncertified body carrying transactions is
// the ordinary ranking block.
func TestDijkstraBlockBodyAcceptsTransactionsWithoutCertificate(t *testing.T) {
	body := DijkstraBlockBody{
		Transactions: []DijkstraTransaction{
			{Body: DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
		},
	}
	bodyCbor, err := body.MarshalCBOR()
	require.NoError(t, err)

	var decoded DijkstraBlockBody
	require.NoError(t, decoded.UnmarshalCBOR(bodyCbor))
	require.Len(t, decoded.Transactions, 1)
	require.Nil(t, decoded.LeiosCertificate)
}

// CIP-0164 places the exclusion on the Leios certificate only, and
// cardano-ledger imposes no equivalent constraint on peras_certificate.
func TestDijkstraBlockBodyAcceptsPerasCertificateWithTransactions(t *testing.T) {
	body := DijkstraBlockBody{
		Transactions: []DijkstraTransaction{
			{Body: DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
		},
		PerasCertificate: []byte{0x01, 0x02},
	}
	bodyCbor, err := body.MarshalCBOR()
	require.NoError(t, err)

	var decoded DijkstraBlockBody
	require.NoError(t, decoded.UnmarshalCBOR(bodyCbor))
	require.Len(t, decoded.Transactions, 1)
	require.Equal(t, []byte{0x01, 0x02}, decoded.PerasCertificate)
}

// peras_certificate is `bytes` in the Dijkstra CDDL and a bare ByteArray in
// cardano-ledger, so no decode-time size bound applies to it.
func TestDijkstraBlockBodyAcceptsLargePerasCertificate(t *testing.T) {
	body := DijkstraBlockBody{
		PerasCertificate: make([]byte, 1<<20),
	}
	bodyCbor, err := body.MarshalCBOR()
	require.NoError(t, err)

	var decoded DijkstraBlockBody
	require.NoError(t, decoded.UnmarshalCBOR(bodyCbor))
	require.Len(t, decoded.PerasCertificate, 1<<20)
}

func TestDijkstraBlockRejectsCertifiedBodyWithTransactions(t *testing.T) {
	body := DijkstraBlockBody{
		Transactions: []DijkstraTransaction{
			{Body: DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
		},
		LeiosCertificate: testLeiosCertificate(t, 1),
	}
	block := testDijkstraBlockWithBody(t, body)
	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	_, err = NewDijkstraBlockFromCbor(blockCbor)
	require.Error(t, err)
	var target *LeiosCertifiedBlockTransactionsError
	require.True(
		t,
		errors.As(err, &target),
		"expected LeiosCertifiedBlockTransactionsError, got %v",
		err,
	)
}

func testDijkstraBlockWithBody(
	t *testing.T,
	body DijkstraBlockBody,
) DijkstraBlock {
	t.Helper()
	return DijkstraBlock{
		BlockHeader: &DijkstraBlockHeader{
			BabbageBlockHeader: babbage.BabbageBlockHeader{
				Body: babbage.BabbageBlockHeaderBody{
					BlockBodyHash: body.Hash(),
					VrfKey:        make([]byte, 32),
					VrfResult: common.VrfResult{
						Output: []byte{},
						Proof:  make([]byte, 80),
					},
					OpCert: babbage.BabbageOpCert{
						HotVkey:   make([]byte, 32),
						Signature: make([]byte, 64),
					},
					ProtoVersion: babbage.BabbageProtoVersion{
						Major: MinProtocolVersionDijkstra,
					},
				},
				Signature: make([]byte, 448),
			},
		},
		BlockBody: body,
	}
}
