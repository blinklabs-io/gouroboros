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

// ErrLocalPeerSharingDisabled is retained for compatibility with callers that
// classified the previous server-side behavior.
//
// Deprecated: the server now answers an unexpected ShareRequest with an empty
// SharePeers message so it cannot tear down the shared bearer.
var ErrLocalPeerSharingDisabled = errors.New(
	"peer sharing: received ShareRequest but local node advertised NoPeerSharing during handshake",
)

// ErrTooManyPeersShared is returned by the client when a peer answers a
// ShareRequest with more addresses than were requested. The protocol permits
// a smaller reply; a larger one is a protocol violation.
var ErrTooManyPeersShared = errors.New(
	"peer sharing: received more peer addresses than requested",
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

// MaxPendingMessageBytes is the maximum allowed pending message bytes in each
// active peer-sharing state. The value is the reference implementation's
// figure, used there both as the mini-protocol ingress queue
// (peerSharingProtocolLimits) and as the per-state codec size limit
// (byteLimitsPeerSharing): four 1440-byte TCP segments, one initial congestion
// window, so a request and its reply complete within a single round trip.
const MaxPendingMessageBytes = 4 * 1440

// MaxSharedPeers is the largest number of addresses a SharePeers message may
// carry and still encode within MaxPendingMessageBytes.
//
// A PeerAddress encodes to at most 25 bytes, its IPv6 form: a 6-element array
// header, the peer type, four uint32 words at 5 bytes each, and a 3-byte port.
// The IPv4 form is 10 bytes. A SharePeers message adds a 2-byte frame (outer
// 2-element array header plus message type) and a 2-byte address-array header
// above 23 entries, so 230 IPv6 addresses encode to 5,754 bytes and 231 to
// 5,779. ShareRequest.Amount is a uint8, so a peer may ask for 255, whose
// reply would encode to 6,379 bytes and exceed MaxPendingMessageBytes.
const MaxSharedPeers = 230

var (
	stateIdle = protocol.NewState(1, "Idle")
	stateBusy = protocol.NewState(2, "Busy")
	stateDone = protocol.NewState(3, "Done")
)

// PeerSharing protocol state machine
var StateMap = protocol.StateMap{
	stateIdle: protocol.StateMapEntry{
		Agency:                  protocol.AgencyClient,
		PendingMessageByteLimit: MaxPendingMessageBytes,
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
		Agency:                  protocol.AgencyServer,
		Timeout:                 BusyTimeout,
		PendingMessageByteLimit: MaxPendingMessageBytes,
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
// the handshake. The server answers incoming ShareRequest messages with an
// empty SharePeers response while this flag is set.
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
