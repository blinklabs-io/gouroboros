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

package common

import (
	"bytes"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPoolRegistrationCertificate(
	margin GenesisRat,
) PoolRegistrationCertificate {
	return PoolRegistrationCertificate{
		CertType: uint(CertificateTypePoolRegistration),
		Operator: NewBlake2b224(
			bytes.Repeat([]byte{0x01}, Blake2b224Size),
		),
		VrfKeyHash: NewBlake2b256(
			bytes.Repeat([]byte{0x02}, Blake2b256Size),
		),
		Pledge: 1_000_000,
		Cost:   340_000_000,
		Margin: margin,
		RewardAccount: NewBlake2b224(
			bytes.Repeat([]byte{0x03}, Blake2b224Size),
		),
		PoolOwners: []AddrKeyHash{
			NewBlake2b224(bytes.Repeat([]byte{0x04}, Blake2b224Size)),
		},
		Relays: []PoolRelay{},
		PoolMetadata: &PoolMetadata{
			Url: "https://example.com/pool.json",
			Hash: NewBlake2b256(
				bytes.Repeat([]byte{0x05}, Blake2b256Size),
			),
		},
	}
}

func testPoolRegistrationWire(
	t *testing.T,
	margin cbor.RawMessage,
	leios bool,
) []byte {
	t.Helper()
	cert := testPoolRegistrationCertificate(NewGenesisRat(0, 1))
	fields := []any{
		cert.CertType,
		cert.Operator,
		cert.VrfKeyHash,
		cert.Pledge,
		cert.Cost,
		margin,
		cert.RewardAccount,
		cert.PoolOwners,
		cert.Relays,
		cert.PoolMetadata,
	}
	if leios {
		fields = append(fields[:3], append([]any{nil}, fields[3:]...)...)
	}
	wire, err := cbor.Encode(fields)
	require.NoError(t, err)
	return wire
}

func TestPoolRegistrationCertificateRejectsInvalidMarginCBOR(t *testing.T) {
	tests := []struct {
		name   string
		margin cbor.RawMessage
		valid  bool
	}{
		{
			name:   "zero",
			margin: cbor.RawMessage{0xd8, 0x1e, 0x82, 0x00, 0x01},
			valid:  true,
		},
		{
			name:   "one",
			margin: cbor.RawMessage{0xd8, 0x1e, 0x82, 0x01, 0x01},
			valid:  true,
		},
		{
			name:   "negative",
			margin: cbor.RawMessage{0xd8, 0x1e, 0x82, 0x20, 0x01},
		},
		{
			name:   "above one",
			margin: cbor.RawMessage{0xd8, 0x1e, 0x82, 0x02, 0x01},
		},
		{
			name:   "zero denominator",
			margin: cbor.RawMessage{0xd8, 0x1e, 0x82, 0x01, 0x00},
		},
		{
			name:   "negative denominator",
			margin: cbor.RawMessage{0xd8, 0x1e, 0x82, 0x01, 0x20},
		},
		{
			name:   "double negative",
			margin: cbor.RawMessage{0xd8, 0x1e, 0x82, 0x20, 0x21},
		},
		{
			name:   "wrong tag",
			margin: cbor.RawMessage{0xd8, 0x1d, 0x82, 0x00, 0x01},
		},
		{
			name:   "extra component",
			margin: cbor.RawMessage{0xd8, 0x1e, 0x83, 0x00, 0x01, 0x01},
		},
	}

	for _, leios := range []bool{false, true} {
		shape := "legacy"
		if leios {
			shape = "leios"
		}
		for _, test := range tests {
			t.Run(shape+"/"+test.name, func(t *testing.T) {
				wire := testPoolRegistrationWire(t, test.margin, leios)
				var wrapper CertificateWrapper
				_, err := cbor.Decode(wire, &wrapper)
				if test.valid {
					require.NoError(t, err)
					require.IsType(
						t,
						&PoolRegistrationCertificate{},
						wrapper.Certificate,
					)
					return
				}
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrPoolMarginOutsideUnitInterval)
			})
		}
	}
}

