// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dijkstra

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestConwayFeatureTransactionUsesSelectedBodyLowerBound(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		topPresent bool
		subPresent bool
	}{
		{
			name:       "top present and sub absent",
			topPresent: true,
		},
		{
			name:       "top absent and sub present",
			subPresent: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			topBody := DijkstraTransactionBody{}
			topBody.SetValidityIntervalLowerBoundPresence(testCase.topPresent)
			subBody := DijkstraSubTransactionBody{}
			subBody.SetValidityIntervalLowerBoundPresence(testCase.subPresent)

			tx := dijkstraConwayFeatureTransaction{
				Transaction: DijkstraTransaction{Body: topBody},
				body:        &subBody,
			}
			lowerBound, present := common.TransactionValidityIntervalLowerBound(tx)
			require.Zero(t, lowerBound)
			require.Equal(t, testCase.subPresent, present)
		})
	}
}
