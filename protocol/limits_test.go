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

// Package protocol_test validates the protocol limits implementation
// as described in PROTOCOL_LIMITS.md
package protocol_test

import (
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/blinklabs-io/gouroboros/protocol/leiosfetch"
	"github.com/blinklabs-io/gouroboros/protocol/leiosnotify"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

// TestChainSyncLimitsAreDefined validates that ChainSync limits are properly defined and positive
func TestChainSyncLimitsAreDefined(t *testing.T) {
	// Test that all constants are positive
	if chainsync.MaxPipelineLimit <= 0 {
		t.Errorf(
			"MaxPipelineLimit must be positive, got %d",
			chainsync.MaxPipelineLimit,
		)
	}
	if chainsync.MaxRecvQueueSize <= 0 {
		t.Errorf(
			"MaxRecvQueueSize must be positive, got %d",
			chainsync.MaxRecvQueueSize,
		)
	}
	if chainsync.DefaultPipelineLimit <= 0 {
		t.Errorf(
			"DefaultPipelineLimit must be positive, got %d",
			chainsync.DefaultPipelineLimit,
		)
	}
	if chainsync.DefaultRecvQueueSize <= 0 {
		t.Errorf(
			"DefaultRecvQueueSize must be positive, got %d",
			chainsync.DefaultRecvQueueSize,
		)
	}
	if chainsync.MaxPendingMessageBytes <= 0 {
		t.Errorf(
			"MaxPendingMessageBytes must be positive, got %d",
			chainsync.MaxPendingMessageBytes,
		)
	}

	// Test that constants match documented values
	expectedMaxPipeline := 100
	if chainsync.MaxPipelineLimit != expectedMaxPipeline {
		t.Errorf(
			"MaxPipelineLimit should be %d, got %d",
			expectedMaxPipeline,
			chainsync.MaxPipelineLimit,
		)
	}
	expectedMaxQueue := 100
	if chainsync.MaxRecvQueueSize != expectedMaxQueue {
		t.Errorf(
			"MaxRecvQueueSize should be %d, got %d",
			expectedMaxQueue,
			chainsync.MaxRecvQueueSize,
		)
	}
	expectedMaxBytes := 102400
	if chainsync.MaxPendingMessageBytes != expectedMaxBytes {
		t.Errorf(
			"MaxPendingMessageBytes should be %d, got %d",
			expectedMaxBytes,
			chainsync.MaxPendingMessageBytes,
		)
	}

	// Test timeout constants
	if chainsync.IdleTimeout <= 0 {
		t.Errorf("IdleTimeout must be positive, got %v", chainsync.IdleTimeout)
	}
	if chainsync.CanAwaitTimeout <= 0 {
		t.Errorf(
			"CanAwaitTimeout must be positive, got %v",
			chainsync.CanAwaitTimeout,
		)
	}
	if chainsync.IntersectTimeout <= 0 {
		t.Errorf(
			"IntersectTimeout must be positive, got %v",
			chainsync.IntersectTimeout,
		)
	}
	if chainsync.MustReplyTimeout <= 0 {
		t.Errorf(
			"MustReplyTimeout must be positive, got %v",
			chainsync.MustReplyTimeout,
		)
	}
}

// TestChainSyncDefaultValuesAreReasonable ensures default values are within limits
func TestChainSyncDefaultValuesAreReasonable(t *testing.T) {
	// Defaults should be less than or equal to maximums
	if chainsync.DefaultPipelineLimit > chainsync.MaxPipelineLimit {
		t.Errorf("DefaultPipelineLimit (%d) exceeds MaxPipelineLimit (%d)",
			chainsync.DefaultPipelineLimit, chainsync.MaxPipelineLimit)
	}
	if chainsync.DefaultRecvQueueSize > chainsync.MaxRecvQueueSize {
		t.Errorf("DefaultRecvQueueSize (%d) exceeds MaxRecvQueueSize (%d)",
			chainsync.DefaultRecvQueueSize, chainsync.MaxRecvQueueSize)
	}

	// Defaults should be reasonably conservative (less than 75% of max)
	maxPipelineThreshold := int(0.75 * float64(chainsync.MaxPipelineLimit))
	if chainsync.DefaultPipelineLimit > maxPipelineThreshold {
		t.Errorf("DefaultPipelineLimit (%d) should be more conservative (< %d)",
			chainsync.DefaultPipelineLimit, maxPipelineThreshold)
	}
	maxQueueThreshold := int(0.75 * float64(chainsync.MaxRecvQueueSize))
	if chainsync.DefaultRecvQueueSize > maxQueueThreshold {
		t.Errorf("DefaultRecvQueueSize (%d) should be more conservative (< %d)",
			chainsync.DefaultRecvQueueSize, maxQueueThreshold)
	}
}

// TestChainSyncConfigurationValidation tests configuration validation and panic behavior
func TestChainSyncConfigurationValidation(t *testing.T) {
	// Test that invalid pipeline limit causes panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid pipeline limit")
		}
	}()
	chainsync.NewConfig(chainsync.WithPipelineLimit(-1))
}

