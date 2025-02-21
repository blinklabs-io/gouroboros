// Copyright 2024 Cardano Foundation
// Copyright 2025 Blink Labs Software
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

// This file is taken almost verbatim (including comments) from
// https://github.com/cardano-foundation/cardano-ibc-incubator

package ledger

import (
	"crypto/sha512"
	"crypto/subtle"
	"encoding/binary"
	"fmt"

	"filippo.io/edwards25519"
	"filippo.io/edwards25519/field"
	"golang.org/x/crypto/blake2b"
)

// This module inspired by https://github.com/input-output-hk/vrf

const (
	VRF_SUITE = 0x04 // ECVRF-ED25519-SHA512-Elligator2
)

func MkInputVrf(slot int64, eta0 []byte) []byte {
	// Ref: https://github.com/IntersectMBO/ouroboros-consensus/blob/de74882102236fdc4dd25aaa2552e8b3e208448c/ouroboros-consensus-protocol/src/ouroboros-consensus-protocol/Ouroboros/Consensus/Protocol/Praos/VRF.hs#L60
	concat := make([]byte, 8+32)
	binary.BigEndian.PutUint64(concat[:8], uint64(slot)) // #nosec G115
	copy(concat[8:], eta0)
	h, err := blake2b.New(32, nil)
	if err != nil {
		panic(
			fmt.Sprintf(
				"unexpected error creating empty blake2b hash: %s",
				err,
			),
		)
	}
	h.Write(concat)
	return h.Sum(nil)
}

func VrfVerifyAndHash(
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
		return nil, fmt.Errorf("public key is a small order point")
	}
	// vrf_verify
	ok, err := vrfVerify(Y, proof, msg)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("issue verifying proof")
	}
	// proof to hash
	return cryptoVrfIetfdraft03ProofToHash(proof)
}

func vrfVerify(Y *edwards25519.Point, pi []byte, alpha []byte) (bool, error) {
	var U, V *edwards25519.Point // ge25519_p3     H_point, Gamma_point, U_point, V_point, tmp_p3_point;
	var tmp1, tmp2 *edwards25519.Point

	Gamma, cScalar, sScalar, err := vrfIetfdraft03DecodeProof(
		pi,
	) // _vrf_ietfdraft03_decode_proof(&Gamma_point, c_scalar, s_scalar, pi) != 0
	if err != nil {
		return false, err
	}
	H, err := vrfHashToCurveElligator225519(Y, alpha)
	if err != nil {
		return false, err
	}

	// // calculate U = s*B - c*Y
	tmp1 = &edwards25519.Point{}
	// SetBytes needs 32 bytes while c_scalar is only 16 bytes long.
	cScalarBytes := make([]byte, 64)
	copy(cScalarBytes, cScalar)
	c := edwards25519.NewScalar()
	_, _ = c.SetUniformBytes(cScalarBytes)
	tmp1.ScalarMult(c, Y)
	tmp2 = (&edwards25519.Point{}).Set(tmp1)
	s := edwards25519.NewScalar()
	sScalarBytes := make([]byte, 64)
	copy(sScalarBytes, sScalar)
	_, _ = s.SetUniformBytes(sScalarBytes)
	tmp1.ScalarBaseMult(s)
	U = &edwards25519.Point{}
	U.Subtract(tmp1, tmp2)

	// // calculate V = s*H -  c*Gamma
	tmp1 = &edwards25519.Point{}
	tmp1.ScalarMult(s, H)
	tmp2 = &edwards25519.Point{}
	tmp2.ScalarMult(c, Gamma)
	V = &edwards25519.Point{}
	V.Subtract(tmp1, tmp2)

	cprime := vrfHashPoints(
		H,
		Gamma,
		U,
		V,
	) // _vrf_ietfdraft03_hash_points(cprime, &H_point, &Gamma_point, &U_point, &V_point);

	cmp := subtle.ConstantTimeCompare(
		cScalar[:],
		cprime.Bytes(),
	) // return crypto_verify_16(c_scalar, cprime);
	return cmp == 1, nil
}

func vrfHashPoints(P1, P2, P3, P4 *edwards25519.Point) *edwards25519.Scalar {
	result := make([]byte, 32)
	var str [2 + (32 * 4)]byte

	str[0] = VRF_SUITE
	str[1] = 0x02
	copy(str[2+(32*0):], P1.Bytes())
	copy(str[2+(32*1):], P2.Bytes())
	copy(str[2+(32*2):], P3.Bytes())
	copy(str[2+(32*3):], P4.Bytes())
	h := sha512.New()
	h.Write(str[:])
	sum := h.Sum(nil)

	copy(result[:], sum[:16])
	r := edwards25519.NewScalar()
	if _, err := r.SetCanonicalBytes(result); err != nil {
		panic(err)
	}
	return r

}

