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
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

var (
	poolOwnerTestOperatorKey = []byte{0x01, 0x01, 0x01}
	poolOwnerTestOwnerKey    = []byte{0x02, 0x02, 0x02}
)

func poolOwnerTestCert(
	t *testing.T,
	pledge uint64,
	leiosKey *common.LeiosKey,
) *common.PoolRegistrationCertificate {
	t.Helper()
	cert := &common.PoolRegistrationCertificate{
		CertType: uint(common.CertificateTypePoolRegistration),
		Operator: common.PoolKeyHash(
			common.Blake2b224Hash(poolOwnerTestOperatorKey),
		),
		VrfKeyHash: common.VrfKeyHash{0x0a},
		LeiosKey:   leiosKey,
		Pledge:     pledge,
		Cost:       340_000_000,
		Margin:     cbor.Rat{Rat: big.NewRat(1, 100)},
		PoolOwners: []common.AddrKeyHash{
			common.AddrKeyHash(common.Blake2b224Hash(poolOwnerTestOwnerKey)),
		},
	}
	require.NoError(t, cert.SetRewardAccountCredential(
		common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: common.Blake2b224Hash(poolOwnerTestOwnerKey),
		},
		common.AddressNetworkTestnet,
	))
	return cert
}

func poolOwnerTestWitnesses(
	keys ...[]byte,
) DijkstraTransactionWitnessSet {
	wits := make([]common.VkeyWitness, 0, len(keys))
	for _, key := range keys {
		wits = append(wits, common.VkeyWitness{Vkey: key})
	}
	return DijkstraTransactionWitnessSet{
		VkeyWitnesses: cbor.NewSetType(wits, true),
	}
}

// poolOwnerTestTx puts the registration at the top level, or in a
// sub-transaction whose witnesses are subKeys while the top level carries
// topKeys.
func poolOwnerTestTx(
	cert *common.PoolRegistrationCertificate,
	valid bool,
	inSub bool,
	topKeys [][]byte,
	subKeys [][]byte,
) *DijkstraTransaction {
	wrapped := []common.CertificateWrapper{{
		Type:        uint(common.CertificateTypePoolRegistration),
		Certificate: cert,
	}}
	tx := &DijkstraTransaction{
		TxIsValid: valid,
		WitnessSet: poolOwnerTestWitnesses(
			topKeys...,
		),
	}
	if !inSub {
		tx.Body.TxCertificates = wrapped
		return tx
	}
	tx.Body.TxSubTransactions = cbor.NewSetType(
		[]DijkstraSubTransaction{{
			Body:       DijkstraSubTransactionBody{TxCertificates: wrapped},
			WitnessSet: poolOwnerTestWitnesses(subKeys...),
		}},
		true,
	)
	return tx
}

func poolOwnerTestState(
	current *common.PoolRegistrationCertificate,
) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithNetworkId(0).
		WithPoolCurrentState(func(
			pool common.PoolKeyHash,
		) (*common.PoolRegistrationCertificate, *uint64, error) {
			if current != nil && pool == current.Operator {
				return current, nil, nil
			}
			return nil, nil, nil
		}).
		Build()
}

// pendingPoolState reports pool parameters awaiting the epoch boundary.
type pendingPoolState struct {
	common.LedgerState
	pending *common.PoolRegistrationCertificate
}

func (s pendingPoolState) PoolPendingParameters(
	common.PoolKeyHash,
) (*common.PoolRegistrationCertificate, error) {
	return s.pending, nil
}

