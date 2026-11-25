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
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

// The header goldens in testdata are copied unchanged from
// ouroboros-consensus-cardano/golden/cardano/CardanoNodeToNodeVersion2 at
// ouroboros-consensus revision ec239d57f618; see testdata/README.md. They
// are the example header of cardano-ledger's
// Test.Cardano.Protocol.TPraos.Examples (exampleBlockHeader, at the
// cardano-ledger revision ouroboros-consensus pins), rebuilt as a Praos
// header for Babbage and Conway by praosHeaderBodyFromTPraos in
// Test.Consensus.Shelley.Examples.
const referenceHeaderDir = "testdata/ouroboros-consensus/ouroboros-consensus-cardano/golden/cardano/CardanoNodeToNodeVersion2"

// The example's keys (exampleKeys): the VRF key is genKeyVRF over 32 bytes
// of 0x01 (mkVRFKeyPair 1, seedFromByte), the cold key genKeyDSIGN over the
// same seed (mkDSIGNKeyPair 1), and the KES key Sum6KES genKeyKES over
// seedFromWords (RawSeed 1 0 0 0 3): Blake2b-256 of the CBOR of the tuple
// (1, 0, 0, 0, 3).
func referenceExampleKeys(t *testing.T) (vrfSeed, coldSeed, kesSeed []byte) {
	t.Helper()
	ones := bytes.Repeat([]byte{0x01}, 32)
	words, err := cbor.Encode([]uint64{1, 0, 0, 0, 3})
	require.NoError(t, err)
	sum := blake2b.Sum256(words)
	return ones, ones, sum[:]
}

// referenceSeedBytes is mkBytes n in exampleBlockHeader: Seed (mkDummyHash n),
// the Blake2b-256 of the CBOR encoding of the integer n. The example certifies
// these seeds, not ones derived from a slot and an epoch nonce.
func referenceSeedBytes(t *testing.T, n uint64) []byte {
	t.Helper()
	enc, err := cbor.Encode(n)
	require.NoError(t, err)
	sum := blake2b.Sum256(enc)
	return sum[:]
}

// referenceVRFSigner proves with the example's VRF key, but over the
// example's fixed seeds in the order BuildHeader asks: the leader
// certificate (mkBytes 1, bheaderL) first and, for TPraos, the nonce
// certificate (mkBytes 0, bheaderEta) second. A Praos header carries one
// certificate, which the reference example takes from bheaderEta.
type referenceVRFSigner struct {
	signer *SimpleVRFSigner
	seeds  [][]byte
	calls  int
}

func (s *referenceVRFSigner) Prove(_ []byte) ([]byte, []byte, error) {
	seed := s.seeds[s.calls]
	s.calls++
	return s.signer.Prove(seed)
}

func (s *referenceVRFSigner) PublicKey() []byte {
	return s.signer.PublicKey()
}

// referenceKESSigner signs with the Sum6 (depth 6) example key at period 0
// (unsoundPureSignedKES () 0 in exampleBlockHeader).
type referenceKESSigner struct {
	sk *kes.SecretKey
	pk []byte
}

func (s *referenceKESSigner) Sign(message []byte) ([]byte, error) {
	return kes.Sign(s.sk, 0, message)
}

func (s *referenceKESSigner) PublicKey() []byte { return s.pk }

func (s *referenceKESSigner) Period() uint64 { return 0 }

// referenceHeader splits a golden: the node-to-node envelope
// [era, #6.24(header)] around the header [body, KES signature].
func referenceHeader(t *testing.T, era string) (eraIndex uint64, body cbor.RawMessage, signature []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(referenceHeaderDir, "Header_"+era))
	require.NoError(t, err)
	var envelope struct {
		cbor.StructAsArray
		Era    uint64
		Header cbor.Tag
	}
	_, err = cbor.Decode(raw, &envelope)
	require.NoError(t, err)
	require.Equal(t, uint64(24), envelope.Header.Number)
	inner, ok := envelope.Header.Content.([]byte)
	require.True(t, ok, "%T", envelope.Header.Content)
	var header struct {
		cbor.StructAsArray
		Body      cbor.RawMessage
		Signature []byte
	}
	_, err = cbor.Decode(inner, &header)
	require.NoError(t, err)
	return envelope.Era, header.Body, header.Signature
}

