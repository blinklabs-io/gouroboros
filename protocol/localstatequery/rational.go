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

package localstatequery

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// decodeRational decodes a Haskell Rational, written as [numerator,
// denominator] with or without the tag 30 that cardano-ledger adds from
// protocol version 9. cardano-binary, which ouroboros-network uses, never
// adds the tag.
func decodeRational(data []byte) (*big.Rat, error) {
	if len(data) >= 2 && data[0] == 0xd8 && data[1] == cbor.CborTagRational {
		data = data[2:]
	}
	var parts []cbor.RawMessage
	if _, err := cbor.Decode(data, &parts); err != nil {
		return nil, fmt.Errorf("decoding rational: %w", err)
	}
	if len(parts) != 2 {
		return nil, fmt.Errorf(
			"decoding rational: expected 2 elements, got %d",
			len(parts),
		)
	}
	var num, denom big.Int
	if _, err := cbor.Decode(parts[0], &num); err != nil {
		return nil, fmt.Errorf("decoding rational numerator: %w", err)
	}
	if _, err := cbor.Decode(parts[1], &denom); err != nil {
		return nil, fmt.Errorf("decoding rational denominator: %w", err)
	}
	if denom.Sign() <= 0 {
		return nil, errors.New(
			"decoding rational: denominator must be positive",
		)
	}
	return new(big.Rat).SetFrac(&num, &denom), nil
}
