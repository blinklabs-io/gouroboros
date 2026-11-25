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
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/stretchr/testify/require"
)

// The committee and vote rules these tests transcribe, all at the revisions
// ouroboros-consensus ec239d57f618 builds against:
//
//   - Seating: cardano-ledger 1210d37f
//     libs/cardano-ledger-core/src/Cardano/Ledger/State/Leios.hs 93-121
//     (selectLeiosCommittee): pools by descending stake, ties by ascending
//     pool id, the first min(Nc, pools); a pool without an honoured key, or
//     whose proof of possession fails, is seated keyless.
//   - Votes: ouroboros-consensus ec239d57f618
//     ouroboros-consensus/src/ouroboros-consensus/Ouroboros/Consensus/Leios/VoteState.hs
//     224-399: a vote of a seat that already voted in the election is dropped
//     unverified, whichever announcement it is for; otherwise the seat must
//     resolve, hold a key and sign; each announcement's tally gains the
//     seat's weight, and the first time a tally reaches the quorum the
//     announcement's votes are aggregated into its certificate.
//   - Verification: cardano-base 242f1ebde40b
//     cardano-crypto-leios/src/Cardano/Crypto/Leios.hs 291-342
//     (verifyLeiosCert): no keyless signer, signer weight >= threshold, and
//     the aggregate signature.

type votingKeyState int

const (
	keyValid votingKeyState = iota
	keyBadProof
	keyAged
	keyNone
)

type votingPool struct {
	id    int
	stake uint64
	key   votingKeyState
}

type votingScenario struct {
	name  string
	pools []votingPool
	nc    int
	tau   *big.Rat
	// seated is the pool ids in seat order, read off the CIP-0164 rule by
	// hand, so the test's own selection is checked against it.
	seated []int
}

func votingScenarios() []votingScenario {
	return []votingScenario{
		{
			// Three pools of equal stake around the seat boundary; the
			// signers' quorum is exactly reachable (6 of 11).
			name: "ties",
			pools: []votingPool{
				{1, 4, keyValid},
				{2, 2, keyValid},
				{3, 2, keyBadProof},
				{4, 2, keyValid},
				{5, 1, keyValid},
			},
			nc:     3,
			tau:    big.NewRat(6, 11),
			seated: []int{1, 2, 3},
		},
		{
			name: "wide",
			pools: []votingPool{
				{1, 2, keyValid},
				{2, 2, keyValid},
				{3, 2, keyValid},
				{4, 1, keyValid},
			},
			nc:     4,
			tau:    big.NewRat(3, 5),
			seated: []int{1, 2, 3, 4},
		},
		{
			// Fewer pools than seats: every pool is seated.
			name: "few",
			pools: []votingPool{
				{1, 3, keyValid},
				{2, 2, keyNone},
			},
			nc:     3,
			tau:    big.NewRat(3, 5),
			seated: []int{1, 2},
		},
		{
			name: "aged",
			pools: []votingPool{
				{1, 3, keyValid},
				{2, 2, keyValid},
				{3, 1, keyAged},
			},
			nc:     3,
			tau:    big.NewRat(1, 2),
			seated: []int{1, 2, 3},
		},
	}
}

// seatCommittee transcribes selectLeiosCommittee.
func seatCommittee(pools []votingPool, nc int) []votingPool {
	sorted := slices.Clone(pools)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].stake != sorted[j].stake {
			return sorted[i].stake > sorted[j].stake
		}
		return sorted[i].id < sorted[j].id
	})
	return sorted[:min(nc, len(sorted))]
}

type votingFixture struct {
	scenario  votingScenario
	seats     []votingPool
	secrets   []*big.Int
	total     uint64
	anns      [2]common.Blake2b256
	voteSeats int
}

func newVotingFixture(t *testing.T, sc votingScenario) *votingFixture {
	t.Helper()
	f := &votingFixture{
		scenario:  sc,
		seats:     seatCommittee(sc.pools, sc.nc),
		voteSeats: max(sc.nc, len(sc.pools)),
		anns: [2]common.Blake2b256{
			common.Blake2b256Hash([]byte("announcing ranking block a")),
			common.Blake2b256Hash([]byte("announcing ranking block b")),
		},
	}
	for _, p := range sc.pools {
		f.total += p.stake
	}
	for _, p := range f.seats {
		f.secrets = append(f.secrets, big.NewInt(int64(1000+p.id)))
	}
	return f
}

