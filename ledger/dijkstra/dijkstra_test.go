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
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func testPlutusInteger(v int64) data.PlutusData {
	return data.NewInteger(big.NewInt(v))
}

func TestDijkstraTransactionBodiesUnmarshalCBORCertificateTypes(t *testing.T) {
	decoders := []struct {
		name   string
		decode func([]byte) ([]common.CertificateWrapper, error)
	}{
		{
			name: "top level",
			decode: func(encoded []byte) ([]common.CertificateWrapper, error) {
				var body DijkstraTransactionBody
				err := body.UnmarshalCBOR(encoded)
				return body.TxCertificates, err
			},
		},
		{
			name: "sub transaction",
			decode: func(encoded []byte) ([]common.CertificateWrapper, error) {
				var body DijkstraSubTransactionBody
				err := body.UnmarshalCBOR(encoded)
				return body.TxCertificates, err
			},
		},
	}
	certificates := certificateFixturesByType(t)
	testCases := make([]struct {
		name     string
		certType common.CertificateType
		wantErr  bool
	}, 0, len(certificates))
	for certType := common.CertificateTypeStakeRegistration; certType <= common.CertificateTypeUpdateDrep; certType++ {
		testCases = append(testCases, struct {
			name     string
			certType common.CertificateType
			wantErr  bool
		}{
			name:     fmt.Sprintf("type %d", certType),
			certType: certType,
			wantErr: certType <= common.CertificateTypeStakeDeregistration ||
				certType == common.CertificateTypeGenesisKeyDelegation ||
				certType == common.CertificateTypeMoveInstantaneousRewards,
		})
	}

	for _, decoder := range decoders {
		t.Run(decoder.name, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					fields := map[uint]any{
						0: cbor.NewSetType([]any{}, false),
						1: []any{},
					}
					if decoder.name == "top level" {
						fields[2] = uint64(0)
					}
					fields[4] = []any{certificates[tc.certType]}
					encoded, err := cbor.Encode(fields)
					require.NoError(t, err)

					certificates, err := decoder.decode(encoded)
					if tc.wantErr {
						require.ErrorContains(
							t,
							err,
							"certificate type is not valid in Dijkstra",
						)
						return
					}
					require.NoError(t, err)
					require.Len(t, certificates, 1)
					require.Equal(
						t,
						uint(tc.certType),
						certificates[0].Type,
					)
				})
			}
		})
	}
}

func TestDijkstraTransactionBodiesRejectNegativeCurrentTreasuryValue(
	t *testing.T,
) {
	encoded, err := cbor.Encode(map[uint]any{21: int64(-1)})
	require.NoError(t, err)

	t.Run("top-level", func(t *testing.T) {
		var body DijkstraTransactionBody
		require.Error(t, body.UnmarshalCBOR(encoded))
	})
	t.Run("subtransaction", func(t *testing.T) {
		var body DijkstraSubTransactionBody
		require.Error(t, body.UnmarshalCBOR(encoded))
	})
}

func certificateFixturesByType(t *testing.T) map[common.CertificateType]any {
	t.Helper()
	credential := common.Credential{
		CredType: common.CredentialTypeAddrKeyHash,
	}
	poolRegistration := &common.PoolRegistrationCertificate{
		CertType: uint(common.CertificateTypePoolRegistration),
		Margin:   cbor.Rat{Rat: big.NewRat(0, 1)},
	}
	require.NoError(t, poolRegistration.SetRewardAccountCredential(
		credential,
		common.AddressNetworkTestnet,
	))
	return map[common.CertificateType]any{
		common.CertificateTypeStakeRegistration: &common.StakeRegistrationCertificate{
			CertType: uint(common.CertificateTypeStakeRegistration),
		},
		common.CertificateTypeStakeDeregistration: &common.StakeDeregistrationCertificate{
			CertType: uint(common.CertificateTypeStakeDeregistration),
		},
		common.CertificateTypeStakeDelegation: &common.StakeDelegationCertificate{
			CertType:        uint(common.CertificateTypeStakeDelegation),
			StakeCredential: &credential,
		},
		common.CertificateTypePoolRegistration: poolRegistration,
		common.CertificateTypePoolRetirement: &common.PoolRetirementCertificate{
			CertType: uint(common.CertificateTypePoolRetirement),
		},
		common.CertificateTypeGenesisKeyDelegation: &common.GenesisKeyDelegationCertificate{
			CertType: uint(common.CertificateTypeGenesisKeyDelegation),
		},
		common.CertificateTypeMoveInstantaneousRewards: []any{
			uint(common.CertificateTypeMoveInstantaneousRewards),
			[]any{uint(0), uint64(0)},
		},
		common.CertificateTypeRegistration: &common.RegistrationCertificate{
			CertType: uint(common.CertificateTypeRegistration),
		},
		common.CertificateTypeDeregistration: &common.DeregistrationCertificate{
			CertType: uint(common.CertificateTypeDeregistration),
		},
		common.CertificateTypeVoteDelegation: &common.VoteDelegationCertificate{
			CertType: uint(common.CertificateTypeVoteDelegation),
			Drep:     common.Drep{Type: common.DrepTypeAbstain},
		},
		common.CertificateTypeStakeVoteDelegation: &common.StakeVoteDelegationCertificate{
			CertType: uint(common.CertificateTypeStakeVoteDelegation),
			Drep:     common.Drep{Type: common.DrepTypeAbstain},
		},
		common.CertificateTypeStakeRegistrationDelegation: &common.StakeRegistrationDelegationCertificate{
			CertType: uint(common.CertificateTypeStakeRegistrationDelegation),
		},
		common.CertificateTypeVoteRegistrationDelegation: &common.VoteRegistrationDelegationCertificate{
			CertType: uint(common.CertificateTypeVoteRegistrationDelegation),
			Drep:     common.Drep{Type: common.DrepTypeAbstain},
		},
		common.CertificateTypeStakeVoteRegistrationDelegation: &common.StakeVoteRegistrationDelegationCertificate{
			CertType: uint(
				common.CertificateTypeStakeVoteRegistrationDelegation,
			),
			Drep: common.Drep{Type: common.DrepTypeAbstain},
		},
		common.CertificateTypeAuthCommitteeHot: &common.AuthCommitteeHotCertificate{
			CertType: uint(common.CertificateTypeAuthCommitteeHot),
		},
		common.CertificateTypeResignCommitteeCold: &common.ResignCommitteeColdCertificate{
			CertType: uint(common.CertificateTypeResignCommitteeCold),
		},
		common.CertificateTypeRegistrationDrep: &common.RegistrationDrepCertificate{
			CertType: uint(common.CertificateTypeRegistrationDrep),
		},
		common.CertificateTypeDeregistrationDrep: &common.DeregistrationDrepCertificate{
			CertType: uint(common.CertificateTypeDeregistrationDrep),
		},
		common.CertificateTypeUpdateDrep: &common.UpdateDrepCertificate{
			CertType: uint(common.CertificateTypeUpdateDrep),
		},
	}
}

func minimalTxBody() map[uint]any {
	return map[uint]any{
		0: []any{},
		1: []any{},
		2: uint64(0),
	}
}

func minimalWitnessSet() map[uint]any {
	return map[uint]any{}
}

func minimalTxParts() []any {
	return []any{minimalTxBody(), minimalWitnessSet(), nil}
}

func testDuplicatePolicyMultiAssetCbor(policyByte byte) []byte {
	policy := bytes.Repeat([]byte{policyByte}, common.Blake2b224Size)
	ret := []byte{0xa2, 0x58, 0x1c}
	ret = append(ret, policy...)
	ret = append(ret, 0xa1, 0x41, 0xaa, 0x01, 0x58, 0x1c)
	ret = append(ret, policy...)
	ret = append(ret, 0xa1, 0x41, 0xbb, 0x02)
	return ret
}

func testDuplicateAssetNameMultiAssetCbor(policyByte byte) []byte {
	policy := bytes.Repeat([]byte{policyByte}, common.Blake2b224Size)
	ret := []byte{0xa1, 0x58, 0x1c}
	ret = append(ret, policy...)
	ret = append(ret, 0xa2, 0x41, 0xcc, 0x01, 0x41, 0xcc, 0x09)
	return ret
}

func testDijkstraOutputWithAssetsCbor(t *testing.T, assets []byte) []byte {
	t.Helper()
	addrBytes, err := hex.DecodeString(
		"40000000000000000000000000000000000000000000000000000000008198bd431b03",
	)
	require.NoError(t, err)
	addr, err := common.NewAddressFromBytes(addrBytes)
	require.NoError(t, err)
	addrCbor, err := cbor.Encode(addr)
	require.NoError(t, err)

	ret := []byte{0xa2, 0x00}
	ret = append(ret, addrCbor...)
	ret = append(ret, 0x01, 0x82, 0x01)
	ret = append(ret, assets...)
	return ret
}

// minimalBlockBodyParts builds a Dijkstra 3-element block_body containing a
// single 4-field block transaction with the requested validity.
//
//	[ [transaction], leios_cert/nil, peras_cert/nil ]
func minimalBlockBodyParts(valid bool) []any {
	return []any{
		[]any{[]any{minimalTxParts()[0], minimalTxParts()[1], minimalTxParts()[2], valid}},
		nil,
		nil,
	}
}

// minimalLegacyBlockBodyParts builds the pre-respin four-element block_body
// containing a legacy invalid_transactions set and a three-field transaction.
func minimalLegacyBlockBodyParts(invalidTxs []uint64) []any {
	return []any{
		invalidTxs,
		[]any{minimalTxParts()},
		nil,
		nil,
	}
}

func TestDijkstraEra(t *testing.T) {
	require.Equal(t, uint8(EraIdDijkstra), EraDijkstra.Id)
	require.Equal(t, EraNameDijkstra, EraDijkstra.Name)
	require.Equal(t, EraDijkstra, common.EraById(EraIdDijkstra))
}

func TestDijkstraTypeConstants(t *testing.T) {
	require.Equal(t, 8, BlockTypeDijkstra)
	require.Equal(t, 7, BlockHeaderTypeDijkstra)
	require.Equal(t, 7, TxTypeDijkstra)
}

func TestDijkstraTransactionDecodesThreePartTx(t *testing.T) {
	txCbor, err := cbor.Encode(minimalTxParts())
	require.NoError(t, err)

	tx, err := NewDijkstraTransactionFromCbor(txCbor)
	require.NoError(t, err)
	require.True(t, tx.IsValid())
	require.Equal(t, TxTypeDijkstra, tx.Type())
	require.Equal(t, txCbor, tx.Cbor())
}

func TestDijkstraTransactionAllowsOnlyTrueIsValidForMempool(t *testing.T) {
	parts := minimalTxParts()
	withTrue := []any{parts[0], parts[1], true, parts[2]}
	txCbor, err := cbor.Encode(withTrue)
	require.NoError(t, err)

	tx, err := NewDijkstraTransactionFromCbor(txCbor)
	require.NoError(t, err)
	require.True(t, tx.IsValid())

	withFalse := []any{parts[0], parts[1], false, parts[2]}
	txCbor, err = cbor.Encode(withFalse)
	require.NoError(t, err)

	_, err = NewDijkstraTransactionFromCbor(txCbor)
	require.ErrorContains(t, err, "is_valid=false")
}

// oversizedTxParts builds a well-formed Dijkstra transaction whose CBOR exceeds
// the current Cardano max_tx_size of 16384 bytes. The bulk is carried by
// direct_deposits (body key 25), a Dijkstra-only key, so the result is also a
// Dijkstra era candidate for DetermineTransactionType.
func oversizedTxParts(entries int) []any {
	deposits := make(map[cbor.ByteString]uint64, entries)
	for i := range entries {
		credential := bytes.Repeat([]byte{0x00}, common.Blake2b224Size)
		credential[0] = byte(i)
		credential[1] = byte(i >> 8)
		credential[2] = byte(i >> 16)
		address := append([]byte{0xe0}, credential...)
		deposits[cbor.NewByteString(address)] = uint64(i + 1)
	}
	body := minimalTxBody()
	body[25] = deposits
	return []any{body, minimalWitnessSet(), nil}
}

// TestDijkstraTransactionDecodesOversizedCbor pins that the decoder applies no
// transaction size limit. The reference decoder does not either
// (decodeDijkstraTopTx in eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/Tx.hs),
// and no transaction decoder in any era does; max_tx_size is a protocol
// parameter enforced by UtxoValidateMaxTxSizeUtxo. A decode failure would fail
// the containing block rather than the one transaction.
func TestDijkstraTransactionDecodesOversizedCbor(t *testing.T) {
	txCbor, err := cbor.Encode(oversizedTxParts(600))
	require.NoError(t, err)
	require.Greater(t, len(txCbor), 16*1024)

	tx, err := NewDijkstraTransactionFromCbor(txCbor)
	require.NoError(t, err)
	require.Equal(t, TxTypeDijkstra, tx.Type())
	require.Equal(t, txCbor, tx.Cbor())
	require.Len(t, tx.Body.TxDirectDeposits, 600)
}

// TestDijkstraTransactionRejectsOversizedMalformedCbor is the negative control
// for the removed size check: an oversized payload that is not a Dijkstra
// transaction is still rejected, on its shape rather than on its length.
func TestDijkstraTransactionRejectsOversizedMalformedCbor(t *testing.T) {
	txCbor, err := cbor.Encode(bytes.Repeat([]byte{0x00}, 32*1024))
	require.NoError(t, err)
	require.Greater(t, len(txCbor), 16*1024)

	_, err = NewDijkstraTransactionFromCbor(txCbor)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "MaxTxSize")
}

func TestDijkstraBlockBodyRejectsWrongComponentCount(t *testing.T) {
	for _, arity := range []int{0, 1, 2, 4, 5} {
		t.Run(fmt.Sprintf("arity_%d", arity), func(t *testing.T) {
			parts := make([]any, arity)
			bodyCbor, err := cbor.Encode(parts)
			require.NoError(t, err)
			var blockBody DijkstraBlockBody
			require.ErrorContains(
				t,
				blockBody.UnmarshalCBOR(bodyCbor),
				"expected 3 components",
			)
		})
	}
}

