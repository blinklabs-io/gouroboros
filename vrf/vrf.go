// Copyright 2024 Cardano Foundation
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

// Package vrf implements ECVRF-ED25519-SHA512-Elligator2 as specified in
// IETF draft-irtf-cfrg-vrf-03, which is used in Cardano's Praos consensus
// protocol for leader election.
//
// VRF (Verifiable Random Function) provides a way to generate a
// deterministic but unpredictable output from an input, along with a
// proof that the output was correctly computed.
//
// # Cryptographic Hash Selection
//
// This implementation uses SHA-512 as mandated by the VRF specification and
// Ed25519 key derivation (RFC 8032 Section 5.1.5). SHA-512 is NOT used for
// password hashing here - it is used for:
//
//  1. Deriving Ed25519 secret scalars from seeds (per RFC 8032)
//  2. Generating deterministic nonces for Schnorr proofs
//  3. Computing VRF output hashes
//
// SHA-512 is cryptographically appropriate for these uses because:
//   - It provides the required 512-bit output for Ed25519 scalar derivation
//   - It is the algorithm specified by IETF CFRG for this VRF suite
//   - Protocol interoperability requires exact algorithm matching
//
// Using password-specific algorithms (bcrypt, argon2) would break protocol
// compatibility and is not appropriate for elliptic curve cryptography.
//
// # References
//
//   - IETF draft-irtf-cfrg-vrf-03: https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-vrf-03
//   - RFC 8032 (Ed25519): https://datatracker.ietf.org/doc/html/rfc8032#section-5.1.5
//   - Cardano Praos: https://iohk.io/en/research/library/papers/ouroboros-praos/
//   - Reference impl: https://github.com/IntersectMBO/cardano-base/tree/master/cardano-crypto-praos
package vrf

import (
	"crypto/sha512"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"

	"filippo.io/edwards25519"
	"filippo.io/edwards25519/field"
	"github.com/blinklabs-io/gouroboros/internal/cryptoutil"
	"golang.org/x/crypto/blake2b"
)

const (
	// Suite is the VRF suite identifier for ECVRF-ED25519-SHA512-Elligator2
	Suite = 0x04

	// ProofSize is the size of a VRF proof in bytes
	ProofSize = 80

	// OutputSize is the size of a VRF output in bytes
	OutputSize = 64

	// SeedSize is the size of a VRF seed/secret key in bytes
	SeedSize = 32

	// PublicKeySize is the size of a VRF public key in bytes
	PublicKeySize = 32
)

// ErrProofVerificationFailed is returned when a VRF proof fails
// cryptographic verification against the provided message. It allows
// callers to distinguish an invalid proof from malformed inputs.
var ErrProofVerificationFailed = errors.New("issue verifying proof")

// ErrNonCanonicalEncoding is returned when a public key or proof gamma encodes
// a field element y that is not reduced modulo p = 2^255 - 19.
var ErrNonCanonicalEncoding = errors.New("non-canonical point encoding")

// KeyGen generates a new VRF keypair from a 32-byte seed.
// Returns (publicKey, secretKey) where secretKey is the original seed.
//
// The seed is a cryptographic random value, NOT a user password.
// SHA-512 is the correct algorithm per RFC 8032 Section 5.1.5.
func KeyGen(seed []byte) ([]byte, []byte, error) {
	if len(seed) != SeedSize {
		return nil, nil, errors.New("seed must be 32 bytes")
	}

	// Derive secret scalar x from SHA512(sk)[0:32] with clamping.
	// SHA-512 is mandated by RFC 8032 for Ed25519 key derivation.
	// This is NOT password hashing - it's elliptic curve scalar derivation.
	// #nosec G401 -- SHA-512 is cryptographically required by RFC 8032/VRF spec
	h := sha512.Sum512(seed)
	defer cryptoutil.Wipe(h[:])
	xScalar := edwards25519.NewScalar()
	defer cryptoutil.WipeScalar(xScalar)
	if _, err := xScalar.SetBytesWithClamping(h[:32]); err != nil {
		return nil, nil, err
	}

	// Compute public key Y = x * B
	Y := (&edwards25519.Point{}).ScalarBaseMult(xScalar)
	publicKey := Y.Bytes()

	// Secret key is the original seed
	secretKey := make([]byte, SeedSize)
	copy(secretKey, seed)

	return publicKey, secretKey, nil
}

