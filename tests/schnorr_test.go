package tests

import (
	"fmt"
	"testing"

	"gopkg.in/dedis/kyber.v2"
	"gopkg.in/dedis/kyber.v2/group/edwards25519"
)

var curve = edwards25519.NewBlakeSHA256Ed25519()
var sha256Curve = curve.Hash()
var g = curve.Point().Base()

type Signature struct {
	r kyber.Point
	s kyber.Scalar
}

func Hash(s string) kyber.Scalar {
	sha256Curve.Reset()
	sha256Curve.Write([]byte(s))
	return curve.Scalar().SetBytes(sha256Curve.Sum(nil))
}

/*
	both generate `k`
	both do r = k*G, r1 e r2
	after some them up r, r1 + r2
	P = (publickey1 + publickey2)
	e = m + (r1+r2) + P
	s = k – e * x , s1 e s2
	e S = s1 + s2
	The verification by checking that R = s * G + H(m || P || R) * P


	C = H(P0 || P1)
	Q0 = H(C || P0) * P0 , Q1 = H(C || P1) * P1
	P = Q0 + Q1
	Alice uses y0 = x0 * H(C || P0) as private key , Bob y1 = x1 * H(C || P1)
*/

// m: Message
// x: Private key
func Sign(m string, x kyber.Scalar, otherR []kyber.Point, otherP []kyber.Point, k kyber.Scalar) kyber.Scalar {
	// Pick a random k from allowed set.
	// k := curve.Scalar().Pick(curve.RandomStream())

	// SHARD THIS
	// r = k * G
	myR := curve.Point().Mul(k, g)

	// p = x * G
	myP := curve.Point().Mul(x, g)

	R := myR
	for _, r := range otherR {
		R = curve.Point().Add(R, r)
	}
	P := myP
	for _, p := range otherP {
		P = curve.Point().Add(P, p)
	}

	// C := Hash(P.String())
	// myQ := curve.Point().Mul(Hash(C.String()+myP.String()), myP)
	// P2 := myQ
	// for _, p := range otherP {
	// 	P2 = curve.Point().Add(P2, curve.Point().Mul(Hash(C.String()+p.String()), p))
	// }
	// e := Hash(m + P2.String() + R.String())

	// private key
	// y := curve.Point().Mul(x, Hash(C + myP))

	// Hash(m || r || p)
	e := Hash(m + P.String() + R.String())

	// s = k - e * x
	s := curve.Scalar().Sub(k, curve.Scalar().Mul(e, x))

	return s
}

// m: Message
// S: Signature
func PublicKey(m string, S Signature) kyber.Point {
	// e = Hash(m || r)
	e := Hash(m + S.r.String())

	// y = (r - s * G) * (1 / e)
	y := curve.Point().Sub(S.r, curve.Point().Mul(S.s, g))
	y = curve.Point().Mul(curve.Scalar().Div(curve.Scalar().One(), e), y)

	return y
}

/*
	The verification by checking that R = s * G + H(m || P || R) * P
*/

// s: Signature
// p: Public key
// func Verify(m string, S Signature, P kyber.Point, R kyber.Point) bool {
// 	// e = Hash(m || r || P)
// 	e := Hash(m + S.r.String() + P.String())

// 	// Attempt to reconstruct 's * G' with a provided signature; s * G = r - e * y
// 	sGv := curve.Point().Sub(S.r, curve.Point().Mul(e, P))
// 	// Construct the actual 's * G'
// 	sG := curve.Point().Mul(S.s, g)
// 	// Equality check; ensure signature and public key outputs to s * G.
// 	return sG.Equal(sGv)
// }

// func Verify(m string, S Signature, P kyber.Point, R kyber.Point) bool {
// 	// e = Hash(m || r || P)
// 	e := Hash(m + P.String() + R.String())

// 	// check R = s * G + H(m || P || R) * P
// 	a := curve.Point().Add(curve.Point().Mul(S.s, g), curve.Point().Mul(e, P))
// 	return R.Equal(a)
// }

