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

// Tests for the Shelley POOL rule (poolTransition in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Pool.hs). They live in
// package ledger_test rather than shelley_test because the rule's predicates
// are gated on protocol version and have to be exercised with the real
// protocol parameters of the eras that carry those versions.

package ledger_test

import (
	"bytes"
	"errors"
	"reflect"
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
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// epochLedgerState adds the optional EpochState capability to a ledger state.
type epochLedgerState struct {
	common.LedgerState
	epoch uint64
	err   error
}

func (s epochLedgerState) EpochForSlot(uint64) (uint64, error) {
	return s.epoch, s.err
}

var _ common.EpochState = epochLedgerState{}

func poolKeyHash(b byte) common.PoolKeyHash {
	return common.NewBlake2b224(bytes.Repeat([]byte{b}, common.Blake2b224Size))
}

func vrfKeyHash(b byte) common.VrfKeyHash {
	return common.NewBlake2b256(bytes.Repeat([]byte{b}, common.Blake2b256Size))
}

// maryPparams returns Mary protocol parameters (major version 4) with the given
// minPoolCost.
func maryPparams(minPoolCost uint64) *mary.MaryProtocolParameters {
	pparams := mockledger.NewMockMaryProtocolParams()
	pparams.MinPoolCost = minPoolCost
	return &pparams
}

// alonzoPparams returns Alonzo protocol parameters (major version 5, the first
// version at which the reward account network id is validated).
func alonzoPparams(minPoolCost uint64) *alonzo.AlonzoProtocolParameters {
	pparams := mockledger.NewMockAlonzoProtocolParams()
	pparams.MinPoolCost = minPoolCost
	return &pparams
}

// conwayPparams returns Conway protocol parameters with the given major version
// and minPoolCost.
func conwayPparams(
	major uint,
	minPoolCost uint64,
) *conway.ConwayProtocolParameters {
	pparams := mockledger.NewMockConwayProtocolParams()
	pparams.ProtocolVersion.Major = major
	pparams.MinPoolCost = minPoolCost
	return &pparams
}

// poolRegCertWire builds a pool registration certificate by encoding the wire
// form and decoding it, so that the reward account keeps its address header
// byte. rewardAccountNetworkId is the low nibble of that header.
func poolRegCertWire(
	t *testing.T,
	operator common.PoolKeyHash,
	vrf common.VrfKeyHash,
	cost uint64,
	rewardAccountNetworkId byte,
) *common.PoolRegistrationCertificate {
	t.Helper()
	rewardAccount := make([]byte, common.Blake2b224Size+1)
	// 0xe0 is the reward-address header for a key-hash stake credential.
	rewardAccount[0] = 0xe0 | (rewardAccountNetworkId & 0x0F)
	for i := 1; i < len(rewardAccount); i++ {
		rewardAccount[i] = 0x07
	}
	wire, err := cbor.Encode([]any{
		uint(common.CertificateTypePoolRegistration),
		operator,
		vrf,
		uint64(1_000_000),
		cost,
		common.NewGenesisRat(0, 1),
		rewardAccount,
		[]common.AddrKeyHash{poolKeyHash(0x09)},
		[]common.PoolRelay{},
		nil,
	})
	require.NoError(t, err)
	cert := &common.PoolRegistrationCertificate{}
	require.NoError(t, cert.UnmarshalCBOR(wire))
	require.Equal(t, operator, cert.Operator)
	require.Equal(t, cost, cert.Cost)
	netId, known := cert.RewardAccountNetworkId()
	require.True(t, known)
	require.Equal(t, uint(rewardAccountNetworkId&0x0F), netId)
	return cert
}

func poolRetirementCert(
	poolKeyHash common.PoolKeyHash,
	epoch uint64,
) *common.PoolRetirementCertificate {
	return &common.PoolRetirementCertificate{
		CertType:    uint(common.CertificateTypePoolRetirement),
		PoolKeyHash: poolKeyHash,
		Epoch:       epoch,
	}
}

func poolCertTx(certs ...common.Certificate) *shelley.ShelleyTransaction {
	wrapped := make([]common.CertificateWrapper, 0, len(certs))
	for _, cert := range certs {
		wrapped = append(
			wrapped,
			common.CertificateWrapper{
				Type:        cert.Type(),
				Certificate: cert,
			},
		)
	}
	return &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
			TxCertificates: wrapped,
		},
	}
}

