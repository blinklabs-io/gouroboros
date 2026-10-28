package messagesubmission

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

type serverTestStakeAuthority struct{}

func (serverTestStakeAuthority) PoolActiveStake(
	pcommon.PoolKeyHash,
) (uint64, error) {
	return 1, nil
}

func serverTestSignedMessages(
	t *testing.T,
	bodies []string,
	issueNumbers []uint64,
) ([]pcommon.DmqMessage, *pcommon.MessageAuthenticator) {
	t.Helper()
	require.Len(t, issueNumbers, len(bodies))
	coldPub, coldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	seed := make([]byte, kes.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	kesKey, kesPub, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)

	messages := make([]pcommon.DmqMessage, len(bodies))
	for i := range bodies {
		payload := pcommon.DmqMessagePayload{
			MessageBody: []byte(bodies[i]),
			KESPeriod:   1,
			ExpiresAt: uint32(
				time.Now().Add(10 * time.Minute).Unix(),
			),
		}
		payloadCBOR, err := cbor.Encode(payload)
		require.NoError(t, err)
		kesSignature, err := kes.Sign(kesKey, 0, payloadCBOR)
		require.NoError(t, err)
		signable := make([]byte, 0, len(kesPub)+16)
		signable = append(signable, kesPub...)
		signable = binary.BigEndian.AppendUint64(
			signable,
			issueNumbers[i],
		)
		signable = binary.BigEndian.AppendUint64(signable, 1)
		messages[i] = pcommon.DmqMessage{
			Payload:      payload,
			KESSignature: kesSignature,
			OperationalCertificate: pcommon.OperationalCertificate{
				KESVerificationKey: kesPub,
				IssueNumber:        issueNumbers[i],
				KESPeriod:          1,
				ColdSignature: ed25519.Sign(
					coldPriv,
					signable,
				),
			},
			ColdVerificationKey: coldPub,
		}
		require.NoError(t, messages[i].SetComputedMessageID())
	}
	authenticator, err := pcommon.NewMessageAuthenticator(
		pcommon.MessageAuthenticatorConfig{
			StakeAuthority: serverTestStakeAuthority{},
			CurrentSlot: func() (uint64, error) {
				return 129600, nil
			},
			PoolOpCertIssueNumber: func(
				pcommon.PoolKeyHash,
			) (uint64, bool, error) {
				return 0, false, nil
			},
		},
	)
	require.NoError(t, err)
	return messages, authenticator
}

func serverWithPendingMessages(
	authenticator *pcommon.MessageAuthenticator,
	messages []pcommon.DmqMessage,
) *Server {
	cfg := NewConfig(WithAuthenticator(authenticator))
	server := NewServer(protocol.ProtocolOptions{}, &cfg)
	ids := make(map[string]struct{}, len(messages))
	for i := range messages {
		ids[string(messages[i].ID())] = struct{}{}
	}
	server.pendingMsgRequest = &messageRequest{ids: ids}
	return server
}

func TestServerGetAvailableMessageIDsRemainLocalDuringRequests(t *testing.T) {
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
		t.Fatalf(
			"local queue IDs became peer outstanding: %x",
			s.pendingMessageIDs,
		)
	}
	if err := s.RequestMessages([][]byte{ids1[0].MessageID}); !errors.Is(
		err,
		protocol.ErrProtocolViolationRequestExceeded,
	) {
		t.Fatalf(
			"RequestMessages error = %v, want outstanding-window rejection",
			err,
		)
	}

	s.pendingIDRequest = &messageIDRequest{blocking: true, requested: 1}
	ids2 := s.GetAvailableMessageIDs(10)
	if len(ids2) != 3 || len(s.pendingMessageIDs) != 0 {
		t.Fatalf(
			"ID request hid local IDs or changed peer outstanding IDs: ids=%d pending=%x",
			len(ids2),
			s.pendingMessageIDs,
		)
	}
	s.pendingIDRequest = nil
	s.pendingMsgRequest = &messageRequest{
		ids: map[string]struct{}{string(id1): {}},
	}
	ids3 := s.GetAvailableMessageIDs(10)
	if len(ids3) != 3 || len(s.pendingMessageIDs) != 0 {
		t.Fatalf(
			"body request hid local IDs or changed peer outstanding IDs: ids=%d pending=%x",
			len(ids3),
			s.pendingMessageIDs,
		)
	}
}

func TestServerRejectedReplyDoesNotCommitAuthenticationState(t *testing.T) {
	messages, authenticator := serverTestSignedMessages(
		t,
		[]string{"higher", "invalid", "lower"},
		[]uint64{2, 1, 1},
	)
	messages[1].OperationalCertificate.ColdSignature[0] ^= 0xff
	server := serverWithPendingMessages(authenticator, messages[:2])

	err := server.handleReplyMessages(NewMsgReplyMessages(messages[:2]))
	require.Error(t, err)
	require.NotNil(t, server.pendingMsgRequest)

	server = serverWithPendingMessages(authenticator, messages[2:])
	require.NoError(
		t,
		server.handleReplyMessages(NewMsgReplyMessages(messages[2:])),
	)
}

