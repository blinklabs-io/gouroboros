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

// Tests for the timing and pot-sufficiency predicates of the Shelley DELEG
// rule for move instantaneous rewards (checkSlotNotTooLate,
// updateReservesAndTreasury and delegationTransition in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Deleg.hs), driven through
// each era's registered delegation rule.

package ledger_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	mirReserves = uint(0)
	mirTreasury = uint(1)
)

// mirPotLedgerState implements every capability the MIR predicates read.
type mirPotLedgerState struct {
	common.LedgerState
	nextEpochFirstSlot uint64
	stabilityWindow    uint64
	totals             map[uint]int64
	transfers          map[uint]int64
	pending            map[[2]uint]*big.Int
}

func (s mirPotLedgerState) NextEpochFirstSlot(uint64) (uint64, error) {
	return s.nextEpochFirstSlot, nil
}

func (s mirPotLedgerState) StabilityWindow() (uint64, error) {
	return s.stabilityWindow, nil
}

func (s mirPotLedgerState) InstantaneousRewardsTotal(
	source uint,
) (*big.Int, error) {
	return big.NewInt(s.totals[source]), nil
}

func (s mirPotLedgerState) InstantaneousPotTransfer(
	source uint,
) (*big.Int, error) {
	return big.NewInt(s.transfers[source]), nil
}

func (s mirPotLedgerState) PendingInstantaneousRewards(
	source uint,
	cred common.Credential,
) (*big.Int, error) {
	return s.pending[[2]uint{source, uint(cred.Credential[0])}], nil
}

// mirWindowOnlyLedgerState lacks InstantaneousRewardsTotalsState.
type mirWindowOnlyLedgerState struct {
	common.LedgerState
}

func (mirWindowOnlyLedgerState) NextEpochFirstSlot(uint64) (uint64, error) {
	return 1000, nil
}

func (mirWindowOnlyLedgerState) StabilityWindow() (uint64, error) {
	return 100, nil
}

// mirTotalsOnlyLedgerState lacks StabilityWindowState.
type mirTotalsOnlyLedgerState struct {
	common.LedgerState
}

func (mirTotalsOnlyLedgerState) InstantaneousRewardsTotal(
	uint,
) (*big.Int, error) {
	return new(big.Int), nil
}

func (mirTotalsOnlyLedgerState) InstantaneousPotTransfer(
	uint,
) (*big.Int, error) {
	return new(big.Int), nil
}

// mirNoPendingLedgerState provides the epoch-end and pot-total capabilities,
// with accumulated in each pot, and no PendingInstantaneousRewardsState.
type mirNoPendingLedgerState struct {
	common.LedgerState
	accumulated int64
}

func (mirNoPendingLedgerState) NextEpochFirstSlot(uint64) (uint64, error) {
	return 1000, nil
}

func (mirNoPendingLedgerState) StabilityWindow() (uint64, error) {
	return 100, nil
}

func (s mirNoPendingLedgerState) InstantaneousRewardsTotal(
	uint,
) (*big.Int, error) {
	return big.NewInt(s.accumulated), nil
}

func (mirNoPendingLedgerState) InstantaneousPotTransfer(
	uint,
) (*big.Int, error) {
	return new(big.Int), nil
}

// newMirFixtureLedger returns a ledger state whose pots hold more than any
// certificate of the delta-sign tests moves.
func newMirFixtureLedger() common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithAdaPots(common.AdaPots{
			Reserves: 1_000_000_000_000,
			Treasury: 1_000_000_000_000,
		}).
		Build()
}

func newMirNoPendingState() common.LedgerState {
	return mirNoPendingLedgerState{LedgerState: newMirFixtureLedger()}
}

// newMirPotState returns a state whose epoch ends at slot 1000 with a
// stability window of 100 slots, so a certificate is accepted below slot 900.
func newMirPotState(reserves, treasury uint64) *mirPotLedgerState {
	return &mirPotLedgerState{
		LedgerState: mockledger.NewLedgerStateBuilder().
			WithAdaPots(common.AdaPots{
				Reserves: reserves,
				Treasury: treasury,
			}).
			Build(),
		nextEpochFirstSlot: 1000,
		stabilityWindow:    100,
	}
}

