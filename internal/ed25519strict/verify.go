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

// Package ed25519strict provides the Ed25519 verification criteria used by the
// Cardano node for every signature outside the Byron era.
//
// The node reaches libsodium's crypto_sign_ed25519_verify_detached through
// cardano-crypto-class's Ed25519DSIGN, which rejects non-canonical point and
// scalar encodings and small-order public and R points. crypto/ed25519.Verify
// applies none of those checks: it accepts the edwards25519 identity public key
// with an all-zero signature for any message, because the verification equation
// [S]B == R + [h]A reduces to identity == identity. Accepting a proof the node
// rejects is a consensus divergence, so every boundary whose reference is
// Ed25519DSIGN must verify through Verify below.
//
// Byron is deliberately not covered. Byron signatures are verified by
// cardano-crypto-wallet's bundled ed25519-donna, whose ed25519_sign_open checks
// only the high bits of S and performs no small-order test, so it accepts
// proofs libsodium rejects. Byron blocks are immutable history; applying these
// criteria to them would reject chain the node accepts.
package ed25519strict

import (
	"bytes"
	"crypto/ed25519"

	"filippo.io/edwards25519"
)

// Verify reports whether sig is a valid signature of msg by pubKey under the
// strict criteria described in the package comment.
func Verify(pubKey, msg, sig []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize ||
		len(sig) != ed25519.SignatureSize {
		return false
	}

	publicPoint, err := new(edwards25519.Point).SetBytes(pubKey)
	if err != nil ||
		!bytes.Equal(publicPoint.Bytes(), pubKey) ||
		IsSmallOrder(publicPoint) {
		return false
	}

	rPoint, err := new(edwards25519.Point).SetBytes(sig[:32])
	if err != nil ||
		!bytes.Equal(rPoint.Bytes(), sig[:32]) ||
		IsSmallOrder(rPoint) {
		return false
	}

	if _, err := new(edwards25519.Scalar).SetCanonicalBytes(sig[32:]); err != nil {
		return false
	}

	return ed25519.Verify(pubKey, msg, sig)
}

// IsSmallOrder reports whether point lies in the small-order subgroup, which
// libsodium refuses for both the public key and R.
func IsSmallOrder(point *edwards25519.Point) bool {
	return new(edwards25519.Point).MultByCofactor(point).
		Equal(edwards25519.NewIdentityPoint()) == 1
}
