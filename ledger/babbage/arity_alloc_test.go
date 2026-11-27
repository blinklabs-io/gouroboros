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

package babbage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internaltest "github.com/blinklabs-io/gouroboros/internal/test"
)

// maxRejectAllocBytes is far below the megabytes needed to materialise
// internaltest.OversizedArrayItems raw items.
const maxRejectAllocBytes = 64 * 1024

func TestBabbageDatumOptionRejectsWrongArityBeforeAllocating(t *testing.T) {
	// Not t.Parallel: heap allocation is measured with a process-wide counter.
	data := internaltest.ArrayOfZeros(internaltest.OversizedArrayItems)
	var err error
	allocated := internaltest.AllocatedBytes(func() {
		err = (&BabbageTransactionOutputDatumOption{}).UnmarshalCBOR(data)
	})
	require.Error(t, err)
	assert.Less(
		t,
		allocated,
		uint64(maxRejectAllocBytes),
		"Babbage datum option allocated before checking its array length",
	)
}
