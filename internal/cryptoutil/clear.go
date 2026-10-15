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

package cryptoutil

import (
	"runtime"

	"filippo.io/edwards25519"
)

// Wipe overwrites b and keeps its backing array live through the write.
//
// Go does not guarantee complete secret erasure because the runtime may have
// copied the data. Wipe clears the supplied backing array on a best-effort
// basis.
//
//go:noinline
func Wipe(b []byte) {
	clear(b)
	runtime.KeepAlive(b)
}

// WipeScalar overwrites s and keeps it live through the write.
//
//go:noinline
func WipeScalar(s *edwards25519.Scalar) {
	*s = edwards25519.Scalar{}
	runtime.KeepAlive(s)
}
