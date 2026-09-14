// Copyright 2025 Blink Labs Software
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

package common

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStakeAuthority is a test StakeAuthority backed by a plain map.
type stubStakeAuthority struct {
	stake map[PoolKeyHash]uint64
	err   error
}

func newStubStakeAuthority() *stubStakeAuthority {
	return &stubStakeAuthority{stake: make(map[PoolKeyHash]uint64)}
}

func (s *stubStakeAuthority) PoolActiveStake(
	poolKeyHash PoolKeyHash,
) (uint64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.stake[poolKeyHash], nil
}

func (s *stubStakeAuthority) register(poolKeyHash PoolKeyHash, stake uint64) {
	s.stake[poolKeyHash] = stake
}

// TestMessageAuthenticatorCreation tests authenticator creation
func TestMessageAuthenticatorCreation(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

// TestMessageAuthenticatorRequiresStakeAuthority tests that construction
// fails without a StakeAuthority.
func TestMessageAuthenticatorRequiresStakeAuthority(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{})
	assert.Nil(t, auth)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthenticatorMisconfigured)
}

// TestVerifyMessageNil tests nil message verification
func TestVerifyMessageNil(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)
	err = auth.VerifyMessage(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message is nil")
}

// TestVerifyMessageInvalidCertificate tests verification with invalid cert
func TestVerifyMessageInvalidCertificate(t *testing.T) {
	stake := newStubStakeAuthority()
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
	})
	require.NoError(t, err)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageBody: []byte("test-body"),
			KESPeriod:   100,
			ExpiresAt:   uint32(time.Now().Add(time.Hour).Unix()),
		},
		KESSignature: make([]byte, kes.CardanoKesSignatureSize),
		OperationalCertificate: OperationalCertificate{
			KESVerificationKey: make([]byte, 32), // All zeros - invalid key
			IssueNumber:        1,
			KESPeriod:          100,
			ColdSignature: make(
				[]byte,
				64,
			), // All zeros - invalid signature
		},
		ColdVerificationKey: make([]byte, 32),
	}
	assert.NoError(t, msg.SetComputedMessageID())
	poolID, perr := poolKeyHash(msg.ColdVerificationKey)
	require.NoError(t, perr)
	stake.register(poolID, 1000)

	err = auth.VerifyMessage(msg)
	assert.Error(t, err)
}

// TestVerifyMessageUnauthorizedPool tests that a message from a pool absent
// from the stake distribution is rejected before any signature is checked.
func TestVerifyMessageUnauthorizedPool(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(), // no pool registered
	})
	require.NoError(t, err)

	msg := buildSignedTestMessage(t, 100, 100)

	err = auth.VerifyMessage(msg)
	assert.ErrorIs(t, err, ErrPoolNotInStakeDistribution)
}

// TestVerifyMessageStakeAuthorityError tests that a StakeAuthority error
// propagates rather than being treated as unauthorized.
func TestVerifyMessageStakeAuthorityError(t *testing.T) {
	stake := newStubStakeAuthority()
	stake.err = errors.New("boom")
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
	})
	require.NoError(t, err)

	msg := buildSignedTestMessage(t, 100, 100)

	err = auth.VerifyMessage(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// TestComputePoolKeyHash tests pool ID computation from cold key
func TestComputePoolKeyHash(t *testing.T) {
	coldKey1 := make([]byte, 32)
	coldKey1[0] = 0x01

	coldKey2 := make([]byte, 32)
	coldKey2[0] = 0x02

	poolID1, err := poolKeyHash(coldKey1)
	require.NoError(t, err)
	poolID2, err := poolKeyHash(coldKey2)
	require.NoError(t, err)

	assert.NotEqual(t, poolID1, poolID2)
	assert.Len(t, poolID1, PoolKeyHashSize)
}

// TestVerifyKESPeriodRotation tests KES period rotation verification
func TestVerifyKESPeriodRotation(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)

	var poolID PoolKeyHash
	poolID[0] = 0x42
	opcert1 := &OperationalCertificate{
		IssueNumber: 1,
	}

	// First time should succeed
	err = auth.verifyKESPeriodRotation(poolID, opcert1)
	assert.NoError(t, err)

	// Same number should succeed
	err = auth.verifyKESPeriodRotation(poolID, opcert1)
	assert.NoError(t, err)

	// Higher number should succeed
	opcert2 := &OperationalCertificate{
		IssueNumber: 2,
	}
	err = auth.verifyKESPeriodRotation(poolID, opcert2)
	assert.NoError(t, err)

	// Lower number should fail
	opcert3 := &OperationalCertificate{
		IssueNumber: 1,
	}
	err = auth.verifyKESPeriodRotation(poolID, opcert3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "went backwards")

	// RemoveKESOpCertCacheEntry drops the baseline, so a previously-lower
	// number is accepted again as a first sighting.
	auth.RemoveKESOpCertCacheEntry(poolID)
	err = auth.verifyKESPeriodRotation(poolID, opcert3)
	assert.NoError(t, err)
}

