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

package conway

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestIsInConwayBootstrapPhase(t *testing.T) {
	tests := []struct {
		name string
		pp   common.ProtocolParameters
		want bool
	}{
		{
			name: "PV8 (Babbage) wrapped in Conway pparams returns false",
			pp:   &ConwayProtocolParameters{ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 8}},
			want: false,
		},
		{
			name: "PV9 (bootstrap) returns true",
			pp:   &ConwayProtocolParameters{ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 9}},
			want: true,
		},
		{
			name: "PV10 (Plomin) returns false",
			pp:   &ConwayProtocolParameters{ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 10}},
			want: false,
		},
		{
			name: "PV11 (VanRossem) returns false",
			pp:   &ConwayProtocolParameters{ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 11}},
			want: false,
		},
		{
			name: "non-Conway pparams returns false (defensive)",
			pp:   &babbage.BabbageProtocolParameters{},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInConwayBootstrapPhase(tt.pp); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
