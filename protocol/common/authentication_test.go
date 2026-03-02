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
	"log/slog"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
)

// TestMessageAuthenticatorCreation tests authenticator creation
func TestMessageAuthenticatorCreation(t *testing.T) {
	auth := NewMessageAuthenticator(nil)
	assert.NotNil(t, auth)
}

// TestRegisterUnregisterSPOPool tests SPO pool registration
func TestRegisterUnregisterSPOPool(t *testing.T) {
	auth := NewMessageAuthenticator(nil)

	poolID := "test-pool-123"
	auth.RegisterSPOPool(poolID)
	assert.True(t, auth.IsSPOPoolRegistered(poolID))

	auth.UnregisterSPOPool(poolID)
	assert.False(t, auth.IsSPOPoolRegistered(poolID))
}

// TestVerifyMessageNil tests nil message verification
func TestVerifyMessageNil(t *testing.T) {
	auth := NewMessageAuthenticator(nil)
	err := auth.VerifyMessage(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message is nil")
}

// TestVerifyMessageInvalidCertificate tests verification with invalid cert
func TestVerifyMessageInvalidCertificate(t *testing.T) {
	auth := NewMessageAuthenticator(nil)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID:   []byte("test-id"),
			MessageBody: []byte("test-body"),
			KESPeriod:   100,
			ExpiresAt:   uint32(time.Now().Add(time.Hour).Unix()),
		},
		KESSignature: make([]byte, 448),
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

	err := auth.VerifyMessage(msg)
	assert.Error(t, err)
}

// TestComputePoolID tests pool ID computation from cold key
// Note: This test uses unexported computePoolID method to verify internal hash logic
func TestComputePoolID(t *testing.T) {
	auth := NewMessageAuthenticator(nil)

	coldKey1 := make([]byte, 32)
	coldKey1[0] = 0x01

	coldKey2 := make([]byte, 32)
	coldKey2[0] = 0x02

	poolID1 := auth.computePoolID(coldKey1)
	poolID2 := auth.computePoolID(coldKey2)

	assert.NotEmpty(t, poolID1)
	assert.NotEmpty(t, poolID2)
	assert.NotEqual(t, poolID1, poolID2)
}

// TestVerifyKESPeriodRotation tests KES period rotation verification
func TestVerifyKESPeriodRotation(t *testing.T) {
	auth := NewMessageAuthenticator(nil)

	poolID := "test-pool"
	opcert1 := &OperationalCertificate{
		IssueNumber: 1,
	}

	// First time should succeed
	err := auth.verifyKESPeriodRotation(poolID, opcert1)
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
}

// TestTTLValidatorCreation tests TTL validator creation
func TestTTLValidatorCreation(t *testing.T) {
	validator := NewTTLValidator(0, nil)
	assert.NotNil(t, validator)
	// Default should be 30 minutes
	assert.Equal(t, 30*time.Minute, validator.maxAllowedTTL)
}

// TestTTLValidatorCustomTTL tests TTL validator with custom TTL
func TestTTLValidatorCustomTTL(t *testing.T) {
	customTTL := 1 * time.Hour
	validator := NewTTLValidator(customTTL, nil)
	assert.Equal(t, customTTL, validator.maxAllowedTTL)
}

// TestValidateMessageTTLExpired tests expired message validation
func TestValidateMessageTTLExpired(t *testing.T) {
	validator := NewTTLValidator(30*time.Minute, nil)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID:   []byte("test-id"),
			MessageBody: []byte("test-body"),
			KESPeriod:   100,
			ExpiresAt: uint32(
				time.Now().Add(-1 * time.Minute).Unix(),
			), // Expired 1 minute ago
		},
	}

	err := validator.ValidateMessageTTL(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

// TestValidateMessageTTLValid tests valid message TTL
func TestValidateMessageTTLValid(t *testing.T) {
	validator := NewTTLValidator(30*time.Minute, nil)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID:   []byte("test-id"),
			MessageBody: []byte("test-body"),
			KESPeriod:   100,
			ExpiresAt: uint32(
				time.Now().Add(10 * time.Minute).Unix(),
			), // Expires in 10 minutes
		},
	}

	err := validator.ValidateMessageTTL(msg)
	assert.NoError(t, err)
}

// TestValidateMessageTTLTooFar tests message with expiration too far in future
func TestValidateMessageTTLTooFar(t *testing.T) {
	validator := NewTTLValidator(30*time.Minute, nil)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID:   []byte("test-id"),
			MessageBody: []byte("test-body"),
			KESPeriod:   100,
			ExpiresAt: uint32(
				time.Now().Add(2 * time.Hour).Unix(),
			), // Expires too far in future
		},
	}

	err := validator.ValidateMessageTTL(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too far in future")
}

