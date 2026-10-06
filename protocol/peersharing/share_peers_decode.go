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

package peersharing

import (
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

// UnmarshalCBOR checks the address count before the generic decoder allocates
// the typed slice. A request count is uint8, so a compact response may legally
// contain 255 addresses; the server's worst-case emission cap is separate.
func (m *MsgSharePeers) UnmarshalCBOR(data []byte) error {
	if err := cborpreflight.ValidateSecondFieldArray(
		data,
		MaxPeerSharingResponseCount,
		"peer-sharing address array",
		nil,
	); err != nil {
		return err
	}
	type message MsgSharePeers
	var decoded message
	if _, err := cbor.Decode(data, &decoded); err != nil {
		return err
	}
	*m = MsgSharePeers(decoded)
	return nil
}
