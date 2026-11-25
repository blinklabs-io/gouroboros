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

// MinMessageBodySize and MaxMessageBodySize bound a DMQ message body, in
// bytes: CIP-0137 messageBody = bstr .size (90..2000).
const (
	MinMessageBodySize = 90
	MaxMessageBodySize = 2000
)

// DefaultMinSigDelay is dmq-node's minimum delay between two messages from
// one pool (DMQ.Policy.minSigDelay).
const DefaultMinSigDelay = 60 * time.Second

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
	// ErrMessageBodySize is returned when a message body is outside
	// [MinMessageBodySize, MaxMessageBodySize].
	ErrMessageBodySize = errors.New(
		"protocol/common: message body size out of range",
	)
	// ErrMessageTooFrequent is returned when a pool's message arrives less
	// than the minimum delay after the last message accepted from that pool.
	ErrMessageTooFrequent = errors.New(
		"protocol/common: message too soon after the pool's last message",
	)
	// ErrKESPeriodOverflow reported a claimed KES period too large to convert
	// to a slot.
	//
	// Deprecated: verification always uses a trusted slot and never converts
	// a claimed KES period to a slot, so this error is no longer returned.
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

	// lastSig and acceptedIDs back the per-pool rate limit, guarded by mu.
	// lastSig holds the time of each pool's last accepted message;
	// acceptedIDs the IDs of accepted, unexpired messages, which verify
	// again without the rate limit, as dmq-node never re-validates a
	// message already in its mempool. Both are written only once every
	// check has passed, so their keys are staked pools and the messages
	// those pools signed: at most one per pool per minSigDelay, kept until
	// expiry.
	lastSig     map[PoolKeyHash]time.Time
	acceptedIDs map[string]uint32
	lastPrune   time.Time
	minSigDelay time.Duration
	now         func() time.Time

	// parent and batchAccepted are set only on PrepareMessages' private
	// authenticator: rate-limit lookups fall through to parent, and the
	// batch's acceptances wait in batchAccepted until commit.
	parent        *MessageAuthenticator
	batchAccepted []rateRecord

	// Maximum evolution distance allowed from an operational certificate.
	maxKESEvolutions uint64
}

// rateRecord is one accepted message, as the rate limit records it.
type rateRecord struct {
	pool      PoolKeyHash
	id        string
	expiresAt uint32
	at        time.Time
}

// MessageAuthenticatorConfig configures a MessageAuthenticator.
type MessageAuthenticatorConfig struct {
	// StakeAuthority backs the pool-authorization check. Required; a nil
	// StakeAuthority is a configuration error rather than a silent skip of
	// the check.
	StakeAuthority StakeAuthority
	// CurrentSlot is not used. A message's KES signature is checked at the
	// period the message claims, inside its operational certificate's window,
	// and the node's own slot plays no part, as in dmq-node.
	//
	// Deprecated: ignored; leave it nil.
	CurrentSlot func() (uint64, error)
	// PoolOpCertIssueNumber is not used. A message's operational-certificate
	// counter is compared only with the counters seen in earlier messages from
	// the same pool, as dmq-node does; comparing it with the chain's counter
	// would refuse messages dmq-node accepts and relays.
	//
	// Deprecated: ignored; leave it nil.
	PoolOpCertIssueNumber func(PoolKeyHash) (uint64, bool, error)
	// SlotsPerKESPeriod is not used: no slot is converted to a KES period.
	//
	// Deprecated: ignored; leave it zero.
	SlotsPerKESPeriod uint64
	// MaxKESEvolutions is the Shelley genesis maxKESEvolutions parameter.
	// Defaults to 62 (Cardano mainnet) when zero.
	MaxKESEvolutions uint64
	// MinSigDelay is the least time between two messages accepted from one
	// pool. Defaults to DefaultMinSigDelay when zero; a negative value turns
	// the limit off, as a vcMinSigDelay of zero does in dmq-node.
	MinSigDelay time.Duration
	// Now returns the current time, read once per verification call.
	// Defaults to time.Now.
	Now func() time.Time
	// Logger, when nil, defaults to slog.Default().
	Logger *slog.Logger
}

