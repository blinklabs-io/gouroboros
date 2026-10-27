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
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func crossLevelPoolCert(
	t *testing.T,
	operator, vrf byte,
) common.CertificateWrapper {
	t.Helper()
	certificate := &common.PoolRegistrationCertificate{
		CertType:   uint(common.CertificateTypePoolRegistration),
		Operator:   common.PoolKeyHash{operator},
		VrfKeyHash: common.Blake2b256{vrf},
		Cost:       340_000_000,
		Margin:     cbor.Rat{Rat: big.NewRat(0, 1)},
	}
	require.NoError(t, certificate.SetRewardAccountCredential(
		common.Credential{CredType: common.CredentialTypeAddrKeyHash},
		common.AddressNetworkTestnet,
	))
	return common.CertificateWrapper{
		Type:        uint(common.CertificateTypePoolRegistration),
		Certificate: certificate,
	}
}

func crossLevelTx(
	sub DijkstraSubTransactionBody,
	top DijkstraTransactionBody,
) *DijkstraTransaction {
	top.TxSubTransactions = cbor.NewSetType(
		[]DijkstraSubTransaction{{Body: sub}},
		true,
	)
	return &DijkstraTransaction{Body: top, TxIsValid: true}
}

func TestUtxoValidatePoolCertificatesCarriesVrfOwnershipAcrossLevels(
	t *testing.T,
) {
	t.Parallel()
	ls := futurePoolLedgerState{
		LedgerState: mockledger.NewLedgerStateBuilder().WithNetworkId(0).Build(),
	}
	pp := &DijkstraProtocolParameters{}
	pp.ProtocolVersion.Major = 12
	pp.MinPoolCost = 340_000_000
	register := func(sub, top []common.CertificateWrapper) error {
		return UtxoValidatePoolCertificates(crossLevelTx(
			DijkstraSubTransactionBody{TxCertificates: sub},
			DijkstraTransactionBody{TxCertificates: top},
		), 0, ls, pp)
	}

	t.Run("other operator reusing an earlier level's key is rejected", func(t *testing.T) {
		t.Parallel()
		err := register(
			[]common.CertificateWrapper{crossLevelPoolCert(t, 0x01, 0xaa)},
			[]common.CertificateWrapper{crossLevelPoolCert(t, 0x02, 0xaa)},
		)
		var dup shelley.VrfKeyHashAlreadyRegisteredError
		require.ErrorAs(t, err, &dup)
		require.Equal(t, common.PoolKeyHash{0x02}, dup.PoolKeyHash)
		require.Equal(t, common.PoolKeyHash{0x01}, dup.RegisteredBy)
	})
	t.Run("same operator may re-register its own key", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, register(
			[]common.CertificateWrapper{crossLevelPoolCert(t, 0x01, 0xaa)},
			[]common.CertificateWrapper{crossLevelPoolCert(t, 0x01, 0xaa)},
		))
	})
	t.Run("distinct keys are accepted", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, register(
			[]common.CertificateWrapper{crossLevelPoolCert(t, 0x01, 0xaa)},
			[]common.CertificateWrapper{crossLevelPoolCert(t, 0x02, 0xbb)},
		))
	})
	t.Run("later level without the earlier registration sees no owner", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, register(
			nil,
			[]common.CertificateWrapper{crossLevelPoolCert(t, 0x02, 0xaa)},
		))
	})
}

