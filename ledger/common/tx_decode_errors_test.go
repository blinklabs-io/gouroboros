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

func TestTxComponentString(t *testing.T) {
	assert.Equal(t, "body", common.TxComponentBody.String())
	assert.Equal(t, "witness", common.TxComponentWitness.String())
	assert.Equal(t, "metadata", common.TxComponentMetadata.String())
	assert.Equal(t, "unknown(99)", common.TxComponent(99).String())
}

func TestTxFieldName(t *testing.T) {
	assert.Equal(t, "inputs", common.TxFieldName(0))
	assert.Equal(t, "outputs", common.TxFieldName(1))
	assert.Equal(t, "fee", common.TxFieldName(2))
	assert.Equal(t, "voting_procedures", common.TxFieldName(19))
	assert.Equal(t, "", common.TxFieldName(123))
}

func TestTxDecodeErrorErrorFull(t *testing.T) {
	inner := errors.New("invalid array length")
	e := common.NewTxDecodeError(inner, 3, common.TxComponentBody, 0, 42, "")
	got := e.Error()
	assert.Contains(t, got, "[tx 3]")
	assert.Contains(t, got, "in body")
	assert.Contains(t, got, "field 0 (inputs)")
	assert.Contains(t, got, "at offset 42")
	assert.Contains(t, got, "invalid array length")
}

func TestTxDecodeErrorErrorPartial(t *testing.T) {
	e := common.NewTxDecodeError(
		errors.New("oops"),
		-1,
		common.TxComponentWitness,
		-1,
		0,
		"",
	)
	got := e.Error()
	assert.NotContains(t, got, "[tx ")
	assert.Contains(t, got, "in witness")
	assert.NotContains(t, got, "field ")
	assert.NotContains(t, got, "at offset")
	assert.Contains(t, got, "oops")
}

func TestTxDecodeErrorWitnessFieldDoesNotUseBodyName(t *testing.T) {
	// Field 0 in a witness map is "vkey witnesses" — it must NOT be labeled
	// "inputs" (which is field 0 of the body map). The current implementation
	// only resolves names when Component == TxComponentBody.
	e := common.NewTxDecodeError(
		errors.New("bad"),
		0,
		common.TxComponentWitness,
		0,
		0,
		"",
	)
	got := e.Error()
	assert.NotContains(t, got, "(inputs)")
	assert.Contains(t, got, "field 0")
}

func TestTxDecodeErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	e := common.NewTxDecodeError(inner, 0, common.TxComponentBody, 0, 0, "")
	assert.Same(t, inner, errors.Unwrap(e))
	assert.True(t, errors.Is(e, inner))
}
