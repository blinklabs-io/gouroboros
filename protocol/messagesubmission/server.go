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
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

var errReplayCacheCapacityExceeded = errors.New(
	"dmq: message submission replay cache capacity exceeded",
)

// Server implements the MessageSubmission server
type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	protoVersion    uint16

	// Server-side state for message queue management
	lock              sync.Mutex
	messageQueue      []*pcommon.DmqMessage
	pendingMessageIDs [][]byte
	pendingIDRequest  *messageIDRequest
	pendingMsgRequest *messageRequest
	// deliveredIDs maps each message ID handed to ReplyMessagesFunc to its
	// expiry. An ID stays until its message expires so that a peer cannot
	// get the same message delivered again by re-announcing it after it was
	// acknowledged.
	deliveredIDs map[string]uint32
	now          func() time.Time
}

// NewServer returns a new MessageSubmission server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	normalizedCfg := normalizeConfig(cfg)
	s := &Server{
		config:            &normalizedCfg,
		messageQueue:      make([]*pcommon.DmqMessage, 0),
		pendingMessageIDs: [][]byte{},
		deliveredIDs:      make(map[string]uint32),
		now:               time.Now,
		protoVersion:      protoOptions.Version,
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}

	// Select state map and initial state based on negotiated version
	isV2 := isMessageSubmissionV2(protoOptions.Version)
	var baseStateMap protocol.StateMap
	var initialState protocol.State
	if isV2 {
		baseStateMap = stateMapV2
		initialState = protocolStateIdle
	} else {
		baseStateMap = stateMapV1
		initialState = protocolStateInit
	}

	protoConfig := protocol.ProtocolConfig{
		Name:               ProtocolName,
		ProtocolId:         ProtocolID,
		Muxer:              protoOptions.Muxer,
		Logger:             protoOptions.Logger,
		ErrorChan:          protoOptions.ErrorChan,
		Mode:               protoOptions.Mode,
		Role:               protocol.ProtocolRoleServer,
		MessageHandlerFunc: s.messageHandler,
		MessageFromCborFunc: func(msgType uint, data []byte) (protocol.Message, error) {
			return decodeMsgFromCborWithLimit(
				msgType,
				data,
				s.collectionLimit(msgType),
			)
		},
		StateMap:     baseStateMap,
		InitialState: initialState,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

// collectionLimit bounds an inbound collection before it is decoded. A reply
// can never be larger than the correlated request that admits it, and with no
// request outstanding only an empty collection can reach the reply validator.
func (s *Server) collectionLimit(msgType uint) int {
	s.lock.Lock()
	defer s.lock.Unlock()
	switch msgType {
	case MessageTypeReplyMessageIds:
		if s.pendingIDRequest == nil {
			return 0
		}
		return s.pendingIDRequest.requested
	case MessageTypeReplyMessages:
		if s.pendingMsgRequest == nil {
			return 0
		}
		return len(s.pendingMsgRequest.ids)
	default:
		return configuredMessageIDLimit(s.config)
	}
}

// AddMessage adds a message to the outbound queue
func (s *Server) AddMessage(msg *pcommon.DmqMessage) error {
	// Validate message before adding to queue
	if err := s.config.TTLValidator.ValidateMessageTTL(msg); err != nil {
		return err
	}
	if s.config.Authenticator == nil {
		return errors.New("dmq: message authenticator not configured")
	}
	if err := s.config.Authenticator.VerifyMessage(msg); err != nil {
		return err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// Check queue size limit
	if len(s.messageQueue) >= s.config.MaxQueueSize {
		return errors.New("message queue full")
	}

	s.messageQueue = append(s.messageQueue, msg)

	s.Protocol.Logger().
		Debug("message added to queue",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
			"message_id", string(msg.ID()),
			"queue_size", len(s.messageQueue),
		)

	return nil
}

// RequestMessageIdsBlocking sends a blocking request for message IDs
func (s *Server) RequestMessageIdsBlocking(
	ackCount, requestCount uint16,
) error {
	return s.requestMessageIDs(true, ackCount, requestCount)
}

// RequestMessageIdsNonBlocking sends a non-blocking request for message IDs
func (s *Server) RequestMessageIdsNonBlocking(
	ackCount, requestCount uint16,
) error {
	return s.requestMessageIDs(false, ackCount, requestCount)
}

func (s *Server) requestMessageIDs(
	blocking bool,
	ackCount, requestCount uint16,
) error {
	s.lock.Lock()
	if s.pendingIDRequest != nil || s.pendingMsgRequest != nil {
		s.lock.Unlock()
		return errors.New("a request is already in flight")
	}
	request := &messageIDRequest{
		blocking:  blocking,
		ack:       int(ackCount),
		requested: int(requestCount),
	}
	if err := validateMessageIDRequest(
		s.pendingMessageIDs,
		*request,
		s.config.MaxUnacknowledgedMessageIDs,
	); err != nil {
		s.lock.Unlock()
		return err
	}
	s.pendingIDRequest = request
	s.lock.Unlock()
	if err := s.SendMessage(
		NewMsgRequestMessageIds(blocking, ackCount, requestCount),
	); err != nil {
		s.lock.Lock()
		if s.pendingIDRequest == request {
			s.pendingIDRequest = nil
		}
		s.lock.Unlock()
		return err
	}
	return nil
}

// RequestMessages sends a request for specific messages by their IDs
func (s *Server) RequestMessages(messageIDs [][]byte) error {
	messageIDs = cloneMessageIDs(messageIDs)
	s.lock.Lock()
	if s.pendingIDRequest != nil || s.pendingMsgRequest != nil {
		s.lock.Unlock()
		return errors.New("a request is already in flight")
	}
	request, err := requestedMessagesAreOutstanding(
		s.pendingMessageIDs,
		messageIDs,
	)
	if err != nil {
		s.lock.Unlock()
		return err
	}
	s.pendingMsgRequest = request
	s.lock.Unlock()
	if err := s.SendMessage(NewMsgRequestMessages(messageIDs)); err != nil {
		s.lock.Lock()
		if s.pendingMsgRequest == request {
			s.pendingMsgRequest = nil
		}
		s.lock.Unlock()
		return err
	}
	return nil
}

// Done sends MsgDone to terminate the protocol from the server side.
// In V1, this can be sent from StIdle. In V2, this is the only way
// to end the protocol (the client cannot send MsgDone).
func (s *Server) Done() error {
	msg := NewMsgDone()
	return s.SendMessage(msg)
}

// GetAvailableMessages returns messages from the outbound queue up to maxCount
func (s *Server) GetAvailableMessages(maxCount int) []pcommon.DmqMessage {
	s.lock.Lock()
	defer s.lock.Unlock()

	var available []pcommon.DmqMessage
	now := time.Now().Unix()

	// Prune expired and gather up to maxCount
	filtered := make([]*pcommon.DmqMessage, 0, len(s.messageQueue))
	for _, msg := range s.messageQueue {
		if msg == nil {
			continue
		}
		// #nosec G115 -- Unix timestamp will not overflow uint32 until year 2106
		if uint32(now) <= msg.Payload.ExpiresAt {
			if len(available) < maxCount {
				available = append(available, *msg)
			} else {
				filtered = append(filtered, msg)
			}
		}
	}
	s.messageQueue = filtered

	return available
}

// GetAvailableMessageIDs returns message IDs from the queue up to count
func (s *Server) GetAvailableMessageIDs(count int) []pcommon.MessageIDAndSize {
	s.lock.Lock()
	defer s.lock.Unlock()

	ids := make([]pcommon.MessageIDAndSize, 0, count)
	now := time.Now().Unix()

	// Prune expired messages and collect IDs
	filtered := make([]*pcommon.DmqMessage, 0, len(s.messageQueue))
	for _, msg := range s.messageQueue {
		if msg == nil {
			continue
		}
		// #nosec G115 -- Unix timestamp will not overflow uint32 until year 2106
		if uint32(now) > msg.Payload.ExpiresAt {
			continue // skip expired messages, don't add to filtered
		}
		if len(ids) < count {
			messageCbor, err := cbor.Encode(*msg)
			if err != nil {
				s.Protocol.Logger().
					Warn("failed to encode message for size calculation",
						"component", "network",
						"protocol", ProtocolName,
						"role", "server",
						"connection_id", s.callbackContext.ConnectionId.String(),
						"error", err,
					)
				filtered = append(filtered, msg)
				continue
			}
			// #nosec G115 -- message size bounded by protocol limits
			ids = append(ids, pcommon.MessageIDAndSize{
				MessageID:   msg.ID(),
				SizeInBytes: uint32(len(messageCbor)),
			})
		}
		filtered = append(filtered, msg)
	}
	s.messageQueue = filtered

	return ids
}

// GetMessagesByIDs returns messages matching the provided IDs
func (s *Server) GetMessagesByIDs(ids [][]byte) []pcommon.DmqMessage {
	s.lock.Lock()
	defer s.lock.Unlock()

	messages := make([]pcommon.DmqMessage, 0, len(ids))
	idSet := make(map[string]struct{}, len(ids))
	now := time.Now().Unix()

	// Build a set of IDs for O(1) lookup
	for _, id := range ids {
		idSet[string(id)] = struct{}{}
	}

	// Check all messages once, return those matching the ID set
	for _, msg := range s.messageQueue {
		if msg == nil {
			continue
		}
		// #nosec G115 -- Unix timestamp will not overflow uint32 until year 2106
		if uint32(now) > msg.Payload.ExpiresAt {
			continue
		}
		if _, exists := idSet[string(msg.ID())]; exists {
			messages = append(messages, *msg)
		}
	}

	return messages
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeInit:
		err = s.handleInit()
	case MessageTypeReplyMessageIds:
		err = s.handleReplyMessageIds(msg)
	case MessageTypeReplyMessages:
		err = s.handleReplyMessages(msg)
	case MessageTypeDone:
		err = s.handleDone()
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

//nolint:unparam
func (s *Server) handleInit() error {
	s.Protocol.Logger().
		Debug("received init message",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	return nil
}

//nolint:unparam
func (s *Server) handleReplyMessageIds(msg protocol.Message) error {
	msgReply := msg.(*MsgReplyMessageIds)

	s.Protocol.Logger().
		Debug("received reply message IDs",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
			"message_count", len(msgReply.Messages),
		)

	s.lock.Lock()
	if s.pendingIDRequest == nil {
		s.lock.Unlock()
		return invalidMessageSubmissionMessage(
			"received message ID reply without an outstanding request",
		)
	}
	next, err := reconcileMessageIDs(
		s.pendingMessageIDs,
		*s.pendingIDRequest,
		msgReply.Messages,
		s.config.MaxUnacknowledgedMessageIDs,
	)
	if err != nil {
		s.lock.Unlock()
		return err
	}
	s.pendingMessageIDs = next
	s.pendingIDRequest = nil
	s.lock.Unlock()

	// Invoke callback
	if s.config.ReplyMessageIdsFunc != nil {
		s.config.ReplyMessageIdsFunc(s.callbackContext, msgReply.Messages)
	}

	return nil
}

func (s *Server) handleReplyMessages(msg protocol.Message) error {
	msgReply := msg.(*MsgReplyMessages)

	s.Protocol.Logger().
		Debug("received reply messages",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
			"message_count", len(msgReply.Messages),
		)

	s.lock.Lock()
	request := s.pendingMsgRequest
	if err := validateMessageReply(request, msgReply.Messages); err != nil {
		s.lock.Unlock()
		return err
	}
	fresh, err := s.admitFreshMessagesLocked(msgReply.Messages)
	s.lock.Unlock()
	if err != nil {
		return err
	}

	for i := range fresh {
		if err := s.config.TTLValidator.ValidateMessageTTL(&fresh[i]); err != nil {
			s.Protocol.Logger().
				Warn("message validation failed",
					"component", "network",
					"protocol", ProtocolName,
					"role", "server",
					"connection_id", s.callbackContext.ConnectionId.String(),
					"error", err,
				)
			return err
		}
	}
	commitAuthentication := func() error { return nil }
	if len(fresh) > 0 {
		if s.config.Authenticator == nil {
			return errors.New("dmq: message authenticator not configured")
		}
		commitAuthentication, err = s.config.Authenticator.PrepareMessages(
			fresh,
		)
		if err != nil {
			s.Protocol.Logger().
				Warn("message authentication failed",
					"component", "network",
					"protocol", ProtocolName,
					"role", "server",
					"connection_id", s.callbackContext.ConnectionId.String(),
					"error", err,
				)
			return err
		}
	}
	s.lock.Lock()
	if s.pendingMsgRequest != request {
		s.lock.Unlock()
		return invalidMessageSubmissionMessage(
			"message request changed while validating its reply",
		)
	}
	if err := commitAuthentication(); err != nil {
		s.lock.Unlock()
		return err
	}
	for i := range fresh {
		s.deliveredIDs[string(fresh[i].ID())] = fresh[i].Payload.ExpiresAt
	}
	s.pendingMsgRequest = nil
	s.lock.Unlock()

	if s.config.ReplyMessagesFunc != nil {
		s.config.ReplyMessagesFunc(s.callbackContext, fresh)
	}

	return nil
}

//nolint:unparam
func (s *Server) handleDone() error {
	s.Protocol.Logger().
		Debug("received done message",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	return nil
}

// admitFreshMessagesLocked returns the messages whose IDs were not already
// delivered on this connection. A repeated ID is dropped rather than treated
// as a protocol violation, and before authentication because the operational
// certificate it carried may have been superseded since. When the unexpired
// delivered IDs plus the fresh messages exceed the configured capacity the
// reply is refused whole, since forgetting an unexpired ID would allow its
// message to be delivered again.
func (s *Server) admitFreshMessagesLocked(
	messages []pcommon.DmqMessage,
) ([]pcommon.DmqMessage, error) {
	s.pruneDeliveredLocked(s.now())
	fresh := make([]pcommon.DmqMessage, 0, len(messages))
	for i := range messages {
		if _, ok := s.deliveredIDs[string(messages[i].ID())]; ok {
			continue
		}
		fresh = append(fresh, messages[i])
	}
	if len(s.deliveredIDs)+len(fresh) > s.config.MaxReplayEntries {
		return nil, errReplayCacheCapacityExceeded
	}
	return fresh, nil
}

func (s *Server) pruneDeliveredLocked(now time.Time) {
	nowUnix := now.Unix()
	for id, expiresAt := range s.deliveredIDs {
		if nowUnix > int64(expiresAt) {
			delete(s.deliveredIDs, id)
		}
	}
}
