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

package conway

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unsupportedGovAction satisfies common.GovAction (via the embedded
// GovActionBase) but is not one of the known Conway action types, so
// NewConwayGovAction must reject it rather than silently coerce it to
// discriminant 0.
type unsupportedGovAction struct {
	common.GovActionBase
}

func (unsupportedGovAction) ToPlutusData() data.PlutusData {
	return data.NewConstr(0)
}

func TestNewConwayParameterChangeGovAction(t *testing.T) {
	minFeeA := uint(44)
	paramUpdate := ConwayProtocolParameterUpdate{MinFeeA: &minFeeA}
	policyHash := make([]byte, common.Blake2b224Size)

	action, err := NewConwayParameterChangeGovAction(nil, paramUpdate, policyHash)
	require.NoError(t, err)
	assert.Equal(t, uint(common.GovActionTypeParameterChange), action.Type)
	assert.Nil(t, action.ActionId)

	// CBOR round-trip through the ConwayGovAction wrapper preserves the
	// concrete type and discriminant.
	ga, err := NewConwayGovAction(action)
	require.NoError(t, err)
	cborData, err := ga.MarshalCBOR()
	require.NoError(t, err)
	var decoded ConwayGovAction
	require.NoError(t, decoded.UnmarshalCBOR(cborData))
	decodedAction, ok := decoded.Action.(*ConwayParameterChangeGovAction)
	require.True(t, ok, "expected *ConwayParameterChangeGovAction")
	assert.Equal(
		t,
		uint(common.GovActionTypeParameterChange),
		decodedAction.Type,
	)

	// Empty policy hash is valid (optional field)
	noPolicy, err := NewConwayParameterChangeGovAction(nil, paramUpdate, nil)
	require.NoError(t, err)
	assert.Empty(t, noPolicy.PolicyHash)

	// Negative: non-empty policy hash must be 28 bytes
	_, err = NewConwayParameterChangeGovAction(nil, paramUpdate, []byte{1, 2})
	require.Error(t, err)
}

func TestNewConwayGovActionType(t *testing.T) {
	testCases := []struct {
		name     string
		action   common.GovAction
		expected common.GovActionType
	}{
		{
			name:     "ParameterChange",
			action:   &ConwayParameterChangeGovAction{},
			expected: common.GovActionTypeParameterChange,
		},
		{
			name:     "HardForkInitiation",
			action:   &common.HardForkInitiationGovAction{},
			expected: common.GovActionTypeHardForkInitiation,
		},
		{
			name:     "TreasuryWithdrawal",
			action:   &common.TreasuryWithdrawalGovAction{},
			expected: common.GovActionTypeTreasuryWithdrawal,
		},
		{
			name:     "NoConfidence",
			action:   &common.NoConfidenceGovAction{},
			expected: common.GovActionTypeNoConfidence,
		},
		{
			name:     "UpdateCommittee",
			action:   &common.UpdateCommitteeGovAction{},
			expected: common.GovActionTypeUpdateCommittee,
		},
		{
			name:     "NewConstitution",
			action:   &common.NewConstitutionGovAction{},
			expected: common.GovActionTypeNewConstitution,
		},
		{
			name:     "Info",
			action:   &common.InfoGovAction{},
			expected: common.GovActionTypeInfo,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ga, err := NewConwayGovAction(tc.action)
			require.NoError(t, err)
			assert.Equal(t, uint(tc.expected), ga.Type)
			assert.Equal(t, tc.action, ga.Action)
		})
	}
}

func TestNewConwayGovActionRejectsInvalid(t *testing.T) {
	// A nil action must be rejected rather than wrapped with a CBOR-null
	// governance action.
	_, err := NewConwayGovAction(nil)
	require.Error(t, err)

	// An action that satisfies common.GovAction but is not a known Conway type
	// must be rejected rather than silently coerced to discriminant 0.
	_, err = NewConwayGovAction(unsupportedGovAction{})
	require.Error(t, err)

	// NewConwayProposalProcedure propagates the error instead of producing a
	// procedure that encodes a CBOR-null governance action.
	addr, err := common.NewAddress(
		"addr1vx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzers66hrl8",
	)
	require.NoError(t, err)
	anchor, err := common.NewGovAnchor(
		"https://example.com/proposal.json",
		make([]byte, 32),
	)
	require.NoError(t, err)
	_, err = NewConwayProposalProcedure(1_000_000, addr, nil, anchor)
	require.Error(t, err)
}

func TestNewConwayProposalProcedure(t *testing.T) {
	addr, err := common.NewAddress(
		"addr1vx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzers66hrl8",
	)
	require.NoError(t, err)
	anchor, err := common.NewGovAnchor(
		"https://example.com/proposal.json",
		make([]byte, 32),
	)
	require.NoError(t, err)
	action, err := common.NewInfoGovAction()
	require.NoError(t, err)

	pp, err := NewConwayProposalProcedure(1_000_000, addr, action, anchor)
	require.NoError(t, err)
	assert.Equal(t, uint64(1_000_000), pp.PPDeposit)
	// The wrapper discriminant is set from the action
	assert.Equal(t, uint(common.GovActionTypeInfo), pp.PPGovAction.Type)

	// CBOR round-trip
	cborData, err := cbor.Encode(pp)
	require.NoError(t, err)
	var decoded ConwayProposalProcedure
	_, err = cbor.Decode(cborData, &decoded)
	require.NoError(t, err)
	assert.Equal(t, pp.PPDeposit, decoded.PPDeposit)
	assert.Equal(t, pp.PPRewardAccount.String(), decoded.PPRewardAccount.String())
	decodedAction, ok := decoded.PPGovAction.Action.(*common.InfoGovAction)
	require.True(t, ok, "expected *common.InfoGovAction")
	assert.Equal(t, uint(common.GovActionTypeInfo), decodedAction.Type)
}
