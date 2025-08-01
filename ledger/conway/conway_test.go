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

package conway

import (
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestConwayRedeemersIter(t *testing.T) {
	testRedeemers := ConwayRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{
				Tag:   common.RedeemerTagMint,
				Index: 2,
			}: {
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  2222,
				},
			},
			{
				Tag:   common.RedeemerTagMint,
				Index: 0,
			}: {
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  0,
				},
			},
			{
				Tag:   common.RedeemerTagSpend,
				Index: 4,
			}: {
				ExUnits: common.ExUnits{
					Memory: 0,
					Steps:  4444,
				},
			},
		},
	}
	expectedOrder := []struct {
		Key   common.RedeemerKey
		Value common.RedeemerValue
	}{
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagSpend,
				Index: 4,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 0,
					Steps:  4444,
				},
			},
		},
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagMint,
				Index: 0,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  0,
				},
			},
		},
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagMint,
				Index: 2,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  2222,
				},
			},
		},
	}
	iterIdx := 0
	for key, val := range testRedeemers.Iter() {
		expected := expectedOrder[iterIdx]
		if !reflect.DeepEqual(key, expected.Key) {
			t.Fatalf("did not get expected key: got %#v, wanted %#v", key, expected.Key)
		}
		if !reflect.DeepEqual(val, expected.Value) {
			t.Fatalf("did not get expected value: got %#v, wanted %#v", val, expected.Value)
		}
		iterIdx++
	}
}
