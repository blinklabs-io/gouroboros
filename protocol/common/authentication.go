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
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"golang.org/x/crypto/blake2b"
)

// MessageAuthenticator handles DMQ message authentication verification per CIP-0137.
// It verifies message signatures, operational certificates, SPO pool registration,
// and KES period rotation to prevent replay attacks and unauthorized messages.
type MessageAuthenticator struct {
	logger *slog.Logger
	// protect maps with mutex
	mu sync.RWMutex

	// When true, all authentication checks are skipped. Intended for testing or
	// environments that explicitly opt out; use NewNoOpAuthenticator to create.
	disableValidation bool

	// SPO stake distribution and registration info
	spoPoolIDs map[string]bool // poolID -> active

	// KES period tracking from opcerts. No automatic eviction; callers running long-lived
	// nodes should remove inactive pool entries (see RemoveKESOpCertCacheEntry) or wrap
	// this with their own TTL/LRU policy to avoid unbounded growth.
	kesOpCertCache map[string]uint64 // poolID -> latest opcert number

	// Slots per KES period used for ledger KES verification. Default is Cardano standard.
	slotsPerKesPeriod uint64
	// AllowInsecureKES when true will accept KES signatures without running
	// a real KES verification (for tests or environments without ledger verifier).
	// This must be false in production.
	AllowInsecureKES bool
	// kesVerifier holds the optional KES verifier callback. If set, it will be used to verify KES signatures.
	// Signature: func(wrappedPayload []byte, signature []byte, vkey []byte, kesPeriod uint64, slot uint64, slotsPerKesPeriod uint64) (bool, error)
	// Uses atomic.Value for race-free concurrent access (typically set once at init, read many times).
	kesVerifier atomic.Value // stores func([]byte,[]byte,[]byte,uint64,uint64,uint64)(bool,error) or nil
}

// package-level default KES verifier used when set by application startup.
// defaultKESVerifier holds an optional package-level KES verifier used when set by application startup.
// Use atomic.Value to allow concurrent, race-free access.
var defaultKESVerifier atomic.Value // stores func([]byte,[]byte,[]byte,uint64,uint64,uint64)(bool,error)

// SetDefaultKESVerifier sets a package-level default KES verifier. Callers (e.g. main)
// can set this once at startup to avoid per-constructor imports of ledger.
func SetDefaultKESVerifier(
	v func([]byte, []byte, []byte, uint64, uint64, uint64) (bool, error),
) {
	defaultKESVerifier.Store(v)
}

// ApplyDefaultKESVerifier applies the package-level default KES verifier to the provided
// authenticator if a default has been set.
func ApplyDefaultKESVerifier(auth *MessageAuthenticator) {
	if auth == nil {
		return
	}
	// Load via atomic.Value and type-assert
	if val := defaultKESVerifier.Load(); val != nil {
		if fn, ok := val.(func([]byte, []byte, []byte, uint64, uint64, uint64) (bool, error)); ok {
			auth.SetKESVerifier(fn)
		}
	}
}

// NewMessageAuthenticator creates a new message authenticator.
func NewMessageAuthenticator(logger *slog.Logger) *MessageAuthenticator {
	if logger == nil {
		logger = slog.Default()
	}

	return &MessageAuthenticator{
		logger:         logger,
		spoPoolIDs:     make(map[string]bool),
		kesOpCertCache: make(map[string]uint64),
		// Default Cardano slots per KES period (standard mainnet value)
		slotsPerKesPeriod: 129600,
	}
}

// NewNoOpAuthenticator returns an authenticator that performs no validation.
// Suitable for testing or trusted environments where authentication is
// intentionally disabled.
func NewNoOpAuthenticator(logger *slog.Logger) *MessageAuthenticator {
	if logger == nil {
		logger = slog.Default()
	}
	return &MessageAuthenticator{
		logger:            logger,
		spoPoolIDs:        make(map[string]bool),
		kesOpCertCache:    make(map[string]uint64),
		slotsPerKesPeriod: 129600,
		disableValidation: true,
	}
}

// SetKESVerifier sets a custom KES verifier function used by VerifyMessage.
// This avoids hard dependency on the ledger package in the protocol/common package
// and lets callers inject the ledger verifier when available.
// Thread-safe; uses atomic.Value for concurrent access.
func (m *MessageAuthenticator) SetKESVerifier(
	v func([]byte, []byte, []byte, uint64, uint64, uint64) (bool, error),
) {
	m.kesVerifier.Store(v)
}

