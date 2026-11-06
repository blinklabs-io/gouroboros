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

package vrf

import (
	"bytes"
	"crypto/sha512"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/rand"
	"testing"

	"filippo.io/edwards25519"
	"filippo.io/edwards25519/field"
)

// The legacy functions below are the VRF verifier as it was before it used
// variable-time scalar multiplication and shared point encodings. They are
// the reference TestVerifyAndHashMatchesLegacy compares against.

// VerifyAndHash verifies a VRF proof and returns the hash output.
func legacyVerifyAndHash(
	publicKey []byte,
	proof []byte,
	msg []byte,
) ([]byte, error) {
	Y := &edwards25519.Point{}
	// validate key
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
	ok, err := legacyVerify(Y, proof, msg)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrProofVerificationFailed
	}
	// proof to hash
	return legacyProofToHash(proof)
}

// ProofToHash extracts the hash output from a VRF proof.
func legacyProofToHash(pi []byte) ([]byte, error) {
	var hashInput [34]byte
	var cArr, sArr [32]byte
	Gamma, err := legacyDecodeProofArrays(pi, &cArr, &sArr)
	if err != nil {
		return nil, err
	}
	// beta_string = Hash(suite_string || three_string || point_to_string(cofactor * Gamma))
	hashInput[0] = Suite
	hashInput[1] = 0x03
	Gamma.MultByCofactor(Gamma)
	copy(hashInput[2:], Gamma.Bytes())
	result := sha512.Sum512(hashInput[:])
	return result[:], nil
}

// verify performs the core VRF verification
func legacyVerify(
	Y *edwards25519.Point,
	pi []byte,
	alpha []byte,
) (bool, error) {
	var U, V *edwards25519.Point
	var tmp1, tmp2 *edwards25519.Point
	var cScalarArr, sScalarArr [32]byte

	Gamma, err := legacyDecodeProofArrays(pi, &cScalarArr, &sScalarArr)
	if err != nil {
		return false, err
	}
	H, err := legacyHashToCurveElligator2(Y, alpha)
	if err != nil {
		return false, err
	}

	// calculate U = s*B - c*Y
	tmp1 = &edwards25519.Point{}
	// SetUniformBytes needs 64 bytes while cScalarArr is only 32 bytes long (with 16 bytes of data).
	var cScalarBytes [64]byte
	copy(cScalarBytes[:], cScalarArr[:])
	c := edwards25519.NewScalar()
	// SetUniformBytes always succeeds with 64 bytes; it reduces mod L
	_, _ = c.SetUniformBytes(cScalarBytes[:])
	tmp1.ScalarMult(c, Y)
	tmp2 = (&edwards25519.Point{}).Set(tmp1)
	// SEC-9: Use SetCanonicalBytes to reject non-canonical s >= L (proof malleability)
	s := edwards25519.NewScalar()
	if _, err := s.SetCanonicalBytes(sScalarArr[:]); err != nil {
		return false, fmt.Errorf("non-canonical s scalar in VRF proof: %w", err)
	}
	tmp1.ScalarBaseMult(s)
	U = &edwards25519.Point{}
	U.Subtract(tmp1, tmp2)

	// calculate V = s*H - c*Gamma
	tmp1 = &edwards25519.Point{}
	tmp1.ScalarMult(s, H)
	tmp2 = &edwards25519.Point{}
	tmp2.ScalarMult(c, Gamma)
	V = &edwards25519.Point{}
	V.Subtract(tmp1, tmp2)

	cprime := legacyHashPoints(H, Gamma, U, V)

	cmp := subtle.ConstantTimeCompare(cScalarArr[:], cprime.Bytes())
	return cmp == 1, nil
}

// hashPoints hashes four curve points for the VRF challenge
func legacyHashPoints(P1, P2, P3, P4 *edwards25519.Point) *edwards25519.Scalar {
	var result [32]byte
	var str [2 + (32 * 4)]byte

	str[0] = Suite
	str[1] = 0x02
	copy(str[2+(32*0):], P1.Bytes())
	copy(str[2+(32*1):], P2.Bytes())
	copy(str[2+(32*2):], P3.Bytes())
	copy(str[2+(32*3):], P4.Bytes())
	sum := sha512.Sum512(str[:])
	copy(result[:], sum[:16])
	r := edwards25519.NewScalar()
	if _, err := r.SetCanonicalBytes(result[:]); err != nil {
		panic(err)
	}
	return r
}

