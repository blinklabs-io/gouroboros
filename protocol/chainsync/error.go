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

package chainsync

import "errors"

var ErrIntersectNotFound = errors.New("chain intersection not found")

// StopChainSync is used as a special return value from a RollForward or RollBackward handler function
// to signify that the sync process should be stopped
var ErrStopSyncProcess = errors.New("stop sync process")