func TestUtxoValidateCertificateDepositsFollowEarlierLevelEffects(
	t *testing.T,
) {
	t.Parallel()
	const keyDeposit = uint64(2_000_000)
	pp := &DijkstraProtocolParameters{}
	pp.KeyDeposit = uint(keyDeposit)
	address := dijkstraIntervalAddress(t, 0x61)
	credential := dijkstraIntervalCredential(t, address)
	key := dijkstraIntervalKey(t, address)
	registeredLedger := func(balance uint64) common.LedgerState {
		return mockledger.NewLedgerStateBuilder().
			WithRewardAccountCredentialBalance(credential, balance).
			WithStakeCredentialDeposits(
				map[mockledger.RewardAccountKey]uint64{
					mockledger.NewRewardAccountKey(credential): keyDeposit,
				},
			).
			Build()
	}
	emptyLedger := mockledger.NewLedgerStateBuilder().Build()
	deregister := func(refund uint64) []common.CertificateWrapper {
		return []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeDeregistration),
			Certificate: &common.DeregistrationCertificate{
				CertType:        uint(common.CertificateTypeDeregistration),
				StakeCredential: credential,
				Amount:          refund,
			},
		}}
	}
	register := func(deposit uint64) []common.CertificateWrapper {
		return []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeRegistration),
			Certificate: &common.RegistrationCertificate{
				CertType:        uint(common.CertificateTypeRegistration),
				StakeCredential: credential,
				Amount:          deposit,
			},
		}}
	}

	t.Run("earlier direct deposit blocks deregistration", func(t *testing.T) {
		t.Parallel()
		tx := crossLevelTx(
			DijkstraSubTransactionBody{
				TxDirectDeposits: DijkstraDirectDeposits{key: 25},
			},
			DijkstraTransactionBody{TxCertificates: deregister(keyDeposit)},
		)
		var balanceErr conway.StakeCredentialNonZeroRewardBalanceError
		require.ErrorAs(
			t,
			UtxoValidateCertificateDeposits(tx, 0, registeredLedger(0), pp),
			&balanceErr,
		)
		require.Equal(t, uint64(25), balanceErr.Balance)
	})
	t.Run("no earlier deposit leaves deregistration valid", func(t *testing.T) {
		t.Parallel()
		tx := crossLevelTx(
			DijkstraSubTransactionBody{},
			DijkstraTransactionBody{TxCertificates: deregister(keyDeposit)},
		)
		require.NoError(t, UtxoValidateCertificateDeposits(
			tx, 0, registeredLedger(0), pp,
		))
	})
	t.Run("earlier withdrawal drains the balance", func(t *testing.T) {
		t.Parallel()
		withdrawals := map[*common.Address]uint64{address: 100}
		tx := crossLevelTx(
			DijkstraSubTransactionBody{TxWithdrawals: withdrawals},
			DijkstraTransactionBody{TxCertificates: deregister(keyDeposit)},
		)
		require.NoError(t, UtxoValidateCertificateDeposits(
			tx, 0, registeredLedger(100), pp,
		))
		tx = crossLevelTx(
			DijkstraSubTransactionBody{},
			DijkstraTransactionBody{TxCertificates: deregister(keyDeposit)},
		)
		var balanceErr conway.StakeCredentialNonZeroRewardBalanceError
		require.ErrorAs(
			t,
			UtxoValidateCertificateDeposits(tx, 0, registeredLedger(100), pp),
			&balanceErr,
		)
	})
	t.Run("earlier registration is visible to later levels", func(t *testing.T) {
		t.Parallel()
		tx := crossLevelTx(
			DijkstraSubTransactionBody{TxCertificates: register(keyDeposit)},
			DijkstraTransactionBody{TxCertificates: register(keyDeposit)},
		)
		var registeredErr conway.StakeCredentialAlreadyRegisteredError
		require.ErrorAs(
			t,
			UtxoValidateCertificateDeposits(tx, 0, emptyLedger, pp),
			&registeredErr,
		)
		tx = crossLevelTx(
			DijkstraSubTransactionBody{TxCertificates: register(keyDeposit)},
			DijkstraTransactionBody{TxCertificates: deregister(keyDeposit)},
		)
		require.NoError(t, UtxoValidateCertificateDeposits(
			tx, 0, emptyLedger, pp,
		))
		tx = crossLevelTx(
			DijkstraSubTransactionBody{TxCertificates: register(keyDeposit)},
			DijkstraTransactionBody{TxCertificates: deregister(keyDeposit + 1)},
		)
		var refundErr conway.CertificateRefundIncorrectError
		require.ErrorAs(
			t,
			UtxoValidateCertificateDeposits(tx, 0, emptyLedger, pp),
			&refundErr,
		)
		require.Equal(t, keyDeposit, refundErr.Expected)
	})
	t.Run("direct deposit to a credential registered earlier blocks deregistration", func(t *testing.T) {
		t.Parallel()
		tx := crossLevelTx(
			DijkstraSubTransactionBody{
				TxCertificates:   register(keyDeposit),
				TxDirectDeposits: DijkstraDirectDeposits{key: 7},
			},
			DijkstraTransactionBody{TxCertificates: deregister(keyDeposit)},
		)
		var balanceErr conway.StakeCredentialNonZeroRewardBalanceError
		require.ErrorAs(
			t,
			UtxoValidateCertificateDeposits(tx, 0, emptyLedger, pp),
			&balanceErr,
		)
		require.Equal(t, uint64(7), balanceErr.Balance)
	})
}
