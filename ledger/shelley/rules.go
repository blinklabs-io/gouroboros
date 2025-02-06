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

package shelley

import (
	common "github.com/blinklabs-io/gouroboros/ledger/common"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	UtxoValidateTimeToLive,
}

// UtxoValidateTimeToLive ensures that the current tip slot is not after the specified TTL value
func UtxoValidateTimeToLive(tx common.Transaction, ls common.LedgerState, ts common.TipState, pp common.ProtocolParameters) error {
	tip, err := ts.Tip()
	if err != nil {
		return err
	}
	ttl := tx.TTL()
	if ttl == 0 || ttl >= tip.Point.Slot {
		return nil
	}
	return ExpiredUtxoError{
		Ttl:  ttl,
		Slot: tip.Point.Slot,
	}
}
