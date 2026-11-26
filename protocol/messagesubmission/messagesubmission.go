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

// Package messagesubmission implements the Ouroboros message-submission protocol (CIP-0137)
package messagesubmission

import (
	"context"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	ProtocolName = "MessageSubmission"
	// ProtocolID is the DMQ N2N mini-protocol number. dmq-node reference
	// (DMQ.NodeToNode: sigSubmissionMiniProtocolNum = MiniProtocolNum 11)
	// and Pallas (PROTOCOL_N2N_DMQ_SIG_SUBMISSION = 11) both use 11.
	ProtocolID = 11
)

// State timeouts for Message Submission protocol
const (
	InitTimeout                  = 30 * time.Second  // Timeout for client to send Init
	IdleTimeout                  = 300 * time.Second // Timeout for server to send RequestMessageIds when idle
	MessageIdsBlockingTimeout    = 30 * time.Second  // Timeout for client to reply to blocking RequestMessageIds
	MessageIdsNonblockingTimeout = 0 * time.Second   // Non-blocking doesn't timeout (server sends when ready)
	MessagesTimeout              = 30 * time.Second  // Timeout for client to reply with messages
	DoneTimeout                  = 10 * time.Second  // Timeout for Done state cleanup
)

// DefaultMaxUnacknowledgedMessageIDs bounds the number of message IDs that may
// be outstanding for a peer when no explicit limit is configured. It is
// dmq-node's window (maxUnacknowledgedTxIds, 4 * 33, in DMQ/Policy.hs): its
// requester fills that window, and the window is not negotiated, so a
// smaller one refuses requests a dmq-node peer is entitled to send.
const DefaultMaxUnacknowledgedMessageIDs = 132

const defaultMaxQueueSize = 100

// The default replay bound covers every message an honest dmq-node can hold
// unexpired. Its mempool admits one message per pool per minute (minSigDelay)
// expiring at most 30 minutes ahead (maxSigExpiresAtDelay, DMQ/Policy.hs), so
// at most 31 per pool. A peer can relay that whole mempool on one connection,
// so a smaller bound refuses honest replies until IDs expire.
const (
	replayEntriesPerPool    = 31
	replayPools             = 4096
	defaultMaxReplayEntries = replayEntriesPerPool * replayPools
)

// State machine states for Message Submission protocol
const (
	stateInitId               = 1
	stateIdleId               = 2
	stateMessageIdsBlockId    = 3
	stateMessageIdsNonBlockId = 4
	stateMessagesId           = 5
	stateDoneId               = 6
)

