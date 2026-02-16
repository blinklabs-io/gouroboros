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

var StateMap = protocol.StateMap{
	StateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
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
		Agency: protocol.AgencyServer,
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
		Agency: protocol.AgencyNone,
	},
}

type LeiosNotify struct {
	Client *Client
	Server *Server
}

type Config struct {
	NotificationFunc NotificationFunc
	PipelineLimit    int
	RequestNextFunc  RequestNextFunc
	Timeout          time.Duration
}

const (
	MaxPipelineLimit     = 100 // Max pipelined requests
	DefaultPipelineLimit = 10  // Default pipeline limit
)

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type (
	RequestNextFunc  func(CallbackContext) (protocol.Message, error)
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
	return nil
}

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

func WithRequestNextFunc(
	requestNextFunc RequestNextFunc,
) LeiosNotifyOptionFunc {
	return func(c *Config) {
		c.RequestNextFunc = requestNextFunc
	}
}

func WithTimeout(timeout time.Duration) LeiosNotifyOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
