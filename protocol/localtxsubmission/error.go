// Copyright 2023 Blink Labs Software
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
	"fmt"
)

// TransactionRejectedError represents an explicit transaction rejection
type TransactionRejectedError struct {
	ReasonCbor []byte
	Reason     error
}

func (e TransactionRejectedError) Error() string {
	if e.Reason != nil {
		return e.Reason.Error()
	} else {
		return fmt.Sprintf("transaction rejected: CBOR reason hex: %x", e.ReasonCbor)
	}
}
