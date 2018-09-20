package wallet

import (
	"fmt"

	"gopkg.in/dedis/kyber.v2"
	"gopkg.in/dedis/kyber.v2/group/edwards25519"
)

var curve = edwards25519.NewBlakeSHA256Ed25519()
var sha256Curve = curve.Hash()

type Signature struct {
	r kyber.Point
	s kyber.Scalar
}

func Hash(s []byte) kyber.Scalar {
	sha256Curve.Reset()
	sha256Curve.Write(s)
	return curve.Scalar().SetBytes(sha256Curve.Sum(nil))
}

// m: Message
// x: Private key
func Sign(m []byte, x kyber.Scalar) Signature {
	// Get the base of the curve.
	g := curve.Point().Base()

	// Pick a random k from allowed set.
	k := curve.Scalar().Pick(curve.RandomStream())

	// r = k * G (a.k.a the same operation as r = g^k)
	r := curve.Point().Mul(k, g)

	// Hash(m || r)
	res := append(m, []byte(r.String())...)
	e := Hash(res)

	// s = k - e * x
	s := curve.Scalar().Sub(k, curve.Scalar().Mul(e, x))

	return Signature{r: r, s: s}
}

// m: Message
// S: Signature
func PublicKey(m []byte, S Signature) kyber.Point {
	// Create a generator.
	g := curve.Point().Base()

	// e = Hash(m || r)
	res := append(m, []byte(S.r.String())...)
	e := Hash(res)

	// y = (r - s * G) * (1 / e)
	y := curve.Point().Sub(S.r, curve.Point().Mul(S.s, g))
	y = curve.Point().Mul(curve.Scalar().Div(curve.Scalar().One(), e), y)

	return y
}

// s: Signature
// y: Public key
func Verify(m []byte, S Signature, y kyber.Point) bool {
	// Create a generator.
	g := curve.Point().Base()

	// e = Hash(m || r)
	res := append(m, []byte(S.r.String())...)
	e := Hash(res)

	// Attempt to reconstruct 's * G' with a provided signature; s * G = r - e * y
	sGv := curve.Point().Sub(S.r, curve.Point().Mul(e, y))

	// Construct the actual 's * G'
	sG := curve.Point().Mul(S.s, g)

	// Equality check; ensure signature and public key outputs to s * G.
	return sG.Equal(sGv)
}

func (S Signature) String() string {
	return fmt.Sprintf("(r=%s, s=%s)", S.r, S.s)
}

func CreateSchnorrAccount() (kyber.Scalar, kyber.Point) {
	privateKey := curve.Scalar().Pick(curve.RandomStream())
	publicKey := curve.Point().Mul(privateKey, curve.Point().Base())
	return privateKey, publicKey
}