// TestBuildHeaderMatchesReferenceGolden builds each era's example header with
// BuildHeader from the example's keys and field values, and checks the header
// body is byte for byte the golden's and, for TPraos, that the KES signature
// is the golden's. The Praos goldens reuse the TPraos example's signature
// (coerce bhSig in translatePraosHeader), which signs the TPraos body, so
// only their body is compared; the signature BuildHeader makes over the Praos
// body is checked to verify instead.
func TestBuildHeaderMatchesReferenceGolden(t *testing.T) {
	vrfSeed, coldSeed, kesSeed := referenceExampleKeys(t)
	vrfSigner, err := NewSimpleVRFSigner(vrfSeed)
	require.NoError(t, err)
	kesSK, kesPK, err := kes.KeyGen(6, kesSeed)
	require.NoError(t, err)
	coldVkey := ed25519.NewKeyFromSeed(coldSeed).Public().(ed25519.PublicKey)

	cases := []struct {
		era  string
		mode ConsensusMode
	}{
		{"Shelley", ConsensusModeTPraos},
		{"Allegra", ConsensusModeTPraos},
		{"Mary", ConsensusModeTPraos},
		{"Alonzo", ConsensusModeTPraos},
		{"Babbage", ConsensusModeCPraos},
		{"Conway", ConsensusModeCPraos},
	}
	for _, tc := range cases {
		t.Run(tc.era, func(t *testing.T) {
			_, goldenBody, goldenSig := referenceHeader(t, tc.era)
			var fields []cbor.RawMessage
			_, err := cbor.Decode(goldenBody, &fields)
			require.NoError(t, err)
			field := func(i int) []byte {
				var b []byte
				_, err := cbor.Decode(fields[i], &b)
				require.NoError(t, err)
				return b
			}

			// Fields the example fixes (exampleBlockHeader): block 3,
			// slot 9, prev hash mkDummyHash 2, body size 2345, protocol
			// version 2.0, opcert counter 0 and KES period 0. The keys
			// derived above must be the ones the golden names.
			var opcertHot, opcertSig []byte
			var bodyHash []byte
			if tc.mode == ConsensusModeTPraos {
				require.Len(t, fields, 15)
				opcertHot = field(9)
				opcertSig = field(12)
				bodyHash = field(8)
			} else {
				require.Len(t, fields, 10)
				var opcert struct {
					cbor.StructAsArray
					Hot     []byte
					Counter uint64
					Period  uint64
					Sig     []byte
				}
				_, err := cbor.Decode(fields[8], &opcert)
				require.NoError(t, err)
				opcertHot = opcert.Hot
				opcertSig = opcert.Sig
				bodyHash = field(7)
			}
			require.Equal(t, []byte(coldVkey), field(3), "cold key")
			require.Equal(t, vrfSigner.PublicKey(), field(4), "VRF key")
			require.Equal(t, kesPK, opcertHot, "KES key")
			// mkOCert signs (hot vkey, counter 0, period 0) with the cold
			// key: OCertSignable is the hot key then two big-endian
			// Word64s.
			signable := append(append([]byte{}, kesPK...), make([]byte, 16)...)
			require.True(t, ed25519.Verify(coldVkey, signable, opcertSig))

			prevHash := referenceSeedBytes(t, 2)
			require.Equal(t, prevHash, field(2))

			seeds := [][]byte{referenceSeedBytes(t, 1), referenceSeedBytes(t, 0)}
			if tc.mode == ConsensusModeCPraos {
				seeds = [][]byte{referenceSeedBytes(t, 0)}
			}
			builder := NewBlockBuilderWithMode(
				&referenceVRFSigner{signer: vrfSigner, seeds: seeds},
				&referenceKESSigner{sk: kesSK, pk: kesPK},
				&OperationalCert{
					HotVkey:        kesPK,
					SequenceNumber: 0,
					KesPeriod:      0,
					Signature:      opcertSig,
				},
				make([]byte, 28),
				coldVkey,
				// The certificates are fixed, so leadership is made near certain: the
				// whole stake at f close to 1.
				big.NewRat(999_999, 1_000_000),
				tc.mode,
			)
			header, leader, err := builder.BuildHeader(BuildHeaderInput{
				Slot:          9,
				BlockNumber:   3,
				PrevHash:      prevHash,
				EpochNonce:    make([]byte, 32),
				PoolStake:     1,
				TotalStake:    1,
				BlockBodyHash: bodyHash,
				BlockBodySize: 2345,
				ProtoMajor:    2,
				ProtoMinor:    0,
			})
			require.NoError(t, err)
			require.True(t, leader.Eligible)

			body, err := builder.serializeHeaderBody(&header.Body)
			require.NoError(t, err)
			require.Equal(
				t,
				hex.EncodeToString(goldenBody),
				hex.EncodeToString(body),
			)
			if tc.mode == ConsensusModeTPraos {
				require.Equal(
					t,
					hex.EncodeToString(goldenSig),
					hex.EncodeToString(header.Signature),
				)
			} else {
				require.True(t, kes.VerifySignedKES(kesPK, 0, body, header.Signature))
				require.False(t, kes.VerifySignedKES(kesPK, 0, body, goldenSig))
			}
		})
	}
}

// TestReferenceHeaderGoldensAreVerbatim checks the vendored goldens against
// the SHA-256 recorded from the reference checkout.
func TestReferenceHeaderGoldensAreVerbatim(t *testing.T) {
	sums, err := os.ReadFile("testdata/ouroboros-consensus.sha256")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(sums)), "\n")
	entries, err := os.ReadDir(referenceHeaderDir)
	require.NoError(t, err)
	require.Len(t, entries, len(lines))
	for _, line := range lines {
		want, path, ok := strings.Cut(line, "  ")
		require.True(t, ok, line)
		raw, err := os.ReadFile(filepath.Join("testdata/ouroboros-consensus", path))
		require.NoError(t, err)
		got := sha256.Sum256(raw)
		require.Equal(t, want, hex.EncodeToString(got[:]), path)
	}
}