// TestTTLValidatorCreation tests TTL validator creation
func TestTTLValidatorCreation(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	assert.NotNil(t, validator)
}

// TestTTLValidatorCustomTTL tests TTL validator with custom TTL
func TestTTLValidatorCustomTTL(t *testing.T) {
	validator := NewTTLValidator(time.Hour, nil)
	assert.NotNil(t, validator)
}

// TestValidateMessageTTLExpired tests TTL validation with expired message
func TestValidateMessageTTLExpired(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(-time.Hour).Unix()),
		},
	}
	err := validator.ValidateMessageTTL(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

// TestValidateMessageTTLValid tests TTL validation with valid message
func TestValidateMessageTTLValid(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(time.Minute).Unix()),
		},
	}
	err := validator.ValidateMessageTTL(msg)
	assert.NoError(t, err)
}

// TestValidateMessageTTLTooFar tests TTL validation with expiration too far in future
func TestValidateMessageTTLTooFar(t *testing.T) {
	validator := NewTTLValidator(time.Minute, nil)
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(time.Hour).Unix()),
		},
	}
	err := validator.ValidateMessageTTL(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too far in future")
}

// TestValidateMessageTTLNil tests TTL validation with nil message
func TestValidateMessageTTLNil(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	err := validator.ValidateMessageTTL(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// TestValidateMessageTTLAtValid tests deterministic TTL validation
func TestValidateMessageTTLAtValid(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	now := time.Unix(1_000_000, 0)
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(now.Add(time.Minute).Unix()),
		},
	}
	assert.NoError(t, validator.ValidateMessageTTLAt(msg, now))
}

// TestValidateMessageTTLAtExpired tests deterministic TTL validation, expired
func TestValidateMessageTTLAtExpired(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	now := time.Unix(1_000_000, 0)
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(now.Add(-time.Minute).Unix()),
		},
	}
	assert.Error(t, validator.ValidateMessageTTLAt(msg, now))
}

// TestValidateMessageTTLAtTooFarFuture tests deterministic TTL validation, too far future
func TestValidateMessageTTLAtTooFarFuture(t *testing.T) {
	validator := NewTTLValidator(time.Minute, nil)
	now := time.Unix(1_000_000, 0)
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(now.Add(time.Hour).Unix()),
		},
	}
	assert.Error(t, validator.ValidateMessageTTLAt(msg, now))
}

// TestValidateMessageTTLAtDisabled tests a no-op TTL validator
func TestValidateMessageTTLAtDisabled(t *testing.T) {
	validator := NewNoOpTTLValidator(nil)
	now := time.Unix(1_000_000, 0)
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(now.Add(-time.Hour).Unix()),
		},
	}
	assert.NoError(t, validator.ValidateMessageTTLAt(msg, now))
}

// TestValidateMessageTTLAtNil tests deterministic TTL validation, nil message
func TestValidateMessageTTLAtNil(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	assert.Error(t, validator.ValidateMessageTTLAt(nil, time.Now()))
}

// TestValidateMessageTTLAtNowBeyondUint32 tests now() past the uint32 domain
func TestValidateMessageTTLAtNowBeyondUint32(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	now := time.Unix(int64(math.MaxUint32)+1000, 0)
	msg := &DmqMessage{
		Payload: DmqMessagePayload{ExpiresAt: math.MaxUint32},
	}
	assert.Error(t, validator.ValidateMessageTTLAt(msg, now))
}

