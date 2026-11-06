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

// Tests for the genesis key delegation predicates of the Shelley DELEG rule
// (GenesisDelegTxCert in delegationTransition,
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Deleg.hs), driven through
// each era's registered delegation rule.

package ledger_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genesisRegistryLedgerState implements GenesisDelegationRegistryState.
type genesisRegistryLedgerState struct {
	common.LedgerState
	current []common.GenesisDelegation
	future  []common.GenesisDelegation
}

func (s genesisRegistryLedgerState) CurrentGenesisDelegations(
	uint64,
) ([]common.GenesisDelegation, error) {
	return s.current, nil
}

func (s genesisRegistryLedgerState) FutureGenesisDelegations(
	uint64,
) ([]common.GenesisDelegation, error) {
	return s.future, nil
}

func genesisDelegation(genesis, delegate, vrf byte) common.GenesisDelegation {
	return common.GenesisDelegation{
		GenesisKeyHash:  common.Blake2b224{genesis},
		DelegateKeyHash: common.Blake2b224{delegate},
		VrfKeyHash:      common.Blake2b256{vrf},
	}
}

func genesisDelegCert(genesis, delegate, vrf byte) common.Certificate {
	d := genesisDelegation(genesis, delegate, vrf)
	return &common.GenesisKeyDelegationCertificate{
		CertType:            uint(common.CertificateTypeGenesisKeyDelegation),
		GenesisHash:         d.GenesisKeyHash[:],
		GenesisDelegateHash: d.DelegateKeyHash[:],
		VrfKeyHash:          d.VrfKeyHash,
	}
}

// newGenesisRegistry maps genesis keys 0x01 and 0x02 to delegates 0x11 and
// 0x12 with VRF keys 0x21 and 0x22; genesis key 0x01 also has a scheduled
// delegation to delegate 0x13 with VRF key 0x23.
func newGenesisRegistry() genesisRegistryLedgerState {
	return genesisRegistryLedgerState{
		LedgerState: mockledger.NewLedgerStateBuilder().Build(),
		current: []common.GenesisDelegation{
			genesisDelegation(0x01, 0x11, 0x21),
			genesisDelegation(0x02, 0x12, 0x22),
		},
		future: []common.GenesisDelegation{
			genesisDelegation(0x01, 0x13, 0x23),
		},
	}
}

