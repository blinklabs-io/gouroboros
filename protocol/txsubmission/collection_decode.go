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

package txsubmission

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

func validateTxSubmissionCollection(msgType uint, data []byte) error {
	maxItemDepth := 1
	if msgType == MessageTypeReplyTxIds || msgType == MessageTypeReplyTxs {
		maxItemDepth = 2
	}
	if err := cborpreflight.ValidateSecondFieldArray(
		data,
		MaxUnackedTxIds,
		"tx-submission collection",
		func(idx int, raw []byte) error {
			return cborpreflight.ValidateItemDepth(
				raw,
				maxItemDepth,
				fmt.Sprintf("tx-submission item %d", idx),
			)
		},
	); err != nil {
		return err
	}
	return cborpreflight.ValidateItemDepth(data, 4, "tx-submission message")
}