// TestValidateMessageTTLAtBoundaryEqualNow tests expiresAt == now is valid
func TestValidateMessageTTLAtBoundaryEqualNow(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	now := time.Unix(1_000_000, 0)
	msg := &DmqMessage{
		Payload: DmqMessagePayload{ExpiresAt: uint32(now.Unix())},
	}
	assert.NoError(t, validator.ValidateMessageTTLAt(msg, now))
}

// TestValidateMessageTTLDelegatesToAt tests ValidateMessageTTL delegates to ValidateMessageTTLAt
func TestValidateMessageTTLDelegatesToAt(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(time.Minute).Unix()),
		},
	}
	assert.NoError(t, validator.ValidateMessageTTL(msg))
}

// TestGetTimeUntilExpiration tests time-until-expiration computation
func TestGetTimeUntilExpiration(t *testing.T) {
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(time.Minute).Unix()),
		},
	}
	remaining := (&TTLValidator{}).GetTimeUntilExpiration(msg)
	assert.Greater(t, remaining, time.Duration(0))
}

// TestGetTimeUntilExpirationExpired tests time-until-expiration for an expired message
func TestGetTimeUntilExpirationExpired(t *testing.T) {
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(-time.Hour).Unix()),
		},
	}
	remaining := (&TTLValidator{}).GetTimeUntilExpiration(msg)
	assert.Equal(t, time.Duration(0), remaining)
}

// TestGetTimeUntilExpirationNil tests time-until-expiration for a nil message
func TestGetTimeUntilExpirationNil(t *testing.T) {
	remaining := (&TTLValidator{}).GetTimeUntilExpiration(nil)
	assert.Equal(t, time.Duration(0), remaining)
}

// TestMessageAuthenticatorWithLogger tests authenticator creation with a logger
func TestMessageAuthenticatorWithLogger(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

// TestMessageIsValid tests the IsValid convenience method
func TestMessageIsValid(t *testing.T) {
	expiredMsg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(-time.Hour).Unix()),
		},
	}
	assert.False(t, expiredMsg.IsValid())
}

// TestVerifyOperationalCertificateInvalidColdKeySize tests cert verification with invalid cold key size
func TestVerifyOperationalCertificateInvalidColdKeySize(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)

	opcert := &OperationalCertificate{
		KESVerificationKey: make([]byte, 32),
		IssueNumber:        1,
		KESPeriod:          100,
		ColdSignature:      make([]byte, 64),
	}

	err = auth.verifyOperationalCertificate(
		opcert,
		make([]byte, 16),
	) // Wrong cold key size
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

// TestVerifyOperationalCertificateInvalidSignatureSize tests cert verification with invalid signature size
func TestVerifyOperationalCertificateInvalidSignatureSize(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)

	opcert := &OperationalCertificate{
		KESVerificationKey: make([]byte, 32),
		IssueNumber:        1,
		KESPeriod:          100,
		ColdSignature:      make([]byte, 32), // Wrong size
	}

	err = auth.verifyOperationalCertificate(opcert, make([]byte, 32))
	assert.Error(t, err)
}

// TestVerifyOperationalCertificate_RawOCertSignableRepresentation verifies a
// real, already-issued Cardano operational certificate — signed over the raw
// OCertSignable representation (KES vkey || issue number BE || KES period
// BE), matching cardano-node/cardano-ledger, not a CBOR encoding.
func TestVerifyOperationalCertificate_RawOCertSignableRepresentation(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	opcert := &OperationalCertificate{
		KESVerificationKey: make([]byte, 32),
		IssueNumber:        7,
		KESPeriod:          42,
	}
	opcert.ColdSignature = ed25519.Sign(priv, opCertSignableBytes(
		opcert.KESVerificationKey,
		opcert.IssueNumber,
		opcert.KESPeriod,
	))

	err = auth.verifyOperationalCertificate(opcert, pub)
	assert.NoError(t, err)
}

