package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"math/big"
	"time"

	"strings"

	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/minio/blake2b-simd"
	"golang.org/x/crypto/ripemd160"
)

// Wallet is an internal representation of a private key
type Wallet struct {
	PrivKey *ecdsa.PrivateKey
	Nonce   int
	Balance int
}

type file struct {
	// content to be converted in json
	PrivKeyString string
	Address       string
	Nonce         int
	Balance       int
}

// GenerateWallet generates a new random wallet with a 0 balance and nonce
func GenerateWallet() (*Wallet, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	return &Wallet{
		PrivKey: priv,
		Nonce:   0,
		Balance: 0,
	}, nil
}

// ImportWallet opens the file passed to it and tries to parse it as a private key
// and convert it into a Wallet struct
func ImportWallet(filePath string) (*Wallet, error) {
	walletfilejson, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var walletfile file
	err = json.Unmarshal(walletfilejson, &walletfile)
	if err != nil {
		return nil, err
	}
	pemEncoded := []byte(walletfile.PrivKeyString)
	decoded, _ := pem.Decode(pemEncoded)

	key, err := x509.ParseECPrivateKey(decoded.Bytes)
	if err != nil {
		return nil, err
	}
	return &Wallet{
		PrivKey: key,
		Nonce:   walletfile.Nonce,
		Balance: walletfile.Balance}, nil
}

// ExportWallet saves the internal Wallet structure to a file
func (w *Wallet) ExportWallet(filePath string) error {
	// convert priv key to x509
	x509Encoded, err := x509.MarshalECPrivateKey(w.PrivKey)
	if err != nil {
		return err
	}
	pemEncoded := pem.EncodeToMemory(&pem.Block{Type: "WALLET PRIVATE KEY", Bytes: x509Encoded})

	add, err := w.GetWallet()
	if err != nil {
		return err

	}
	walletfile := file{
		Address:       add,
		PrivKeyString: string(pemEncoded),
		Nonce:         w.Nonce,
		Balance:       w.Balance,
	}

	result, err := json.Marshal(walletfile)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, result, 400)
}

// GetWallet returns the address of a wallet
func (w *Wallet) GetWallet() (string, error) {
	x509Encoded, err := w.GetPubKey()
	if err != nil {
		return "", err
	}

	return BytesToAddress(x509Encoded), nil
}

// GetPubKey returns a x509 encoded public key for the wallet
func (w *Wallet) GetPubKey() ([]byte, error) {
	return x509.MarshalPKIXPublicKey(&w.PrivKey.PublicKey)
}

// IsWalletValid checks if a wallet is valid by checking the checksum
func IsWalletValid(wallet string) bool {
	if wallet == "DexmVoid" || wallet == "DexmPoS" {
		return true
	}

	parts := strings.Split(wallet, "l")
	if len(parts) != 2 {
		return false
	}

	// len("Dexm") + 1
	if len(parts[0]) < 5 {
		return false
	}

	sum := crc32.ChecksumIEEE([]byte(parts[0][4:]))
	return fmt.Sprintf("%x", sum) == parts[1]
}

// BytesToAddress converts the bytes of the PublicKey into a wallet address
func BytesToAddress(data []byte) string {
	hash := blake2b.Sum256(data)

	h := ripemd160.New()
	h.Write(hash[:])

	mainWal := base58Encoding(h.Sum(nil))
	sum := crc32.ChecksumIEEE([]byte(mainWal))

	wal := fmt.Sprintf("Dexm%sl%x", mainWal, sum)

	return wal
}

// StrippedBytesToAddr converts the bytes of the PublicKey into a wallet address
// without the Dexm header and the checksum
func StrippedBytesToAddr(data []byte) []byte {
	hash := blake2b.Sum256(data)

	h := ripemd160.New()
	h.Write(hash[:])

	return h.Sum(nil)
}

// Sign signs the bytes passed to it with ECDSA
func (w *Wallet) Sign(data []byte) (r, s *big.Int, e error) {
	r, s, err := ecdsa.Sign(rand.Reader, w.PrivKey, data)
	if err != nil {
		return nil, nil, err
	}

	return r, s, nil
}

// Taken from https://github.com/mr-tron/go-base58
const b58digitsOrdered string = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func base58Encoding(bin []byte) string {
	binsz := len(bin)
	var i, j, high, zcount, carry int

	for zcount < binsz && bin[zcount] == 0 {
		zcount++
	}

	size := (binsz-zcount)*138/100 + 1
	var buf = make([]byte, size)

	high = size - 1
	for i = zcount; i < binsz; i++ {
		j = size - 1
		for carry = int(bin[i]); j > high || carry != 0; j-- {
			carry = carry + 256*int(buf[j])
			buf[j] = byte(carry % 58)
			carry /= 58
		}
		high = j
	}

	for j = 0; j < size && buf[j] == 0; j++ {
	}

	var b58 = make([]byte, size-j+zcount)

	if zcount != 0 {
		for i = 0; i < zcount; i++ {
			b58[i] = '1'
		}
	}

	for i = zcount; j < size; i++ {
		b58[i] = b58digitsOrdered[buf[j]]
		j++
	}

	return string(b58)
}

// NewTransaction generates a signed transaction for the given arguments without
// broadcasting it to the newtwork
func (w *Wallet) NewTransaction(recipient string, amount uint64, gas uint32) ([]byte, error) {
	if !IsWalletValid(recipient) {
		return nil, errors.New("Invalid recipient")
	}

	if int(amount+uint64(gas)) > w.Balance {
		return nil, errors.New("Insufficient Balance")
	}

	if !strings.HasPrefix(recipient, "Dexm") && len(recipient) > 30 {
		return nil, errors.New("Invalid Recipient")
	}

	w.Nonce++
	w.Balance -= int(amount + uint64(gas))

	x509Encoded, err := x509.MarshalPKIXPublicKey(&w.PrivKey.PublicKey)
	if err != nil {
		return nil, err
	}

	newT := &protobufs.Transaction{
		Sender:    x509Encoded,
		Recipient: recipient,
		Nonce:     uint32(w.Nonce),
		Amount:    amount,
		Gas:       gas,
		Timestamp: uint64(time.Now().Unix()),
	}

	result, err := proto.Marshal(newT)
	if err != nil {
		return nil, err
	}

	r, s, err := w.Sign(result)
	if err != nil {
		return nil, err
	}

	newT.R = r.Bytes()
	newT.S = s.Bytes()

	return proto.Marshal(newT)
}
