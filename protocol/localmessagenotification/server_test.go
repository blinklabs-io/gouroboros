package localmessagenotification

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestServerStopUnblocksWaitingRequest(t *testing.T) {
	server := NewServer(protocol.ProtocolOptions{}, nil)
	result := make(chan error, 1)
	go func() { result <- server.WaitForMessage(0) }()

	server.stop()
	select {
	case err := <-result:
		require.ErrorContains(t, err, "server shutting down")
	case <-time.After(time.Second):
		t.Fatal("waiting request was not cancelled")
	}
}

func TestServerStopStopsExpirationCleanerBeforeStart(t *testing.T) {
	server := NewServer(protocol.ProtocolOptions{}, nil)
	server.stop()

	select {
	case <-server.expirationStopChan:
	default:
		t.Fatal("expiration cleaner was not stopped")
	}
}