// refKeyed reports whether the reference seats a key in seat i.
func (f *votingFixture) refKeyed(i int) bool {
	return i < len(f.seats) && f.seats[i].key == keyValid
}

// context builds the committee a caller hands gOuroboros: the seated pools in
// seat order with their registered keys, a badly proven one included, since
// gOuroboros checks proofs itself. passAged says whether the caller also
// hands over keys past their honoured age, which only it can judge.
func (f *votingFixture) context(
	t *testing.T,
	ann int,
	passAged bool,
) common.DijkstraLeiosCertificateContext {
	t.Helper()
	ctx := common.DijkstraLeiosCertificateContext{
		AnnouncingBlockHash: f.anns[ann],
		TotalActiveStake:    f.total,
	}
	for i, p := range f.seats {
		member := common.DijkstraLeiosCommitteeMember{Stake: p.stake}
		switch p.key {
		case keyValid:
			member.Key = makeLeiosTestKey(t, f.secrets[i])
		case keyAged:
			if passAged {
				member.Key = makeLeiosTestKey(t, f.secrets[i])
			}
		case keyBadProof:
			key := makeLeiosTestKey(t, f.secrets[i])
			other := makeLeiosTestKey(t, big.NewInt(7))
			key.PossessionProof = other.PossessionProof
			member.Key = key
		case keyNone:
		}
		ctx.Committee = append(ctx.Committee, member)
	}
	return ctx
}

// vote is one vote message: a voter id, the announcement it is for, and
// whether its signature is good. A bad one signs the other announcement.
type vote struct {
	seat int
	ann  int
	ok   bool
}

// aggregation is one way of collecting votes into certificates: the
// reference's, or the reference with one check dropped.
type aggregation string

const (
	aggReference         aggregation = "reference"
	aggCountEquivocation aggregation = "count-equivocation"
	aggCountDuplicate    aggregation = "count-duplicate"
	aggAcceptKeyless     aggregation = "accept-keyless"
	aggAcceptBadSig      aggregation = "accept-bad-signature"
	aggAggregateAll      aggregation = "aggregate-all-votes"
)

type voteState struct {
	recorded []vote // sorted, no repeats
	tally    [2]uint64
	cert     [2][]vote // nil: no certificate
}

func (s voteState) key() string {
	return fmt.Sprint(s.recorded, s.tally, s.cert)
}

// receive applies one arriving vote to the state as the given aggregation
// does, returning the new state and the certificate it formed, if any.
func (f *votingFixture) receive(
	s voteState,
	v vote,
	agg aggregation,
) (voteState, []vote, int) {
	same, seen := false, false
	for _, r := range s.recorded {
		if r.seat == v.seat {
			seen = true
			same = same || r.ann == v.ann
		}
	}
	switch agg {
	case aggCountDuplicate:
		seen = false
	case aggCountEquivocation:
		seen = same
	}
	if seen || v.seat >= len(f.seats) {
		return s, nil, -1
	}
	if agg != aggAcceptKeyless && !f.refKeyed(v.seat) {
		return s, nil, -1
	}
	if agg != aggAcceptBadSig && !v.ok {
		return s, nil, -1
	}
	next := voteState{
		recorded: slices.Clone(s.recorded),
		tally:    s.tally,
		cert:     s.cert,
	}
	if !slices.Contains(next.recorded, v) {
		next.recorded = append(next.recorded, v)
		slices.SortFunc(next.recorded, func(a, b vote) int {
			return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
		})
	}
	next.tally[v.ann] += f.seats[v.seat].stake
	if next.cert[v.ann] != nil || !f.reachesQuorum(next.tally[v.ann]) {
		return next, nil, -1
	}
	var votes []vote
	for _, r := range next.recorded {
		if agg == aggAggregateAll || r.ann == v.ann {
			votes = append(votes, r)
		}
	}
	next.cert[v.ann] = votes
	return next, votes, v.ann
}

func (f *votingFixture) reachesQuorum(stake uint64) bool {
	left := new(big.Int).Mul(
		new(big.Int).SetUint64(stake), f.scenario.tau.Denom(),
	)
	right := new(big.Int).Mul(
		new(big.Int).SetUint64(f.total), f.scenario.tau.Num(),
	)
	return left.Cmp(right) >= 0
}

