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

package common_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

type transactionBodyDecoder interface {
	UnmarshalCBOR([]byte) error
}

func transactionBodyDecoders() map[string]func() transactionBodyDecoder {
	return map[string]func() transactionBodyDecoder{
		"shelley":      func() transactionBodyDecoder { return &shelley.ShelleyTransactionBody{} },
		"allegra":      func() transactionBodyDecoder { return &allegra.AllegraTransactionBody{} },
		"mary":         func() transactionBodyDecoder { return &mary.MaryTransactionBody{} },
		"alonzo":       func() transactionBodyDecoder { return &alonzo.AlonzoTransactionBody{} },
		"babbage":      func() transactionBodyDecoder { return &babbage.BabbageTransactionBody{} },
		"conway":       func() transactionBodyDecoder { return &conway.ConwayTransactionBody{} },
		"dijkstra":     func() transactionBodyDecoder { return &dijkstra.DijkstraTransactionBody{} },
		"dijkstra_sub": func() transactionBodyDecoder { return &dijkstra.DijkstraSubTransactionBody{} },
	}
}

func testRewardAddress(t *testing.T) common.Address {
	t.Helper()
	addressBytes := make([]byte, 29)
	addressBytes[0] = 0xe1
	for idx := 1; idx < len(addressBytes); idx++ {
		addressBytes[idx] = byte(idx)
	}
	address, err := common.NewAddressFromBytes(addressBytes)
	require.NoError(t, err)
	return address
}

func requireDuplicateLogicalMapKey(t *testing.T, err error) {
	t.Helper()
	var duplicateError common.DuplicateLogicalMapKeyError
	require.ErrorAs(t, err, &duplicateError)
}

func TestTransactionBodiesRejectDuplicateLogicalWithdrawalKeys(t *testing.T) {
	address1 := testRewardAddress(t)
	address2 := address1
	duplicateWithdrawals := map[*common.Address]uint64{
		&address1: 1,
		&address2: 2,
	}
	duplicateBody, err := cbor.Encode(map[uint]any{5: duplicateWithdrawals})
	require.NoError(t, err)

	validAddress := testRewardAddress(t)
	validBody, err := cbor.Encode(map[uint]any{
		5: map[*common.Address]uint64{&validAddress: 1},
	})
	require.NoError(t, err)

	for era, newBody := range transactionBodyDecoders() {
		t.Run(era, func(t *testing.T) {
			err := newBody().UnmarshalCBOR(duplicateBody)
			requireDuplicateLogicalMapKey(t, err)
			require.NoError(t, newBody().UnmarshalCBOR(validBody))
		})
	}
}

func testRegistrationCertificate() *common.RegistrationCertificate {
	var hash common.Blake2b224
	for idx := range hash {
		hash[idx] = byte(idx + 1)
	}
	return &common.RegistrationCertificate{
		CertType: uint(common.CertificateTypeRegistration),
		StakeCredential: common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: hash,
		},
		Amount: 1,
	}
}

func TestTransactionBodiesRejectDuplicateCertificates(t *testing.T) {
	for _, tagged := range []bool{false, true} {
		encoding := "untagged"
		if tagged {
			encoding = "tagged"
		}
		t.Run(encoding, func(t *testing.T) {
			certificate := testRegistrationCertificate()
			differentCertificate := testRegistrationCertificate()
			differentCertificate.Amount = 2
			duplicateCertificates := any([]any{certificate, certificate})
			validCertificates := any(
				[]any{certificate, differentCertificate},
			)
			if tagged {
				duplicateCertificates = cbor.Set(
					[]any{certificate, certificate},
				)
				validCertificates = cbor.Set(
					[]any{certificate, differentCertificate},
				)
			}
			duplicateBody, err := cbor.Encode(map[uint]any{
				4: duplicateCertificates,
			})
			require.NoError(t, err)
			validBody, err := cbor.Encode(map[uint]any{4: validCertificates})
			require.NoError(t, err)

			for era, newBody := range transactionBodyDecoders() {
				t.Run(era, func(t *testing.T) {
					err := newBody().UnmarshalCBOR(duplicateBody)
					var duplicateError common.DuplicateCertificateError
					require.ErrorAs(t, err, &duplicateError)
					require.NoError(t, newBody().UnmarshalCBOR(validBody))
				})
			}
		})
	}
}

func TestTransactionBodiesRejectEquivalentCertificateEncodings(t *testing.T) {
	canonical, err := cbor.Encode(testRegistrationCertificate())
	require.NoError(t, err)
	require.Equal(t, byte(1), canonical[len(canonical)-1])
	nonShortestAmount := append([]byte(nil), canonical[:len(canonical)-1]...)
	nonShortestAmount = append(nonShortestAmount, 0x18, 0x01)
	body, err := cbor.Encode(map[uint]any{
		4: []any{
			cbor.RawMessage(canonical),
			cbor.RawMessage(nonShortestAmount),
		},
	})
	require.NoError(t, err)

	for era, newBody := range transactionBodyDecoders() {
		t.Run(era, func(t *testing.T) {
			err := newBody().UnmarshalCBOR(body)
			var duplicateError common.DuplicateCertificateError
			require.ErrorAs(t, err, &duplicateError)
		})
	}
}

