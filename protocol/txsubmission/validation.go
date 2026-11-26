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

package txsubmission

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
)

func invalidTxSubmissionMessage(format string, args ...any) error {
	return fmt.Errorf(
		"%s: %s: %w",
		ProtocolName,
		fmt.Sprintf(format, args...),
		protocol.ErrProtocolViolationInvalidMessage,
	)
}

func requestedTxIdsAreOutstanding(
	outstanding []TxIdAndSize,
	requested []TxId,
) error {
	if len(requested) > MaxUnackedTxIds {
		return fmt.Errorf(
			"%s: requested %d transaction IDs, limit %d: %w",
			ProtocolName,
			len(requested),
			MaxUnackedTxIds,
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	available := make(map[TxId]struct{}, len(outstanding))
	for _, txIdAndSize := range outstanding {
		available[txIdAndSize.TxId] = struct{}{}
	}
	seen := make(map[TxId]struct{}, len(requested))
	for _, txId := range requested {
		if _, ok := seen[txId]; ok {
			return invalidTxSubmissionMessage(
				"request contains duplicate transaction ID %x",
				txId.TxId,
			)
		}
		seen[txId] = struct{}{}
		if _, ok := available[txId]; !ok {
			return invalidTxSubmissionMessage(
				"request contains unannounced transaction ID %x",
				txId.TxId,
			)
		}
	}
	return nil
}

// checkRequestTxIdsStyle applies the rules on the blocking flag of a
// MsgRequestTxIds, given the number of transaction IDs unacknowledged before
// it. The flag is not a choice: a blocking request must leave nothing
// unacknowledged after its acknowledgement and ask for at least one ID, and a
// non-blocking request must leave something unacknowledged and acknowledge or
// ask for something. These are the reference implementation's
// ProtocolErrorRequestedNothing, ProtocolErrorRequestBlocking and
// ProtocolErrorRequestNonBlocking (Ouroboros.Network.TxSubmission.Outbound).
// The caller has already checked that ack does not exceed unacked.
func checkRequestTxIdsStyle(blocking bool, ack, req, unacked int) error {
	remaining := unacked - ack
	if blocking {
		if req == 0 {
			return invalidTxSubmissionMessage(
				"blocking request for no transaction IDs",
			)
		}
		if remaining != 0 {
			return invalidTxSubmissionMessage(
				"blocking request leaves %d transaction IDs unacknowledged",
				remaining,
			)
		}
		return nil
	}
	if req == 0 && ack == 0 {
		return invalidTxSubmissionMessage(
			"non-blocking request neither acknowledges nor requests" +
				" transaction IDs",
		)
	}
	if remaining == 0 {
		return invalidTxSubmissionMessage(
			"non-blocking request leaves no transaction IDs unacknowledged",
		)
	}
	return nil
}

// checkAdvertisedTxSizes refuses an announcement whose advertised size
// exceeds MaxTxSizeBytes, as the reference implementation does for
// max_TX_SIZE. It deliberately ignores Config.MaxTxSize: a peer advertises the
// wrapped wire size of a reply item, a few bytes more than the body that
// limit is compared with, so a body exactly at the limit is advertised above
// it.
func checkAdvertisedTxSizes(reply []TxIdAndSize) error {
	for _, txIdAndSize := range reply {
		if txIdAndSize.Size > MaxTxSizeBytes {
			return invalidTxSubmissionMessage(
				"transaction ID %x advertised size %d exceeds the limit %d",
				txIdAndSize.TxId.TxId,
				txIdAndSize.Size,
				MaxTxSizeBytes,
			)
		}
	}
	return nil
}

func reconcileTxIds(
	outstanding []TxIdAndSize,
	ack int,
	requested int,
	blocking bool,
	reply []TxIdAndSize,
) ([]TxIdAndSize, error) {
	if ack < 0 || ack > len(outstanding) {
		return nil, fmt.Errorf(
			"%s: acknowledgement %d exceeds %d outstanding transaction IDs: %w",
			ProtocolName,
			ack,
			len(outstanding),
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	if len(reply) > requested {
		return nil, fmt.Errorf(
			"%s: reply contains %d transaction IDs for a request of %d: %w",
			ProtocolName,
			len(reply),
			requested,
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	if blocking && requested > 0 && len(reply) == 0 {
		return nil, invalidTxSubmissionMessage(
			"blocking request for %d transaction IDs received an empty reply",
			requested,
		)
	}
	remaining := outstanding[ack:]
	if len(remaining)+len(reply) > MaxUnackedTxIds {
		return nil, fmt.Errorf(
			"%s: reply would leave %d transaction IDs outstanding, limit %d: %w",
			ProtocolName,
			len(remaining)+len(reply),
			MaxUnackedTxIds,
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	// A reply may name an ID that is still outstanding: the outbound
	// announces mempool entries after the last index it sent, and a
	// transaction removed and added back (a fork switch) has a new index.
	// The reference inbound keeps both entries and acknowledges by count.
	// One reply comes from one mempool snapshot, so it never names an ID
	// twice, and one that does is refused.
	seen := make(map[TxId]struct{}, len(reply))
	for _, txIdAndSize := range reply {
		if _, ok := seen[txIdAndSize.TxId]; ok {
			return nil, invalidTxSubmissionMessage(
				"reply repeats transaction ID %x",
				txIdAndSize.TxId.TxId,
			)
		}
		seen[txIdAndSize.TxId] = struct{}{}
	}
	ret := make([]TxIdAndSize, 0, len(remaining)+len(reply))
	ret = append(ret, remaining...)
	ret = append(ret, reply...)
	return ret, nil
}

func validateAndOrderTxBodies(
	requested []TxId,
	bodies []TxBody,
	advertisedSizes map[TxId]uint32,
	maxTxSize int,
) ([]TxBody, error) {
	if len(requested) > MaxUnackedTxIds {
		return nil, fmt.Errorf(
			"%s: requested %d transaction bodies, limit %d: %w",
			ProtocolName,
			len(requested),
			MaxUnackedTxIds,
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	if len(bodies) > len(requested) {
		return nil, fmt.Errorf(
			"%s: reply contains %d transaction bodies for a request of %d: %w",
			ProtocolName,
			len(bodies),
			len(requested),
			protocol.ErrProtocolViolationRequestExceeded,
		)
	}
	requestedSet := make(map[TxId]struct{}, len(requested))
	for _, txId := range requested {
		if _, ok := requestedSet[txId]; ok {
			return nil, invalidTxSubmissionMessage(
				"request contains duplicate transaction ID %x",
				txId.TxId,
			)
		}
		requestedSet[txId] = struct{}{}
	}

	retainedBytes := 0
	decodedBytes := 0
	bodiesById := make(map[TxId]TxBody, len(bodies))
	for _, body := range bodies {
		bodySize := len(body.TxBody)
		if bodySize == 0 || bodySize > MaxTxSizeBytes {
			return nil, invalidTxSubmissionMessage(
				"transaction body size %d is outside the accepted range 1..%d",
				bodySize,
				MaxTxSizeBytes,
			)
		}
		// The check precedes the decode below: the decoder's work grows
		// with the body, and the peer controls the advertised size that the
		// later comparison trusts.
		if bodySize > maxTxSize {
			return nil, invalidTxSubmissionMessage(
				"transaction body size %d exceeds the configured limit %d",
				bodySize,
				maxTxSize,
			)
		}
		if bodySize > MaxPendingMessageBytes-retainedBytes {
			return nil, invalidTxSubmissionMessage(
				"transaction body reply exceeds retained-byte limit %d",
				MaxPendingMessageBytes,
			)
		}
		retainedBytes += bodySize
		if bodySize > MaxDecodedTxBytes-decodedBytes {
			return nil, invalidTxSubmissionMessage(
				"transaction body reply exceeds decode-input limit %d",
				MaxDecodedTxBytes,
			)
		}
		decodedBytes += bodySize

		tx, err := ledger.NewTransactionFromCbor(uint(body.EraId), body.TxBody)
		if err != nil {
			return nil, invalidTxSubmissionMessage(
				"cannot decode era %d transaction body: %v",
				body.EraId,
				err,
			)
		}
		if tx.Type() != int(body.EraId) {
			return nil, invalidTxSubmissionMessage(
				"transaction decoder returned era %d for era %d body",
				tx.Type(),
				body.EraId,
			)
		}
		actualId := TxId{EraId: body.EraId}
		hash := tx.Id()
		copy(actualId.TxId[:], hash[:])
		if _, ok := requestedSet[actualId]; !ok {
			return nil, invalidTxSubmissionMessage(
				"reply contains unrequested transaction ID %x in era %d",
				actualId.TxId,
				actualId.EraId,
			)
		}
		if _, ok := bodiesById[actualId]; ok {
			return nil, invalidTxSubmissionMessage(
				"reply contains duplicate transaction ID %x in era %d",
				actualId.TxId,
				actualId.EraId,
			)
		}
		if advertisedSize, ok := advertisedSizes[actualId]; ok {
			actualSize := uint64(bodySize)
			advertised := uint64(advertisedSize)
			var difference uint64
			if advertised > actualSize {
				difference = advertised - actualSize
			} else {
				difference = actualSize - advertised
			}
			if difference > 32 {
				return nil, invalidTxSubmissionMessage(
					"transaction ID %x advertised size %d differs from reply size %d by more than 32 bytes",
					actualId.TxId,
					advertisedSize,
					bodySize,
				)
			}
		}
		bodiesById[actualId] = body
	}

	ret := make([]TxBody, 0, len(bodies))
	for _, txId := range requested {
		if body, ok := bodiesById[txId]; ok {
			ret = append(ret, body)
		}
	}
	return ret, nil
}