func cryptoVrfIetfdraft03ProofToHash(pi []byte) ([]byte, error) {
	var hashInput [34]byte // unsigned char hash_input[2+32];
	Gamma, _, _, err := vrfIetfdraft03DecodeProof(pi)
	if err != nil {
		return nil, err
	}
	// beta_string = Hash(suite_string || three_string || point_to_string(cofactor * Gamma))
	hashInput[0] = VRF_SUITE
	hashInput[1] = 0x03
	Gamma.MultByCofactor(Gamma)
	copy(hashInput[2:], Gamma.Bytes())
	h := sha512.New()
	h.Write(hashInput[:])
	return h.Sum(nil), nil
}

func vrfIetfdraft03DecodeProof(
	pi []byte,
) (gamma *edwards25519.Point, c []byte, s []byte, err error) {
	if len(pi) != 80 {
		return nil, nil, nil, fmt.Errorf("unexpected length of pi (must be 80)")
	}
	/* gamma = decode_point(pi[0:32]) */
	gamma = &edwards25519.Point{}
	_, _ = gamma.SetBytes(pi[:32])
	c = make([]byte, 32)
	s = make([]byte, 32)
	copy(c[:], pi[32:48]) // c = pi[32:48]
	copy(s[:], pi[48:80]) // s = pi[48:80]
	return gamma, c, s, nil
}

func vrfHashToCurveElligator225519(
	Y *edwards25519.Point,
	alpha []byte,
) (*edwards25519.Point, error) {
	hs := sha512.New()

	hs.Write([]byte{VRF_SUITE})
	hs.Write([]byte{1})
	hs.Write(Y.Bytes())
	hs.Write(alpha)
	rString := hs.Sum(nil)
	rString[31] &= 0x7f // clear sign bit

	hBytes, err := ge25519FromUniform(rString)
	if err != nil {
		return nil, err
	}
	result := &edwards25519.Point{}
	_, _ = result.SetBytes(hBytes[:]) // ge25519_frombytes(&H_point, h_string);
	return result, nil
}
func ge25519FromUniform(r []byte) ([]byte, error) {
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
	_, _ = rr2.SetBytes(s) // fe25519_frombytes(rr2, s);

	// elligator
	rr2.Square(rr2) // fe25519_sq2(rr2, rr2);
	rr2.Add(rr2, rr2)
	rr2.Add(rr2, one)
	rr2.Invert(rr2) // fe25519_invert(rr2, rr2);

	x = &field.Element{}

	const curve25519A = 486662
	curve25519AElement := new(field.Element).Mult32(one, curve25519A)

	x.Mult32(rr2, curve25519A) // fe25519_mul(x, curve25519_A, rr2);
	x.Negate(x)                // fe25519_neg(x, x);

	x2 = &field.Element{}
	x2.Multiply(x, x) // fe25519_sq(x2, x);
	x3 = &field.Element{}
	x3.Multiply(x, x2) // fe25519_mul(x3, x, x2);

	e = &field.Element{}
	e.Add(x3, x)               // fe25519_add(e, x3, x);
	x2.Mult32(x2, curve25519A) // fe25519_mul(x2, x2, curve25519_A);
	e.Add(x2, e)               // fe25519_add(e, x2, e);

	e = chi25519(e) // chi25519(e, e);
	s = e.Bytes()   // fe25519_tobytes(s, e);

	eIsMinus1 = int(s[1] & 1) // e_is_minus_1 = s[1] & 1;
	eIsNotMinus1 := eIsMinus1 ^ 1
	negx = new(field.Element).Negate(x) // fe25519_neg(negx, x);
	x.Select(
		x,
		negx,
		eIsNotMinus1,
	) // fe25519_cmov(x, negx, e_is_minus_1);
	x2.Zero() // fe25519_0(x2);
	x2.Select(
		x2,
		curve25519AElement,
		eIsNotMinus1,
	) // fe25519_cmov(x2, curve25519_A, e_is_minus_1);
	x.Subtract(x, x2) // fe25519_sub(x, x, x2);
	// yed = (x-1)/(x+1)
	{
		var one, xPlusOne, xPlusOneInv, xMinusOne, yed *field.Element

		one = (&field.Element{}).One() // fe25519_1(one);
		xPlusOne = (&field.Element{}).Add(
			x,
			one,
		) // fe25519_add(x_plus_one, x, one);
		xMinusOne = (&field.Element{}).Subtract(
			x,
			one,
		) // fe25519_sub(x_minus_one, x, one);
		xPlusOneInv = (&field.Element{}).Invert(
			xPlusOne,
		) // fe25519_invert(x_plus_one_inv, x_plus_one);
		yed = (&field.Element{}).Multiply(
			xMinusOne,
			xPlusOneInv,
		) // fe25519_mul(yed, x_minus_one, x_plus_one_inv);
		s = yed.Bytes() // fe25519_tobytes(s, yed);
	}

	// recover x
	s[31] |= xSign

	p3 = &edwards25519.Point{}
	_, err := p3.SetBytes(s) // ge25519_frombytes(&p3, s) != 0
	if err != nil {
		// fmt.Printf("issue setting bytes: %x - %v\n", s, err)
		return nil, err
	}

	// // multiply by the cofactor
	p3.MultByCofactor(p3)

	s = p3.Bytes() // ge25519_p3_tobytes(s, &p3);
	return s, nil
}

