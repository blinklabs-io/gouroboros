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

package localstatequery

import (
	"bytes"
	"testing"
)

func shelleySetOf(n int, item []byte) []byte {
	buf := []byte{0xd9, 0x01, 0x02, 0x99, byte(n >> 8), byte(n)}
	return append(buf, bytes.Repeat(item, n)...)
}

func TestShelleyQuerySetFiltersBounded(t *testing.T) {
	cred := append([]byte{0x82, 0x00, 0x58, 0x1c}, bytes.Repeat([]byte{0xbb}, 28)...)
	pool := append([]byte{0x58, 0x1c}, bytes.Repeat([]byte{0xaa}, 28)...)
	cases := []struct {
		name string
		typ  int
		sets [][]byte
	}{
		{"GetDRepState", QueryTypeShelleyDRepState, [][]byte{shelleySetOf(maxLocalStateQuerySetItems+1, cred)}},
		{"GetStakePoolParams", QueryTypeShelleyStakePoolParams, [][]byte{shelleySetOf(maxLocalStateQuerySetItems+1, pool)}},
		{"GetSPOStakeDistr", QueryTypeShelleySPOStakeDistr, [][]byte{shelleySetOf(maxLocalStateQuerySetItems+1, pool)}},
		{"GetFilteredVoteDelegatees", QueryTypeShelleyFilteredVoteDelegatees, [][]byte{shelleySetOf(maxLocalStateQuerySetItems+1, cred)}},
		{"GetCommitteeMembersState", QueryTypeShelleyCommitteeMembersState, [][]byte{
			{0xd9, 0x01, 0x02, 0x80},
			shelleySetOf(maxLocalStateQuerySetItems+1, cred),
			{0xd9, 0x01, 0x02, 0x80},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte{byte(0x80 + 1 + len(tc.sets)), byte(tc.typ)}
			for _, s := range tc.sets {
				data = append(data, s...)
			}
			if _, err := decodeShelleyQuery(data); err == nil {
				t.Fatalf("%s accepted more than %d filter items", tc.name, maxLocalStateQuerySetItems)
			}
		})
	}
}
