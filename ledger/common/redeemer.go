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
	"cmp"
	"encoding/json"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type RedeemerTag uint8

const (
	RedeemerTagSpend     RedeemerTag = 0
	RedeemerTagMint      RedeemerTag = 1
	RedeemerTagCert      RedeemerTag = 2
	RedeemerTagReward    RedeemerTag = 3
	RedeemerTagVoting    RedeemerTag = 4
	RedeemerTagProposing RedeemerTag = 5
)

var redeemerTagNames = map[RedeemerTag]string{
	RedeemerTagSpend:     "spend",
	RedeemerTagMint:      "mint",
	RedeemerTagCert:      "cert",
	RedeemerTagReward:    "reward",
	RedeemerTagVoting:    "voting",
	RedeemerTagProposing: "proposing",
}

var redeemerTagValues = map[string]RedeemerTag{
	"spend":     RedeemerTagSpend,
	"mint":      RedeemerTagMint,
	"cert":      RedeemerTagCert,
	"reward":    RedeemerTagReward,
	"voting":    RedeemerTagVoting,
	"proposing": RedeemerTagProposing,
}

func (t RedeemerTag) MarshalJSON() ([]byte, error) {
	name, ok := redeemerTagNames[t]
	if !ok {
		return nil, fmt.Errorf("unknown redeemer tag: %d", t)
	}
	return json.Marshal(name)
}

func (t *RedeemerTag) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	val, ok := redeemerTagValues[name]
	if !ok {
		return fmt.Errorf("unknown redeemer tag name: %q", name)
	}
	*t = val
	return nil
}

type RedeemerKey struct {
	cbor.StructAsArray
	Tag   RedeemerTag `json:"tag"`
	Index uint32      `json:"index"`
}

type RedeemerValue struct {
	cbor.StructAsArray
	Data    Datum   `json:"data"`
	ExUnits ExUnits `json:"exUnits"`
}

// CompareRedeemerKeys compares two RedeemerKey values by Tag then Index,
// returning -1, 0, or 1. Suitable for use with slices.SortFunc.
func CompareRedeemerKeys(a, b RedeemerKey) int {
	if c := cmp.Compare(a.Tag, b.Tag); c != 0 {
		return c
	}
	return cmp.Compare(a.Index, b.Index)
}