func TestDijkstraPoolOwnerWitnessExemptions(t *testing.T) {
	t.Parallel()
	validator := dijkstraRule(
		t,
		common.UtxoValidationRuleRequiredVKeyWitnesses,
	)
	operator, owner := poolOwnerTestOperatorKey, poolOwnerTestOwnerKey
	blsKey := &common.LeiosKey{PublicKey: []byte{0x0b}}
	current := poolOwnerTestCert(t, 100, nil)

	for _, tc := range []struct {
		name    string
		cert    *common.PoolRegistrationCertificate
		state   common.LedgerState
		valid   bool
		inSub   bool
		top     [][]byte
		sub     [][]byte
		wantErr bool
	}{
		{
			name:  "phase-2 invalid new pool needs only operator",
			cert:  poolOwnerTestCert(t, 100, nil),
			state: poolOwnerTestState(nil),
			top:   [][]byte{operator},
		},
		{
			name:    "phase-2 invalid new pool still needs operator",
			cert:    poolOwnerTestCert(t, 100, nil),
			state:   poolOwnerTestState(nil),
			top:     [][]byte{owner},
			wantErr: true,
		},
		{
			name:  "phase-2 invalid sub-transaction needs only operator",
			cert:  poolOwnerTestCert(t, 100, nil),
			state: poolOwnerTestState(nil),
			inSub: true,
			sub:   [][]byte{operator},
		},
		{
			name:    "phase-2 invalid sub-transaction still needs operator",
			cert:    poolOwnerTestCert(t, 100, nil),
			state:   poolOwnerTestState(nil),
			inSub:   true,
			sub:     [][]byte{owner},
			wantErr: true,
		},
		{
			name:    "valid new pool needs owner",
			cert:    poolOwnerTestCert(t, 100, nil),
			state:   poolOwnerTestState(nil),
			valid:   true,
			top:     [][]byte{operator},
			wantErr: true,
		},
		{
			name:  "valid new pool with owner",
			cert:  poolOwnerTestCert(t, 100, nil),
			state: poolOwnerTestState(nil),
			valid: true,
			top:   [][]byte{operator, owner},
		},
		{
			name:  "bls-only change needs no owner",
			cert:  poolOwnerTestCert(t, 100, blsKey),
			state: pendingPoolState{LedgerState: poolOwnerTestState(current)},
			valid: true,
			top:   [][]byte{operator},
		},
		{
			name:  "bls-only change in sub-transaction needs no owner",
			cert:  poolOwnerTestCert(t, 100, blsKey),
			state: pendingPoolState{LedgerState: poolOwnerTestState(current)},
			valid: true,
			inSub: true,
			sub:   [][]byte{operator},
		},
		{
			name:    "bls-only change still needs operator",
			cert:    poolOwnerTestCert(t, 100, blsKey),
			state:   pendingPoolState{LedgerState: poolOwnerTestState(current)},
			valid:   true,
			top:     [][]byte{owner},
			wantErr: true,
		},
		{
			name:    "changed pledge needs owner",
			cert:    poolOwnerTestCert(t, 200, blsKey),
			state:   pendingPoolState{LedgerState: poolOwnerTestState(current)},
			valid:   true,
			top:     [][]byte{operator},
			wantErr: true,
		},
		{
			name: "matching pending params need no owner",
			cert: poolOwnerTestCert(t, 300, blsKey),
			state: pendingPoolState{
				LedgerState: poolOwnerTestState(current),
				pending:     poolOwnerTestCert(t, 300, nil),
			},
			valid: true,
			top:   [][]byte{operator},
		},
		{
			name: "matching current but not pending needs owner",
			cert: poolOwnerTestCert(t, 100, blsKey),
			state: pendingPoolState{
				LedgerState: poolOwnerTestState(current),
				pending:     poolOwnerTestCert(t, 300, nil),
			},
			valid:   true,
			top:     [][]byte{operator},
			wantErr: true,
		},
		{
			name:    "state without pending lookup never exempts",
			cert:    poolOwnerTestCert(t, 100, blsKey),
			state:   poolOwnerTestState(current),
			valid:   true,
			top:     [][]byte{operator},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := poolOwnerTestTx(tc.cert, tc.valid, tc.inSub, tc.top, tc.sub)
			err := validator(tx, 0, tc.state, nil)
			if tc.wantErr {
				require.ErrorAs(
					t,
					err,
					&common.MissingRequiredVKeyWitnessForSignerError{},
				)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConwayPoolOwnerWitnessStillRequired(t *testing.T) {
	t.Parallel()
	current := poolOwnerTestCert(t, 100, nil)
	cert := poolOwnerTestCert(t, 100, &common.LeiosKey{PublicKey: []byte{0x0b}})
	tx := &conway.ConwayTransaction{
		TxIsValid: true,
		Body: conway.ConwayTransactionBody{
			TxCertificates: []common.CertificateWrapper{{
				Type:        uint(common.CertificateTypePoolRegistration),
				Certificate: cert,
			}},
		},
		WitnessSet: conway.ConwayTransactionWitnessSet{
			VkeyWitnesses: cbor.NewSetType(
				[]common.VkeyWitness{{Vkey: poolOwnerTestOperatorKey}},
				true,
			),
		},
	}
	err := conway.UtxoValidateRequiredVKeyWitnesses(
		tx,
		0,
		pendingPoolState{LedgerState: poolOwnerTestState(current)},
		nil,
	)
	require.ErrorAs(
		t,
		err,
		&common.MissingRequiredVKeyWitnessForSignerError{},
	)
}
