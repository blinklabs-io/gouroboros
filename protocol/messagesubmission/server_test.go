package messagesubmission

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

func TestServerGetAvailableMessageIDsDoNotBecomePeerOutstanding(t *testing.T) {
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
	id1 := testMessageID(0xa1)
	id2 := testMessageID(0xb2)
	msg1 := &pcommon.DmqMessage{
		Payload: pcommon.DmqMessagePayload{
			MessageID:   id1,
			MessageBody: []byte("body1"),
			KESPeriod:   1,
			ExpiresAt:   exp,
		},
	}
	msg2 := &pcommon.DmqMessage{
		Payload: pcommon.DmqMessagePayload{
			MessageID:   id1,
			MessageBody: []byte("body1"),
			KESPeriod:   1,
			ExpiresAt:   exp,
		},
	}
	msg3 := &pcommon.DmqMessage{
		Payload: pcommon.DmqMessagePayload{
			MessageID:   id2,
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
	if len(ids1) != 3 {
		t.Fatalf("expected three queued IDs, got %d", len(ids1))
	}
	if len(s.pendingMessageIDs) != 0 {
		t.Fatalf("local queue IDs became peer outstanding: %x", s.pendingMessageIDs)
	}
	if err := s.RequestMessages([][]byte{ids1[0].MessageID}); !errors.Is(err, protocol.ErrProtocolViolationRequestExceeded) {
		t.Fatalf("RequestMessages error = %v, want outstanding-window rejection", err)
	}

	ids2 := s.GetAvailableMessageIDs(10)
	if len(ids2) != 3 || len(s.pendingMessageIDs) != 0 {
		t.Fatalf("second queue read changed peer outstanding IDs: ids=%d pending=%x", len(ids2), s.pendingMessageIDs)
	}
}