var (
	protocolStateInit            = protocol.NewState(stateInitId, "init")
	protocolStateIdle            = protocol.NewState(stateIdleId, "idle")
	protocolStateMessageIdsBlock = protocol.NewState(
		stateMessageIdsBlockId,
		"messageIdsBlocking",
	)
	protocolStateMessageIdsNonBlk = protocol.NewState(
		stateMessageIdsNonBlockId,
		"messageIdsNonBlocking",
	)
	protocolStateMessages = protocol.NewState(
		stateMessagesId,
		"messages",
	)
	protocolStateDone = protocol.NewState(stateDoneId, "done")

	// MessageSubmissionV2MinVersion is the minimum Cardano NtN protocol
	// version that uses Message Submission V2 (no StInit, MsgDone only from
	// StIdle). Standalone DMQ NodeToNodeV_2 is version 2 and is handled by
	// isMessageSubmissionV2.
	MessageSubmissionV2MinVersion uint16 = 15

	// stateMapV1 is the V1 state map: tx-submission's (ouroboros-network
	// TxSubmission2/Type.hs). The Client, which has the messages, is the
	// initiator and holds agency in StInit and while answering; MsgDone is
	// its alternative to answering a blocking request.
	stateMapV1 = protocol.StateMap{
		protocolStateInit: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 InitTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeV1Init,
					NewState: protocolStateIdle,
				},
			},
		},
		protocolStateIdle: protocol.StateMapEntry{
			Agency:                  protocol.AgencyServer,
			PendingMessageByteLimit: 0,
			Timeout:                 IdleTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeV1RequestMessageIds,
					NewState: protocolStateMessageIdsBlock,
					MatchFunc: func(_ any, msg protocol.Message) bool {
						msgReq := msg.(*MsgRequestMessageIds)
						return msgReq.IsBlocking
					},
				},
				{
					MsgType:  MessageTypeV1RequestMessageIds,
					NewState: protocolStateMessageIdsNonBlk,
					MatchFunc: func(_ any, msg protocol.Message) bool {
						msgReq := msg.(*MsgRequestMessageIds)
						return !msgReq.IsBlocking
					},
				},
				{
					MsgType:  MessageTypeV1RequestMessages,
					NewState: protocolStateMessages,
				},
			},
		},
		protocolStateMessageIdsBlock: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 MessageIdsBlockingTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeV1ReplyMessageIds,
					NewState: protocolStateIdle,
				},
				{
					MsgType:  MessageTypeV1Done,
					NewState: protocolStateDone,
				},
			},
		},
		protocolStateMessageIdsNonBlk: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 MessageIdsNonblockingTimeout,
			// dmq-node's requester pipelines a request for messages behind
			// an unanswered non-blocking request for ids; the outbound side
			// holds it until it has replied. A blocking request is never
			// pipelined, so MessageIdsBlocking declares none.
			PipelinedMessageTypes: []uint8{
				MessageTypeV1RequestMessageIds,
				MessageTypeV1RequestMessages,
			},
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeV1ReplyMessageIds,
					NewState: protocolStateIdle,
				},
			},
		},
		protocolStateMessages: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 MessagesTimeout,
			// dmq-node's requester pipelines its next request for ids or
			// for messages behind an unanswered request for messages; the
			// outbound side holds it until it has replied.
			PipelinedMessageTypes: []uint8{
				MessageTypeV1RequestMessageIds,
				MessageTypeV1RequestMessages,
			},
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeV1ReplyMessages,
					NewState: protocolStateIdle,
				},
			},
		},
		protocolStateDone: protocol.StateMapEntry{
			Agency:                  protocol.AgencyNone,
			PendingMessageByteLimit: 0,
		},
	}

	// stateMapV2 is the V2 state map (dmq-node
	// DMQ/Protocol/SigSubmissionV2/Type.hs). There is no StInit. The side
	// that wants messages is the initiator and holds agency in StIdle, so
	// AgencyClient here is the initiator's agency: the Server type runs with
	// the initiator role in V2 and the Client type with the responder role.
	// Only the requesting side sends MsgDone, from StIdle.
	stateMapV2 = protocol.StateMap{
		protocolStateIdle: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 IdleTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeV2RequestMessageIds,
					NewState: protocolStateMessageIdsBlock,
					MatchFunc: func(_ any, msg protocol.Message) bool {
						msgReq := msg.(*MsgRequestMessageIds)
						return msgReq.IsBlocking
					},
				},
				{
					MsgType:  MessageTypeV2RequestMessageIds,
					NewState: protocolStateMessageIdsNonBlk,
					MatchFunc: func(_ any, msg protocol.Message) bool {
						msgReq := msg.(*MsgRequestMessageIds)
						return !msgReq.IsBlocking
					},
				},
				{
					MsgType:  MessageTypeV2RequestMessages,
					NewState: protocolStateMessages,
				},
				{
					MsgType:  MessageTypeV2Done,
					NewState: protocolStateDone,
				},
			},
		},
		protocolStateMessageIdsBlock: protocol.StateMapEntry{
			Agency:                  protocol.AgencyServer,
			PendingMessageByteLimit: 0,
			Timeout:                 MessageIdsBlockingTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeV2ReplyMessageIds,
					NewState: protocolStateIdle,
				},
				{
					MsgType:  MessageTypeV2ReplyNoMessageIds,
					NewState: protocolStateIdle,
				},
			},
		},
		protocolStateMessageIdsNonBlk: protocol.StateMapEntry{
			Agency:                  protocol.AgencyServer,
			PendingMessageByteLimit: 0,
			Timeout:                 MessageIdsNonblockingTimeout,
			// dmq-node's requester pipelines a request for messages behind
			// an unanswered non-blocking request for ids; the outbound side
			// holds it until it has replied. A blocking request is never
			// pipelined, so MessageIdsBlocking declares none.
			PipelinedMessageTypes: []uint8{
				MessageTypeV2RequestMessageIds,
				MessageTypeV2RequestMessages,
			},
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeV2ReplyMessageIds,
					NewState: protocolStateIdle,
				},
			},
		},
		protocolStateMessages: protocol.StateMapEntry{
			Agency:                  protocol.AgencyServer,
			PendingMessageByteLimit: 0,
			Timeout:                 MessagesTimeout,
			// dmq-node's requester pipelines its next request for ids or
			// for messages behind an unanswered request for messages; the
			// outbound side holds it until it has replied.
			PipelinedMessageTypes: []uint8{
				MessageTypeV2RequestMessageIds,
				MessageTypeV2RequestMessages,
			},
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeV2ReplyMessages,
					NewState: protocolStateIdle,
				},
			},
		},
		protocolStateDone: protocol.StateMapEntry{
			Agency:                  protocol.AgencyNone,
			PendingMessageByteLimit: 0,
		},
	}
)