// TestUtxoValidatePoolCertificatesCostTooLow covers StakePoolCostTooLowPOOL.
func TestUtxoValidatePoolCertificatesCostTooLow(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		Build()
	pparams := maryPparams(340_000_000)

	t.Run("cost below minimum is rejected", func(t *testing.T) {
		cert := poolRegCertWire(
			t,
			poolKeyHash(0x01),
			vrfKeyHash(0x02),
			339_999_999,
			common.AddressNetworkMainnet,
		)
		err := shelley.UtxoValidatePoolCertificates(
			poolCertTx(cert),
			0,
			ls,
			pparams,
		)
		var target shelley.StakePoolCostTooLowError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, poolKeyHash(0x01), target.PoolKeyHash)
		assert.Equal(t, uint64(339_999_999), target.Supplied)
		assert.Equal(t, uint64(340_000_000), target.Min)
	})

	accepted := map[string]uint64{
		"cost equal to minimum": 340_000_000,
		"cost above minimum":    500_000_000,
	}
	for name, cost := range accepted {
		t.Run(name+" is accepted", func(t *testing.T) {
			cert := poolRegCertWire(
				t,
				poolKeyHash(0x01),
				vrfKeyHash(0x02),
				cost,
				common.AddressNetworkMainnet,
			)
			require.NoError(t, shelley.UtxoValidatePoolCertificates(
				poolCertTx(cert),
				0,
				ls,
				pparams,
			))
		})
	}

	t.Run("zero minimum accepts a zero cost", func(t *testing.T) {
		cert := poolRegCertWire(
			t,
			poolKeyHash(0x01),
			vrfKeyHash(0x02),
			0,
			common.AddressNetworkMainnet,
		)
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(cert),
			0,
			ls,
			maryPparams(0),
		))
	})

	t.Run("shelley reports no minimum", func(t *testing.T) {
		// ShelleyProtocolParameters carries no minPoolCost entry in this
		// repository, so the floor is zero and the predicate accepts any
		// cost for Shelley and Allegra blocks.
		shelleyPparams := mockledger.NewMockShelleyProtocolParams()
		cert := poolRegCertWire(
			t,
			poolKeyHash(0x01),
			vrfKeyHash(0x02),
			0,
			common.AddressNetworkMainnet,
		)
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(cert),
			0,
			ls,
			&shelleyPparams,
		))
	})
}

// TestUtxoValidatePoolCertificatesWrongNetwork covers WrongNetworkPOOL and its
// hardforkAlonzoValidatePoolAccountAddressNetID gate.
func TestUtxoValidatePoolCertificatesWrongNetwork(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		Build()
	mismatched := func(t *testing.T) *common.PoolRegistrationCertificate {
		return poolRegCertWire(
			t,
			poolKeyHash(0x01),
			vrfKeyHash(0x02),
			340_000_000,
			common.AddressNetworkTestnet,
		)
	}

	t.Run("rejected at major version 5", func(t *testing.T) {
		err := shelley.UtxoValidatePoolCertificates(
			poolCertTx(mismatched(t)),
			0,
			ls,
			alonzoPparams(340_000_000),
		)
		var target shelley.WrongNetworkPoolError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, poolKeyHash(0x01), target.PoolKeyHash)
		assert.Equal(t, uint(common.AddressNetworkTestnet), target.Supplied)
		assert.Equal(t, uint(common.AddressNetworkMainnet), target.Expected)
	})

	t.Run("not checked at major version 4", func(t *testing.T) {
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(mismatched(t)),
			0,
			ls,
			maryPparams(340_000_000),
		))
	})

	t.Run("matching network is accepted", func(t *testing.T) {
		cert := poolRegCertWire(
			t,
			poolKeyHash(0x01),
			vrfKeyHash(0x02),
			340_000_000,
			common.AddressNetworkMainnet,
		)
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(cert),
			0,
			ls,
			conwayPparams(common.ProtocolVersionConway, 340_000_000),
		))
	})

	t.Run("no header byte skips the check", func(t *testing.T) {
		// A certificate that was not decoded from a wire reward account
		// carries no network id, so the check has nothing to compare.
		cert := &common.PoolRegistrationCertificate{
			CertType:      uint(common.CertificateTypePoolRegistration),
			Operator:      poolKeyHash(0x01),
			VrfKeyHash:    vrfKeyHash(0x02),
			Cost:          340_000_000,
			Margin:        common.NewGenesisRat(0, 1),
			RewardAccount: poolKeyHash(0x07),
		}
		_, known := cert.RewardAccountNetworkId()
		require.False(t, known)
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(cert),
			0,
			ls,
			conwayPparams(common.ProtocolVersionConway, 340_000_000),
		))
	})
}

