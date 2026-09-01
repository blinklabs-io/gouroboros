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

import "encoding/binary"

// OpCertSignableBytes returns the bytes the pool cold key signs for an
// operational certificate — the cardano-ledger OCertSignable representation.
//
// It is the raw concatenation of the KES (hot) verification key, the issue
// counter as a big-endian uint64, and the KES period as a big-endian uint64.
// This is NOT a CBOR encoding. It matches cardano-node / cardano-ledger
// (Cardano.Protocol.TPraos.OCert, OCertSignable.getSignableRepresentation) and
// is verified byte-for-byte against a real cardano-cli operational certificate
// in the tests. Signing or verifying over any other encoding (e.g. a CBOR
// array) rejects real blocks.
func OpCertSignableBytes(
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
