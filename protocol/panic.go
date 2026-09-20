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

package protocol

import "errors"

// ErrHandlerPanic matches the error a mini-protocol reports when a panic was
// raised while processing a peer's message and contained rather than allowed
// to terminate the process. Test for it with errors.Is.
//
// A mini-protocol runs its read, receive and state goroutines itself, so a
// panic on any of those paths unwinds a goroutine no consumer frame sits
// above: without containment one peer's message kills the whole process,
// including every unrelated connection. The containment sites make no
// distinction between a panic from this library's own decoding and one from a
// callback the consumer registered, because both run in the same frame and
// neither is distinguishable there. The error carries the panic value and the
// stack of the panicking goroutine, which does name the responsible frame, so
// a consumer's own programming error stays diagnosable instead of being
// silently absorbed.
//
// Containment is never a licence to carry on. A contained panic takes the
// path a decode error or a protocol violation already takes: it is reported
// on the protocol's error channel and the protocol is stopped, terminating
// the connection. The message that triggered it is never treated as handled.
var ErrHandlerPanic = errors.New("recovered panic")
