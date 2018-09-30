package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"math/big"
	"sync"
	"time"

	"gopkg.in/dedis/kyber.v2"

	protobufs "github.com/dexm-coin/protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/gopherjs/gopherjs/js"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ripemd160"
)

// Wallet is an internal representation of a private key
type Wallet struct {
	PrivKey        *ecdsa.PrivateKey
	Shard          uint8
	Nonce          int
	Balance        int
	PrivKeySchnorr []byte
	PubKeySchnorr  []byte
}

type file struct {
	// content to be converted in json
	PrivKeyString        string
	Address              string
	Nonce                int
	Balance              int
	Shard                int
	PrivKeySchnorrString []byte
	PubKeySchnorrString  []byte
}

// GenerateWallet generates a new random wallet with a 0 balance and nonce
func GenerateWallet() (*Wallet, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	x, p := CreateSchnorrKeys()
	xByte, err := x.MarshalBinary()
	if err != nil {
		log.Error(err)
		return nil, err
	}

	pByte, err := p.MarshalBinary()
	if err != nil {
		log.Error(err)
		return nil, err
	}

	// shard := uint8(rand.Int31n(10) + 1)
	shardA := make([]byte, 1)
	rand.Read(shardA)

	return &Wallet{
		PrivKey:        priv,
		Shard:          uint8(shardA[0]),
		Nonce:          0,
		Balance:        0,
		PrivKeySchnorr: xByte,
		PubKeySchnorr:  pByte,
	}, nil
}

// GetPrivateKeySchnorr return the private schnorr key to the wallet
func (w *Wallet) GetPrivateKeySchnorr() (kyber.Scalar, error) {
	return ByteToScalar(w.PrivKeySchnorr)
}

// GetPrivateKeySchnorr return the private schnorr key to the wallet
func (w *Wallet) GetPublicKeySchnorr() (kyber.Point, error) {
	return ByteToPoint(w.PubKeySchnorr)
}

// GetPrivateKeySchnorr return the private schnorr key to the wallet
func (w *Wallet) GetPublicKeySchnorrByte() []byte {
	return w.PubKeySchnorr
}

// GetShardWallet return the shard to the wallet
func (w *Wallet) GetShardWallet() uint8 {
	return w.Shard
}

// JSWallet returns a gopherjs wrapper to the wallet
func JSWallet() *js.Object {
	wal, _ := GenerateWallet()
	return js.MakeWrapper(wal)
}

// ImportWallet opens the file passed to it and tries to parse it as a private key
// and convert it into a Wallet struct
func ImportWallet(filePath string) (*Wallet, error) {
	walletfilejson, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return jsonKeyToStruct(walletfilejson)
}

func jsonKeyToStruct(walletJSON []byte) (*Wallet, error) {
	var walletfile file
	err := json.Unmarshal(walletJSON, &walletfile)
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
		PrivKey:        key,
		Nonce:          walletfile.Nonce,
		Balance:        walletfile.Balance,
		PrivKeySchnorr: walletfile.PrivKeySchnorrString,
		PubKeySchnorr:  walletfile.PubKeySchnorrString,
	}, nil
}

// ExportWallet saves the internal Wallet structure to a file
func (w *Wallet) ExportWallet(filePath string) error {
	result, err := w.GetEncodedWallet()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, result, 400)
}

// GetEncodedWallet returns a JSON encoded wallet
func (w *Wallet) GetEncodedWallet() ([]byte, error) {
	// convert priv key to x509
	x509Encoded, err := x509.MarshalECPrivateKey(w.PrivKey)
	if err != nil {
		return nil, err
	}
	pemEncoded := pem.EncodeToMemory(&pem.Block{Type: "WALLET PRIVATE KEY", Bytes: x509Encoded})

	add, err := w.GetWallet()
	if err != nil {
		return nil, err
	}
	// x, err := ByteToScalar(w.PrivKeySchnorr)
	// if err != nil {
	// 	log.Error(err)
	// 	return nil, err
	// }
	// p, err := ByteToPoint(w.PubKeySchnorr)
	// if err != nil {
	// 	log.Error(err)
	// 	return nil, err
	// }

	walletfile := file{
		Address:              add,
		PrivKeyString:        string(pemEncoded),
		Nonce:                w.Nonce,
		Balance:              w.Balance,
		PrivKeySchnorrString: w.PrivKeySchnorr,
		PubKeySchnorrString:  w.PubKeySchnorr,
	}

	return json.Marshal(walletfile)
}

// GetWallet returns the address of a wallet
func (w *Wallet) GetWallet() (string, error) {
	x509Encoded, err := w.GetPubKey()
	if err != nil {
		return "", err
	}

	return BytesToAddress(x509Encoded, uint32(w.Shard)), nil
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

	if len(wallet) < 30 {
		return false
	}

	// Make a crc of the wallet excluding the header and shard
	sum := crc32.ChecksumIEEE([]byte(wallet[6 : len(wallet)-8]))
	return fmt.Sprintf("%08X", sum) == wallet[len(wallet)-8:len(wallet)]
}

// BytesToAddress converts the bytes of the PublicKey into a wallet address
func BytesToAddress(data []byte, shard uint32) string {
	hash := sha256.Sum256(data)

	h := ripemd160.New()
	h.Write(hash[:])

	mainWal := base58Encoding(h.Sum(nil))
	sum := crc32.ChecksumIEEE([]byte(mainWal))

	wal := fmt.Sprintf("Dexm%02X%s%08X", uint8(shard), mainWal, sum)

	return wal
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

// RawTransaction returns a struct with a transaction. Used in GopherJS to avoid
// protobuf which uses the unsupported unsafe
func (w *Wallet) RawTransaction(recipient string, amount uint64, gas uint32, data []byte, shard uint32) (*protobufs.Transaction, error) {
	if !IsWalletValid(recipient) {
		return nil, errors.New("Invalid recipient")
	}

	if int(amount+uint64(gas)) > w.Balance {
		return nil, errors.New("Insufficient Balance")
	}

	w.Balance -= int(amount + uint64(gas))

	x509Encoded, err := x509.MarshalPKIXPublicKey(&w.PrivKey.PublicKey)
	if err != nil {
		return nil, err
	}

	w.Nonce++

	newT := &protobufs.Transaction{
		Sender:    x509Encoded,
		Recipient: recipient,
		Nonce:     uint32(w.Nonce),
		Amount:    amount,
		Gas:       gas,
		Timestamp: uint64(time.Now().Unix()),
		Data:      data,
		Shard:     shard,
	}

	if len(data) != 0 {
		newT.ContractCreation = true
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

	return newT, nil
}

// NewTransaction generates a signed transaction for the given arguments without
// broadcasting it to the newtwork
func (w *Wallet) NewTransaction(recipient string, amount uint64, gas uint32, data []byte, shard uint32) ([]byte, error) {
	newT, err := w.RawTransaction(recipient, amount, gas, data, shard)
	if err != nil {
		return nil, err
	}

	return proto.Marshal(newT)
}

// GenerateVanityWallet create a wallet that start with "Dexm" + your_word
func GenerateVanityWallet(vanity string, userWallet string, vainityFound *bool, wg *sync.WaitGroup) error {
	for {

		if *vainityFound == true {
			wg.Done()
			return nil
		}

		wal, _ := GenerateWallet()
		wallString, _ := wal.GetWallet()

		if wallString[:4+len(vanity)] == "Dexm"+vanity {
			log.Info("Found wallet: ", wallString)
			wal.ExportWallet(userWallet)

			*vainityFound = true
			wg.Done()
			return nil
		}
	}
}
