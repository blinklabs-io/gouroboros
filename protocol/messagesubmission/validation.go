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

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"golang.org/x/crypto/blake2b"
)

type messageIDRequest struct {
	blocking  bool
	ack       int
	requested int
}

type messageRequest struct {
	ids map[string]struct{}
}

func invalidMessageSubmissionMessage(format string, args ...any) error {
	return fmt.Errorf(
		"%s: %s: %w",
		ProtocolName,
		fmt.Sprintf(format, args...),
		protocol.ErrProtocolViolationInvalidMessage,
	)
}

func validateMessageID(id []byte) error {
	if len(id) != blake2b.Size256 {
		return invalidMessageSubmissionMessage(
			"message ID must be %d bytes, got %d",
			blake2b.Size256,
			len(id),
		)
	}
	return nil
}

func validateMessageIDRequest(
	outstanding [][]byte,
	request messageIDRequest,
	limit int,
) error {
	if request.ack < 0 || request.ack > len(outstanding) {
		return fmt.Errorf(
			"%s: acknowledgement %d exceeds %d outstanding message IDs: %w",
			ProtocolName,
			request.ack,
			len(outstanding),
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	if request.requested < 0 ||
		(request.blocking && request.requested == 0) ||
		(!request.blocking && request.ack == 0 && request.requested == 0) {
		return fmt.Errorf(
			"%s: invalid message ID request counts: %w",
			ProtocolName,
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	remaining := len(outstanding) - request.ack
	if request.blocking && remaining != 0 {
		return fmt.Errorf(
			"%s: blocking request leaves %d message IDs unacknowledged: %w",
			ProtocolName,
			remaining,
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	if remaining+request.requested > limit {
		return fmt.Errorf(
			"%s: request would leave %d message IDs outstanding, limit %d: %w",
			ProtocolName,
			remaining+request.requested,
			limit,
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	return nil
}

func reconcileMessageIDs(
	outstanding [][]byte,
	request messageIDRequest,
	reply []pcommon.MessageIDAndSize,
	limit int,
) ([][]byte, error) {
	if err := validateMessageIDRequest(outstanding, request, limit); err != nil {
		return nil, err
	}
	if len(reply) > request.requested {
		return nil, fmt.Errorf(
			"%s: reply contains %d message IDs for a request of %d: %w",
			ProtocolName,
			len(reply),
			request.requested,
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	if request.blocking && len(reply) == 0 {
		return nil, invalidMessageSubmissionMessage(
			"blocking request for %d message IDs received an empty reply",
			request.requested,
		)
	}
	remaining := outstanding[request.ack:]
	seen := make(map[string]struct{}, len(remaining)+len(reply))
	ret := make([][]byte, 0, len(remaining)+len(reply))
	for _, id := range remaining {
		key := string(id)
		seen[key] = struct{}{}
		ret = append(ret, append([]byte(nil), id...))
	}
	for _, item := range reply {
		if err := validateMessageID(item.MessageID); err != nil {
			return nil, err
		}
		key := string(item.MessageID)
		if _, ok := seen[key]; ok {
			return nil, invalidMessageSubmissionMessage(
				"reply repeats outstanding message ID %x",
				item.MessageID,
			)
		}
		seen[key] = struct{}{}
		ret = append(ret, append([]byte(nil), item.MessageID...))
	}
	return ret, nil
}

func requestedMessagesAreOutstanding(
	outstanding [][]byte,
	requested [][]byte,
) (*messageRequest, error) {
	if len(requested) > len(outstanding) {
		return nil, fmt.Errorf(
			"%s: request contains %d message IDs with only %d outstanding: %w",
			ProtocolName,
			len(requested),
			len(outstanding),
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	available := make(map[string]struct{}, len(outstanding))
	for _, id := range outstanding {
		available[string(id)] = struct{}{}
	}
	ids := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		if err := validateMessageID(id); err != nil {
			return nil, err
		}
		key := string(id)
		if _, ok := ids[key]; ok {
			return nil, invalidMessageSubmissionMessage(
				"request contains duplicate message ID %x",
				id,
			)
		}
		if _, ok := available[key]; !ok {
			return nil, invalidMessageSubmissionMessage(
				"request contains unannounced message ID %x",
				id,
			)
		}
		ids[key] = struct{}{}
	}
	return &messageRequest{ids: ids}, nil
}

func validateMessageReply(
	request *messageRequest,
	messages []pcommon.DmqMessage,
) error {
	if request == nil {
		return invalidMessageSubmissionMessage(
			"received message reply without an outstanding request",
		)
	}
	if len(messages) > len(request.ids) {
		return fmt.Errorf(
			"%s: reply contains %d messages for a request of %d: %w",
			ProtocolName,
			len(messages),
			len(request.ids),
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	seen := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		id := message.ID()
		if err := validateMessageID(id); err != nil {
			return err
		}
		key := string(id)
		if _, ok := request.ids[key]; !ok {
			return invalidMessageSubmissionMessage(
				"reply contains unrequested message ID %x",
				id,
			)
		}
		if _, ok := seen[key]; ok {
			return invalidMessageSubmissionMessage(
				"reply contains duplicate message ID %x",
				id,
			)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func cloneMessageIDs(ids [][]byte) [][]byte {
	ret := make([][]byte, len(ids))
	for i, id := range ids {
		ret[i] = append([]byte(nil), id...)
	}
	return ret
}
