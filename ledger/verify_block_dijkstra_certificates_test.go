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

func (s dijkstraLeiosTestState) DijkstraLeiosCertificateContext(
	common.BlockHeader,
) (common.DijkstraLeiosCertificateContext, error) {
	return s.context, s.err
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
		require.Equal(t, common.ValidationErrorTypeConfiguration, validationErr.Type)
	})
	t.Run("certificate requires key-backed committee state", func(t *testing.T) {
		state := dijkstraLeiosTestState{
			context: common.DijkstraLeiosCertificateContext{
				TotalActiveStake: 100,
				Committee:        make([]common.DijkstraLeiosCommitteeMember, 9),
			},
		}
		err := validateDijkstraBlockCertificates(block, params, state)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
	})
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
		invalid := *validCert
		invalid.Signers = []byte{0x80}
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
		require.Equal(t, common.ValidationErrorTypeConfiguration, validationErr.Type)
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
	message := append([]byte{0x58, 0x20}, blockHash[:]...)
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
	require.Equal(
		t, common.ValidationErrorTypeConfiguration, validationErr.Type,
	)
}
