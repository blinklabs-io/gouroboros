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
