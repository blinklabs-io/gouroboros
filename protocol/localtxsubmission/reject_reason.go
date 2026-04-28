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

package localtxsubmission

import (
	"errors"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// CborRejectReason marks errors whose canonical wire form is a
// structured CBOR value rather than the error's text representation.
//
// SubmitTxFunc callbacks may return values implementing this interface
// when the rejection reason has a well-defined wire format on a real
// cardano-node (e.g. *ledger.EraMismatch for HardForkApplyTxErrWrongEra).
// The server places MarshalCBOR's output verbatim in MsgRejectTx.Reason;
// errors that do not implement this interface are encoded as a CBOR
// text string of err.Error(), preserving the pre-typed behaviour for
// callers that haven't migrated to typed reject reasons yet.
//
// Implementations must produce the exact wire bytes a Haskell node
// would emit for the same condition — see the ledger package's
// EraMismatch for the reference implementation.
type CborRejectReason interface {
	error
	MarshalCBOR() ([]byte, error)
}

// encodeRejectReason returns the bytes to place in MsgRejectTx.Reason
// for the given submit-tx error. errors.As walks the wrapping chain so
// callers can fmt.Errorf("...: %w", typedErr) without losing the typed
// encoding.
func encodeRejectReason(err error) ([]byte, error) {
	var typed CborRejectReason
	if errors.As(err, &typed) {
		return typed.MarshalCBOR()
	}
	return cbor.Encode(err.Error())
}
