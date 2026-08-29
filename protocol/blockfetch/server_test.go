package blockfetch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServerStateMapUsesInstanceTimeouts(t *testing.T) {
	cfg := &Config{
		BatchStartTimeout: 13 * time.Second,
		BlockTimeout:      17 * time.Second,
	}

	got := serverStateMap(cfg)
	require.Equal(t, 13*time.Second, got[StateBusy].Timeout)
	require.Equal(t, 17*time.Second, got[StateStreaming].Timeout)
	require.Equal(t, BusyTimeout, StateMap[StateBusy].Timeout)
	require.Equal(t, StreamingTimeout, StateMap[StateStreaming].Timeout)
}

func TestServerStateMapsAreIndependent(t *testing.T) {
	first := serverStateMap(&Config{BlockTimeout: 2 * time.Second})
	second := serverStateMap(&Config{BlockTimeout: 3 * time.Second})

	require.Equal(t, 2*time.Second, first[StateStreaming].Timeout)
	require.Equal(t, 3*time.Second, second[StateStreaming].Timeout)
}
