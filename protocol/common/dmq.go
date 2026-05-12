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
	"errors"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"golang.org/x/crypto/blake2b"
)

// DMQ Message structures following CIP-0137 specification
// https://github.com/cardano-foundation/CIPs/tree/master/CIP-0137

// DmqMessagePayload represents the unsigned message payload for DMQ messages.
type DmqMessagePayload struct {
	cbor.StructAsArray
	// MessageID is a legacy alias kept for source compatibility. It is not
	// part of the current CIP-0137 messagePayload CBOR encoding.
	MessageID []byte `cbor:"-"`
	// MessageBody contains the actual message data
	MessageBody []byte
	// KESPeriod is the KES key evolution step period
	KESPeriod uint64
	// ExpiresAt is the Unix timestamp when the message expires
	ExpiresAt uint32
}

// OperationalCertificate represents an SPO's operational certificate used for message authentication.
type OperationalCertificate struct {
	cbor.StructAsArray
	// KESVerificationKey is the 32-byte KES public key
	KESVerificationKey []byte
	// IssueNumber is the certificate issue number (for rotation tracking)
	IssueNumber uint64
	// KESPeriod is the KES period at certificate creation
	KESPeriod uint64
	// ColdSignature is the 64-byte signature by the cold key
	ColdSignature []byte
}

// DmqMessage represents a complete authenticated DMQ message with cryptographic proofs.
type DmqMessage struct {
	cbor.StructAsArray
	// MessageID is the Blake2b-256 hash of the CBOR-encoded payload.
	MessageID []byte
	// Payload is the unsigned message payload
	Payload DmqMessagePayload
	// KESSignature is the 448-byte KES signature over the payload
	KESSignature []byte
	// OperationalCertificate contains the KES certificate and cold signature
	OperationalCertificate OperationalCertificate
	// ColdVerificationKey is the 32-byte SPO cold public key
	ColdVerificationKey []byte
}

// MarshalCBOR encodes the payload using the current CIP-0137 shape:
// [messageBody, kesPeriod, expiresAt].
func (p DmqMessagePayload) MarshalCBOR() ([]byte, error) {
	return cbor.Encode([]any{
		p.MessageBody,
		p.KESPeriod,
		p.ExpiresAt,
	})
}

// UnmarshalCBOR decodes both the current payload shape and the legacy V1
// implementation shape, which included messageId inside messagePayload.
func (p *DmqMessagePayload) UnmarshalCBOR(data []byte) error {
	var current struct {
		cbor.StructAsArray
		MessageBody []byte
		KESPeriod   uint64
		ExpiresAt   uint32
	}
	if _, err := cbor.Decode(data, &current); err == nil {
		p.MessageID = nil
		p.MessageBody = current.MessageBody
		p.KESPeriod = current.KESPeriod
		p.ExpiresAt = current.ExpiresAt
		return nil
	}

	var legacy struct {
		cbor.StructAsArray
		MessageID   []byte
		MessageBody []byte
		KESPeriod   uint64
		ExpiresAt   uint32
	}
	if _, err := cbor.Decode(data, &legacy); err != nil {
		return err
	}
	p.MessageID = legacy.MessageID
	p.MessageBody = legacy.MessageBody
	p.KESPeriod = legacy.KESPeriod
	p.ExpiresAt = legacy.ExpiresAt
	return nil
}

// MarshalCBOR encodes the message using the current CIP-0137 shape:
// [messageId, messagePayload, kesSignature, operationalCertificate,
// coldVerificationKey].
func (m DmqMessage) MarshalCBOR() ([]byte, error) {
	msgID := m.ID()
	if len(msgID) == 0 {
		var err error
		msgID, err = ComputeDmqMessageID(m.Payload)
		if err != nil {
			return nil, err
		}
	}
	return cbor.Encode([]any{
		msgID,
		m.Payload,
		m.KESSignature,
		m.OperationalCertificate,
		m.ColdVerificationKey,
	})
}