// TestVerifyOperationalCertificate_RejectsCBORArraySignature proves a
// certificate signed the old, incorrect way — a CBOR array of
// [kesVkey, issueNumber, kesPeriod] — does not verify. A real pool's
// operational certificate is never signed this way.
func TestVerifyOperationalCertificate_RejectsCBORArraySignature(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	opcert := &OperationalCertificate{
		KESVerificationKey: make([]byte, 32),
		IssueNumber:        7,
		KESPeriod:          42,
	}
	certData := []any{
		opcert.KESVerificationKey,
		opcert.IssueNumber,
		opcert.KESPeriod,
	}
	certCbor, err := cbor.Encode(certData)
	require.NoError(t, err)
	opcert.ColdSignature = ed25519.Sign(priv, certCbor)

	err = auth.verifyOperationalCertificate(opcert, pub)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cold signature verification failed")
}

// TestVerifyMessageIDInvalid tests message ID verification with invalid ID
func TestVerifyMessageIDInvalid(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID: []byte{}, // Empty ID
		},
	}

	err = auth.verifyMessageID(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestVerifyMessageIDTooLong tests message ID verification with too long ID
func TestVerifyMessageIDTooLong(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID: make([]byte, 300), // Too long
		},
	}

	err = auth.verifyMessageID(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 32 bytes")
}

func TestComputeDmqMessageIDGoldenVector(t *testing.T) {
	messageID, err := ComputeDmqMessageID(DmqMessagePayload{
		MessageBody: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		KESPeriod:   123,
		ExpiresAt:   123456,
	})
	assert.NoError(t, err)
	assert.Equal(
		t,
		[]byte{
			202, 230, 133, 93, 29, 204, 161, 252,
			87, 183, 156, 101, 193, 251, 172, 245,
			171, 98, 179, 213, 232, 216, 239, 9,
			94, 155, 194, 226, 246, 17, 50, 185,
		},
		messageID,
	)
}

func TestVerifyMessageIDMismatch(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)

	msg := &DmqMessage{
		MessageID: []byte{
			202, 230, 133, 93, 29, 204, 161, 252,
			87, 183, 156, 101, 193, 251, 172, 245,
			171, 98, 179, 213, 232, 216, 239, 9,
			94, 155, 194, 226, 246, 17, 50, 184,
		},
		Payload: DmqMessagePayload{
			MessageBody: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			KESPeriod:   123,
			ExpiresAt:   123456,
		},
	}

	err = auth.verifyMessageID(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message ID mismatch")
}

func TestVerifyMessageIDValid(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageBody: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			KESPeriod:   123,
			ExpiresAt:   123456,
		},
	}
	assert.NoError(t, msg.SetComputedMessageID())

	err = auth.verifyMessageID(msg)
	assert.NoError(t, err)
}

func TestDmqMessageCBORUsesCurrentCIPShape(t *testing.T) {
	msg := DmqMessage{
		Payload: DmqMessagePayload{
			MessageID:   []byte("legacy-id"),
			MessageBody: []byte("body"),
			KESPeriod:   1,
			ExpiresAt:   2,
		},
		KESSignature: make([]byte, 448),
		OperationalCertificate: OperationalCertificate{
			KESVerificationKey: make([]byte, 32),
			IssueNumber:        1,
			KESPeriod:          1,
			ColdSignature:      make([]byte, 64),
		},
		ColdVerificationKey: make([]byte, 32),
	}

	data, err := cbor.Encode(msg)
	assert.NoError(t, err)

	var outer []cbor.RawMessage
	_, err = cbor.Decode(data, &outer)
	require.NoError(t, err)
	require.Len(t, outer, 5)

	var payload []cbor.RawMessage
	_, err = cbor.Decode(outer[1], &payload)
	require.NoError(t, err)
	require.Len(t, payload, 3)

	var decoded DmqMessage
	_, err = cbor.Decode(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, []byte("legacy-id"), decoded.MessageID)
	assert.Equal(t, []byte("legacy-id"), decoded.Payload.MessageID)
}

