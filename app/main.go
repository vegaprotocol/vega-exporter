package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type App struct {
	datanodeAddr       string
	datanodeTls        bool
	tendermintAddr     string
	tendermintTls      bool
	ethereumRpcAddr    string
	assetPoolContract  string
	prometheusCounters map[string]*prometheus.CounterVec
	prometheusGauges   map[string]*prometheus.GaugeVec
	nodeList           map[string]string
}

func Run(datanodeAddr, tendermintAddr, ethereumRpcAddr, assetPoolContract, listenAddr string, datanodeTls, tendermintTls bool) error {
	app := &App{
		datanodeAddr:       datanodeAddr,
		datanodeTls:        datanodeTls,
		tendermintAddr:     tendermintAddr,
		tendermintTls:      tendermintTls,
		ethereumRpcAddr:    ethereumRpcAddr,
		assetPoolContract:  assetPoolContract,
		prometheusCounters: make(map[string]*prometheus.CounterVec),
		prometheusGauges:   make(map[string]*prometheus.GaugeVec),
	}
	app.initMetrics()
	http.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}))
	go http.ListenAndServe(listenAddr, nil)

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, NoColor: true})
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
		log.Error().Err(err).Msg("error when starting the stream")
		return err
	}
	if err := app.StartTMObserver(ctx, cancel, &wg); err != nil {
		log.Error().Err(err).Msg("error when observing tendermint")
		return err
	}
	if err := app.StartAssetPoolWatch(ctx, cancel, &wg); err != nil {
		log.Error().Err(err).Msg("error when observing tendermint")
		return err
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
		[]string{"chain_id", "address", "name"},
	)

	a.prometheusCounters["totalSignedBlocks"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_validator_signed_blocks_total",
			Help: "Number of block signed per validator",
		},
		[]string{"chain_id", "address", "name"},
	)

	a.prometheusCounters["totalNodeVote"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_validator_node_vote_total",
			Help: "Number of node votes submitted per validator",
		},
		[]string{"chain_id", "address", "name"},
	)

	a.prometheusCounters["totalChainEvent"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_validator_chain_event_total",
			Help: "Number of node votes submitted per validator",
		},
		[]string{"chain_id", "address", "name"},
	)

	a.prometheusCounters["proposals"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_proposals_count_total",
			Help: "Number of proposals on the network",
		},
		[]string{"chain_id", "state"},
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

	a.prometheusGauges["marketSettlementPrice"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_settlement_price",
			Help: "Market Settlement price per market",
		},
		[]string{"chain_id", "asset", "market", "market_id"},
	)

	a.prometheusGauges["partyCountTotal"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_party_count_total",
			Help: "Number of parties in the network",
		},
		[]string{"chain_id"},
	)

	a.prometheusGauges["erc20AssetBalance"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_erc20_bridge_balance_total",
			Help: "ERC20 Asset balance on the bridge",
		},
		[]string{"asset"},
	)

	a.prometheusGauges["totalRewardPayout"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_total_reward_payout_nodecimal",
			Help: "Total reward payout per asset, based on the percentOfTotalReward value and amount of each event.",
		},
		[]string{"chain_id", "reward_type", "asset"},
	)

	for _, counter := range a.prometheusCounters {
		prometheus.MustRegister(counter)
	}

	for _, gauge := range a.prometheusGauges {
		prometheus.MustRegister(gauge)
	}
}