// UnmarshalCBOR decodes both the current CIP-0137 message shape and the
// legacy V1 implementation shape, which encoded [payload, signature, opcert,
// coldKey] and kept messageId inside payload.
func (m *DmqMessage) UnmarshalCBOR(data []byte) error {
	var current struct {
		cbor.StructAsArray
		MessageID              []byte
		Payload                DmqMessagePayload
		KESSignature           []byte
		OperationalCertificate OperationalCertificate
		ColdVerificationKey    []byte
	}
	if _, err := cbor.Decode(data, &current); err == nil {
		m.MessageID = current.MessageID
		m.Payload = current.Payload
		m.Payload.MessageID = current.MessageID
		m.KESSignature = current.KESSignature
		m.OperationalCertificate = current.OperationalCertificate
		m.ColdVerificationKey = current.ColdVerificationKey
		return nil
	}

	var legacy struct {
		cbor.StructAsArray
		Payload                DmqMessagePayload
		KESSignature           []byte
		OperationalCertificate OperationalCertificate
		ColdVerificationKey    []byte
	}
	if _, err := cbor.Decode(data, &legacy); err != nil {
		return err
	}
	m.MessageID = legacy.Payload.MessageID
	m.Payload = legacy.Payload
	m.KESSignature = legacy.KESSignature
	m.OperationalCertificate = legacy.OperationalCertificate
	m.ColdVerificationKey = legacy.ColdVerificationKey
	return nil
}

// ID returns the message id, accepting Payload.MessageID as a legacy alias.
func (m DmqMessage) ID() []byte {
	if len(m.MessageID) > 0 {
		return m.MessageID
	}
	return m.Payload.MessageID
}

// SetMessageID updates both the current field and the legacy payload alias.
func (m *DmqMessage) SetMessageID(messageID []byte) {
	m.MessageID = cloneBytes(messageID)
	m.Payload.MessageID = cloneBytes(messageID)
}

// SetComputedMessageID computes and stores the CIP-0137 message id.
func (m *DmqMessage) SetComputedMessageID() error {
	messageID, err := ComputeDmqMessageID(m.Payload)
	if err != nil {
		return err
	}
	m.SetMessageID(messageID)
	return nil
}

// ComputeDmqMessageID returns Blake2b-256(cbor(messagePayload)).
func ComputeDmqMessageID(payload DmqMessagePayload) ([]byte, error) {
	payload.MessageID = nil
	payloadCbor, err := cbor.Encode(payload)
	if err != nil {
		return nil, err
	}
	sum := blake2b.Sum256(payloadCbor)
	return cloneBytes(sum[:]), nil
}

// MarshalDmqMessageLegacyCBOR encodes the legacy V1 wire shape used before
// CIP-0137 moved messageId from messagePayload to message.
func MarshalDmqMessageLegacyCBOR(msg DmqMessage) ([]byte, error) {
	legacyPayload := struct {
		cbor.StructAsArray
		MessageID   []byte
		MessageBody []byte
		KESPeriod   uint64
		ExpiresAt   uint32
	}{
		MessageID:   msg.ID(),
		MessageBody: msg.Payload.MessageBody,
		KESPeriod:   msg.Payload.KESPeriod,
		ExpiresAt:   msg.Payload.ExpiresAt,
	}
	legacyMessage := struct {
		cbor.StructAsArray
		Payload                any
		KESSignature           []byte
		OperationalCertificate OperationalCertificate
		ColdVerificationKey    []byte
	}{
		Payload:                legacyPayload,
		KESSignature:           msg.KESSignature,
		OperationalCertificate: msg.OperationalCertificate,
		ColdVerificationKey:    msg.ColdVerificationKey,
	}
	return cbor.Encode(legacyMessage)
}

func cloneBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	ret := make([]byte, len(src))
	copy(ret, src)
	return ret
}

// MessageIDAndSize represents a message identifier with its serialized size in bytes.
type MessageIDAndSize struct {
	cbor.StructAsArray
	// MessageID is the unique message identifier
	MessageID []byte
	// SizeInBytes is the total size of the serialized message
	SizeInBytes uint32
}

// IsValid checks if a message has not yet expired by comparing its expiration time
// against the current Unix timestamp.
func (m *DmqMessage) IsValid() bool {
	// Delegate to IsValidAt using current time for testability
	// #nosec G115 -- Unix timestamp will not overflow uint32 until year 2106
	return m.IsValidAt(time.Now())
}

// IsValidAt checks if a message has not yet expired when evaluated at the provided time.
// This variant is provided for testability so callers can validate expiration behavior
// deterministically by passing a specific timestamp.
func (m *DmqMessage) IsValidAt(t time.Time) bool {
	// #nosec G115 -- Unix timestamp will not overflow uint32 until year 2106
	return uint32(t.Unix()) <= m.Payload.ExpiresAt
}

