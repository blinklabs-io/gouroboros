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

package ledger

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/stretchr/testify/require"
)

type dijkstraLeiosTestState struct {
	common.LedgerState
	context common.DijkstraLeiosCertificateContext
	err     error
}

// testLeiosKeyAge honours every key registered in epoch 0: mainnet's KES
// parameters give a maximum key age of 21 epochs.
var testLeiosKeyAge = common.DijkstraLeiosKeyAgeContext{
	Epoch:             1,
	MarkEpochLength:   432000,
	MaxKESEvolutions:  62,
	SlotsPerKESPeriod: 129600,
}

func (s dijkstraLeiosTestState) DijkstraLeiosKeyAgeContext(
	common.BlockHeader,
) (common.DijkstraLeiosKeyAgeContext, error) {
	return testLeiosKeyAge, nil
}

func (s dijkstraLeiosTestState) DijkstraLeiosCertificateContext(
	common.BlockHeader,
) (common.DijkstraLeiosCertificateContext, error) {
	return s.context, s.err
}

// validateDijkstraBlockCertificates validates a block's certificates the way
// VerifyBlockState does for blocks not received over node-to-client.
func validateDijkstraBlockCertificates(
	block Block,
	protocolParameters common.ProtocolParameters,
	ledgerState common.LedgerState,
) error {
	return validateDijkstraBlockCertificatesWith(
		block,
		protocolParameters,
		ledgerState,
		false,
	)
}

func TestValidateDijkstraBlockCertificates(t *testing.T) {
	t.Parallel()

	validCert := &dijkstra.DijkstraLeiosCertificate{
		Signers:             []byte{0x80, 0x80},
		AggregatedSignature: make([]byte, common.LeiosBlsSignatureSize),
	}
	block := &dijkstra.DijkstraBlock{
		BlockHeader: &dijkstra.DijkstraBlockHeader{
			LeiosHeaderExtension: []cbor.RawMessage{{0xf5}},
		},
		BlockBody: dijkstra.DijkstraBlockBody{LeiosCertificate: validCert},
	}
	params := &dijkstra.DijkstraProtocolParameters{
		LeiosCommitteeSize:        9,
		LeiosQuorumStakeThreshold: &cbor.Rat{Rat: big.NewRat(1, 2)},
	}

	t.Run("valid certificate", func(t *testing.T) {
		err := validateDijkstraBlockCertificates(block, params, nil)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(
			t,
			common.ValidationErrorTypeConfiguration,
			validationErr.Type,
		)
	})
	t.Run(
		"certificate requires key-backed committee state",
		func(t *testing.T) {
			state := dijkstraLeiosTestState{
				context: common.DijkstraLeiosCertificateContext{
					TotalActiveStake: 100,
					Committee: make(
						[]common.DijkstraLeiosCommitteeMember,
						9,
					),
				},
			}
			err := validateDijkstraBlockCertificates(block, params, state)
			require.Error(t, err)
			var validationErr *common.ValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(
				t,
				common.ValidationErrorTypeProtocol,
				validationErr.Type,
			)
		},
	)
	t.Run("missing protocol parameters", func(t *testing.T) {
		err := validateDijkstraBlockCertificates(block, nil, nil)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(
			t,
			common.ValidationErrorTypeConfiguration,
			validationErr.Type,
		)
	})
	t.Run("invalid committee bitfield", func(t *testing.T) {
		// Longer than any committee of at most nine seats needs.
		invalid := *validCert
		invalid.Signers = []byte{0x80, 0x80, 0x80}
		invalidBlock := &dijkstra.DijkstraBlock{
			BlockHeader: block.BlockHeader,
			BlockBody:   dijkstra.DijkstraBlockBody{LeiosCertificate: &invalid},
		}
		err := validateDijkstraBlockCertificates(invalidBlock, params, nil)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
	})
	t.Run("block without certificate", func(t *testing.T) {
		require.NoError(t, validateDijkstraBlockCertificates(
			&dijkstra.DijkstraBlock{},
			nil,
			nil,
		))
	})
	t.Run("zero committee size", func(t *testing.T) {
		zeroCommittee := &dijkstra.DijkstraProtocolParameters{}
		err := validateDijkstraBlockCertificates(block, zeroCommittee, nil)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(
			t,
			common.ValidationErrorTypeConfiguration,
			validationErr.Type,
		)
	})
	t.Run("certified header without certificate", func(t *testing.T) {
		missing := &dijkstra.DijkstraBlock{
			BlockHeader: block.BlockHeader,
		}
		err := validateDijkstraBlockCertificates(missing, params, nil)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
	})
	t.Run("uncertified header with certificate", func(t *testing.T) {
		uncertified := *block
		uncertified.BlockHeader = &dijkstra.DijkstraBlockHeader{
			LeiosHeaderExtension: []cbor.RawMessage{{0xf4}},
		}
		err := validateDijkstraBlockCertificates(&uncertified, params, nil)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
	})
	t.Run("malformed certified flag", func(t *testing.T) {
		malformed := *block
		malformed.BlockHeader = &dijkstra.DijkstraBlockHeader{
			LeiosHeaderExtension: []cbor.RawMessage{{0x01}},
		}
		err := validateDijkstraBlockCertificates(&malformed, params, nil)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
	})
}