// Prove generates a VRF proof for the given secret key and input.
// Returns (proof, output) where proof is 80 bytes and output is 64 bytes.
//
// The secretKey is a 32-byte cryptographic seed, NOT a user password.
// SHA-512 is the correct algorithm per IETF draft-irtf-cfrg-vrf-03.
func Prove(secretKey []byte, alpha []byte) ([]byte, []byte, error) {
	if len(secretKey) != SeedSize {
		return nil, nil, errors.New("secret key must be 32 bytes")
	}

	// Step 1: Derive secret scalar x from SHA512(sk)[0:32] with clamping.
	// SHA-512 is mandated by RFC 8032 for Ed25519 and by the VRF spec.
	// This is NOT password hashing - it's elliptic curve scalar derivation.
	// #nosec G401 -- SHA-512 is cryptographically required by RFC 8032/VRF spec
	skHash := sha512.Sum512(secretKey)
	defer cryptoutil.Wipe(skHash[:])

	// Step 2: Compute public key Y = x * B
	xScalar := edwards25519.NewScalar()
	defer cryptoutil.WipeScalar(xScalar)
	if _, err := xScalar.SetBytesWithClamping(skHash[:32]); err != nil {
		return nil, nil, err
	}
	Y := (&edwards25519.Point{}).ScalarBaseMult(xScalar)

	// Step 3: Hash to curve using Elligator2
	H, hBytes, err := hashToCurveElligator2(Y, alpha)
	if err != nil {
		return nil, nil, err
	}

	// Step 4: Compute Gamma = x * H
	Gamma := (&edwards25519.Point{}).ScalarMult(xScalar, H)

	// Step 5: Generate nonce k from SHA512(SHA512(sk)[32:64] || H_bytes) mod ORDER
	var nonceInput [64]byte
	copy(nonceInput[:32], skHash[32:64])
	copy(nonceInput[32:], hBytes)

	defer cryptoutil.Wipe(nonceInput[:])

	nonceHash := sha512.Sum512(nonceInput[:])
	defer cryptoutil.Wipe(nonceHash[:])
	// SetUniformBytes takes a 64-byte slice and reduces mod L automatically
	kScalar := edwards25519.NewScalar()
	defer cryptoutil.WipeScalar(kScalar)
	if _, err := kScalar.SetUniformBytes(nonceHash[:]); err != nil {
		return nil, nil, err
	}

	// Step 6: Compute U = k * B and V = k * H
	U := (&edwards25519.Point{}).ScalarBaseMult(kScalar)
	V := (&edwards25519.Point{}).ScalarMult(kScalar, H)

	// Step 7: Hash points to get challenge c
	encoded := encodePoints(Gamma, U, V)
	cScalar := hashPoints(hBytes, encoded[0][:], encoded[1][:], encoded[2][:])
	cBytes := cScalar.Bytes()

	// Step 8: Compute s = (k + c * x) mod ORDER
	// Avoid MultiplyAdd's internal copy of the nonce scalar.
	sScalar := edwards25519.NewScalar()
	sScalar.Multiply(cScalar, xScalar).Add(sScalar, kScalar)
	sBytes := sScalar.Bytes()

	// Step 9: Encode proof: Gamma_bytes (32) || c_bytes (16) || s_bytes (32) = 80 bytes
	var proof [ProofSize]byte
	copy(proof[0:32], encoded[0][:])
	copy(proof[32:48], cBytes[:16]) // c is only 16 bytes in the proof
	copy(proof[48:80], sBytes)

	// Step 10: Get output using proof to hash
	output := gammaToHash(Gamma)

	// Return slices for API compatibility
	proofSlice := make([]byte, ProofSize)
	copy(proofSlice, proof[:])
	return proofSlice, output, nil
}

// VerifyAndHash verifies a VRF proof and returns the hash output.
func VerifyAndHash(publicKey []byte, proof []byte, msg []byte) ([]byte, error) {
	Y := &edwards25519.Point{}
	// validate key
	if !isCanonicalY(publicKey) {
		return nil, ErrNonCanonicalEncoding
	}
	if _, err := Y.SetBytes(publicKey); err != nil {
		return nil, err
	}
	isSmallOrder := (&edwards25519.Point{}).MultByCofactor(Y).
		Equal(edwards25519.NewIdentityPoint()) ==
		1
	if isSmallOrder {
		return nil, errors.New("public key is a small order point")
	}
	// vrf_verify
	output, ok, err := verify(Y, proof, msg)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrProofVerificationFailed
	}
	return output, nil
}

