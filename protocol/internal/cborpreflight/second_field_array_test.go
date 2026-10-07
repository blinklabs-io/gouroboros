// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cborpreflight

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestValidateSecondFieldArrayRejectsDeepItemIteratively(t *testing.T) {
	item := append(bytes.Repeat([]byte{0x81}, cbor.MaxNestedLevels+1), 0)
	wire := append([]byte{0x82, 0, 0x81}, item...)
	err := ValidateSecondFieldArray(wire, 1, "test collection", nil)
	require.ErrorContains(t, err, "nesting exceeds maximum depth")
}

func TestValidateItemDepthUsesWireShapeLimit(t *testing.T) {
	require.NoError(t, ValidateItemDepth([]byte{0x81, 0}, 1, "item"))
	require.ErrorContains(
		t,
		ValidateItemDepth([]byte{0x81, 0x81, 0}, 1, "item"),
		"nesting exceeds maximum depth 1",
	)
}
