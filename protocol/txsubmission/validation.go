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
	seen := make(map[TxId]struct{}, len(remaining)+len(reply))
	for _, txIdAndSize := range remaining {
		seen[txIdAndSize.TxId] = struct{}{}
	}
	for _, txIdAndSize := range reply {
		if _, ok := seen[txIdAndSize.TxId]; ok {
			return nil, invalidTxSubmissionMessage(
				"reply repeats outstanding transaction ID %x",
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
