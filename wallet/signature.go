package wallet

import (
	"crypto/ecdsa"
	"crypto/x509"
	"math/big"
)

// SignatureValid checks if a signature is valid for a x509 encoded ecdsa pubkey
func SignatureValid(x509pub, r, s, data []byte) (bool, error) {
	genericPubKey, err := x509.ParsePKIXPublicKey(x509pub)
	if err != nil {
		return false, err
	}

	rb := new(big.Int)
	sb := new(big.Int)

	rb.SetBytes(r)
	sb.SetBytes(s)

	return ecdsa.Verify(genericPubKey.(*ecdsa.PublicKey), data, rb, sb), nil
}