func TestPoolRegistrationCertificateRejectsInvalidMarginJSON(t *testing.T) {
	tests := []struct {
		name   string
		margin string
		valid  bool
	}{
		{name: "zero", margin: `{"numerator":0,"denominator":1}`, valid: true},
		{name: "one", margin: `{"numerator":1,"denominator":1}`, valid: true},
		{name: "negative", margin: `{"numerator":-1,"denominator":1}`},
		{name: "above one", margin: `{"numerator":2,"denominator":1}`},
		{name: "zero denominator", margin: `{"numerator":1,"denominator":0}`},
		{
			name:   "negative denominator",
			margin: `{"numerator":1,"denominator":-1}`,
		},
		{name: "double negative", margin: `{"numerator":-1,"denominator":-2}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var cert PoolRegistrationCertificate
			err := json.Unmarshal(
				[]byte(`{"margin":`+test.margin+`}`),
				&cert,
			)
			if test.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPoolMarginOutsideUnitInterval)
		})
	}
}

func TestPoolRegistrationCertificateConstructedMarginBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		margin GenesisRat
		valid  bool
	}{
		{name: "zero", margin: NewGenesisRat(0, 1), valid: true},
		{name: "one", margin: NewGenesisRat(1, 1), valid: true},
		{name: "negative", margin: NewGenesisRat(-1, 1)},
		{name: "above one", margin: NewGenesisRat(2, 1)},
		{name: "missing", margin: GenesisRat{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cert := testPoolRegistrationCertificate(test.margin)
			wire, err := cbor.Encode(cert)
			if test.valid {
				require.NoError(t, err)
				var decoded PoolRegistrationCertificate
				_, err = cbor.Decode(wire, &decoded)
				require.NoError(t, err)
				assert.Zero(t, decoded.Margin.Cmp(test.margin.Rat))
				_, err = cert.Utxorpc()
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPoolMarginOutsideUnitInterval)
			_, err = cert.Utxorpc()
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPoolMarginOutsideUnitInterval)
		})
	}
}

func TestCalculateRewardsRejectsInvalidPoolMargins(t *testing.T) {
	poolID := PoolKeyHash{0x01}
	owner := AddrKeyHash{0x02}
	delegator := AddrKeyHash{0x03}
	tests := []struct {
		name   string
		margin GenesisRat
		valid  bool
	}{
		{name: "zero", margin: NewGenesisRat(0, 1), valid: true},
		{name: "one", margin: NewGenesisRat(1, 1), valid: true},
		{name: "negative", margin: NewGenesisRat(-1, 1)},
		{name: "above one", margin: NewGenesisRat(2, 1)},
		{name: "missing", margin: GenesisRat{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := CalculateRewards(
				AdaPots{Rewards: 1_000_000_000},
				RewardSnapshot{
					TotalActiveStake: 1_000_000_000,
					PoolStake: map[PoolKeyHash]uint64{
						poolID: 1_000_000_000,
					},
					DelegatorStake: map[PoolKeyHash]map[AddrKeyHash]uint64{
						poolID: {
							owner:     100_000_000,
							delegator: 900_000_000,
						},
					},
					PoolParams: map[PoolKeyHash]*PoolRegistrationCertificate{
						poolID: {
							Cost:       340_000_000,
							Margin:     test.margin,
							PoolOwners: []AddrKeyHash{owner},
						},
					},
					StakeRegistrations: map[AddrKeyHash]bool{
						owner:     true,
						delegator: true,
					},
					PoolBlocks:         map[PoolKeyHash]uint32{poolID: 10},
					TotalBlocksInEpoch: 10,
				},
				RewardParameters{PoolInfluence: big.NewRat(3, 10)},
			)
			if !test.valid {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrPoolMarginOutsideUnitInterval)
				assert.Nil(t, result)
				return
			}
			require.NoError(t, err)
			poolRewards := result.PoolRewards[poolID]
			distributed := poolRewards.OperatorRewards
			for _, reward := range poolRewards.DelegatorRewards {
				distributed += reward
			}
			assert.Equal(t, poolRewards.TotalRewards, distributed)
		})
	}
}

func TestDistributePoolRewardsConservesInvalidMargins(t *testing.T) {
	owner := AddrKeyHash{0x02}
	delegator := AddrKeyHash{0x03}
	for _, test := range []struct {
		name   string
		margin GenesisRat
	}{
		{name: "negative", margin: NewGenesisRat(-1, 1)},
		{name: "above one", margin: NewGenesisRat(2, 1)},
		{name: "missing", margin: GenesisRat{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rewards := distributePoolRewards(
				PoolKeyHash{0x01},
				1_000_000_000,
				map[AddrKeyHash]uint64{
					owner:     100_000_000,
					delegator: 900_000_000,
				},
				&PoolRegistrationCertificate{
					Cost:       340_000_000,
					Margin:     test.margin,
					PoolOwners: []AddrKeyHash{owner},
				},
				RewardSnapshot{StakeRegistrations: map[AddrKeyHash]bool{
					owner:     true,
					delegator: true,
				}},
			)
			distributed := rewards.OperatorRewards
			for _, reward := range rewards.DelegatorRewards {
				distributed += reward
			}
			assert.LessOrEqual(t, distributed, rewards.TotalRewards)
		})
	}
}

func TestDistributePoolRewardsConservesMaximumPot(t *testing.T) {
	owner := AddrKeyHash{0x02}
	rewards := distributePoolRewards(
		PoolKeyHash{0x01},
		^uint64(0),
		map[AddrKeyHash]uint64{owner: 1},
		&PoolRegistrationCertificate{
			Margin:     NewGenesisRat(1, 1),
			PoolOwners: []AddrKeyHash{owner},
		},
		RewardSnapshot{StakeRegistrations: map[AddrKeyHash]bool{owner: true}},
	)
	assert.Equal(t, ^uint64(0), rewards.OperatorRewards)
	assert.Empty(t, rewards.DelegatorRewards)
	assert.Equal(t, ^uint64(0), rewards.TotalRewards)
}

func TestMarginFloatIsBounded(t *testing.T) {
	tests := []struct {
		name   string
		margin GenesisRat
		want   float64
	}{
		{name: "missing", margin: GenesisRat{}, want: 0},
		{name: "negative", margin: NewGenesisRat(-1, 1), want: 0},
		{name: "zero", margin: NewGenesisRat(0, 1), want: 0},
		{name: "half", margin: NewGenesisRat(1, 2), want: 0.5},
		{name: "one", margin: NewGenesisRat(1, 1), want: 1},
		{name: "above one", margin: NewGenesisRat(2, 1), want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, marginFloat(test.margin))
		})
	}
}