// NewMessageAuthenticator constructs a MessageAuthenticator. It returns
// ErrAuthenticatorMisconfigured when any authoritative chain-state lookup is
// missing.
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
	maxKESEvolutions := cfg.MaxKESEvolutions
	if maxKESEvolutions == 0 {
		maxKESEvolutions = DefaultMaxKESEvolutions
	}
	minSigDelay := cfg.MinSigDelay
	switch {
	case minSigDelay == 0:
		minSigDelay = DefaultMinSigDelay
	case minSigDelay < 0:
		minSigDelay = 0
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &MessageAuthenticator{
		lastSig:          make(map[PoolKeyHash]time.Time),
		acceptedIDs:      make(map[string]uint32),
		minSigDelay:      minSigDelay,
		now:              now,
		logger:           logger,
		stakeAuthority:   cfg.StakeAuthority,
		kesOpCertCache:   make(map[PoolKeyHash]uint64),
		maxKESEvolutions: maxKESEvolutions,
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
		maxKESEvolutions:  DefaultMaxKESEvolutions,
		disableValidation: true,
	}
}

// VerifyMessage performs complete message authentication as per CIP-0137.
// It verifies: message ID, pool-ID derivation and stake-distribution
// authorization, operational certificate, KES signature, and KES period
// rotation. Returns error if verification fails (which is a protocol
// violation and should result in peer disconnection).
func (m *MessageAuthenticator) VerifyMessage(msg *DmqMessage) error {
	if m.disableValidation {
		return nil
	}
	if msg == nil {
		return errors.New("message is nil")
	}
	// Prepare and commit, so that the counter and the rate limit are
	// recorded together or not at all.
	return m.VerifyMessages([]DmqMessage{*msg})
}

// PrepareMessages verifies a batch and returns a function that atomically commits its operational-certificate
// observations. Verification does not mutate the authenticator until the
// returned function succeeds.
func (m *MessageAuthenticator) PrepareMessages(
	messages []DmqMessage,
) (func() error, error) {
	if m.disableValidation || len(messages) == 0 {
		return func() error { return nil }, nil
	}
	m.mu.Lock()
	cacheSnapshot := make(map[PoolKeyHash]uint64, len(m.kesOpCertCache))
	for poolID, issueNumber := range m.kesOpCertCache {
		cacheSnapshot[poolID] = issueNumber
	}
	m.mu.Unlock()
	batchNow := m.now()
	batchAuthenticator := &MessageAuthenticator{
		parent:           m,
		minSigDelay:      m.minSigDelay,
		now:              func() time.Time { return batchNow },
		logger:           m.logger,
		stakeAuthority:   m.stakeAuthority,
		kesOpCertCache:   cacheSnapshot,
		maxKESEvolutions: m.maxKESEvolutions,
	}
	// lowestIssue keeps each touched pool's lowest counter in the batch. At
	// commit every message must still be no lower than the pool's recorded
	// counter, as if it were validated then; checking only the batch's
	// final counter would let an earlier, lower one through after a
	// concurrent commit recorded a higher one.
	lowestIssue := make(map[PoolKeyHash]uint64)
	for i := range messages {
		if err := batchAuthenticator.verifyMessageInternal(&messages[i]); err != nil {
			return nil, fmt.Errorf(
				"message %d authentication failed: %w",
				i,
				err,
			)
		}
		poolID, err := poolKeyHash(messages[i].ColdVerificationKey)
		if err != nil {
			return nil, fmt.Errorf("message %d compute pool id: %w", i, err)
		}
		issue := messages[i].OperationalCertificate.IssueNumber
		if lowest, seen := lowestIssue[poolID]; !seen || issue < lowest {
			lowestIssue[poolID] = issue
		}
	}

	return func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		for poolID, issueNumber := range lowestIssue {
			if currentIssue, exists := m.kesOpCertCache[poolID]; exists &&
				issueNumber < currentIssue {
				return fmt.Errorf(
					"opcert number went backwards: previous=%d, current=%d",
					currentIssue,
					issueNumber,
				)
			}
		}
		// A message accepted meanwhile by another commit is a duplicate:
		// valid, and already recorded, so it is neither refused nor
		// recorded again.
		var fresh []rateRecord
		for _, r := range batchAuthenticator.batchAccepted {
			if _, held := m.acceptedIDs[r.id]; held {
				continue
			}
			if last, ok := m.lastSig[r.pool]; ok &&
				r.at.Sub(last) < m.minSigDelay {
				return fmt.Errorf(
					"%w: %s since the last",
					ErrMessageTooFrequent,
					r.at.Sub(last),
				)
			}
			fresh = append(fresh, r)
		}
		for _, r := range fresh {
			m.recordRateLocked(r)
		}
		for poolID := range lowestIssue {
			issueNumber := batchAuthenticator.kesOpCertCache[poolID]
			if currentIssue, exists := m.kesOpCertCache[poolID]; !exists ||
				issueNumber > currentIssue {
				m.kesOpCertCache[poolID] = issueNumber
			}
		}
		return nil
	}, nil
}