// decodeProofArrays decodes a VRF proof into its components using pre-allocated arrays.
// c and s are output parameters that must be at least 32 bytes.
func legacyDecodeProofArrays(
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
	gamma = &edwards25519.Point{}
	if _, err := gamma.SetBytes(pi[:32]); err != nil {
		return nil, fmt.Errorf("invalid gamma encoding: %w", err)
	}
	copy(c[:], pi[32:48]) // c = pi[32:48]
	copy(s[:], pi[48:80]) // s = pi[48:80]
	return gamma, nil
}

// hashToCurveElligator2 hashes to the curve using Elligator2
func legacyHashToCurveElligator2(
	Y *edwards25519.Point,
	alpha []byte,
) (*edwards25519.Point, error) {
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

	hBytes, err := legacyGe25519FromUniform(rString)
	if err != nil {
		return nil, err
	}
	result := &edwards25519.Point{}
	if _, err := result.SetBytes(hBytes[:]); err != nil {
		return nil, fmt.Errorf(
			"invalid point encoding from Elligator2: %w",
			err,
		)
	}
	return result, nil
}

// ge25519FromUniform maps uniform bytes to a curve point
func legacyGe25519FromUniform(r []byte) ([]byte, error) {
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

	e = legacyChi25519(e)
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
		return nil, err
	}

	// multiply by the cofactor
	p3.MultByCofactor(p3)

	s = p3.Bytes()
	return s, nil
}

