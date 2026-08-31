package blockfetch

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestStateMapPipeliningIsLimitedToRangeRequests(t *testing.T) {
	for _, state := range []struct {
		name  string
		state protocol.State
	}{
		{name: "busy", state: StateBusy},
		{name: "streaming", state: StateStreaming},
	} {
		t.Run(state.name, func(t *testing.T) {
			types := StateMap[state.state].PipelinedMessageTypes
			require.Equal(t, []uint8{MessageTypeRequestRange}, types)
		})
	}
	require.Empty(t, StateMap[StateIdle].PipelinedMessageTypes)
	require.Empty(t, StateMap[StateDone].PipelinedMessageTypes)
}