// TestValidateMessageTTLNil tests nil message validation
func TestValidateMessageTTLNil(t *testing.T) {
	validator := NewTTLValidator(30*time.Minute, nil)
	err := validator.ValidateMessageTTL(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// TestGetTimeUntilExpiration tests time until expiration calculation
func TestGetTimeUntilExpiration(t *testing.T) {
	validator := NewTTLValidator(30*time.Minute, nil)

	// Message expiring in 5 minutes
	expiresAt := uint32(time.Now().Add(5 * time.Minute).Unix())
	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID:   []byte("test-id"),
			MessageBody: []byte("test-body"),
			KESPeriod:   100,
			ExpiresAt:   expiresAt,
		},
	}

	timeLeft := validator.GetTimeUntilExpiration(msg)
	assert.Greater(t, timeLeft, 4*time.Minute)
	assert.Less(t, timeLeft, 6*time.Minute)
}

// TestGetTimeUntilExpirationExpired tests time calculation for expired message
func TestGetTimeUntilExpirationExpired(t *testing.T) {
	validator := NewTTLValidator(30*time.Minute, nil)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID:   []byte("test-id"),
			MessageBody: []byte("test-body"),
			KESPeriod:   100,
			ExpiresAt: uint32(
				time.Now().Add(-1 * time.Minute).Unix(),
			), // Already expired
		},
	}

	timeLeft := validator.GetTimeUntilExpiration(msg)
	assert.Equal(t, time.Duration(0), timeLeft)
}

// TestGetTimeUntilExpirationNil tests time calculation for nil message
func TestGetTimeUntilExpirationNil(t *testing.T) {
	validator := NewTTLValidator(30*time.Minute, nil)
	timeLeft := validator.GetTimeUntilExpiration(nil)
	assert.Equal(t, time.Duration(0), timeLeft)
}

// TestMessageAuthenticatorWithLogger tests authenticator with custom logger
func TestMessageAuthenticatorWithLogger(t *testing.T) {
	logger := slog.Default()
	auth := NewMessageAuthenticator(logger)
	assert.NotNil(t, auth)
	assert.NotNil(t, auth.logger)
}

// TestMessageIsValid tests message validity check
func TestMessageIsValid(t *testing.T) {
	// Valid message (expires in future)
	validMsg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(time.Hour).Unix()),
		},
	}
	assert.True(t, validMsg.IsValid())

	// Expired message
	expiredMsg := &DmqMessage{
		Payload: DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(-time.Hour).Unix()),
		},
	}
	assert.False(t, expiredMsg.IsValid())
}

// TestVerifyOperationalCertificateInvalidColdKeySize tests cert verification with invalid cold key size
func TestVerifyOperationalCertificateInvalidColdKeySize(t *testing.T) {
	auth := NewMessageAuthenticator(nil)

	opcert := &OperationalCertificate{
		KESVerificationKey: make([]byte, 32),
		IssueNumber:        1,
		KESPeriod:          100,
		ColdSignature:      make([]byte, 64),
	}

	err := auth.verifyOperationalCertificate(
		opcert,
		make([]byte, 16),
	) // Wrong cold key size
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

// TestVerifyOperationalCertificateInvalidSignatureSize tests cert verification with invalid signature size
func TestVerifyOperationalCertificateInvalidSignatureSize(t *testing.T) {
	auth := NewMessageAuthenticator(nil)

	opcert := &OperationalCertificate{
		KESVerificationKey: make([]byte, 32),
		IssueNumber:        1,
		KESPeriod:          100,
		ColdSignature:      make([]byte, 32), // Wrong size
	}

	err := auth.verifyOperationalCertificate(opcert, make([]byte, 32))
	assert.Error(t, err)
}

// TestVerifyMessageIDInvalid tests message ID verification with invalid ID
func TestVerifyMessageIDInvalid(t *testing.T) {
	auth := NewMessageAuthenticator(nil)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID: []byte{}, // Empty ID
		},
	}

	err := auth.verifyMessageID(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestVerifyMessageIDTooLong tests message ID verification with too long ID
func TestVerifyMessageIDTooLong(t *testing.T) {
	auth := NewMessageAuthenticator(nil)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID: make([]byte, 300), // Too long
		},
	}

	err := auth.verifyMessageID(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too long")
}

// TestVerifyKESSignatureInvalidSize tests KES signature verification with invalid size
func TestVerifyKESSignatureInvalidSize(t *testing.T) {
	auth := NewMessageAuthenticator(nil)
	// Generate an ed25519 keypair for a valid cold key and sign the opcert
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	assert.NoError(t, err)

	opcert := OperationalCertificate{
		KESVerificationKey: make([]byte, 32),
		IssueNumber:        1,
		KESPeriod:          1,
	}

	certData := []any{
		opcert.KESVerificationKey,
		opcert.IssueNumber,
		opcert.KESPeriod,
	}
	certCbor, err := cbor.Encode(certData)
	assert.NoError(t, err)

	sig := ed25519.Sign(priv, certCbor)
	opcert.ColdSignature = sig

	msg := &DmqMessage{
		KESSignature: make(
			[]byte,
			256,
		), // Wrong size triggers KES check
		OperationalCertificate: opcert,
		ColdVerificationKey:    []byte(pub),
		Payload: DmqMessagePayload{
			MessageID:   []byte("test-id"),
			MessageBody: []byte("body"),
			KESPeriod:   1,
			ExpiresAt:   uint32(time.Now().Add(time.Minute).Unix()),
		},
	}

	err = auth.VerifyMessage(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "448 bytes")
}