// Verify verifies a VRF proof and compares the output to expected.
func Verify(
	vrfKey []byte,
	proof []byte,
	expectedOutput []byte,
	msg []byte,
) (bool, error) {
	output, err := VerifyAndHash(vrfKey, proof, msg)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(output, expectedOutput) == 1, nil
}

// MkInputVrf creates a VRF input from a slot and epoch nonce (eta0).
// This is used in Cardano for leader election.
func MkInputVrf(slot int64, eta0 []byte) ([]byte, error) {
	if len(eta0) != 32 {
		return nil, fmt.Errorf("eta0 must be 32 bytes, got %d", len(eta0))
	}
	concat := make([]byte, 8+32)
	binary.BigEndian.PutUint64(concat[:8], uint64(slot)) // #nosec G115
	copy(concat[8:], eta0)
	h, err := blake2b.New(32, nil)
	if err != nil {
		return nil, fmt.Errorf(
			"unexpected error creating empty blake2b hash: %w",
			err,
		)
	}
	h.Write(concat)
	result := h.Sum(nil)
	// blake2b.Sum always returns non-nil, but nilaway needs reassurance
	if result == nil {
		return nil, errors.New("blake2b.Sum returned nil")
	}
	return result, nil
}

// mkNonceFromNumber computes blake2b-256 of a uint64 in big-endian encoding.
// This matches Haskell's mkNonceFromNumber from Cardano.Ledger.BaseTypes.
func mkNonceFromNumber(n uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n)
	h, err := blake2b.New(32, nil)
	if err != nil {
		panic(fmt.Sprintf(
			"unexpected error creating empty blake2b hash: %s", err,
		))
	}
	h.Write(buf)
	result := h.Sum(nil)
	if result == nil {
		panic("blake2b.Sum returned nil")
	}
	return result
}

// seedEta is the TPraos nonce VRF seed constant: blake2b-256(uint64be(0)).
// Used with NonceVrf (bheaderEta) for epoch nonce contribution.
// Matches Haskell: seedEta = mkNonceFromNumber 0
var seedEta = mkNonceFromNumber(0)

// seedL is the TPraos leader VRF seed constant: blake2b-256(uint64be(1)).
// Used with LeaderVrf (bheaderL) for leader eligibility checks.
// Matches Haskell: seedL = mkNonceFromNumber 1
var seedL = mkNonceFromNumber(1)

// SeedEta returns a copy of the TPraos nonce VRF seed constant.
func SeedEta() []byte {
	out := make([]byte, len(seedEta))
	copy(out, seedEta)
	return out
}

// SeedL returns a copy of the TPraos leader VRF seed constant.
func SeedL() []byte {
	out := make([]byte, len(seedL))
	copy(out, seedL)
	return out
}

// MkSeedTPraos constructs the TPraos VRF input used in Shelley through
// Alonzo eras. The TPraos construction computes the same base hash as
// MkInputVrf, then XORs it with a seed constant:
//
//	result = blake2b-256(slot_be64 || eta0) XOR seedConstant
//
// For the leader VRF check, pass SeedL() as seedConstant.
// For the nonce VRF contribution, pass SeedEta() as seedConstant.
//
// This matches Haskell's mkSeed from Cardano.Protocol.TPraos.BHeader:
//
//	mkSeed ucNonce slot eNonce =
//	    Hash.xor(ucNonce) . Hash.hashWith id $ slot <> eNonce
//
// IMPORTANT: eta0 must be exactly 32 bytes and seedConstant must be
// exactly 32 bytes.
func MkSeedTPraos(
	slot int64,
	eta0 []byte,
	seedConstant []byte,
) ([]byte, error) {
	if len(seedConstant) != 32 {
		return nil, fmt.Errorf(
			"seedConstant must be 32 bytes, got %d", len(seedConstant),
		)
	}
	base, err := MkInputVrf(slot, eta0)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 32)
	for i := range result {
		result[i] = base[i] ^ seedConstant[i]
	}
	return result, nil
}

