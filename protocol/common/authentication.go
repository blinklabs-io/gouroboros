// Copyright 2026 Blink Labs Software
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
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/ed25519strict"
	"github.com/blinklabs-io/gouroboros/kes"
	"golang.org/x/crypto/blake2b"
)

// PoolKeyHashSize is the byte width of a Cardano pool ID: Blake2b-224 of the
// pool's cold verification key.
const PoolKeyHashSize = 28

// DefaultMaxKESEvolutions is the Cardano mainnet maxKESEvolutions genesis value.
const DefaultMaxKESEvolutions uint64 = 62

// PoolKeyHash identifies a Cardano stake pool: Blake2b-224 of its cold
// verification key. This is a true alias for [PoolKeyHashSize]byte (not a
// defined type), so it accepts ledger/common.PoolKeyHash values (and any
// other [28]byte-shaped pool ID) directly, with no conversion required —
// this package cannot import ledger/common without an import cycle
// (ledger -> ledger/common -> protocol/common).
type PoolKeyHash = [PoolKeyHashSize]byte

// StakeAuthority reports a pool's active stake for CIP-0137 message
// authorization. A message's issuing pool must hold stake in the current
// distribution for MessageAuthenticator to accept it.
//
// A pool absent from the current stake distribution snapshot should return
// (0, nil), not an error. Implementations typically adapt a node's live
// ledger state (e.g. ledger.LedgerView.GetPoolStake against the Praos-active
// epoch) to this interface.
type StakeAuthority interface {
	PoolActiveStake(poolKeyHash PoolKeyHash) (uint64, error)
}

// Sentinel errors returned by MessageAuthenticator. Wrap with errors.Is
// rather than matching message text.
var (
	// ErrAuthenticatorMisconfigured is returned by NewMessageAuthenticator
	// when a required config field is missing.
	ErrAuthenticatorMisconfigured = errors.New(
		"protocol/common: message authenticator missing required configuration",
	)
	// ErrPoolNotInStakeDistribution is returned when the issuing pool holds
	// no stake in the current distribution.
	ErrPoolNotInStakeDistribution = errors.New(
		"protocol/common: issuing pool holds no stake in the current distribution",
	)
	// ErrKESPeriodOverflow is returned when a message's claimed KES period
	// is so large that converting it to a slot (period * slotsPerKesPeriod)
	// would overflow uint64. Rejecting it outright, rather than letting the
	// multiplication wrap, matters because the wrapped slot can land on a
	// small, easy-to-produce evolution that has nothing to do with the
	// claimed period -- silently verifying it there would let a large
	// claimed period smuggle through a signature made at a completely
	// different, attacker-chosen evolution.
	ErrKESPeriodOverflow = errors.New(
		"protocol/common: message KES period would overflow when converted to a slot",
	)
)

// MessageAuthenticator handles DMQ message authentication verification per
// CIP-0137. It verifies message-ID integrity, pool-ID derivation and
// stake-distribution authorization, the operational certificate's cold-key
// signature, the KES signature over the message payload, and
// operational-certificate issue-number monotonicity (replay protection).
type MessageAuthenticator struct {
	logger *slog.Logger

	// When true, all authentication checks are skipped. Intended for testing or
	// environments that explicitly opt out; use NewNoOpAuthenticator to create.
	disableValidation bool

	// stakeAuthority backs pool authorization. Immutable after construction,
	// so it needs no lock.
	stakeAuthority StakeAuthority

	// KES period tracking from opcerts, keyed by pool ID. Only
	// verifyKESPeriodRotation writes here, and it runs after stake
	// authorization and both signature checks, so a pool ID is recorded
	// only once its holder has proved active stake and possession of the
	// matching cold and KES keys. The key space is the stake distribution
	// -- a few thousand 36-byte entries on mainnet -- not anything a remote
	// peer can choose, so the map needs no size bound.
	//
	// There is deliberately no automatic eviction. Dropping an entry
	// restores the pool to "never seen" and lets a stale operational
	// certificate be replayed, so a TTL or LRU policy would trade replay
	// protection away for memory that is already bounded.
	mu             sync.Mutex
	kesOpCertCache map[PoolKeyHash]uint64

	// Slots per KES period used for KES verification. Default is Cardano standard.
	slotsPerKesPeriod uint64
	// Maximum evolution distance allowed from an operational certificate.
	maxKESEvolutions uint64
}