func TestDmqMessageCBORDecodesLegacyShape(t *testing.T) {
	msg := DmqMessage{
		Payload: DmqMessagePayload{
			MessageID:   []byte("legacy-id"),
			MessageBody: []byte("body"),
			KESPeriod:   1,
			ExpiresAt:   2,
		},
		KESSignature: make([]byte, 448),
		OperationalCertificate: OperationalCertificate{
			KESVerificationKey: make([]byte, 32),
			IssueNumber:        1,
			KESPeriod:          1,
			ColdSignature:      make([]byte, 64),
		},
		ColdVerificationKey: make([]byte, 32),
	}

	data, err := MarshalDmqMessageLegacyCBOR(msg)
	assert.NoError(t, err)

	var outer []cbor.RawMessage
	_, err = cbor.Decode(data, &outer)
	assert.NoError(t, err)
	assert.Len(t, outer, 4)

	var decoded DmqMessage
	_, err = cbor.Decode(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, []byte("legacy-id"), decoded.MessageID)
	assert.Equal(t, []byte("legacy-id"), decoded.Payload.MessageID)
	assert.Equal(t, []byte("body"), decoded.Payload.MessageBody)
}

// TestVerifyKESSignatureInvalidSize tests KES signature verification with invalid size
func TestVerifyKESSignatureInvalidSize(t *testing.T) {
	stake := newStubStakeAuthority()
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
	})
	require.NoError(t, err)
	// Generate an ed25519 keypair for a valid cold key and sign the opcert
	pub, priv, kerr := ed25519.GenerateKey(rand.Reader)
	assert.NoError(t, kerr)

	opcert := OperationalCertificate{
		KESVerificationKey: make([]byte, 32),
		IssueNumber:        1,
		KESPeriod:          1,
	}
	opcert.ColdSignature = ed25519.Sign(priv, opCertSignableBytes(
		opcert.KESVerificationKey,
		opcert.IssueNumber,
		opcert.KESPeriod,
	))

	msg := &DmqMessage{
		KESSignature: make(
			[]byte,
			256,
		), // Wrong size triggers KES check
		OperationalCertificate: opcert,
		ColdVerificationKey:    []byte(pub),
		Payload: DmqMessagePayload{
			MessageBody: []byte("body"),
			KESPeriod:   1,
			ExpiresAt:   uint32(time.Now().Add(time.Minute).Unix()),
		},
	}
	assert.NoError(t, msg.SetComputedMessageID())
	poolID, perr := poolKeyHash(msg.ColdVerificationKey)
	require.NoError(t, perr)
	stake.register(poolID, 1000)

	err = auth.VerifyMessage(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "448 bytes")
}

// buildSignedTestMessage builds a DmqMessage with a real cold-key opcert
// signature and a real KES signature over the payload, evolved to
// (msgPeriod - certPeriod). It does not register the pool with any
// StakeAuthority — callers needing an authorized message must do so.
func buildSignedTestMessage(
	t *testing.T,
	certPeriod uint64,
	msgPeriod uint64,
) *DmqMessage {
	t.Helper()

	// Cold keypair signs the operational certificate.
	coldPub, coldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// KES keypair signs the message payload, evolved to the signing period.
	seed := make([]byte, kes.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	sk, kesPub, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)

	require.GreaterOrEqual(t, msgPeriod, certPeriod)
	evolution := msgPeriod - certPeriod
	for range evolution {
		sk, err = kes.Update(sk)
		require.NoError(t, err)
	}

	opcert := OperationalCertificate{
		KESVerificationKey: kesPub,
		IssueNumber:        1,
		KESPeriod:          certPeriod,
	}
	opcert.ColdSignature = ed25519.Sign(coldPriv, opCertSignableBytes(
		opcert.KESVerificationKey,
		opcert.IssueNumber,
		opcert.KESPeriod,
	))

	payload := DmqMessagePayload{
		MessageBody: []byte("hello"),
		KESPeriod:   msgPeriod,
		ExpiresAt:   uint32(time.Now().Add(time.Hour).Unix()),
	}
	payloadCbor, err := cbor.Encode(payload)
	require.NoError(t, err)
	wrappedCbor, err := cbor.Encode(payloadCbor)
	require.NoError(t, err)

	sig, err := kes.Sign(sk, evolution, wrappedCbor)
	require.NoError(t, err)

	msg := &DmqMessage{
		Payload:                payload,
		KESSignature:           sig,
		OperationalCertificate: opcert,
		ColdVerificationKey:    coldPub,
	}
	require.NoError(t, msg.SetComputedMessageID())
	return msg
}

