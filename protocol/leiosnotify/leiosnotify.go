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

package leiosnotify

import (
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

const (
	ProtocolName        = "leios-notify"
	ProtocolId   uint16 = 18
)

var (
	StateIdle = protocol.NewState(1, "Idle")
	StateBusy = protocol.NewState(2, "Busy")
	StateDone = protocol.NewState(3, "Done")
)

// StateMap is the LeiosNotify state machine. Every state carries
// DefaultMaxPendingBytes as its pending-message byte limit; a client or
// server sizes its own copy from Config.MaxPendingBytes.
var StateMap = protocol.StateMap{
	StateIdle: protocol.StateMapEntry{
		Agency:                  protocol.AgencyClient,
		PendingMessageByteLimit: DefaultMaxPendingBytes,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeNotificationRequestNext,
				NewState: StateBusy,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: StateDone,
			},
		},
	},
	StateBusy: protocol.StateMapEntry{
		Agency:                  protocol.AgencyServer,
		PendingMessageByteLimit: DefaultMaxPendingBytes,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeBlockAnnouncement,
				NewState: StateIdle,
			},
			{
				MsgType:  MessageTypeBlockOffer,
				NewState: StateIdle,
			},
			{
				MsgType:  MessageTypeBlockTxsOffer,
				NewState: StateIdle,
			},
			{
				MsgType:  MessageTypeVotesOffer,
				NewState: StateIdle,
			},
		},
	},
	StateDone: protocol.StateMapEntry{
		Agency:                  protocol.AgencyNone,
		PendingMessageByteLimit: DefaultMaxPendingBytes,
	},
}

type LeiosNotify struct {
	Client *Client
	Server *Server
}

type Config struct {
	NotificationFunc NotificationFunc
	PipelineLimit    int
	// MaxPendingBytes bounds the encoded bytes of received messages queued
	// for their handler, and is the largest message accepted. While it is
	// held the protocol takes no more from the muxer, whose ingress queue
	// for this protocol is limited to the same value, or ten maximum-size
	// segments if that is larger. Ingress past that fails the connection.
	// The client therefore holds at most about twice MaxPendingBytes, plus
	// 24 whole segments in delivery, in the read loop and in
	// NotificationFunc (see README.md). It also sets the receive queue
	// length. Zero sizes it to hold PipelineLimit
	// block announcements.
	MaxPendingBytes  int
	ResponseSentFunc ResponseSentFunc
	RequestNextFunc  RequestNextFunc
	Timeout          time.Duration
}

const (
	MaxPipelineLimit     = 100 // Max pipelined requests
	DefaultPipelineLimit = 10  // Default pipeline limit
	// DefaultMaxPendingBytes is the MaxPendingBytes derived for
	// DefaultPipelineLimit.
	DefaultMaxPendingBytes = DefaultPipelineLimit * MaxBlockAnnouncementBytes
)

// pendingBytes returns the aggregate byte budget for received messages: the
// configured MaxPendingBytes, or enough for PipelineLimit block
// announcements and never less than the largest notification.
func (c *Config) pendingBytes() int {
	if c.MaxPendingBytes > 0 {
		return c.MaxPendingBytes
	}
	return max(
		max(c.PipelineLimit, 1)*MaxBlockAnnouncementBytes,
		MaxVotesOfferBytes,
	)
}

// protocolConfig applies the byte budget to a copy of StateMap and sizes the
// receive queue from it, so the pipeline, the queue and the retained bytes
// share one bound.
func (c *Config) protocolConfig(
	protoConfig protocol.ProtocolConfig,
) protocol.ProtocolConfig {
	budget := c.pendingBytes()
	stateMap := StateMap.Copy()
	for state, entry := range stateMap {
		entry.PendingMessageByteLimit = budget
		stateMap[state] = entry
	}
	protoConfig.StateMap = stateMap
	protoConfig.RecvQueueSize = max(budget/MaxBlockAnnouncementBytes, 1)
	return protoConfig
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	// ConnectionDoneChan is closed when the owning connection begins shutdown.
	// It is distinct from the current protocol instance's DoneChan and is nil
	// when the protocol was constructed without an owning connection.
	ConnectionDoneChan <-chan any
	Client             *Client
	Server             *Server
}

