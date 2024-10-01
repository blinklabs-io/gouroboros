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
	"testing"

	"github.com/stretchr/testify/assert"
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
