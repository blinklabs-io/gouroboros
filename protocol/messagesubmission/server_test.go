package messagesubmission

import (
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

func TestServerGetAvailableMessageIDs_DedupAcrossCalls(t *testing.T) {
	cfg := NewConfig()
	// Disable validation to simplify queueing in this unit test
	cfg.Authenticator = pcommon.NewNoOpAuthenticator(nil)
	cfg.TTLValidator = pcommon.NewNoOpTTLValidator(nil)
	// Provide a non-nil ConnectionId to satisfy logging code paths
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	s := NewServer(protocol.ProtocolOptions{ConnectionId: connId}, &cfg)
	// Add two identical messages (same ID) and one different
	exp := uint32(time.Now().Unix() + 60)
	msg1 := &pcommon.DmqMessage{
		Payload: pcommon.DmqMessagePayload{
			MessageID:   []byte("id1"),
			MessageBody: []byte("body1"),
			KESPeriod:   1,
			ExpiresAt:   exp,
		},
	}
	msg2 := &pcommon.DmqMessage{
		Payload: pcommon.DmqMessagePayload{
			MessageID:   []byte("id1"),
			MessageBody: []byte("body1"),
			KESPeriod:   1,
			ExpiresAt:   exp,
		},
	}
	msg3 := &pcommon.DmqMessage{
		Payload: pcommon.DmqMessagePayload{
			MessageID:   []byte("id2"),
			MessageBody: []byte("body2"),
			KESPeriod:   1,
			ExpiresAt:   exp,
		},
	}
	if err := s.AddMessage(msg1); err != nil {
		t.Fatalf("failed to add msg1: %v", err)
	}
	if err := s.AddMessage(msg2); err != nil {
		t.Fatalf("failed to add msg2: %v", err)
	}
	if err := s.AddMessage(msg3); err != nil {
		t.Fatalf("failed to add msg3: %v", err)
	}

	ids1 := s.GetAvailableMessageIDs(10)
	if len(ids1) == 0 {
		t.Fatalf("expected some IDs, got 0")
	}
	// pendingMessageIDs should not contain duplicates across calls
	seen := map[string]int{}
	for _, b := range s.pendingMessageIDs {
		seen[string(b)]++
	}
	for k, count := range seen {
		if count > 1 {
			t.Fatalf(
				"duplicate pendingMessageID %s count=%d; expected deduplication",
				k,
				count,
			)
		}
	}
}