func TestChainSyncConfigurationValidationExceedsMax(t *testing.T) {
	// Test that exceeding max pipeline limit causes panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for pipeline limit exceeding maximum")
		}
	}()
	chainsync.NewConfig(
		chainsync.WithPipelineLimit(chainsync.MaxPipelineLimit + 1),
	)
}

func TestChainSyncQueueConfigurationValidation(t *testing.T) {
	// Test that invalid queue size causes panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid queue size")
		}
	}()
	chainsync.NewConfig(chainsync.WithRecvQueueSize(-1))
}

func TestChainSyncQueueConfigurationValidationExceedsMax(t *testing.T) {
	// Test that exceeding max queue size causes panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for queue size exceeding maximum")
		}
	}()
	chainsync.NewConfig(
		chainsync.WithRecvQueueSize(chainsync.MaxRecvQueueSize + 1),
	)
}

// TestBlockFetchLimitsAreDefined validates that BlockFetch limits are properly defined and positive
func TestBlockFetchLimitsAreDefined(t *testing.T) {
	// Test that all constants are positive
	if blockfetch.MaxRecvQueueSize <= 0 {
		t.Errorf(
			"MaxRecvQueueSize must be positive, got %d",
			blockfetch.MaxRecvQueueSize,
		)
	}
	if blockfetch.DefaultRecvQueueSize <= 0 {
		t.Errorf(
			"DefaultRecvQueueSize must be positive, got %d",
			blockfetch.DefaultRecvQueueSize,
		)
	}
	if blockfetch.IdleBusyMaxPendingMessageBytes <= 0 {
		t.Errorf(
			"IdleBusyMaxPendingMessageBytes must be positive, got %d",
			blockfetch.IdleBusyMaxPendingMessageBytes,
		)
	}
	if blockfetch.StreamingMaxPendingMessageBytes <= 0 {
		t.Errorf(
			"StreamingMaxPendingMessageBytes must be positive, got %d",
			blockfetch.StreamingMaxPendingMessageBytes,
		)
	}

	// Test that constants match documented values
	expectedMaxQueue := 512
	if blockfetch.MaxRecvQueueSize != expectedMaxQueue {
		t.Errorf(
			"MaxRecvQueueSize should be %d, got %d",
			expectedMaxQueue,
			blockfetch.MaxRecvQueueSize,
		)
	}
	expectedMaxBytes := 2500000
	if blockfetch.StreamingMaxPendingMessageBytes != expectedMaxBytes {
		t.Errorf(
			"StreamingMaxPendingMessageBytes should be %d, got %d",
			expectedMaxBytes,
			blockfetch.StreamingMaxPendingMessageBytes,
		)
	}

	// Test timeout constants
	if blockfetch.BusyTimeout <= 0 {
		t.Errorf("BusyTimeout must be positive, got %v", blockfetch.BusyTimeout)
	}
	if blockfetch.StreamingTimeout <= 0 {
		t.Errorf(
			"StreamingTimeout must be positive, got %v",
			blockfetch.StreamingTimeout,
		)
	}
}

