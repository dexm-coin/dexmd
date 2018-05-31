package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"

	"golang.org/x/crypto/bcrypt"
)

// EncryptWallet returns a wallet encrypted with the key passed as argument
func EncryptWallet(password []byte, wal *Wallet) ([]byte, error) {
	pk, err := wal.GetWallet()
	if err != nil {
		return nil, err
	}

	// use bcrypt to make key generation slower
	key, err := bcrypt.GenerateFromPassword([]byte(pk), 13)
	if err != nil {
		return nil, err
	}

	keyValidSize := sha256.Sum256(key)

	block, err := aes.NewCipher(keyValidSize[:])
	if err != nil {
		return nil, err
	}

	toEncrypt, err := wal.GetEncodedWallet()
	if err != nil {
		return nil, err
	}

	// cipthert is the iv + the cipher
	ciphert := make([]byte, aes.BlockSize+len(toEncrypt))

	// the iv is put in the first 24 bytes of the cipher
	iv := ciphert[:aes.BlockSize]

	// fill the iv with random bytes
	rand.Read(iv)

	cfb := cipher.NewCFBEncrypter(block, iv)
	cfb.XORKeyStream(ciphert[aes.BlockSize:], toEncrypt)

	return ciphert, err
}
