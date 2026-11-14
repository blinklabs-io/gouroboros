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
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// A value key lets the decoder retain the last occurrence in wire order.
// Address pointers would retain separate entries for the same reward account.
type withdrawalRewardAccount string

func (a *withdrawalRewardAccount) UnmarshalCBOR(data []byte) error {
	var address Address
	if err := address.UnmarshalCBOR(data); err != nil {
		return err
	}
	if _, err := address.RewardAccountCredential(); err != nil {
		return fmt.Errorf("invalid withdrawal address: %w", err)
	}
	raw, err := address.Bytes()
	if err != nil {
		return err
	}
	*a = withdrawalRewardAccount(raw)
	return nil
}

// PreConwayWithdrawals decodes withdrawal maps with the pre-version-9
// last-occurrence policy while validating every reward account.
type PreConwayWithdrawals map[*Address]uint64

// UnmarshalCBOR retains the final amount for each logical reward account.
func (w *PreConwayWithdrawals) UnmarshalCBOR(data []byte) error {
	var entries map[withdrawalRewardAccount]uint64
	// The key decoder validates every occurrence before duplicate resolution.
	if _, err := cbor.DecodeLenient(data, &entries); err != nil {
		return err
	}
	if entries == nil {
		*w = nil
		return nil
	}
	withdrawals := make(PreConwayWithdrawals, len(entries))
	for raw, amount := range entries {
		address, err := NewAddressFromBytes([]byte(raw))
		if err != nil {
			return err
		}
		withdrawals[&address] = amount
	}
	*w = withdrawals
	return nil
}
