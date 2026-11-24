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

package consensus

import (
	"crypto/ed25519"
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/stretchr/testify/require"
)

// testEnvelope returns envelope inputs every test header passes: mainnet's
// Conway limits and a protocol version the node supports.
func testEnvelope() *EnvelopeInput {
	return &EnvelopeInput{
		ProtocolMajorVersion:    2,
		MaxMajorProtocolVersion: 10,
		HeaderSize:              860,
		BlockBodySize:           4,
		MaxBlockHeaderSize:      1100,
		MaxBlockBodySize:        90112,
	}
}

// testOverlayD0 returns overlay inputs with d = 0, where TPraos is plain
// Praos.
func testOverlayD0() *TPraosOverlayInput {
	return &TPraosOverlayInput{
		Decentralization: new(big.Rat),
		GenesisDelegates: map[common.Blake2b224]GenesisDelegation{},
	}
}

// overlayFixture is one TPraos header signer: a cold key, a VRF key, and a
// KES key, which may act as a stake pool or as a genesis delegate.
type overlayFixture struct {
	cold  ed25519.PrivateKey
	vrf   *SimpleVRFSigner
	kesSk *kes.SecretKey
	kesPk []byte
	nonce []byte
}

func newOverlayFixture(t *testing.T, tag byte) *overlayFixture {
	t.Helper()
	seed := func(kind byte) []byte {
		s := make([]byte, 32)
		for i := range s {
			s[i] = tag ^ kind ^ byte(i)
		}
		return s
	}
	signer, err := NewSimpleVRFSigner(seed(0x10))
	require.NoError(t, err)
	kesSk, kesPk, err := kes.KeyGen(kes.CardanoKesDepth, seed(0x20))
	require.NoError(t, err)
	nonce := make([]byte, 32)
	for i := range nonce {
		nonce[i] = byte(i)
	}
	return &overlayFixture{
		cold:  ed25519.NewKeyFromSeed(seed(0x30)),
		vrf:   signer,
		kesSk: kesSk,
		kesPk: kesPk,
		nonce: nonce,
	}
}

func (f *overlayFixture) coldHash() common.Blake2b224 {
	return common.Blake2b224Hash(f.cold.Public().(ed25519.PublicKey))
}

func (f *overlayFixture) delegation() GenesisDelegation {
	return GenesisDelegation{
		Delegate:   f.coldHash(),
		VrfKeyHash: common.Blake2b256Hash(f.vrf.PublicKey()),
	}
}

// header returns a TPraos header at slot whose proofs and signatures are
// all valid, with the issuer holding all of the stake.
func (f *overlayFixture) header(
	t *testing.T,
	slot uint64,
) *ValidateHeaderInput {
	t.Helper()
	prove := func(seed []byte) ([]byte, []byte) {
		in, err := vrf.MkSeedTPraos(int64(slot), f.nonce, seed) //nolint:gosec
		require.NoError(t, err)
		proof, out, err := f.vrf.Prove(in)
		require.NoError(t, err)
		return proof, out
	}
	leaderProof, leaderOut := prove(vrf.SeedL())
	nonceProof, nonceOut := prove(vrf.SeedEta())
	body := []byte("tpraos overlay header body")
	kesSig, err := kes.Sign(f.kesSk, 0, body)
	require.NoError(t, err)
	prevHash := make([]byte, 32)
	return &ValidateHeaderInput{
		Slot:           slot,
		BlockNumber:    1,
		PrevHash:       prevHash,
		IssuerVkey:     f.cold.Public().(ed25519.PublicKey),
		VrfKey:         f.vrf.PublicKey(),
		VrfProof:       leaderProof,
		VrfOutput:      leaderOut,
		NonceVrfProof:  nonceProof,
		NonceVrfOutput: nonceOut,
		KesSignature:   kesSig,
		HeaderBodyCbor: body,
		OpCertHotVkey:  f.kesPk,
		OpCertSignature: ed25519.Sign(
			f.cold,
			common.OpCertSignableBytes(f.kesPk, 0, 0),
		),
		OpCertCounterState:   opCertSequence(0),
		PrevHeaderHash:       prevHash,
		EpochNonce:           f.nonce,
		PoolStake:            1,
		TotalStake:           1,
		RegisteredVrfKeyHash: common.Blake2b256Hash(f.vrf.PublicKey()).Bytes(),
		Envelope:             testEnvelope(),
	}
}

