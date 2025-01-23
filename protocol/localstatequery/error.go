// Copyright 2023 Blink Labs Software
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

import "errors"

// ErrAcquireFailurePointTooOld indicates a failure to acquire a point due to it being too old
var ErrAcquireFailurePointTooOld = errors.New("acquire failure: point too old")

// ErrAcquireFailurePointNotOnChain indicates a failure to acquire a point due to it not being present on the chain
var ErrAcquireFailurePointNotOnChain = errors.New(
	"acquire failure: point not on chain",
)