// helper to build a minimal valid Message suitable for KES verification tests
func buildTestMessage(t *testing.T) *DmqMessage {
	// generate cold keypair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	// create opcert with a 32-byte KES vkey (zeros are fine for these tests)
	opcert := OperationalCertificate{
		KESVerificationKey: make([]byte, 32),
		IssueNumber:        1,
		KESPeriod:          100,
	}

	// build cert cbor and sign it with cold key
	certData := []any{
		opcert.KESVerificationKey,
		opcert.IssueNumber,
		opcert.KESPeriod,
	}
	certCbor, err := cbor.Encode(certData)
	if err != nil {
		t.Fatalf("failed to encode cert cbor: %v", err)
	}
	opcert.ColdSignature = ed25519.Sign(priv, certCbor)

	msg := &DmqMessage{
		Payload: DmqMessagePayload{
			MessageID:   []byte("test-id"),
			MessageBody: []byte("hello"),
			KESPeriod:   100,
			ExpiresAt:   uint32(time.Now().Add(time.Hour).Unix()),
		},
		KESSignature:           make([]byte, 448),
		OperationalCertificate: opcert,
		ColdVerificationKey:    pub,
	}

	return msg
}

func TestVerifyMessageWithSlot_CallsInjectedVerifier_Success(t *testing.T) {
	auth := NewMessageAuthenticator(nil)
	msg := buildTestMessage(t)

	// register poolID so SPO check passes
	poolID := auth.computePoolID(msg.ColdVerificationKey)
	auth.RegisterSPOPool(poolID)

	called := false
	var gotKesPeriod uint64
	var gotSlot uint64
	var gotSignature []byte
	var gotVkey []byte

	auth.SetKESVerifier(
		func(wrappedPayload []byte, signature []byte, vkey []byte, kesPeriod uint64, slot uint64, slotsPerKesPeriod uint64) (bool, error) {
			called = true
			gotKesPeriod = kesPeriod
			gotSlot = slot
			gotSignature = signature
			gotVkey = vkey
			return true, nil
		},
	)

	// call verification with explicit slot
	err := auth.VerifyMessageWithSlot(msg, 12345)
	assert.NoError(t, err)
	assert.True(t, called, "expected verifier to be called")
	assert.Equal(t, uint64(msg.Payload.KESPeriod), gotKesPeriod)
	assert.Equal(t, uint64(12345), gotSlot)
	assert.Equal(t, msg.KESSignature, gotSignature)
	assert.Equal(t, msg.OperationalCertificate.KESVerificationKey, gotVkey)
}

func TestVerifyMessageWithSlot_VerifierRejects(t *testing.T) {
	auth := NewMessageAuthenticator(nil)
	msg := buildTestMessage(t)
	poolID := auth.computePoolID(msg.ColdVerificationKey)
	auth.RegisterSPOPool(poolID)

	auth.SetKESVerifier(
		func(_ []byte, _ []byte, _ []byte, _ uint64, _ uint64, _ uint64) (bool, error) {
			return false, nil
		},
	)

	err := auth.VerifyMessageWithSlot(msg, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "KES signature verification failed")
}

func TestVerifyMessageWithSlot_VerifierError(t *testing.T) {
	auth := NewMessageAuthenticator(nil)
	msg := buildTestMessage(t)
	poolID := auth.computePoolID(msg.ColdVerificationKey)
	auth.RegisterSPOPool(poolID)

	auth.SetKESVerifier(
		func(_ []byte, _ []byte, _ []byte, _ uint64, _ uint64, _ uint64) (bool, error) {
			return false, errors.New("boom")
		},
	)

	err := auth.VerifyMessageWithSlot(msg, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "KES verification failed")
}

func TestDefaultKESVerifier_IsAppliedAndCalled(t *testing.T) {
	// set package-level default verifier to a mock
	called := false
	pcommonVerifier := func(wrappedPayload []byte, signature []byte, vkey []byte, kesPeriod uint64, slot uint64, slotsPerKesPeriod uint64) (bool, error) {
		called = true
		return true, nil
	}

	// Use SetDefaultKESVerifier from this package (same package)
	// Save previous default and restore after test to avoid package-level state pollution
	prev := defaultKESVerifier.Load()
	SetDefaultKESVerifier(pcommonVerifier)
	defer func() {
		// Restore previous default verifier (nil or func); defaultKESVerifier only stores this concrete type.
		if fn, ok := prev.(func([]byte, []byte, []byte, uint64, uint64, uint64) (bool, error)); ok {
			SetDefaultKESVerifier(fn)
			return
		}
		SetDefaultKESVerifier(nil)
	}()

	auth := NewMessageAuthenticator(nil)
	// apply default verifier to auth
	ApplyDefaultKESVerifier(auth)

	// build message and register pool
	msg := buildTestMessage(t)
	poolID := auth.computePoolID(msg.ColdVerificationKey)
	auth.RegisterSPOPool(poolID)

	// call verification which should trigger default verifier
	err := auth.VerifyMessageWithSlot(msg, 0)
	assert.NoError(t, err)
	assert.True(t, called, "expected default verifier to be called")
}
