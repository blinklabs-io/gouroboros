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

package dijkstra

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDijkstraParameterChangeUtxorpcUpdate(t *testing.T) {
	t.Parallel()
	minFeeA := uint(44)
	action := &DijkstraParameterChangeGovAction{
		ParamUpdate: DijkstraProtocolParameterUpdate{
			MinFeeA: &minFeeA,
		},
	}
	got, err := action.ProtocolParamUpdateUtxorpc()
	require.NoError(t, err)
	require.Equal(t, int64(44), got.MinFeeCoefficient.GetInt())
	require.Nil(t, got.MinFeeConstant)
}
