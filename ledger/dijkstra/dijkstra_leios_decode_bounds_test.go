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
	"unsafe"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func testLeiosCertificate(
	t *testing.T,
	signersLen int,
) *DijkstraLeiosCertificate {
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
	require.NotNil(t, target)
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

// Decoding does not apply the CIP-0164 certificate-or-transaction rule.
// ValidateLeiosBlockBody enforces it during validation.
func TestDijkstraBlockBodyDecodesCertifiedBodyWithTransactions(t *testing.T) {
	body := DijkstraBlockBody{
		Transactions: []DijkstraTransaction{
			{Body: DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
		},
		LeiosCertificate: testLeiosCertificate(t, 1),
	}
	bodyCbor, err := body.MarshalCBOR()
	require.NoError(t, err)

	var decoded DijkstraBlockBody
	require.NoError(t, decoded.UnmarshalCBOR(bodyCbor))
	require.Len(t, decoded.Transactions, 1)
	require.NotNil(t, decoded.LeiosCertificate)
	require.Len(t, decoded.LeiosCertificate.Signers, 1)
	require.Equal(t, bodyCbor, decoded.Cbor())
}

// The pre-respin four-element body is rejected before Leios semantics.
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
	require.ErrorContains(t, err, "expected 3 components")
}

// An empty transaction list alongside a certificate is the legal certified
// block shape.
func TestDijkstraBlockBodyAcceptsCertifiedBodyWithoutTransactions(
	t *testing.T,
) {
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
func TestDijkstraBlockBodyAcceptsPerasCertificateWithTransactions(
	t *testing.T,
) {
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

// NewDijkstraBlockFromCbor is the entry point for blocks off the wire, so the
// envelope and body-hash path must also accept a certified body with
// transactions and leave the CIP-0164 exclusion to ValidateLeiosBlockBody.
func TestDijkstraBlockDecodesCertifiedBodyWithTransactions(t *testing.T) {
	body := DijkstraBlockBody{
		Transactions: []DijkstraTransaction{
			{Body: DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
		},
		LeiosCertificate: testLeiosCertificate(t, 1),
	}
	block := testDijkstraBlockWithBody(t, body)
	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	decoded, err := NewDijkstraBlockFromCbor(blockCbor)
	require.NoError(t, err)
	require.Len(t, decoded.BlockBody.Transactions, 1)
	require.NotNil(t, decoded.BlockBody.LeiosCertificate)
	require.Len(t, decoded.BlockBody.LeiosCertificate.Signers, 1)
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
						Output: make([]byte, 64),
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

// Each one-byte entry would reserve a whole DijkstraTransaction if the
// transaction slice were sized from the entry count, so the slice must grow
// only with entries that decode.
func TestDijkstraBlockBodyTransactionsGrowAsEntriesDecode(t *testing.T) {
	const count = 10_000
	data := []byte{0x83, 0x99, count >> 8, count & 0xff}
	data = append(data, make([]byte, count)...)
	data = append(data, 0xf6, 0xf6)
	var body DijkstraBlockBody
	err := body.UnmarshalCBOR(data)
	require.ErrorContains(t, err, "decode Dijkstra transaction 0")
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			var body DijkstraBlockBody
			if err := body.UnmarshalCBOR(data); err == nil {
				b.Fatal("expected transaction decode error")
			}
		}
	})
	t.Logf(
		"%d one-byte transaction entries failing at the first: %d bytes/op",
		count,
		result.AllocedBytesPerOp(),
	)
	require.Less(
		t,
		result.AllocedBytesPerOp(),
		int64(count*unsafe.Sizeof(DijkstraTransaction{})/4),
	)
}

// The transaction entries decode one at a time, so a declared count reserves
// no per-entry raw message before the first entry fails.
func TestDijkstraBlockBodyTransactionEntriesDecodeOneAtATime(t *testing.T) {
	const count = 10_000
	data := []byte{0x83, 0x99, count >> 8, count & 0xff}
	data = append(data, make([]byte, count)...)
	data = append(data, 0xf6, 0xf6)
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			var body DijkstraBlockBody
			if err := body.UnmarshalCBOR(data); err == nil {
				b.Fatal("expected transaction decode error")
			}
		}
	})
	t.Logf(
		"%d one-byte transaction entries failing at the first: %d bytes/op",
		count,
		result.AllocedBytesPerOp(),
	)
	require.Less(
		t,
		result.AllocedBytesPerOp(),
		int64(count*unsafe.Sizeof(cbor.RawMessage{})/2),
	)
}

