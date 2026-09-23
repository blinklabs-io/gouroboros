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

// Package ed25519byron implements the historical Ed25519 verification
// semantics used by Cardano's Byron-era ed25519-donna verifier.
package ed25519byron

import (
	"bytes"
	"crypto/sha512"

	"filippo.io/edwards25519"
)

// Verify reports whether sig is a Byron-compatible Ed25519 signature of msg
// by pubKey. Byron checks only that S's top three bits are clear, reduces S
// modulo the group order, and accepts small-order and non-canonical A/R point
// encodings. Do not use this verifier outside Byron.
func Verify(pubKey, msg, sig []byte) bool {
	if len(pubKey) != 32 || len(sig) != 64 || sig[63]&0xe0 != 0 {
		return false
	}

	publicPoint, err := new(edwards25519.Point).SetBytes(pubKey)
	if err != nil {
		return false
	}
	rPoint, err := new(edwards25519.Point).SetBytes(sig[:32])
	if err != nil {
		return false
	}

	wideS := make([]byte, 64)
	copy(wideS, sig[32:])
	scalarS, err := new(edwards25519.Scalar).SetUniformBytes(wideS)
	if err != nil {
		return false
	}

	h := sha512.New()
	_, _ = h.Write(sig[:32])
	_, _ = h.Write(pubKey)
	_, _ = h.Write(msg)
	challenge, err := new(edwards25519.Scalar).SetUniformBytes(h.Sum(nil))
	if err != nil {
		return false
	}

	left := new(edwards25519.Point).ScalarBaseMult(scalarS)
	right := new(edwards25519.Point).Add(
		rPoint,
		new(edwards25519.Point).ScalarMult(challenge, publicPoint),
	)
	return left.Equal(right) == 1 && bytes.Equal(rPoint.Bytes(), sig[:32])
}
