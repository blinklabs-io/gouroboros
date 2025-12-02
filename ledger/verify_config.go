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

package ledger

import "sync"

// VerifyConfig holds runtime verification toggles.
// Default values favor safety; tests or specific flows can opt out.
type VerifyConfig struct {
	// SkipBodyHashValidation disables body hash verification in VerifyBlock().
	// Useful for scenarios where full block CBOR is unavailable.
	SkipBodyHashValidation bool
}

var (
	verifyConfig   VerifyConfig
	verifyConfigMu sync.RWMutex
)

// SetVerifyConfig sets global verification configuration.
// Safe for concurrent use.
func SetVerifyConfig(cfg VerifyConfig) {
	verifyConfigMu.Lock()
	defer verifyConfigMu.Unlock()
	verifyConfig = cfg
}

// GetVerifyConfig returns current verification configuration.
// Safe for concurrent use.
func GetVerifyConfig() VerifyConfig {
	verifyConfigMu.RLock()
	defer verifyConfigMu.RUnlock()
	return verifyConfig
}
