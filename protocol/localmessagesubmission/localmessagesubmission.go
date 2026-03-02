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

// Package localmessagesubmission implements the Ouroboros local-message-submission protocol (CIP-0137)
package localmessagesubmission

import (
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	ProtocolName = "LocalMessageSubmission"
	ProtocolID   = 18
)

// State timeouts for Local Message Submission protocol
const (
	IdleTimeout = 300 * time.Second // Timeout for client to send SubmitMessage when idle
	BusyTimeout = 30 * time.Second  // Timeout for server to reply with Accept/Reject
)

// State machine states for Local Message Submission protocol
const (
	stateIdleId = 1
	stateBusyId = 2
	stateDoneId = 3
)

var (
	protocolStateIdle = protocol.NewState(stateIdleId, "idle")
	protocolStateBusy = protocol.NewState(stateBusyId, "busy")
	protocolStateDone = protocol.NewState(stateDoneId, "done")
	stateMap          = protocol.StateMap{
		protocolStateIdle: protocol.StateMapEntry{
			Agency:                  protocol.AgencyClient,
			PendingMessageByteLimit: 0,
			Timeout:                 IdleTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeSubmitMessage,
					NewState: protocolStateBusy,
				},
				{
					MsgType:  MessageTypeDone,
					NewState: protocolStateDone,
				},
			},
		},
		protocolStateBusy: protocol.StateMapEntry{
			Agency:                  protocol.AgencyServer,
			PendingMessageByteLimit: 0,
			Timeout:                 BusyTimeout,
			Transitions: []protocol.StateTransition{
				{
					MsgType:  MessageTypeAcceptMessage,
					NewState: protocolStateIdle,
				},
				{
					MsgType:  MessageTypeRejectMessage,
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

// LocalMessageSubmission is a wrapper object that holds the client and server instances
type LocalMessageSubmission struct {
	Client *Client
	Server *Server
}

// Config is used to configure the LocalMessageSubmission protocol instance
type Config struct {
	// Client callbacks
	AcceptMessageFunc AcceptMessageFunc
	RejectMessageFunc RejectMessageFunc

	// Server callbacks
	SubmitMessageFunc SubmitMessageFunc

	// Shared configuration
	Authenticator *pcommon.MessageAuthenticator
	TTLValidator  *pcommon.TTLValidator
	Timeout       time.Duration
}

// CallbackContext provides context for callback functions
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type (
	SubmitMessageFunc func(CallbackContext, *pcommon.DmqMessage) pcommon.RejectReason
	AcceptMessageFunc func(CallbackContext)
	RejectMessageFunc func(CallbackContext, pcommon.RejectReason)
)

// New returns a new LocalMessageSubmission object
func New(
	protoOptions protocol.ProtocolOptions,
	cfg *Config,
) *LocalMessageSubmission {
	l := &LocalMessageSubmission{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return l
}

// LocalMessageSubmissionOptionFunc represents a function used to modify the LocalMessageSubmission protocol config
type LocalMessageSubmissionOptionFunc func(*Config)

// NewConfig returns a new LocalMessageSubmission config object with the provided options
func NewConfig(options ...LocalMessageSubmissionOptionFunc) Config {
	c := Config{
		Timeout: BusyTimeout,
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
	// overridden by defaults here; opt-out of defaults is not currently
	// supported. If callers need to explicitly disable validation/authentication
	// they should pass a no-op implementation instead of nil.
	return c
}

// WithSubmitMessageFunc specifies the callback function when a message is submitted when acting as a server
func WithSubmitMessageFunc(
	submitMessageFunc SubmitMessageFunc,
) LocalMessageSubmissionOptionFunc {
	return func(c *Config) {
		c.SubmitMessageFunc = submitMessageFunc
	}
}

// WithAcceptMessageFunc specifies the callback function when a message is accepted when acting as a client
func WithAcceptMessageFunc(
	acceptMessageFunc AcceptMessageFunc,
) LocalMessageSubmissionOptionFunc {
	return func(c *Config) {
		c.AcceptMessageFunc = acceptMessageFunc
	}
}

// WithRejectMessageFunc specifies the callback function when a message is rejected when acting as a client
func WithRejectMessageFunc(
	rejectMessageFunc RejectMessageFunc,
) LocalMessageSubmissionOptionFunc {
	return func(c *Config) {
		c.RejectMessageFunc = rejectMessageFunc
	}
}

// WithAuthenticator specifies the message authenticator to use for message verification
func WithAuthenticator(
	authenticator *pcommon.MessageAuthenticator,
) LocalMessageSubmissionOptionFunc {
	return func(c *Config) {
		c.Authenticator = authenticator
	}
}

// WithTTLValidator specifies the TTL validator to use for message TTL validation
func WithTTLValidator(
	ttlValidator *pcommon.TTLValidator,
) LocalMessageSubmissionOptionFunc {
	return func(c *Config) {
		c.TTLValidator = ttlValidator
	}
}

// WithTimeout specifies the timeout for message submit operations when acting as a client
func WithTimeout(timeout time.Duration) LocalMessageSubmissionOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
