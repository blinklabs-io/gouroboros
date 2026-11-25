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

package ledger_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// A genesis delegate hash is an arbitrary key hash, so it may be the address
// root of a bootstrap witness. The reference counts bootstrap roots among the
// witness key hashes for both the MIR genesis quorum and the update-proposal
// delegates (keyHashWitnessesTxWits in Cardano/Ledger/Core.hs, used by
// validateMIRInsufficientGenesisSigs and validateNeededWitnesses in
// Shelley/Rules/Utxow.hs).
func TestGenesisDelegateWitnessedByBootstrap(t *testing.T) {
	boot := common.BootstrapWitness{
		PublicKey:  make([]byte, 32),
		Signature:  make([]byte, 64),
		ChainCode:  make([]byte, 32),
		Attributes: []byte{0xa0},
	}
	root, err := boot.KeyHash()
	require.NoError(t, err)
	genesisKey := common.Blake2b224Hash([]byte("genesis-key"))
	ls := mockledger.NewLedgerStateBuilder().
		WithGenesisDelegation(
			map[common.Blake2b224]common.Blake2b224{genesisKey: root},
			1,
		).
		WithProtocolParameterUpdateWindow(
			func(uint64) (uint64, uint64, error) { return 0, 1 << 40, nil },
		).
		Build()
	witnesses := shelley.ShelleyTransactionWitnessSet{
		BootstrapWitnesses: []common.BootstrapWitness{boot},
	}
	pp := &shelley.ShelleyProtocolParameters{ProtocolMajor: 2}

	t.Run("MIR quorum", func(t *testing.T) {
		tx := &shelley.ShelleyTransaction{
			Body: shelley.ShelleyTransactionBody{
				TxCertificates: []common.CertificateWrapper{{
					Type: uint(common.CertificateTypeMoveInstantaneousRewards),
					Certificate: &common.MoveInstantaneousRewardsCertificate{
						CertType: uint(
							common.CertificateTypeMoveInstantaneousRewards,
						),
					},
				}},
			},
			WitnessSet: witnesses,
		}
		require.NoError(t, shelley.UtxoValidateMIRGenesisQuorum(tx, 1, ls, pp))
	})

	t.Run("update proposal delegate", func(t *testing.T) {
		tx := &shelley.ShelleyTransaction{
			Body: shelley.ShelleyTransactionBody{
				Update: &shelley.ShelleyTransactionPparamUpdate{
					ProtocolParamUpdates: map[common.Blake2b224]shelley.ShelleyProtocolParameterUpdate{
						genesisKey: {},
					},
				},
			},
			WitnessSet: witnesses,
		}
		require.NoError(
			t,
			shelley.UtxoValidateProtocolParameterUpdates(tx, 1, ls, pp),
		)
	})

	// Alonzo and Babbage check only these witnesses for a phase-2-invalid
	// transaction's update; the reference does not gate
	// validateNeededWitnesses on IsValid, so a bootstrap root counts there
	// too.
	t.Run("update proposal delegate of an invalid transaction", func(t *testing.T) {
		tx := &shelley.ShelleyTransaction{
			Body: shelley.ShelleyTransactionBody{
				Update: &shelley.ShelleyTransactionPparamUpdate{
					ProtocolParamUpdates: map[common.Blake2b224]shelley.ShelleyProtocolParameterUpdate{
						genesisKey: {},
					},
				},
			},
			WitnessSet: witnesses,
		}
		require.NoError(
			t,
			common.ValidateClassicProtocolParameterUpdateWitnesses(tx, 1, ls),
		)
	})
}