// VerifyMessages verifies a batch and commits its authentication state.
func (m *MessageAuthenticator) VerifyMessages(messages []DmqMessage) error {
	commit, err := m.PrepareMessages(messages)
	if err != nil {
		return err
	}
	return commit()
}

// VerifyMessageWithSlot verifies a message as VerifyMessage does. The slot
// is not used.
//
// Deprecated: use VerifyMessage; no check depends on the slot.
func (m *MessageAuthenticator) VerifyMessageWithSlot(
	msg *DmqMessage,
	_ uint64,
) error {
	return m.verifyMessageInternal(msg)
}

// verifyMessageInternal contains the core verification logic.
//
// Stake authorization runs before either signature check. Deriving a pool ID
// from ColdVerificationKey needs no signature -- anyone can self-sign an
// internally consistent opcert/KES chain over freshly generated keys, so the
// signature checks only prove the sender holds the claimed private keys, not
// that those keys belong to a real, staked pool. Checking authorization
// first turns away a message from an unregistered identity before paying
// for an ed25519 verify and the KES verify, rather than after.
func (m *MessageAuthenticator) verifyMessageInternal(msg *DmqMessage) error {
	if m.disableValidation {
		return nil
	}

	if msg == nil {
		return errors.New("message is nil")
	}

	// A decoded message was size-checked already; this covers one built
	// locally, before it is relayed.
	if err := checkMessageBodySize(msg.Payload.MessageBody); err != nil {
		return err
	}

	poolID, err := poolKeyHash(msg.ColdVerificationKey)
	if err != nil {
		return fmt.Errorf("compute pool id: %w", err)
	}

	// The rate limit comes first, as in dmq-node (Validate.hs 124-145,
	// after only the expiry checks), so a too-frequent message is refused
	// as such whatever else is wrong with it.
	now := m.now()
	held, last, seen := m.rateLookup(poolID, string(msg.ID()))
	if !held && seen && now.Sub(last) < m.minSigDelay {
		return fmt.Errorf(
			"%w: %s since the last",
			ErrMessageTooFrequent,
			now.Sub(last),
		)
	}

	// Step 1: Verify deterministic message ID before heavier auth checks.
	if err := m.verifyMessageID(msg); err != nil {
		return fmt.Errorf("message ID verification failed: %w", err)
	}

	// Step 2: Stake authorization before either signature check -- see the
	// doc comment above for why.
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

	// Step 4: Verify KES signature over message payload.
	if err := m.verifyKESSignature(msg); err != nil {
		return fmt.Errorf("KES signature verification failed: %w", err)
	}

	// Step 5: Verify KES period rotation (opcert number doesn't go backwards).
	// Not updated until every earlier check has passed, so a message that
	// fails an earlier check cannot poison replay protection for a later,
	// legitimately higher-numbered certificate from the same pool.
	if err := m.verifyKESPeriodRotation(poolID, &msg.OperationalCertificate); err != nil {
		return fmt.Errorf("KES period rotation verification failed: %w", err)
	}

	if !held {
		return m.recordRate(rateRecord{
			pool:      poolID,
			id:        string(msg.ID()),
			expiresAt: msg.Payload.ExpiresAt,
			at:        now,
		})
	}
	return nil
}