// RegisterSPOPool adds an SPO pool ID to the known active pools.
func (m *MessageAuthenticator) RegisterSPOPool(poolID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spoPoolIDs[poolID] = true
}

// UnregisterSPOPool removes an SPO pool ID from known pools.
func (m *MessageAuthenticator) UnregisterSPOPool(poolID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.spoPoolIDs, poolID)
}

// IsSPOPoolRegistered returns whether a poolID is known and registered.
// This provides a stable public API for tests and callers instead of
// accessing internal maps directly.
func (m *MessageAuthenticator) IsSPOPoolRegistered(poolID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.spoPoolIDs[poolID]
}

// VerifyMessage performs complete message authentication as per CIP-0137.
// It verifies: operational certificate, KES signature, SPO pool registration,
// message ID, and KES period rotation. Returns error if verification fails
// (which is a protocol violation and should result in peer disconnection).
// VerifyMessage verifies a message using no explicit slot (slot==0). Use VerifyMessageWithSlot
// when the caller has an explicit slot value to supply.
func (m *MessageAuthenticator) VerifyMessage(msg *DmqMessage) error {
	return m.verifyMessageInternal(msg, 0)
}

// VerifyMessageWithSlot verifies a message using an explicit slot value. When available,
// callers should supply the slot (e.g., from the block header) so KES period/current period
// checks are accurate.
func (m *MessageAuthenticator) VerifyMessageWithSlot(
	msg *DmqMessage,
	slot uint64,
) error {
	return m.verifyMessageInternal(msg, slot)
}

// verifyMessageInternal contains the core verification logic. If slot==0, the verifier
// will compute a slot based on kesPeriod * slotsPerKesPeriod as a fallback.
func (m *MessageAuthenticator) verifyMessageInternal(
	msg *DmqMessage,
	slot uint64,
) error {
	if m.disableValidation {
		return nil
	}

	if msg == nil {
		return errors.New("message is nil")
	}

	// Step 1: Verify operational certificate validity
	if err := m.verifyOperationalCertificate(&msg.OperationalCertificate, msg.ColdVerificationKey); err != nil {
		return fmt.Errorf(
			"operational certificate verification failed: %w",
			err,
		)
	}

	// Step 2: Verify KES signature over message payload
	if err := m.verifyKESSignature(msg, slot); err != nil {
		return fmt.Errorf("KES signature verification failed: %w", err)
	}

	// Step 3: Compute SPO pool ID from cold key and verify it's registered and active
	poolID := m.computePoolID(msg.ColdVerificationKey)
	m.mu.RLock()
	registered := m.spoPoolIDs[poolID]
	m.mu.RUnlock()
	if !registered {
		return fmt.Errorf("SPO pool %s is not registered or not active", poolID)
	}

	// Step 4: Verify message ID format and size constraints
	if err := m.verifyMessageID(msg); err != nil {
		return fmt.Errorf("message ID verification failed: %w", err)
	}

	// Step 5: Verify KES period rotation (opcert number doesn't go backwards)
	if err := m.verifyKESPeriodRotation(poolID, &msg.OperationalCertificate); err != nil {
		return fmt.Errorf("KES period rotation verification failed: %w", err)
	}

	return nil
}

// verifyOperationalCertificate verifies the operational certificate signature by the cold key.
func (m *MessageAuthenticator) verifyOperationalCertificate(
	opcert *OperationalCertificate,
	coldVerificationKey []byte,
) error {
	if opcert == nil {
		return errors.New("operational certificate is nil")
	}

	if len(coldVerificationKey) != 32 {
		return fmt.Errorf(
			"cold verification key must be 32 bytes, got %d",
			len(coldVerificationKey),
		)
	}

	if len(opcert.ColdSignature) != 64 {
		return fmt.Errorf(
			"cold signature must be 64 bytes, got %d",
			len(opcert.ColdSignature),
		)
	}

	// Create the message to verify: [KES vkey, issue number, KES period]
	certData := []any{
		opcert.KESVerificationKey,
		opcert.IssueNumber,
		opcert.KESPeriod,
	}

	certCbor, err := cbor.Encode(certData)
	if err != nil {
		return fmt.Errorf("failed to encode certificate data: %w", err)
	}

	// Verify signature using cold verification key
	pubKey := ed25519.PublicKey(coldVerificationKey)
	if !ed25519.Verify(pubKey, certCbor, opcert.ColdSignature) {
		return errors.New("cold signature verification failed")
	}

	return nil
}

