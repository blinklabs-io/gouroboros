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

package leiosfetch

import "errors"

// ErrBlockNotFound signals that a requested endorser block is not available.
// A BlockRequestFunc callback returns it (directly or wrapped) to make the
// server respond with MsgNoBlock instead of tearing down the connection; the
// client's BlockRequest returns it to the caller when the server sends
// MsgNoBlock. Callers can test for it with errors.Is.
var ErrBlockNotFound = errors.New(
	"leios-fetch: endorser block not available",
)

// ErrBlockTxsNotFound signals that the requested endorser block transactions
// are not available. A BlockTxsRequestFunc callback returns it (directly or
// wrapped) to make the server respond with MsgNoBlockTxs instead of tearing
// down the connection; the client's BlockTxsRequest returns it to the caller
// when the server sends MsgNoBlockTxs. Callers can test for it with errors.Is.
var ErrBlockTxsNotFound = errors.New(
	"leios-fetch: endorser block transactions not available",
)
