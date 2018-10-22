package tests

// import (
// 	"math/rand"
// 	"testing"

// 	"github.com/dexm-coin/dexmd/wallet"
// )

// func benchmarkEncryption(l int, b *testing.B) {
// 	w, _ := wallet.GenerateWallet()

// 	k := make([]byte, l)
// 	// Use math/rand so it's faster in benchmarks don't use it in actual code
// 	rand.Read(k)

// 	for n := 0; n < b.N; n++ {
// 		_, err := wallet.EncryptWallet(k, w)
// 		if err != nil {
// 			b.Error(err)
// 		}
// 	}
// }

// func BenchmarkEncryptWal5(b *testing.B)  { benchmarkEncryption(5, b) }
// func BenchmarkEncryptWal10(b *testing.B) { benchmarkEncryption(10, b) }
// func BenchmarkEncryptWal20(b *testing.B) { benchmarkEncryption(20, b) }
// func BenchmarkEncryptWal40(b *testing.B) { benchmarkEncryption(40, b) }
