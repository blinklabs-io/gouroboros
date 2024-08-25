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

package ledger

import (
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type Era = common.Era

var EraInvalid = common.EraInvalid

func GetEraById(eraId uint8) Era {
	return common.EraById(eraId)
}

// BlockHeaderToBlockTypeMap is a mapping of NtN chainsync block header types
// (era ID) to NtC block types
var BlockHeaderToBlockTypeMap = map[uint]uint{
	BlockHeaderTypeShelley: BlockTypeShelley,
	BlockHeaderTypeAllegra: BlockTypeAllegra,
	BlockHeaderTypeMary:    BlockTypeMary,
	BlockHeaderTypeAlonzo:  BlockTypeAlonzo,
	BlockHeaderTypeBabbage: BlockTypeBabbage,
	BlockHeaderTypeConway:  BlockTypeConway,
}

// BlockToBlockHeaderTypeMap is a mapping of NtC chainsync block types
// to NtN block header types (era ID)
var BlockToBlockHeaderTypeMap = map[uint]uint{
	BlockTypeShelley: BlockHeaderTypeShelley,
	BlockTypeAllegra: BlockHeaderTypeAllegra,
	BlockTypeMary:    BlockHeaderTypeMary,
	BlockTypeAlonzo:  BlockHeaderTypeAlonzo,
	BlockTypeBabbage: BlockHeaderTypeBabbage,
	BlockTypeConway:  BlockHeaderTypeConway,
}
