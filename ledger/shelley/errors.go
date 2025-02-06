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

package shelley

import "fmt"

type ExpiredUtxoError struct {
	Ttl  uint64
	Slot uint64
}

func (e ExpiredUtxoError) Error() string {
	return fmt.Sprintf(
		"expired UTxO: TTL %d, slot %d",
		e.Ttl,
		e.Slot,
	)
}
