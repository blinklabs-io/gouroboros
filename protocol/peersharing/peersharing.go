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

// Package peersharing implements the Ouroboros PeerSharing protocol
package peersharing

import (
	"errors"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// ErrRemotePeerSharingDisabled is returned by Client.GetPeers when the remote
// peer advertised NoPeerSharing during the handshake. Sending a request in
// that case would be a protocol violation.
var ErrRemotePeerSharingDisabled = errors.New(
	"peer sharing: remote peer advertised NoPeerSharing during handshake",
)

// ErrLocalPeerSharingDisabled is returned by the server when a peer sends a
// ShareRequest but we advertised NoPeerSharing during the handshake. A
// spec-compliant peer must not send ShareRequest in that case.
var ErrLocalPeerSharingDisabled = errors.New(
	"peer sharing: received ShareRequest but local node advertised NoPeerSharing during handshake",
)

// Protocol identifiers
const (
	ProtocolName = "peer-sharing"
	ProtocolId   = 10
)

// Protocol state timeout constants per Ouroboros Network Specification (Table 3.15).
const (
	BusyTimeout = 60 * time.Second // Timeout for server to respond with peers
)

var (
	stateIdle = protocol.NewState(1, "Idle")
	stateBusy = protocol.NewState(2, "Busy")
	stateDone = protocol.NewState(3, "Done")
)

// PeerSharing protocol state machine
var StateMap = protocol.StateMap{
	stateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeShareRequest,
				NewState: stateBusy,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: stateDone,
			},
		},
	},
	stateBusy: protocol.StateMapEntry{
		Agency:  protocol.AgencyServer,
		Timeout: BusyTimeout,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeSharePeers,
				NewState: stateIdle,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// PeerSharing is a wrapper object that holds the client and server instances
type PeerSharing struct {
	Client *Client
	Server *Server
}

// Config is used to configure the PeerSharing protocol instance.
//
// LocalDisabled and RemoteDisabled reflect the outcome of the handshake's
// PeerSharing-mode negotiation and are populated by the connection layer when
// either side advertised NoPeerSharing. The zero value (false) preserves
// legacy permissive behaviour, so an operator-supplied Config (via
// WithPeerSharingConfig) and direct callers of New that do not perform a
// handshake do not need to set them.
type Config struct {
	ShareRequestFunc ShareRequestFunc
	Timeout          time.Duration
	LocalDisabled    bool
	RemoteDisabled   bool
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type ShareRequestFunc func(CallbackContext, int) ([]PeerAddress, error)

// New returns a new PeerSharing object
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *PeerSharing {
	h := &PeerSharing{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return h
}

// PeerSharingOptionFunc represents a function used to modify the PeerSharing protocol config
type PeerSharingOptionFunc func(*Config)

// NewConfig returns a new PeerSharing config object with the provided options
func NewConfig(options ...PeerSharingOptionFunc) Config {
	c := Config{
		Timeout: BusyTimeout,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

// WithShareRequestFunc specifies the ShareRequest callback function
func WithShareRequestFunc(
	shareRequestFunc ShareRequestFunc,
) PeerSharingOptionFunc {
	return func(c *Config) {
		c.ShareRequestFunc = shareRequestFunc
	}
}

// WithTimeout specifies the timeout for the handshake operation
func WithTimeout(timeout time.Duration) PeerSharingOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithLocalDisabled records that this node advertised NoPeerSharing during
// the handshake. The server uses this to refuse incoming ShareRequest
// messages with ErrLocalPeerSharingDisabled.
func WithLocalDisabled(disabled bool) PeerSharingOptionFunc {
	return func(c *Config) {
		c.LocalDisabled = disabled
	}
}

// WithRemoteDisabled records that the remote peer advertised NoPeerSharing
// during the handshake. The client uses this to refuse to send ShareRequest
// messages with ErrRemotePeerSharingDisabled.
func WithRemoteDisabled(disabled bool) PeerSharingOptionFunc {
	return func(c *Config) {
		c.RemoteDisabled = disabled
	}
}
