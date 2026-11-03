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

package ledger

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// maxRejectionItems bounds the total number of CBOR data items in one
// transaction rejection reason. The decoder's per-collection limit does not
// bound a payload made of many collections, and a rejection is peer-supplied.
// A rejection echoes at most a transaction's worth of data (tens of thousands
// of items for a maximum-size transaction), so this is far above any real
// ApplyTxError while keeping decoded memory in the tens of megabytes.
const maxRejectionItems = 131072

// checkRejectionBudget returns an error when the first item in data holds
// more than maxRejectionItems data items or nests too deeply.
func checkRejectionBudget(data []byte) error {
	return cbor.CheckItemBudget(data, maxRejectionItems)
}

// maxFailureFields bounds the entries of one predicate failure constructor,
// which encodes as a flat list of its tag and a handful of fields. Real
// constructors carry fewer than ten.
const maxFailureFields = 16

// decodeFailureFields decodes data as the field list of a predicate failure
// constructor whose tag is already known to be recognized. The declared length
// is checked from the header before a slice is sized from it, so a peer cannot
// size an allocation by declaring many fields in a fixed-shape constructor.
func decodeFailureFields(data []byte, label string) ([]cbor.RawMessage, error) {
	fields, count, err := decodeFailureHead(data, label)
	if err != nil {
		return nil, err
	}
	if count > maxFailureFields {
		return nil, failureFieldLimitError(label, count)
	}
	return fields, nil
}

// decodeFailureHead decodes data as the field list of one predicate failure
// constructor whose tag the caller has yet to dispatch on, and returns the
// list's length. Over maxFailureFields entries only the tag and first field
// are returned, which is all any decoder reads, so nothing is sized from the
// declared length. The caller must settle that case with
// checkFailureFieldLimit, which keeps an unrecognized constructor raw as
// before and refuses a recognized one.
func decodeFailureHead(
	data []byte,
	label string,
) ([]cbor.RawMessage, int, error) {
	count, err := cbor.ListLength(data)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", label, err)
	}
	if count <= maxFailureFields {
		fields := make([]cbor.RawMessage, 0, count)
		if _, err := cbor.Decode(data, &fields); err != nil {
			return nil, 0, err
		}
		return fields, count, nil
	}
	// The full decode above also proves the entries well-formed; the walk
	// does that here without materializing them.
	if err := checkRejectionBudget(data); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", label, err)
	}
	_, headerSize, _ := cbor.ArrayInfo(data)
	rest := data[headerSize:]
	fields := make([]cbor.RawMessage, 0, 2)
	for len(fields) < 2 {
		var field cbor.RawMessage
		n, err := cbor.Decode(rest, &field)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", label, err)
		}
		fields = append(fields, field)
		rest = rest[n:]
	}
	return fields, count, nil
}

// checkFailureFieldLimit settles a constructor decoded from decodeFailureHead.
// Over maxFailureFields entries, only an unrecognized constructor, preserved
// raw in decoded, is accepted: a recognized one has a fixed shape that the
// list does not match.
func checkFailureFieldLimit(
	label string,
	count int,
	decoded error,
	err error,
) error {
	if count <= maxFailureFields {
		return err
	}
	if err == nil {
		// The constructor itself must be unrecognized; a recognized wrapper
		// whose cause is unrecognized still has a fixed shape.
		switch decoded.(type) { //nolint:errorlint // see above
		case *UnknownUtxowFailureError, *UnknownUtxoFailureError:
			return nil
		}
	}
	return failureFieldLimitError(label, count)
}

func failureFieldLimitError(label string, count int) error {
	return fmt.Errorf(
		"%s: %d fields exceeds the maximum of %d",
		label,
		count,
		maxFailureFields,
	)
}
