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
	"errors"
	"math/bits"
	"time"

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

// ValidateLeiosAnnouncementSize rejects a header that announces an endorser
// block larger than maxEndorserBlockReferencesSize. pp must be the protocol
// parameters in force at the header's slot.
func ValidateLeiosAnnouncementSize(
	header *DijkstraBlockHeader,
	pp *DijkstraProtocolParameters,
) error {
	_, size, ok := header.LeiosAnnouncement()
	if !ok {
		return nil
	}
	if size > uint64(pp.MaxEndorserBlockReferencesSize) {
		return &LeiosEbTooBigError{
			// The header decoder bounds the announced size to uint32.
			Announced: uint32(size), // #nosec G115
			Maximum:   pp.MaxEndorserBlockReferencesSize,
		}
	}
	return nil
}

// LeiosMinCertificationSlot returns the earliest slot in which a ranking block
// may certify the endorser block announced at announcingSlot: the
// announcement, vote and diffusion periods, with the announcement period
// counted three times, rounded up to whole slots of slotLength.
func LeiosMinCertificationSlot(
	slotLength time.Duration,
	announcementMs, voteMs, diffusionMs uint32,
	announcingSlot uint64,
) (uint64, error) {
	slotMs := uint64(slotLength.Milliseconds()) // #nosec G115
	if slotLength <= 0 || slotMs == 0 {
		return 0, errors.New("leios certification slot: zero slot length")
	}
	// Three uint32 periods plus two more cannot overflow uint64.
	totalMs := 3*uint64(announcementMs) + uint64(voteMs) +
		uint64(diffusionMs)
	slots := (totalMs + slotMs - 1) / slotMs
	earliest, carry := bits.Add64(announcingSlot, slots, 0)
	if carry != 0 {
		return 0, errors.New("leios certification slot overflows uint64")
	}
	return earliest, nil
}

// ValidateLeiosCertificateTiming applies the header checks that depend on the
// immediately preceding header. A header that sets its certificate flag must
// follow a header that announced an endorser block, and must not be earlier
// than LeiosMinCertificationSlot after that announcement. predecessor is the
// header the given one extends, or nil at the chain origin; a pre-Dijkstra
// predecessor announces nothing. pp and slotLength must be those in force at
// the header's slot.
func ValidateLeiosCertificateTiming(
	header *DijkstraBlockHeader,
	predecessor common.BlockHeader,
	pp *DijkstraProtocolParameters,
	slotLength time.Duration,
) error {
	if certified, _ := header.LeiosCertified(); !certified {
		return nil
	}
	// Only the immediate predecessor counts: the reference chain state keeps
	// one announcement and replaces it on every header, including with
	// nothing.
	announcing, ok := predecessor.(*DijkstraBlockHeader)
	if !ok || announcing == nil {
		return &LeiosCertWithoutAnnouncementError{}
	}
	if _, _, announced := announcing.LeiosAnnouncement(); !announced {
		return &LeiosCertWithoutAnnouncementError{}
	}
	earliest, err := LeiosMinCertificationSlot(
		slotLength,
		pp.LeiosAnnouncementPeriodLength,
		pp.LeiosVotePeriodLength,
		pp.LeiosDiffusionPeriodLength,
		announcing.SlotNumber(),
	)
	if err != nil {
		return err
	}
	if header.SlotNumber() < earliest {
		return &LeiosCertTooYoungError{
			AnnouncingSlot:  announcing.SlotNumber(),
			Slot:            header.SlotNumber(),
			EarliestAllowed: earliest,
		}
	}
	return nil
}

// ValidateLeiosBlockBody checks the block body against its header: the
// header's certificate flag must match the presence of a Leios certificate, a
// certificate excludes transactions, and no Peras certificate is accepted. A
// header without the Leios extension counts as uncertified.
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
	if hasCertificate && len(block.BlockBody.Transactions) > 0 {
		return &LeiosCertifiedBlockTransactionsError{
			TransactionCount: len(block.BlockBody.Transactions),
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