// verifyKESSignature verifies the KES signature over the message payload (CBOR encoded).
// If slot==0, a fallback slot will be computed from the KES period and configured
// slots per KES period.
func (m *MessageAuthenticator) verifyKESSignature(
	msg *DmqMessage,
	slot uint64,
) error {
	if len(msg.KESSignature) != 448 {
		return fmt.Errorf(
			"KES signature must be 448 bytes, got %d",
			len(msg.KESSignature),
		)
	}

	// Create CBOR encoding of the message payload as required by spec
	payload := []any{
		msg.Payload.MessageID,
		msg.Payload.MessageBody,
		msg.Payload.KESPeriod,
		msg.Payload.ExpiresAt,
	}

	payloadCbor, err := cbor.Encode(payload)
	if err != nil {
		return fmt.Errorf("failed to encode payload: %w", err)
	}

	// Wrap payloadCbor into a CBOR byte string (bstr .cbor messagePayload).
	// Encoding the raw payloadCbor bytes will produce a CBOR byte string containing
	// the original CBOR-encoded payload (bstr), which matches the spec.
	wrappedCbor, err := cbor.Encode(payloadCbor)
	if err != nil {
		return fmt.Errorf("failed to encode wrapped payload as bstr: %w", err)
	}

	// KES signature verification with KES verification key
	// Note: Real KES verification would be more complex and require the KES scheme implementation
	// For now, we verify the basic structure
	if len(msg.OperationalCertificate.KESVerificationKey) != 32 {
		return errors.New("KES verification key must be 32 bytes")
	}

	// If a KES verifier has been injected, use it.
	kesVerifierVal := m.kesVerifier.Load()
	if kesVerifierVal != nil {
		kesVerifier, ok := kesVerifierVal.(func([]byte, []byte, []byte, uint64, uint64, uint64) (bool, error))
		if ok {
			kesPeriod := msg.Payload.KESPeriod
			computedSlot := slot
			if computedSlot == 0 {
				computedSlot = kesPeriod * m.slotsPerKesPeriod
			}
			// Log the slot used for verification to aid debugging
			m.logger.Debug(
				"KES verification using slot",
				"slot",
				computedSlot,
				"kes_period",
				kesPeriod,
			)
			valid, err := kesVerifier(
				wrappedCbor,
				msg.KESSignature,
				msg.OperationalCertificate.KESVerificationKey,
				kesPeriod,
				computedSlot,
				m.slotsPerKesPeriod,
			)
			if err != nil {
				return fmt.Errorf("KES verification failed: %w", err)
			}
			if !valid {
				return errors.New("KES signature verification failed")
			}
			m.logger.Debug(
				"KES signature verified",
				"payload_size",
				len(wrappedCbor),
			)
			return nil
		}
	}

	// No verifier injected: either allow insecure bypass (tests/dev) or error.
	if m.AllowInsecureKES {
		m.logger.Warn(
			"Insecure KES bypass enabled: accepting KES signature without cryptographic verification",
			"payload_size",
			len(wrappedCbor),
		)
		return nil
	}

	// Strong failure: production should set a real verifier
	m.logger.Error("No KES verifier available and insecure bypass disabled")
	return errors.New(
		"KES verification not implemented: no verifier injected and AllowInsecureKES is false",
	)
}

// verifyMessageID verifies the message ID format and size constraints.
func (m *MessageAuthenticator) verifyMessageID(msg *DmqMessage) error {
	if len(msg.Payload.MessageID) == 0 {
		return errors.New("message ID cannot be empty")
	}

	if len(msg.Payload.MessageID) > 256 {
		return fmt.Errorf(
			"message ID is too long: %d bytes",
			len(msg.Payload.MessageID),
		)
	}

	// In the actual implementation, message ID should be computed as a hash of the message
	// For now, we just verify it exists and has reasonable size
	return nil
}

// computePoolID computes the SPO pool ID from the cold verification key.
// Pool ID = blake2b-256(coldVerificationKey)
func (m *MessageAuthenticator) computePoolID(
	coldVerificationKey []byte,
) string {
	hash := blake2b.Sum256(coldVerificationKey)
	// Convert to hex string for use as pool ID
	return hex.EncodeToString(hash[:])
}

