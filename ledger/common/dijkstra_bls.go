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

package common

// DijkstraBlsProofDomain selects the domain required by a network's POOL rule.
type DijkstraBlsProofDomain uint8

const (
	// DijkstraBlsProofDomainPop uses the IETF BLS_POP proof domain.
	DijkstraBlsProofDomainPop DijkstraBlsProofDomain = iota
	// DijkstraBlsProofDomainLegacySig uses the BLS_SIG domain required by
	// DijkstraNet's older cardano-crypto proof-of-possession implementation.
	DijkstraBlsProofDomainLegacySig
)

// DijkstraBlsProofDomainState supplies a network's pool-registration proof
// domain. A provider must select the network's rule, not infer it from a proof.
type DijkstraBlsProofDomainState interface {
	DijkstraBlsProofDomain() DijkstraBlsProofDomain
}

// DijkstraBlsProofDomainFor returns the first proof-domain provider's choice
// through validation adapters, or the IETF proof domain when none is present.
func DijkstraBlsProofDomainFor(ls LedgerState) DijkstraBlsProofDomain {
	if state, ok := ledgerStateCapabilityFor[DijkstraBlsProofDomainState](ls); ok {
		return state.DijkstraBlsProofDomain()
	}
	return DijkstraBlsProofDomainPop
}