// genesisKeys returns seven genesis key hashes (mainnet has seven) in byte
// order, so the overlay schedule's index i names keys[i].
func genesisKeys() []common.Blake2b224 {
	keys := make([]common.Blake2b224, 7)
	for i := range keys {
		keys[i][0] = byte(0x10 * (i + 1))
	}
	return keys
}

// overlayWith schedules delegate under genesis key index idx, and unrelated
// delegates under the other six.
func overlayWith(
	d *big.Rat,
	idx int,
	delegate GenesisDelegation,
) *TPraosOverlayInput {
	delegs := map[common.Blake2b224]GenesisDelegation{}
	for i, key := range genesisKeys() {
		if i == idx {
			delegs[key] = delegate
			continue
		}
		other := GenesisDelegation{}
		other.Delegate[0] = byte(0xa0 + i)
		other.VrfKeyHash[0] = byte(0xa0 + i)
		delegs[key] = other
	}
	return &TPraosOverlayInput{Decentralization: d, GenesisDelegates: delegs}
}

func errorsMatch(errs []error, target error) bool {
	for _, err := range errs {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// TestOverlayScheduleMatchesModel checks the overlay schedule against the
// reference's isOverlaySlot and classifyOverlaySlot (cardano-ledger TPraos
// Rules/Overlay.hs) for every slot of a 6-slot epoch with f = 1/2 and two
// genesis keys, and for mainnet-like cells (f = 1/20, seven genesis keys).
func TestOverlayScheduleMatchesModel(t *testing.T) {
	const (
		praos  = overlaySlotPraos
		active = overlaySlotActive
		idle   = overlaySlotNonActive
	)
	type cell struct {
		d      *big.Rat
		f      *big.Rat
		keys   int
		offset uint64
		kind   overlaySlotKind
		index  int
	}
	half := big.NewRat(1, 2)
	cells := []cell{
		{big.NewRat(0, 1), half, 2, 0, praos, 0},
		{big.NewRat(0, 1), half, 2, 3, praos, 0},
		{big.NewRat(1, 3), half, 2, 0, active, 0},
		{big.NewRat(1, 3), half, 2, 1, praos, 0},
		{big.NewRat(1, 3), half, 2, 2, praos, 0},
		{big.NewRat(1, 3), half, 2, 3, idle, 0},
		{big.NewRat(1, 3), half, 2, 4, praos, 0},
		{big.NewRat(1, 3), half, 2, 5, praos, 0},
		{half, half, 2, 0, active, 0},
		{half, half, 2, 1, praos, 0},
		{half, half, 2, 2, idle, 0},
		{half, half, 2, 3, praos, 0},
		{half, half, 2, 4, active, 1},
		{half, half, 2, 5, praos, 0},
		{big.NewRat(1, 1), half, 2, 0, active, 0},
		{big.NewRat(1, 1), half, 2, 1, idle, 0},
		{big.NewRat(1, 1), half, 2, 2, active, 1},
		{big.NewRat(1, 1), half, 2, 3, idle, 0},
		{big.NewRat(1, 1), half, 2, 4, active, 0},
		{big.NewRat(1, 1), half, 2, 5, idle, 0},
		// Mainnet: f = 1/20, so ascInv = 20.
		{big.NewRat(1, 1), big.NewRat(1, 20), 7, 0, active, 0},
		{big.NewRat(1, 1), big.NewRat(1, 20), 7, 5, idle, 0},
		{big.NewRat(1, 1), big.NewRat(1, 20), 7, 20, active, 1},
		{big.NewRat(1, 1), big.NewRat(1, 20), 7, 140, active, 0},
		{half, big.NewRat(1, 20), 7, 1, praos, 0},
		{half, big.NewRat(1, 20), 7, 2, idle, 0},
		{half, big.NewRat(1, 20), 7, 40, active, 1},
	}
	keys := genesisKeys()
	for _, c := range cells {
		delegs := map[common.Blake2b224]GenesisDelegation{}
		for i := range c.keys {
			deleg := GenesisDelegation{}
			deleg.Delegate[0] = byte(i + 1)
			delegs[keys[i]] = deleg
		}
		const first = 1000
		got := classifyOverlaySlot(
			first+c.offset,
			&TPraosOverlayInput{
				Decentralization: c.d,
				EpochFirstSlot:   first,
				GenesisDelegates: delegs,
			},
			c.f,
		)
		require.Equal(
			t, c.kind, got.kind,
			"d=%s f=%s offset=%d", c.d.RatString(), c.f.RatString(), c.offset,
		)
		if c.kind == active {
			require.Equal(
				t, byte(c.index+1), got.delegation.Delegate[0],
				"d=%s offset=%d", c.d.RatString(), c.offset,
			)
		}
	}
}

// TestValidateHeaderTPraosOverlay covers both directions of the overlay for
// a mainnet-like epoch with d > 0 (f = 1/20, seven genesis keys).
func TestValidateHeaderTPraosOverlay(t *testing.T) {
	validator := NewHeaderValidatorWithMode(
		testNetworkConfig(),
		ConsensusModeTPraos,
	)
	pool := newOverlayFixture(t, 0x01)
	delegate := newOverlayFixture(t, 0x02)

	// Slot 5 is eligible for a pool with all the stake (see
	// TestValidateHeaderTPraosFull), so its header is valid as a pool's.
	t.Run("pool header in a plain slot", func(t *testing.T) {
		for _, d := range []*big.Rat{big.NewRat(0, 1), big.NewRat(1, 2)} {
			in := pool.header(t, 5)
			in.Overlay = overlayWith(d, 0, delegate.delegation())
			res := validator.ValidateHeader(in)
			require.True(t, res.Valid, "d=%s: %v", d.RatString(), res.Errors)
		}
	})

	t.Run("pool header in a non-active slot is rejected", func(t *testing.T) {
		in := pool.header(t, 5)
		in.Overlay = overlayWith(big.NewRat(1, 1), 0, delegate.delegation())
		res := validator.ValidateHeader(in)
		require.False(t, res.Valid)
		require.Len(t, res.Errors, 1, "%v", res.Errors)
		require.ErrorIs(t, res.Errors[0], ErrNotActiveSlot)
	})

	// With d = 1 and ascInv = 20, slot 20 has position 20, so it is active
	// and belongs to genesis key index (20 / 20) mod 7 = 1.
	genesisHeader := func(t *testing.T) *ValidateHeaderInput {
		in := delegate.header(t, 20)
		in.PoolStake = 0
		in.TotalStake = 0
		in.RegisteredVrfKeyHash = nil
		in.Overlay = overlayWith(big.NewRat(1, 1), 1, delegate.delegation())
		return in
	}

	t.Run("genesis delegate header in its slot", func(t *testing.T) {
		res := validator.ValidateHeader(genesisHeader(t))
		require.True(t, res.Valid, "%v", res.Errors)
	})

	t.Run("genesis delegate header in another key's slot", func(t *testing.T) {
		in := genesisHeader(t)
		in.Overlay = overlayWith(big.NewRat(1, 1), 2, delegate.delegation())
		res := validator.ValidateHeader(in)
		require.False(t, res.Valid)
		require.True(
			t,
			errorsMatch(res.Errors, ErrWrongGenesisColdKey),
			"%v",
			res.Errors,
		)
		require.True(
			t,
			errorsMatch(res.Errors, ErrWrongGenesisVRFKey),
			"%v",
			res.Errors,
		)
	})

	t.Run("genesis delegate header with another VRF key", func(t *testing.T) {
		in := genesisHeader(t)
		deleg := delegate.delegation()
		deleg.VrfKeyHash[0] ^= 0xff
		in.Overlay = overlayWith(big.NewRat(1, 1), 1, deleg)
		res := validator.ValidateHeader(in)
		require.False(t, res.Valid)
		require.Len(t, res.Errors, 1, "%v", res.Errors)
		require.ErrorIs(t, res.Errors[0], ErrWrongGenesisVRFKey)
	})

	t.Run("genesis delegate header with a bad nonce proof", func(t *testing.T) {
		in := genesisHeader(t)
		in.NonceVrfProof = append([]byte(nil), in.NonceVrfProof...)
		in.NonceVrfProof[0] ^= 0xff
		res := validator.ValidateHeader(in)
		require.False(t, res.Valid)
	})
}

// TestValidateHeaderRequiresInputs checks that a missing input is reported
// as the only error, in the modes that need it.
func TestValidateHeaderRequiresInputs(t *testing.T) {
	tpraos := NewHeaderValidatorWithMode(
		testNetworkConfig(),
		ConsensusModeTPraos,
	)
	cpraos := NewHeaderValidator(testNetworkConfig())
	pool := newOverlayFixture(t, 0x01)
	valid := func() *ValidateHeaderInput {
		in := pool.header(t, 5)
		in.Overlay = testOverlayD0()
		return in
	}
	require.True(t, tpraos.ValidateHeader(valid()).Valid)

	cases := map[string]func(*ValidateHeaderInput){
		"no envelope":       func(in *ValidateHeaderInput) { in.Envelope = nil },
		"no protocol major": func(in *ValidateHeaderInput) { in.Envelope.ProtocolMajorVersion = 0 },
		"no max major":      func(in *ValidateHeaderInput) { in.Envelope.MaxMajorProtocolVersion = 0 },
		"no header size":    func(in *ValidateHeaderInput) { in.Envelope.HeaderSize = 0 },
		"no body size":      func(in *ValidateHeaderInput) { in.Envelope.BlockBodySize = 0 },
		"no max header":     func(in *ValidateHeaderInput) { in.Envelope.MaxBlockHeaderSize = 0 },
		"no max body":       func(in *ValidateHeaderInput) { in.Envelope.MaxBlockBodySize = 0 },
		"no overlay":        func(in *ValidateHeaderInput) { in.Overlay = nil },
		"no d":              func(in *ValidateHeaderInput) { in.Overlay.Decentralization = nil },
		"d above one":       func(in *ValidateHeaderInput) { in.Overlay.Decentralization = big.NewRat(3, 2) },
		"no delegates":      func(in *ValidateHeaderInput) { in.Overlay.GenesisDelegates = nil },
		"d > 0, no delegates": func(in *ValidateHeaderInput) {
			in.Overlay.Decentralization = big.NewRat(1, 2)
		},
		"epoch after slot": func(in *ValidateHeaderInput) { in.Overlay.EpochFirstSlot = 6 },
	}
	for name, mutate := range cases {
		in := valid()
		mutate(in)
		res := tpraos.ValidateHeader(in)
		require.False(t, res.Valid, name)
		require.Len(t, res.Errors, 1, "%s: %v", name, res.Errors)
		require.ErrorIs(t, res.Errors[0], ErrMissingHeaderInput, name)
	}

	noOverlay := valid()
	noOverlay.Overlay = nil
	res := cpraos.ValidateHeader(noOverlay)
	require.False(
		t,
		errorsMatch(res.Errors, ErrMissingHeaderInput),
		"%v",
		res.Errors,
	)

	noEnvelope := valid()
	noEnvelope.Envelope = nil
	res = cpraos.ValidateHeader(noEnvelope)
	require.Len(t, res.Errors, 1, "%v", res.Errors)
	require.ErrorIs(t, res.Errors[0], ErrMissingHeaderInput)
}

// TestValidateHeaderEnvelopeLimits checks ObsoleteNode and the header and
// body size limits, which are inclusive.
func TestValidateHeaderEnvelopeLimits(t *testing.T) {
	validator := NewHeaderValidatorWithMode(
		testNetworkConfig(),
		ConsensusModeTPraos,
	)
	pool := newOverlayFixture(t, 0x01)
	cases := []struct {
		name   string
		mutate func(*EnvelopeInput)
		want   error
	}{
		{"at every limit", func(e *EnvelopeInput) {
			e.ProtocolMajorVersion = e.MaxMajorProtocolVersion
			e.HeaderSize = e.MaxBlockHeaderSize
			e.BlockBodySize = e.MaxBlockBodySize
		}, nil},
		{"obsolete node", func(e *EnvelopeInput) {
			e.ProtocolMajorVersion = e.MaxMajorProtocolVersion + 1
		}, ErrObsoleteNode},
		{"header too large", func(e *EnvelopeInput) {
			e.HeaderSize = e.MaxBlockHeaderSize + 1
		}, ErrHeaderSizeTooLarge},
		{"body too large", func(e *EnvelopeInput) {
			e.BlockBodySize = e.MaxBlockBodySize + 1
		}, ErrBlockSizeTooLarge},
	}
	for _, c := range cases {
		in := pool.header(t, 5)
		in.Overlay = testOverlayD0()
		c.mutate(in.Envelope)
		res := validator.ValidateHeader(in)
		if c.want == nil {
			require.True(t, res.Valid, "%s: %v", c.name, res.Errors)
			continue
		}
		require.False(t, res.Valid, c.name)
		require.Len(t, res.Errors, 1, "%s: %v", c.name, res.Errors)
		require.ErrorIs(t, res.Errors[0], c.want, c.name)
	}
}