// roleFor maps a side to its connection role. In V1 the Client (which has the
// messages) is the initiator, as tx-submission's outbound is. In V2 the
// Server (which wants them) is the initiator, as dmq-node runs its V2 inbound
// as the initiator (DMQ/NodeToNode.hs aSigSubmissionV2Client).
func roleFor(version uint16, side protocol.ProtocolRole) protocol.ProtocolRole {
	if !isMessageSubmissionV2(version) {
		return side
	}
	if side == protocol.ProtocolRoleClient {
		return protocol.ProtocolRoleServer
	}
	return protocol.ProtocolRoleClient
}

func isMessageSubmissionV2(version uint16) bool {
	return version == protocol.ProtocolVersionDMQNtN2 ||
		version >= MessageSubmissionV2MinVersion
}

// MessageSubmission is a wrapper object that holds the client and server instances
type MessageSubmission struct {
	Client *Client
	Server *Server
}

// Config is used to configure the MessageSubmission protocol instance
type Config struct {
	// Client callbacks
	RequestMessageIdsFunc RequestMessageIdsFunc
	RequestMessagesFunc   RequestMessagesFunc

	// Server callbacks
	ReplyMessageIdsFunc ReplyMessageIdsFunc
	ReplyMessagesFunc   ReplyMessagesFunc

	// Client timeouts (use defaults from state machine constants if not set)
	InitTimeout                  time.Duration
	IdleTimeout                  time.Duration
	MessageIdsBlockingTimeout    time.Duration
	MessageIdsNonblockingTimeout time.Duration
	MessagesTimeout              time.Duration
	DoneTimeout                  time.Duration

	// Shared configuration
	MaxQueueSize                int
	MaxUnacknowledgedMessageIDs int
	// MaxReplayEntries bounds the message IDs a server retains, per
	// connection, as delivered until their signed expiration time. A message
	// whose ID is retained is dropped instead of being delivered again. An
	// ID is never evicted before its message expires, so a reply whose fresh
	// messages would exceed the bound is refused with an error, which closes
	// the connection. The default holds 31 unexpired messages per stake pool,
	// the most a dmq-node admits, for 4096 pools. Zero or a negative value
	// selects the default.
	MaxReplayEntries int
	Authenticator    *pcommon.MessageAuthenticator
	TTLValidator     *pcommon.TTLValidator
	// LegacyV1MessageEncoding preserves the pre-CIP-0137-v4 V1 message
	// wire shape for peers that still expect messageId inside messagePayload.
	// The default false value uses the current CIP-0137 message shape.
	LegacyV1MessageEncoding bool
	// Done callback invoked when peer sends Done
	DoneFunc func(CallbackContext) error
}