// TestBlockFetchDefaultValuesAreReasonable ensures default values are within limits
func TestBlockFetchDefaultValuesAreReasonable(t *testing.T) {
	// Defaults should be less than or equal to maximums
	if blockfetch.DefaultRecvQueueSize > blockfetch.MaxRecvQueueSize {
		t.Errorf("DefaultRecvQueueSize (%d) exceeds MaxRecvQueueSize (%d)",
			blockfetch.DefaultRecvQueueSize, blockfetch.MaxRecvQueueSize)
	}

	// Defaults should be reasonably conservative (less than 75% of max)
	maxQueueThreshold := int(0.75 * float64(blockfetch.MaxRecvQueueSize))
	if blockfetch.DefaultRecvQueueSize > maxQueueThreshold {
		t.Errorf("DefaultRecvQueueSize (%d) should be more conservative (< %d)",
			blockfetch.DefaultRecvQueueSize, maxQueueThreshold)
	}
}

// TestBlockFetchConfigurationValidation tests configuration validation and panic behavior
func TestBlockFetchConfigurationValidation(t *testing.T) {
	// Test that invalid queue size causes panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid queue size")
		}
	}()
	blockfetch.NewConfig(blockfetch.WithRecvQueueSize(-1))
}

func TestBlockFetchConfigurationValidationExceedsMax(t *testing.T) {
	// Test that exceeding max queue size causes panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for queue size exceeding maximum")
		}
	}()
	blockfetch.NewConfig(
		blockfetch.WithRecvQueueSize(blockfetch.MaxRecvQueueSize + 1),
	)
}

// TestTxSubmissionLimitsAreDefined validates that TxSubmission limits are properly defined and positive
func TestTxSubmissionLimitsAreDefined(t *testing.T) {
	// Test that all constants are positive
	if txsubmission.MaxRequestCount <= 0 {
		t.Errorf(
			"MaxRequestCount must be positive, got %d",
			txsubmission.MaxRequestCount,
		)
	}
	if txsubmission.MaxAckCount <= 0 {
		t.Errorf(
			"MaxAckCount must be positive, got %d",
			txsubmission.MaxAckCount,
		)
	}
	if txsubmission.DefaultRequestLimit <= 0 {
		t.Errorf(
			"DefaultRequestLimit must be positive, got %d",
			txsubmission.DefaultRequestLimit,
		)
	}
	if txsubmission.DefaultAckLimit <= 0 {
		t.Errorf(
			"DefaultAckLimit must be positive, got %d",
			txsubmission.DefaultAckLimit,
		)
	}

	// Test that constants match documented values (uint16 limit)
	expectedMax := 65535
	if txsubmission.MaxRequestCount != expectedMax {
		t.Errorf(
			"MaxRequestCount should be %d, got %d",
			expectedMax,
			txsubmission.MaxRequestCount,
		)
	}
	if txsubmission.MaxAckCount != expectedMax {
		t.Errorf(
			"MaxAckCount should be %d, got %d",
			expectedMax,
			txsubmission.MaxAckCount,
		)
	}

	// Test timeout constants
	if txsubmission.InitTimeout <= 0 {
		t.Errorf(
			"InitTimeout must be positive, got %v",
			txsubmission.InitTimeout,
		)
	}
	if txsubmission.IdleTimeout <= 0 {
		t.Errorf(
			"IdleTimeout must be positive, got %v",
			txsubmission.IdleTimeout,
		)
	}
	if txsubmission.TxIdsBlockingTimeout <= 0 {
		t.Errorf(
			"TxIdsBlockingTimeout must be positive, got %v",
			txsubmission.TxIdsBlockingTimeout,
		)
	}
	if txsubmission.TxIdsNonblockingTimeout <= 0 {
		t.Errorf(
			"TxIdsNonblockingTimeout must be positive, got %v",
			txsubmission.TxIdsNonblockingTimeout,
		)
	}
	if txsubmission.TxsTimeout <= 0 {
		t.Errorf("TxsTimeout must be positive, got %v", txsubmission.TxsTimeout)
	}
}

