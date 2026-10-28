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

package ledger

import "github.com/blinklabs-io/gouroboros/cbor"

// maxRejectionItems bounds the total number of CBOR data items in one
// transaction rejection reason. The decoder's per-collection limit does not
// bound a payload made of many collections, and a rejection is peer-supplied.
// A rejection echoes at most a transaction's worth of data (tens of thousands
// of items for a maximum-size transaction), so this is far above any real
// ApplyTxError while keeping decoded memory in the tens of megabytes.
const maxRejectionItems = 131072

// checkRejectionBudget returns an error when the first item in data holds
// more than maxRejectionItems data items or nests too deeply.
func checkRejectionBudget(data []byte) error {
	return cbor.CheckItemBudget(data, maxRejectionItems)
}
