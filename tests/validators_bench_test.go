package tests

import (
	"strconv"
	"testing"

	"github.com/dexm-coin/dexmd/blockchain"
)

// GenerateBigValidatorsBook creates a book with many entries,
// for benchmarking purposes
func GenerateBigValidatorsBook(nEntry int) *blockchain.ValidatorsBook {
	v := blockchain.NewValidatorsBook()
	for i := 1; i < nEntry+1; i++ {
		wallet := strconv.Itoa(i)
		v.AddValidatorFast(wallet, uint64(i))
	}
	return &v
}

// benchmarkIEVals times the functions for importing and exporting validators
func benchmarkIEVals(n int, b *testing.B) {
	v := GenerateBigValidatorsBook(n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := v.ExportValidatorsBook("/tmp/validators")
		if err != nil {
			panic(err)
		}
		_, err = blockchain.ImportValidatorsBook("/tmp/validators")
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkIEVals2(b *testing.B) { benchmarkIEVals(100, b) }
func BenchmarkIEVals3(b *testing.B) { benchmarkIEVals(1000, b) }
func BenchmarkIEVals4(b *testing.B) { benchmarkIEVals(10000, b) }
func BenchmarkIEVals5(b *testing.B) { benchmarkIEVals(100000, b) }
func BenchmarkIEVals6(b *testing.B) { benchmarkIEVals(1000000, b) }

// Benchmark the funcions to Add and Remove a Validator
func benchmarkAddRemoveValidator(n int, b *testing.B) {
	v := GenerateBigValidatorsBook(n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.AddValidator("stevejons", 1337)
		v.RemoveValidator("stevejons")
	}
}

func BenchmarkARVal2(b *testing.B) { benchmarkAddRemoveValidator(100, b) }
func BenchmarkARVal3(b *testing.B) { benchmarkAddRemoveValidator(1000, b) }
func BenchmarkARVal4(b *testing.B) { benchmarkAddRemoveValidator(10000, b) }
func BenchmarkARVal5(b *testing.B) { benchmarkAddRemoveValidator(100000, b) }
func BenchmarkARVal6(b *testing.B) { benchmarkAddRemoveValidator(1000000, b) }
func BenchmarkARVal7(b *testing.B) { benchmarkAddRemoveValidator(10000000, b) }

// Benchmark the function to choose a Validator
func benchmarkChooseValidator(n int, b *testing.B) {
	v := GenerateBigValidatorsBook(n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.ChooseValidator(int64(i))
	}
}

func BenchmarkChooseValidator2(b *testing.B) { benchmarkChooseValidator(100, b) }
func BenchmarkChooseValidator3(b *testing.B) { benchmarkChooseValidator(1000, b) }
func BenchmarkChooseValidator4(b *testing.B) { benchmarkChooseValidator(10000, b) }
func BenchmarkChooseValidator5(b *testing.B) { benchmarkChooseValidator(100000, b) }
func BenchmarkChooseValidator6(b *testing.B) { benchmarkChooseValidator(1000000, b) }
func BenchmarkChooseValidator7(b *testing.B) { benchmarkChooseValidator(10000000, b) }
