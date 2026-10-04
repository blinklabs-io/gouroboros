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

package localstatequery

import (
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
)

// DebugEpochStateResult is the result of DebugEpochState (Shelley sub-query 8):
// the ledger's EpochState,
//
//	[accountState, ledgerState, snapshots, nonMyopic]
//
// The account state is decoded. The remaining components are era-dependent
// and are not decoded, so they are kept as the node's exact CBOR for callers
// that know the era they are talking to.
type DebugEpochStateResult struct {
	cbor.StructAsArray
	AccountState AccountState
	LedgerState  cbor.RawMessage
	Snapshots    cbor.RawMessage
	NonMyopic    cbor.RawMessage
}

// DebugNewEpochStateResult is the result of DebugNewEpochState (Shelley
// sub-query 12): the ledger's NewEpochState,
//
//	[epoch, blocksMadePrev, blocksMadeCur, epochState, rewardUpdate,
//	 poolDistr, stashedAVVMAddresses]
//
// The reward update (a pulsing reward computation in flight), the pool
// distribution and the stashed AVVM addresses are era-dependent and are kept
// as the node's exact CBOR. The pool distribution is a bare map of pool to
// individual stake in older ledgers and gains the total active stake in newer
// ones, and the individual stake entry also changes width.
type DebugNewEpochStateResult struct {
	cbor.StructAsArray
	Epoch uint64
	// BlocksMadePrev and BlocksMadeCur count the blocks each pool minted in
	// the previous and the current epoch.
	BlocksMadePrev       map[ledger.Blake2b224]uint64
	BlocksMadeCur        map[ledger.Blake2b224]uint64
	EpochState           DebugEpochStateResult
	RewardUpdate         cbor.RawMessage
	PoolDistr            cbor.RawMessage
	StashedAVVMAddresses cbor.RawMessage
}