// TestUtxoValidatePoolCertificatesVrfKeyHash covers
// VRFKeyHashAlreadyRegistered and its hardforkConwayDisallowDuplicatedVRFKeys
// gate.
func TestUtxoValidatePoolCertificatesVrfKeyHash(t *testing.T) {
	registeringPool := poolKeyHash(0x01)
	otherPool := poolKeyHash(0x02)
	sharedVrf := vrfKeyHash(0x03)
	pparams := conwayPparams(common.ProtocolVersionVanRossem, 0)

	inUseBy := func(owner common.PoolKeyHash) *mockledger.MockLedgerState {
		return mockledger.NewLedgerStateBuilder().
			WithNetworkId(common.AddressNetworkMainnet).
			WithVrfKeyInUseFunc(
				func(
					hash common.Blake2b256,
				) (bool, common.PoolKeyHash, error) {
					if hash == sharedVrf {
						return true, owner, nil
					}
					return false, common.PoolKeyHash{}, nil
				},
			).
			Build()
	}

	t.Run("another pool's VRF key hash is rejected", func(t *testing.T) {
		cert := poolRegCertWire(
			t,
			registeringPool,
			sharedVrf,
			0,
			common.AddressNetworkMainnet,
		)
		err := shelley.UtxoValidatePoolCertificates(
			poolCertTx(cert),
			0,
			inUseBy(otherPool),
			pparams,
		)
		var target shelley.VrfKeyHashAlreadyRegisteredError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, registeringPool, target.PoolKeyHash)
		assert.Equal(t, sharedVrf, target.VrfKeyHash)
		assert.Equal(t, otherPool, target.RegisteredBy)
	})

	t.Run("not checked at major version 10", func(t *testing.T) {
		cert := poolRegCertWire(
			t,
			registeringPool,
			sharedVrf,
			0,
			common.AddressNetworkMainnet,
		)
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(cert),
			0,
			inUseBy(otherPool),
			conwayPparams(common.ProtocolVersionPlomin, 0),
		))
	})

	t.Run("re-registering with its own VRF key hash", func(t *testing.T) {
		cert := poolRegCertWire(
			t,
			registeringPool,
			sharedVrf,
			0,
			common.AddressNetworkMainnet,
		)
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(cert),
			0,
			inUseBy(registeringPool),
			pparams,
		))
	})

	t.Run("unused VRF key hash is accepted", func(t *testing.T) {
		cert := poolRegCertWire(
			t,
			registeringPool,
			vrfKeyHash(0x04),
			0,
			common.AddressNetworkMainnet,
		)
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(cert),
			0,
			inUseBy(otherPool),
			pparams,
		))
	})

	t.Run("two pools claiming one VRF key hash in one tx", func(t *testing.T) {
		first := poolRegCertWire(
			t,
			registeringPool,
			sharedVrf,
			0,
			common.AddressNetworkMainnet,
		)
		second := poolRegCertWire(
			t,
			otherPool,
			sharedVrf,
			0,
			common.AddressNetworkMainnet,
		)
		ls := mockledger.NewLedgerStateBuilder().
			WithNetworkId(common.AddressNetworkMainnet).
			Build()
		err := shelley.UtxoValidatePoolCertificates(
			poolCertTx(first, second),
			0,
			ls,
			pparams,
		)
		var target shelley.VrfKeyHashAlreadyRegisteredError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, otherPool, target.PoolKeyHash)
		assert.Equal(t, registeringPool, target.RegisteredBy)
	})

	t.Run("one pool re-registering twice in one tx", func(t *testing.T) {
		first := poolRegCertWire(
			t,
			registeringPool,
			sharedVrf,
			0,
			common.AddressNetworkMainnet,
		)
		second := poolRegCertWire(
			t,
			registeringPool,
			sharedVrf,
			0,
			common.AddressNetworkMainnet,
		)
		ls := mockledger.NewLedgerStateBuilder().
			WithNetworkId(common.AddressNetworkMainnet).
			Build()
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(first, second),
			0,
			ls,
			pparams,
		))
	})

	t.Run("VRF lookup error propagates", func(t *testing.T) {
		wantErr := errors.New("vrf lookup failed")
		ls := mockledger.NewLedgerStateBuilder().
			WithNetworkId(common.AddressNetworkMainnet).
			WithVrfKeyInUseFunc(
				func(
					common.Blake2b256,
				) (bool, common.PoolKeyHash, error) {
					return false, common.PoolKeyHash{}, wantErr
				},
			).
			Build()
		cert := poolRegCertWire(
			t,
			registeringPool,
			sharedVrf,
			0,
			common.AddressNetworkMainnet,
		)
		err := shelley.UtxoValidatePoolCertificates(
			poolCertTx(cert),
			0,
			ls,
			pparams,
		)
		require.ErrorIs(t, err, wantErr)
	})
}

