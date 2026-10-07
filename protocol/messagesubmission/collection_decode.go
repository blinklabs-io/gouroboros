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

package messagesubmission

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

// RequestCount is uint16 on the wire, so no conforming session can have more
// message IDs outstanding than this even when its configured window is larger.
const maxWireMessageIDs = int(^uint16(0))

func configuredMessageIDLimit(cfg *Config) int {
	if cfg == nil || cfg.MaxUnacknowledgedMessageIDs <= 0 {
		return DefaultMaxUnacknowledgedMessageIDs
	}
	return min(cfg.MaxUnacknowledgedMessageIDs, maxWireMessageIDs)
}

func validateMessageSubmissionCollection(msgType uint, data []byte, maxCount int) error {
	var maxItemDepth int
	switch msgType {
	case MessageTypeRequestMessages:
		maxItemDepth = 0
	case MessageTypeReplyMessages:
		// A DMQ message contains payload and operational-certificate arrays,
		// whose fields are all scalars or byte strings.
		maxItemDepth = 2
	default:
		maxItemDepth = 1
	}
	if err := cborpreflight.ValidateSecondFieldArray(
		data,
		maxCount,
		"message-submission collection",
		func(idx int, raw []byte) error {
			return cborpreflight.ValidateItemDepth(
				raw,
				maxItemDepth,
				fmt.Sprintf("message-submission item %d", idx),
			)
		},
	); err != nil {
		return err
	}
	return cborpreflight.ValidateItemDepth(data, 4, "message-submission message")
}