func TestConwayTransactionDecoderRejectsDuplicateCollections(t *testing.T) {
	address1 := testRewardAddress(t)
	address2 := address1
	certificate := testRegistrationCertificate()
	tests := []struct {
		name string
		body map[uint]any
	}{
		{
			name: "withdrawals",
			body: map[uint]any{
				5: map[*common.Address]uint64{&address1: 1, &address2: 2},
			},
		},
		{
			name: "untagged certificates",
			body: map[uint]any{4: []any{certificate, certificate}},
		},
		{
			name: "tagged certificates",
			body: map[uint]any{
				4: cbor.Set([]any{certificate, certificate}),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bodyCbor, err := cbor.Encode(test.body)
			require.NoError(t, err)
			txCbor, err := cbor.Encode([]any{
				cbor.RawMessage(bodyCbor),
				map[uint]any{},
				true,
				nil,
			})
			require.NoError(t, err)
			_, err = conway.NewConwayTransactionFromCbor(txCbor)
			require.Error(t, err)
		})
	}

	validBody, err := cbor.Encode(map[uint]any{
		4: []any{certificate},
		5: map[*common.Address]uint64{&address1: 1},
	})
	require.NoError(t, err)
	validTx, err := cbor.Encode([]any{
		cbor.RawMessage(validBody),
		map[uint]any{},
		true,
		nil,
	})
	require.NoError(t, err)
	_, err = conway.NewConwayTransactionFromCbor(validTx)
	require.NoError(t, err)
}

func TestVotingProceduresRejectDuplicateLogicalKeys(t *testing.T) {
	var voterHash [28]byte
	voterHash[0] = 1
	voter1 := common.Voter{Type: common.VoterTypeDRepKeyHash, Hash: voterHash}
	voter2 := voter1
	var txId [32]byte
	txId[0] = 2
	action1 := common.GovActionId{TransactionId: txId, GovActionIdx: 3}
	action2 := action1
	procedure := common.VotingProcedure{Vote: common.GovVoteYes}

	t.Run("voter", func(t *testing.T) {
		encoded, err := cbor.Encode(
			map[*common.Voter]map[*common.GovActionId]common.VotingProcedure{
				&voter1: {&action1: procedure},
				&voter2: {&action1: procedure},
			},
		)
		require.NoError(t, err)
		var procedures common.VotingProcedures
		_, err = cbor.Decode(encoded, &procedures)
		requireDuplicateLogicalMapKey(t, err)
	})

	t.Run("action", func(t *testing.T) {
		encoded, err := cbor.Encode(
			map[*common.Voter]map[*common.GovActionId]common.VotingProcedure{
				&voter1: {
					&action1: procedure,
					&action2: procedure,
				},
			},
		)
		require.NoError(t, err)
		var procedures common.VotingProcedures
		_, err = cbor.Decode(encoded, &procedures)
		requireDuplicateLogicalMapKey(t, err)
	})

	t.Run("valid", func(t *testing.T) {
		encoded, err := cbor.Encode(
			map[*common.Voter]map[*common.GovActionId]common.VotingProcedure{
				&voter1: {&action1: procedure},
			},
		)
		require.NoError(t, err)
		var procedures common.VotingProcedures
		_, err = cbor.Decode(encoded, &procedures)
		require.NoError(t, err)
	})
}

func TestGovernanceMapsRejectDuplicateLogicalKeys(t *testing.T) {
	t.Run("treasury withdrawals", func(t *testing.T) {
		address1 := testRewardAddress(t)
		address2 := address1
		encoded, err := cbor.Encode([]any{
			uint(common.GovActionTypeTreasuryWithdrawal),
			map[*common.Address]uint64{&address1: 1, &address2: 2},
			nil,
		})
		require.NoError(t, err)
		var action common.TreasuryWithdrawalGovAction
		_, err = cbor.Decode(encoded, &action)
		requireDuplicateLogicalMapKey(t, err)

		encoded, err = cbor.Encode([]any{
			uint(common.GovActionTypeTreasuryWithdrawal),
			map[*common.Address]uint64{&address1: 1},
			nil,
		})
		require.NoError(t, err)
		_, err = cbor.Decode(encoded, &action)
		require.NoError(t, err)
	})

	t.Run("committee credential epochs", func(t *testing.T) {
		var hash common.Blake2b224
		hash[0] = 4
		credential1 := common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: hash,
		}
		credential2 := credential1
		quorum := cbor.Rat{Rat: big.NewRat(1, 2)}
		encoded, err := cbor.Encode([]any{
			uint(common.GovActionTypeUpdateCommittee),
			nil,
			[]common.Credential{},
			map[*common.Credential]uint{&credential1: 1, &credential2: 2},
			quorum,
		})
		require.NoError(t, err)
		var action common.UpdateCommitteeGovAction
		_, err = cbor.Decode(encoded, &action)
		requireDuplicateLogicalMapKey(t, err)

		encoded, err = cbor.Encode([]any{
			uint(common.GovActionTypeUpdateCommittee),
			nil,
			[]common.Credential{},
			map[*common.Credential]uint{&credential1: 1},
			quorum,
		})
		require.NoError(t, err)
		_, err = cbor.Decode(encoded, &action)
		require.NoError(t, err)
	})
}

func TestInstantaneousRewardsRejectDuplicateLogicalCredentialKeys(
	t *testing.T,
) {
	var hash common.Blake2b224
	hash[0] = 5
	credential1 := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: hash,
	}
	credential2 := credential1
	encoded, err := cbor.Encode([]any{
		uint(common.MirSourceReserves),
		map[*common.Credential]uint64{&credential1: 1, &credential2: 2},
	})
	require.NoError(t, err)
	var reward common.MoveInstantaneousRewardsCertificateReward
	_, err = cbor.Decode(encoded, &reward)
	requireDuplicateLogicalMapKey(t, err)

	encoded, err = cbor.Encode([]any{
		uint(common.MirSourceReserves),
		map[*common.Credential]uint64{&credential1: 1},
	})
	require.NoError(t, err)
	_, err = cbor.Decode(encoded, &reward)
	require.NoError(t, err)
}