// TestTxSubmissionDefaultValuesAreReasonable ensures default values are within limits
func TestTxSubmissionDefaultValuesAreReasonable(t *testing.T) {
	// Defaults should be less than or equal to maximums
	if txsubmission.DefaultRequestLimit > txsubmission.MaxRequestCount {
		t.Errorf("DefaultRequestLimit (%d) exceeds MaxRequestCount (%d)",
			txsubmission.DefaultRequestLimit, txsubmission.MaxRequestCount)
	}
	if txsubmission.DefaultAckLimit > txsubmission.MaxAckCount {
		t.Errorf("DefaultAckLimit (%d) exceeds MaxAckCount (%d)",
			txsubmission.DefaultAckLimit, txsubmission.MaxAckCount)
	}

	// Defaults should be reasonably small for transaction handling
	reasonableLimit := 10000
	if txsubmission.DefaultRequestLimit > reasonableLimit {
		t.Errorf("DefaultRequestLimit (%d) should be reasonable (< %d)",
			txsubmission.DefaultRequestLimit, reasonableLimit)
	}
	if txsubmission.DefaultAckLimit > reasonableLimit {
		t.Errorf("DefaultAckLimit (%d) should be reasonable (< %d)",
			txsubmission.DefaultAckLimit, reasonableLimit)
	}
}

// TestProtocolViolationErrorsAreDefined verifies that protocol violation errors are defined
func TestProtocolViolationErrorsAreDefined(t *testing.T) {
	// Test that all protocol violation errors are defined and not nil
	if protocol.ErrProtocolViolationQueueExceeded == nil {
		t.Error("ErrProtocolViolationQueueExceeded should be defined")
	}
	if protocol.ErrProtocolViolationPipelineExceeded == nil {
		t.Error("ErrProtocolViolationPipelineExceeded should be defined")
	}
	if protocol.ErrProtocolViolationRequestExceeded == nil {
		t.Error("ErrProtocolViolationRequestExceeded should be defined")
	}
	if protocol.ErrProtocolViolationInvalidMessage == nil {
		t.Error("ErrProtocolViolationInvalidMessage should be defined")
	}

	// Test that error messages are meaningful
	errors := []error{
		protocol.ErrProtocolViolationQueueExceeded,
		protocol.ErrProtocolViolationPipelineExceeded,
		protocol.ErrProtocolViolationRequestExceeded,
		protocol.ErrProtocolViolationInvalidMessage,
	}

	for _, err := range errors {
		if err.Error() == "" {
			t.Errorf(
				"Protocol violation error should have meaningful message, got empty string",
			)
		}
		if len(err.Error()) < 10 {
			t.Errorf(
				"Protocol violation error message should be descriptive, got: %s",
				err.Error(),
			)
		}
	}
}

// TestProtocolViolationErrorMessages verifies error message content
func TestProtocolViolationErrorMessages(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		contains string
	}{
		{
			name:     "Queue exceeded error",
			err:      protocol.ErrProtocolViolationQueueExceeded,
			contains: "queue",
		},
		{
			name:     "Pipeline exceeded error",
			err:      protocol.ErrProtocolViolationPipelineExceeded,
			contains: "pipeline",
		},
		{
			name:     "Request exceeded error",
			err:      protocol.ErrProtocolViolationRequestExceeded,
			contains: "request",
		},
		{
			name:     "Invalid message error",
			err:      protocol.ErrProtocolViolationInvalidMessage,
			contains: "message",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errMsg := tc.err.Error()
			if errMsg == "" {
				t.Fatalf("Error message should not be empty")
			}
			if !strings.Contains(
				strings.ToLower(errMsg),
				strings.ToLower(tc.contains),
			) {
				t.Errorf(
					"Error message %q should contain %q",
					errMsg,
					tc.contains,
				)
			}
		})
	}
}

