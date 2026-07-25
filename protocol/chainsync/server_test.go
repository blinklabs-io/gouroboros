// Copyright 2024 Blink Labs Software
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

package chainsync

import (
	"bytes"
	"log/slog"
	"testing"

	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRequestNext_Callback(t *testing.T) {
	called := false
	server := &Server{
		config: &Config{
			RequestNextFunc: func(ctx CallbackContext) error {
				called = true
				return nil
			},
		},
		callbackContext: CallbackContext{},
	}

	err := server.handleRequestNext()

	assert.NoError(t, err, "expected no error")
	assert.True(t, called, "expected RequestNextFunc to be called")
}

func TestHandleRequestNext_NilCallback(t *testing.T) {
	server := &Server{
		config: &Config{
			RequestNextFunc: nil,
		},
		callbackContext: CallbackContext{},
	}

	err := server.handleRequestNext()
	expectedError := "received chain-sync RequestNext message but no callback function is defined"

	assert.Error(t, err, "expected an error due to nil callback")
	assert.EqualError(t, err, expectedError)
}

func TestLogRollForwardDoesNotLogBlockData(t *testing.T) {
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewTextHandler(
		&logOutput,
		&slog.HandlerOptions{Level: slog.LevelDebug},
	))
	tip := Tip{
		Point: pcommon.Point{
			Slot: 42,
			Hash: []byte{0xca, 0xfe},
		},
		BlockNumber: 24,
	}

	logRollForward(
		logger,
		5,
		[]byte{0xde, 0xad, 0xbe, 0xef},
		tip,
		"connection-id",
	)

	output := logOutput.String()
	require.NotEmpty(t, output)
	assert.Contains(t, output, "msg=\"calling RollForward\"")
	assert.Contains(t, output, "block_type=5")
	assert.Contains(t, output, "block_size=4")
	assert.Contains(t, output, "tip_slot=42")
	assert.Contains(t, output, "tip_block_number=24")
	assert.NotContains(
		t,
		output,
		"deadbeef",
		"serialized block data should not be logged",
	)
}
