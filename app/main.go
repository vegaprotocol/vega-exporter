package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"

	"flag"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type App struct {
	datanodeAddr       string
	datanodeInsecure   bool
	tendermintAddr     string
	tendermintInsecure bool
	prometheusCounters map[string]*prometheus.CounterVec
	prometheusGauges   map[string]*prometheus.GaugeVec
	nodeList           map[string]string
}

func Run(datanodeAddr, tendermintAddr, listenAddr string, datanodeInsecure, tendermintInsecure bool) error {
	app := &App{
		datanodeAddr:       datanodeAddr,
		datanodeInsecure:   datanodeInsecure,
		tendermintAddr:     tendermintAddr,
		tendermintInsecure: tendermintInsecure,
		prometheusCounters: make(map[string]*prometheus.CounterVec),
		prometheusGauges:   make(map[string]*prometheus.GaugeVec),
	}
	app.initMetrics()
	http.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}))
	go http.ListenAndServe(listenAddr, nil)

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, NoColor: true})

	flag.Parse()
	if len(app.datanodeAddr) <= 0 {
		return fmt.Errorf("error: missing datanode grpc server address")
	}
	if len(app.tendermintAddr) <= 0 {
		return fmt.Errorf("error: missing tendermint server address")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg := sync.WaitGroup{}
	log.Info().Msg("Started")

	if err := app.StartVegaObserver(ctx, cancel, &wg); err != nil {
		return fmt.Errorf("error when starting the stream: %v", err)
	}
	if err := app.StartTMObserver(ctx, cancel, &wg); err != nil {
		return fmt.Errorf("error when observing tendermint: %v", err)
	}

	app.waitSig(ctx, cancel)
	wg.Wait()
	return nil
}

func (a *App) initMetrics() {
	a.prometheusCounters["sumWithdrawals"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_withdrawals_sum_total",
			Help: "Total amount of withdrawals",
		},
		[]string{"chain_id", "status", "asset", "eth_tx"},
	)
	a.prometheusCounters["countWithdrawals"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_withdrawals_count_total",
			Help: "Total count of withdrawals",
		},
		[]string{"chain_id", "status", "asset", "eth_tx"},
	)
	a.prometheusCounters["sumTransfers"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_transfers_sum_total",
			Help: "Total amount of transfers",
		},
		[]string{"chain_id", "status", "asset"},
	)
	a.prometheusCounters["countTransfers"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_transfers_count_total",
			Help: "Total count of transfers",
		},
		[]string{"chain_id", "status", "asset"},
	)

	a.prometheusCounters["sumLedgerMvt"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_ledger_mvt_sum_total",
			Help: "Total amount of ledger movement",
		},
		[]string{"chain_id", "asset", "type", "from_account_type", "from_market", "to_account_type"},
	)
	a.prometheusCounters["countLedgerMvt"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_ledger_mvt_count_total",
			Help: "Total count of ledger movement",
		},
		[]string{"chain_id", "asset", "type", "from_account_type", "from_market", "to_account_type"},
	)

	a.prometheusGauges["assetQuantum"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_asset_quantum",
			Help: "Quantum value for each asset",
		},
		[]string{"chain_id", "asset"},
	)
	a.prometheusGauges["assetDecimals"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_asset_decimals",
			Help: "Decimals for each asset",
		},
		[]string{"chain_id", "asset"},
	)

	a.prometheusGauges["marketBestOfferPrice"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_best_offer_price",
			Help: "Best sell price per market",
		},
		[]string{"chain_id", "market", "market_id"},
	)

	a.prometheusGauges["marketBestBidPrice"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_best_bid_price",
			Help: "Best buy price per market",
		},
		[]string{"chain_id", "market", "market_id"},
	)

	a.prometheusCounters["totalProposedBlocks"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_validator_proposed_blocks_total",
			Help: "Number of block proposed per validator",
		},
		[]string{"address", "name"},
	)

	a.prometheusCounters["totalSignedBlocks"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_validator_signed_blocks_total",
			Help: "Number of block signed per validator",
		},
		[]string{"address", "name"},
	)

	a.prometheusCounters["totalNodeVote"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_validator_node_vote_total",
			Help: "Number of node votes submitted per validator",
		},
		[]string{"address", "name"},
	)

	a.prometheusCounters["totalChainEvent"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_validator_chain_event_total",
			Help: "Number of node votes submitted per validator",
		},
		[]string{"address", "name"},
	)

	a.prometheusGauges["priceMonitoringBoundsMin"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_min_valid_price",
			Help: "Market price monitoring bound: minimal valid price",
		},
		[]string{"chain_id", "market", "market_id"},
	)

	a.prometheusGauges["priceMonitoringBoundsMax"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_max_valid_price",
			Help: "Market price monitoring bound: maximal valid price",
		},
		[]string{"chain_id", "market", "market_id"},
	)

	a.prometheusGauges["priceMonitoringBoundsMax"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_max_valid_price",
			Help: "Market price monitoring bound: maximal valid price",
		},
		[]string{"chain_id", "market", "market_id"},
	)

	for _, counter := range a.prometheusCounters {
		prometheus.MustRegister(counter)
	}

	for _, gauge := range a.prometheusGauges {
		prometheus.MustRegister(gauge)
	}
}
