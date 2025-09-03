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

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/assert"
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