// refAccepts is verifyLeiosCert's verdict on a certificate for announcement
// ann aggregating the given votes.
func (f *votingFixture) refAccepts(ann int, votes []vote) bool {
	var stake uint64
	signers := map[int]bool{}
	for _, v := range votes {
		if !v.ok || v.ann != ann || signers[v.seat] {
			return false
		}
		signers[v.seat] = true
		if !f.refKeyed(v.seat) {
			return false
		}
		stake += f.seats[v.seat].stake
	}
	return f.reachesQuorum(stake)
}

// certificate builds the real signer bitfield and aggregate BLS signature
// for the given votes over a committee of the seated size.
func (f *votingFixture) certificate(
	t *testing.T,
	votes []vote,
) ([]byte, []byte) {
	t.Helper()
	signers := make([]byte, common.LeiosSignerBitfieldSize(uint64(len(f.seats))))
	var aggregate bls12381.G1Affine
	for i, v := range votes {
		signers[v.seat/8] |= 1 << (7 - v.seat%8)
		message := f.anns[v.ann]
		if !v.ok {
			message = f.anns[1-v.ann]
		}
		encoded := signLeiosTestMessage(
			t, f.secrets[v.seat], message[:], leiosSignatureDST,
		)
		var sig bls12381.G1Affine
		_, err := sig.SetBytes(encoded)
		require.NoError(t, err)
		if i == 0 {
			aggregate = sig
		} else {
			aggregate.Add(&aggregate, &sig)
		}
	}
	encoded := aggregate.Bytes()
	return signers, encoded[:]
}

func (f *votingFixture) goAccepts(
	t *testing.T,
	ann int,
	votes []vote,
	passAged bool,
) (bool, error) {
	t.Helper()
	signers, signature := f.certificate(t, votes)
	return f.goVerify(t, ann, signers, signature, passAged)
}

func (f *votingFixture) goVerify(
	t *testing.T,
	ann int,
	signers, signature []byte,
	passAged bool,
) (bool, error) {
	t.Helper()
	err := VerifyDijkstraCertificateForMessage(
		signers,
		signature,
		uint16(len(f.seats)),
		f.scenario.tau,
		f.context(t, ann, passAged),
		common.DijkstraBlsProofDomainPop,
		common.DijkstraLeiosCertificateMessageRawHash,
	)
	return err == nil, err
}

type formedCert struct {
	ann   int
	votes []vote
}

type walkResult struct {
	states int
	certs  map[string]formedCert
	// twoCerts counts reachable states certifying both announcements.
	twoCerts int
}

// walk explores every state the aggregation reaches from no votes, with
// votes from every voter id (including ids past the committee), for either
// announcement, signed well or badly, arriving in any order and any number
// of times, and collects every certificate formed.
func (f *votingFixture) walk(agg aggregation) walkResult {
	var messages []vote
	for seat := range f.voteSeats {
		for ann := range 2 {
			messages = append(messages, vote{seat, ann, true}, vote{seat, ann, false})
		}
	}
	res := walkResult{certs: map[string]formedCert{}}
	start := voteState{}
	seen := map[string]bool{start.key(): true}
	queue := []voteState{start}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		res.states++
		if s.cert[0] != nil && s.cert[1] != nil {
			res.twoCerts++
		}
		for _, m := range messages {
			// A miscounting aggregation can grow a tally without bound;
			// the reference never counts more than the total stake.
			if s.tally[m.ann] > 2*f.total {
				continue
			}
			next, votes, ann := f.receive(s, m, agg)
			if votes != nil {
				res.certs[fmt.Sprint(ann, votes)] = formedCert{ann, votes}
			}
			if k := next.key(); !seen[k] {
				seen[k] = true
				queue = append(queue, next)
			}
		}
	}
	return res
}

func TestLeiosVotingCommitteeSeating(t *testing.T) {
	for _, sc := range votingScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			var ids []int
			for _, p := range seatCommittee(sc.pools, sc.nc) {
				ids = append(ids, p.id)
			}
			require.Equal(t, sc.seated, ids)
		})
	}
}