// verifyKESPeriodRotation verifies that the opcert number doesn't go backwards for each pool,
// preventing replay attacks with stale credentials.
func (m *MessageAuthenticator) verifyKESPeriodRotation(
	poolID string,
	opcert *OperationalCertificate,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	lastOpCertNumber, exists := m.kesOpCertCache[poolID]

	// If we've seen this pool before, opcert number must be >= previous
	if exists && opcert.IssueNumber < lastOpCertNumber {
		return fmt.Errorf(
			"opcert number went backwards: previous=%d, current=%d",
			lastOpCertNumber,
			opcert.IssueNumber,
		)
	}

	// Update cache with latest opcert number
	m.kesOpCertCache[poolID] = opcert.IssueNumber

	return nil
}

// RemoveKESOpCertCacheEntry removes a pool entry from the KES opcert cache.
// The cache does not evict automatically, so long-running nodes should call
// this (or wrap the authenticator with their own TTL/LRU policy) when pools
// are unregistered or otherwise inactive to avoid unbounded growth.
func (m *MessageAuthenticator) RemoveKESOpCertCacheEntry(poolID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.kesOpCertCache, poolID)
}

// TTLValidator enforces message time-to-live (TTL) constraints per CIP-0137.
// It validates that messages have not expired and rejects messages with
// expiration timestamps too far in the future.
type TTLValidator struct {
	maxAllowedTTL time.Duration
	logger        *slog.Logger
	disabled      bool
}

// NewTTLValidator creates a new TTL validator with configurable max TTL.
// If maxAllowedTTL is 0, defaults to 30 minutes per CIP-0137.
// Negative values are clamped to 0.
func NewTTLValidator(
	maxAllowedTTL time.Duration,
	logger *slog.Logger,
) *TTLValidator {
	if logger == nil {
		logger = slog.Default()
	}

	if maxAllowedTTL < 0 {
		maxAllowedTTL = 0
	}

	if maxAllowedTTL == 0 {
		// Default to 30 minutes as per CIP-0137
		maxAllowedTTL = 30 * time.Minute
	}

	return &TTLValidator{
		maxAllowedTTL: maxAllowedTTL,
		logger:        logger,
	}
}

// NewNoOpTTLValidator returns a validator that performs no TTL checks. Intended
// for trusted/testing environments where TTL enforcement is explicitly disabled.
func NewNoOpTTLValidator(logger *slog.Logger) *TTLValidator {
	if logger == nil {
		logger = slog.Default()
	}
	return &TTLValidator{disabled: true, logger: logger}
}

// ValidateMessageTTL checks if a message's expiration is valid (not expired and not too far in future).
func (v *TTLValidator) ValidateMessageTTL(msg *DmqMessage) error {
	if v.disabled {
		return nil
	}
	if msg == nil {
		return errors.New("message is nil")
	}

	// #nosec G115 -- Unix timestamp will not overflow uint32 until year 2106
	now := uint32(time.Now().Unix())
	expiresAt := msg.Payload.ExpiresAt

	// Check if message has already expired
	if now > expiresAt {
		return fmt.Errorf(
			"message has expired: now=%d, expiresAt=%d",
			now,
			expiresAt,
		)
	}

	// Check if expiration is too far in the future (protocol violation).
	// Compute in uint64 to avoid overflow when adding TTL seconds to current time.
	maxAllowedTTLSeconds := uint64(v.maxAllowedTTL.Seconds())
	nowUint64 := uint64(now)
	maxAllowedExpirationUint64 := min(
		nowUint64+maxAllowedTTLSeconds,
		math.MaxUint32,
	)
	// #nosec G115 -- overflow prevented by clamping to MaxUint32 above
	maxAllowedExpiration := uint32(maxAllowedExpirationUint64)
	if expiresAt > maxAllowedExpiration {
		return fmt.Errorf(
			"message expiration too far in future: now=%d, expiresAt=%d, max_allowed=%d",
			now,
			expiresAt,
			maxAllowedExpiration,
		)
	}

	return nil
}

// GetTimeUntilExpiration returns the time remaining before message expires, or 0 if already expired.
func (v *TTLValidator) GetTimeUntilExpiration(msg *DmqMessage) time.Duration {
	if msg == nil {
		return 0
	}

	// #nosec G115 -- Unix timestamp will not overflow uint32 until year 2106
	now := uint32(time.Now().Unix())
	if now >= msg.Payload.ExpiresAt {
		return 0
	}

	secondsLeft := msg.Payload.ExpiresAt - now
	return time.Duration(secondsLeft) * time.Second
}