func TestDelegGenesisKeyDelegation(t *testing.T) {
	t.Parallel()

	for _, era := range delegEras() {
		t.Run(era.name, func(t *testing.T) {
			t.Parallel()
			ls := newGenesisRegistry()
			validate := func(
				ls common.LedgerState,
				certs ...common.Certificate,
			) error {
				return era.validateDelegation(t, poolCertTx(certs...), ls)
			}

			t.Run("known genesis key with fresh keys", func(t *testing.T) {
				t.Parallel()
				require.NoError(
					t,
					validate(ls, genesisDelegCert(0x01, 0x31, 0x41)),
				)
			})

			t.Run("genesis key not in the mapping", func(t *testing.T) {
				t.Parallel()
				err := validate(ls, genesisDelegCert(0x09, 0x31, 0x41))
				var target shelley.GenesisKeyNotInMappingError
				require.ErrorAs(t, err, &target)
				assert.Equal(t, common.Blake2b224{0x09}, target.GenesisKeyHash)
			})

			t.Run(
				"genesis key only scheduled is not in the mapping",
				func(t *testing.T) {
					t.Parallel()
					state := newGenesisRegistry()
					state.future = append(
						state.future,
						genesisDelegation(0x07, 0x17, 0x27),
					)
					var target shelley.GenesisKeyNotInMappingError
					require.ErrorAs(t, validate(
						state, genesisDelegCert(0x07, 0x31, 0x41),
					), &target)
				},
			)

			t.Run("delegate of another genesis key", func(t *testing.T) {
				t.Parallel()
				err := validate(ls, genesisDelegCert(0x01, 0x12, 0x41))
				var target shelley.DuplicateGenesisDelegateError
				require.ErrorAs(t, err, &target)
				assert.Equal(t, common.Blake2b224{0x12}, target.DelegateKeyHash)
			})

			t.Run(
				"own current delegate is not a duplicate",
				func(t *testing.T) {
					t.Parallel()
					require.NoError(
						t,
						validate(ls, genesisDelegCert(0x01, 0x11, 0x41)),
					)
				},
			)

			t.Run(
				"scheduled delegate of another genesis key",
				func(t *testing.T) {
					t.Parallel()
					var target shelley.DuplicateGenesisDelegateError
					require.ErrorAs(t, validate(
						ls, genesisDelegCert(0x02, 0x13, 0x41),
					), &target)
				},
			)

			t.Run(
				"own scheduled delegate is not a duplicate",
				func(t *testing.T) {
					t.Parallel()
					require.NoError(
						t,
						validate(ls, genesisDelegCert(0x01, 0x13, 0x41)),
					)
				},
			)

			t.Run("VRF key of another genesis key", func(t *testing.T) {
				t.Parallel()
				err := validate(ls, genesisDelegCert(0x01, 0x31, 0x22))
				var target shelley.DuplicateGenesisVRFError
				require.ErrorAs(t, err, &target)
				assert.Equal(t, common.Blake2b256{0x22}, target.VrfKeyHash)
			})

			t.Run("own current VRF key is not a duplicate", func(t *testing.T) {
				t.Parallel()
				require.NoError(
					t,
					validate(ls, genesisDelegCert(0x01, 0x31, 0x21)),
				)
			})

			t.Run(
				"scheduled VRF key of another genesis key",
				func(t *testing.T) {
					t.Parallel()
					var target shelley.DuplicateGenesisVRFError
					require.ErrorAs(t, validate(
						ls, genesisDelegCert(0x02, 0x31, 0x23),
					), &target)
				},
			)

			t.Run(
				"own scheduled VRF key is not a duplicate",
				func(t *testing.T) {
					t.Parallel()
					require.NoError(
						t,
						validate(ls, genesisDelegCert(0x01, 0x31, 0x23)),
					)
				},
			)

			t.Run(
				"a delegate duplicate is reported before a VRF duplicate",
				func(t *testing.T) {
					t.Parallel()
					var target shelley.DuplicateGenesisDelegateError
					require.ErrorAs(t, validate(
						ls, genesisDelegCert(0x01, 0x12, 0x22),
					), &target)
				},
			)

			t.Run(
				"earlier certificate schedules a delegate",
				func(t *testing.T) {
					t.Parallel()
					var target shelley.DuplicateGenesisDelegateError
					require.ErrorAs(t, validate(
						ls,
						genesisDelegCert(0x01, 0x31, 0x41),
						genesisDelegCert(0x02, 0x31, 0x42),
					), &target)
				},
			)

			t.Run(
				"earlier certificate schedules a VRF key",
				func(t *testing.T) {
					t.Parallel()
					var target shelley.DuplicateGenesisVRFError
					require.ErrorAs(t, validate(
						ls,
						genesisDelegCert(0x01, 0x31, 0x41),
						genesisDelegCert(0x02, 0x32, 0x41),
					), &target)
				},
			)

			t.Run(
				"one genesis key may reschedule its own keys",
				func(t *testing.T) {
					t.Parallel()
					require.NoError(t, validate(
						ls,
						genesisDelegCert(0x01, 0x31, 0x41),
						genesisDelegCert(0x01, 0x31, 0x41),
						genesisDelegCert(0x01, 0x32, 0x42),
					))
				},
			)

			t.Run("registry unavailable", func(t *testing.T) {
				t.Parallel()
				var target common.GenesisDelegationRegistryStateUnavailableError
				require.ErrorAs(t, validate(
					mockledger.NewLedgerStateBuilder().Build(),
					genesisDelegCert(0x01, 0x31, 0x41),
				), &target)
			})

			t.Run(
				"registry not read without a genesis delegation",
				func(t *testing.T) {
					t.Parallel()
					require.NoError(t, validate(
						mockledger.NewLedgerStateBuilder().Build(),
					))
				},
			)
		})
	}
}

// TestDelegGenesisKeyDelegationEarlierTransactionsInBlock proves the
// delegations a rule reads include those scheduled by earlier transactions in
// the block.
func TestDelegGenesisKeyDelegationEarlierTransactionsInBlock(t *testing.T) {
	t.Parallel()

	pparams := maryPparams(0)
	block := common.NewBlockLedgerState(newGenesisRegistry())
	require.NoError(t, block.ApplyTransaction(
		poolCertTx(genesisDelegCert(0x01, 0x31, 0x41)),
		pparams,
	))
	validate := func(cert common.Certificate) error {
		return shelley.UtxoValidateDelegation(
			poolCertTx(cert), 0, block, pparams,
		)
	}

	var delegate shelley.DuplicateGenesisDelegateError
	require.ErrorAs(t, validate(genesisDelegCert(0x02, 0x31, 0x42)), &delegate)
	var vrf shelley.DuplicateGenesisVRFError
	require.ErrorAs(t, validate(genesisDelegCert(0x02, 0x32, 0x41)), &vrf)
	require.NoError(t, validate(genesisDelegCert(0x01, 0x31, 0x41)))
	require.NoError(t, validate(genesisDelegCert(0x02, 0x32, 0x42)))
}