// CallbackContext provides context for callback functions
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
	// ctx is returned by Context.
	ctx context.Context
}

// Context returns the context of this callback. A callback that stops its
// own protocol, or closes its own connection, passes it to that Stop or
// Close: the call then signals shutdown and returns without waiting for the
// goroutine running the callback. It carries no deadline and is never
// cancelled. It is context.Background for a CallbackContext that the package
// did not build.
func (c CallbackContext) Context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// withOwner returns c with the context of a callback that p runs.
func (c CallbackContext) withOwner(p *protocol.Protocol) CallbackContext {
	c.ctx = p.CallbackContext()
	return c
}

// Callback function types
type (
	RequestMessageIdsFunc func(CallbackContext, bool, uint16, uint16)
	RequestMessagesFunc   func(CallbackContext, [][]byte)
	ReplyMessageIdsFunc   func(CallbackContext, []pcommon.MessageIDAndSize)
	ReplyMessagesFunc     func(CallbackContext, []pcommon.DmqMessage)
)

// New returns a new MessageSubmission object
func New(
	protoOptions protocol.ProtocolOptions,
	cfg *Config,
) *MessageSubmission {
	m := &MessageSubmission{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return m
}

// MessageSubmissionOptionFunc represents a function used to modify the MessageSubmission protocol config
type MessageSubmissionOptionFunc func(*Config)

// NewConfig returns a new MessageSubmission config object with the provided options
func NewConfig(options ...MessageSubmissionOptionFunc) Config {
	c := Config{
		MaxQueueSize:                 defaultMaxQueueSize,
		MaxUnacknowledgedMessageIDs:  DefaultMaxUnacknowledgedMessageIDs,
		MaxReplayEntries:             defaultMaxReplayEntries,
		InitTimeout:                  InitTimeout,
		IdleTimeout:                  IdleTimeout,
		MessageIdsBlockingTimeout:    MessageIdsBlockingTimeout,
		MessageIdsNonblockingTimeout: MessageIdsNonblockingTimeout,
		MessagesTimeout:              MessagesTimeout,
		DoneTimeout:                  DoneTimeout,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	// Set defaults
	// Authenticator has no default: constructing a real one requires a
	// StakeAuthority backed by live chain state, which this package has no
	// access to. A nil Authenticator is a deliberate fail-closed
	// configuration error (see server.go) rather than a silent skip of
	// verification -- callers must explicitly supply one, or
	// common.NewNoOpAuthenticator() to intentionally disable authentication.
	if c.TTLValidator == nil {
		c.TTLValidator = pcommon.NewTTLValidator(0, nil)
	}
	return c
}

func normalizeConfig(cfg *Config) Config {
	if cfg == nil {
		return NewConfig()
	}
	ret := *cfg
	if ret.MaxQueueSize <= 0 {
		ret.MaxQueueSize = defaultMaxQueueSize
	}
	if ret.MaxUnacknowledgedMessageIDs <= 0 {
		ret.MaxUnacknowledgedMessageIDs = DefaultMaxUnacknowledgedMessageIDs
	}
	if ret.MaxReplayEntries <= 0 {
		ret.MaxReplayEntries = defaultMaxReplayEntries
	}
	if ret.InitTimeout == 0 {
		ret.InitTimeout = InitTimeout
	}
	if ret.IdleTimeout == 0 {
		ret.IdleTimeout = IdleTimeout
	}
	if ret.MessageIdsBlockingTimeout == 0 {
		ret.MessageIdsBlockingTimeout = MessageIdsBlockingTimeout
	}
	if ret.MessagesTimeout == 0 {
		ret.MessagesTimeout = MessagesTimeout
	}
	if ret.DoneTimeout == 0 {
		ret.DoneTimeout = DoneTimeout
	}
	if ret.TTLValidator == nil {
		ret.TTLValidator = pcommon.NewTTLValidator(0, nil)
	}
	return ret
}

// WithRequestMessageIdsFunc specifies the callback function when message IDs are requested when acting as a client
func WithRequestMessageIdsFunc(
	requestMessageIdsFunc RequestMessageIdsFunc,
) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.RequestMessageIdsFunc = requestMessageIdsFunc
	}
}