// TestStateMapEntryHasPendingMessageByteLimit verifies that StateMapEntry includes the PendingMessageByteLimit field
func TestStateMapEntryHasPendingMessageByteLimit(t *testing.T) {
	// Create a StateMapEntry with a pending message byte limit
	entry := protocol.StateMapEntry{
		Agency:                  protocol.AgencyClient,
		Transitions:             []protocol.StateTransition{},
		Timeout:                 0,
		PendingMessageByteLimit: 1000,
	}

	// Verify the field is set correctly
	if entry.PendingMessageByteLimit != 1000 {
		t.Errorf(
			"PendingMessageByteLimit should be 1000, got %d",
			entry.PendingMessageByteLimit,
		)
	}

	// Test zero value (no limit)
	entryZero := protocol.StateMapEntry{
		Agency:                  protocol.AgencyClient,
		Transitions:             []protocol.StateTransition{},
		Timeout:                 0,
		PendingMessageByteLimit: 0,
	}

	if entryZero.PendingMessageByteLimit != 0 {
		t.Errorf(
			"PendingMessageByteLimit should be 0, got %d",
			entryZero.PendingMessageByteLimit,
		)
	}
}

// TestProtocolStateTimeouts verifies that all protocol state timeout constants are properly defined
func TestProtocolStateTimeouts(t *testing.T) {
	t.Run("ChainSync timeouts", func(t *testing.T) {
		// Test that all timeout constants are positive
		if chainsync.IdleTimeout <= 0 {
			t.Errorf(
				"IdleTimeout must be positive, got %v",
				chainsync.IdleTimeout,
			)
		}
		if chainsync.CanAwaitTimeout <= 0 {
			t.Errorf(
				"CanAwaitTimeout must be positive, got %v",
				chainsync.CanAwaitTimeout,
			)
		}
		if chainsync.IntersectTimeout <= 0 {
			t.Errorf(
				"IntersectTimeout must be positive, got %v",
				chainsync.IntersectTimeout,
			)
		}
		if chainsync.MustReplyTimeout <= 0 {
			t.Errorf(
				"MustReplyTimeout must be positive, got %v",
				chainsync.MustReplyTimeout,
			)
		}

		// Test that timeout values are reasonable (between 1 second and 1 hour)
		timeouts := map[string]time.Duration{
			"IdleTimeout":      chainsync.IdleTimeout,
			"CanAwaitTimeout":  chainsync.CanAwaitTimeout,
			"IntersectTimeout": chainsync.IntersectTimeout,
			"MustReplyTimeout": chainsync.MustReplyTimeout,
		}
		for name, timeout := range timeouts {
			if timeout < time.Second || timeout > time.Hour {
				t.Errorf(
					"%s should be between 1 second and 1 hour, got %v",
					name,
					timeout,
				)
			}
		}
	})

	t.Run("BlockFetch timeouts", func(t *testing.T) {
		// Test that all timeout constants are positive
		if blockfetch.BusyTimeout <= 0 {
			t.Errorf(
				"BusyTimeout must be positive, got %v",
				blockfetch.BusyTimeout,
			)
		}
		if blockfetch.StreamingTimeout <= 0 {
			t.Errorf(
				"StreamingTimeout must be positive, got %v",
				blockfetch.StreamingTimeout,
			)
		}

		// Test that timeout values are reasonable
		timeouts := map[string]time.Duration{
			"BusyTimeout":      blockfetch.BusyTimeout,
			"StreamingTimeout": blockfetch.StreamingTimeout,
		}
		for name, timeout := range timeouts {
			if timeout < time.Second || timeout > time.Hour {
				t.Errorf(
					"%s should be between 1 second and 1 hour, got %v",
					name,
					timeout,
				)
			}
		}
	})

	t.Run("TxSubmission timeouts", func(t *testing.T) {
		// Test that all timeout constants are positive
		if txsubmission.InitTimeout <= 0 {
			t.Errorf(
				"InitTimeout must be positive, got %v",
				txsubmission.InitTimeout,
			)
		}
		if txsubmission.IdleTimeout <= 0 {
			t.Errorf(
				"IdleTimeout must be positive, got %v",
				txsubmission.IdleTimeout,
			)
		}
		if txsubmission.TxIdsBlockingTimeout <= 0 {
			t.Errorf(
				"TxIdsBlockingTimeout must be positive, got %v",
				txsubmission.TxIdsBlockingTimeout,
			)
		}
		if txsubmission.TxIdsNonblockingTimeout <= 0 {
			t.Errorf(
				"TxIdsNonblockingTimeout must be positive, got %v",
				txsubmission.TxIdsNonblockingTimeout,
			)
		}
		if txsubmission.TxsTimeout <= 0 {
			t.Errorf(
				"TxsTimeout must be positive, got %v",
				txsubmission.TxsTimeout,
			)
		}

		// Test that timeout values are reasonable
		timeouts := map[string]time.Duration{
			"InitTimeout":             txsubmission.InitTimeout,
			"IdleTimeout":             txsubmission.IdleTimeout,
			"TxIdsBlockingTimeout":    txsubmission.TxIdsBlockingTimeout,
			"TxIdsNonblockingTimeout": txsubmission.TxIdsNonblockingTimeout,
			"TxsTimeout":              txsubmission.TxsTimeout,
		}
		for name, timeout := range timeouts {
			if timeout < time.Second || timeout > time.Hour {
				t.Errorf(
					"%s should be between 1 second and 1 hour, got %v",
					name,
					timeout,
				)
			}
		}
	})

	t.Run("Handshake timeouts", func(t *testing.T) {
		// Test that all timeout constants are positive
		if handshake.ProposeTimeout <= 0 {
			t.Errorf(
				"ProposeTimeout must be positive, got %v",
				handshake.ProposeTimeout,
			)
		}
		if handshake.ConfirmTimeout <= 0 {
			t.Errorf(
				"ConfirmTimeout must be positive, got %v",
				handshake.ConfirmTimeout,
			)
		}

		// Test that timeout values are reasonable
		timeouts := map[string]time.Duration{
			"ProposeTimeout": handshake.ProposeTimeout,
			"ConfirmTimeout": handshake.ConfirmTimeout,
		}
		for name, timeout := range timeouts {
			if timeout < time.Second || timeout > time.Minute {
				t.Errorf(
					"%s should be between 1 second and 1 minute, got %v",
					name,
					timeout,
				)
			}
		}
	})

	t.Run("Keepalive timeouts", func(t *testing.T) {
		// Test that all timeout constants are positive
		if keepalive.ClientTimeout <= 0 {
			t.Errorf(
				"ClientTimeout must be positive, got %v",
				keepalive.ClientTimeout,
			)
		}
		if keepalive.ServerTimeout <= 0 {
			t.Errorf(
				"ServerTimeout must be positive, got %v",
				keepalive.ServerTimeout,
			)
		}

		// Test that timeout values are reasonable
		timeouts := map[string]time.Duration{
			"ClientTimeout": keepalive.ClientTimeout,
			"ServerTimeout": keepalive.ServerTimeout,
		}
		for name, timeout := range timeouts {
			if timeout < time.Second || timeout > time.Hour {
				t.Errorf(
					"%s should be between 1 second and 1 hour, got %v",
					name,
					timeout,
				)
			}
		}
	})

	t.Run("LocalStateQuery timeouts", func(t *testing.T) {
		cfg := localstatequery.NewConfig()
		timeouts := map[string]time.Duration{
			"AcquireTimeout": cfg.AcquireTimeout,
			"QueryTimeout":   cfg.QueryTimeout,
		}
		for name, timeout := range timeouts {
			if timeout <= 0 {
				t.Errorf("%s must be positive, got %v", name, timeout)
			}
			if timeout < time.Second || timeout > time.Hour {
				t.Errorf(
					"%s should be between 1 second and 1 hour, got %v",
					name,
					timeout,
				)
			}
		}
	})

	t.Run("LocalTxMonitor timeouts", func(t *testing.T) {
		cfg := localtxmonitor.NewConfig()
		timeouts := map[string]time.Duration{
			"AcquireTimeout": cfg.AcquireTimeout,
			"QueryTimeout":   cfg.QueryTimeout,
		}
		for name, timeout := range timeouts {
			if timeout <= 0 {
				t.Errorf("%s must be positive, got %v", name, timeout)
			}
			if timeout < time.Second || timeout > time.Hour {
				t.Errorf(
					"%s should be between 1 second and 1 hour, got %v",
					name,
					timeout,
				)
			}
		}
	})

	t.Run("LocalTxSubmission timeouts", func(t *testing.T) {
		cfg := localtxsubmission.NewConfig()
		if cfg.Timeout <= 0 {
			t.Errorf("Timeout must be positive, got %v", cfg.Timeout)
		}
		if cfg.Timeout < time.Second || cfg.Timeout > time.Hour {
			t.Errorf(
				"Timeout should be between 1 second and 1 hour, got %v",
				cfg.Timeout,
			)
		}
	})

	t.Run("PeerSharing timeouts", func(t *testing.T) {
		cfg := peersharing.NewConfig()
		if cfg.Timeout <= 0 {
			t.Errorf("Timeout must be positive, got %v", cfg.Timeout)
		}
		if cfg.Timeout < time.Second || cfg.Timeout > time.Hour {
			t.Errorf(
				"Timeout should be between 1 second and 1 hour, got %v",
				cfg.Timeout,
			)
		}
	})

	t.Run("LeiosFetch timeouts", func(t *testing.T) {
		cfg := leiosfetch.NewConfig()
		if cfg.Timeout <= 0 {
			t.Errorf("Timeout must be positive, got %v", cfg.Timeout)
		}
		if cfg.Timeout < time.Second || cfg.Timeout > time.Hour {
			t.Errorf(
				"Timeout should be between 1 second and 1 hour, got %v",
				cfg.Timeout,
			)
		}
	})

	t.Run("LeiosNotify timeouts", func(t *testing.T) {
		cfg := leiosnotify.NewConfig()
		if cfg.Timeout <= 0 {
			t.Errorf("Timeout must be positive, got %v", cfg.Timeout)
		}
		if cfg.Timeout < time.Second || cfg.Timeout > time.Hour {
			t.Errorf(
				"Timeout should be between 1 second and 1 hour, got %v",
				cfg.Timeout,
			)
		}
	})
}

