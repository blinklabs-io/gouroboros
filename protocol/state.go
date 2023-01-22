package protocol

import (
	"time"
)

const (
	AGENCY_NONE   uint = 0
	AGENCY_CLIENT uint = 1
	AGENCY_SERVER uint = 2
)

type State struct {
	Id   uint
	Name string
}

func NewState(id uint, name string) State {
	return State{
		Id:   id,
		Name: name,
	}
}

func (s State) String() string {
	return s.Name
}

type StateTransition struct {
	MsgType   uint8
	NewState  State
	MatchFunc StateTransitionMatchFunc
}

type StateTransitionMatchFunc func(Message) bool

type StateMapEntry struct {
	Agency      uint
	Transitions []StateTransition
	Timeout     time.Duration
}

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
