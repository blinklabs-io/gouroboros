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

package dijkstra

import (
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// ValidateHeaderProtocolVersion applies the Dijkstra BBODY header version
// rule: the issuer's highest supported major protocol version, carried in the
// header's version info, must not be below the major version in force. Unlike
// earlier eras, Dijkstra places no upper bound on it.
func ValidateHeaderProtocolVersion(
	header *DijkstraBlockHeader,
	pp *DijkstraProtocolParameters,
) error {
	supplied := header.Body.ProtoVersion.Major
	expected := uint64(pp.ProtocolVersion.Major)
	if supplied < expected {
		return &HeaderProtVerTooLowError{
			Supplied: supplied,
			Expected: expected,
		}
	}
	return nil
}

// leiosAnnouncer is the part of a Dijkstra header the certification check reads
// from its predecessor. A chain state may hand back a wrapper
// around the header, so the check does not require the concrete type.
type leiosAnnouncer interface {
	SlotNumber() uint64
	LeiosAnnouncement() (
		hash common.Blake2b256,
		size uint64,
		ok bool,
	)
}

// ValidateLeiosCertificateAnnouncement requires a header that sets its
// certificate flag to follow a header that announced an endorser block.
// predecessor is the header the given one extends, or nil at the chain origin;
// a pre-Dijkstra predecessor announces nothing.
func ValidateLeiosCertificateAnnouncement(
	header *DijkstraBlockHeader,
	predecessor common.BlockHeader,
) error {
	if certified, _ := header.LeiosCertified(); !certified {
		return nil
	}
	// Only the immediate predecessor counts: the reference chain state keeps
	// one announcement and replaces it on every header, including with
	// nothing.
	if concrete, isConcrete := predecessor.(*DijkstraBlockHeader); isConcrete &&
		concrete == nil {
		return &LeiosCertWithoutAnnouncementError{}
	}
	announcing, ok := predecessor.(leiosAnnouncer)
	if !ok {
		return &LeiosCertWithoutAnnouncementError{}
	}
	if _, _, announced := announcing.LeiosAnnouncement(); !announced {
		return &LeiosCertWithoutAnnouncementError{}
	}
	return nil
}

// ValidateLeiosBlockBody checks the block body against its header: the
// header's certificate flag must match the presence of a Leios certificate,
// and no Peras certificate is accepted. A header without the Leios extension
// counts as uncertified.
func ValidateLeiosBlockBody(block *DijkstraBlock) error {
	var certified bool
	if block.BlockHeader != nil {
		certified, _ = block.BlockHeader.LeiosCertified()
	}
	hasCertificate := block.BlockBody.LeiosCertificate != nil
	if certified != hasCertificate {
		return &LeiosCertificateFlagMismatchError{
			HeaderCertified:    certified,
			BodyHasCertificate: hasCertificate,
		}
	}
	// The reference ledger fails every Peras certificate until Peras is
	// enabled, so even an empty byte string is rejected.
	if block.BlockBody.PerasCertificate != nil {
		return &PerasCertValidationFailedError{
			Certificate: block.BlockBody.PerasCertificate,
		}
	}
	return nil
}