func TestServerAcceptedReplyCommitsAuthenticationState(t *testing.T) {
	messages, authenticator := serverTestSignedMessages(
		t,
		[]string{"lower", "higher", "stale"},
		[]uint64{1, 2, 1},
	)
	server := serverWithPendingMessages(authenticator, messages[:2])

	require.NoError(
		t,
		server.handleReplyMessages(NewMsgReplyMessages(messages[:2])),
	)
	require.Nil(t, server.pendingMsgRequest)

	server = serverWithPendingMessages(authenticator, messages[2:])
	err := server.handleReplyMessages(NewMsgReplyMessages(messages[2:]))
	require.ErrorContains(t, err, "opcert number went backwards")
}

func TestServerDropsReplyForAlreadyDeliveredMessage(t *testing.T) {
	t.Parallel()
	messages, authenticator := serverTestSignedMessages(
		t,
		[]string{"first", "second"},
		[]uint64{1, 1},
	)
	var delivered [][]byte
	cfg := NewConfig(
		WithAuthenticator(authenticator),
		WithReplyMessagesFunc(
			func(_ CallbackContext, msgs []pcommon.DmqMessage) {
				for i := range msgs {
					delivered = append(delivered, msgs[i].ID())
				}
			},
		),
	)
	server := NewServer(protocol.ProtocolOptions{}, &cfg)
	request := func(msgs ...pcommon.DmqMessage) {
		ids := make(map[string]struct{}, len(msgs))
		for i := range msgs {
			ids[string(msgs[i].ID())] = struct{}{}
		}
		server.pendingMsgRequest = &messageRequest{ids: ids}
	}

	request(messages[0])
	require.NoError(
		t,
		server.handleReplyMessages(NewMsgReplyMessages(messages[:1])),
	)
	require.Equal(t, [][]byte{messages[0].ID()}, delivered)

	// The first ID is acknowledged and announced again alongside a new one.
	request(messages...)
	require.NoError(
		t,
		server.handleReplyMessages(NewMsgReplyMessages(messages)),
	)
	require.Equal(
		t,
		[][]byte{messages[0].ID(), messages[1].ID()},
		delivered,
		"an already delivered message must not reach the callback again",
	)
	require.Nil(t, server.pendingMsgRequest)
}

func serverTestRequest(server *Server, msgs ...pcommon.DmqMessage) {
	ids := make(map[string]struct{}, len(msgs))
	for i := range msgs {
		ids[string(msgs[i].ID())] = struct{}{}
	}
	server.pendingMsgRequest = &messageRequest{ids: ids}
}

func TestServerReplayCacheCapacityIsBounded(t *testing.T) {
	t.Parallel()
	messages, authenticator := serverTestSignedMessages(
		t,
		[]string{"first", "second"},
		[]uint64{1, 1},
	)
	var delivered int
	cfg := NewConfig(
		WithAuthenticator(authenticator),
		WithMaxReplayEntries(1),
		WithReplyMessagesFunc(
			func(_ CallbackContext, msgs []pcommon.DmqMessage) {
				delivered += len(msgs)
			},
		),
	)
	server := NewServer(protocol.ProtocolOptions{}, &cfg)

	serverTestRequest(server, messages[0])
	require.NoError(
		t,
		server.handleReplyMessages(NewMsgReplyMessages(messages[:1])),
	)
	serverTestRequest(server, messages[1])
	err := server.handleReplyMessages(NewMsgReplyMessages(messages[1:]))
	require.ErrorIs(t, err, errReplayCacheCapacityExceeded)
	require.Equal(t, 1, delivered)
	require.Len(t, server.deliveredIDs, 1)
}

func TestServerReplayCacheReleasesExpiredIDs(t *testing.T) {
	t.Parallel()
	messages, authenticator := serverTestSignedMessages(
		t,
		[]string{"first", "second"},
		[]uint64{1, 1},
	)
	var delivered [][]byte
	cfg := NewConfig(
		WithAuthenticator(authenticator),
		WithMaxReplayEntries(1),
		WithReplyMessagesFunc(
			func(_ CallbackContext, msgs []pcommon.DmqMessage) {
				for i := range msgs {
					delivered = append(delivered, msgs[i].ID())
				}
			},
		),
	)
	server := NewServer(protocol.ProtocolOptions{}, &cfg)

	serverTestRequest(server, messages[0])
	require.NoError(
		t,
		server.handleReplyMessages(NewMsgReplyMessages(messages[:1])),
	)

	// Once the first message expires its slot is released, so a full cache
	// admits the next message.
	server.now = func() time.Time {
		return time.Unix(int64(messages[0].Payload.ExpiresAt)+1, 0)
	}
	serverTestRequest(server, messages[1])
	require.NoError(
		t,
		server.handleReplyMessages(NewMsgReplyMessages(messages[1:])),
	)
	require.Equal(t, [][]byte{messages[0].ID(), messages[1].ID()}, delivered)
	require.Equal(
		t,
		map[string]uint32{
			string(messages[1].ID()): messages[1].Payload.ExpiresAt,
		},
		server.deliveredIDs,
	)
}

func TestServerReplayCacheDefaultCoversHonestMempool(t *testing.T) {
	t.Parallel()
	for _, maxEntries := range []int{0, -1} {
		cfg := Config{MaxReplayEntries: maxEntries}
		server := NewServer(protocol.ProtocolOptions{}, &cfg)
		require.Equal(t, 31*4096, server.config.MaxReplayEntries)
	}
	require.Equal(t, 31*4096, NewConfig().MaxReplayEntries)
}
