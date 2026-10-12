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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/leios"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/stretchr/testify/require"
)

const testLeiosProofDST = "BLS_POP_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_"

func testLeiosPoolKey(t *testing.T, secret int64) common.LeiosKey {
	t.Helper()
	scalar := big.NewInt(secret)
	var public bls12381.G2Affine
	public.ScalarMultiplicationBase(scalar)
	encodedPublic := public.Bytes()
	hash, err := bls12381.HashToG1(
		encodedPublic[:],
		[]byte(testLeiosProofDST),
	)
	require.NoError(t, err)
	var proof bls12381.G1Affine
	proof.ScalarMultiplication(&hash, scalar)
	encodedProof := proof.Bytes()
	return common.LeiosKey{
		PublicKey:       encodedPublic[:],
		PossessionProof: encodedProof[:],
	}
}

func testPoolRegistrationTx(
	t *testing.T,
	key *common.LeiosKey,
	subTransaction bool,
) *DijkstraTransaction {
	t.Helper()
	certificate := &common.PoolRegistrationCertificate{
		CertType: uint(common.CertificateTypePoolRegistration),
		Operator: common.PoolKeyHash{0x01},
		LeiosKey: key,
		Cost:     340_000_000,
		Margin:   cbor.Rat{Rat: big.NewRat(0, 1)},
	}
	require.NoError(t, certificate.SetRewardAccountCredential(
		common.Credential{CredType: common.CredentialTypeAddrKeyHash},
		common.AddressNetworkTestnet,
	))
	certificates := []common.CertificateWrapper{{
		Type:        uint(common.CertificateTypePoolRegistration),
		Certificate: certificate,
	}}
	tx := &DijkstraTransaction{TxIsValid: true}
	if !subTransaction {
		tx.Body.TxCertificates = certificates
		return tx
	}
	tx.Body.TxSubTransactions = cbor.NewSetType(
		[]DijkstraSubTransaction{{
			Body: DijkstraSubTransactionBody{TxCertificates: certificates},
		}},
		true,
	)
	return tx
}

func TestUtxoValidatePoolCertificatesBlsProofOfPossession(t *testing.T) {
	t.Parallel()
	ls := mockledger.NewLedgerStateBuilder().WithNetworkId(0).Build()
	pp := &DijkstraProtocolParameters{}
	pp.ProtocolVersion.Major = 12
	pp.MinPoolCost = 340_000_000
	valid := testLeiosPoolKey(t, 11)
	other := testLeiosPoolKey(t, 12)
	invalid := common.LeiosKey{
		PublicKey:       valid.PublicKey,
		PossessionProof: other.PossessionProof,
	}

	for _, subTransaction := range []bool{false, true} {
		require.NoError(t, UtxoValidatePoolCertificates(
			testPoolRegistrationTx(t, nil, subTransaction), 0, ls, pp,
		))
		require.NoError(t, UtxoValidatePoolCertificates(
			testPoolRegistrationTx(t, &valid, subTransaction), 0, ls, pp,
		))
		err := UtxoValidatePoolCertificates(
			testPoolRegistrationTx(t, &invalid, subTransaction), 0, ls, pp,
		)
		var popErr *BlsKeyInvalidProofOfPossessionError
		require.ErrorAs(t, err, &popErr)
		require.Equal(t, common.PoolKeyHash{0x01}, popErr.PoolId)
		require.Equal(t, invalid, popErr.Key)
		require.ErrorIs(t, err, leios.ErrInvalidProof)
	}

	// The POOL rule is not reached by a phase-2-invalid transaction.
	invalidTx := testPoolRegistrationTx(t, &invalid, false)
	invalidTx.TxIsValid = false
	require.NoError(t, UtxoValidatePoolCertificates(invalidTx, 0, ls, pp))
}