func TestDijkstraBlockBodyDecodesIndefiniteTransactionArray(t *testing.T) {
	body := DijkstraBlockBody{
		Transactions: []DijkstraTransaction{
			{Body: DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
			{Body: DijkstraTransactionBody{TxFee: 2}, TxIsValid: false},
		},
	}
	definite, err := body.MarshalCBOR()
	require.NoError(t, err)
	var items []cbor.RawMessage
	_, err = cbor.Decode(definite, &items)
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.Equal(t, byte(0x82), items[0][0])
	txs := append([]byte{0x9f}, items[0][1:]...)
	txs = append(txs, 0xff)
	indefinite, err := cbor.Encode([]cbor.RawMessage{txs, items[1], items[2]})
	require.NoError(t, err)

	var decoded DijkstraBlockBody
	require.NoError(t, decoded.UnmarshalCBOR(indefinite))
	require.Len(t, decoded.Transactions, 2)
	require.Equal(t, uint64(1), decoded.Transactions[0].Body.TxFee)
	require.True(t, decoded.Transactions[0].TxIsValid)
	require.Equal(t, uint64(2), decoded.Transactions[1].Body.TxFee)
	require.False(t, decoded.Transactions[1].TxIsValid)
}

func TestDijkstraBlockBodyRejectsNonArrayTransactions(t *testing.T) {
	var body DijkstraBlockBody
	err := body.UnmarshalCBOR([]byte{0x83, 0xa0, 0xf6, 0xf6})
	require.ErrorContains(t, err, "decode Dijkstra transactions")
}

// testFeeTxs returns count distinct minimal transactions, with fees 1 to
// count.
func testFeeTxs(count int) []DijkstraTransaction {
	txs := make([]DijkstraTransaction, count)
	for i := range txs {
		txs[i] = DijkstraTransaction{
			Body:      DijkstraTransactionBody{TxFee: uint64(i + 1)}, // #nosec G115
			TxIsValid: true,
		}
	}
	return txs
}

// testInlinedCertRb builds a CertRB as LocalChainSync sends it over
// node-to-client: the header's block_body_hash covers the on-chain body (the
// certificate alone), but the body also carries the endorser block's
// transactions.
func testInlinedCertRb(t *testing.T, certified bool) []byte {
	t.Helper()
	cert := testLeiosCertificate(t, 1)
	return testInlinedCertRbWithCert(t, certified, cert, cert)
}

// testInlinedCertRbWithCert is testInlinedCertRb with the header's body hash
// computed over onChainCert and the sent body carrying sentCert.
func testInlinedCertRbWithCert(
	t *testing.T,
	certified bool,
	onChainCert, sentCert *DijkstraLeiosCertificate,
) []byte {
	t.Helper()
	onChain := DijkstraBlockBody{LeiosCertificate: onChainCert}
	block := testDijkstraBlockWithBody(t, onChain)
	certifiedField := cbor.RawMessage{0xf4}
	if certified {
		certifiedField = cbor.RawMessage{0xf5}
	}
	block.BlockHeader.LeiosHeaderExtension = []cbor.RawMessage{
		certifiedField,
		cbor.RawMessage{0xf6},
	}
	block.BlockBody = DijkstraBlockBody{
		Transactions:     testFeeTxs(1),
		LeiosCertificate: sentCert,
	}
	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)
	return blockCbor
}