// ProofToHash extracts the hash output from a VRF proof.
func ProofToHash(pi []byte) ([]byte, error) {
	var cArr, sArr [32]byte
	Gamma, err := decodeProofArrays(pi, &cArr, &sArr)
	if err != nil {
		return nil, err
	}
	return gammaToHash(Gamma), nil
}

// gammaToHash returns beta_string = Hash(suite_string || three_string ||
// point_to_string(cofactor * Gamma)).
func gammaToHash(Gamma *edwards25519.Point) []byte {
	cofactorGamma := (&edwards25519.Point{}).MultByCofactor(Gamma)
	return cofactorGammaToHash(cofactorGamma.Bytes())
}

// cofactorGammaToHash is gammaToHash given the encoding of cofactor * Gamma.
func cofactorGammaToHash(cofactorGamma []byte) []byte {
	var hashInput [34]byte
	hashInput[0] = Suite
	hashInput[1] = 0x03
	copy(hashInput[2:], cofactorGamma)
	result := sha512.Sum512(hashInput[:])
	return result[:]
}

// verify performs the core VRF verification and returns the proof's output
// hash along with the outcome.
//
// Every input is public, so the scalar multiplications use the variable-time
// algorithms: U = s*B - c*Y is one double-scalar multiplication with the base
// point, and V = s*H - c*Gamma one two-term multi-scalar multiplication. They
// compute the same points as separate constant-time multiplications would.
func verify(
	Y *edwards25519.Point,
	pi []byte,
	alpha []byte,
) ([]byte, bool, error) {
	var cScalarArr, sScalarArr [32]byte

	Gamma, err := decodeProofArrays(pi, &cScalarArr, &sScalarArr)
	if err != nil {
		return nil, false, err
	}
	H, hBytes, err := hashToCurveElligator2(Y, alpha)
	if err != nil {
		return nil, false, err
	}

	// SetUniformBytes needs 64 bytes while cScalarArr is only 32 bytes long (with 16 bytes of data).
	var cScalarBytes [64]byte
	copy(cScalarBytes[:], cScalarArr[:])
	c := edwards25519.NewScalar()
	// SetUniformBytes always succeeds with 64 bytes; it reduces mod L
	_, _ = c.SetUniformBytes(cScalarBytes[:])
	// SEC-9: Use SetCanonicalBytes to reject non-canonical s >= L (proof malleability)
	s := edwards25519.NewScalar()
	if _, err := s.SetCanonicalBytes(sScalarArr[:]); err != nil {
		return nil, false, fmt.Errorf(
			"non-canonical s scalar in VRF proof: %w",
			err,
		)
	}

	// Negate the points rather than c: (L-c)*P equals -c*P only for
	// prime-order P, and Y and Gamma may carry a torsion component.
	// U = s*B - c*Y
	negY := (&edwards25519.Point{}).Negate(Y)
	U := (&edwards25519.Point{}).VarTimeDoubleScalarBaseMult(c, negY, s)
	// V = s*H - c*Gamma
	negGamma := (&edwards25519.Point{}).Negate(Gamma)
	V := (&edwards25519.Point{}).VarTimeMultiScalarMult(
		[]*edwards25519.Scalar{s, c},
		[]*edwards25519.Point{H, negGamma},
	)

	cofactorGamma := (&edwards25519.Point{}).MultByCofactor(Gamma)
	encoded := encodePoints(Gamma, U, V, cofactorGamma)
	cprime := hashPoints(hBytes, encoded[0][:], encoded[1][:], encoded[2][:])

	cmp := subtle.ConstantTimeCompare(cScalarArr[:], cprime.Bytes())
	return cofactorGammaToHash(encoded[3][:]), cmp == 1, nil
}

// hashPoints hashes the encodings of four curve points for the VRF
// challenge.
func hashPoints(P1, P2, P3, P4 []byte) *edwards25519.Scalar {
	var result [32]byte
	var str [2 + (32 * 4)]byte

	str[0] = Suite
	str[1] = 0x02
	copy(str[2+(32*0):], P1)
	copy(str[2+(32*1):], P2)
	copy(str[2+(32*2):], P3)
	copy(str[2+(32*3):], P4)
	sum := sha512.Sum512(str[:])
	copy(result[:], sum[:16])
	r := edwards25519.NewScalar()
	if _, err := r.SetCanonicalBytes(result[:]); err != nil {
		panic(err)
	}
	return r
}

