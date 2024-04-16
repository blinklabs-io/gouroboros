// Copyright 2023 Blink Labs Software
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

package protocol

import (
	"time"
)

// ProtocolStateAgency is an enum representing the possible protocol state agency values
type ProtocolStateAgency uint

const (
	AgencyNone   ProtocolStateAgency = 0 // Default (invalid) value
	AgencyClient ProtocolStateAgency = 1 // Client agency
	AgencyServer ProtocolStateAgency = 2 // Server agency
)

// State represents protocol state with both a numeric ID and a string identifer
type State struct {
	Id   uint
	Name string
}

// NewState returns a new State object with the provided numeric ID and string identifier
func NewState(id uint, name string) State {
	return State{
		Id:   id,
		Name: name,
	}
}

// String returns the state string identifier
func (s State) String() string {
	return s.Name
}

// StateTransition represents a protocol state transition
type StateTransition struct {
	MsgType   uint8
	NewState  State
	MatchFunc StateTransitionMatchFunc
}

// StateTransitionMatchFunc represents a function that will take a Message and return a bool
// that indicates whether the message is a match for the state transition rule
type StateTransitionMatchFunc func(interface{}, Message) bool

// StateMapEntry represents a protocol state, it's possible state transitions, and an optional timeout
type StateMapEntry struct {
	Agency      ProtocolStateAgency
	Transitions []StateTransition
	Timeout     time.Duration
}

// StateMap represents the state machine definition for a mini-protocol
type StateMap map[State]StateMapEntry

// Copy returns a copy of the state map. This is mostly for convenience,
// since we need to copy the state map in various places
func (s StateMap) Copy() StateMap {
	ret := StateMap{}
	for k, v := range s {
		ret[k] = v
	}
	return ret
}
