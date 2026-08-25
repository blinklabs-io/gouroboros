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

package leiosvotes

import (
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

type Client struct {
	*protocol.Protocol
	busyMutex       sync.Mutex
	callbackContext CallbackContext
	config          *Config
	onceStart       sync.Once
	onceStop        sync.Once
	sendMutex       sync.Mutex
	stopErr         error
	stopping        bool
	syncRunning     bool
	voteChan        chan Vote
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	cfg = normalizeConfig(cfg)
	c := &Client{
		config:   cfg,
		voteChan: make(chan Vote),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[StateBusy]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[StateBusy] = entry
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               protoOptions.Muxer,
		Logger:              protoOptions.Logger,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateContext:        &stateContext{},
		StateMap:            stateMap,
		InitialState:        StateIdle,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) Start() {
	c.onceStart.Do(func() {
		c.Protocol.Logger().
			Debug("starting client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		c.Protocol.Start()
	})
}

func (c *Client) Stop() error {
	c.onceStop.Do(func() {
		c.Protocol.Logger().
			Debug("stopping client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		c.sendMutex.Lock()
		defer c.sendMutex.Unlock()
		c.stopping = true
		c.stopErr = c.SendMessage(NewMsgDone())
	})
	return c.stopErr
}

func (c *Client) sendRequestNext(count uint64) error {
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()
	if c.stopping || c.IsDone() {
		return protocol.ErrProtocolShuttingDown
	}
	return c.SendMessage(NewMsgVotesRequestNext(count))
}

func (c *Client) isStopping() bool {
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()
	return c.stopping || c.IsDone()
}

func (c *Client) RequestNext(count uint64) ([]Vote, error) {
	if count == 0 {
		return nil, errors.New("request count must be positive")
	}
	if count > MaxRequestNextCount {
		return nil, fmt.Errorf(
			"request count %d exceeds maximum allowed %d",
			count,
			MaxRequestNextCount,
		)
	}
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if c.syncRunning {
		return nil, errors.New("vote sync loop is already running")
	}
	if err := c.sendRequestNext(count); err != nil {
		return nil, err
	}
	votes := make([]Vote, 0, count)
	for range count {
		select {
		case <-c.DoneChan():
			return nil, protocol.ErrProtocolShuttingDown
		case vote := <-c.voteChan:
			votes = append(votes, vote)
		}
	}
	return votes, nil
}

func (c *Client) Sync() error {
	if c.config.VoteFunc == nil {
		return errors.New("you must configure VoteFunc to receive votes")
	}
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if c.syncRunning {
		return errors.New("vote sync loop is already running")
	}
	pipelineLimit := max(c.config.PipelineLimit, 1)
	c.sendMutex.Lock()
	for range pipelineLimit {
		if c.stopping || c.IsDone() {
			c.sendMutex.Unlock()
			return protocol.ErrProtocolShuttingDown
		}
		if err := c.SendMessage(
			NewMsgVotesRequestNext(c.config.RequestNextCount),
		); err != nil {
			c.sendMutex.Unlock()
			return err
		}
	}
	c.syncRunning = true
	c.sendMutex.Unlock()
	go c.voteLoop(uint64(pipelineLimit) * c.config.RequestNextCount)
	return nil
}

func (c *Client) voteLoop(outstanding uint64) {
	defer func() {
		c.busyMutex.Lock()
		c.syncRunning = false
		c.busyMutex.Unlock()
	}()
	refillThreshold := uint64(max(c.config.PipelineLimit-1, 0)) *
		c.config.RequestNextCount
	for {
		select {
		case vote := <-c.voteChan:
			if outstanding > 0 {
				outstanding--
			}
			if err := c.config.VoteFunc(c.callbackContext, vote); err != nil {
				if errors.Is(err, ErrStopVoteProcess) {
					c.drainVotes(outstanding)
					return
				}
				c.SendError(err)
				return
			}
			if c.isStopping() {
				c.drainVotes(outstanding)
				return
			}
			if outstanding <= refillThreshold {
				if err := c.sendRequestNext(c.config.RequestNextCount); err != nil {
					if errors.Is(err, protocol.ErrProtocolShuttingDown) {
						c.drainVotes(outstanding)
						return
					}
					c.SendError(err)
					return
				}
				outstanding += c.config.RequestNextCount
			}
		case <-c.DoneChan():
			return
		}
	}
}

func (c *Client) drainVotes(remaining uint64) {
	for remaining > 0 {
		select {
		case <-c.voteChan:
			remaining--
		case <-c.DoneChan():
			return
		}
	}
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeVote:
		c.handleVote(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleVote(msg protocol.Message) {
	msgVote := msg.(*MsgVote)
	select {
	case <-c.DoneChan():
	case c.voteChan <- msgVote.Vote:
	}
}