// chi25519 computes the Legendre symbol for field elements
func legacyChi25519(z *field.Element) *field.Element {
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

// TestVerifyAndHashMatchesLegacy checks that VerifyAndHash gives the
// previous verifier's verdict and output for valid proofs, for proofs with
// any single bit flipped, and for corrupted keys and messages.
func TestVerifyAndHashMatchesLegacy(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	cases := 300
	if testing.Short() {
		cases = 30
	}
	compared, accepted := 0, 0
	check := func(pk, proof, msg []byte) {
		t.Helper()
		got, gotErr := VerifyAndHash(pk, proof, msg)
		want, wantErr := legacyVerifyAndHash(pk, proof, msg)
		compared++
		if (gotErr == nil) != (wantErr == nil) {
			t.Fatalf(
				"pk %x proof %x msg %x: error %v, legacy %v",
				pk,
				proof,
				msg,
				gotErr,
				wantErr,
			)
		}
		if gotErr != nil {
			// The legacy verifier reports a rejected non-canonical key as a
			// failed proof; both reject.
			if !errors.Is(gotErr, ErrNonCanonicalEncoding) &&
				gotErr.Error() != wantErr.Error() {
				t.Fatalf(
					"pk %x proof %x: error %q, legacy %q",
					pk,
					proof,
					gotErr,
					wantErr,
				)
			}
			return
		}
		accepted++
		if !bytes.Equal(got, want) {
			t.Fatalf(
				"pk %x proof %x: output %x, legacy %x",
				pk,
				proof,
				got,
				want,
			)
		}
	}
	for range cases {
		seed := make([]byte, SeedSize)
		rng.Read(seed)
		pk, sk, err := KeyGen(seed)
		if err != nil {
			t.Fatal(err)
		}
		msg := make([]byte, rng.Intn(64))
		rng.Read(msg)
		proof, _, err := Prove(sk, msg)
		if err != nil {
			t.Fatal(err)
		}
		check(pk, proof, msg)
		// Every single-bit corruption of a few positions in each field.
		for _, bit := range []int{0, 7, 100, 255, 256, 300, 383, 384, 500, 639} {
			bad := bytes.Clone(proof)
			bad[bit/8] ^= 1 << (bit % 8)
			check(pk, bad, msg)
		}
		badPk := bytes.Clone(pk)
		badPk[rng.Intn(len(badPk))] ^= 1 << rng.Intn(8)
		check(badPk, proof, msg)
		check(pk, proof, append(bytes.Clone(msg), 0))
		// s >= L, and a non-canonical key encoding of a valid point.
		highS := bytes.Clone(proof)
		for i := 48; i < 80; i++ {
			highS[i] = 0xff
		}
		check(pk, highS, msg)
	}
	// Small-order and non-canonical keys.
	for _, pk := range [][]byte{
		make([]byte, 32),
		append([]byte{1}, make([]byte, 31)...),
		bytes.Repeat([]byte{0xff}, 32),
	} {
		check(pk, make([]byte, ProofSize), nil)
	}
	t.Logf("%d verifications compared, %d accepted", compared, accepted)
	if accepted < cases {
		t.Fatalf("only %d of %d valid proofs were accepted", accepted, cases)
	}
}

// TestProveMatchesLegacyProofToHash checks that Prove's output equals the
// previous ProofToHash of its proof.
func TestProveMatchesLegacyProofToHash(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for range 50 {
		seed := make([]byte, SeedSize)
		rng.Read(seed)
		_, sk, err := KeyGen(seed)
		if err != nil {
			t.Fatal(err)
		}
		msg := make([]byte, 32)
		rng.Read(msg)
		proof, output, err := Prove(sk, msg)
		if err != nil {
			t.Fatal(err)
		}
		want, err := legacyProofToHash(proof)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(output, want) {
			t.Fatalf("output %x, legacy %x", output, want)
		}
	}
}

// orderTwoPoint returns the point (0, -1), of order 2.
func orderTwoPoint(t *testing.T) *edwards25519.Point {
	t.Helper()
	enc := bytes.Repeat([]byte{0xff}, 32)
	enc[0] = 0xec
	enc[31] = 0x7f
	p, err := new(edwards25519.Point).SetBytes(enc)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// proveWithTorsion builds a proof that the legacy verifier accepts although
// the public key, Gamma, or both carry the order-2 torsion point T. It draws
// nonces until the challenge c is even, so c*T vanishes in s*B - c*Y and
// s*H - c*Gamma. It returns the public key and proof.
func proveWithTorsion(
	t *testing.T,
	rng *rand.Rand,
	alpha []byte,
	torsionOnY, torsionOnGamma bool,
) ([]byte, []byte) {
	t.Helper()
	torsion := orderTwoPoint(t)
	seed := make([]byte, SeedSize)
	rng.Read(seed)
	skHash := sha512.Sum512(seed)
	x := edwards25519.NewScalar()
	if _, err := x.SetBytesWithClamping(skHash[:32]); err != nil {
		t.Fatal(err)
	}
	Y := new(edwards25519.Point).ScalarBaseMult(x)
	if torsionOnY {
		Y.Add(Y, torsion)
	}
	H, hBytes, err := hashToCurveElligator2(Y, alpha)
	if err != nil {
		t.Fatal(err)
	}
	// H is a cofactor multiple, so x*H is the same for either key.
	Gamma := new(edwards25519.Point).ScalarMult(x, H)
	if torsionOnGamma {
		Gamma.Add(Gamma, torsion)
	}
	gammaBytes := Gamma.Bytes()
	for range 256 {
		var kBytes [64]byte
		rng.Read(kBytes[:])
		k, err := edwards25519.NewScalar().SetUniformBytes(kBytes[:])
		if err != nil {
			t.Fatal(err)
		}
		U := new(edwards25519.Point).ScalarBaseMult(k)
		V := new(edwards25519.Point).ScalarMult(k, H)
		c := hashPoints(hBytes, gammaBytes, U.Bytes(), V.Bytes())
		cBytes := c.Bytes()
		if cBytes[0]&1 != 0 {
			continue
		}
		s := edwards25519.NewScalar().Multiply(c, x)
		s.Add(s, k)
		proof := make([]byte, 0, ProofSize)
		proof = append(proof, gammaBytes...)
		proof = append(proof, cBytes[:16]...)
		proof = append(proof, s.Bytes()...)
		return Y.Bytes(), proof
	}
	t.Fatal("no even challenge in 256 nonces")
	return nil, nil
}

// TestVerifyAndHashMatchesLegacyWithTorsion checks verification of keys and
// Gammas that carry a torsion component. Negating the scalar c instead of
// the points computes (L-c)*P, which differs from -c*P by L*P and so
// rejects proofs the legacy verifier accepts.
func TestVerifyAndHashMatchesLegacyWithTorsion(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for _, tc := range []struct {
		name                   string
		torsionY, torsionGamma bool
	}{
		{"key", true, false},
		{"gamma", false, true},
		{"key and gamma", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for range 20 {
				alpha := make([]byte, 32)
				rng.Read(alpha)
				pk, proof := proveWithTorsion(
					t,
					rng,
					alpha,
					tc.torsionY,
					tc.torsionGamma,
				)
				want, err := legacyVerifyAndHash(pk, proof, alpha)
				if err != nil {
					t.Fatalf(
						"legacy verifier rejected the torsion proof: %v",
						err,
					)
				}
				got, err := VerifyAndHash(pk, proof, alpha)
				if err != nil {
					t.Fatalf(
						"VerifyAndHash rejected a proof the legacy verifier accepts: %v",
						err,
					)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("output %x, legacy %x", got, want)
				}
			}
		})
	}
}