// registeredPoolLedgerState returns a ledger state in which pool 0x01 is
// registered.
func registeredPoolLedgerState() (
	common.PoolKeyHash,
	*mockledger.MockLedgerState,
) {
	registered := poolKeyHash(0x01)
	cert := common.PoolRegistrationCertificate{
		CertType: uint(common.CertificateTypePoolRegistration),
		Operator: registered,
		Margin:   common.NewGenesisRat(0, 1),
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		WithPools([]*common.PoolRegistrationCertificate{&cert}).
		Build()
	return registered, ls
}

// TestUtxoValidatePoolCertificatesRetirementRegistered covers
// StakePoolNotRegisteredOnKeyPOOL.
func TestUtxoValidatePoolCertificatesRetirementRegistered(t *testing.T) {
	registered, ls := registeredPoolLedgerState()
	unregistered := poolKeyHash(0x02)
	pparams := conwayPparams(common.ProtocolVersionConway, 0)

	t.Run("retiring an unregistered pool is rejected", func(t *testing.T) {
		err := shelley.UtxoValidatePoolCertificates(
			poolCertTx(poolRetirementCert(unregistered, 5)),
			0,
			ls,
			pparams,
		)
		var target shelley.StakePoolNotRegisteredOnKeyError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, unregistered, target.PoolKeyHash)
	})

	t.Run("retiring a registered pool is accepted", func(t *testing.T) {
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(poolRetirementCert(registered, 5)),
			0,
			ls,
			pparams,
		))
	})

	t.Run("registered earlier in the same tx", func(t *testing.T) {
		reg := poolRegCertWire(
			t,
			unregistered,
			vrfKeyHash(0x03),
			0,
			common.AddressNetworkMainnet,
		)
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(reg, poolRetirementCert(unregistered, 5)),
			0,
			ls,
			pparams,
		))
	})

	t.Run("retirement does not unregister within the tx", func(t *testing.T) {
		// poolTransition's RetirePool branch only inserts into psRetiring;
		// the pool stays in psStakePools until POOLREAP.
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(
				poolRetirementCert(registered, 5),
				poolRetirementCert(registered, 6),
			),
			0,
			ls,
			pparams,
		))
	})
}

// TestUtxoValidatePoolCertificatesRetirementEpoch covers
// StakePoolRetirementWrongEpochPOOL and the degrading EpochState capability.
func TestUtxoValidatePoolCertificatesRetirementEpoch(t *testing.T) {
	registered, base := registeredPoolLedgerState()
	pparams := conwayPparams(common.ProtocolVersionConway, 0)
	// The Conway mock protocol parameters use the mainnet eMax of 18.
	require.Equal(t, uint(18), pparams.MaxEpoch)
	const currentEpoch = uint64(100)
	const limitEpoch = currentEpoch + 18
	withEpoch := epochLedgerState{LedgerState: base, epoch: currentEpoch}

	accepted := map[string]uint64{
		"first allowed epoch": currentEpoch + 1,
		"mid range":           currentEpoch + 9,
		"last allowed epoch":  limitEpoch,
	}
	for name, epoch := range accepted {
		t.Run(name+" is accepted", func(t *testing.T) {
			require.NoError(t, shelley.UtxoValidatePoolCertificates(
				poolCertTx(poolRetirementCert(registered, epoch)),
				0,
				withEpoch,
				pparams,
			))
		})
	}

	rejected := map[string]uint64{
		"current epoch":      currentEpoch,
		"past epoch":         currentEpoch - 1,
		"epoch zero":         0,
		"one beyond eMax":    limitEpoch + 1,
		"far beyond eMax":    currentEpoch + 1_000,
		"maximum uint epoch": ^uint64(0),
	}
	for name, epoch := range rejected {
		t.Run(name+" is rejected", func(t *testing.T) {
			err := shelley.UtxoValidatePoolCertificates(
				poolCertTx(poolRetirementCert(registered, epoch)),
				0,
				withEpoch,
				pparams,
			)
			var target shelley.StakePoolRetirementWrongEpochError
			require.ErrorAs(t, err, &target)
			assert.Equal(t, registered, target.PoolKeyHash)
			assert.Equal(t, epoch, target.Supplied)
			assert.Equal(t, currentEpoch, target.CurrentEpoch)
			assert.Equal(t, uint64(limitEpoch), target.LimitEpoch)
		})
	}

	t.Run("no EpochState skips the bound", func(t *testing.T) {
		// base does not implement EpochState, so an otherwise invalid
		// retirement epoch must still be accepted rather than failing
		// closed.
		_, ok := any(base).(common.EpochState)
		require.False(t, ok)
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(poolRetirementCert(registered, currentEpoch)),
			0,
			base,
			pparams,
		))
	})

	t.Run("EpochState error propagates", func(t *testing.T) {
		wantErr := errors.New("epoch lookup failed")
		err := shelley.UtxoValidatePoolCertificates(
			poolCertTx(poolRetirementCert(registered, currentEpoch+1)),
			0,
			epochLedgerState{LedgerState: base, err: wantErr},
			pparams,
		)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("saturating eMax leaves the bound vacuous", func(t *testing.T) {
		saturating := conwayPparams(common.ProtocolVersionConway, 0)
		saturating.MaxEpoch = ^uint(0)
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(poolRetirementCert(registered, ^uint64(0))),
			0,
			withEpoch,
			saturating,
		))
	})
}