func TestDijkstraBlockBodyRejectsPreRespinLayoutWithMatchingHeaderHash(
	t *testing.T,
) {
	legacyBody, err := cbor.Encode(minimalLegacyBlockBodyParts([]uint64{0}))
	require.NoError(t, err)
	legacyHash := common.Blake2b256Hash(legacyBody)
	header := &DijkstraBlockHeader{
		BabbageBlockHeader: babbage.BabbageBlockHeader{
			Body: babbage.BabbageBlockHeaderBody{
				BlockBodyHash: legacyHash,
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
	}
	blockCbor, err := cbor.Encode([]any{
		header,
		cbor.RawMessage(legacyBody),
	})
	require.NoError(t, err)

	var blockParts []cbor.RawMessage
	_, err = cbor.Decode(blockCbor, &blockParts)
	require.NoError(t, err)
	if len(blockParts) == 0 {
		t.Fatal("block CBOR did not contain a header")
	}
	var decodedHeader DijkstraBlockHeader
	_, err = cbor.Decode(blockParts[0], &decodedHeader)
	require.NoError(t, err)
	require.Equal(t, legacyHash, decodedHeader.BlockBodyHash())

	_, err = NewDijkstraBlockFromCbor(blockCbor)
	require.ErrorContains(t, err, "expected 3 components")
}

func TestDijkstraBlockBodyPreservesRawBlockTransactionCbor(t *testing.T) {
	parts := minimalTxParts()
	body, err := cbor.Encode(parts[0])
	require.NoError(t, err)
	witnesses, err := cbor.Encode(parts[1])
	require.NoError(t, err)
	auxiliary, err := cbor.Encode(parts[2])
	require.NoError(t, err)
	rawTx := []byte{0x9f}
	rawTx = append(rawTx, body...)
	rawTx = append(rawTx, witnesses...)
	rawTx = append(rawTx, auxiliary...)
	rawTx = append(rawTx, 0xf5)
	rawTx = append(rawTx, 0xff)
	bodyCbor, err := cbor.Encode([]any{
		[]cbor.RawMessage{rawTx},
		nil,
		nil,
	})
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	require.NoError(t, blockBody.UnmarshalCBOR(bodyCbor))
	blockBody.SetCbor(nil)
	encoded, err := blockBody.MarshalCBOR()
	require.NoError(t, err)
	var encodedBody []cbor.RawMessage
	_, err = cbor.Decode(encoded, &encodedBody)
	require.NoError(t, err)
	if len(encodedBody) == 0 {
		t.Fatal("encoded block body is empty")
	}
	var encodedTxs []cbor.RawMessage
	_, err = cbor.Decode(encodedBody[0], &encodedTxs)
	require.NoError(t, err)
	if len(encodedTxs) == 0 {
		t.Fatal("encoded transaction list is empty")
	}
	require.Equal(t, rawTx, []byte(encodedTxs[0]))
}

func TestDijkstraBlockBodyEncodesCompatibilityInvalidTransactionIndices(t *testing.T) {
	bodyCbor, err := cbor.Encode(minimalBlockBodyParts(true))
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	require.NoError(t, blockBody.UnmarshalCBOR(bodyCbor))
	blockBody.InvalidTransactions = []uint{0}
	blockBody.SetCbor(nil)

	encoded, err := blockBody.MarshalCBOR()
	require.NoError(t, err)
	var decoded DijkstraBlockBody
	require.NoError(t, decoded.UnmarshalCBOR(encoded))
	require.Len(t, decoded.Transactions, 1)
	require.False(t, decoded.Transactions[0].IsValid())
}

func TestDijkstraBlockBodyUsesPerTransactionValidity(t *testing.T) {
	bodyCbor, err := cbor.Encode(minimalBlockBodyParts(false))
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	require.NoError(t, blockBody.UnmarshalCBOR(bodyCbor))
	require.Empty(t, blockBody.InvalidTransactions)
	require.Len(t, blockBody.Transactions, 1)
	require.False(t, blockBody.Transactions[0].IsValid())
}

func TestDijkstraBlockBodyRequiresTrailingTransactionIsValidFlag(t *testing.T) {
	parts := minimalTxParts()
	withoutFlag := []any{parts[0], parts[1], parts[2]}
	bodyCbor, err := cbor.Encode([]any{
		[]any{withoutFlag}, nil, nil,
	})
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	err = blockBody.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "expected 4 components")
}

func TestDijkstraBlockMarshalUsesTwoItemEnvelope(t *testing.T) {
	sig := make([]byte, common.LeiosBlsSignatureSize)
	leiosCert := &DijkstraLeiosCertificate{
		Signers:             []byte{0x01},
		AggregatedSignature: sig,
	}
	// Per CIP-0164 a block that carries a Leios certificate carries no
	// Dijkstra-era transactions.
	block := DijkstraBlock{
		BlockBody: DijkstraBlockBody{
			LeiosCertificate: leiosCert,
		},
	}

	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	// block = [header, block_body]
	var raw []cbor.RawMessage
	_, err = cbor.Decode(blockCbor, &raw)
	require.NoError(t, err)
	require.Len(t, raw, 2)

	// block_body = [transactions, leios_cert/nil, peras_cert/nil]
	var body []cbor.RawMessage
	_, err = cbor.Decode(raw[1], &body)
	require.NoError(t, err)
	require.Len(t, body, 3)
	// No transactions.
	require.Equal(t, []byte{0x80}, []byte(body[0]))
	// Real Leios certificate [signers, aggregated_signature].
	expectedCert, err := cbor.Encode([]any{[]byte{0x01}, sig})
	require.NoError(t, err)
	require.Equal(t, expectedCert, []byte(body[1]))
	// No Peras certificate.
	require.Equal(t, []byte{0xf6}, []byte(body[2]))
}

// leiosExtendedHeaderHex is a real Dijkstra block header captured from the
// Leios prototype testnet (network magic 164) at slot 1309596. After the
// Leios header extension activated mid-Dijkstra, the header body carries an
// 11th element (a [hash, uint] pair) following protocol_version, which a plain
// Babbage header decoder rejects.
const leiosExtendedHeaderHex = "828b19ff661a0013fb9c582017f18b18364e406fdf68a72845f9b005119425ddd344d171a7d5dd005df8b7f058200c058acd8105777ffee584b5bc3e7507fe1615179bb2d291f83b089e7733de5f582017658451b63aa1614b7a93c12533a9d73032a599a411533cf5e88f9fb98343c58258405231c2bde7d989ef9df5f31dd636ad8bb6773ef1d3b1e54f35e9c2ebac647aa525b6a9219763b3a9e285a5f7acd8d1cf8c5c3ed2fc780a9f4b9f1f063a7346a05850d58cc6245cad4d434f6b17fc5af1d9d56df46f7d483af40b134f817903f0061ce3cac58fda15cfe085fa42407d03b40d207dad9d2255791f1c000bfebd36a3017aa274cbb138e6768199574e0466f8091a000151c158206f351bafba4758062454b2125a57419c3599e62be312ed0677411d4ad05c0f6184582022cead0f99f08fb8002d88ce78ec2b8bc2657b0b9e4b96f68b01dcc249e71671000058403f56eec6cb0cb526154a6fd0dcc27fe1efcbeabac244bc49c2c1ac304db52bd5d40da6960c6331a808307340c285b1abc94ea69a947ef64980f0015f1f77a607820c00825820a2dcb7c0d77a9ab7396d734665d5b25f7ead1ee0595704a90f9825c6b533925019567f5901c0103d6a7191e176f98c0dd438ba9fb84f665d17728eb659e26096fee55f9493f8d3914ebd42204b6d1ca0c93d0edab06235e7e44b5458c3ab676763dd1306d60388573c0fd8f581c24f0bbeb91eae9379d175fd6c532120acd570e3b83db66b03dade4cdfa86f4fc857c3c3580eeb14d5eb7922b2c83531aea1c725d9a296ee51a24b6006c0c0b4f0842ed247536aafe86b983252566d869ae04a3bab013ff55a116b7a220285cce2e49355b2e9aa316f3e4f71c4e1ffc880462bc49e39e064c783ce40c166b39673407fea92ed55a9721fdf9d3c06d38ec0edd2278effc70b9c985ec7c776edc22234f7659e4c085821c27756713ad2243080e1e32d28a635ff9f45309b537c142f880ff07898aa4cf5b2f26356e267a2c8fb425b324653c991d0c9138a27891be8d5d8a427446cce884e3cb4b3d9d9b3a8d551a3e0471091a4dffb3ef2868c9982a9fafcc6ef22ba750bf9a38c821e1c496511b98ee90e43b01f088e3a96e2ddb0be9309b63ca34c317e708d0a739f69e979f9b9a97cd608e1a64266877574542014af4f55223c6ff56eaa98730bde2ad7bef8d164b5a4d0a729e746714501986f1d97c69e5645befd8771bf628a3fdf6011bf8e438aaadc35"

// TestDijkstraBlockHeaderDecodesLeiosExtension verifies that the Leios-extended
// Dijkstra block header (11-field body) decodes, exposes the standard Babbage
// fields, retains the extra Leios field, and round-trips byte-for-byte so the
// header hash is stable.
func TestDijkstraBlockHeaderDecodesLeiosExtension(t *testing.T) {
	raw, err := hex.DecodeString(leiosExtendedHeaderHex)
	require.NoError(t, err)

	var header DijkstraBlockHeader
	_, err = cbor.Decode(raw, &header)
	require.NoError(t, err)

	// Standard Babbage header fields decode positionally.
	require.Equal(t, uint64(65382), header.BlockNumber())
	require.Equal(t, uint64(1309596), header.SlotNumber())

	// The 11th body element is retained verbatim.
	require.Len(t, header.LeiosHeaderExtension, 1)

	// The body's stored CBOR must be the ORIGINAL (Leios-extended) body bytes,
	// not a re-encoding of the leading Babbage fields: KES signature
	// verification (ledger.extractOriginalBodyCbor) is computed over the
	// original header-body encoding.
	var top []cbor.RawMessage
	_, err = cbor.Decode(raw, &top)
	require.NoError(t, err)
	if len(top) == 0 {
		t.Fatal("expected header CBOR to contain a body")
	}
	require.Equal(t, []byte(top[0]), header.Body.Cbor())

	// Round-trips byte-for-byte so the header hash matches the wire bytes.
	out, err := header.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, raw, out)
	require.Equal(t, common.Blake2b256Hash(raw), header.Hash())
}

// TestDijkstraBlockHeaderDecodesLegacyBabbageBody verifies that a plain
// 10-field Babbage-shaped Dijkstra header (pre-Leios-extension) still decodes
// with no Leios extension captured.
func TestDijkstraBlockHeaderDecodesLegacyBabbageBody(t *testing.T) {
	// Re-encode the captured header with only the first 10 body fields to
	// model a legacy Dijkstra header.
	full, err := hex.DecodeString(leiosExtendedHeaderHex)
	require.NoError(t, err)
	var top []cbor.RawMessage
	_, err = cbor.Decode(full, &top)
	require.NoError(t, err)
	if len(top) < 2 {
		t.Fatal("expected header CBOR to contain body and signature")
	}
	var bodyElems []cbor.RawMessage
	_, err = cbor.Decode(top[0], &bodyElems)
	require.NoError(t, err)
	if len(bodyElems) < 10 {
		t.Fatal("expected at least 10 header body fields")
	}
	legacyBody, err := cbor.Encode(bodyElems[:10])
	require.NoError(t, err)
	legacyHeader, err := cbor.Encode([]cbor.RawMessage{
		cbor.RawMessage(legacyBody),
		top[1],
	})
	require.NoError(t, err)

	var header DijkstraBlockHeader
	_, err = cbor.Decode(legacyHeader, &header)
	require.NoError(t, err)
	require.Nil(t, header.LeiosHeaderExtension)
	require.Equal(t, uint64(65382), header.BlockNumber())
	require.Equal(t, uint64(1309596), header.SlotNumber())
}

func TestDijkstraBlockBodyHashIncludesLeiosAndPerasCertSlots(t *testing.T) {
	withoutCerts := DijkstraBlockBody{}
	withCerts := DijkstraBlockBody{
		LeiosCertificate: &DijkstraLeiosCertificate{},
		PerasCertificate: []byte{0x01},
	}

	require.NotEqual(t, withoutCerts.Hash(), withCerts.Hash())
}

// prototype-2026w27 replaced the empty-list leios_cert placeholder with a real
// two-field certificate (IntersectMBO/cardano-ledger #5872); the old empty-list
// form is no longer valid. Full round-trip coverage is in
// dijkstra_leios_w27_test.go.
func TestDijkstraLeiosCertificateRejectsEmptyPlaceholder(t *testing.T) {
	empty, err := cbor.Encode([]any{})
	require.NoError(t, err)
	require.Equal(t, []byte{0x80}, empty)

	var cert DijkstraLeiosCertificate
	require.Error(t, cert.UnmarshalCBOR(empty))

	sig := make([]byte, common.LeiosBlsSignatureSize)
	realCert, err := cbor.Encode([]any{[]byte{0x03}, sig})
	require.NoError(t, err)
	require.NoError(t, cert.UnmarshalCBOR(realCert))
	require.Equal(t, []byte{0x03}, cert.Signers)
	require.Len(t, cert.AggregatedSignature, common.LeiosBlsSignatureSize)
}

