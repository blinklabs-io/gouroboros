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
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultConfigHasNoBusyTimeout pins the specification's "No timeouts"
// for local-tx-submission. A node holds MsgSubmitTx until its mempool has
// room for the transaction, for as long as the mempool stays full, so a
// default timer would tear down the whole node-to-client connection while
// the node is working as intended. A caller may still opt in with
// WithTimeout.
func TestDefaultConfigHasNoBusyTimeout(t *testing.T) {
	t.Parallel()
	require.Zero(t, NewConfig().Timeout)
	require.Zero(t, StateMap[stateBusy].Timeout)
}
