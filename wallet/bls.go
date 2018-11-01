// Copyright 2016 The Alpenhorn Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bls implements BLS aggregate signatures.
//
// This package implements the scheme described in
// "Aggregate and Verifiably Encrypted Signatures from Bilinear Maps"
// by Boneh, Gentry, Lynn, and Shacham (Eurocrypt 2003):
// https://www.iacr.org/archive/eurocrypt2003/26560416/26560416.pdf.
package wallet

// import (
// 	"crypto/sha256"
// 	"crypto/subtle"
// 	"encoding/base64"
// 	"errors"
// 	"io"
// 	"math/big"

// 	"vuvuzela.io/crypto/bn256"
// )

// const CompressedSize = 32

// type PrivateKeyBls struct {
// 	x *big.Int
// }

// type PublicKeyBls struct {
// 	gx *bn256.G2
// }

// // type Signature []byte

// var g2gen = new(bn256.G2).ScalarBaseMult(big.NewInt(1))

// func GetByteX(p *PrivateKeyBls) []byte {
// 	return p.x.Bytes()
// }

// func GenerateKeyBls(rand io.Reader) (*PublicKeyBls, *PrivateKeyBls, error) {
// 	x, gx, err := bn256.RandomG2(rand)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	return &PublicKeyBls{gx}, &PrivateKeyBls{x}, nil
// }

// func SignBls(PrivateKeyBls *PrivateKeyBls, message []byte) []byte {
// 	h := new(bn256.G1).HashToPoint(message)
// 	hx := new(bn256.G1).ScalarMult(h, PrivateKeyBls.x)
// 	return hx.Marshal()
// }

// // Aggregate combines signatures on distinct messages.  The messages must
// // be distinct, otherwise the scheme is vulnerable to chosen-key attack.
// func AggregateBls(sigs ...[]byte) []byte {
// 	var sum *bn256.G1
// 	for i, sig := range sigs {
// 		hx, ok := new(bn256.G1).Unmarshal(sig)
// 		if !ok {
// 			panic("invalid signature")
// 		}
// 		if i == 0 {
// 			sum = new(bn256.G1).Set(hx)
// 		} else {
// 			sum.Add(sum, hx)
// 		}
// 	}
// 	return sum.Marshal()
// }

// // Compress reduces the size of a signature by dropping its y-coordinate.
// // func (sig []byte) Compress() *[CompressedSize]byte {
// // 	// only keep the x-coordinate
// // 	var compressed [CompressedSize]byte
// // 	copy(compressed[:], sig[0:32])
// // 	return &compressed
// // }

// // Verify verifies an aggregate signature.  Returns false if messages
// // are not distinct or if sig is not a valid signature.
// func VerifyBls(keys []*PublicKeyBls, messages [][]byte, sig []byte) bool {
// 	hx, ok := new(bn256.G1).Unmarshal(sig)
// 	if !ok {
// 		return false
// 	}

// 	if !distinct(messages) {
// 		return false
// 	}

// 	var sum *bn256.GT
// 	for i := range messages {
// 		h := new(bn256.G1).HashToPoint(messages[i])
// 		p := bn256.Pair(h, keys[i].gx)
// 		if i == 0 {
// 			sum = p
// 		} else {
// 			sum.Add(sum, p)
// 		}
// 	}

// 	u := bn256.Pair(hx, g2gen)
// 	return subtle.ConstantTimeCompare(u.Marshal(), sum.Marshal()) == 1
// }

// // VerifyCompressed verifies a compressed aggregate signature.  Returns
// // false if messages are not distinct.
// func VerifyCompressedBls(keys []*PublicKeyBls, messages [][]byte, sig *[CompressedSize]byte) bool {
// 	if !distinct(messages) {
// 		return false
// 	}

// 	xCord := new(big.Int).SetBytes(sig[:])
// 	hx, ok := new(bn256.G1).FromX(xCord)
// 	if !ok {
// 		return false
// 	}

// 	var sum *bn256.GT
// 	for i := range messages {
// 		h := new(bn256.G1).HashToPoint(messages[i])
// 		p := bn256.Pair(h, keys[i].gx)
// 		if i == 0 {
// 			sum = p
// 		} else {
// 			sum.Add(sum, p)
// 		}
// 	}

// 	u := bn256.Pair(hx, g2gen)
// 	ub := u.Marshal()
// 	vb := sum.Marshal()
// 	ok1 := subtle.ConstantTimeCompare(ub, vb) == 1

// 	uinv := new(bn256.GT).Neg(u)
// 	uinvb := uinv.Marshal()
// 	ok2 := subtle.ConstantTimeCompare(uinvb, vb) == 1

// 	return ok1 || ok2
// }

// func distinct(msgs [][]byte) bool {
// 	m := make(map[[32]byte]bool)
// 	for _, msg := range msgs {
// 		h := sha256.Sum256(msg)
// 		if m[h] {
// 			return false
// 		}
// 		m[h] = true
// 	}
// 	return true
// }

// func (pk *PublicKeyBls) MarshalText() ([]byte, error) {
// 	return encodeToText(pk.gx.Marshal()), nil
// }

// func (pk *PublicKeyBls) UnmarshalText(data []byte) error {
// 	bs, err := decodeText(data)
// 	if err != nil {
// 		return err
// 	}
// 	pk.gx = new(bn256.G2)
// 	_, ok := pk.gx.Unmarshal(bs)
// 	if !ok {
// 		return errors.New("bls.PublicKeyBls: failed to unmarshal underlying point")
// 	}
// 	return nil
// }

// func (pk *PublicKeyBls) MarshalBinary() ([]byte, error) {
// 	return pk.gx.Marshal(), nil
// }

// func UnmarshalBinaryBls(data []byte) (*PublicKeyBls, error) {
// 	pk := &PublicKeyBls{}
// 	pk.gx = new(bn256.G2)
// 	_, ok := pk.gx.Unmarshal(data)
// 	if !ok {
// 		return nil, errors.New("bls.PublicKeyBls: failed to unmarshal underlying point")
// 	}
// 	return pk, nil
// }

// func encodeToText(data []byte) []byte {
// 	buf := make([]byte, base64.RawURLEncoding.EncodedLen(len(data)))
// 	base64.RawURLEncoding.Encode(buf, data)
// 	return buf
// }

// func decodeText(data []byte) ([]byte, error) {
// 	buf := make([]byte, base64.RawURLEncoding.DecodedLen(len(data)))
// 	n, err := base64.RawURLEncoding.Decode(buf, data)
// 	return buf[:n], err
// }
