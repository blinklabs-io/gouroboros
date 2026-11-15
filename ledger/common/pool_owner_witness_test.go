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
	"bytes"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// The reference compares StakePoolParams with its derived Eq: owners are a
// set, relays a sequence, and the margin a normalised ratio.
func TestPoolOwnersWitnessRequiredParameterEquality(t *testing.T) {
	t.Parallel()
	pool := common.PoolKeyHash(bytes.Repeat([]byte{0x33}, 28))
	ownerA := common.AddrKeyHash(bytes.Repeat([]byte{0x0a}, 28))
	ownerB := common.AddrKeyHash(bytes.Repeat([]byte{0x0b}, 28))
	port := uint32(3001)
	hostA, hostB := "a.example", "b.example"
	params := func(
		mutate func(*common.PoolRegistrationCertificate),
	) *common.PoolRegistrationCertificate {
		cert := &common.PoolRegistrationCertificate{
			Operator:   pool,
			VrfKeyHash: common.VrfKeyHash{0x01},
			Pledge:     10,
			Cost:       20,
			Margin:     cbor.Rat{Rat: big.NewRat(1, 100)},
			PoolOwners: []common.AddrKeyHash{ownerA, ownerB},
			Relays: []common.PoolRelay{
				{
					Type:     common.PoolRelayTypeSingleHostName,
					Port:     &port,
					Hostname: &hostA,
				},
				{Type: common.PoolRelayTypeMultiHostName, Hostname: &hostB},
			},
			PoolMetadata: &common.PoolMetadata{
				Url:  "https://a.example/pool.json",
				Hash: common.PoolMetadataHash{0x02},
			},
		}
		if mutate != nil {
			mutate(cert)
		}
		return cert
	}
	ls := pendingPoolBase{
		LedgerState: mockledger.NewLedgerStateBuilder().
			WithPoolCurrentState(func(
				common.PoolKeyHash,
			) (*common.PoolRegistrationCertificate, *uint64, error) {
				return params(nil), nil, nil
			}).
			Build(),
	}

	for _, tc := range []struct {
		name     string
		mutate   func(*common.PoolRegistrationCertificate)
		required bool
	}{
		{name: "unchanged"},
		{
			name: "bls key only",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.LeiosKey = &common.LeiosKey{PublicKey: []byte{0x03}}
			},
		},
		{
			name: "owners reordered",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.PoolOwners = []common.AddrKeyHash{ownerB, ownerA}
			},
		},
		{
			name: "margin not in lowest terms",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.Margin = cbor.Rat{Rat: new(big.Rat).SetFrac64(2, 200)}
			},
		},
		{
			name: "owner removed",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.PoolOwners = c.PoolOwners[:1]
			},
			required: true,
		},
		{
			name: "relays reordered",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.Relays[0], c.Relays[1] = c.Relays[1], c.Relays[0]
			},
			required: true,
		},
		{
			name: "relay port removed",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.Relays[0].Port = nil
			},
			required: true,
		},
		{
			name: "metadata removed",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.PoolMetadata = nil
			},
			required: true,
		},
		{
			name: "metadata hash changed",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.PoolMetadata.Hash = common.PoolMetadataHash{0x04}
			},
			required: true,
		},
		{
			name: "vrf key changed",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.VrfKeyHash = common.VrfKeyHash{0x05}
			},
			required: true,
		},
		{
			name: "cost changed",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.Cost++
			},
			required: true,
		},
		{
			name: "margin changed",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.Margin = cbor.Rat{Rat: big.NewRat(1, 50)}
			},
			required: true,
		},
		{
			name: "reward account changed",
			mutate: func(c *common.PoolRegistrationCertificate) {
				c.RewardAccount = ownerB
			},
			required: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(
				t,
				tc.required,
				common.PoolOwnersWitnessRequired(ls, 0, params(tc.mutate)),
			)
		})
	}
}

func TestPoolOwnersWitnessRequiredWithoutLedgerState(t *testing.T) {
	cert := &common.PoolRegistrationCertificate{
		Operator: common.PoolKeyHash{0x01},
	}
	for name, ls := range map[string]common.LedgerState{
		"nil":       nil,
		"typed nil": (*mockledger.MockLedgerState)(nil),
	} {
		t.Run(name, func(t *testing.T) {
			var required bool
			require.NotPanics(t, func() {
				required = common.PoolOwnersWitnessRequired(ls, 0, cert)
			})
			require.True(t, required)
		})
	}
}
