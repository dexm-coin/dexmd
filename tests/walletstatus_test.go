package tests

import (
	"testing"
	"time"

	"github.com/dexm-coin/dexmd/blockchain"
)

func TestDataProcessing(t *testing.T) {
	blockchain.OpenService(1)
	time.Sleep(50 * time.Second)
}