// encodePoints returns the canonical encodings of points, the bytes
// (*edwards25519.Point).Bytes returns, with one field inversion for all of
// them rather than one each (Montgomery's batch inversion).
func encodePoints(points ...*edwards25519.Point) [][32]byte {
	zs := make([]field.Element, len(points))
	xs := make([]field.Element, len(points))
	ys := make([]field.Element, len(points))
	// prefix[i] is the product of the Z coordinates of points[:i+1].
	prefix := make([]field.Element, len(points))
	for i, p := range points {
		X, Y, Z, _ := p.ExtendedCoordinates()
		xs[i].Set(X)
		ys[i].Set(Y)
		zs[i].Set(Z)
		if i == 0 {
			prefix[i].Set(Z)
		} else {
			prefix[i].Multiply(&prefix[i-1], Z)
		}
	}
	var inv field.Element
	inv.Invert(&prefix[len(points)-1])
	out := make([][32]byte, len(points))
	for i := len(points) - 1; i >= 0; i-- {
		// inv is the inverse of prefix[i]; Z_i^-1 = inv * prefix[i-1].
		var zInv field.Element
		if i == 0 {
			zInv.Set(&inv)
		} else {
			zInv.Multiply(&inv, &prefix[i-1])
			inv.Multiply(&inv, &zs[i])
		}
		var x, y field.Element
		x.Multiply(&xs[i], &zInv)
		y.Multiply(&ys[i], &zInv)
		copy(out[i][:], y.Bytes())
		// #nosec G115 -- IsNegative returns 0 or 1
		out[i][31] |= byte(x.IsNegative() << 7)
	}
	return out
}

// decodeProofArrays decodes a VRF proof into its components using pre-allocated arrays.
// c and s are output parameters that must be at least 32 bytes.
func decodeProofArrays(
	pi []byte,
	c *[32]byte,
	s *[32]byte,
) (gamma *edwards25519.Point, err error) {
	if len(pi) != ProofSize {
		return nil, fmt.Errorf(
			"unexpected length of pi (must be %d)",
			ProofSize,
		)
	}
	if !isCanonicalY(pi[:32]) {
		return nil, fmt.Errorf(
			"invalid gamma encoding: %w",
			ErrNonCanonicalEncoding,
		)
	}
	gamma = &edwards25519.Point{}
	if _, err := gamma.SetBytes(pi[:32]); err != nil {
		return nil, fmt.Errorf("invalid gamma encoding: %w", err)
	}
	copy(c[:], pi[32:48]) // c = pi[32:48]
	copy(s[:], pi[48:80]) // s = pi[48:80]
	return gamma, nil
}

// isCanonicalY reports whether the y coordinate of a 32-byte point encoding is
// below p. SetBytes silently reduces y >= p, so the check must precede it. The
// sign bit is ignored; only y in [p, p+19) has a second encoding. An encoding
// of any other length is not judged here: SetBytes rejects it with its own
// length error.
func isCanonicalY(enc []byte) bool {
	if len(enc) != 32 || enc[31]&0x7f != 0x7f || enc[0] < 0xed {
		return true
	}
	for _, b := range enc[1:31] {
		if b != 0xff {
			return true
		}
	}
	return false
}

// hashToCurveElligator2 hashes to the curve using Elligator2 and returns the
// point with its canonical encoding.
func hashToCurveElligator2(
	Y *edwards25519.Point,
	alpha []byte,
) (*edwards25519.Point, []byte, error) {
	hs := sha512.New()

	hs.Write([]byte{Suite})
	hs.Write([]byte{1})
	hs.Write(Y.Bytes())
	hs.Write(alpha)
	rString := hs.Sum(nil)
	// sha512.Sum always returns non-nil, but nilaway needs reassurance
	if rString == nil {
		panic("sha512.Sum returned nil")
	}
	rString[31] &= 0x7f // clear sign bit

	return ge25519FromUniform(rString)
}