// Callback function types
type (
	RequestNextFunc  func(CallbackContext) (protocol.Message, error)
	ResponseSentFunc func(CallbackContext, protocol.Message, error)
	NotificationFunc func(CallbackContext, protocol.Message) error
)

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *LeiosNotify {
	b := &LeiosNotify{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return b
}

type LeiosNotifyOptionFunc func(*Config)

func NewConfig(options ...LeiosNotifyOptionFunc) Config {
	c := Config{
		Timeout:       60 * time.Second,
		PipelineLimit: DefaultPipelineLimit,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	// Validate configuration against protocol limits
	if err := c.validate(); err != nil {
		panic("invalid LeiosNotify configuration: " + err.Error())
	}

	return c
}

// validate checks that the configuration values are within protocol limits
func (c *Config) validate() error {
	if c.PipelineLimit < 0 {
		return fmt.Errorf(
			"PipelineLimit %d must be non-negative",
			c.PipelineLimit,
		)
	}
	if c.PipelineLimit > MaxPipelineLimit {
		return fmt.Errorf(
			"PipelineLimit %d exceeds maximum allowed %d",
			c.PipelineLimit,
			MaxPipelineLimit,
		)
	}
	if c.MaxPendingBytes < 0 {
		return fmt.Errorf(
			"MaxPendingBytes %d must be non-negative",
			c.MaxPendingBytes,
		)
	}
	if c.MaxPendingBytes > 0 {
		if c.MaxPendingBytes < MaxVotesOfferBytes {
			return fmt.Errorf(
				"MaxPendingBytes %d is below the largest notification (%d bytes)",
				c.MaxPendingBytes,
				MaxVotesOfferBytes,
			)
		}
		need := c.PipelineLimit * MaxBlockAnnouncementBytes
		if c.MaxPendingBytes < need {
			return fmt.Errorf(
				"MaxPendingBytes %d cannot hold %d block announcements (%d bytes)",
				c.MaxPendingBytes,
				c.PipelineLimit,
				need,
			)
		}
	}
	return nil
}

// WithNotificationFunc sets the callback function invoked for each notification received from the server
func WithNotificationFunc(
	notificationFunc NotificationFunc,
) LeiosNotifyOptionFunc {
	return func(c *Config) {
		c.NotificationFunc = notificationFunc
	}
}

// WithPipelineLimit specifies the maximum number of notification requests to pipeline
func WithPipelineLimit(limit int) LeiosNotifyOptionFunc {
	return func(c *Config) {
		if limit < 0 {
			panic(
				fmt.Sprintf(
					"PipelineLimit %d must be non-negative",
					limit,
				),
			)
		}
		if limit > MaxPipelineLimit {
			panic(
				fmt.Sprintf(
					"PipelineLimit %d exceeds maximum %d",
					limit,
					MaxPipelineLimit,
				),
			)
		}
		c.PipelineLimit = limit
	}
}

// WithMaxPendingBytes sets the aggregate byte budget for received messages.
// See Config.MaxPendingBytes.
func WithMaxPendingBytes(maxPendingBytes int) LeiosNotifyOptionFunc {
	return func(c *Config) {
		c.MaxPendingBytes = maxPendingBytes
	}
}

func WithRequestNextFunc(
	requestNextFunc RequestNextFunc,
) LeiosNotifyOptionFunc {
	return func(c *Config) {
		c.RequestNextFunc = requestNextFunc
	}
}

// WithResponseSentFunc registers a callback invoked after the server attempts
// to send a response returned by RequestNextFunc. The callback receives the
// send result so notification sources can commit or release delivery
// reservations without advancing them before transport delivery.
func WithResponseSentFunc(
	responseSentFunc ResponseSentFunc,
) LeiosNotifyOptionFunc {
	return func(c *Config) {
		c.ResponseSentFunc = responseSentFunc
	}
}

func WithTimeout(timeout time.Duration) LeiosNotifyOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