// TestVerifyMessage_EndToEndValid proves a message signed with a real cold
// key and a real, correctly-evolved KES key verifies in full, in-process --
// no injected verifier callback.
func TestVerifyMessage_EndToEndValid(t *testing.T) {
	stake := newStubStakeAuthority()
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
	})
	require.NoError(t, err)

	msg := buildSignedTestMessage(t, 100, 100)
	poolID, perr := poolKeyHash(msg.ColdVerificationKey)
	require.NoError(t, perr)
	stake.register(poolID, 1000)

	assert.NoError(t, auth.VerifyMessage(msg))
}

// TestVerifyMessage_EndToEndEvolvedKES proves a message signed at a KES
// evolution after the certificate's own issuance period verifies too.
func TestVerifyMessage_EndToEndEvolvedKES(t *testing.T) {
	stake := newStubStakeAuthority()
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
	})
	require.NoError(t, err)

	msg := buildSignedTestMessage(t, 50, 53)
	poolID, perr := poolKeyHash(msg.ColdVerificationKey)
	require.NoError(t, perr)
	stake.register(poolID, 1000)

	assert.NoError(t, auth.VerifyMessage(msg))
}

// TestVerifyKESSignature_RejectsPayloadPeriodBeforeCertificateIssuance proves
// a message claiming to have been signed at a KES period earlier than its
// own certificate's issuance period is rejected outright.
func TestVerifyKESSignature_RejectsPayloadPeriodBeforeCertificateIssuance(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)

	msg := &DmqMessage{
		OperationalCertificate: OperationalCertificate{
			KESVerificationKey: make([]byte, 32),
			KESPeriod:          80,
		},
		Payload: DmqMessagePayload{
			KESPeriod: 50,
		},
		KESSignature: make([]byte, kes.CardanoKesSignatureSize),
	}

	err = auth.verifyKESSignature(msg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "precedes certificate issuance period")
}

// TestVerifyKESSignature_OverflowGuard proves a message whose claimed KES
// period would overflow the period-to-slot conversion is rejected rather
// than silently wrapping to an unrelated, easy evolution.
func TestVerifyKESSignature_OverflowGuard(t *testing.T) {
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: newStubStakeAuthority(),
	})
	require.NoError(t, err)

	msg := &DmqMessage{
		OperationalCertificate: OperationalCertificate{
			KESVerificationKey: make([]byte, 32),
			KESPeriod:          0,
		},
		Payload: DmqMessagePayload{
			KESPeriod: math.MaxUint64,
		},
		KESSignature: make([]byte, kes.CardanoKesSignatureSize),
	}

	err = auth.verifyKESSignature(msg, nil)
	assert.ErrorIs(t, err, ErrKESPeriodOverflow)
}

// TestVerifyMessageWithSlot_ExplicitSlotOverridesDerivedOne proves an
// explicit slot is honored over the slot implied by the message's own
// claimed period.
func TestVerifyMessageWithSlot_ExplicitSlotOverridesDerivedOne(t *testing.T) {
	stake := newStubStakeAuthority()
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
	})
	require.NoError(t, err)

	msg := buildSignedTestMessage(t, 100, 100)
	poolID, perr := poolKeyHash(msg.ColdVerificationKey)
	require.NoError(t, perr)
	stake.register(poolID, 1000)

	// The real slot for period 100 at the default 129600 slots/period.
	realSlot := uint64(100) * 129600
	assert.NoError(t, auth.VerifyMessageWithSlot(msg, realSlot))

	// A slot corresponding to an earlier period than the cert's own
	// issuance period must fail (KES signature won't verify at that
	// evolution).
	assert.Error(t, auth.VerifyMessageWithSlot(msg, 0))
}