// allowsPotTransfer reports whether the era's protocol version is at or after
// the Alonzo hard fork (hardforkAlonzoAllowMIRTransfer).
func (e delegEra) allowsPotTransfer() bool {
	return e.name == "Alonzo" || e.name == "Babbage"
}

func mirPay(
	source uint,
	creds map[byte]*big.Int,
) common.Certificate {
	rewards := make(map[*common.Credential]*big.Int, len(creds))
	for b, amount := range creds {
		rewards[mirStakeCredential(b)] = amount
	}
	return mirCert(source, rewards)
}

func mirPayOne(source uint, amount *big.Int) common.Certificate {
	return mirPay(source, map[byte]*big.Int{0xab: amount})
}

func TestDelegMIRCertificateTooLateInEpoch(t *testing.T) {
	t.Parallel()

	for _, era := range delegEras() {
		t.Run(era.name, func(t *testing.T) {
			t.Parallel()
			ls := newMirPotState(1_000_000, 0)
			validate := func(slot uint64) error {
				return era.validateRegisteredRule(
					t,
					common.UtxoValidationRuleDelegation,
					poolCertTx(mirPayOne(mirReserves, big.NewInt(1))),
					slot,
					ls,
				)
			}

			t.Run("last allowed slot", func(t *testing.T) {
				t.Parallel()
				require.NoError(t, validate(899))
			})

			for _, slot := range []uint64{900, 901, 1000, math.MaxUint64} {
				t.Run("rejected slot", func(t *testing.T) {
					t.Parallel()
					var target shelley.MIRCertificateTooLateinEpochError
					require.ErrorAs(t, validate(slot), &target)
					assert.Equal(t, slot, target.Slot)
					assert.Equal(t, uint64(900), target.Cutoff)
				})
			}
		})
	}
}

func TestDelegMIRInsufficientForInstantaneousRewards(t *testing.T) {
	t.Parallel()

	maxU64 := new(big.Int).SetUint64(math.MaxUint64)
	for _, era := range delegEras() {
		t.Run(era.name, func(t *testing.T) {
			t.Parallel()
			validate := func(
				ls common.LedgerState,
				certs ...common.Certificate,
			) error {
				return era.validateRegisteredRule(
					t,
					common.UtxoValidationRuleDelegation,
					poolCertTx(certs...),
					0,
					ls,
				)
			}
			insufficient := func(
				t *testing.T,
				err error,
			) shelley.InsufficientForInstantaneousRewardsError {
				t.Helper()
				var target shelley.InsufficientForInstantaneousRewardsError
				require.ErrorAs(t, err, &target)
				return target
			}

			for _, source := range []uint{mirReserves, mirTreasury} {
				ls := newMirPotState(1000, 1000)
				t.Run("exact pot is accepted", func(t *testing.T) {
					t.Parallel()
					require.NoError(t, validate(
						ls,
						mirPayOne(source, big.NewInt(1000)),
					))
				})
				t.Run("pot plus one is rejected", func(t *testing.T) {
					t.Parallel()
					target := insufficient(t, validate(
						ls,
						mirPayOne(source, big.NewInt(1001)),
					))
					assert.Equal(t, source, target.Source)
					assert.Equal(t, "1001", target.Required.String())
					assert.Equal(t, "1000", target.Available.String())
				})
				t.Run("max uint64 is rejected", func(t *testing.T) {
					t.Parallel()
					target := insufficient(t, validate(
						ls,
						mirPayOne(source, maxU64),
					))
					assert.Equal(t, maxU64.String(), target.Required.String())
				})
				t.Run("credentials are summed", func(t *testing.T) {
					t.Parallel()
					require.NoError(
						t,
						validate(ls, mirPay(source, map[byte]*big.Int{
							0x01: big.NewInt(600),
							0x02: big.NewInt(400),
						})),
					)
					insufficient(
						t,
						validate(ls, mirPay(source, map[byte]*big.Int{
							0x01: big.NewInt(600),
							0x02: big.NewInt(401),
						})),
					)
				})
				t.Run(
					"certificates in a transaction are summed",
					func(t *testing.T) {
						t.Parallel()
						require.NoError(t, validate(
							ls,
							mirPay(
								source,
								map[byte]*big.Int{0x01: big.NewInt(600)},
							),
							mirPay(
								source,
								map[byte]*big.Int{0x02: big.NewInt(400)},
							),
						))
						insufficient(t, validate(
							ls,
							mirPay(
								source,
								map[byte]*big.Int{0x01: big.NewInt(600)},
							),
							mirPay(
								source,
								map[byte]*big.Int{0x02: big.NewInt(401)},
							),
						))
					},
				)
			}

			t.Run("pots are independent", func(t *testing.T) {
				t.Parallel()
				ls := newMirPotState(1000, 1000)
				require.NoError(t, validate(
					ls,
					mirPayOne(mirReserves, big.NewInt(1000)),
					mirPayOne(mirTreasury, big.NewInt(1000)),
				))
			})

			t.Run("sum beyond uint64 is rejected", func(t *testing.T) {
				t.Parallel()
				ls := newMirPotState(math.MaxUint64, 0)
				target := insufficient(t, validate(
					ls,
					mirPay(mirReserves, map[byte]*big.Int{0x01: maxU64}),
					mirPay(mirReserves, map[byte]*big.Int{0x02: maxU64}),
				))
				want := new(big.Int).Mul(maxU64, big.NewInt(2))
				assert.Equal(t, want.String(), target.Required.String())
			})

			t.Run("accumulated rewards count", func(t *testing.T) {
				t.Parallel()
				ls := newMirPotState(1000, 0)
				ls.totals = map[uint]int64{mirReserves: 600}
				require.NoError(t, validate(
					ls,
					mirPay(
						mirReserves,
						map[byte]*big.Int{0x01: big.NewInt(400)},
					),
				))
				target := insufficient(t, validate(
					ls,
					mirPay(
						mirReserves,
						map[byte]*big.Int{0x01: big.NewInt(401)},
					),
				))
				assert.Equal(t, "1001", target.Required.String())
			})
		})
	}
}

