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
	"errors"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// DecodeProtocolParameterUpdateProposals decodes a pre-Conway transaction
// protocol parameter update, an array of a map from genesis delegate hash to
// update and an epoch. It reads the map pair by pair, so decoding stops at the
// first invalid update and memory grows only with updates that decode, never
// with the pair count the input declares.
func DecodeProtocolParameterUpdateProposals[U any](
	data []byte,
) (map[Blake2b224]U, uint64, error) {
	count, headerSize, indefinite := cbor.ArrayInfo(data)
	if count != 2 || indefinite {
		return decodeProposalsGeneric[U](data)
	}
	updates, mapSize, err := cbor.DecodeMapIncremental[Blake2b224, U](
		data[headerSize:],
		nil,
	)
	if err != nil {
		return nil, 0, err
	}
	var epoch uint64
	epochSize, err := cbor.Decode(data[int(headerSize)+mapSize:], &epoch)
	if err != nil {
		return nil, 0, err
	}
	if int(headerSize)+mapSize+epochSize != len(data) {
		return nil, 0, errors.New(
			"protocol parameter update has trailing data",
		)
	}
	return updates, epoch, nil
}

// decodeProposalsGeneric decodes encodings the fast path does not read in
// place, such as an indefinite-length array or the wrong item count. It copies
// the map bytes but still reads the map pair by pair.
func decodeProposalsGeneric[U any](
	data []byte,
) (map[Blake2b224]U, uint64, error) {
	var tmp struct {
		cbor.StructAsArray
		Updates cbor.RawMessage
		Epoch   uint64
	}
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return nil, 0, err
	}
	updates, _, err := cbor.DecodeMapIncremental[Blake2b224, U](
		tmp.Updates,
		nil,
	)
	if err != nil {
		return nil, 0, err
	}
	return updates, tmp.Epoch, nil
}
