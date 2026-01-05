// Copyright 2024 Blink Labs Software
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
"math/big"
	"testing"

	_ "github.com/blinklabs-io/gouroboros/ledger" // This is needed to get the eras registered
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type getEraByIdTestDefinition struct {
	Id   uint8
	Name string
}

var getEraByIdTests = []getEraByIdTestDefinition{
	{
		Id:   0,
		Name: "Byron",
	},
	{
		Id:   1,
		Name: "Shelley",
	},
	{
		Id:   2,
		Name: "Allegra",
	},
	{
		Id:   3,
		Name: "Mary",
	},
	{
		Id:   4,
		Name: "Alonzo",
	},
	{
		Id:   5,
		Name: "Babbage",
	},
	{
		Id:   6,
		Name: "Conway",
	},
	{
		Id:   99,
		Name: "invalid",
	},
}

func TestGetEraById(t *testing.T) {
	for _, test := range getEraByIdTests {
		era := common.EraById(test.Id)
		if era == common.EraInvalid {
			if test.Name != "invalid" {
				t.Fatalf("got unexpected EraInvalid, wanted %s", test.Name)
			}
		} else {
			if era.Name != test.Name {
				t.Fatalf("did not get expected era name for ID %d, got: %s, wanted: %s", test.Id, era.Name, test.Name)
			}
		}
	}
}