// rateLookup reports whether message id is already accepted, and the time
// of pool's last accepted message. In a batch the batch's own acceptances
// come first.
func (m *MessageAuthenticator) rateLookup(
	pool PoolKeyHash,
	id string,
) (bool, time.Time, bool) {
	var last time.Time
	seen := false
	for _, r := range m.batchAccepted {
		if r.id == id {
			return true, time.Time{}, false
		}
		if r.pool == pool {
			last, seen = r.at, true
		}
	}
	owner := m
	if m.parent != nil {
		owner = m.parent
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if _, held := owner.acceptedIDs[id]; held {
		return true, time.Time{}, false
	}
	if !seen {
		last, seen = owner.lastSig[pool]
	}
	return false, last, seen
}

// recordRate records an accepted message: in a batch, until commit;
// otherwise at once, re-checking the limit under the lock in case another
// message of the pool was accepted since the lookup.
func (m *MessageAuthenticator) recordRate(r rateRecord) error {
	if m.parent != nil {
		m.batchAccepted = append(m.batchAccepted, r)
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, held := m.acceptedIDs[r.id]; held {
		return nil
	}
	if last, ok := m.lastSig[r.pool]; ok && r.at.Sub(last) < m.minSigDelay {
		return fmt.Errorf(
			"%w: %s since the last",
			ErrMessageTooFrequent,
			r.at.Sub(last),
		)
	}
	m.recordRateLocked(r)
	return nil
}

// recordRateLocked stores r and, at most once a minute, forgets accepted IDs
// whose messages have expired. The caller holds m.mu.
func (m *MessageAuthenticator) recordRateLocked(r rateRecord) {
	m.lastSig[r.pool] = r.at
	m.acceptedIDs[r.id] = r.expiresAt
	if r.at.Sub(m.lastPrune) < time.Minute {
		return
	}
	m.lastPrune = r.at
	for id, expiresAt := range m.acceptedIDs {
		if !r.at.Before(time.Unix(int64(expiresAt), 0)) {
			delete(m.acceptedIDs, id)
		}
	}
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
	if !ed25519strict.Verify(
		coldVerificationKey,
		signable,
		opcert.ColdSignature,
	) {
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

// verifyKESSignature verifies the KES signature over the message payload (CBOR
// encoded). The signature is checked at the evolution from the certificate's
// issuance period (msg.OperationalCertificate.KESPeriod) to the message's
// claimed signing period (msg.Payload.KESPeriod), which must lie in the
// certificate's window. As in dmq-node, the node's own slot is not
// consulted: a certificate that has ended, or not yet started, at the
// node's current period does not by itself make a message invalid.
func (m *MessageAuthenticator) verifyKESSignature(msg *DmqMessage) error {
	if len(msg.KESSignature) != kes.CardanoKesSignatureSize {
		return fmt.Errorf(
			"KES signature must be %d bytes, got %d",
			kes.CardanoKesSignatureSize,
			len(msg.KESSignature),
		)
	}

	// The KES signature covers the payload array's own CBOR bytes, the same
	// bytes the message ID hashes. Wrapping them in a byte string first
	// produces a different signed message than dmq-node signs and verifies.
	payloadCbor, err := cbor.Encode(msg.Payload)
	if err != nil {
		return fmt.Errorf("failed to encode payload: %w", err)
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
	if err := m.checkKESWindow(msgPeriod, certPeriod); err != nil {
		return err
	}

	m.logger.Debug(
		"KES verification",
		"cert_period", certPeriod,
		"msg_period", msgPeriod,
	)

	// The signer evolves its key to the period it claims, so the signature
	// is checked at that evolution, not at the node's current period: a
	// message signed late in one period is still valid in the next
	// (dmq-node Validate.hs verifyKES at sigKESPeriod - ocertKESPeriod).
	if !kes.VerifySignedKES(
		msg.OperationalCertificate.KESVerificationKey,
		msgPeriod-certPeriod,
		payloadCbor,
		msg.KESSignature,
	) {
		return errors.New("KES signature verification failed")
	}
	m.logger.Debug(
		"KES signature verified",
		"payload_size", len(payloadCbor),
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

// ErrMessageExpired indicates that a DMQ message's expiration time has passed.
var ErrMessageExpired = errors.New("message has expired")

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

	// Compare at the clock's full precision: dmq-node's now is a POSIXTime
	// with sub-second precision, so a message is already expired half a
	// second after its whole-second expiresAt. Truncating now to seconds
	// would accept it.
	expiresAt := msg.Payload.ExpiresAt
	expiry := time.Unix(int64(expiresAt), 0)
	if now.After(expiry) {
		return fmt.Errorf(
			"%w: now=%s, expiresAt=%d",
			ErrMessageExpired,
			now.UTC().Format(time.RFC3339Nano),
			expiresAt,
		)
	}

	// The bound is exclusive, as in dmq-node (expiresAt < now + maxTTL): an
	// expiry exactly maxTTL ahead is a protocol violation.
	bound := now.Add(v.maxAllowedTTL)
	if !expiry.Before(bound) {
		return fmt.Errorf(
			"message expiration too far in future: now=%s, expiresAt=%d, bound=%s",
			now.UTC().Format(time.RFC3339Nano),
			expiresAt,
			bound.UTC().Format(time.RFC3339Nano),
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
