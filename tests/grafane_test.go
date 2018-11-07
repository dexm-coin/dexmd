package tests

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/christophberger/grada"
	log "github.com/sirupsen/logrus"
)

func newFakeDataFunc(max int, volatility float64) func() float64 {
	log.Info("newFakeDataFunc")
	return func() float64 {
		// time.Sleep(time.Duration(1) * time.Second)
		change := volatility*2*(rand.Float64()-0.5)*0.1 + rand.Float64()
		log.Info(change)
		return math.Max(1, change*float64(max))
	}
}

func TestGrafane(t *testing.T) {
	dash := grada.GetDashboard()
	UpdateDash1, err := dash.CreateMetric("CPU1", 10*time.Minute, time.Second)
	if err != nil {
		log.Fatal(err)
	}
	UpdateDash2, err := dash.CreateMetric("CPU2", 10*time.Minute, time.Second)
	if err != nil {
		log.Fatal(err)
	}
	MempoolTransactions := newFakeDataFunc(100, 0.1)
	MempoolTransactionsFunc := func(metric *grada.Metric, dataFunc func() float64) {
		for {
			log.Info("loop1")
			metric.Add(dataFunc())
		}
	}
	BlockTransactions := newFakeDataFunc(200, 0.1)
	BlockTransactionsFunc := func(metric *grada.Metric, dataFunc func() float64) {
		for {
			log.Info("loop2")
			metric.Add(dataFunc())
		}
	}
	go MempoolTransactionsFunc(UpdateDash1, MempoolTransactions)
	go BlockTransactionsFunc(UpdateDash2, BlockTransactions)
	select {}
}