// TestDelegMIRPotTransferBounds covers the pot-to-pot target of
// InsufficientForTransferDELEG and how pending transfers move the bound of a
// payment to stake credentials. Both exist only from Alonzo.
func TestDelegMIRPotTransferBounds(t *testing.T) {
	t.Parallel()

	maxU64 := new(big.Int).SetUint64(math.MaxUint64)
	for _, era := range delegEras() {
		if !era.allowsPotTransfer() {
			continue
		}
		t.Run(era.name, func(t *testing.T) {
			t.Parallel()
			validate := func(
				ls common.LedgerState,
				certs ...common.Certificate,
			) error {
				return era.validateRegisteredRule(
					t,
					common.UtxoValidationRuleDelegation,
					poolCertTx(certs...),
					0,
					ls,
				)
			}
			insufficientTransfer := func(
				t *testing.T,
				err error,
			) shelley.InsufficientForTransferError {
				t.Helper()
				var target shelley.InsufficientForTransferError
				require.ErrorAs(t, err, &target)
				return target
			}

			for _, source := range []uint{mirReserves, mirTreasury} {
				ls := newMirPotState(1000, 1000)
				t.Run("exact pot is accepted", func(t *testing.T) {
					t.Parallel()
					require.NoError(t, validate(
						ls,
						mirOppositePotCert(source, 1000),
					))
				})
				t.Run("pot plus one is rejected", func(t *testing.T) {
					t.Parallel()
					target := insufficientTransfer(t, validate(
						ls,
						mirOppositePotCert(source, 1001),
					))
					assert.Equal(t, source, target.Source)
					assert.Equal(t, uint64(1001), target.Amount)
					assert.Equal(t, "1000", target.Available.String())
				})
				t.Run("max uint64 is rejected", func(t *testing.T) {
					t.Parallel()
					insufficientTransfer(t, validate(
						ls,
						mirOppositePotCert(source, math.MaxUint64),
					))
				})
			}

			t.Run("accumulated rewards reduce the pot", func(t *testing.T) {
				t.Parallel()
				ls := newMirPotState(1000, 0)
				ls.totals = map[uint]int64{mirReserves: 400}
				require.NoError(t, validate(
					ls,
					mirOppositePotCert(mirReserves, 600),
				))
				insufficientTransfer(t, validate(
					ls,
					mirOppositePotCert(mirReserves, 601),
				))
			})

			t.Run("transfers in a transaction accumulate", func(t *testing.T) {
				t.Parallel()
				ls := newMirPotState(1000, 0)
				require.NoError(t, validate(
					ls,
					mirOppositePotCert(mirReserves, 600),
					mirOppositePotCert(mirReserves, 400),
				))
				insufficientTransfer(t, validate(
					ls,
					mirOppositePotCert(mirReserves, 600),
					mirOppositePotCert(mirReserves, 401),
				))
			})

			t.Run("a transfer funds the opposite pot", func(t *testing.T) {
				t.Parallel()
				ls := newMirPotState(1000, 0)
				require.NoError(t, validate(
					ls,
					mirOppositePotCert(mirReserves, 600),
					mirPayOne(mirTreasury, big.NewInt(600)),
				))
				var target shelley.InsufficientForInstantaneousRewardsError
				require.ErrorAs(t, validate(
					ls,
					mirOppositePotCert(mirReserves, 600),
					mirPayOne(mirTreasury, big.NewInt(601)),
				), &target)
				assert.Equal(t, mirTreasury, target.Source)
			})

			t.Run("a transfer drains the source pot", func(t *testing.T) {
				t.Parallel()
				ls := newMirPotState(1000, 0)
				var target shelley.InsufficientForInstantaneousRewardsError
				require.ErrorAs(t, validate(
					ls,
					mirOppositePotCert(mirReserves, 600),
					mirPayOne(mirReserves, big.NewInt(401)),
				), &target)
				require.NoError(t, validate(
					ls,
					mirOppositePotCert(mirReserves, 600),
					mirPayOne(mirReserves, big.NewInt(400)),
				))
			})

			t.Run("pending transfers move the bound", func(t *testing.T) {
				t.Parallel()
				in := newMirPotState(1000, 0)
				in.transfers = map[uint]int64{mirReserves: 100}
				require.NoError(t, validate(
					in,
					mirPayOne(mirReserves, big.NewInt(1100)),
				))
				var target shelley.InsufficientForInstantaneousRewardsError
				require.ErrorAs(t, validate(
					in,
					mirPayOne(mirReserves, big.NewInt(1101)),
				), &target)

				out := newMirPotState(1000, 0)
				out.transfers = map[uint]int64{mirReserves: -100}
				require.NoError(t, validate(
					out,
					mirPayOne(mirReserves, big.NewInt(900)),
				))
				require.ErrorAs(t, validate(
					out,
					mirPayOne(mirReserves, big.NewInt(901)),
				), &target)
			})

			t.Run("overdrawn pot rejects any transfer", func(t *testing.T) {
				t.Parallel()
				ls := newMirPotState(1000, 0)
				ls.totals = map[uint]int64{mirReserves: 1500}
				target := insufficientTransfer(t, validate(
					ls,
					mirOppositePotCert(mirReserves, 0),
				))
				assert.Equal(t, "-500", target.Available.String())
			})

			t.Run("max uint64 reserves are not wrapped", func(t *testing.T) {
				t.Parallel()
				ls := newMirPotState(math.MaxUint64, 0)
				ls.transfers = map[uint]int64{mirReserves: math.MaxInt64}
				require.NoError(t, validate(
					ls,
					mirPayOne(mirReserves, maxU64),
				))
			})
		})
	}
}

