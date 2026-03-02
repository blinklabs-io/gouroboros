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

// Package localmessagenotification implements the Ouroboros local-message-notification protocol (CIP-0137)
package localmessagenotification

import (
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	ProtocolName = "LocalMessageNotification"
	ProtocolID   = 19
)

// State timeouts for Local Message Notification protocol
const (
	IdleTimeout            = 300 * time.Second // Timeout for client to send RequestMessages when idle
	BusyNonblockingTimeout = 0 * time.Second   // Non-blocking returns immediately, no timeout
	BusyBlockingTimeout    = 0 * time.Second   // Blocking waits indefinitely for messages, no timeout
)

// State machine states for Local Message Notification protocol
const (
	stateIdleId         = 1
	stateBusyNonBlockId = 2
	stateBusyBlockId    = 3
	stateDoneId         = 4
)

var (
	protocolStateIdle         = protocol.NewState(stateIdleId, "idle")
	protocolStateBusyNonBlock = protocol.NewState(
		stateBusyNonBlockId,
		"busyNonBlocking",
	)
	protocolStateBusyBlock = protocol.NewState(
		stateBusyBlockId,
		"busyBlocking",
	)
	protocolStateDone = protocol.NewState(stateDoneId, "done")
	stateMap          = protocol.StateMap{
		protocolStateIdle: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 IdleTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeRequestMessages,
					NewState: protocolStateBusyNonBlock,
					MatchFunc: func(_ any, msg protocol.Message) bool {
						msgReq := msg.(*MsgRequestMessages)
						return !msgReq.IsBlocking
					},
				},
				{
					MsgType:  MessageTypeRequestMessages,
					NewState: protocolStateBusyBlock,
					MatchFunc: func(_ any, msg protocol.Message) bool {
						msgReq := msg.(*MsgRequestMessages)
						return msgReq.IsBlocking
					},
				},
				{
					MsgType:  MessageTypeClientDone,
					NewState: protocolStateDone,
				},
			},
		},
		protocolStateBusyNonBlock: protocol.StateMapEntry{
			Agency:                  protocol.AgencyServer,
			PendingMessageByteLimit: 0,
			Timeout:                 BusyNonblockingTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeReplyMessagesNonBlocking,
					NewState: protocolStateIdle,
				},
			},
		},
		protocolStateBusyBlock: protocol.StateMapEntry{
			Agency:                  protocol.AgencyServer,
			PendingMessageByteLimit: 0,
			Timeout:                 BusyBlockingTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeReplyMessagesBlocking,
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

// LocalMessageNotification is a wrapper object that holds the client and server instances
type LocalMessageNotification struct {
	Client *Client
	Server *Server
}

// Config is used to configure the LocalMessageNotification protocol instance
type Config struct {
	// Client callbacks
	ReplyMessagesFunc ReplyMessagesFunc

	// Server configuration
	MaxQueueSize int

	// Client timeouts
	BlockingRequestTimeout time.Duration

	// Shared configuration
	Authenticator *pcommon.MessageAuthenticator
	TTLValidator  *pcommon.TTLValidator
}

// CallbackContext provides context for callback functions
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type ReplyMessagesFunc func(CallbackContext, []pcommon.DmqMessage, bool)

// New returns a new LocalMessageNotification object
func New(
	protoOptions protocol.ProtocolOptions,
	cfg *Config,
) *LocalMessageNotification {
	l := &LocalMessageNotification{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return l
}

// LocalMessageNotificationOptionFunc represents a function used to modify the LocalMessageNotification protocol config
type LocalMessageNotificationOptionFunc func(*Config)

// NewConfig returns a new LocalMessageNotification config object with the provided options
func NewConfig(options ...LocalMessageNotificationOptionFunc) Config {
	c := Config{
		MaxQueueSize: 100,
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
	// Note: The above applies defaults when Authenticator or TTLValidator are
	// nil. Explicitly setting these fields to nil via option functions will be
	// overridden by defaults here. To explicitly opt out, use the provided no-op
	// helpers: common.NewNoOpAuthenticator() and common.NewNoOpTTLValidator().
	return c
}

// WithReplyMessagesFunc specifies the callback function when messages are received when acting as a client
func WithReplyMessagesFunc(
	replyMessagesFunc ReplyMessagesFunc,
) LocalMessageNotificationOptionFunc {
	return func(c *Config) {
		c.ReplyMessagesFunc = replyMessagesFunc
	}
}

// WithMaxQueueSize specifies the maximum queue size for the server
func WithMaxQueueSize(maxQueueSize int) LocalMessageNotificationOptionFunc {
	return func(c *Config) {
		c.MaxQueueSize = maxQueueSize
	}
}

// WithBlockingRequestTimeout specifies the timeout for blocking message requests when acting as a client
func WithBlockingRequestTimeout(
	timeout time.Duration,
) LocalMessageNotificationOptionFunc {
	return func(c *Config) {
		c.BlockingRequestTimeout = timeout
	}
}

// WithAuthenticator specifies the message authenticator to use for message verification
func WithAuthenticator(
	authenticator *pcommon.MessageAuthenticator,
) LocalMessageNotificationOptionFunc {
	return func(c *Config) {
		c.Authenticator = authenticator
	}
}

// WithTTLValidator specifies the TTL validator to use for message TTL validation
func WithTTLValidator(
	ttlValidator *pcommon.TTLValidator,
) LocalMessageNotificationOptionFunc {
	return func(c *Config) {
		c.TTLValidator = ttlValidator
	}
}
