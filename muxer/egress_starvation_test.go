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

package muxer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// While a Praos segment waits, a Leios turn is never followed by another
// Leios turn, so Leios traffic cannot starve Praos traffic.
func TestEgressPraosWaitsForAtMostOneLeiosTurn(t *testing.T) {
	e := egress{busy: true, praosRun: praosBurst}
	l1, l2, p := make(chan struct{}), make(chan struct{}), make(chan struct{})
	e.leios = []chan struct{}{l1, l2}
	e.praos = []chan struct{}{p}
	e.release()
	require.True(t, isClosed(l1))
	e.release()
	require.True(t, isClosed(p), "Praos goes after one Leios turn")
	require.False(t, isClosed(l2))
}
