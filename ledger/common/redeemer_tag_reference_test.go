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

package common_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
)

// TestRedeemerTagLimitAsReference checks the redeemer purposes each era
// defines. cardano-ledger 1210d37f decodes the purpose tag as a Word8 and
// fails on any tag past the era's last constructor: Alonzo and Babbage stop
// at 3 (rewarding; eras/alonzo/impl/src/Cardano/Ledger/Alonzo/Scripts.hs:
// 407-413), Conway at 5 (proposing; eras/conway/impl/src/Cardano/Ledger/
// Conway/Scripts.hs:292-300) and Dijkstra at 6 (guarding; eras/dijkstra/impl/
// src/Cardano/Ledger/Dijkstra/Scripts.hs:173-182). The redeemers are a
// one-entry redeemer map, written by hand from the dijkstra.cddl redeemers
// rule, decoded with the widest era's decoder.
func TestRedeemerTagLimitAsReference(t *testing.T) {
	eras := []struct {
		name   string
		maxTag common.RedeemerTag
	}{
		{"Alonzo", common.RedeemerTagReward},
		{"Babbage", common.RedeemerTagReward},
		{"Conway", common.RedeemerTagProposing},
		{"Dijkstra", common.RedeemerTagGuarding},
	}
	referenceMax := map[string]uint{
		"Alonzo": 3, "Babbage": 3, "Conway": 5, "Dijkstra": 6,
	}
	for tag := range uint(7) {
		raw, _ := hex.DecodeString(
			fmt.Sprintf("a182%02x00"+"82"+"00"+"820000", tag),
		)
		var redeemers dijkstra.DijkstraRedeemers
		if _, err := cbor.Decode(raw, &redeemers); err != nil {
			t.Fatalf("tag %d: decode: %v", tag, err)
		}
		for _, era := range eras {
			err := common.ValidateRedeemerTagLimit(&redeemers, era.maxTag)
			want := tag <= referenceMax[era.name]
			if got := err == nil; got != want {
				t.Errorf("%s tag %d: accepted = %v, reference = %v",
					era.name, tag, got, want)
			}
		}
	}
	if err := common.ValidateRedeemerTagLimit(
		nil,
		common.RedeemerTagSpend,
	); err != nil {
		t.Errorf("no redeemers rejected: %v", err)
	}
}