// RejectReason provides information about why a submitted message was rejected.
// Implementations must support the following reason types via RejectReasonType().
type RejectReason interface {
	// RejectReasonType returns the reason code (0-3)
	RejectReasonType() uint8
}

// InvalidReason indicates the message failed validation checks.
type InvalidReason struct {
	// Message describes the validation error
	Message string
}

// RejectReasonType returns 0 for invalid reason
func (r InvalidReason) RejectReasonType() uint8 {
	return 0
}

// AlreadyReceivedReason indicates the message was already received and processed.
type AlreadyReceivedReason struct{}

// RejectReasonType returns 1 for already received reason
func (r AlreadyReceivedReason) RejectReasonType() uint8 {
	return 1
}

// ExpiredReason indicates the message's TTL has passed.
type ExpiredReason struct{}

// RejectReasonType returns 2 for expired reason
func (r ExpiredReason) RejectReasonType() uint8 {
	return 2
}

// OtherReason indicates some other rejection cause not covered by the standard reasons.
type OtherReason struct {
	// Message describes the other rejection reason
	Message string
}

// RejectReasonType returns 3 for other reason
func (r OtherReason) RejectReasonType() uint8 {
	return 3
}

// RejectReasonData is a concrete, CBOR-marshalable representation of a reject reason.
// It encodes as an array [type, message?] where type is 0..3 and message is optional.
type RejectReasonData struct {
	cbor.StructAsArray
	Type    uint8
	Message string
}

// RejectReasonType implements the RejectReason interface.
func (r RejectReasonData) RejectReasonType() uint8 {
	return r.Type
}

// ToRejectReasonData converts any RejectReason implementation into a concrete RejectReasonData.
func ToRejectReasonData(rr RejectReason) RejectReasonData {
	if rr == nil {
		return RejectReasonData{Type: 3}
	}
	switch v := rr.(type) {
	case InvalidReason:
		return RejectReasonData{Type: 0, Message: v.Message}
	case AlreadyReceivedReason:
		return RejectReasonData{Type: 1}
	case ExpiredReason:
		return RejectReasonData{Type: 2}
	case OtherReason:
		return RejectReasonData{Type: 3, Message: v.Message}
	case RejectReasonData:
		return v
	default:
		// Fallback: encode as OtherReason with type 3
		return RejectReasonData{Type: 3}
	}
}

// MarshalCBOR encodes RejectReasonData as an array [type, message?] for deterministic wire format.
func (r RejectReasonData) MarshalCBOR() ([]byte, error) {
	// Encode as array [type, message]
	// If message is empty, it still appears in the array (empty string) to keep structure consistent
	data := []any{r.Type, r.Message}
	return cbor.Encode(data)
}

// UnmarshalCBOR decodes a CBOR array [type, message?] back into RejectReasonData.
func (r *RejectReasonData) UnmarshalCBOR(data []byte) error {
	if r == nil {
		return errors.New("cannot unmarshal CBOR into nil RejectReasonData")
	}
	// Decode as a CBOR array
	var arr []any
	if _, err := cbor.Decode(data, &arr); err != nil {
		return fmt.Errorf("failed to decode RejectReasonData: %w", err)
	}

	if len(arr) < 1 {
		return fmt.Errorf(
			"RejectReasonData array must have at least 1 element (type), got %d",
			len(arr),
		)
	}

	// Extract type (first element)
	typeVal, ok := arr[0].(uint64)
	if !ok {
		return fmt.Errorf("RejectReasonData type must be uint, got %T", arr[0])
	}
	if typeVal > 3 {
		return fmt.Errorf("RejectReasonData type out of range: %d", typeVal)
	}
	r.Type = uint8(typeVal)

	// Extract message (second element, if present)
	r.Message = ""
	if len(arr) > 1 && arr[1] != nil {
		msg, ok := arr[1].(string)
		if !ok {
			return fmt.Errorf(
				"RejectReasonData message must be string or nil, got %T",
				arr[1],
			)
		}
		r.Message = msg
	}

	return nil
}

// FromRejectReasonData converts RejectReasonData back to a concrete public API type.
// This is used during decoding to reconstruct the public RejectReason interface type.
func FromRejectReasonData(data RejectReasonData) RejectReason {
	switch data.Type {
	case 0:
		return InvalidReason{Message: data.Message}
	case 1:
		return AlreadyReceivedReason{}
	case 2:
		return ExpiredReason{}
	case 3:
		return OtherReason{Message: data.Message}
	default:
		return OtherReason{Message: data.Message}
	}
}
