package protocol

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
	MsgType  uint8
	NewState State
}

type StateMapEntry struct {
	Agency      uint
	Transitions []StateTransition
}

type StateMap map[State]StateMapEntry
