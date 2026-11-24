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
	if !request.blocking && remaining == 0 {
		return fmt.Errorf(
			"%s: non-blocking request leaves no message IDs outstanding: %w",
			ProtocolName,
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

// acknowledgeMessageIDs returns the window after a blocking request is
// answered with MsgReplyNoMessageIds: the acknowledged IDs leave it and none
// join. Only a blocking request may be answered that way (the V2 state map
// allows the message only from StMessageIdsBlocking).
func acknowledgeMessageIDs(
	outstanding [][]byte,
	request messageIDRequest,
	limit int,
) ([][]byte, error) {
	if err := validateMessageIDRequest(outstanding, request, limit); err != nil {
		return nil, err
	}
	if !request.blocking {
		return nil, invalidMessageSubmissionMessage(
			"a non-blocking message ID request cannot be answered with no IDs",
		)
	}
	remaining := outstanding[request.ack:]
	ret := make([][]byte, 0, len(remaining))
	for _, id := range remaining {
		ret = append(ret, append([]byte(nil), id...))
	}
	return ret, nil
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

// requestedMessagesAreOutstanding checks a message request: each ID must be
// outstanding, named once, and not already in requestedIDs, the IDs asked for
// earlier and not yet acknowledged. The protocol allows a body to be asked
// for at most once while its ID is outstanding.
func requestedMessagesAreOutstanding(
	outstanding [][]byte,
	requestedIDs map[string]struct{},
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
		if _, ok := requestedIDs[key]; ok {
			return nil, invalidMessageSubmissionMessage(
				"request repeats already requested message ID %x",
				id,
			)
		}
		ids[key] = struct{}{}
	}
	return &messageRequest{ids: ids}, nil
}

// forgetAcknowledgedIDs returns requested without the first ack IDs of
// outstanding, which a request has just acknowledged. An acknowledged ID
// leaves the record with the window, so a later announcement of the same ID
// is new and its body may be requested again.
func forgetAcknowledgedIDs(
	requested map[string]struct{},
	outstanding [][]byte,
	ack int,
) map[string]struct{} {
	ret := make(map[string]struct{}, len(requested))
	for id := range requested {
		ret[id] = struct{}{}
	}
	for _, id := range outstanding[:ack] {
		delete(ret, string(id))
	}
	return ret
}

// withRequestedIDs returns a copy of requested with the IDs of request
// added. It copies so that a caller can restore the previous record if the
// request is never sent.
func withRequestedIDs(
	requested map[string]struct{},
	request *messageRequest,
) map[string]struct{} {
	ret := make(map[string]struct{}, len(requested)+len(request.ids))
	for id := range requested {
		ret[id] = struct{}{}
	}
	for id := range request.ids {
		ret[id] = struct{}{}
	}
	return ret
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
