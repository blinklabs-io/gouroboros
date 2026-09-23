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
	"cmp"
	"slices"
	"strings"
)

// SortRewardAccountAddresses returns reward-account addresses in the order
// used by cardano-ledger: network, then script/key credential type, then hash.
func SortRewardAccountAddresses[V any](values map[*Address]V) []*Address {
	sorted := make([]*Address, 0, len(values))
	for address := range values {
		sorted = append(sorted, address)
	}
	slices.SortFunc(sorted, func(a, b *Address) int {
		if a == nil {
			if b == nil {
				return 0
			}
			return -1
		}
		if b == nil {
			return 1
		}
		aCredential, aErr := a.RewardAccountCredential()
		bCredential, bErr := b.RewardAccountCredential()
		if aErr != nil || bErr != nil {
			return strings.Compare(a.String(), b.String())
		}
		if c := cmp.Compare(a.NetworkId(), b.NetworkId()); c != 0 {
			return c
		}
		if c := cmp.Compare(aCredential.CredType, bCredential.CredType); c != 0 {
			return -c
		}
		return bytes.Compare(
			aCredential.Credential[:],
			bCredential.Credential[:],
		)
	})
	return sorted
}