// func (S Signature) String() string {
// 	return fmt.Sprintf("(r=%s, s=%s)", S.r, S.s)
// }

func Verify(m string, rSignature kyber.Point, sSignature kyber.Scalar, P kyber.Point, R kyber.Point) bool {
	// e = Hash(m || r || P)
	e := Hash(m + P.String() + R.String())

	// check R = s * G + H(m || P || R) * P
	a := curve.Point().Add(curve.Point().Mul(sSignature, g), curve.Point().Mul(e, P))
	return R.Equal(a)
}

func CreateSignature(Rs []kyber.Point, Ss []kyber.Scalar) ([]byte, []byte, error) {
	R := Rs[0]
	for _, r := range Rs[1:] {
		R = curve.Point().Add(R, r)
	}

	S := Ss[0]
	for _, s := range Ss[1:] {
		S = curve.Scalar().Add(S, s)
	}

	// signature := Signature{r: R, s: S}

	// return signature, nil

	byteR, err := R.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}

	byteS, err := S.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}

	return byteR, byteS, nil
}

func VerifySignature(message string, rSignature kyber.Point, sSignature kyber.Scalar, otherP []kyber.Point, myP kyber.Point, otherR []kyber.Point, myR kyber.Point) bool {
	P := myP
	for _, p := range otherP {
		P = curve.Point().Add(P, p)
	}

	R := myR
	for _, r := range otherR {
		R = curve.Point().Add(R, r)
	}

	v := Verify(message, rSignature, sSignature, P, R)
	return v
}

func ByteToPointSchnorr(b []byte) (kyber.Point, error) {
	p := curve.Point()
	err := p.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func ByteToScalarSchnorr(b []byte) (kyber.Scalar, error) {
	p := curve.Scalar()
	err := p.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func TestSchnorr(t *testing.T) {
	x1 := curve.Scalar().Pick(curve.RandomStream())
	publicKey1 := curve.Point().Mul(x1, g)

	fmt.Println(publicKey1)
	fmt.Println(publicKey1.String())
	k1 := curve.Scalar().Pick(curve.RandomStream())
	R1 := curve.Point().Mul(k1, g)

	x2 := curve.Scalar().Pick(curve.RandomStream())
	publicKey2 := curve.Point().Mul(x2, g)
	k2 := curve.Scalar().Pick(curve.RandomStream())
	R2 := curve.Point().Mul(k2, g)

	message := "We're gonna be signing this!"

	s1 := Sign(message, x1, []kyber.Point{R2}, []kyber.Point{publicKey2}, k1)
	fmt.Printf("Signature1 %s\n\n", s1)
	s2 := Sign(message, x2, []kyber.Point{R1}, []kyber.Point{publicKey1}, k2)
	fmt.Printf("Signature2 %s\n\n", s2)

	rSigned, sSigned, err := CreateSignature([]kyber.Point{R1, R2}, []kyber.Scalar{s1, s2})
	fmt.Println(err)
	rSignedTransaction, err := ByteToPointSchnorr(rSigned)
	fmt.Println(err)
	sSignedTransaction, err := ByteToScalarSchnorr(sSigned)
	fmt.Println(err)
	fmt.Printf("Is the signature valid %t\n\n", VerifySignature(message, rSignedTransaction, sSignedTransaction, []kyber.Point{publicKey2}, publicKey1, []kyber.Point{R2}, R1))

	R := R1
	for _, r := range []kyber.Point{R2} {
		R = curve.Point().Add(R, r)
	}
	S := s1
	for _, s := range []kyber.Scalar{s2} {
		S = curve.Scalar().Add(S, s)
	}
	signature := Signature{r: R, s: S}

	P := publicKey1
	for _, p := range []kyber.Point{publicKey2} {
		P = curve.Point().Add(P, p)
	}
	fmt.Printf("Is the signature valid %t\n\n", Verify(message, signature.r, signature.s, P, R))
	fmt.Printf("Is the signature valid %t\n\n", VerifySignature(message, signature.r, signature.s, []kyber.Point{publicKey2}, publicKey1, []kyber.Point{R2}, R1))
}
