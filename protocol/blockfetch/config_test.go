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

package blockfetch_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/stretchr/testify/require"
)

func TestNewConfigReturnsErrorForNegativeRecvQueueSize(t *testing.T) {
	_, err := blockfetch.NewConfig(blockfetch.WithRecvQueueSize(-1))
	require.Error(t, err, "NewConfig should return an error for negative RecvQueueSize")
}

func TestNewConfigReturnsErrorForExceedingMaxRecvQueueSize(t *testing.T) {
	_, err := blockfetch.NewConfig(
		blockfetch.WithRecvQueueSize(blockfetch.MaxRecvQueueSize + 1),
	)
	require.Error(t, err, "NewConfig should return an error when RecvQueueSize exceeds maximum")
}

func TestNewConfigSucceedsWithValidOptions(t *testing.T) {
	cfg, err := blockfetch.NewConfig(
		blockfetch.WithRecvQueueSize(blockfetch.DefaultRecvQueueSize),
	)
	require.NoError(t, err)
	require.Equal(t, blockfetch.DefaultRecvQueueSize, cfg.RecvQueueSize)
}

func TestNewConfigSucceedsWithDefaults(t *testing.T) {
	cfg, err := blockfetch.NewConfig()
	require.NoError(t, err)
	require.Equal(t, blockfetch.DefaultRecvQueueSize, cfg.RecvQueueSize)
}