// TestStateMapTimeouts verifies that StateMap entries use the correct timeout constants
func TestStateMapTimeouts(t *testing.T) {
	t.Run("ChainSync StateMap timeouts", func(t *testing.T) {
		stateIdle := protocol.NewState(1, "Idle")
		stateCanAwait := protocol.NewState(2, "CanAwait")
		stateIntersect := protocol.NewState(4, "Intersect")
		stateMustReply := protocol.NewState(3, "MustReply")

		if entry, exists := chainsync.StateMap[stateIdle]; exists {
			if entry.Timeout != chainsync.IdleTimeout {
				t.Errorf(
					"stateIdle timeout should be %v, got %v",
					chainsync.IdleTimeout,
					entry.Timeout,
				)
			}
		}
		if entry, exists := chainsync.StateMap[stateCanAwait]; exists {
			if entry.Timeout != chainsync.CanAwaitTimeout {
				t.Errorf(
					"stateCanAwait timeout should be %v, got %v",
					chainsync.CanAwaitTimeout,
					entry.Timeout,
				)
			}
		}
		if entry, exists := chainsync.StateMap[stateIntersect]; exists {
			if entry.Timeout != chainsync.IntersectTimeout {
				t.Errorf(
					"stateIntersect timeout should be %v, got %v",
					chainsync.IntersectTimeout,
					entry.Timeout,
				)
			}
		}
		if entry, exists := chainsync.StateMap[stateMustReply]; exists {
			if entry.Timeout != chainsync.MustReplyTimeout {
				t.Errorf(
					"stateMustReply timeout should be %v, got %v",
					chainsync.MustReplyTimeout,
					entry.Timeout,
				)
			}
		}
	})

	t.Run("BlockFetch StateMap timeouts", func(t *testing.T) {
		stateBusy := protocol.NewState(2, "Busy")
		stateStreaming := protocol.NewState(3, "Streaming")

		if entry, exists := blockfetch.StateMap[stateBusy]; exists {
			if entry.Timeout != blockfetch.BusyTimeout {
				t.Errorf(
					"StateBusy timeout should be %v, got %v",
					blockfetch.BusyTimeout,
					entry.Timeout,
				)
			}
		}
		if entry, exists := blockfetch.StateMap[stateStreaming]; exists {
			if entry.Timeout != blockfetch.StreamingTimeout {
				t.Errorf(
					"StateStreaming timeout should be %v, got %v",
					blockfetch.StreamingTimeout,
					entry.Timeout,
				)
			}
		}
	})
}
