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
	"sync"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

// verifiedKeyCacheMax bounds the verified-key cache. A committee holds 900
// seats and its keys change only with pool registrations, so the cache holds
// several committees' keys; filling it empties it.
const verifiedKeyCacheMax = 8192

// verifiedKeys maps a proof domain, public key and proof of possession, as
// bytes, to the decoded public key, for keys whose proof has verified.
// Verifying a certificate checks every signer's proof of possession: a
// pairing, a hash to G1 and a G2 decompression with a subgroup check, about
// 6ms per signer, so 5.6s for a 900-signer certificate. The outcome is a
// function of the bytes it is keyed on, so an entry is never stale. Failures
// are not cached.
var verifiedKeys = struct {
	sync.RWMutex
	keys map[verifiedKeyID]*bls12381.G2Affine
}{keys: make(map[verifiedKeyID]*bls12381.G2Affine)}

// verifiedKeyID identifies a key and proof of possession under a proof
// domain. A fixed-size key keeps lookups allocation free.
type verifiedKeyID struct {
	dst       string
	publicKey [common.LeiosBlsPublicKeySize]byte
	proof     [common.LeiosBlsPossessionProofSize]byte
}

// keyCheckStage reports which check a key failed.
type keyCheckStage int

const (
	keyCheckDecode keyCheckStage = iota
	keyCheckProof
)

// verifiedPossessionKey returns key's decoded public key once its proof of
// possession verifies under dst. On failure it reports whether decoding the
// public key or verifying the proof failed, with that check's error. The
// returned point is shared and must not be modified.
func verifiedPossessionKey(
	key *common.LeiosKey,
	dst string,
) (*bls12381.G2Affine, keyCheckStage, error) {
	// Only well-formed lengths are cached; decoding rejects the others.
	cacheable := len(key.PublicKey) == common.LeiosBlsPublicKeySize &&
		len(key.PossessionProof) == common.LeiosBlsPossessionProofSize
	var id verifiedKeyID
	if cacheable {
		id.dst = dst
		copy(id.publicKey[:], key.PublicKey)
		copy(id.proof[:], key.PossessionProof)
		verifiedKeys.RLock()
		pub, ok := verifiedKeys.keys[id]
		verifiedKeys.RUnlock()
		if ok {
			return pub, 0, nil
		}
	}
	pub, err := decodePublicKey(key.PublicKey)
	if err != nil {
		return nil, keyCheckDecode, err
	}
	if err := verifySignature(
		pub,
		key.PublicKey,
		key.PossessionProof,
		dst,
	); err != nil {
		return nil, keyCheckProof, err
	}
	if !cacheable {
		return pub, 0, nil
	}
	verifiedKeys.Lock()
	if len(verifiedKeys.keys) >= verifiedKeyCacheMax {
		verifiedKeys.keys = make(map[verifiedKeyID]*bls12381.G2Affine)
	}
	verifiedKeys.keys[id] = pub
	verifiedKeys.Unlock()
	return pub, 0, nil
}
