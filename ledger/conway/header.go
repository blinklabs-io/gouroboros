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

import "github.com/blinklabs-io/gouroboros/ledger/common"

// ValidateHeaderProtocolVersion applies the Conway BBODY header version rule:
// on mainnet, the header's major protocol version must not exceed the major
// version in force by more than one. Other networks have no upper bound.
// networkId is the ledger's network, as returned by LedgerState.NetworkId.
func ValidateHeaderProtocolVersion(
	header *ConwayBlockHeader,
	pp *ConwayProtocolParameters,
	networkId uint,
) error {
	if networkId != common.AddressNetworkMainnet {
		return nil
	}
	supplied := header.Body.ProtoVersion.Major
	expected := uint64(pp.ProtocolVersion.Major) + 1
	if supplied > expected {
		return &HeaderProtVerTooHighError{
			Supplied: supplied,
			Expected: expected,
		}
	}
	return nil
}