// TestDelegMIRPreAlonzoReplacesPendingRewards pins the pre-Alonzo union of
// the pending map: a credential in a new certificate replaces, rather than
// adds to, its earlier amount.
func TestDelegMIRPreAlonzoReplacesPendingRewards(t *testing.T) {
	t.Parallel()

	for _, era := range delegEras() {
		t.Run(era.name, func(t *testing.T) {
			t.Parallel()
			ls := newMirPotState(500, 0)
			ls.totals = map[uint]int64{mirReserves: 300}
			ls.pending = map[[2]uint]*big.Int{{0, 0xab}: big.NewInt(300)}
			validate := func(amount int64) error {
				return era.validateRegisteredRule(
					t,
					common.UtxoValidationRuleDelegation,
					poolCertTx(mirPayOne(mirReserves, big.NewInt(amount))),
					0,
					ls,
				)
			}
			if era.allowsPotTransfer() {
				// The amount is added to the pending 300, so 201 more
				// exceeds the 500 pot.
				require.NoError(t, validate(200))
				var target shelley.InsufficientForInstantaneousRewardsError
				require.ErrorAs(t, validate(201), &target)
				return
			}
			require.NoError(t, validate(500))
			var target shelley.InsufficientForInstantaneousRewardsError
			require.ErrorAs(t, validate(501), &target)
			assert.Equal(t, "501", target.Required.String())
		})
	}
}

