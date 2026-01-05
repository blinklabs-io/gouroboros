// Copyright 2025 Blink Labs Software
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

func TestConwayProposalProcedureToPlutusData(t *testing.T) {
	addr := common.Address{}
	action := &common.InfoGovAction{}

	pp := &ConwayProposalProcedure{
		PPDeposit:       1000000,
		PPRewardAccount: addr,
		PPGovAction:     ConwayGovAction{Action: action},
	}

	pd := pp.ToPlutusData()
	constr, ok := pd.(*data.Constr)
	assert.True(t, ok)
	assert.Equal(t, uint(0), constr.Tag)
	assert.Len(t, constr.Fields, 3)
}

func TestConwayProposalProcedureCbor(t *testing.T) {
	// Test CBOR encoding/decoding for CIP-1694 governance proposal procedures
	addr, err := common.NewAddress(
		"addr1vx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzers66hrl8",
	)
	require.NoError(t, err)
	action := &common.InfoGovAction{Type: common.GovActionTypeInfo}

	pp := &ConwayProposalProcedure{
		PPDeposit:       1000000,
		PPRewardAccount: addr,
		PPGovAction:     ConwayGovAction{Action: action},
		PPAnchor:        common.GovAnchor{},
	}

	// Encode to CBOR
	cborData, err := cbor.Encode(pp)
	require.NoError(t, err)
	assert.NotEmpty(t, cborData)

	// Decode back from CBOR
	var decoded ConwayProposalProcedure
	_, err = cbor.Decode(cborData, &decoded)
	require.NoError(t, err)

	// Verify fields match
	assert.Equal(t, pp.PPDeposit, decoded.PPDeposit)
	assert.Equal(
		t,
		pp.PPRewardAccount.String(),
		decoded.PPRewardAccount.String(),
	)
	// Verify the gov action type
	decodedAction, ok := decoded.PPGovAction.Action.(*common.InfoGovAction)
	require.True(t, ok, "expected InfoGovAction type")
	assert.Equal(t, uint(common.GovActionTypeInfo), decodedAction.Type)
}

func TestConwayGovActionCbor(t *testing.T) {
	// Test CBOR encoding/decoding for CIP-1694 governance actions
	action := &common.InfoGovAction{Type: common.GovActionTypeInfo}

	ga := ConwayGovAction{Action: action}

	// Encode to CBOR
	cborData, err := ga.MarshalCBOR()
	assert.NoError(t, err)
	assert.NotEmpty(t, cborData)

	// Decode back from CBOR
	var decoded ConwayGovAction
	err = decoded.UnmarshalCBOR(cborData)
	assert.NoError(t, err)

	// Verify the action type matches
	assert.IsType(t, &common.InfoGovAction{}, decoded.Action)
	infoAction, ok := decoded.Action.(*common.InfoGovAction)
	require.True(t, ok, "type assertion to *common.InfoGovAction failed")
	assert.Equal(t, uint(common.GovActionTypeInfo), infoAction.Type)
}