// WithRequestMessagesFunc specifies the callback function when messages are requested when acting as a client
func WithRequestMessagesFunc(
	requestMessagesFunc RequestMessagesFunc,
) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.RequestMessagesFunc = requestMessagesFunc
	}
}

// WithReplyMessageIdsFunc specifies the callback function when message IDs are received when acting as a server
func WithReplyMessageIdsFunc(
	replyMessageIdsFunc ReplyMessageIdsFunc,
) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.ReplyMessageIdsFunc = replyMessageIdsFunc
	}
}

// WithReplyMessagesFunc specifies the callback function when messages are received when acting as a server
func WithReplyMessagesFunc(
	replyMessagesFunc ReplyMessagesFunc,
) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.ReplyMessagesFunc = replyMessagesFunc
	}
}

// WithMaxQueueSize specifies the maximum queue size for the server
func WithMaxQueueSize(maxQueueSize int) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.MaxQueueSize = maxQueueSize
	}
}

// WithMaxUnacknowledgedMessageIDs sets the maximum IDs outstanding with a peer.
func WithMaxUnacknowledgedMessageIDs(max int) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.MaxUnacknowledgedMessageIDs = max
	}
}

// WithMaxReplayEntries sets the maximum delivered message IDs a server retains
// per connection until their messages expire.
func WithMaxReplayEntries(maxReplayEntries int) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.MaxReplayEntries = maxReplayEntries
	}
}

// WithInitTimeout specifies the timeout for the Init state
func WithInitTimeout(timeout time.Duration) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.InitTimeout = timeout
	}
}

// WithIdleTimeout specifies the timeout for the Idle state
func WithIdleTimeout(timeout time.Duration) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.IdleTimeout = timeout
	}
}

// WithMessageIdsBlockingTimeout specifies the timeout for blocking MessageIds requests
func WithMessageIdsBlockingTimeout(
	timeout time.Duration,
) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.MessageIdsBlockingTimeout = timeout
	}
}

// WithMessageIdsNonblockingTimeout specifies the timeout for non-blocking MessageIds requests
func WithMessageIdsNonblockingTimeout(
	timeout time.Duration,
) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.MessageIdsNonblockingTimeout = timeout
	}
}

// WithMessagesTimeout specifies the timeout for the Messages state
func WithMessagesTimeout(timeout time.Duration) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.MessagesTimeout = timeout
	}
}

// WithDoneTimeout specifies the timeout for the Done state
func WithDoneTimeout(timeout time.Duration) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.DoneTimeout = timeout
	}
}

// WithAuthenticator specifies the message authenticator to use for message verification
func WithAuthenticator(
	authenticator *pcommon.MessageAuthenticator,
) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.Authenticator = authenticator
	}
}

// WithTTLValidator specifies the TTL validator to use for message TTL validation
func WithTTLValidator(
	ttlValidator *pcommon.TTLValidator,
) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.TTLValidator = ttlValidator
	}
}

// WithLegacyV1MessageEncoding enables the pre-CIP-0137-v4 V1 message wire
// shape when replying with messages on a V1 Message Submission connection.
func WithLegacyV1MessageEncoding(enabled bool) MessageSubmissionOptionFunc {
	return func(c *Config) {
		c.LegacyV1MessageEncoding = enabled
	}
}