type dijkstraLeiosDomainState struct {
	dijkstraLeiosTestState
	domain common.DijkstraBlsProofDomain
}

func (s dijkstraLeiosDomainState) DijkstraBlsProofDomain() common.DijkstraBlsProofDomain {
	return s.domain
}

type dijkstraLeiosMessageState struct {
	dijkstraLeiosTestState
	message common.DijkstraLeiosCertificateMessage
}

func (s dijkstraLeiosMessageState) DijkstraLeiosCertificateMessage() common.DijkstraLeiosCertificateMessage {
	return s.message
}

func TestValidateDijkstraBlockCertificatesUsesLedgerProofDomain(t *testing.T) {
	t.Parallel()

	const legacyDST = "BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_"
	sign := func(message []byte, secret *big.Int) []byte {
		hash, err := bls12381.HashToG1(message, []byte(legacyDST))
		require.NoError(t, err)
		var sig bls12381.G1Affine
		sig.ScalarMultiplication(&hash, secret)
		encoded := sig.Bytes()
		return encoded[:]
	}
	secret := big.NewInt(23)
	var public bls12381.G2Affine
	public.ScalarMultiplicationBase(secret)
	encodedPublic := public.Bytes()
	blockHash := common.Blake2b256Hash([]byte("announcing ranking block"))
	message := blockHash[:]
	framed := append([]byte{0x58, 0x20}, blockHash[:]...)
	state := dijkstraLeiosTestState{
		context: common.DijkstraLeiosCertificateContext{
			AnnouncingBlockHash: blockHash,
			TotalActiveStake:    1,
			Committee: []common.DijkstraLeiosCommitteeMember{{
				Stake: 1,
				Key: &common.LeiosKey{
					PublicKey:       encodedPublic[:],
					PossessionProof: sign(encodedPublic[:], secret),
				},
			}},
		},
	}
	block := &dijkstra.DijkstraBlock{
		BlockHeader: &dijkstra.DijkstraBlockHeader{
			LeiosHeaderExtension: []cbor.RawMessage{{0xf5}},
		},
		BlockBody: dijkstra.DijkstraBlockBody{
			LeiosCertificate: &dijkstra.DijkstraLeiosCertificate{
				Signers:             []byte{0x80},
				AggregatedSignature: sign(message, secret),
			},
		},
	}
	params := &dijkstra.DijkstraProtocolParameters{
		LeiosCommitteeSize:        1,
		LeiosQuorumStakeThreshold: &cbor.Rat{Rat: big.NewRat(1, 1)},
	}

	require.NoError(t, validateDijkstraBlockCertificates(
		block, params,
		dijkstraLeiosDomainState{
			state, common.DijkstraBlsProofDomainLegacySig,
		},
	))
	require.Error(t, validateDijkstraBlockCertificates(block, params, state))

	// The ledger state also selects the signed message: a certificate over
	// the raw hash fails under the prototype encoding and the reverse.
	both := dijkstraLeiosMessageState{
		dijkstraLeiosTestState: state,
		message:                common.DijkstraLeiosCertificateMessageCborHash,
	}
	legacyState := func(
		s dijkstraLeiosMessageState,
	) dijkstraLeiosDomainStateWithMessage {
		return dijkstraLeiosDomainStateWithMessage{
			s, common.DijkstraBlsProofDomainLegacySig,
		}
	}
	require.Error(t, validateDijkstraBlockCertificates(
		block, params, legacyState(both),
	))
	block.BlockBody.LeiosCertificate.AggregatedSignature = sign(framed, secret)
	require.NoError(t, validateDijkstraBlockCertificates(
		block, params, legacyState(both),
	))
	require.Error(t, validateDijkstraBlockCertificates(
		block, params,
		legacyState(dijkstraLeiosMessageState{
			dijkstraLeiosTestState: state,
			message:                common.DijkstraLeiosCertificateMessageRawHash,
		}),
	))
}

