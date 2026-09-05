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

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func TestProtocolParametersUpgradePreservesMinPoolCost(t *testing.T) {
	const minPoolCost = uint64(123456789)

	shelleyParams := shelley.ShelleyProtocolParameters{
		MinPoolCost: minPoolCost,
	}
	allegraParams := allegra.UpgradePParams(shelleyParams)
	maryParams := mary.UpgradePParams(allegraParams)
	alonzoParams := alonzo.UpgradePParams(maryParams)
	babbageParams := babbage.UpgradePParams(alonzoParams)
	conwayParams := conway.UpgradePParams(babbageParams)

	paramsByEra := []struct {
		name string
		got  uint64
	}{
		{name: "Shelley", got: shelleyParams.MinPoolCost},
		{name: "Allegra", got: allegraParams.MinPoolCost},
		{name: "Mary", got: maryParams.MinPoolCost},
		{name: "Alonzo", got: alonzoParams.MinPoolCost},
		{name: "Babbage", got: babbageParams.MinPoolCost},
		{name: "Conway", got: conwayParams.MinPoolCost},
	}
	for _, tc := range paramsByEra {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, minPoolCost, tc.got)
		})
	}
}