// TestUtxoValidatePoolCertificatesNoCertificates confirms the rule is inert for
// transactions without pool certificates.
func TestUtxoValidatePoolCertificatesNoCertificates(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	pparams := conwayPparams(common.ProtocolVersionConway, 0)
	stakeCert := &common.StakeRegistrationCertificate{
		CertType:        uint(common.CertificateTypeStakeRegistration),
		StakeCredential: common.Credential{},
	}

	t.Run("no certificates at all", func(t *testing.T) {
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(),
			0,
			ls,
			pparams,
		))
	})

	t.Run("only non-pool certificates", func(t *testing.T) {
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(stakeCert),
			0,
			ls,
			pparams,
		))
	})

	t.Run("unusable pparams without pool certificates", func(t *testing.T) {
		// A transaction the POOL transition would never see must not be
		// rejected for protocol parameters the rule cannot read.
		require.NoError(t, shelley.UtxoValidatePoolCertificates(
			poolCertTx(stakeCert),
			0,
			ls,
			nil,
		))
	})
}

// TestUtxoValidatePoolCertificatesWrongPparams confirms the rule rejects a
// protocol parameter set that cannot answer the POOL queries, rather than
// silently skipping every predicate.
func TestUtxoValidatePoolCertificatesWrongPparams(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	cert := poolRegCertWire(
		t,
		poolKeyHash(0x01),
		vrfKeyHash(0x02),
		0,
		common.AddressNetworkTestnet,
	)
	err := shelley.UtxoValidatePoolCertificates(
		poolCertTx(cert),
		0,
		ls,
		nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pparams are not expected type")
}

// TestPoolRuleInEveryEraRuleSet proves the Shelley POOL rule is reachable from
// the production rule set of every era that inherits it. Rules/Pool.hs in
// eras/allegra, eras/mary, eras/alonzo, eras/babbage, eras/conway and
// eras/dijkstra all reuse Shelley.poolTransition, so none of them may omit it.
func TestPoolRuleInEveryEraRuleSet(t *testing.T) {
	eras := []struct {
		name  string
		rules []common.UtxoValidationRuleFunc
		want  common.UtxoValidationRuleFunc
	}{
		{
			"shelley",
			shelley.UtxoValidationRules,
			shelley.UtxoValidatePoolCertificates,
		},
		{
			"allegra",
			allegra.UtxoValidationRules,
			allegra.UtxoValidatePoolCertificates,
		},
		{
			"mary",
			mary.UtxoValidationRules,
			mary.UtxoValidatePoolCertificates,
		},
		{
			"alonzo",
			alonzo.UtxoValidationRules,
			alonzo.UtxoValidatePoolCertificates,
		},
		{
			"babbage",
			babbage.UtxoValidationRules,
			babbage.UtxoValidatePoolCertificates,
		},
		{
			"conway",
			conway.UtxoValidationRules,
			conway.UtxoValidatePoolCertificates,
		},
		{
			"dijkstra",
			dijkstra.UtxoValidationRules,
			conway.UtxoValidatePoolCertificates,
		},
	}
	for _, era := range eras {
		t.Run(era.name, func(t *testing.T) {
			want := reflect.ValueOf(era.want).Pointer()
			found := false
			for _, rule := range era.rules {
				if reflect.ValueOf(rule).Pointer() == want {
					found = true
					break
				}
			}
			assert.True(
				t,
				found,
				"%s UtxoValidationRules is missing the POOL rule",
				era.name,
			)
		})
	}
}

// TestPoolRulePparamsWiring proves every era's protocol parameter type answers
// the POOL rule's parameter queries from its own fields. A type that failed to
// implement the interface would make the rule return "pparams are not expected
// type" for that era's blocks.
func TestPoolRulePparamsWiring(t *testing.T) {
	tests := []struct {
		name            string
		pparams         common.ProtocolParameters
		wantMajor       uint
		wantMinPoolCost uint64
		wantMaxEpoch    uint64
	}{
		{
			// Shelley and Allegra share ShelleyProtocolParameters, which
			// this repository models without a minPoolCost entry, so the
			// cost floor is reported as zero for those eras.
			name: "shelley",
			pparams: &shelley.ShelleyProtocolParameters{
				ProtocolMajor: common.ProtocolVersionShelley,
				MaxEpoch:      18,
			},
			wantMajor:       common.ProtocolVersionShelley,
			wantMinPoolCost: 0,
			wantMaxEpoch:    18,
		},
		{
			name: "allegra",
			pparams: &allegra.AllegraProtocolParameters{
				ProtocolMajor: common.ProtocolVersionAllegra,
				MaxEpoch:      18,
			},
			wantMajor:       common.ProtocolVersionAllegra,
			wantMinPoolCost: 0,
			wantMaxEpoch:    18,
		},
		{
			name: "mary",
			pparams: &mary.MaryProtocolParameters{
				ProtocolMajor: common.ProtocolVersionMary,
				MaxEpoch:      18,
				MinPoolCost:   340_000_000,
			},
			wantMajor:       common.ProtocolVersionMary,
			wantMinPoolCost: 340_000_000,
			wantMaxEpoch:    18,
		},
		{
			name: "alonzo",
			pparams: &alonzo.AlonzoProtocolParameters{
				ProtocolMajor: common.ProtocolVersionAlonzo,
				MaxEpoch:      18,
				MinPoolCost:   340_000_000,
			},
			wantMajor:       common.ProtocolVersionAlonzo,
			wantMinPoolCost: 340_000_000,
			wantMaxEpoch:    18,
		},
		{
			name: "babbage",
			pparams: &babbage.BabbageProtocolParameters{
				ProtocolMajor: common.ProtocolVersionBabbage,
				MaxEpoch:      18,
				MinPoolCost:   340_000_000,
			},
			wantMajor:       common.ProtocolVersionBabbage,
			wantMinPoolCost: 340_000_000,
			wantMaxEpoch:    18,
		},
		{
			name: "conway",
			pparams: &conway.ConwayProtocolParameters{
				ProtocolVersion: common.ProtocolParametersProtocolVersion{
					Major: common.ProtocolVersionPlomin,
				},
				MaxEpoch:    18,
				MinPoolCost: 340_000_000,
			},
			wantMajor:       common.ProtocolVersionPlomin,
			wantMinPoolCost: 340_000_000,
			wantMaxEpoch:    18,
		},
		{
			name: "dijkstra",
			pparams: &dijkstra.DijkstraProtocolParameters{
				ConwayProtocolParameters: conway.ConwayProtocolParameters{
					ProtocolVersion: common.ProtocolParametersProtocolVersion{
						Major: common.ProtocolVersionDijkstra,
					},
					MaxEpoch:    18,
					MinPoolCost: 340_000_000,
				},
			},
			wantMajor:       common.ProtocolVersionDijkstra,
			wantMinPoolCost: 340_000_000,
			wantMaxEpoch:    18,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			poolPparams, ok := test.pparams.(common.PoolRuleProtocolParameters)
			require.True(
				t,
				ok,
				"%s protocol parameters do not implement PoolRuleProtocolParameters",
				test.name,
			)
			assert.Equal(
				t,
				test.wantMajor,
				poolPparams.ProtocolMajorVersion(),
			)
			assert.Equal(
				t,
				test.wantMinPoolCost,
				poolPparams.MinPoolCostValue(),
			)
			assert.Equal(
				t,
				test.wantMaxEpoch,
				poolPparams.PoolRetirementMaxEpoch(),
			)
		})
	}
}
