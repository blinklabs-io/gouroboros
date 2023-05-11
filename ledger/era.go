// Copyright 2023 Blink Labs, LLC.
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

package ledger

type Era struct {
	Id   uint8
	Name string
}

var eras = map[uint8]Era{
	ERA_ID_BYRON: Era{
		Id:   ERA_ID_BYRON,
		Name: "Byron",
	},
	ERA_ID_SHELLEY: Era{
		Id:   ERA_ID_SHELLEY,
		Name: "Shelley",
	},
	ERA_ID_ALLEGRA: Era{
		Id:   ERA_ID_ALLEGRA,
		Name: "Allegra",
	},
	ERA_ID_MARY: Era{
		Id:   ERA_ID_MARY,
		Name: "Mary",
	},
	ERA_ID_ALONZO: Era{
		Id:   ERA_ID_ALONZO,
		Name: "Alonzo",
	},
	ERA_ID_BABBAGE: Era{
		Id:   ERA_ID_BABBAGE,
		Name: "Babbage",
	},
}

func GetEraById(eraId uint8) *Era {
	era, ok := eras[eraId]
	if !ok {
		return nil
	}
	return &era
}