// ge25519FromUniform maps uniform bytes to a curve point.
// It returns the point together with its canonical encoding, so a caller that
// needs the point does not decode the encoding again.
func ge25519FromUniform(r []byte) (*edwards25519.Point, []byte, error) {
	s := make([]byte, 32)
	var e, negx, rr2, x, x2, x3 *field.Element
	var p3 *edwards25519.Point
	var eIsMinus1 int
	var xSign byte

	one := new(field.Element).One()
	copy(s, r)
	xSign = s[31] & 0x80
	s[31] &= 0x7f

	rr2 = &field.Element{}
	// SetBytes always succeeds; it reduces any 32 bytes mod p
	_, _ = rr2.SetBytes(s)

	// elligator
	rr2.Square(rr2)
	rr2.Add(rr2, rr2)
	rr2.Add(rr2, one)
	rr2.Invert(rr2)

	x = &field.Element{}

	const curve25519A = 486662
	curve25519AElement := new(field.Element).Mult32(one, curve25519A)

	x.Mult32(rr2, curve25519A)
	x.Negate(x)

	x2 = &field.Element{}
	x2.Multiply(x, x)
	x3 = &field.Element{}
	x3.Multiply(x, x2)

	e = &field.Element{}
	e.Add(x3, x)
	x2.Mult32(x2, curve25519A)
	e.Add(x2, e)

	e = chi25519(e)
	s = e.Bytes()

	eIsMinus1 = int(s[1] & 1)
	eIsNotMinus1 := eIsMinus1 ^ 1
	negx = new(field.Element).Negate(x)
	x.Select(x, negx, eIsNotMinus1)
	x2.Zero()
	x2.Select(x2, curve25519AElement, eIsNotMinus1)
	x.Subtract(x, x2)
	// yed = (x-1)/(x+1)
	{
		var one, xPlusOne, xPlusOneInv, xMinusOne, yed *field.Element

		one = (&field.Element{}).One()
		xPlusOne = (&field.Element{}).Add(x, one)
		xMinusOne = (&field.Element{}).Subtract(x, one)
		xPlusOneInv = (&field.Element{}).Invert(xPlusOne)
		yed = (&field.Element{}).Multiply(xMinusOne, xPlusOneInv)
		s = yed.Bytes()
	}

	// recover x
	s[31] |= xSign

	p3 = &edwards25519.Point{}
	_, err := p3.SetBytes(s)
	if err != nil {
		return nil, nil, err
	}

	// multiply by the cofactor
	p3.MultByCofactor(p3)

	s = p3.Bytes()
	return p3, s, nil
}

// chi25519 computes the Legendre symbol for field elements
func chi25519(z *field.Element) *field.Element {
	out := &field.Element{}

	var t0, t1, t2, t3 *field.Element
	var i int

	t0 = &field.Element{}
	t1 = &field.Element{}
	t2 = &field.Element{}
	t3 = &field.Element{}

	t0.Square(z)
	t1.Multiply(t0, z)
	t0.Square(t1)
	t2.Square(t0)
	t2.Square(t2)
	t2.Multiply(t2, t0)
	t1.Multiply(t2, z)
	t2.Square(t1)

	for i = 1; i < 5; i++ {
		t2.Square(t2)
	}
	t1.Multiply(t2, t1)
	t2.Square(t1)
	for i = 1; i < 10; i++ {
		t2.Square(t2)
	}
	t2.Multiply(t2, t1)
	t3.Square(t2)
	for i = 1; i < 20; i++ {
		t3.Square(t3)
	}
	t2.Multiply(t3, t2)
	t2.Square(t2)
	for i = 1; i < 10; i++ {
		t2.Square(t2)
	}
	t1.Multiply(t2, t1)
	t2.Square(t1)

	for i = 1; i < 50; i++ {
		t2.Square(t2)
	}
	t2.Multiply(t2, t1)
	t3.Square(t2)
	for i = 1; i < 100; i++ {
		t3.Square(t3)
	}
	t2.Multiply(t3, t2)
	t2.Square(t2)
	for i = 1; i < 50; i++ {
		t2.Square(t2)
	}
	t1.Multiply(t2, t1)
	t1.Square(t1)
	for i = 1; i < 4; i++ {
		t1.Square(t1)
	}
	out.Multiply(t1, t0)

	return out
}