// MessageAuthenticatorConfig configures a MessageAuthenticator.
type MessageAuthenticatorConfig struct {
	// StakeAuthority backs the pool-authorization check. Required; a nil
	// StakeAuthority is a configuration error rather than a silent skip of
	// the check.
	StakeAuthority StakeAuthority
	// SlotsPerKESPeriod is the Shelley genesis slotsPerKESPeriod parameter.
	// Defaults to 129600 (Cardano mainnet) when zero.
	SlotsPerKESPeriod uint64
	// MaxKESEvolutions is the Shelley genesis maxKESEvolutions parameter.
	// Defaults to 62 (Cardano mainnet) when zero.
	MaxKESEvolutions uint64
	// Logger, when nil, defaults to slog.Default().
	Logger *slog.Logger
}

// NewMessageAuthenticator constructs a MessageAuthenticator. It returns
// ErrAuthenticatorMisconfigured if cfg.StakeAuthority is nil.
func NewMessageAuthenticator(
	cfg MessageAuthenticatorConfig,
) (*MessageAuthenticator, error) {
	if cfg.StakeAuthority == nil {
		return nil, fmt.Errorf(
			"%w: StakeAuthority is required",
			ErrAuthenticatorMisconfigured,
		)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	slotsPerKesPeriod := cfg.SlotsPerKESPeriod
	if slotsPerKesPeriod == 0 {
		// Default Cardano slots per KES period (standard mainnet value)
		slotsPerKesPeriod = 129600
	}
	maxKESEvolutions := cfg.MaxKESEvolutions
	if maxKESEvolutions == 0 {
		maxKESEvolutions = DefaultMaxKESEvolutions
	}
	return &MessageAuthenticator{
		logger:            logger,
		stakeAuthority:    cfg.StakeAuthority,
		kesOpCertCache:    make(map[PoolKeyHash]uint64),
		slotsPerKesPeriod: slotsPerKesPeriod,
		maxKESEvolutions:  maxKESEvolutions,
	}, nil
}

// NewNoOpAuthenticator returns an authenticator that performs no validation.
// Suitable for testing or trusted environments where authentication is
// intentionally disabled. Unlike NewMessageAuthenticator, it needs no
// StakeAuthority since no check ever runs.
func NewNoOpAuthenticator(logger *slog.Logger) *MessageAuthenticator {
	if logger == nil {
		logger = slog.Default()
	}
	return &MessageAuthenticator{
		logger:            logger,
		kesOpCertCache:    make(map[PoolKeyHash]uint64),
		slotsPerKesPeriod: 129600,
		maxKESEvolutions:  DefaultMaxKESEvolutions,
		disableValidation: true,
	}
}

// VerifyMessage performs complete message authentication as per CIP-0137.
// It verifies: message ID, pool-ID derivation and stake-distribution
// authorization, operational certificate, KES signature, and KES period
// rotation. Returns error if verification fails (which is a protocol
// violation and should result in peer disconnection).
// VerifyMessage verifies a message using no explicit slot. Use VerifyMessageWithSlot
// when the caller has an explicit slot value to supply.
func (m *MessageAuthenticator) VerifyMessage(msg *DmqMessage) error {
	return m.verifyMessageInternal(msg, nil)
}

// VerifyMessageWithSlot verifies a message using an explicit slot value. When available,
// callers should supply the slot (e.g., from the block header) so KES period/current period
// checks are accurate.
func (m *MessageAuthenticator) VerifyMessageWithSlot(
	msg *DmqMessage,
	slot uint64,
) error {
	return m.verifyMessageInternal(msg, &slot)
}

// verifyMessageInternal contains the core verification logic. A nil slot means
// that the verifier should derive the slot from the message's claimed KES period.
//
// Stake authorization runs before either signature check. Deriving a pool ID
// from ColdVerificationKey needs no signature -- anyone can self-sign an
// internally consistent opcert/KES chain over freshly generated keys, so the
// signature checks only prove the sender holds the claimed private keys, not
// that those keys belong to a real, staked pool. Checking authorization
// first turns away a message from an unregistered identity before paying
// for an ed25519 verify and the KES verify, rather than after.
func (m *MessageAuthenticator) verifyMessageInternal(
	msg *DmqMessage,
	slot *uint64,
) error {
	if m.disableValidation {
		return nil
	}

	if msg == nil {
		return errors.New("message is nil")
	}

	// Step 1: Verify deterministic message ID before heavier auth checks.
	if err := m.verifyMessageID(msg); err != nil {
		return fmt.Errorf("message ID verification failed: %w", err)
	}

	// Step 2: Stake authorization before either signature check -- see the
	// doc comment above for why.
	poolID, err := poolKeyHash(msg.ColdVerificationKey)
	if err != nil {
		return fmt.Errorf("compute pool id: %w", err)
	}
	stake, err := m.stakeAuthority.PoolActiveStake(poolID)
	if err != nil {
		return fmt.Errorf("look up pool stake: %w", err)
	}
	if stake == 0 {
		return ErrPoolNotInStakeDistribution
	}

	// Step 3: Verify operational certificate validity
	if err := m.verifyOperationalCertificate(&msg.OperationalCertificate, msg.ColdVerificationKey); err != nil {
		return fmt.Errorf(
			"operational certificate verification failed: %w",
			err,
		)
	}

	// Step 4: Verify KES signature over message payload
	if err := m.verifyKESSignature(msg, slot); err != nil {
		return fmt.Errorf("KES signature verification failed: %w", err)
	}

	// Step 5: Verify KES period rotation (opcert number doesn't go backwards).
	// Not updated until every earlier check has passed, so a message that
	// fails an earlier check cannot poison replay protection for a later,
	// legitimately higher-numbered certificate from the same pool.
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

	// The cold key signs the raw OCertSignable representation used by
	// cardano-node/cardano-ledger — KES vkey || issue number (8-byte BE) ||
	// KES period (8-byte BE) — NOT a CBOR encoding. A DMQ message's
	// operational certificate is the pool's existing, already-issued
	// certificate (the same one used for block production), so verifying it
	// against any other byte representation rejects every real pool's
	// message. This mirrors ledger.VerifyOpCertSignature /
	// ledger/common.OpCertSignableBytes; it is reimplemented locally
	// (opCertSignableBytes below) rather than imported, to avoid a package
	// import cycle (ledger -> ledger/common -> protocol/common).
	signable := opCertSignableBytes(
		opcert.KESVerificationKey,
		opcert.IssueNumber,
		opcert.KESPeriod,
	)

	// Verify signature using cold verification key
	if !ed25519strict.Verify(coldVerificationKey, signable, opcert.ColdSignature) {
		return errors.New("cold signature verification failed")
	}

	return nil
}

// opCertSignableBytes returns the bytes an operational certificate's cold key
// signs: the raw concatenation of the KES (hot) verification key, the issue
// number as big-endian uint64, and the KES period as big-endian uint64. This
// is the cardano-ledger OCertSignable representation
// (Cardano.Protocol.TPraos.OCert.OCertSignable), not a CBOR encoding. Kept in
// sync with ledger/common.OpCertSignableBytes, which this package cannot
// import without an import cycle (ledger -> ledger/common -> protocol/common).
func opCertSignableBytes(
	kesVkey []byte,
	issueNumber uint64,
	kesPeriod uint64,
) []byte {
	out := make([]byte, 0, len(kesVkey)+16)
	out = append(out, kesVkey...)
	out = binary.BigEndian.AppendUint64(out, issueNumber)
	out = binary.BigEndian.AppendUint64(out, kesPeriod)
	return out
}

// verifyKESSignature verifies the KES signature over the message payload (CBOR encoded).
// If slot is nil, a slot will be computed from the message's claimed signing
// period and configured slots per KES period. The certificate's own issuance
// period (msg.OperationalCertificate.KESPeriod) — not the message's claimed
// signing period (msg.Payload.KESPeriod) — is what a real KES evolution check
// needs as its baseline; this function keeps them distinct and rejects a
// message that claims to have been signed before its own certificate was
// issued.
func (m *MessageAuthenticator) verifyKESSignature(
	msg *DmqMessage,
	slot *uint64,
) error {
	if len(msg.KESSignature) != kes.CardanoKesSignatureSize {
		return fmt.Errorf(
			"KES signature must be %d bytes, got %d",
			kes.CardanoKesSignatureSize,
			len(msg.KESSignature),
		)
	}

	// Create CBOR encoding of the message payload as required by spec.
	payloadCbor, err := cbor.Encode(msg.Payload)
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

	if len(msg.OperationalCertificate.KESVerificationKey) != 32 {
		return errors.New("KES verification key must be 32 bytes")
	}

	// The certificate's own issuance period is the evolution baseline; the
	// message's claimed signing period must not precede it, or the message
	// is claiming a KES evolution that predates the key it was allegedly
	// signed with.
	certPeriod := msg.OperationalCertificate.KESPeriod
	msgPeriod := msg.Payload.KESPeriod
	if msgPeriod < certPeriod {
		return fmt.Errorf(
			"message KES period %d precedes certificate issuance period %d",
			msgPeriod,
			certPeriod,
		)
	}
	if slot == nil && m.slotsPerKesPeriod != 0 &&
		msgPeriod > math.MaxUint64/m.slotsPerKesPeriod {
		return ErrKESPeriodOverflow
	}
	if err := m.checkKESWindow(msgPeriod, certPeriod); err != nil {
		return err
	}

	// Absent an explicit slot, fall back to the slot implied by the
	// message's own claimed signing period. msgPeriod is attacker-controlled
	// (CIP-0137's CDDL says word32, but decoding doesn't enforce that range),
	// so reject outright when the conversion would overflow uint64 rather
	// than let it wrap to an unrelated, easy evolution.
	var computedSlot uint64
	if slot != nil {
		computedSlot = *slot
	} else {
		computedSlot = msgPeriod * m.slotsPerKesPeriod
	}
	currentKesPeriod := computedSlot / m.slotsPerKesPeriod
	if err := m.checkKESWindow(currentKesPeriod, certPeriod); err != nil {
		return err
	}

	m.logger.Debug(
		"KES verification using slot",
		"slot", computedSlot,
		"cert_period", certPeriod,
		"msg_period", msgPeriod,
	)

	valid, err := verifyKesComponents(
		wrappedCbor,
		msg.KESSignature,
		msg.OperationalCertificate.KESVerificationKey,
		certPeriod,
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
		"payload_size", len(wrappedCbor),
	)
	return nil
}

func (m *MessageAuthenticator) checkKESWindow(
	period uint64,
	certPeriod uint64,
) error {
	if period >= certPeriod &&
		period-certPeriod >= m.maxKESEvolutions {
		return fmt.Errorf(
			"KES period %d exceeds the maximum %d evolutions from certificate period %d",
			period,
			m.maxKESEvolutions,
			certPeriod,
		)
	}
	return nil
}

// verifyKesComponents verifies a KES signature the same way
// ledger.VerifyKesComponents does for block headers: it converts (kesPeriod,
// slot, slotsPerKesPeriod) into a KES evolution index and checks the
// signature at that evolution with kes.VerifySignedKES. Reimplemented
// locally (rather than imported) to avoid the import cycle importing
// ledger would create (ledger -> ledger/common -> protocol/common); the kes
// package itself has no such dependency and is imported directly.
func verifyKesComponents(
	message []byte,
	signature []byte,
	hotVkey []byte,
	kesPeriod uint64,
	slot uint64,
	slotsPerKesPeriod uint64,
) (bool, error) {
	if slotsPerKesPeriod == 0 {
		return false, errors.New("slotsPerKesPeriod must be greater than 0")
	}
	if len(signature) != kes.CardanoKesSignatureSize {
		return false, fmt.Errorf(
			"invalid KES signature length: expected %d bytes, got %d",
			kes.CardanoKesSignatureSize,
			len(signature),
		)
	}
	currentKesPeriod := slot / slotsPerKesPeriod
	if currentKesPeriod < kesPeriod {
		// Certificate start period is in the future - invalid.
		return false, nil
	}
	t := currentKesPeriod - kesPeriod
	return kes.VerifySignedKES(hotVkey, t, message, signature), nil
}

// verifyMessageID verifies the message ID format and size constraints.
func (m *MessageAuthenticator) verifyMessageID(msg *DmqMessage) error {
	messageID := msg.ID()
	if len(messageID) == 0 {
		return errors.New("message ID cannot be empty")
	}

	if len(messageID) != blake2b.Size256 {
		return fmt.Errorf(
			"message ID must be %d bytes, got %d",
			blake2b.Size256,
			len(messageID),
		)
	}

	expected, err := ComputeDmqMessageID(msg.Payload)
	if err != nil {
		return fmt.Errorf("failed to compute message ID: %w", err)
	}
	if !bytes.Equal(messageID, expected) {
		return fmt.Errorf(
			"message ID mismatch: expected %x, got %x",
			expected,
			messageID,
		)
	}
	msg.SetMessageID(messageID)
	return nil
}

// poolKeyHash derives a Cardano pool ID from its cold verification key:
// Blake2b-224(coldVerificationKey). This matches
// ledger/common.Blake2b224Hash byte-for-byte; reimplemented locally to
// avoid the ledger -> ledger/common -> protocol/common import cycle.
func poolKeyHash(coldVerificationKey []byte) (PoolKeyHash, error) {
	var out PoolKeyHash
	h, err := blake2b.New(PoolKeyHashSize, nil)
	if err != nil {
		return out, fmt.Errorf(
			"unexpected error generating empty blake2b-224 hash: %w",
			err,
		)
	}
	h.Write(coldVerificationKey)
	copy(out[:], h.Sum(nil))
	return out, nil
}

// verifyKESPeriodRotation verifies that the opcert number doesn't go backwards for each pool,
// preventing replay attacks with stale credentials.
func (m *MessageAuthenticator) verifyKESPeriodRotation(
	poolID PoolKeyHash,
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
//
// Removing an entry restores the pool to "never seen", so the next message
// from it is accepted at any issue number -- including one this
// authenticator has already rejected as stale. Call it only for a pool known
// to have retired. It is not needed to bound memory: the cache only ever
// holds pools that passed stake authorization and signature verification.
func (m *MessageAuthenticator) RemoveKESOpCertCacheEntry(poolID PoolKeyHash) {
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
// It evaluates the message against the current wall-clock time. For deterministic
// evaluation against a caller-supplied time, use [TTLValidator.ValidateMessageTTLAt].
func (v *TTLValidator) ValidateMessageTTL(msg *DmqMessage) error {
	return v.ValidateMessageTTLAt(msg, time.Now())
}

// ValidateMessageTTLAt checks if a message's expiration is valid (not expired and not too far in future)
// when evaluated at the provided point in time. This variant lets callers pin the
// evaluation moment explicitly, which is useful for tests, deterministic replay,
// or evaluating messages against a clock that is not the local wall clock.
//
// A disabled (no-op) validator returns nil regardless of the message or now value.
// A nil message returns an error. Otherwise the same max-TTL and expired/too-far-future
// rules used by [TTLValidator.ValidateMessageTTL] apply.
func (v *TTLValidator) ValidateMessageTTLAt(
	msg *DmqMessage,
	now time.Time,
) error {
	if v.disabled {
		return nil
	}
	if msg == nil {
		return errors.New("message is nil")
	}

	nowUnix := now.Unix()
	expiresAt := msg.Payload.ExpiresAt
	// Reject `now` past the uint32 domain explicitly: clamping to MaxUint32
	// would leave a message with expiresAt == MaxUint32 looking valid even
	// though any uint32 expiresAt is strictly in the past relative to such
	// a time.
	if nowUnix > math.MaxUint32 {
		return fmt.Errorf(
			"message has expired: now=%d, expiresAt=%d",
			nowUnix,
			expiresAt,
		)
	}
	if nowUnix < 0 {
		nowUnix = 0
	}
	// #nosec G115 -- bounded to [0, MaxUint32] above
	nowSec := uint32(nowUnix)

	// Check if message has already expired
	if nowSec > expiresAt {
		return fmt.Errorf(
			"message has expired: now=%d, expiresAt=%d",
			nowSec,
			expiresAt,
		)
	}

	// Check if expiration is too far in the future (protocol violation).
	// Compute in uint64 to avoid overflow when adding TTL seconds to current time.
	maxAllowedTTLSeconds := uint64(v.maxAllowedTTL.Seconds())
	nowUint64 := uint64(nowSec)
	maxAllowedExpirationUint64 := min(
		nowUint64+maxAllowedTTLSeconds,
		math.MaxUint32,
	)
	// #nosec G115 -- overflow prevented by clamping to MaxUint32 above
	maxAllowedExpiration := uint32(maxAllowedExpirationUint64)
	if expiresAt > maxAllowedExpiration {
		return fmt.Errorf(
			"message expiration too far in future: now=%d, expiresAt=%d, max_allowed=%d",
			nowSec,
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