func TestDelegMIRRequiresStateCapabilities(t *testing.T) {
	t.Parallel()

	for _, era := range delegEras() {
		t.Run(era.name, func(t *testing.T) {
			t.Parallel()
			base := mockledger.NewLedgerStateBuilder().
				WithAdaPots(common.AdaPots{Reserves: 1000}).
				Build()
			validate := func(ls common.LedgerState) error {
				return era.validateRegisteredRule(
					t,
					common.UtxoValidationRuleDelegation,
					poolCertTx(mirPayOne(mirReserves, big.NewInt(1))),
					0,
					ls,
				)
			}

			t.Run("no stability window", func(t *testing.T) {
				t.Parallel()
				var target common.StabilityWindowStateUnavailableError
				require.ErrorAs(t, validate(
					mirTotalsOnlyLedgerState{LedgerState: base},
				), &target)
			})
			t.Run("no instantaneous rewards totals", func(t *testing.T) {
				t.Parallel()
				var target common.InstantaneousRewardsTotalsStateUnavailableError
				require.ErrorAs(t, validate(
					mirWindowOnlyLedgerState{LedgerState: base},
				), &target)
			})
			t.Run("transaction without a MIR certificate", func(t *testing.T) {
				t.Parallel()
				require.NoError(t, era.validateRegisteredRule(
					t,
					common.UtxoValidationRuleDelegation,
					poolCertTx(),
					0,
					base,
				))
			})
		})
	}
}

// TestMIRNegativeTransferIsNotDecodable documents why MIRNegativeTransfer has
// no counterpart rule: the pot-to-pot amount is an unsigned wire integer, so a
// negative amount never reaches validation.
func TestMIRNegativeTransferIsNotDecodable(t *testing.T) {
	t.Parallel()

	// [source 0, -1]
	var reward common.MoveInstantaneousRewardsCertificateReward
	_, err := cbor.Decode([]byte{0x82, 0x00, 0x20}, &reward)
	require.Error(t, err)

	// [source 0, 1] is accepted as a pot-to-pot transfer.
	_, err = cbor.Decode([]byte{0x82, 0x00, 0x01}, &reward)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), reward.OtherPot)
}

