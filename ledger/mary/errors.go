// Copyright 2025 Blink Labs Software
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

package mary

import (
	"fmt"
	"strings"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type OutputTooBigUtxoError struct {
	Outputs []common.TransactionOutput
}

func (e OutputTooBigUtxoError) Error() string {
	tmpOutputs := make([]string, len(e.Outputs))
	for idx, tmpOutput := range e.Outputs {
		tmpOutputs[idx] = fmt.Sprintf("%#v", tmpOutput)
	}
	return "output value too large: " + strings.Join(tmpOutputs, ", ")
}
