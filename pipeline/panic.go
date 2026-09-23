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

package pipeline

import "errors"

// ErrStagePanic matches the error the pipeline reports on its errors channel
// when a panic raised inside a stage was contained rather than allowed to
// terminate the process. Test for it with errors.Is.
//
// A stage runs on a worker goroutine the pipeline started, so an uncontained
// panic kills that worker: with a single worker the pipeline stops making
// progress and WaitForDrain never returns, and with several it silently loses
// throughput. The error carries the panic value and the stack of the panicking
// goroutine, so a panic from a consumer-supplied ApplyFunc, Eta0Provider or
// Stage remains diagnosable.
//
// Unlike a mini-protocol, a panic in an independent decode or validation stage
// fails only the block item being processed, which carries the failure
// downstream while the worker takes the next item. Apply-stage runner and
// completion-accounting panics stop the pipeline instead: their ordered state
// may already have changed, so continuing could move a completion fence across
// an unresolved sequence gap.
var ErrStagePanic = errors.New("recovered panic")

// markUnresolvedPhase records err against the earliest processing phase the
// item has not resolved, preserving the invariant that an item leaving a stage
// never looks successful when that stage did not run to completion.
//
// A stage sets its own outcome on the item just before returning, so a panic
// leaves the item with neither a result nor an error for that phase. Without
// this, an item whose decode panicked would reach the apply stage indexed as a
// decoded block with a nil Block, and a consumer's ApplyFunc would be handed
// it; both the apply stage and the validation-required check skip an item that
// carries an error, so recording one is what makes the skip happen.
func markUnresolvedPhase(item *BlockItem, err error) {
	if item == nil {
		return
	}
	if !item.IsDecoded() && item.DecodeError() == nil {
		item.SetDecodeError(err, 0)
		return
	}
	if !item.IsValid() && item.ValidationError() == nil {
		item.SetValidation(false, "", err, 0)
		return
	}
	if !item.IsApplied() && item.ApplyError() == nil {
		item.SetApplied(false, err, 0)
	}
}
