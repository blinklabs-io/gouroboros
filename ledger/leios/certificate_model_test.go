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

package leios

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// certModelScenario is one configuration of the LeiosCommittee TLA+ model:
// the committee-size parameter, the seated committee (stake, and a secret
// for each keyed seat), the total active stake, and the quorum threshold.
type certModelScenario struct {
	name          string
	committeeSize uint16
	stakes        []uint64
	secrets       []int64 // 0 marks a keyless seat
	totalStake    uint64
	quorum        *big.Rat
}

// certModelCell is one certificate: its bitfield length, the bit indices
// set (a bit past the last seat is padding), and the seats whose
// signatures were aggregated.
type certModelCell struct {
	length   int
	bits     []int
	signedBy []int
}

// refAccept is the model's RefAccept: verifyLeiosCert in
// cardano-crypto-leios sizes the bitfield by the committee's own seat
// count, refuses keyless signers, needs the quorum, and needs the
// aggregate of exactly the signers' keys.
func (s certModelScenario) refAccept(c certModelCell) bool {
	n := len(s.stakes)
	if c.length != (n+7)/8 {
		return false
	}
	var signers []int
	var stake uint64
	for _, bit := range c.bits {
		if bit >= n {
			continue
		}
		if s.secrets[bit] == 0 {
			return false
		}
		signers = append(signers, bit)
		stake += s.stakes[bit]
	}
	weight := new(big.Rat).SetFrac(
		new(big.Int).SetUint64(stake),
		new(big.Int).SetUint64(s.totalStake),
	)
	if len(signers) == 0 || weight.Cmp(s.quorum) < 0 {
		return false
	}
	return fmt.Sprint(signers) == fmt.Sprint(c.signedBy)
}

// cells enumerates certificates for the scenario: every set of the bits
// up to one past the last seat, against every set of keyed signers, at
// the right length; and the full bitfield at one byte short and one over.
func (s certModelScenario) cells() []certModelCell {
	n := len(s.stakes)
	var keyed []int
	for i, secret := range s.secrets {
		if secret != 0 {
			keyed = append(keyed, i)
		}
	}
	right := (n + 7) / 8
	var cells []certModelCell
	for mask := 0; mask < 1<<(n+1); mask++ {
		for signedMask := 0; signedMask < 1<<len(keyed); signedMask++ {
			cell := certModelCell{
				length:   right,
				bits:     subset(mask, n+1, nil),
				signedBy: subset(signedMask, len(keyed), keyed),
			}
			cells = append(cells, cell)
			if mask == 1<<n-1 {
				for _, length := range []int{right - 1, right + 1} {
					wrong := cell
					wrong.length = length
					cells = append(cells, wrong)
				}
			}
		}
	}
	return cells
}

// subset returns the members of 0..size-1 (or of from) selected by mask.
func subset(mask, size int, from []int) []int {
	var out []int
	for i := range size {
		if mask&(1<<i) == 0 {
			continue
		}
		if from == nil {
			out = append(out, i)
		} else {
			out = append(out, from[i])
		}
	}
	return out
}

// certModelScenarios are the passing configurations of LeiosCommittee.tla
// with real BLS keys. Seat i holds the i-th largest stake.
func certModelScenarios() []certModelScenario {
	return []certModelScenario{
		{
			name:          "as many pools as seats",
			committeeSize: 3,
			stakes:        []uint64{3, 2, 1},
			secrets:       []int64{11, 13, 0},
			totalStake:    6,
			quorum:        big.NewRat(3, 5),
		},
		{
			name:          "more pools than seats",
			committeeSize: 2,
			stakes:        []uint64{4, 3},
			secrets:       []int64{17, 19},
			totalStake:    10,
			quorum:        big.NewRat(3, 5),
		},
	}
}

func runCertModelScenario(t *testing.T, s certModelScenario) {
	t.Helper()
	hash := common.Blake2b256Hash([]byte("announcing ranking block"))
	context := common.DijkstraLeiosCertificateContext{
		AnnouncingBlockHash: hash,
		TotalActiveStake:    s.totalStake,
	}
	for i, stake := range s.stakes {
		seat := common.DijkstraLeiosCommitteeMember{Stake: stake}
		if s.secrets[i] != 0 {
			seat.Key = makeLeiosTestKey(t, big.NewInt(s.secrets[i]))
		}
		context.Committee = append(context.Committee, seat)
	}
	accepted := 0
	cells := s.cells()
	for _, cell := range cells {
		// BLS signatures add: the aggregate of the signers' signatures is
		// the signature under the sum of their secrets. No signer at all
		// stands for a signature by a key outside the committee.
		secret := big.NewInt(0)
		for _, seat := range cell.signedBy {
			secret.Add(secret, big.NewInt(s.secrets[seat]))
		}
		if len(cell.signedBy) == 0 {
			secret.SetInt64(97)
		}
		signature := signLeiosTestMessage(
			t, secret, hash[:], leiosSignatureDST,
		)
		signers := make([]byte, max(cell.length, 0))
		for _, bit := range cell.bits {
			if bit/8 < len(signers) {
				signers[bit/8] |= 1 << (7 - bit%8)
			}
		}
		err := VerifyDijkstraCertificate(
			signers, signature, s.committeeSize, s.quorum, context,
		)
		want := s.refAccept(cell)
		require.Equal(t, want, err == nil, "%+v: err %v", cell, err)
		if want {
			accepted++
		}
	}
	require.Positive(t, accepted)
	require.Less(t, accepted, len(cells))
}

// TestCertificateModelCells checks VerifyDijkstraCertificate against the
// reference's acceptance on every certificate of each LeiosCommittee model
// scenario.
func TestCertificateModelCells(t *testing.T) {
	t.Parallel()
	for _, s := range certModelScenarios() {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			runCertModelScenario(t, s)
		})
	}
}
