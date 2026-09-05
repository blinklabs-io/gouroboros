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

package conway_test

import (
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type committeeTermLedgerState struct {
	common.LedgerState
	currentEpoch uint64
}

func (s committeeTermLedgerState) CurrentEpoch() uint64 {
	return s.currentEpoch
}

func committeeTermProposal(
	credential common.Credential,
	expiry uint,
) *conway.ConwayTransaction {
	return mkProposalTx(
		0,
		common.Address{},
		&common.UpdateCommitteeGovAction{
			CredEpochs: map[*common.Credential]uint{&credential: expiry},
		},
	)
}

func TestCommitteeTermLimitProductionRule(t *testing.T) {
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("committee-cold-key")),
	}
	tests := []struct {
		name         string
		currentEpoch uint64
		expiry       uint
		maxTerm      uint64
		wantError    bool
	}{
		{name: "below boundary", currentEpoch: 100, expiry: 109, maxTerm: 10},
		{name: "at boundary", currentEpoch: 100, expiry: 110, maxTerm: 10},
		{
			name:         "over boundary",
			currentEpoch: 100,
			expiry:       111,
			maxTerm:      10,
			wantError:    true,
		},
		{name: "zero limit at current epoch", currentEpoch: 100, expiry: 100},
		{
			name:         "zero limit with positive remaining term",
			currentEpoch: 100,
			expiry:       101,
			wantError:    true,
		},
		{
			name:         "uint64 addition would overflow",
			currentEpoch: math.MaxUint64 - 2,
			expiry:       math.MaxUint64,
			maxTerm:      5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls := committeeTermLedgerState{currentEpoch: tt.currentEpoch}
			pp := &conway.ConwayProtocolParameters{
				CommitteeTermLimit: tt.maxTerm,
			}
			err := conway.UtxoValidateGovActionWellFormedness(
				committeeTermProposal(credential, tt.expiry),
				0,
				ls,
				pp,
			)
			if !tt.wantError {
				require.NoError(t, err)
				return
			}
			var termErr conway.CommitteeTermTooLongError
			require.ErrorAs(t, err, &termErr)
			assert.Equal(t, credential.Credential, termErr.Credential)
			assert.Equal(t, tt.currentEpoch, termErr.CurrentEpoch)
			assert.Equal(t, uint64(tt.expiry), termErr.ExpiryEpoch)
			assert.Equal(t, tt.maxTerm, termErr.MaxTermLength)
		})
	}
}

func TestCommitteeTermLimitRequiresStateAndParameters(t *testing.T) {
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("committee-cold-key")),
	}
	tx := committeeTermProposal(credential, 101)

	t.Run("unsupported parameters", func(t *testing.T) {
		err := conway.UtxoValidateGovActionWellFormedness(
			tx,
			0,
			nil,
			mockProtocolParameters{},
		)
		var unavailable conway.CommitteeTermLimitUnavailableError
		require.ErrorAs(t, err, &unavailable)
	})

	t.Run("missing epoch state", func(t *testing.T) {
		err := conway.UtxoValidateGovActionWellFormedness(
			tx,
			0,
			nil,
			&conway.ConwayProtocolParameters{CommitteeTermLimit: 10},
		)
		var unavailable conway.CurrentEpochStateUnavailableError
		require.ErrorAs(t, err, &unavailable)
	})
}

type mockProtocolParameters struct{}

func (mockProtocolParameters) Utxorpc() (*utxorpc.PParams, error) {
	return nil, nil
}