// TestLeiosVotingReferenceCertificates checks that every certificate the
// reference's vote collection can form, over a committee seated by the
// reference's rule, is one VerifyDijkstraCertificateForMessage accepts.
func TestLeiosVotingReferenceCertificates(t *testing.T) {
	for _, sc := range votingScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			f := newVotingFixture(t, sc)
			res := f.walk(aggReference)
			require.NotEmpty(t, res.certs, "no certificate is reachable")
			require.Zero(
				t, res.twoCerts,
				"the reference certified both announcements of an election",
			)
			for _, c := range res.certs {
				require.True(t, f.refAccepts(c.ann, c.votes), "%v", c)
				ok, err := f.goAccepts(t, c.ann, c.votes, false)
				require.True(t, ok, "certificate %v: %v", c, err)
			}
		})
	}
}

// TestLeiosVotingMutatedAggregation drops one of the reference's vote checks
// at a time and checks that gOuroboros rejects exactly the resulting
// certificates the reference rejects, and that each dropped check does yield
// such a certificate.
func TestLeiosVotingMutatedAggregation(t *testing.T) {
	cases := []struct {
		scenario string
		agg      aggregation
	}{
		{"ties", aggCountDuplicate},
		{"ties", aggAcceptKeyless},
		{"ties", aggAcceptBadSig},
		{"few", aggAcceptKeyless},
		{"wide", aggAggregateAll},
	}
	scenarios := map[string]votingScenario{}
	for _, sc := range votingScenarios() {
		scenarios[sc.name] = sc
	}
	for _, tc := range cases {
		t.Run(tc.scenario+"/"+string(tc.agg), func(t *testing.T) {
			f := newVotingFixture(t, scenarios[tc.scenario])
			res := f.walk(tc.agg)
			rejected := 0
			for _, c := range res.certs {
				want := f.refAccepts(c.ann, c.votes)
				if !want {
					rejected++
				}
				got, err := f.goAccepts(t, c.ann, c.votes, false)
				require.Equal(t, want, got, "certificate %v: %v", c, err)
			}
			require.NotZero(
				t, rejected,
				"the dropped check produced no certificate the reference rejects",
			)
		})
	}
}

// TestLeiosVotingEquivocationIsTheCallers checks the one rule a certificate
// cannot show: a seat counted for both announcements of an election lets
// both be certified, and each certificate alone is valid. Only the vote
// collector, which is the caller's, can keep a seat to one vote per election.
func TestLeiosVotingEquivocationIsTheCallers(t *testing.T) {
	sc := votingScenarios()[0]
	f := newVotingFixture(t, sc)
	res := f.walk(aggCountEquivocation)
	require.NotZero(t, res.twoCerts)
	for _, c := range res.certs {
		require.True(t, f.refAccepts(c.ann, c.votes), "%v", c)
		ok, err := f.goAccepts(t, c.ann, c.votes, false)
		require.True(t, ok, "certificate %v: %v", c, err)
	}
}

// TestLeiosVotingEverySignerSet checks gOuroboros against verifyLeiosCert on
// every signer set of every seated committee, each signer signing well.
// An aged-out key is withheld by the caller; with it handed over, gOuroboros
// cannot tell it apart and accepts a certificate the reference rejects
// (SignerWithoutKey), so key age is the caller's to enforce.
func TestLeiosVotingEverySignerSet(t *testing.T) {
	for _, sc := range votingScenarios() {
		for _, passAged := range []bool{false, true} {
			name := sc.name
			if passAged {
				name += "/aged-key-handed-over"
			}
			t.Run(name, func(t *testing.T) {
				f := newVotingFixture(t, sc)
				disagree := 0
				for mask := 1; mask < 1<<len(f.seats); mask++ {
					var votes []vote
					for i := range f.seats {
						if mask&(1<<i) != 0 {
							votes = append(votes, vote{i, 0, true})
						}
					}
					want := f.refAccepts(0, votes)
					got, err := f.goAccepts(t, 0, votes, passAged)
					if passAged && got != want {
						disagree++
						continue
					}
					require.Equal(t, want, got, "signers %v: %v", votes, err)
				}
				if passAged && sc.name == "aged" {
					require.NotZero(t, disagree)
				} else {
					require.Zero(t, disagree)
				}
			})
		}
	}
}

