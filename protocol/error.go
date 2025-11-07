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

package protocol

import "errors"

var ErrProtocolShuttingDown = errors.New("protocol is shutting down")

// Protocol violation errors cause connection termination per Ouroboros Network Spec
var (
	ErrProtocolViolationQueueExceeded = errors.New(
		"protocol violation: message queue limit exceeded",
	)
	ErrProtocolViolationPipelineExceeded = errors.New(
		"protocol violation: pipeline limit exceeded",
	)
	ErrProtocolViolationRequestExceeded = errors.New(
		"protocol violation: request count limit exceeded",
	)
	ErrProtocolViolationInvalidMessage = errors.New(
		"protocol violation: invalid message received",
	)
)