func TestDijkstraBlockInlinedCertRbBodyHash(t *testing.T) {
	t.Parallel()
	n2c := common.VerifyConfig{AllowCertifiedBlockInlinedTransactions: true}

	t.Run("rejected by default", func(t *testing.T) {
		t.Parallel()
		_, err := NewDijkstraBlockFromCbor(testInlinedCertRb(t, true))
		var vErr *common.ValidationError
		require.ErrorAs(t, err, &vErr)
		require.Equal(t, common.ValidationErrorTypeBodyHash, vErr.Type)
	})

	t.Run("accepted on node-to-client", func(t *testing.T) {
		t.Parallel()
		decoded, err := NewDijkstraBlockFromCbor(
			testInlinedCertRb(t, true),
			n2c,
		)
		require.NoError(t, err)
		require.Len(t, decoded.BlockBody.Transactions, 1)
		require.NotNil(t, decoded.BlockBody.LeiosCertificate)
		certified, present := decoded.BlockHeader.LeiosCertified()
		require.True(t, present)
		require.True(t, certified)
	})

	// Only the transactions are exempt: the rest of the body must still
	// hash to the header's block_body_hash.
	t.Run("tampered certificate rejected", func(t *testing.T) {
		t.Parallel()
		blockCbor := testInlinedCertRbWithCert(
			t,
			true,
			testLeiosCertificate(t, 1),
			testLeiosCertificate(t, 2),
		)
		_, err := NewDijkstraBlockFromCbor(blockCbor, n2c)
		var vErr *common.ValidationError
		require.ErrorAs(t, err, &vErr)
		require.Equal(t, common.ValidationErrorTypeBodyHash, vErr.Type)
	})

	// The exemption needs the header's certified flag, so an uncertified
	// block with a mismatched body is still rejected.
	t.Run("uncertified header still checked", func(t *testing.T) {
		t.Parallel()
		_, err := NewDijkstraBlockFromCbor(testInlinedCertRb(t, false), n2c)
		var vErr *common.ValidationError
		require.ErrorAs(t, err, &vErr)
		require.Equal(t, common.ValidationErrorTypeBodyHash, vErr.Type)
	})

	// A certified block with no transactions is what appears on chain, so the
	// body hash check still applies to it.
	t.Run("certified without transactions still checked", func(t *testing.T) {
		t.Parallel()
		block := testDijkstraBlockWithBody(
			t,
			DijkstraBlockBody{LeiosCertificate: testLeiosCertificate(t, 2)},
		)
		block.BlockHeader.LeiosHeaderExtension = []cbor.RawMessage{
			{0xf5},
			{0xf6},
		}
		block.BlockBody = DijkstraBlockBody{
			LeiosCertificate: testLeiosCertificate(t, 1),
		}
		blockCbor, err := block.MarshalCBOR()
		require.NoError(t, err)
		_, err = NewDijkstraBlockFromCbor(blockCbor, n2c)
		var vErr *common.ValidationError
		require.ErrorAs(t, err, &vErr)
		require.Equal(t, common.ValidationErrorTypeBodyHash, vErr.Type)
	})
}

// OnChainBlockBodyCbor must keep the body's own outer array header, whatever
// framing the decoder accepted, and replace only the transactions field.
func TestOnChainBlockBodyCborKeepsArrayFraming(t *testing.T) {
	t.Parallel()
	sent, err := cbor.Encode(DijkstraBlockBody{
		Transactions:     testFeeTxs(1),
		LeiosCertificate: testLeiosCertificate(t, 1),
	})
	require.NoError(t, err)
	var fields []cbor.RawMessage
	_, err = cbor.Decode(sent, &fields)
	require.NoError(t, err)
	require.Len(t, fields, 3)
	// The indefinite case below swaps this one-byte header for 0x9f.
	require.Equal(t, byte(0x81), fields[0][0], "one-transaction array header")
	rest := append(append([]byte{}, fields[1]...), fields[2]...)

	tests := []struct {
		name   string
		header []byte
		txs    []byte
		footer []byte
	}{
		{name: "definite", header: []byte{0x83}, txs: fields[0]},
		{
			name:   "indefinite",
			header: []byte{0x9f},
			txs:    fields[0],
			footer: []byte{0xff},
		},
		{name: "non-minimal length", header: []byte{0x98, 0x03}, txs: fields[0]},
		{
			name:   "indefinite transactions",
			header: []byte{0x83},
			txs: append(
				append([]byte{0x9f}, fields[0][1:]...),
				0xff,
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := append(append([]byte{}, tc.header...), tc.txs...)
			raw = append(append(raw, rest...), tc.footer...)
			var body DijkstraBlockBody
			require.NoError(t, body.UnmarshalCBOR(raw))
			require.Len(t, body.Transactions, 1)
			block := DijkstraBlock{BlockBody: body}

			got, err := block.OnChainBlockBodyCbor()
			require.NoError(t, err)
			want := append(append([]byte{}, tc.header...), 0x80)
			want = append(append(want, rest...), tc.footer...)
			require.Equal(t, want, got)
		})
	}
}
