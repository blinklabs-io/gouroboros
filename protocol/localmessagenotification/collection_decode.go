// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package localmessagenotification

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

// replyDecodeLimit is the most messages a client decodes from one reply. A
// reply carrying more fresh messages than the replay cache holds is refused
// after decoding, so the cache bound also caps the decode budget.
func replyDecodeLimit(cfg *Config) int {
	maxReply := cfg.MaxReplyMessages
	if maxReply <= 0 {
		maxReply = DefaultMaxReplyMessages
	}
	maxReplay := cfg.MaxReplayEntries
	if maxReplay <= 0 {
		maxReplay = defaultMaxReplayEntries
	}
	return min(maxReply, maxReplay)
}

func validateReplyCollection(msgType uint, data []byte, maxMessages int) error {
	fields, err := cborpreflight.ArrayItems(
		data,
		3,
		"local message notification reply",
	)
	if err != nil {
		return err
	}
	expectedFields := 2
	if msgType == MessageTypeReplyMessagesNonBlocking {
		expectedFields = 3
	}
	if len(fields) != expectedFields {
		return fmt.Errorf(
			"local message notification reply has %d fields, expected %d",
			len(fields),
			expectedFields,
		)
	}
	if err := cborpreflight.ValidateItemDepth(
		fields[0],
		0,
		"local message notification reply type",
	); err != nil {
		return err
	}
	if err := cborpreflight.ValidateArray(
		fields[1],
		maxMessages,
		"local message notification messages",
		func(idx int, raw []byte) error {
			return cborpreflight.ValidateItemDepth(
				raw,
				2,
				fmt.Sprintf("local message notification message %d", idx),
			)
		},
	); err != nil {
		return err
	}
	return cborpreflight.ValidateItemDepth(
		data,
		4,
		"local message notification reply",
	)
}
