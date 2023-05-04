package app

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (a *App) initMetrics(listenAddr string) {
	a.prometheusCounters["sumWithdrawals"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_withdrawals_sum_total",
			Help: "Total amount of withdrawals",
		},
		[]string{"status", "asset", "eth_tx"},
	)
	a.prometheusCounters["countWithdrawals"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_withdrawals_count_total",
			Help: "Total count of withdrawals",
		},
		[]string{"status", "asset", "eth_tx"},
	)
	a.prometheusCounters["sumTransfers"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_transfers_sum_total",
			Help: "Total amount of transfers",
		},
		[]string{"status", "asset"},
	)
	a.prometheusCounters["countTransfers"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_transfers_count_total",
			Help: "Total count of transfers",
		},
		[]string{"status", "asset"},
	)

	a.prometheusCounters["sumLedgerMvt"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_ledger_mvt_sum_total",
			Help: "Total amount of ledger movement",
		},
		[]string{"asset", "type", "from_account_type", "from_market", "to_account_type"},
	)
	a.prometheusCounters["countLedgerMvt"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_ledger_mvt_count_total",
			Help: "Total count of ledger movement",
		},
		[]string{"asset", "type", "from_account_type", "from_market", "to_account_type"},
	)

	a.prometheusGauges["assetQuantum"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_asset_quantum",
			Help: "Quantum value for each asset",
		},
		[]string{"asset"},
	)
	a.prometheusGauges["assetDecimals"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_asset_decimals",
			Help: "Decimals for each asset",
		},
		[]string{"asset"},
	)

	a.prometheusGauges["marketBestOfferPrice"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_best_offer_price",
			Help: "Best sell price per market",
		},
		[]string{"market", "market_id"},
	)

	a.prometheusGauges["marketBestBidPrice"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_best_bid_price",
			Help: "Best buy price per market",
		},
		[]string{"market", "market_id"},
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

	a.prometheusCounters["proposals"] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vega_proposals_count_total",
			Help: "Number of proposals on the network",
		},
		[]string{"state"},
	)

	a.prometheusGauges["priceMonitoringBoundsMin"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_min_valid_price",
			Help: "Market price monitoring bound: minimal valid price",
		},
		[]string{"market", "market_id"},
	)

	a.prometheusGauges["priceMonitoringBoundsMax"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_max_valid_price",
			Help: "Market price monitoring bound: maximal valid price",
		},
		[]string{"market", "market_id"},
	)

	a.prometheusGauges["marketSettlementPrice"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_settlement_price",
			Help: "Market Settlement price per market",
		},
		[]string{"asset", "market", "market_id"},
	)

	a.prometheusGauges["partyCountTotal"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_party_count_total",
			Help: "Number of parties in the network",
		},
		[]string{},
	)

	a.prometheusGauges["erc20AssetBalance"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_erc20_pool_balance_total",
			Help: "ERC20 asset balance on the asset pool contract",
		},
		[]string{"asset"},
	)

	a.prometheusGauges["totalRewardPayout"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_total_reward_payout_nodecimal",
			Help: "Total reward payout per asset, based on the percentOfTotalReward value and amount of each event.",
		},
		[]string{"reward_type", "asset"},
	)

	for _, counter := range a.prometheusCounters {
		prometheus.MustRegister(counter)
	}

	for _, gauge := range a.prometheusGauges {
		prometheus.MustRegister(gauge)
	}

	http.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}))
	go http.ListenAndServe(listenAddr, nil)
}