// TestLeiosVotingKeylessBitsDoNotCount sets the bits of keyless seats next
// to keyed signers whose signatures alone make the aggregate, so a keyless
// seat's stake would complete the quorum if it were counted. The reference
// rejects every such certificate before counting weight (SignerWithoutKey).
func TestLeiosVotingKeylessBitsDoNotCount(t *testing.T) {
	for _, sc := range votingScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			f := newVotingFixture(t, sc)
			cells := 0
			for mask := 1; mask < 1<<len(f.seats); mask++ {
				var keyed []vote
				padded := false
				for i := range f.seats {
					if mask&(1<<i) == 0 {
						continue
					}
					if f.refKeyed(i) {
						keyed = append(keyed, vote{i, 0, true})
					} else {
						padded = true
					}
				}
				if !padded || len(keyed) == 0 {
					continue
				}
				cells++
				_, signature := f.certificate(t, keyed)
				signers := make(
					[]byte,
					common.LeiosSignerBitfieldSize(uint64(len(f.seats))),
				)
				for i := range f.seats {
					if mask&(1<<i) != 0 {
						signers[i/8] |= 1 << (7 - i%8)
					}
				}
				ok, err := f.goVerify(t, 0, signers, signature, false)
				require.False(t, ok, "signer mask %b accepted", mask)
				require.Error(t, err)
			}
			if sc.name != "wide" {
				require.NotZero(t, cells)
			}
		})
	}
}

// TestCertificateQuorumBoundary pins the stake threshold arithmetic. The
// expectations are computed from verifyLeiosCert's rule (cardano-base
// 242f1ebde40b Cardano/Crypto/Leios.hs 316-318): each seat weighs its stake
// over the total active stake (cardano-ledger 1210d37f State/SnapShots.hs
// 228), and a certificate is refused when the signers' summed weight is
// below the threshold, so exactly the threshold is accepted. They are not
// gOuroboros output.
func TestCertificateQuorumBoundary(t *testing.T) {
	const maxUint64 = ^uint64(0)
	cases := []struct {
		name   string
		signer uint64
		total  uint64
		tau    *big.Rat
		accept bool
	}{
		{"exactly 3/5", 60, 100, big.NewRat(3, 5), true},
		{"one below 3/5", 59, 100, big.NewRat(3, 5), false},
		{"1/2 of an odd total, rounded down", 50, 101, big.NewRat(1, 2), false},
		{"1/2 of an odd total, rounded up", 51, 101, big.NewRat(1, 2), true},
		{"3/4 exactly", 75, 100, big.NewRat(3, 4), true},
		{"3/4 one below", 74, 100, big.NewRat(3, 4), false},
		{"all stake at threshold 1", 100, 100, big.NewRat(1, 1), true},
		{"all but one at threshold 1", 99, 100, big.NewRat(1, 1), false},
		{"any signer at threshold 0", 1, 100, big.NewRat(0, 1), true},
		// 2^64-1 is divisible by 3, so 2/3 of it is exactly
		// 12297829382473034410.
		{"2/3 of max uint64", 12297829382473034410, maxUint64, big.NewRat(2, 3), true},
		{"one below 2/3 of max uint64", 12297829382473034409, maxUint64, big.NewRat(2, 3), false},
		{"6/11 of 11", 6, 11, big.NewRat(6, 11), true},
		{"5/11 of 11 at 6/11", 5, 11, big.NewRat(6, 11), false},
	}
	secret := big.NewInt(23)
	key := makeLeiosTestKey(t, secret)
	hash := common.Blake2b256Hash([]byte("quorum boundary"))
	signature := signLeiosTestMessage(t, secret, hash[:], leiosSignatureDST)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			committee := []common.DijkstraLeiosCommitteeMember{
				{Stake: tc.signer, Key: key},
			}
			if rest := tc.total - tc.signer; rest > 0 {
				committee = append(
					committee,
					common.DijkstraLeiosCommitteeMember{Stake: rest},
				)
			}
			err := VerifyDijkstraCertificate(
				[]byte{0x80},
				signature,
				uint16(len(committee)),
				tc.tau,
				common.DijkstraLeiosCertificateContext{
					AnnouncingBlockHash: hash,
					TotalActiveStake:    tc.total,
					Committee:           committee,
				},
			)
			if tc.accept {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrInsufficientQuorum)
			}
		})
	}
}
