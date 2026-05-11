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

package common_test

import (
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
)

func TestValidationErrorErrorIncludesCborContext(t *testing.T) {
	e := &common.ValidationError{
		Type:        common.ValidationErrorTypeBodyHash,
		Message:     "hash mismatch",
		ByteOffset:  128,
		CborContext: "block/body/tx[3]",
		Diagnostic:  "[...]",
	}
	got := e.Error()
	assert.Contains(t, got, "body_hash")
	assert.Contains(t, got, "hash mismatch")
	assert.Contains(t, got, "block/body/tx[3]")
	assert.Contains(t, got, "@offset 128")
}

func TestValidationErrorErrorOffsetOnly(t *testing.T) {
	e := &common.ValidationError{
		Type:       common.ValidationErrorTypeTransaction,
		Message:    "decode failed",
		ByteOffset: 7,
		Cause:      errors.New("inner"),
	}
	got := e.Error()
	assert.Contains(t, got, "@offset 7")
	assert.Contains(t, got, "(inner)")
}

func TestValidationErrorErrorWithoutCborFieldsUnchanged(t *testing.T) {
	e := &common.ValidationError{
		Type:    common.ValidationErrorTypeProtocol,
		Message: "bad",
	}
	assert.Equal(t, "protocol: bad", e.Error())
}