type dijkstraLeiosDomainStateWithMessage struct {
	dijkstraLeiosMessageState
	domain common.DijkstraBlsProofDomain
}

func (s dijkstraLeiosDomainStateWithMessage) DijkstraBlsProofDomain() common.DijkstraBlsProofDomain {
	return s.domain
}

func TestValidateDijkstraBlockCertificatesInvalidDomainIsConfigurationError(
	t *testing.T,
) {
	t.Parallel()

	blockHash := common.Blake2b256Hash([]byte("announcing ranking block"))
	state := dijkstraLeiosDomainState{
		dijkstraLeiosTestState: dijkstraLeiosTestState{
			context: common.DijkstraLeiosCertificateContext{
				AnnouncingBlockHash: blockHash,
				TotalActiveStake:    1,
				Committee: []common.DijkstraLeiosCommitteeMember{{
					Stake: 1,
					Key:   &common.LeiosKey{},
				}},
			},
		},
		domain: common.DijkstraBlsProofDomain(0xff),
	}
	block := &dijkstra.DijkstraBlock{
		BlockHeader: &dijkstra.DijkstraBlockHeader{
			LeiosHeaderExtension: []cbor.RawMessage{{0xf5}},
		},
		BlockBody: dijkstra.DijkstraBlockBody{
			LeiosCertificate: &dijkstra.DijkstraLeiosCertificate{
				Signers:             []byte{0x80},
				AggregatedSignature: make([]byte, 48),
			},
		},
	}
	params := &dijkstra.DijkstraProtocolParameters{
		LeiosCommitteeSize:        1,
		LeiosQuorumStakeThreshold: &cbor.Rat{Rat: big.NewRat(1, 1)},
	}

	err := validateDijkstraBlockCertificates(block, params, state)
	var validationErr *common.ValidationError
	require.ErrorAs(t, err, &validationErr)
	if validationErr == nil {
		t.Fatal("expected validation error")
	}
	require.Equal(
		t, common.ValidationErrorTypeConfiguration, validationErr.Type,
	)
}

// A committee smaller than LeiosCommitteeSize, as when fewer pools are
// registered, has a signer bitfield sized by its own seats.
func TestValidateDijkstraBlockCertificatesSmallCommittee(t *testing.T) {
	t.Parallel()
	secret := big.NewInt(37)
	blockHash := common.Blake2b256Hash([]byte("announcing ranking block"))
	sign := func(message []byte, dst string) []byte {
		hash, err := bls12381.HashToG1(message, []byte(dst))
		require.NoError(t, err)
		var sig bls12381.G1Affine
		sig.ScalarMultiplication(&hash, secret)
		encoded := sig.Bytes()
		return encoded[:]
	}
	var public bls12381.G2Affine
	public.ScalarMultiplicationBase(secret)
	encodedPublic := public.Bytes()
	state := dijkstraLeiosTestState{
		context: common.DijkstraLeiosCertificateContext{
			AnnouncingBlockHash: blockHash,
			TotalActiveStake:    1,
			Committee: []common.DijkstraLeiosCommitteeMember{{
				Stake: 1,
				Key: &common.LeiosKey{
					PublicKey: encodedPublic[:],
					PossessionProof: sign(
						encodedPublic[:],
						"BLS_POP_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_",
					),
				},
			}},
		},
	}
	block := &dijkstra.DijkstraBlock{
		BlockHeader: &dijkstra.DijkstraBlockHeader{
			LeiosHeaderExtension: []cbor.RawMessage{{0xf5}},
		},
		BlockBody: dijkstra.DijkstraBlockBody{
			LeiosCertificate: &dijkstra.DijkstraLeiosCertificate{
				Signers: []byte{0x80},
				AggregatedSignature: sign(
					blockHash[:],
					"BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_",
				),
			},
		},
	}
	params := &dijkstra.DijkstraProtocolParameters{
		LeiosCommitteeSize:        900,
		LeiosQuorumStakeThreshold: &cbor.Rat{Rat: big.NewRat(3, 4)},
	}
	require.NoError(t, validateDijkstraBlockCertificates(block, params, state))

	oversized := *block.BlockBody.LeiosCertificate
	oversized.Signers = make([]byte, common.LeiosSignerBitfieldSize(900)+1)
	oversized.Signers[0] = 0x80
	oversizedBlock := &dijkstra.DijkstraBlock{
		BlockHeader: block.BlockHeader,
		BlockBody:   dijkstra.DijkstraBlockBody{LeiosCertificate: &oversized},
	}
	err := validateDijkstraBlockCertificates(oversizedBlock, params, nil)
	var validationErr *common.ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.NotNil(t, validationErr)
	require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
}