func TestDijkstraBlockRoundTripWithBodyHash(t *testing.T) {
	blockBody := DijkstraBlockBody{
		Transactions:        []DijkstraTransaction{},
		InvalidTransactions: []uint{},
	}
	block := DijkstraBlock{
		BlockHeader: &DijkstraBlockHeader{
			BabbageBlockHeader: babbage.BabbageBlockHeader{
				Body: babbage.BabbageBlockHeaderBody{
					BlockBodyHash: blockBody.Hash(),
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
		BlockBody: blockBody,
	}

	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	decoded, err := NewDijkstraBlockFromCbor(blockCbor)
	require.NoError(t, err)
	require.Equal(t, blockBody.Hash(), decoded.BlockBodyHash())
	require.Empty(t, decoded.Transactions())

	var raw []cbor.RawMessage
	_, err = cbor.Decode(blockCbor, &raw)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	require.NotEmpty(t, decoded.BlockHeader.Cbor())
	require.Equal(t, []byte(raw[0]), decoded.BlockHeader.Cbor())
}

// TestDijkstraBlockNonEmptyTransactionsValidity exercises a synthetic block
// with two inline transactions and per-transaction validity flags. It confirms
// the block/body wire shape, body-hash validation, and validity propagation.
func TestDijkstraBlockNonEmptyTransactionsValidity(t *testing.T) {
	blockBody := DijkstraBlockBody{
		Transactions: []DijkstraTransaction{
			{Body: DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
			{Body: DijkstraTransactionBody{TxFee: 2}, TxIsValid: false},
		},
	}
	block := DijkstraBlock{
		BlockHeader: &DijkstraBlockHeader{
			BabbageBlockHeader: babbage.BabbageBlockHeader{
				Body: babbage.BabbageBlockHeaderBody{
					BlockBodyHash: blockBody.Hash(),
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
		BlockBody: blockBody,
	}

	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	// Wire shape: block = [header, block_body];
	// block_body = [[tx1, tx2], nil, nil]
	var raw []cbor.RawMessage
	_, err = cbor.Decode(blockCbor, &raw)
	require.NoError(t, err)
	require.Len(t, raw, 2)
	var body []cbor.RawMessage
	_, err = cbor.Decode(raw[1], &body)
	require.NoError(t, err)
	require.Len(t, body, 3)
	var wireTxs []cbor.RawMessage
	_, err = cbor.Decode(body[0], &wireTxs)
	require.NoError(t, err)
	require.Len(t, wireTxs, 2)
	// Each block transaction has a trailing is_valid flag.
	for _, wt := range wireTxs {
		var txFields []cbor.RawMessage
		_, err = cbor.Decode(wt, &txFields)
		require.NoError(t, err)
		require.Len(t, txFields, 4)
	}
	require.Equal(t, []byte{0xf6}, []byte(body[2])) // no peras cert

	// Body-hash validation is enabled by default and must pass.
	decoded, err := NewDijkstraBlockFromCbor(blockCbor)
	require.NoError(t, err)
	require.Equal(t, blockBody.Hash(), decoded.BlockBodyHash())
	txs := decoded.Transactions()
	require.Len(t, txs, 2)
	require.True(t, txs[0].IsValid())
	require.False(t, txs[1].IsValid())
	require.Equal(t, int64(1), txs[0].Fee().Int64())
	require.Equal(t, int64(2), txs[1].Fee().Int64())

	require.Nil(t, decoded.BlockBody.LeiosCertificate)
}

func TestDijkstraRedeemersRejectsDuplicateMapKey(t *testing.T) {
	// Craft a raw CBOR map with two identical RedeemerKey entries.
	// RedeemerKey{Tag:0, Index:0} encodes as StructAsArray [0,0] = 82 00 00.
	// RedeemerValue{Datum(int 1), ExUnits{1,1}} encodes as [1,[1,1]] = 82 01 82 01 01.
	// DupMapKeyEnforcedAPF on the shared decode mode must reject this before rule evaluation.
	dupCbor := []byte{
		0xa2,             // map(2)
		0x82, 0x00, 0x00, // key:   [0, 0]
		0x82, 0x01, 0x82, 0x01, 0x01, // value: [1, [1, 1]]
		0x82, 0x00, 0x00, // key:   [0, 0]  -- duplicate
		0x82, 0x02, 0x82, 0x02, 0x02, // value: [2, [2, 2]]
	}
	var r DijkstraRedeemers
	err := r.UnmarshalCBOR(dupCbor)
	require.Error(t, err)
}

func TestDijkstraRejectsDuplicateMultiAssetKeys(t *testing.T) {
	t.Run("body mint duplicate policy", func(t *testing.T) {
		bodyCbor := append(
			[]byte{0xa1, 0x09},
			testDuplicatePolicyMultiAssetCbor(0x44)...,
		)
		var body DijkstraTransactionBody
		err := body.UnmarshalCBOR(bodyCbor)
		require.ErrorContains(t, err, "duplicate map key")
	})
	t.Run("body mint duplicate asset name", func(t *testing.T) {
		bodyCbor := append(
			[]byte{0xa1, 0x09},
			testDuplicateAssetNameMultiAssetCbor(0x55)...,
		)
		var body DijkstraTransactionBody
		err := body.UnmarshalCBOR(bodyCbor)
		require.ErrorContains(t, err, "duplicate map key")
	})
	t.Run("output duplicate asset name", func(t *testing.T) {
		outputCbor := testDijkstraOutputWithAssetsCbor(
			t,
			testDuplicateAssetNameMultiAssetCbor(0x66),
		)
		var output DijkstraTransactionOutput
		err := output.UnmarshalCBOR(outputCbor)
		require.ErrorContains(t, err, "duplicate map key")
	})
	t.Run("subtransaction mint duplicate policy", func(t *testing.T) {
		bodyCbor := append(
			[]byte{0xa1, 0x09},
			testDuplicatePolicyMultiAssetCbor(0x77)...,
		)
		var body DijkstraSubTransactionBody
		err := body.UnmarshalCBOR(bodyCbor)
		require.ErrorContains(t, err, "duplicate map key")
	})
}

func TestDijkstraWitnessSetRejectsDuplicateTaggedVkeyWitness(t *testing.T) {
	// Craft a witness set CBOR where field 0 (vkey witnesses) is a tag-258 set
	// containing two identical VkeyWitness entries.
	// VkeyWitness{Vkey:[0x01], Signature:[0x02]} = 82 41 01 41 02.
	// The Dijkstra guard introduced in UnmarshalCBOR must reject this.
	dupCbor := []byte{
		0xa1,             // map(1)
		0x00,             // key: 0  (VkeyWitnesses field)
		0xd9, 0x01, 0x02, // tag(258) — CBOR set
		0x82,                         // array(2)
		0x82, 0x41, 0x01, 0x41, 0x02, // VkeyWitness{[0x01], [0x02]}
		0x82, 0x41, 0x01, 0x41, 0x02, // duplicate
	}
	var ws DijkstraTransactionWitnessSet
	err := ws.UnmarshalCBOR(dupCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraTransactionBodyRejectsDuplicateSubTransaction(t *testing.T) {
	// Craft a transaction body where field 23 (TxSubTransactions) is a tag-258 set
	// containing two identical minimal sub-transactions.
	// Minimal sub-transaction = [empty-body, empty-witness, null] = 83 a0 a0 f6.
	dupCbor := []byte{
		0xa4,                         // map(4)
		0x00, 0xd9, 0x01, 0x02, 0x80, // empty transaction inputs
		0x01, 0x80, // empty outputs
		0x02, 0x00, // fee
		0x17,             // key: 23 (TxSubTransactions field)
		0xd9, 0x01, 0x02, // tag(258) — CBOR set
		0x82, // array(2)
		0x83, 0xa2, 0x00, 0xd9, 0x01, 0x02, 0x80, 0x01, 0x80,
		0xa0, 0xf6, // sub-tx: [required empty body, empty witness, null]
		0x83, 0xa2, 0x00, 0xd9, 0x01, 0x02, 0x80, 0x01, 0x80,
		0xa0, 0xf6, // duplicate
	}
	var body DijkstraTransactionBody
	err := body.UnmarshalCBOR(dupCbor)
	require.ErrorContains(t, err, "duplicate Dijkstra sub-transaction body")
}

func TestDijkstraTransactionBodyRejectsSubTransactionsWithDuplicateBodyID(
	t *testing.T,
) {
	key := make([]byte, 32)
	signature := make([]byte, 64)
	witnessesA := map[uint]any{
		0: cbor.NewSetType([]common.VkeyWitness{{
			Vkey: key, Signature: signature,
		}}, true),
	}
	witnessesB := map[uint]any{
		0: cbor.NewSetType([]common.VkeyWitness{{
			Vkey:      append([]byte(nil), key...),
			Signature: bytes.Repeat([]byte{1}, 64),
		}}, true),
	}
	body := map[uint]any{
		0: cbor.NewSetType([]shelley.ShelleyTransactionInput{}, true),
		1: []any{},
		3: uint64(10),
	}
	for _, tc := range []struct {
		name       string
		witnesses  map[uint]any
		auxiliaryA any
		auxiliaryB any
	}{
		{name: "witnesses differ", witnesses: witnessesB},
		{
			name:       "auxiliary data differs",
			witnesses:  witnessesA,
			auxiliaryB: map[uint]any{1: "metadata"},
		},
		{
			name:       "witnesses and auxiliary data differ",
			witnesses:  witnessesB,
			auxiliaryB: map[uint]any{1: "metadata"},
		},
	} {
		for _, tagged := range []bool{false, true} {
			name := tc.name + "/untagged"
			if tagged {
				name = tc.name + "/tagged"
			}
			t.Run(name, func(t *testing.T) {
				firstBytes, err := cbor.Encode(
					[]any{body, witnessesA, tc.auxiliaryA},
				)
				require.NoError(t, err)
				secondBytes, err := cbor.Encode(
					[]any{body, tc.witnesses, tc.auxiliaryB},
				)
				require.NoError(t, err)
				var first, second DijkstraSubTransaction
				require.NoError(t, first.UnmarshalCBOR(firstBytes))
				require.NoError(t, second.UnmarshalCBOR(secondBytes))
				require.Equal(t, first.Body.Id(), second.Body.Id())
				require.NotEqual(t, first.Cbor(), second.Cbor())

				subTransactions := []any{
					[]any{body, witnessesA, tc.auxiliaryA},
					[]any{body, tc.witnesses, tc.auxiliaryB},
				}
				bodyValue := map[uint]any{
					0:  cbor.NewSetType([]shelley.ShelleyTransactionInput{}, true),
					1:  []any{},
					2:  uint64(0),
					23: subTransactions,
				}
				if tagged {
					bodyValue[23] = cbor.NewSetType(subTransactions, true)
				}
				bodyCbor, err := cbor.Encode(bodyValue)
				require.NoError(t, err)
				var decoded DijkstraTransactionBody
				require.ErrorContains(
					t, decoded.UnmarshalCBOR(bodyCbor),
					"duplicate Dijkstra sub-transaction body",
				)
			})
		}
	}
}

func TestDijkstraTransactionBodyRejectsExplicitlyEmptySubTransactions(t *testing.T) {
	for _, value := range []any{[]any{}, cbor.NewSetType([]any{}, true)} {
		bodyCbor, err := cbor.Encode(map[uint]any{
			0:  cbor.NewSetType([]any{}, false),
			1:  []any{},
			2:  uint64(0),
			23: value,
		})
		require.NoError(t, err)
		var body DijkstraTransactionBody
		require.ErrorContains(t, body.UnmarshalCBOR(bodyCbor), "must not be empty")
	}
}

func TestDijkstraTransactionBodyAcceptsDistinctSubTransactionBodies(t *testing.T) {
	subBody := func(ttl uint64) map[uint]any {
		return map[uint]any{
			0: cbor.NewSetType([]shelley.ShelleyTransactionInput{}, true),
			1: []any{},
			3: ttl,
		}
	}
	bodyCbor, err := cbor.Encode(map[uint]any{
		0: cbor.NewSetType([]shelley.ShelleyTransactionInput{}, true),
		1: []any{},
		2: uint64(0),
		23: []any{
			[]any{subBody(10), map[uint]any{}, nil},
			[]any{subBody(11), map[uint]any{}, nil},
		},
	})
	require.NoError(t, err)
	var body DijkstraTransactionBody
	require.NoError(t, body.UnmarshalCBOR(bodyCbor))
	require.Len(t, body.TxSubTransactions.Items(), 2)
}

func TestDijkstraSubTransactionsDeduplicateByBodyID(t *testing.T) {
	bodyCBOR := func(donation uint64) cbor.RawMessage {
		encoded, err := cbor.Encode(map[uint]any{
			0:  []any{},
			1:  []any{},
			22: donation,
		})
		require.NoError(t, err)
		return encoded
	}
	witnessCBOR := func(key byte) cbor.RawMessage {
		witnesses := map[uint]any{}
		if key != 0 {
			witnesses[0] = cbor.NewSetType([]common.VkeyWitness{{
				Vkey:      []byte{key},
				Signature: []byte{key},
			}}, true)
		}
		encoded, err := cbor.Encode(witnesses)
		require.NoError(t, err)
		return encoded
	}
	metadataCBOR := func(value uint64) cbor.RawMessage {
		if value == 0 {
			return cbor.RawMessage{0xf6}
		}
		encoded, err := cbor.Encode(map[uint]any{1: value})
		require.NoError(t, err)
		return encoded
	}
	subTransactionCBOR := func(
		body, witnesses, metadata cbor.RawMessage,
	) cbor.RawMessage {
		encoded, err := cbor.Encode([]cbor.RawMessage{
			body,
			witnesses,
			metadata,
		})
		require.NoError(t, err)
		return encoded
	}
	transactionCBOR := func(
		tagged bool,
		subTransactions ...cbor.RawMessage,
	) []byte {
		encodedSet, err := cbor.Encode(subTransactions)
		require.NoError(t, err)
		if tagged {
			encodedSet = append([]byte{0xd9, 0x01, 0x02}, encodedSet...)
		}
		encoded, err := cbor.Encode(map[uint]any{
			0:  []any{},
			1:  []any{},
			2:  uint64(1),
			23: cbor.RawMessage(encodedSet),
		})
		require.NoError(t, err)
		return encoded
	}

	body := bodyCBOR(1)
	witnessA, witnessB := witnessCBOR(1), witnessCBOR(2)
	metadataA, metadataB := metadataCBOR(1), metadataCBOR(2)
	testCases := []struct {
		name      string
		first     cbor.RawMessage
		second    cbor.RawMessage
		wantErr   bool
		taggedSet bool
	}{
		{
			name:    "different witnesses, untagged",
			first:   subTransactionCBOR(body, witnessA, metadataCBOR(0)),
			second:  subTransactionCBOR(body, witnessB, metadataCBOR(0)),
			wantErr: true,
		},
		{
			name:      "different auxiliary data, tagged",
			first:     subTransactionCBOR(body, witnessCBOR(0), metadataA),
			second:    subTransactionCBOR(body, witnessCBOR(0), metadataB),
			wantErr:   true,
			taggedSet: true,
		},
		{
			name:    "different witnesses and auxiliary data",
			first:   subTransactionCBOR(body, witnessA, metadataA),
			second:  subTransactionCBOR(body, witnessB, metadataB),
			wantErr: true,
		},
		{
			name:   "distinct bodies preserve order",
			first:  subTransactionCBOR(body, witnessA, metadataA),
			second: subTransactionCBOR(bodyCBOR(2), witnessB, metadataB),
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			firstBody := DijkstraSubTransaction{}
			_, err := cbor.Decode(test.first, &firstBody)
			require.NoError(t, err)
			secondBody := DijkstraSubTransaction{}
			_, err = cbor.Decode(test.second, &secondBody)
			require.NoError(t, err)
			if test.wantErr {
				require.Equal(t, firstBody.Body.Id(), secondBody.Body.Id())
			} else {
				require.NotEqual(t, firstBody.Body.Id(), secondBody.Body.Id())
			}

			var decoded DijkstraTransactionBody
			err = decoded.UnmarshalCBOR(transactionCBOR(
				test.taggedSet,
				test.first,
				test.second,
			))
			if test.wantErr {
				require.ErrorContains(
					t,
					err,
					"duplicate Dijkstra sub-transaction body",
				)
				return
			}
			require.NoError(t, err)
			items := decoded.TxSubTransactions.Items()
			require.Len(t, items, 2)
			require.Equal(t, uint64(1), items[0].Body.TxDonation)
			require.Equal(t, uint64(2), items[1].Body.TxDonation)
		})
	}
}

func TestDijkstraTransactionBodyRejectsDuplicateTaggedInputs(t *testing.T) {
	input := testShelleyInput()
	bodyCbor, err := cbor.Encode(map[uint]any{
		1: []any{},
		2: uint64(0),
		0: cbor.NewSetType(
			[]shelley.ShelleyTransactionInput{input, input},
			true,
		),
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraTransactionBodyRejectsDuplicateTaggedInputSets(t *testing.T) {
	input := testShelleyInput()
	for _, tt := range []struct {
		name  string
		field uint
	}{
		{name: "collateral", field: 13},
		{name: "reference", field: 18},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bodyCbor, err := cbor.Encode(map[uint]any{
				0: cbor.NewSetType([]any{}, false),
				1: []any{},
				2: uint64(0),
				tt.field: cbor.NewSetType(
					[]shelley.ShelleyTransactionInput{input, input},
					true,
				),
			})
			require.NoError(t, err)

			var body DijkstraTransactionBody
			err = body.UnmarshalCBOR(bodyCbor)
			require.ErrorContains(t, err, "duplicate member in set")
		})
	}
}

func TestDijkstraRejectsDuplicateUntaggedInputSets(t *testing.T) {
	input := testShelleyInput()
	for _, tt := range []struct {
		name  string
		field uint
	}{
		{name: "regular", field: 0},
		{name: "collateral", field: 13},
		{name: "reference", field: 18},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bodyCbor, err := cbor.Encode(map[uint]any{
				0: cbor.NewSetType([]any{}, false),
				1: []any{},
				2: uint64(0),
				tt.field: cbor.NewSetType(
					[]shelley.ShelleyTransactionInput{input, input},
					false,
				),
			})
			require.NoError(t, err)

			var body DijkstraTransactionBody
			require.ErrorContains(
				t,
				body.UnmarshalCBOR(bodyCbor),
				"duplicate member in set",
			)
		})
	}
}

func TestDijkstraTransactionBodyRejectsDuplicateSubTransactionInputs(
	t *testing.T,
) {
	input := testShelleyInput()
	subTx := []any{
		map[uint]any{
			0: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{input, input},
				true,
			),
			1: []any{},
		},
		map[uint]any{},
		nil,
	}
	bodyCbor, err := cbor.Encode(map[uint]any{
		0:  cbor.NewSetType([]any{}, false),
		1:  []any{},
		2:  uint64(0),
		23: cbor.NewSetType([]any{subTx}, true),
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraTransactionBodyRejectsDuplicateSubTransactionReferenceInputs(
	t *testing.T,
) {
	refInput := testShelleyInput()
	subTx := []any{
		map[uint]any{
			0: cbor.NewSetType([]any{}, false),
			1: []any{},
			18: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{refInput, refInput},
				true,
			),
		},
		map[uint]any{},
		nil,
	}
	bodyCbor, err := cbor.Encode(map[uint]any{
		0:  cbor.NewSetType([]any{}, false),
		1:  []any{},
		2:  uint64(0),
		23: cbor.NewSetType([]any{subTx}, true),
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraRejectsDuplicateProposalProcedures(t *testing.T) {
	rewardAccount, err := common.NewAddress(
		"stake_test1uqehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gssrtvn",
	)
	require.NoError(t, err)
	action, err := common.NewInfoGovAction()
	require.NoError(t, err)
	procedure := DijkstraProposalProcedure{
		PPRewardAccount: rewardAccount,
		PPGovAction: DijkstraGovAction{
			Action: action,
		},
	}
	for _, tt := range []struct {
		name  string
		field uint
	}{
		{name: "transaction body", field: 20},
		{name: "subtransaction body", field: 20},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bodyCbor, err := cbor.Encode(map[uint]any{
				tt.field: []DijkstraProposalProcedure{procedure, procedure},
			})
			require.NoError(t, err)

			if tt.name == "transaction body" {
				var body DijkstraTransactionBody
				err = body.UnmarshalCBOR(bodyCbor)
			} else {
				var body DijkstraSubTransactionBody
				err = body.UnmarshalCBOR(bodyCbor)
			}
			require.ErrorContains(t, err, "duplicate member in set")
		})
	}
}

func TestDijkstraProposalProcedureRejectsBaseAddress(t *testing.T) {
	baseBytes := append([]byte{common.AddressTypeKeyKey << 4}, make([]byte, 56)...)
	actionWire, err := cbor.Encode(common.InfoGovAction{Type: uint(common.GovActionTypeInfo)})
	require.NoError(t, err)
	wire, err := cbor.Encode([]any{
		uint64(0), baseBytes, cbor.RawMessage(actionWire), common.GovAnchor{},
	})
	require.NoError(t, err)
	var decoded DijkstraProposalProcedure
	require.ErrorContains(t, decoded.UnmarshalCBOR(wire), "invalid account address type")
	proceduresWire, err := cbor.Encode([]any{cbor.RawMessage(wire)})
	require.NoError(t, err)
	bodyWire, err := cbor.Encode(map[uint]any{20: cbor.RawMessage(proceduresWire)})
	require.NoError(t, err)
	t.Run("top-level body", func(t *testing.T) {
		var body DijkstraTransactionBody
		require.ErrorContains(t, body.UnmarshalCBOR(bodyWire), "invalid account address type")
	})
	t.Run("child body", func(t *testing.T) {
		var body DijkstraSubTransactionBody
		require.ErrorContains(t, body.UnmarshalCBOR(bodyWire), "invalid account address type")
	})
}

func testShelleyInput() shelley.ShelleyTransactionInput {
	var txId common.Blake2b256
	txId[0] = 1
	return shelley.ShelleyTransactionInput{
		TxId:        txId,
		OutputIndex: 0,
	}
}

func TestDijkstraTransactionBodyRejectsDuplicateCredentialGuards(t *testing.T) {
	var hash common.Blake2b224
	hash[0] = 1
	guard := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: hash,
	}
	bodyCbor, err := cbor.Encode(map[uint]any{
		14: cbor.NewSetType([]common.Credential{guard, guard}, true),
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraTransactionBodyRejectsDuplicateKeyHashGuards(t *testing.T) {
	var hash common.Blake2b224
	hash[0] = 1
	bodyCbor, err := cbor.Encode(map[uint]any{
		14: cbor.NewSetType([]common.Blake2b224{hash, hash}, true),
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraAccountBalanceIntervalCBOR(t *testing.T) {
	tests := []struct {
		name    string
		raw     []any
		wantErr string
		check   func(t *testing.T, interval DijkstraAccountBalanceInterval)
	}{
		{
			name: "both bounds",
			raw:  []any{uint64(10), uint64(20)},
			check: func(t *testing.T, interval DijkstraAccountBalanceInterval) {
				require.Equal(t, uint64(10), *interval.LowerBound)
				require.Equal(t, uint64(20), *interval.UpperBound)
				require.Nil(t, interval.Exact)
			},
		},
		{
			name: "lower only",
			raw:  []any{uint64(10), nil},
			check: func(t *testing.T, interval DijkstraAccountBalanceInterval) {
				require.Equal(t, uint64(10), *interval.LowerBound)
				require.Nil(t, interval.UpperBound)
			},
		},
		{
			name: "upper only",
			raw:  []any{nil, uint64(20)},
			check: func(t *testing.T, interval DijkstraAccountBalanceInterval) {
				require.Nil(t, interval.LowerBound)
				require.Equal(t, uint64(20), *interval.UpperBound)
			},
		},
		{
			name:    "both bounds nil is rejected",
			raw:     []any{nil, nil},
			wantErr: "requires a lower or upper bound",
		},
		{
			name:    "wrong array length is rejected",
			raw:     []any{uint64(10), uint64(20), uint64(30)},
			wantErr: "invalid Dijkstra account balance interval encoding",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := cbor.Encode(test.raw)
			require.NoError(t, err)
			var interval DijkstraAccountBalanceInterval
			_, err = cbor.Decode(raw, &interval)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			test.check(t, interval)
			reencoded, err := cbor.Encode(interval)
			require.NoError(t, err)
			require.Equal(t, raw, reencoded)
		})
	}
}

func TestDijkstraAccountBalanceIntervalExactCBOR(t *testing.T) {
	raw, err := cbor.Encode(uint64(42))
	require.NoError(t, err)
	var interval DijkstraAccountBalanceInterval
	_, err = cbor.Decode(raw, &interval)
	require.NoError(t, err)
	require.Equal(t, uint64(42), *interval.Exact)
	require.Nil(t, interval.LowerBound)
	require.Nil(t, interval.UpperBound)
	reencoded, err := cbor.Encode(interval)
	require.NoError(t, err)
	require.Equal(t, raw, reencoded)
}

func TestDijkstraTransactionBodyBalanceIntervalsRejectsEmptyMap(t *testing.T) {
	bodyCbor, err := cbor.Encode(map[uint]any{
		26: map[*common.Credential]*DijkstraAccountBalanceInterval{},
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "must not be empty")
}

func TestDijkstraAccountBalanceIntervalMarshalCBORRejectsAllNilBounds(t *testing.T) {
	_, err := DijkstraAccountBalanceInterval{}.MarshalCBOR()
	require.ErrorContains(t, err, "requires a lower or upper bound")
}

// TestDijkstraTransactionBodyOmitsNilAccountBalanceIntervalsField guards
// against a regression: giving DijkstraAccountBalanceIntervals (or
// DijkstraRequiredTopLevelGuards) a custom MarshalCBOR makes fxamacker treat
// it as always non-empty for omitempty purposes, so a nil (field-absent) map
// would stop being omitted and every Dijkstra body would gain a spurious
// empty key 26/24. Both types are documented as deliberately having no
// MarshalCBOR for exactly this reason; this test would fail if one were
// added back without also solving the omitempty problem.
func TestDijkstraTransactionBodyOmitsNilAccountBalanceIntervalsField(t *testing.T) {
	body := DijkstraTransactionBody{TxFee: 1}
	encoded, err := body.MarshalCBOR()
	require.NoError(t, err)
	var fields map[uint]cbor.RawMessage
	_, err = cbor.Decode(encoded, &fields)
	require.NoError(t, err)
	_, present := fields[26]
	require.False(t, present, "key 26 should be omitted when the field is nil")

	subBody := DijkstraSubTransactionBody{Ttl: 1}
	encodedSub, err := subBody.MarshalCBOR()
	require.NoError(t, err)
	var subFields map[uint]cbor.RawMessage
	_, err = cbor.Decode(encodedSub, &subFields)
	require.NoError(t, err)
	_, present = subFields[24]
	require.False(t, present, "key 24 should be omitted when the field is nil")
	_, present = subFields[26]
	require.False(t, present, "key 26 should be omitted when the field is nil")
}

func TestDijkstraTransactionBodyRequiredTopLevelGuards(t *testing.T) {
	guard := testGuardCredential()
	body := DijkstraTransactionBody{
		TxRequiredTopLevelGuards: DijkstraRequiredTopLevelGuards{&guard: nil},
	}
	encoded, err := body.MarshalCBOR()
	require.NoError(t, err)
	var fields map[uint]cbor.RawMessage
	_, err = cbor.Decode(encoded, &fields)
	require.NoError(t, err)
	require.Contains(t, fields, uint(24))

	decoded := DijkstraTransactionBody{}
	require.NoError(t, decoded.UnmarshalCBOR(encoded))
	require.Len(t, decoded.TxRequiredTopLevelGuards, 1)
	for _, datum := range decoded.TxRequiredTopLevelGuards {
		require.Nil(t, datum)
	}

	empty, err := cbor.Encode(map[uint]any{
		24: map[*common.Credential]*common.Datum{},
	})
	require.NoError(t, err)
	var emptyBody DijkstraTransactionBody
	require.ErrorContains(t, emptyBody.UnmarshalCBOR(empty), "must not be empty")

	malformed := DijkstraTransactionBody{
		TxRequiredTopLevelGuards: DijkstraRequiredTopLevelGuards{&guard: {}},
	}
	_, err = malformed.MarshalCBOR()
	require.ErrorContains(t, err, "missing Plutus data")
}

// TestDijkstraBodyMarshalCBORRejectsMalformedDirectlyConstructedMaps proves
// the containing body's MarshalCBOR validates a directly constructed
// DijkstraAccountBalanceIntervals/DijkstraRequiredTopLevelGuards before wire
// encoding, without breaking omitempty for a nil (field-absent) map. Decoding
// from CBOR can never produce these malformed states (UnmarshalCBOR already
// rejects them); this covers a caller that builds one of these exported map
// types directly and encodes it without ever decoding first.
func TestDijkstraBodyMarshalCBORRejectsMalformedDirectlyConstructedMaps(t *testing.T) {
	t.Run("transaction body: all-nil-bounds interval", func(t *testing.T) {
		guard := dijkstraRewardAddressForCredential(t, testGuardCredential())
		body := DijkstraTransactionBody{
			TxBalanceIntervals: DijkstraAccountBalanceIntervals{
				dijkstraIntervalKey(t, guard): {},
			},
		}
		_, err := body.MarshalCBOR()
		require.ErrorContains(t, err, "requires a lower or upper bound")
	})

	t.Run("sub-transaction body: nil interval", func(t *testing.T) {
		guard := dijkstraRewardAddressForCredential(t, testGuardCredential())
		body := DijkstraSubTransactionBody{
			TxAccountBalanceIntervals: DijkstraAccountBalanceIntervals{
				dijkstraIntervalKey(t, guard): nil,
			},
		}
		_, err := body.MarshalCBOR()
		require.ErrorContains(t, err, "must not be nil")
	})

	t.Run("sub-transaction body: required guard datum missing Plutus data", func(t *testing.T) {
		guard := testGuardCredential()
		body := DijkstraSubTransactionBody{
			TxRequiredTopLevelGuards: DijkstraRequiredTopLevelGuards{
				&guard: {},
			},
		}
		_, err := body.MarshalCBOR()
		require.ErrorContains(t, err, "missing Plutus data")
	})
}

func TestDijkstraTransactionBodyBalanceIntervalsRequireRewardAccountKeys(t *testing.T) {
	var hash common.Blake2b224
	hash[0] = 1
	cred := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: hash,
	}
	lower := uint64(5)
	bodyCbor, err := cbor.Encode(map[uint]any{
		26: map[*common.Credential]*DijkstraAccountBalanceInterval{
			&cred: {LowerBound: &lower},
		},
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "cannot unmarshal array")
}

func TestDijkstraSubTransactionBodyAccountBalanceIntervalsRejectsNilInterval(
	t *testing.T,
) {
	var hash common.Blake2b224
	hash[0] = 1
	cred := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: hash,
	}
	address := dijkstraRewardAddressForCredential(t, cred)
	bodyCbor, err := cbor.Encode(map[uint]any{
		26: map[cbor.ByteString]any{dijkstraIntervalKey(t, address): nil},
	})
	require.NoError(t, err)

	var body DijkstraSubTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "must not contain a nil interval")
}

func TestDijkstraAccountStartingBalanceIntervalsWireField(t *testing.T) {
	address := dijkstraRewardAddressForCredential(t, testGuardCredential())
	interval := DijkstraAccountBalanceIntervals{
		dijkstraIntervalKey(t, address): dijkstraIntervalExact(5),
	}
	body := DijkstraTransactionBody{TxStartingBalanceIntervals: interval}
	encoded, err := body.MarshalCBOR()
	require.NoError(t, err)
	var fields map[uint]cbor.RawMessage
	_, err = cbor.Decode(encoded, &fields)
	require.NoError(t, err)
	require.Contains(t, fields, uint(27))

	emptyDeposits, err := cbor.Encode(map[uint]any{
		25: map[cbor.ByteString]uint64{},
	})
	require.NoError(t, err)
	var decoded DijkstraTransactionBody
	require.ErrorContains(t, decoded.UnmarshalCBOR(emptyDeposits), "must not be empty")

	subWithStartingIntervals, err := cbor.Encode(map[uint]any{
		0:  []any{},
		1:  []any{},
		2:  uint64(0),
		27: map[cbor.ByteString]any{dijkstraIntervalKey(t, address): map[uint]uint{0: 5}},
	})
	require.NoError(t, err)
	var subBody DijkstraSubTransactionBody
	require.ErrorContains(
		t,
		subBody.UnmarshalCBOR(subWithStartingIntervals),
		"unknown field",
	)
}

func TestDijkstraSubTransactionBodyRequiredTopLevelGuardsRejectsEmptyMap(
	t *testing.T,
) {
	bodyCbor, err := cbor.Encode(map[uint]any{
		24: map[*common.Credential]*common.Datum{},
	})
	require.NoError(t, err)

	var body DijkstraSubTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "must not be empty")
}

func TestDijkstraSubTransactionBodyRequiredTopLevelGuardsRejectsDuplicateCredential(
	t *testing.T,
) {
	var hash common.Blake2b224
	hash[0] = 1
	cred1 := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: hash,
	}
	cred2 := cred1
	bodyCbor, err := cbor.Encode(map[uint]any{
		24: map[*common.Credential]*common.Datum{
			&cred1: nil,
			&cred2: nil,
		},
	})
	require.NoError(t, err)

	var body DijkstraSubTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "contains a duplicate credential")
}

func TestDijkstraWitnessSetRejectsDuplicateUntaggedVkeyWitness(t *testing.T) {
	dupCbor := []byte{
		0xa1, // map(1)
		0x00, // key: 0  (VkeyWitnesses field)
		// plain array — no tag 258
		0x82,                         // array(2)
		0x82, 0x41, 0x01, 0x41, 0x02, // VkeyWitness{[0x01], [0x02]}
		0x82, 0x41, 0x01, 0x41, 0x02, // duplicate
	}
	var ws DijkstraTransactionWitnessSet
	require.ErrorContains(
		t,
		ws.UnmarshalCBOR(dupCbor),
		"duplicate member in set",
	)
}

func TestDijkstraBlockDecodesRedeemerWitnessMap(t *testing.T) {
	expectedRedeemerData := data.NewInteger(big.NewInt(42))
	blockBody := DijkstraBlockBody{
		Transactions: []DijkstraTransaction{
			{
				Body:      DijkstraTransactionBody{TxFee: 1},
				TxIsValid: true,
				WitnessSet: DijkstraTransactionWitnessSet{
					WsRedeemers: DijkstraRedeemers{
						Redeemers: map[common.RedeemerKey]common.RedeemerValue{
							{Tag: common.RedeemerTagGuarding, Index: 0}: {
								Data: common.Datum{
									Data: expectedRedeemerData,
								},
								ExUnits: common.ExUnits{
									Memory: 11,
									Steps:  22,
								},
							},
						},
					},
				},
			},
		},
		InvalidTransactions: []uint{},
	}
	block := DijkstraBlock{
		BlockHeader: &DijkstraBlockHeader{
			BabbageBlockHeader: babbage.BabbageBlockHeader{
				Body: babbage.BabbageBlockHeaderBody{
					BlockBodyHash: blockBody.Hash(),
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
		BlockBody: blockBody,
	}

	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	decoded, err := NewDijkstraBlockFromCbor(blockCbor)
	require.NoError(t, err)
	require.Len(t, decoded.BlockBody.Transactions, 1)

	redeemers := decoded.BlockBody.Transactions[0].WitnessSet.WsRedeemers
	require.Equal(t, 1, redeemers.Len())
	redeemer := redeemers.Value(0, common.RedeemerTagGuarding)
	require.NotNil(t, redeemer.Data.Data)
	require.True(
		t,
		expectedRedeemerData.Equal(redeemer.Data.Data),
		"redeemer data mismatch: got %s, want %s",
		redeemer.Data.Data,
		expectedRedeemerData,
	)
	require.Equal(t, int64(11), redeemer.ExUnits.Memory)
	require.Equal(t, int64(22), redeemer.ExUnits.Steps)
	require.Equal(t, blockBody.Hash(), decoded.BlockBodyHash())
}

func TestIsCborNullOnlyAcceptsEncodedNull(t *testing.T) {
	require.True(t, isCborNull(cbor.RawMessage{0xf6}))
	require.False(t, isCborNull(nil))
	require.False(t, isCborNull(cbor.RawMessage{}))
}

func TestDijkstraProtocolParametersRoundTrip(t *testing.T) {
	rat := func(num, denom int64) *cbor.Rat {
		return &cbor.Rat{Rat: big.NewRat(num, denom)}
	}
	ratValue := func(num, denom int64) cbor.Rat {
		return cbor.Rat{Rat: big.NewRat(num, denom)}
	}
	pparams := DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			MinFeeA:    44,
			MaxTxSize:  16384,
			A0:         rat(1, 2),
			Rho:        rat(3, 1000),
			Tau:        rat(1, 5),
			CostModels: map[uint][]int64{3: {1, 2, 3}},
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: 12,
			},
			ExecutionCosts: common.ExUnitPrice{
				MemPrice:  rat(1, 10),
				StepPrice: rat(2, 10),
			},
			MaxTxExUnits:        common.ExUnits{Memory: 1, Steps: 2},
			MaxBlockExUnits:     common.ExUnits{Memory: 3, Steps: 4},
			MaxCollateralInputs: 3,
			PoolVotingThresholds: conway.PoolVotingThresholds{
				MotionNoConfidence:    ratValue(1, 2),
				CommitteeNormal:       ratValue(1, 2),
				CommitteeNoConfidence: ratValue(1, 2),
				HardForkInitiation:    ratValue(1, 2),
				PpSecurityGroup:       ratValue(1, 2),
			},
			DRepVotingThresholds: conway.DRepVotingThresholds{
				MotionNoConfidence:    ratValue(1, 2),
				CommitteeNormal:       ratValue(1, 2),
				CommitteeNoConfidence: ratValue(1, 2),
				UpdateToConstitution:  ratValue(1, 2),
				HardForkInitiation:    ratValue(1, 2),
				PpNetworkGroup:        ratValue(1, 2),
				PpEconomicGroup:       ratValue(1, 2),
				PpTechnicalGroup:      ratValue(1, 2),
				PpGovGroup:            ratValue(1, 2),
				TreasuryWithdrawal:    ratValue(1, 2),
			},
			MinFeeRefScriptCostPerByte: rat(15, 1000),
		},
		MaxRefScriptSizePerBlock: 1000,
		MaxRefScriptSizePerTx:    2000,
		RefScriptCostStride:      16,
		RefScriptCostMultiplier:  rat(2, 1),
	}

	pparamsCbor, err := cbor.Encode(pparams)
	require.NoError(t, err)

	var decoded DijkstraProtocolParameters
	_, err = cbor.Decode(pparamsCbor, &decoded)
	require.NoError(t, err)
	require.Equal(t, uint(44), decoded.MinFeeA)
	require.Equal(t, uint(16384), decoded.MaxTxSize)
	require.Equal(t, uint(3), decoded.MaxCollateralInputs)
	require.Equal(t, uint32(1000), decoded.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(2000), decoded.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(16), decoded.RefScriptCostStride)
	require.Equal(t, 0, decoded.RefScriptCostMultiplier.Cmp(big.NewRat(2, 1)))
}

func TestDijkstraProtocolParametersDecodesLegacyArray(t *testing.T) {
	rat := func(num, denom int64) *cbor.Rat {
		return &cbor.Rat{Rat: big.NewRat(num, denom)}
	}
	ratValue := func(num, denom int64) cbor.Rat {
		return cbor.Rat{Rat: big.NewRat(num, denom)}
	}
	params := DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			MinFeeA:   44,
			MaxTxSize: 16384,
			A0:        rat(1, 2),
			Rho:       rat(3, 1000),
			Tau:       rat(1, 5),
			ExecutionCosts: common.ExUnitPrice{
				MemPrice:  rat(1, 10),
				StepPrice: rat(2, 10),
			},
			MaxTxExUnits: common.ExUnits{Memory: 1, Steps: 2},
			MaxBlockExUnits: common.ExUnits{
				Memory: 100,
				Steps:  200,
			},
			PoolVotingThresholds: conway.PoolVotingThresholds{
				MotionNoConfidence:    ratValue(1, 2),
				CommitteeNormal:       ratValue(1, 2),
				CommitteeNoConfidence: ratValue(1, 2),
				HardForkInitiation:    ratValue(1, 2),
				PpSecurityGroup:       ratValue(1, 2),
			},
			DRepVotingThresholds: conway.DRepVotingThresholds{
				MotionNoConfidence:    ratValue(1, 2),
				CommitteeNormal:       ratValue(1, 2),
				CommitteeNoConfidence: ratValue(1, 2),
				UpdateToConstitution:  ratValue(1, 2),
				HardForkInitiation:    ratValue(1, 2),
				PpNetworkGroup:        ratValue(1, 2),
				PpEconomicGroup:       ratValue(1, 2),
				PpTechnicalGroup:      ratValue(1, 2),
				PpGovGroup:            ratValue(1, 2),
				TreasuryWithdrawal:    ratValue(1, 2),
			},
			MinFeeRefScriptCostPerByte: rat(15, 1000),
		},
		MaxRefScriptSizePerBlock: 1000,
		MaxRefScriptSizePerTx:    2000,
		RefScriptCostStride:      3000,
		RefScriptCostMultiplier:  rat(2, 1),
	}
	full, err := cbor.Encode(params.toCbor())
	require.NoError(t, err)
	var fields []cbor.RawMessage
	_, err = cbor.Decode(full, &fields)
	require.NoError(t, err)
	require.Len(t, fields, 46)
	legacy, err := cbor.Encode(fields[:35])
	require.NoError(t, err)

	var decoded DijkstraProtocolParameters
	require.NoError(t, decoded.UnmarshalCBOR(legacy))
	require.Equal(t, uint(44), decoded.MinFeeA)
	require.Equal(t, uint(16384), decoded.MaxTxSize)
	require.Equal(t, uint32(1000), decoded.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(2000), decoded.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(3000), decoded.RefScriptCostStride)
	require.Zero(t, decoded.MaxPledgeLeverage)
	require.Zero(t, decoded.LeiosAnnouncementPeriodLength)
}

func TestDijkstraProtocolParametersRejectsUnsupportedArrayLength(t *testing.T) {
	data, err := cbor.Encode(make([]any, 34))
	require.NoError(t, err)
	var decoded DijkstraProtocolParameters
	require.Error(t, decoded.UnmarshalCBOR(data))
}

func TestDijkstraProtocolParametersRejectsOversizedArrayHeader(t *testing.T) {
	// The arity guard must reject an array whose declared length cannot be
	// represented before attempting to decode all of its elements.
	data := []byte{0x9b, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	var decoded DijkstraProtocolParameters
	require.Error(t, decoded.UnmarshalCBOR(data))
}

func TestDijkstraProtocolParametersUpdateNil(t *testing.T) {
	pparams := DijkstraProtocolParameters{
		MaxRefScriptSizePerBlock: 1000,
	}

	require.NotPanics(t, func() {
		pparams.Update(nil)
	})
	require.Equal(t, uint32(1000), pparams.MaxRefScriptSizePerBlock)
}

func TestDijkstraProtocolParameterUpdateDecodesConwayAndDijkstraFields(
	t *testing.T,
) {
	updateCbor, err := cbor.Encode(map[uint]any{
		0:  uint(44),
		34: uint32(1000),
		35: uint32(2000),
		36: uint32(16),
		37: cbor.Rat{Rat: big.NewRat(2, 1)},
	})
	require.NoError(t, err)

	var update DijkstraProtocolParameterUpdate
	_, err = cbor.Decode(updateCbor, &update)
	require.NoError(t, err)
	require.Equal(t, updateCbor, update.Cbor())
	require.NotNil(t, update.MinFeeA)
	require.Equal(t, uint(44), *update.MinFeeA)
	require.NotNil(t, update.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(1000), *update.MaxRefScriptSizePerBlock)
	require.NotNil(t, update.RefScriptCostMultiplier)
	require.Equal(t, 0, update.RefScriptCostMultiplier.Cmp(big.NewRat(2, 1)))

	var pparams DijkstraProtocolParameters
	pparams.Update(&update)
	require.Equal(t, uint(44), pparams.MinFeeA)
	require.Equal(t, uint32(1000), pparams.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(2000), pparams.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(16), pparams.RefScriptCostStride)
	require.Equal(t, 0, pparams.RefScriptCostMultiplier.Cmp(big.NewRat(2, 1)))
}

func TestDijkstraProtocolParameterUpdateCostModelLanguageIDDomain(t *testing.T) {
	for _, tc := range []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{name: "unknown Word8 ID remains valid", id: 255},
		{name: "out-of-domain ID rejected", id: 256, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := cbor.Encode(map[int]any{
				18: map[uint][]int64{tc.id: {1}},
			})
			require.NoError(t, err)
			var update DijkstraProtocolParameterUpdate
			err = update.UnmarshalCBOR(encoded)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Contains(t, update.CostModels, tc.id)
			}
		})
	}
	var params DijkstraProtocolParameters
	err := params.ApplyUpdate(&DijkstraProtocolParameterUpdate{
		CostModels: map[uint][]int64{256: {1}},
	})
	require.Error(t, err)
	require.Empty(t, params.CostModels)
}

func TestProtocolParameterUpdateFixedWidthIntegerDomains(t *testing.T) {
	tests := []struct {
		name string
		tag  int
		max  uint64
	}{
		{"maxBlockBodySize", 2, math.MaxUint32},
		{"maxTxSize", 3, math.MaxUint32},
		{"maxBlockHeaderSize", 4, math.MaxUint16},
		{"maxEpoch", 7, math.MaxUint32},
		{"nOpt", 8, math.MaxUint16},
		{"maxValueSize", 22, math.MaxUint32},
		{"collateralPercentage", 23, math.MaxUint16},
		{"maxCollateralInputs", 24, math.MaxUint16},
		{"minCommitteeSize", 27, math.MaxUint16},
		{"committeeTermLimit", 28, math.MaxUint32},
		{"govActionValidityPeriod", 29, math.MaxUint32},
		{"dRepInactivityPeriod", 32, math.MaxUint32},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, tcValue := range []struct {
				name    string
				value   uint64
				wantErr bool
			}{
				{name: "maximum", value: tc.max},
				{name: "maximum plus one", value: tc.max + 1, wantErr: true},
			} {
				t.Run(tcValue.name, func(t *testing.T) {
					encoded, err := cbor.Encode(map[int]any{tc.tag: tcValue.value})
					require.NoError(t, err)
					var conwayUpdate conway.ConwayProtocolParameterUpdate
					err = conwayUpdate.UnmarshalCBOR(encoded)
					if tcValue.wantErr {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
					var dijkstraUpdate DijkstraProtocolParameterUpdate
					err = dijkstraUpdate.UnmarshalCBOR(encoded)
					if tcValue.wantErr {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
				})
			}
		})
	}
	if strconv.IntSize == 64 {
		tooLarge := uint(math.MaxUint32)
		tooLarge++
		conwayParams := conway.ConwayProtocolParameters{}
		require.Error(t, conwayParams.ApplyUpdate(&conway.ConwayProtocolParameterUpdate{
			MaxBlockBodySize: &tooLarge,
		}))
		dijkstraParams := DijkstraProtocolParameters{}
		require.Error(t, dijkstraParams.ApplyUpdate(&DijkstraProtocolParameterUpdate{
			MaxBlockBodySize: &tooLarge,
		}))
		require.Zero(t, conwayParams.MaxBlockBodySize)
		require.Zero(t, dijkstraParams.MaxBlockBodySize)
	}
}

func TestDijkstraProtocolParameterUpdateDijkstraFieldWidths(t *testing.T) {
	fields := []struct {
		name string
		tag  int
		max  uint64
	}{
		{"maxRefScriptSizePerBlock", 34, math.MaxUint32},
		{"maxRefScriptSizePerTx", 35, math.MaxUint32},
		{"refScriptCostStride", 36, math.MaxUint32},
		{"leiosAnnouncementPeriodLength", 40, math.MaxUint32},
		{"leiosVotePeriodLength", 41, math.MaxUint32},
		{"leiosDiffusionPeriodLength", 42, math.MaxUint32},
		{"leiosCommitteeSize", 43, math.MaxUint16},
		{"maxEndorserBlockReferencesSize", 45, math.MaxUint32},
		{"maxEndorserBlockTxsSize", 46, math.MaxUint32},
		{"maxRefScriptSizePerEndorserBlock", 48, math.MaxUint32},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			for _, value := range []struct {
				name string
				v    uint64
				bad  bool
			}{
				{name: "maximum", v: field.max},
				{name: "maximum plus one", v: field.max + 1, bad: true},
			} {
				t.Run(value.name, func(t *testing.T) {
					encoded, err := cbor.Encode(map[int]any{field.tag: value.v})
					require.NoError(t, err)
					var update DijkstraProtocolParameterUpdate
					err = update.UnmarshalCBOR(encoded)
					if value.bad {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
				})
			}
		})
	}
}

func TestDijkstraMaxPledgeLeverageNullUpdate(t *testing.T) {
	encoded, err := cbor.Encode(map[int]any{38: nil})
	require.NoError(t, err)
	var update DijkstraProtocolParameterUpdate
	require.NoError(t, update.UnmarshalCBOR(encoded))
	require.True(t, update.MaxPledgeLeverageSet)
	require.Nil(t, update.MaxPledgeLeverage)
	require.True(t, update.hasUpdate())
	require.NoError(t, validateDijkstraProtocolParameterUpdate(&update))

	update.SetCbor(nil)
	roundTrip, err := update.MarshalCBOR()
	require.NoError(t, err)
	var decoded DijkstraProtocolParameterUpdate
	require.NoError(t, decoded.UnmarshalCBOR(roundTrip))
	require.True(t, decoded.MaxPledgeLeverageSet)
	require.Nil(t, decoded.MaxPledgeLeverage)

	params := DijkstraProtocolParameters{
		MaxPledgeLeverage: &cbor.Rat{Rat: big.NewRat(3, 2)},
	}
	require.NoError(t, params.ApplyUpdate(&update))
	require.Nil(t, params.MaxPledgeLeverage)

	got := update.ToPlutusData()
	want := data.NewMap([][2]data.PlutusData{{
		data.NewInteger(big.NewInt(38)),
		data.NewConstr(1),
	}})
	require.True(t, got.Equal(want))
	action := DijkstraParameterChangeGovAction{
		ParamUpdate: update,
	}
	wantAction := data.NewConstr(
		0,
		data.NewConstr(1),
		want,
		data.NewConstr(1),
	)
	require.True(t, action.ToPlutusData().Equal(wantAction))
}

func TestDijkstraProtocolParameterUpdateRejectsNullForNonNullableFields(t *testing.T) {
	tags := []int{0, 1, 5, 6, 14, 16, 17, 18, 20, 21, 25, 26, 30, 31}
	for tag := 34; tag <= 48; tag++ {
		if tag != 38 {
			tags = append(tags, tag)
		}
	}
	for _, tag := range tags {
		t.Run(fmt.Sprintf("tag_%d", tag), func(t *testing.T) {
			encoded, err := cbor.Encode(map[int]any{0: uint(1), tag: nil})
			require.NoError(t, err)
			var update DijkstraProtocolParameterUpdate
			require.Error(t, update.UnmarshalCBOR(encoded))
		})
	}
}

func TestDijkstraProtocolParameterUpdateDomains(t *testing.T) {
	ratio := func(numerator, denominator int64) *cbor.Rat {
		return &cbor.Rat{Rat: big.NewRat(numerator, denominator)}
	}
	tests := []struct {
		name  string
		field map[int]any
		valid bool
	}{
		{name: "positive stride multiplier", field: map[int]any{37: ratio(1, 1)}, valid: true},
		{name: "positive leverage update", field: map[int]any{38: ratio(0, 1)}, valid: true},
		{name: "negative leverage update", field: map[int]any{38: ratio(-1, 1)}},
		{name: "zero stride multiplier", field: map[int]any{37: ratio(0, 1)}},
		{name: "negative stride multiplier", field: map[int]any{37: ratio(-1, 1)}},
		{name: "min pool margin zero", field: map[int]any{39: ratio(0, 1)}, valid: true},
		{name: "min pool margin one", field: map[int]any{39: ratio(1, 1)}, valid: true},
		{name: "min pool margin below zero", field: map[int]any{39: ratio(-1, 1)}},
		{name: "min pool margin above one", field: map[int]any{39: ratio(2, 1)}},
		{name: "quorum threshold zero", field: map[int]any{44: ratio(0, 1)}, valid: true},
		{name: "quorum threshold one", field: map[int]any{44: ratio(1, 1)}, valid: true},
		{name: "quorum threshold below zero", field: map[int]any{44: ratio(-1, 1)}},
		{name: "quorum threshold above one", field: map[int]any{44: ratio(2, 1)}},
		{name: "negative endorser memory", field: map[int]any{47: []int64{-1, 0}}},
		{name: "negative endorser steps", field: map[int]any{47: []int64{0, -1}}},
		{name: "zero endorser ex-units", field: map[int]any{47: []int64{0, 0}}, valid: true},
		{name: "maximum endorser ex-units", field: map[int]any{47: []int64{math.MaxInt64, math.MaxInt64}}, valid: true},
		{name: "inherited negative a0", field: map[int]any{9: ratio(-1, 1)}},
		{name: "inherited rho above unit interval", field: map[int]any{10: ratio(2, 1)}},
		{name: "inherited tau below unit interval", field: map[int]any{11: ratio(-1, 1)}},
		{name: "inherited negative execution memory price", field: map[int]any{19: []any{ratio(-1, 1), ratio(1, 1)}}},
		{name: "inherited negative execution step price", field: map[int]any{19: []any{ratio(1, 1), ratio(-1, 1)}}},
		{name: "inherited null execution price", field: map[int]any{0: uint(1), 19: []any{nil, ratio(1, 1)}}},
		{name: "inherited negative tx ex-units", field: map[int]any{20: []int64{-1, 0}}},
		{name: "inherited negative block ex-units", field: map[int]any{21: []int64{0, -1}}},
		{name: "inherited pool threshold above unit interval", field: map[int]any{25: conway.PoolVotingThresholds{
			MotionNoConfidence:    cbor.Rat{Rat: big.NewRat(1, 2)},
			CommitteeNormal:       cbor.Rat{Rat: big.NewRat(1, 2)},
			CommitteeNoConfidence: cbor.Rat{Rat: big.NewRat(1, 2)},
			HardForkInitiation:    cbor.Rat{Rat: big.NewRat(1, 2)},
			PpSecurityGroup:       cbor.Rat{Rat: big.NewRat(2, 1)},
		}}},
		{name: "inherited DRep threshold below unit interval", field: map[int]any{26: conway.DRepVotingThresholds{
			MotionNoConfidence:    cbor.Rat{Rat: big.NewRat(1, 2)},
			CommitteeNormal:       cbor.Rat{Rat: big.NewRat(1, 2)},
			CommitteeNoConfidence: cbor.Rat{Rat: big.NewRat(1, 2)},
			UpdateToConstitution:  cbor.Rat{Rat: big.NewRat(1, 2)},
			HardForkInitiation:    cbor.Rat{Rat: big.NewRat(1, 2)},
			PpNetworkGroup:        cbor.Rat{Rat: big.NewRat(1, 2)},
			PpEconomicGroup:       cbor.Rat{Rat: big.NewRat(1, 2)},
			PpTechnicalGroup:      cbor.Rat{Rat: big.NewRat(1, 2)},
			PpGovGroup:            cbor.Rat{Rat: big.NewRat(1, 2)},
			TreasuryWithdrawal:    cbor.Rat{Rat: big.NewRat(-1, 2)},
		}}},
		{name: "inherited negative ref-script fee", field: map[int]any{33: ratio(-1, 1)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := cbor.Encode(tc.field)
			require.NoError(t, err)
			var update DijkstraProtocolParameterUpdate
			err = update.UnmarshalCBOR(encoded)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestDijkstraProtocolParameterUpdateRejectsOutOfRangeRatios(t *testing.T) {
	tooWide := new(big.Int).Lsh(big.NewInt(1), 64)
	update := DijkstraProtocolParameterUpdate{
		RefScriptCostMultiplier: &cbor.Rat{Rat: new(big.Rat).SetInt(tooWide)},
	}
	require.Error(t, validateDijkstraProtocolParameterUpdateDomains(&update))

	max := new(big.Int).SetUint64(math.MaxUint64)
	valid := DijkstraProtocolParameterUpdate{
		RefScriptCostMultiplier: &cbor.Rat{Rat: new(big.Rat).SetFrac(max, big.NewInt(1))},
	}
	require.NoError(t, validateDijkstraProtocolParameterUpdateDomains(&valid))

	invalidWireValues := map[string][]byte{
		// Both raw components are 2^64, which cbor.Rat reduces to 1/1.
		"oversized components that reduce into range": {
			0xa1, 0x18, 0x27, 0xd8, 0x1e, 0x82,
			0xc2, 0x49, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0xc2, 0x49, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		},
		"negative numerator and denominator that reduce into range": {
			0xa1, 0x18, 0x27, 0xd8, 0x1e, 0x82, 0x20, 0x20,
		},
		"negative denominator": {
			0xa1, 0x18, 0x27, 0xd8, 0x1e, 0x82, 0x01, 0x20,
		},
	}
	for name, raw := range invalidWireValues {
		t.Run(name, func(t *testing.T) {
			var decoded DijkstraProtocolParameterUpdate
			require.Error(t, decoded.UnmarshalCBOR(raw))
		})
	}
	maxBoundedComponents := []byte{
		0xa1, 0x18, 0x27, 0xd8, 0x1e, 0x82,
		0x1b, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0x1b, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	}
	var maxBounded DijkstraProtocolParameterUpdate
	require.NoError(t, maxBounded.UnmarshalCBOR(maxBoundedComponents))
}

func TestDijkstraProtocolParameterUpdateNullEncodingChangedParameters(t *testing.T) {
	var absent DijkstraProtocolParameterUpdate
	absentWire, err := absent.MarshalCBOR()
	require.NoError(t, err)
	var absentRoundTrip DijkstraProtocolParameterUpdate
	require.NoError(t, absentRoundTrip.UnmarshalCBOR(absentWire))
	require.False(t, absentRoundTrip.MaxPledgeLeverageSet)
	require.False(t, absentRoundTrip.hasUpdate())

	update := DijkstraProtocolParameterUpdate{MaxPledgeLeverageSet: true}
	encoded, err := update.MarshalCBOR()
	require.NoError(t, err)
	var decoded DijkstraProtocolParameterUpdate
	require.NoError(t, decoded.UnmarshalCBOR(encoded))
	require.True(t, decoded.MaxPledgeLeverageSet)
	require.Nil(t, decoded.MaxPledgeLeverage)

	got := update.ToPlutusData()
	want := data.NewMap([][2]data.PlutusData{{
		data.NewInteger(big.NewInt(38)),
		data.NewConstr(1),
	}})
	require.True(t, got.Equal(want))

	ratioUpdate := DijkstraProtocolParameterUpdate{
		MaxPledgeLeverageSet: true,
		MaxPledgeLeverage:    &cbor.Rat{Rat: big.NewRat(3, 2)},
	}
	got = ratioUpdate.ToPlutusData()
	want = data.NewMap([][2]data.PlutusData{{
		data.NewInteger(big.NewInt(38)),
		data.NewConstr(0, data.NewList(
			data.NewInteger(big.NewInt(3)),
			data.NewInteger(big.NewInt(2)),
		)),
	}})
	require.True(t, got.Equal(want))

	mixedWire, err := cbor.Encode(map[int]any{0: uint(44), 38: nil})
	require.NoError(t, err)
	var mixed DijkstraProtocolParameterUpdate
	require.NoError(t, mixed.UnmarshalCBOR(mixedWire))
	require.True(t, mixed.MaxPledgeLeverageSet)
	params := DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{MinFeeA: 1},
		MaxPledgeLeverage:        &cbor.Rat{Rat: big.NewRat(3, 2)},
	}
	require.NoError(t, params.ApplyUpdate(&mixed))
	require.Equal(t, uint(44), params.MinFeeA)
	require.Nil(t, params.MaxPledgeLeverage)
}

func TestDijkstraProtocolParameterUpdateDecodesLeiosFields(t *testing.T) {
	quorum := cbor.Rat{Rat: big.NewRat(3, 4)}
	exUnits := common.ExUnits{Memory: 123, Steps: 456}
	updateCbor, err := cbor.Encode(map[uint]any{
		40: uint32(1000),
		41: uint32(2000),
		42: uint32(3000),
		43: uint16(42),
		44: quorum,
		45: uint32(500000),
		46: uint32(12000000),
		47: exUnits,
		48: uint32(1048576),
	})
	require.NoError(t, err)

	var update DijkstraProtocolParameterUpdate
	_, err = cbor.Decode(updateCbor, &update)
	require.NoError(t, err)
	require.Equal(t, updateCbor, update.Cbor())
	require.Equal(t, uint32(1000), *update.LeiosAnnouncementPeriodLength)
	require.Equal(t, uint32(2000), *update.LeiosVotePeriodLength)
	require.Equal(t, uint32(3000), *update.LeiosDiffusionPeriodLength)
	require.Equal(t, uint16(42), *update.LeiosCommitteeSize)
	require.Equal(t, 0, update.LeiosQuorumStakeThreshold.Cmp(quorum.Rat))
	require.Equal(t, uint32(500000), *update.MaxEndorserBlockReferencesSize)
	require.Equal(t, uint32(12000000), *update.MaxEndorserBlockTxsSize)
	require.Equal(t, exUnits, *update.MaxEndorserBlockExUnits)
	require.Equal(t, uint32(1048576), *update.MaxRefScriptSizePerEndorserBlock)

	var params DijkstraProtocolParameters
	params.Update(&update)
	require.Equal(t, uint32(1000), params.LeiosAnnouncementPeriodLength)
	require.Equal(t, uint32(2000), params.LeiosVotePeriodLength)
	require.Equal(t, uint32(3000), params.LeiosDiffusionPeriodLength)
	require.Equal(t, uint16(42), params.LeiosCommitteeSize)
	require.Equal(t, 0, params.LeiosQuorumStakeThreshold.Cmp(quorum.Rat))
	require.Equal(t, uint32(500000), params.MaxEndorserBlockReferencesSize)
	require.Equal(t, uint32(12000000), params.MaxEndorserBlockTxsSize)
	require.Equal(t, exUnits, params.MaxEndorserBlockExUnits)
	require.Equal(t, uint32(1048576), params.MaxRefScriptSizePerEndorserBlock)
}

func TestDijkstraProtocolParameterUpdateEncodesLeiosFields(t *testing.T) {
	announcement, vote, diffusion := uint32(1000), uint32(2000), uint32(3000)
	committee := uint16(42)
	references, txs, refScripts := uint32(500000), uint32(12000000), uint32(1048576)
	exUnits := common.ExUnits{Memory: 123, Steps: 456}
	update := DijkstraProtocolParameterUpdate{
		LeiosAnnouncementPeriodLength:    &announcement,
		LeiosVotePeriodLength:            &vote,
		LeiosDiffusionPeriodLength:       &diffusion,
		LeiosCommitteeSize:               &committee,
		LeiosQuorumStakeThreshold:        &cbor.Rat{Rat: big.NewRat(3, 4)},
		MaxEndorserBlockReferencesSize:   &references,
		MaxEndorserBlockTxsSize:          &txs,
		MaxEndorserBlockExUnits:          &exUnits,
		MaxRefScriptSizePerEndorserBlock: &refScripts,
	}
	encoded, err := cbor.Encode(update)
	require.NoError(t, err)

	var fields map[uint]cbor.RawMessage
	_, err = cbor.Decode(encoded, &fields)
	require.NoError(t, err)
	for _, key := range []uint{40, 41, 42, 43, 44, 45, 46, 47, 48} {
		require.Contains(t, fields, key)
	}
}

func TestDijkstraProtocolParameterUpdateLeiosStakeFieldsExcludedFromCbor(
	t *testing.T,
) {
	updateCbor, err := cbor.Encode(DijkstraProtocolParameterUpdate{
		CommitteeStakeCoverage: &cbor.Rat{Rat: big.NewRat(99, 100)},
		QuorumStakeThreshold:   &cbor.Rat{Rat: big.NewRat(3, 4)},
	})
	require.NoError(t, err)

	var decoded map[uint]cbor.RawMessage
	_, err = cbor.Decode(updateCbor, &decoded)
	require.NoError(t, err)
	require.Empty(t, decoded)
}

func TestDijkstraGenesisDecodesCurrentDevnetExample(t *testing.T) {
	genesis, err := NewDijkstraGenesisFromReader(strings.NewReader(`{
  "maxRefScriptSizePerBlock": 1048576,
  "maxRefScriptSizePerTx": 204800,
  "refScriptCostStride": 25600,
  "refScriptCostMultiplier": 1.2
}`))
	require.NoError(t, err)
	require.Equal(t, uint32(1048576), genesis.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(204800), genesis.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(25600), genesis.RefScriptCostStride)
	require.Equal(t, 0, genesis.RefScriptCostMultiplier.Cmp(big.NewRat(6, 5)))

	var pparams DijkstraProtocolParameters
	require.NoError(t, pparams.UpdateFromGenesis(&genesis))
	require.Equal(t, uint32(1048576), pparams.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(204800), pparams.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(25600), pparams.RefScriptCostStride)
	require.Equal(t, 0, pparams.RefScriptCostMultiplier.Cmp(big.NewRat(6, 5)))
}

func TestDijkstraGenesisLeiosStakeParameters(t *testing.T) {
	genesis, err := NewDijkstraGenesisFromReader(strings.NewReader(`{
  "committeeStakeCoverage": 0.99,
  "quorumStakeThreshold": 0.75
}`))
	require.NoError(t, err)

	var pparams DijkstraProtocolParameters
	require.NoError(t, pparams.UpdateFromGenesis(&genesis))
	require.Equal(
		t,
		0,
		pparams.CommitteeStakeCoverage.Cmp(big.NewRat(99, 100)),
	)
	require.Equal(
		t,
		0,
		pparams.QuorumStakeThreshold.Cmp(big.NewRat(3, 4)),
	)
}

func TestDijkstraGenesisDecodesLeiosProtocolParameters(t *testing.T) {
	genesis, err := NewDijkstraGenesisFromReader(strings.NewReader(`{
  "leiosAnnouncementPeriodLength": 1000,
  "leiosVotePeriodLength": 2000,
  "leiosDiffusionPeriodLength": 3000,
  "leiosCommitteeSize": 42,
  "leiosQuorumStakeThreshold": 0.75,
  "plutusV4CostModel": [4000, 5000, 6000],
  "maxEndorserBlockReferencesSize": 500000,
  "maxEndorserBlockTxsSize": 12000000,
  "maxEndorserBlockExecutionUnits": {"memory": 123, "steps": 456},
  "maxRefScriptSizePerEndorserBlock": 1048576
}`))
	require.NoError(t, err)

	var params DijkstraProtocolParameters
	require.NoError(t, params.UpdateFromGenesis(&genesis))
	require.Equal(t, uint32(1000), params.LeiosAnnouncementPeriodLength)
	require.Equal(t, uint32(2000), params.LeiosVotePeriodLength)
	require.Equal(t, uint32(3000), params.LeiosDiffusionPeriodLength)
	require.Equal(t, uint16(42), params.LeiosCommitteeSize)
	require.Equal(t, 0, params.LeiosQuorumStakeThreshold.Cmp(big.NewRat(3, 4)))
	require.Equal(t, uint32(500000), params.MaxEndorserBlockReferencesSize)
	require.Equal(t, uint32(12000000), params.MaxEndorserBlockTxsSize)
	require.Equal(t, common.ExUnits{Memory: 123, Steps: 456}, params.MaxEndorserBlockExUnits)
	require.Equal(t, uint32(1048576), params.MaxRefScriptSizePerEndorserBlock)
	require.Equal(t, []int64{4000, 5000, 6000}, params.CostModels[3])
}

func TestDijkstraGenesisDefaultsReferenceScriptFeeParameters(t *testing.T) {
	var pparams DijkstraProtocolParameters
	require.NoError(t, pparams.UpdateFromGenesis(&DijkstraGenesis{}))
	require.Equal(
		t,
		uint32(conway.RefScriptCostStride),
		pparams.RefScriptCostStride,
	)
	require.NotNil(t, pparams.RefScriptCostMultiplier)
	require.Equal(
		t,
		0,
		pparams.RefScriptCostMultiplier.Cmp(big.NewRat(6, 5)),
	)
}

func TestDijkstraGenesisRejectsInvalidLeiosStakeParameters(t *testing.T) {
	genesis, err := NewDijkstraGenesisFromReader(strings.NewReader(`{
  "committeeStakeCoverage": 0.75,
  "quorumStakeThreshold": 0.75
}`))
	require.NoError(t, err)

	var pparams DijkstraProtocolParameters
	err = pparams.UpdateFromGenesis(&genesis)
	require.ErrorAs(t, err, &LeiosCommitteeStakeParametersError{})
}

func TestDijkstraLeiosStakeParametersValidateSingleField(t *testing.T) {
	tests := []struct {
		name                   string
		committeeStakeCoverage *cbor.Rat
		quorumStakeThreshold   *cbor.Rat
		expectErr              bool
	}{
		{
			name:                   "valid coverage without quorum",
			committeeStakeCoverage: &cbor.Rat{Rat: big.NewRat(1, 2)},
		},
		{
			name:                 "valid quorum without coverage",
			quorumStakeThreshold: &cbor.Rat{Rat: big.NewRat(1, 2)},
		},
		{
			name:                   "invalid coverage without quorum",
			committeeStakeCoverage: &cbor.Rat{Rat: big.NewRat(0, 1)},
			expectErr:              true,
		},
		{
			name:                 "invalid quorum without coverage",
			quorumStakeThreshold: &cbor.Rat{Rat: big.NewRat(2, 1)},
			expectErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLeiosCommitteeStakeParameters(
				tt.committeeStakeCoverage,
				tt.quorumStakeThreshold,
			)
			if tt.expectErr {
				require.ErrorAs(
					t,
					err,
					&LeiosCommitteeStakeParametersError{},
				)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDijkstraProtocolParametersApplyUpdateLeiosStakeInvariant(
	t *testing.T,
) {
	pparams := DijkstraProtocolParameters{
		CommitteeStakeCoverage: &cbor.Rat{Rat: big.NewRat(99, 100)},
		QuorumStakeThreshold:   &cbor.Rat{Rat: big.NewRat(3, 4)},
	}
	validQuorum := &cbor.Rat{Rat: big.NewRat(4, 5)}
	require.NoError(t, pparams.ApplyUpdate(&DijkstraProtocolParameterUpdate{
		QuorumStakeThreshold: validQuorum,
	}))
	require.Equal(t, 0, pparams.QuorumStakeThreshold.Cmp(big.NewRat(4, 5)))

	invalidCoverage := &cbor.Rat{Rat: big.NewRat(4, 5)}
	err := pparams.ApplyUpdate(&DijkstraProtocolParameterUpdate{
		CommitteeStakeCoverage: invalidCoverage,
	})
	require.ErrorAs(t, err, &LeiosCommitteeStakeParametersError{})
	require.Equal(
		t,
		0,
		pparams.CommitteeStakeCoverage.Cmp(big.NewRat(99, 100)),
	)
}

func TestDijkstraProtocolParametersUpdatePreservesLeiosStakeInvariant(
	t *testing.T,
) {
	pparams := DijkstraProtocolParameters{
		MaxRefScriptSizePerBlock: 1000,
		CommitteeStakeCoverage:   &cbor.Rat{Rat: big.NewRat(99, 100)},
		QuorumStakeThreshold:     &cbor.Rat{Rat: big.NewRat(3, 4)},
	}
	validQuorum := &cbor.Rat{Rat: big.NewRat(4, 5)}
	pparams.Update(&DijkstraProtocolParameterUpdate{
		QuorumStakeThreshold: validQuorum,
	})
	require.Equal(t, 0, pparams.QuorumStakeThreshold.Cmp(big.NewRat(4, 5)))

	invalidCoverage := &cbor.Rat{Rat: big.NewRat(4, 5)}
	maxRefScriptSizePerBlock := uint32(2000)
	pparams.Update(&DijkstraProtocolParameterUpdate{
		MaxRefScriptSizePerBlock: &maxRefScriptSizePerBlock,
		CommitteeStakeCoverage:   invalidCoverage,
	})
	require.Equal(t, uint32(1000), pparams.MaxRefScriptSizePerBlock)
	require.Equal(
		t,
		0,
		pparams.CommitteeStakeCoverage.Cmp(big.NewRat(99, 100)),
	)
	require.Equal(t, 0, pparams.QuorumStakeThreshold.Cmp(big.NewRat(4, 5)))
}

func TestDijkstraProtocolParameterUpdateToPlutusDataCostModels(t *testing.T) {
	maxRefScriptSizePerBlock := uint32(1234)
	maxRefScriptSizePerTx := uint32(4321)
	refScriptCostStride := uint32(64)
	update := DijkstraProtocolParameterUpdate{
		CostModels: map[uint][]int64{
			3: {9, 8},
			1: {7},
		},
		MaxRefScriptSizePerBlock: &maxRefScriptSizePerBlock,
		MaxRefScriptSizePerTx:    &maxRefScriptSizePerTx,
		RefScriptCostStride:      &refScriptCostStride,
		RefScriptCostMultiplier:  &cbor.Rat{Rat: big.NewRat(5, 2)},
	}
	expected := data.NewMap([][2]data.PlutusData{
		{
			testPlutusInteger(18),
			data.NewMap([][2]data.PlutusData{
				{
					testPlutusInteger(1),
					data.NewList(testPlutusInteger(7)),
				},
				{
					testPlutusInteger(3),
					data.NewList(
						testPlutusInteger(9),
						testPlutusInteger(8),
					),
				},
			}),
		},
		{testPlutusInteger(34), testPlutusInteger(1234)},
		{testPlutusInteger(35), testPlutusInteger(4321)},
		{testPlutusInteger(36), testPlutusInteger(64)},
		{
			testPlutusInteger(37),
			data.NewList(testPlutusInteger(5), testPlutusInteger(2)),
		},
	})

	result := update.ToPlutusData()
	require.True(t, expected.Equal(result), "got %#v", result)
}

func TestDijkstraParameterChangeGovActionDecodesDijkstraUpdateFields(
	t *testing.T,
) {
	maxRefScriptSizePerBlock := uint32(1000)
	action := &DijkstraParameterChangeGovAction{
		Type: uint(common.GovActionTypeParameterChange),
		ParamUpdate: DijkstraProtocolParameterUpdate{
			MaxRefScriptSizePerBlock: &maxRefScriptSizePerBlock,
		},
	}
	actionCbor, err := cbor.Encode(&DijkstraGovAction{Action: action})
	require.NoError(t, err)

	var decoded DijkstraGovAction
	_, err = cbor.Decode(actionCbor, &decoded)
	require.NoError(t, err)
	decodedAction, ok := decoded.Action.(*DijkstraParameterChangeGovAction)
	require.True(t, ok)
	require.NotNil(t, decodedAction.ParamUpdate.MaxRefScriptSizePerBlock)
	require.Equal(
		t,
		maxRefScriptSizePerBlock,
		*decodedAction.ParamUpdate.MaxRefScriptSizePerBlock,
	)
}

func TestDijkstraWitnessSetRejectsEmptyCollectionsAndUnsupportedField8(
	t *testing.T,
) {
	for _, data := range [][]byte{
		{0xa1, 0x00, 0x80},
		{0xa1, 0x08, 0x80},
	} {
		var witnesses DijkstraTransactionWitnessSet
		require.Error(t, witnesses.UnmarshalCBOR(data))
	}

	unsupported, err := cbor.Encode(map[uint]any{8: []any{[]byte{0x01}}})
	require.NoError(t, err)
	var witnesses DijkstraTransactionWitnessSet
	require.ErrorContains(
		t,
		witnesses.UnmarshalCBOR(unsupported),
		"does not support field 8",
	)
}

func TestDijkstraWitnessSetMarshalRejectsField8(t *testing.T) {
	witnesses := DijkstraTransactionWitnessSet{
		WsPlutusV4Scripts: cbor.NewSetType(
			[]common.PlutusV4Script{{0x01}},
			false,
		),
	}
	_, err := cbor.Encode(witnesses)
	require.ErrorContains(t, err, "does not support field 8")
}

func TestDijkstraTransactionBodiesRequiredAndGuardedFields(t *testing.T) {
	topRequired := map[uint]any{
		0: cbor.NewSetType([]any{}, false),
		1: []any{},
		2: uint64(0),
	}
	for _, key := range []uint{0, 1, 2} {
		fields := cloneDijkstraBodyFields(topRequired)
		delete(fields, key)
		wire, err := cbor.Encode(fields)
		require.NoError(t, err)
		var body DijkstraTransactionBody
		require.ErrorContains(
			t,
			body.UnmarshalCBOR(wire),
			fmt.Sprintf("field %d is missing", key),
		)
	}
	topGuarded := map[uint]any{
		4:  []any{},
		5:  map[uint]any{},
		13: cbor.NewSetType([]any{}, false),
		18: cbor.NewSetType([]any{}, false),
		20: []any{},
		23: cbor.NewSetType([]any{}, false),
	}
	for key, value := range topGuarded {
		fields := cloneDijkstraBodyFields(topRequired)
		fields[key] = value
		wire, err := cbor.Encode(fields)
		require.NoError(t, err)
		var body DijkstraTransactionBody
		require.Error(t, body.UnmarshalCBOR(wire), "empty key %d", key)
	}
	wire, err := cbor.Encode(topRequired)
	require.NoError(t, err)
	var topBody DijkstraTransactionBody
	require.NoError(t, topBody.UnmarshalCBOR(wire))

	subRequired := map[uint]any{
		0: cbor.NewSetType([]any{}, false),
		1: []any{},
	}
	for _, key := range []uint{0, 1} {
		fields := cloneDijkstraBodyFields(subRequired)
		delete(fields, key)
		wire, err := cbor.Encode(fields)
		require.NoError(t, err)
		var body DijkstraSubTransactionBody
		require.ErrorContains(
			t,
			body.UnmarshalCBOR(wire),
			fmt.Sprintf("field %d is missing", key),
		)
	}
	subGuarded := map[uint]any{
		4:  []any{},
		5:  map[uint]any{},
		18: cbor.NewSetType([]any{}, false),
		20: []any{},
	}
	for key, value := range subGuarded {
		fields := cloneDijkstraBodyFields(subRequired)
		fields[key] = value
		wire, err := cbor.Encode(fields)
		require.NoError(t, err)
		var body DijkstraSubTransactionBody
		require.Error(t, body.UnmarshalCBOR(wire), "empty key %d", key)
	}
	wire, err = cbor.Encode(subRequired)
	require.NoError(t, err)
	var subBody DijkstraSubTransactionBody
	require.NoError(t, subBody.UnmarshalCBOR(wire))
}

func cloneDijkstraBodyFields(fields map[uint]any) map[uint]any {
	clone := make(map[uint]any, len(fields))
	for key, value := range fields {
		clone[key] = value
	}
	return clone
}

func TestDijkstraWitnessSetRejectsEveryPresentEmptyField(t *testing.T) {
	var absent DijkstraTransactionWitnessSet
	require.NoError(t, absent.UnmarshalCBOR([]byte{0xa0}))
	for key := uint(0); key <= 7; key++ {
		values := []any{[]any{}}
		if key == 0 || key == 1 || key == 2 || key == 3 || key == 4 ||
			key == 6 || key == 7 {
			values = append(values, cbor.Set{})
		}
		if key == 5 {
			values = append(values, map[uint]any{})
		}
		for _, value := range values {
			wire, err := cbor.Encode(map[uint]any{key: value})
			require.NoError(t, err)
			var witnesses DijkstraTransactionWitnessSet
			require.Error(t, witnesses.UnmarshalCBOR(wire), "empty key %d", key)
		}
	}
	legacyEmptyRedeemer, err := cbor.Encode(
		map[uint]any{5: []any{}},
	)
	require.NoError(t, err)
	var witnesses DijkstraTransactionWitnessSet
	require.Error(t, witnesses.UnmarshalCBOR(legacyEmptyRedeemer))

	bodyWire, err := cbor.Encode(map[uint]any{
		0: cbor.NewSetType([]any{}, false),
		1: []any{},
		2: uint64(0),
	})
	require.NoError(t, err)
	witnessWire, err := cbor.Encode(map[uint]any{0: []any{}})
	require.NoError(t, err)
	txWire, err := cbor.Encode([]any{
		cbor.RawMessage(bodyWire),
		cbor.RawMessage(witnessWire),
		nil,
	})
	require.NoError(t, err)
	_, err = NewDijkstraTransactionFromCbor(txWire)
	require.ErrorContains(t, err, "failed to decode transaction witness set")
}

func TestDijkstraWitnessSetRejectsField8TaggedAndUntagged(t *testing.T) {
	for _, value := range []any{
		[]any{},
		[]any{[]byte{0x01}},
		cbor.Set{},
		cbor.Set{[]byte{0x01}},
	} {
		wire, err := cbor.Encode(map[uint]any{8: value})
		require.NoError(t, err)
		var witnesses DijkstraTransactionWitnessSet
		require.ErrorContains(
			t,
			witnesses.UnmarshalCBOR(wire),
			"does not support field 8",
		)
	}
}

func TestDijkstraBootstrapWitnessChainCodeHasProtocolWidth(t *testing.T) {
	publicKey := bytes.Repeat([]byte{0x11}, ed25519.PublicKeySize)
	signature := bytes.Repeat([]byte{0x22}, ed25519.SignatureSize)
	for _, tagged := range []bool{false, true} {
		for _, length := range []int{31, 32, 33} {
			name := fmt.Sprintf("%d bytes/tagged=%t", length, tagged)
			t.Run(name, func(t *testing.T) {
				witness := []any{
					publicKey,
					signature,
					bytes.Repeat([]byte{0x33}, length),
					[]byte{},
				}
				witnesses := any([]any{witness})
				if tagged {
					witnesses = cbor.NewSetType([]any{witness}, true)
				}
				wire, err := cbor.Encode(map[uint]any{2: witnesses})
				require.NoError(t, err)
				var decoded DijkstraTransactionWitnessSet
				err = decoded.UnmarshalCBOR(wire)
				if length == 32 {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "chain code must be 32 bytes")
				}
			})
		}
	}
}

func TestDijkstraTransactionRejectsUnusedBootstrapWitnessWithBadChainCode(
	t *testing.T,
) {
	bodyWire, err := cbor.Encode(minimalTxBody())
	require.NoError(t, err)
	bodyHash := common.Blake2b256Hash(bodyWire)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x44}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	signature := ed25519.Sign(privateKey, bodyHash[:])
	for _, length := range []int{31, 32, 33} {
		t.Run(fmt.Sprintf("chain-code-%d", length), func(t *testing.T) {
			witnessWire, encodeErr := cbor.Encode(map[uint]any{
				2: []any{[]any{
					[]byte(publicKey),
					signature,
					bytes.Repeat([]byte{0x55}, length),
					[]byte{},
				}},
			})
			require.NoError(t, encodeErr)
			txWire, encodeErr := cbor.Encode([]any{
				cbor.RawMessage(bodyWire),
				cbor.RawMessage(witnessWire),
				nil,
			})
			require.NoError(t, encodeErr)
			_, decodeErr := NewDijkstraTransactionFromCbor(txWire)
			if length == 32 {
				require.NoError(t, decodeErr)
			} else {
				require.ErrorContains(t, decodeErr, "chain code must be 32 bytes")
			}
		})
	}
}