func chi25519(z *field.Element) *field.Element {
	out := &field.Element{}
	out.Set(z)

	var t0, t1, t2, t3 *field.Element // fe25519 t0, t1, t2, t3;
	var i int                         // int     i;

	t0 = &field.Element{}
	t1 = &field.Element{}
	t2 = &field.Element{}
	t3 = &field.Element{}

	t0.Square(z)        // fe25519_sq(t0, z);
	t1.Multiply(t0, z)  // fe25519_mul(t1, t0, z);
	t0.Square(t1)       // fe25519_sq(t0, t1);
	t2.Square(t0)       // fe25519_sq(t2, t0);
	t2.Square(t2)       // fe25519_sq(t2, t2);
	t2.Multiply(t2, t0) // fe25519_mul(t2, t2, t0);
	t1.Multiply(t2, z)  // fe25519_mul(t1, t2, z);
	t2.Square(t1)       // fe25519_sq(t2, t1);

	for i = 1; i < 5; i++ {
		t2.Square(t2) // fe25519_sq(t2, t2);
	}
	t1.Multiply(t2, t1) // fe25519_mul(t1, t2, t1);
	t2.Square(t1)       // fe25519_sq(t2, t1);
	for i = 1; i < 10; i++ {
		t2.Square(t2) //     fe25519_sq(t2, t2);
	}
	t2.Multiply(t2, t1) // fe25519_mul(t2, t2, t1);
	t3.Square(t2)       // fe25519_sq(t3, t2);
	// fmt.Printf("g t2b: %x\n", t2.Bytes())
	// fmt.Printf("g t3b: %x\n", t3.Bytes())
	for i = 1; i < 20; i++ {
		t3.Square(t3) //     fe25519_sq(t3, t3);
	}
	t2.Multiply(t3, t2) // fe25519_mul(t2, t3, t2);
	t2.Square(t2)       // fe25519_sq(t2, t2);
	for i = 1; i < 10; i++ {
		t2.Square(t2) //     fe25519_sq(t2, t2);
	}
	t1.Multiply(t2, t1) // fe25519_mul(t1, t2, t1);
	t2.Square(t1)       // fe25519_sq(t2, t1);
	// fmt.Printf("g t2c: %x\n", t2.Bytes())
	// fmt.Printf("g t3c: %x\n", t3.Bytes())
	// fmt.Printf("g t1 50: %x\n", t1.Bytes())
	// fmt.Printf("g t2 50: %x\n", t2.Bytes())
	// fmt.Printf("g t3 50: %x\n", t3.Bytes())

	for i = 1; i < 50; i++ {
		t2.Square(t2) //     fe25519_sq(t2, t2);
	}
	t2.Multiply(t2, t1) // fe25519_mul(t2, t2, t1);
	t3.Square(t2)       // fe25519_sq(t3, t2);
	for i = 1; i < 100; i++ {
		t3.Square(t3) //     fe25519_sq(t3, t3);
	}
	t2.Multiply(t3, t2) // fe25519_mul(t2, t3, t2);
	t2.Square(t2)       // fe25519_sq(t2, t2);
	for i = 1; i < 50; i++ {
		t2.Square(t2) //     fe25519_sq(t2, t2);
	}
	t1.Multiply(t2, t1) // fe25519_mul(t1, t2, t1);
	t1.Square(t1)       // fe25519_sq(t1, t1);
	for i = 1; i < 4; i++ {
		t1.Square(t1) //     fe25519_sq(t1, t1);
	}
	out.Multiply(t1, t0) // fe25519_mul(out, t1, t0);

	return out
}
