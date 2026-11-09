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
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
)

// CheckPlutusV1V3Features rejects a transaction body that carries direct
// deposits, account balance intervals, required top-level guards or
// script-hash guards. Only the Plutus V4 context represents them, so building a
// V1, V2 or V3 context for such a transaction fails, as in cardano-ledger's
// guardDijkstraFeaturesForPlutusV1toV3. Key-hash guards are allowed.
func (t DijkstraTransaction) CheckPlutusV1V3Features() error {
	switch {
	case len(t.Body.TxDirectDeposits) > 0:
		return script.UnsupportedFeatureError{Feature: "direct deposits"}
	case len(t.Body.TxBalanceIntervals) > 0:
		return script.UnsupportedFeatureError{
			Feature: "account balance intervals",
		}
	case len(t.Body.TxRequiredTopLevelGuards) > 0:
		return script.UnsupportedFeatureError{
			Feature: "required top-level guards",
		}
	}
	if t.Body.TxGuards != nil {
		for _, guard := range t.Body.TxGuards.Credentials {
			if guard.CredType == common.CredentialTypeScriptHash {
				return script.UnsupportedFeatureError{
					Feature: "script-hash guards",
				}
			}
		}
	}
	return nil
}

// CheckPlutusV1V3Features forwards the check through the wrapper, which hides
// the optional interface of the transaction it embeds.
func (t transactionWithoutGuardingRedeemers) CheckPlutusV1V3Features() error {
	return script.CheckPlutusV1V3Features(t.Transaction)
}

// plutusV4BaseTransaction is the transaction the V4 context builds its V3 base
// from. The V4 context carries the fields the V1 to V3 check rejects, so the
// check is a no-op here.
type plutusV4BaseTransaction struct {
	transactionWithoutGuardingRedeemers
}

func (plutusV4BaseTransaction) CheckPlutusV1V3Features() error {
	return nil
}
