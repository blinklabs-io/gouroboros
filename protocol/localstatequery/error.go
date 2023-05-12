// Copyright 2023 Blink Labs, LLC.
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

package localstatequery

// AcquireFailurePointTooOldError indicates a failure to acquire a point due to it being too old
type AcquireFailurePointTooOldError struct {
}

func (e AcquireFailurePointTooOldError) Error() string {
	return "acquire failure: point too old"
}

// AcquireFailurePointNotOnChainError indicates a failure to acquire a point due to it not being present on the chain
type AcquireFailurePointNotOnChainError struct {
}

func (e AcquireFailurePointNotOnChainError) Error() string {
	return "acquire failure: point not on chain"
}
