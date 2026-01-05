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

// Package messagesubmission implements the Ouroboros message-submission protocol (CIP-0137)
package messagesubmission

import (
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	ProtocolName = "MessageSubmission"
	ProtocolID   = 17
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
	stateMap          = protocol.StateMap{
		protocolStateInit: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 InitTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeInit,
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
					MsgType:  MessageTypeRequestMessageIds,
					NewState: protocolStateMessageIdsBlock,
					MatchFunc: func(_ any, msg protocol.Message) bool {
						msgReq := msg.(*MsgRequestMessageIds)
						return msgReq.IsBlocking
					},
				},
				{
					MsgType:  MessageTypeRequestMessageIds,
					NewState: protocolStateMessageIdsNonBlk,
					MatchFunc: func(_ any, msg protocol.Message) bool {
						msgReq := msg.(*MsgRequestMessageIds)
						return !msgReq.IsBlocking
					},
				},
				{
					MsgType:  MessageTypeRequestMessages,
					NewState: protocolStateMessages,
				},
				{
					MsgType:  MessageTypeDone,
					NewState: protocolStateDone,
				},
			},
		},
		protocolStateMessageIdsBlock: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 MessageIdsBlockingTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeReplyMessageIds,
					NewState: protocolStateIdle,
				},
				{
					MsgType:  MessageTypeDone,
					NewState: protocolStateDone,
				},
			},
		},
		protocolStateMessageIdsNonBlk: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 MessageIdsNonblockingTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeReplyMessageIds,
					NewState: protocolStateIdle,
				},
				{
					MsgType:  MessageTypeDone,
					NewState: protocolStateDone,
				},
			},
		},
		protocolStateMessages: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 MessagesTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeReplyMessages,
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
	MaxQueueSize  int
	Authenticator *pcommon.MessageAuthenticator
	TTLValidator  *pcommon.TTLValidator
	// Done callback invoked when peer sends Done
	DoneFunc func(CallbackContext) error
}

// CallbackContext provides context for callback functions
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
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
		MaxQueueSize:                 100,
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
	if c.Authenticator == nil {
		c.Authenticator = pcommon.NewMessageAuthenticator(nil)
		pcommon.ApplyDefaultKESVerifier(c.Authenticator)
	}
	if c.TTLValidator == nil {
		c.TTLValidator = pcommon.NewTTLValidator(0, nil)
	}
	return c
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
