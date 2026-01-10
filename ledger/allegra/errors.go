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

package allegra

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type OutsideValidityIntervalUtxoError struct {
	ValidityIntervalStart uint64
	Slot                  uint64
}

func (e OutsideValidityIntervalUtxoError) Error() string {
	return fmt.Sprintf(
		"outside validity interval: start %d, slot %d",
		e.ValidityIntervalStart,
		e.Slot,
	)
}

type NativeScriptFailedError struct {
	ScriptHash common.ScriptHash
}

func (e NativeScriptFailedError) Error() string {
	return fmt.Sprintf("native script failed (hash=%x)", e.ScriptHash[:])
}

// Sentinel error for native script failures so callers can use errors.Is
var ErrNativeScriptFailed = errors.New("native script failed")

func (NativeScriptFailedError) Is(target error) bool {
	return target == ErrNativeScriptFailed
}