// TestDelegMIREarlierTransactionsInBlockCount proves that the totals a rule
// reads include the certificates of earlier transactions in the block.
func TestDelegMIREarlierTransactionsInBlockCount(t *testing.T) {
	t.Parallel()

	validate := func(
		t *testing.T,
		pparams common.ProtocolParameters,
		ls common.LedgerState,
		certs ...common.Certificate,
	) error {
		t.Helper()
		return shelley.UtxoValidateDelegation(
			poolCertTx(certs...), 0, ls, pparams,
		)
	}

	t.Run("rewards accumulate", func(t *testing.T) {
		t.Parallel()
		block := common.NewBlockLedgerState(newMirPotState(1000, 0))
		pparams := alonzoPparams(0)
		require.NoError(t, block.ApplyTransaction(
			poolCertTx(mirPay(mirReserves, map[byte]*big.Int{
				0x01: big.NewInt(400),
			})),
			pparams,
		))
		pay := func(amount int64) common.Certificate {
			return mirPay(mirReserves, map[byte]*big.Int{
				0x02: big.NewInt(amount),
			})
		}
		require.NoError(t, validate(t, pparams, block, pay(600)))
		var target shelley.InsufficientForInstantaneousRewardsError
		require.ErrorAs(t, validate(t, pparams, block, pay(601)), &target)
		assert.Equal(t, "1001", target.Required.String())
	})

	t.Run("pot transfers accumulate", func(t *testing.T) {
		t.Parallel()
		block := common.NewBlockLedgerState(newMirPotState(1000, 0))
		pparams := alonzoPparams(0)
		require.NoError(t, block.ApplyTransaction(
			poolCertTx(mirOppositePotCert(mirReserves, 600)),
			pparams,
		))
		require.NoError(t, validate(
			t, pparams, block, mirOppositePotCert(mirReserves, 400),
		))
		var target shelley.InsufficientForTransferError
		require.ErrorAs(t, validate(
			t, pparams, block, mirOppositePotCert(mirReserves, 401),
		), &target)
		require.NoError(t, validate(
			t, pparams, block, mirPayOne(mirTreasury, big.NewInt(600)),
		))
	})

	t.Run("pre-Alonzo rewards are replaced", func(t *testing.T) {
		t.Parallel()
		block := common.NewBlockLedgerState(newMirPotState(500, 0))
		pparams := maryPparams(0)
		require.NoError(t, block.ApplyTransaction(
			poolCertTx(mirPay(mirReserves, map[byte]*big.Int{
				0xab: big.NewInt(300),
			})),
			pparams,
		))
		pay := func(amount int64) common.Certificate {
			return mirPay(mirReserves, map[byte]*big.Int{
				0xab: big.NewInt(amount),
			})
		}
		require.NoError(t, validate(t, pparams, block, pay(500)))
		var target shelley.InsufficientForInstantaneousRewardsError
		require.ErrorAs(t, validate(t, pparams, block, pay(501)), &target)
	})

	t.Run("pre-Alonzo rewards replace the wrapped state", func(t *testing.T) {
		t.Parallel()
		base := newMirPotState(500, 0)
		base.totals = map[uint]int64{mirReserves: 300}
		base.pending = map[[2]uint]*big.Int{{0, 0xab}: big.NewInt(300)}
		block := common.NewBlockLedgerState(base)
		pparams := maryPparams(0)
		// 0xab's pending 300 becomes 100, so the pot holds 400 more.
		require.NoError(t, block.ApplyTransaction(
			poolCertTx(mirPay(mirReserves, map[byte]*big.Int{
				0xab: big.NewInt(100),
			})),
			pparams,
		))
		pay := func(amount int64) common.Certificate {
			return mirPay(mirReserves, map[byte]*big.Int{
				0xcd: big.NewInt(amount),
			})
		}
		require.NoError(t, validate(t, pparams, block, pay(400)))
		var target shelley.InsufficientForInstantaneousRewardsError
		require.ErrorAs(t, validate(t, pparams, block, pay(401)), &target)
		assert.Equal(t, "501", target.Required.String())
	})
}

// TestDelegMIRPreAlonzoZeroTotalsLeavePendingKnown covers a ledger state
// without PendingInstantaneousRewardsState that reports nothing accumulated
// in the pot: every credential's pending rewards are then zero, so a later
// certificate for a credential an earlier one in the block paid replaces that
// amount.
func TestDelegMIRPreAlonzoZeroTotalsLeavePendingKnown(t *testing.T) {
	t.Parallel()

	const pot = 1_000_000_000_000
	pparams := maryPparams(0)
	validate := func(ls common.LedgerState, certs ...common.Certificate) error {
		return shelley.UtxoValidateDelegation(
			poolCertTx(certs...), 0, ls, pparams,
		)
	}
	pay := func(cred byte, amount int64) common.Certificate {
		return mirPay(mirReserves, map[byte]*big.Int{cred: big.NewInt(amount)})
	}

	t.Run("within a transaction", func(t *testing.T) {
		t.Parallel()
		// Replacement leaves pot in use; addition would need twice that.
		require.NoError(t, validate(
			newMirNoPendingState(), pay(0xab, pot), pay(0xab, pot),
		))
		var target shelley.InsufficientForInstantaneousRewardsError
		require.ErrorAs(t, validate(
			newMirNoPendingState(), pay(0xab, pot), pay(0xab, pot+1),
		), &target)
		require.NoError(t, validate(
			newMirNoPendingState(), pay(0xab, 1), pay(0xcd, 1),
		))
	})

	t.Run("across transactions in a block", func(t *testing.T) {
		t.Parallel()
		block := common.NewBlockLedgerState(newMirNoPendingState())
		require.NoError(t, block.ApplyTransaction(
			poolCertTx(pay(0xab, 300)),
			pparams,
		))
		require.NoError(t, validate(block, pay(0xab, pot)))
		require.NoError(t, validate(block, pay(0xcd, pot-300)))
		var target shelley.InsufficientForInstantaneousRewardsError
		require.ErrorAs(t, validate(block, pay(0xcd, pot-299)), &target)
	})
}
