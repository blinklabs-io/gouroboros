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
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

func (t *TxId) UnmarshalCBOR(cborData []byte) error {
	if t == nil {
		return errors.New("nil TxId receiver")
	}
	var decoded struct {
		cbor.StructAsArray
		EraId uint16
		TxId  []byte
	}
	if _, err := cbor.Decode(cborData, &decoded); err != nil {
		return fmt.Errorf("decode transaction ID: %w", err)
	}
	if len(decoded.TxId) != len(t.TxId) {
		return fmt.Errorf(
			"invalid transaction ID length: expected %d bytes, got %d",
			len(t.TxId),
			len(decoded.TxId),
		)
	}
	t.EraId = decoded.EraId
	copy(t.TxId[:], decoded.TxId)
	return nil
}
