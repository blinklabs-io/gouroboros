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
	return testLeiosPoolKeyForDomain(t, secret, testLeiosProofDST)
}

func testLeiosPoolKeyForDomain(
	t *testing.T,
	secret int64,
	dst string,
) common.LeiosKey {
	t.Helper()
	scalar := big.NewInt(secret)
	var public bls12381.G2Affine
	public.ScalarMultiplicationBase(scalar)
	encodedPublic := public.Bytes()
	hash, err := bls12381.HashToG1(
		encodedPublic[:],
		[]byte(dst),
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

// futurePoolLedgerState adds the future pool parameter lookup the pool
// certificate rules require.
type futurePoolLedgerState struct {
	common.LedgerState
}

func (futurePoolLedgerState) IsFutureVrfKeyInUse(
	common.Blake2b256,
) (bool, common.PoolKeyHash, error) {
	return false, common.PoolKeyHash{}, nil
}

func TestUtxoValidatePoolCertificatesBlsProofOfPossession(t *testing.T) {
	t.Parallel()
	ls := futurePoolLedgerState{
		LedgerState: mockledger.NewLedgerStateBuilder().
			WithNetworkId(0).
			Build(),
	}
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
		require.NotNil(t, popErr)
		require.Equal(t, common.PoolKeyHash{0x01}, popErr.PoolId)
		require.Equal(t, invalid, popErr.Key)
		require.ErrorIs(t, err, leios.ErrInvalidProof)
	}

	// The POOL rule is not reached by a phase-2-invalid transaction.
	invalidTx := testPoolRegistrationTx(t, &invalid, false)
	invalidTx.TxIsValid = false
	require.NoError(t, UtxoValidatePoolCertificates(invalidTx, 0, ls, pp))
}

type poolProofDomainState struct {
	common.LedgerState
	domain common.DijkstraBlsProofDomain
}

func (s poolProofDomainState) DijkstraBlsProofDomain() common.DijkstraBlsProofDomain {
	return s.domain
}

func (s poolProofDomainState) UnwrapLedgerState() common.LedgerState {
	return s.LedgerState
}

func TestUtxoValidatePoolCertificatesSelectsBlsProofDomain(t *testing.T) {
	t.Parallel()
	base := futurePoolLedgerState{
		LedgerState: mockledger.NewLedgerStateBuilder().
			WithNetworkId(0).
			Build(),
	}
	pp := &DijkstraProtocolParameters{}
	pp.ProtocolVersion.Major = 12
	pp.MinPoolCost = 340_000_000
	standard := testLeiosPoolKey(t, 11)
	legacy := testLeiosPoolKeyForDomain(
		t,
		11,
		"BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_",
	)
	other := testLeiosPoolKeyForDomain(
		t,
		12,
		"BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_",
	)
	mismatched := common.LeiosKey{
		PublicKey:       legacy.PublicKey,
		PossessionProof: other.PossessionProof,
	}
	for _, tc := range []struct {
		name     string
		state    common.LedgerState
		accepted *common.LeiosKey
		rejected *common.LeiosKey
	}{
		{"default", base, &standard, &legacy},
		{"explicit_pop", poolProofDomainState{base, common.DijkstraBlsProofDomainPop}, &standard, &legacy},
		{"legacy_sig", poolProofDomainState{base, common.DijkstraBlsProofDomainLegacySig}, &legacy, &standard},
		{"outer_pop", poolProofDomainState{common.NewBlockLedgerState(poolProofDomainState{base, common.DijkstraBlsProofDomainLegacySig}), common.DijkstraBlsProofDomainPop}, &standard, &legacy},
		{"outer_legacy", poolProofDomainState{common.NewBlockLedgerState(poolProofDomainState{base, common.DijkstraBlsProofDomainPop}), common.DijkstraBlsProofDomainLegacySig}, &legacy, &standard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, sub := range []bool{false, true} {
				for _, state := range []common.LedgerState{tc.state, common.NewBlockLedgerState(tc.state), common.NewBlockLedgerState(common.NewBlockLedgerState(tc.state))} {
					for _, key := range []*common.LeiosKey{nil, tc.accepted} {
						for _, tx := range testPoolProofTransactions(t, key, sub) {
							require.NoError(
								t,
								common.VerifyTransaction(
									tx,
									0,
									state,
									pp,
									[]common.UtxoValidationRuleFunc{
										UtxoValidatePoolCertificates,
									},
								),
							)
						}
					}
					for _, key := range []*common.LeiosKey{tc.rejected, &mismatched} {
						for _, tx := range testPoolProofTransactions(t, key, sub) {
							err := common.VerifyTransaction(
								tx,
								0,
								state,
								pp,
								[]common.UtxoValidationRuleFunc{
									UtxoValidatePoolCertificates,
								},
							)
							var proofErr *BlsKeyInvalidProofOfPossessionError
							require.ErrorAs(t, err, &proofErr)
							require.ErrorIs(t, err, leios.ErrInvalidProof)
						}
					}
				}
			}
		})
	}
}

func testPoolProofTransactions(
	t *testing.T,
	key *common.LeiosKey,
	sub bool,
) []common.Transaction {
	t.Helper()
	tx := testPoolRegistrationTx(t, key, sub)
	if sub {
		return []common.Transaction{tx}
	}
	ordinary := mockledger.NewTransactionBuilder().
		WithCertificates(tx.Certificates()...)
	return []common.Transaction{tx, ordinary}
}

func TestUtxoValidatePoolCertificatesUnknownBlsProofDomain(t *testing.T) {
	t.Parallel()
	state := poolProofDomainState{
		futurePoolLedgerState{
			mockledger.NewLedgerStateBuilder().WithNetworkId(0).Build(),
		},
		common.DijkstraBlsProofDomain(255),
	}
	pp := &DijkstraProtocolParameters{}
	pp.ProtocolVersion.Major = 12
	pp.MinPoolCost = 340_000_000
	key := testLeiosPoolKey(t, 11)
	for _, sub := range []bool{false, true} {
		for _, tx := range testPoolProofTransactions(t, &key, sub) {
			err := UtxoValidatePoolCertificates(
				tx,
				0,
				common.NewBlockLedgerState(state),
				pp,
			)
			var configErr *common.ValidationError
			require.ErrorAs(t, err, &configErr)
			require.Equal(
				t,
				common.ValidationErrorTypeConfiguration,
				configErr.Type,
			)
			var proofErr *BlsKeyInvalidProofOfPossessionError
			require.NotErrorAs(t, err, &proofErr)
		}
	}
}
