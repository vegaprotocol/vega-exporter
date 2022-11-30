package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	datanode "code.vegaprotocol.io/vega/protos/data-node/api/v1"
	proto "code.vegaprotocol.io/vega/protos/vega"
	api "code.vegaprotocol.io/vega/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/vega/protos/vega/events/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func (a *App) StartVegaObserver() error {

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
		[]string{"chain_id", "market"},
	)
	a.prometheusGauges["marketBestBidPrice"] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vega_market_best_bid_price",
			Help: "Best buy price per market",
		},
		[]string{"chain_id", "market"},
	)

	for _, counter := range a.prometheusCounters {
		prometheus.MustRegister(counter)
	}

	for _, gauge := range a.prometheusGauges {
		prometheus.MustRegister(gauge)
	}

	flag.Parse()

	if len(a.serverAddr) <= 0 {
		return fmt.Errorf("error: missing grpc server address")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg := sync.WaitGroup{}
	log.Info().Msg("Started")
	if err := a.readEvents(ctx, cancel, &wg, a.serverAddr, a.listenAddr); err != nil {
		return fmt.Errorf("error when starting the stream: %v", err)
	}

	a.waitSig(ctx, cancel)
	wg.Wait()

	return nil
}

func (a *App) connect(ctx context.Context, serverAddr string) (*grpc.ClientConn, api.CoreService_ObserveEventBusClient, error) {
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	client := api.NewCoreServiceClient(conn)
	stream, err := client.ObserveEventBus(ctx)
	if err != nil {
		conn.Close()
		return conn, stream, err
	}

	busEventTypes := []eventspb.BusEventType{
		eventspb.BusEventType_BUS_EVENT_TYPE_WITHDRAWAL,
		eventspb.BusEventType_BUS_EVENT_TYPE_TRANSFER,
		eventspb.BusEventType_BUS_EVENT_TYPE_MARKET_DATA,
		eventspb.BusEventType_BUS_EVENT_TYPE_LEDGER_MOVEMENTS,
	}
	if err != nil {
		return conn, stream, err
	}

	req := &api.ObserveEventBusRequest{Type: busEventTypes}

	if err := stream.Send(req); err != nil {
		return conn, stream, fmt.Errorf("error when sending initial message in stream: %w", err)
	}
	return conn, stream, nil
}

// ReadEvents reads all the events from the server
func (a *App) readEvents(
	ctx context.Context,
	cancel context.CancelFunc,
	wg *sync.WaitGroup,
	serverAddr string,
	listenAddr string,
) error {
	conn, stream, err := a.connect(ctx, serverAddr)

	if err != nil {
		return fmt.Errorf("failed to connect to event stream: %w", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for {
			defer conn.Close()
			defer stream.CloseSend()
			for {
				o, err := stream.Recv()
				if err == io.EOF {
					log.Error().Err(err).Msg("stream closed by server")
					break
				}
				if err != nil {
					log.Error().Err(err).Msg("stream closed")
					break
				}
				for _, e := range o.Events {
					a.handleEvents(ctx, conn, e)
				}
			}

			for {
				select {
				case <-ctx.Done():
					return
				default:
					time.Sleep(time.Second * 5)
					log.Warn().Msg("Attempting to reconnect to the node")
					conn, stream, err = a.connect(ctx, serverAddr)
				}
				if err == nil {
					break
				}
			}
		}
	}()
	http.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	http.ListenAndServe(listenAddr, nil)
	return nil
}

func (a *App) handleEvents(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	switch e.Type {
	case eventspb.BusEventType_BUS_EVENT_TYPE_WITHDRAWAL:
		a.handleWithdrawals(ctx, conn, e)
	case eventspb.BusEventType_BUS_EVENT_TYPE_TRANSFER:
		a.handleTransfers(ctx, conn, e)
	case eventspb.BusEventType_BUS_EVENT_TYPE_MARKET_DATA:
		a.handleMarketData(ctx, conn, e)
	case eventspb.BusEventType_BUS_EVENT_TYPE_LEDGER_MOVEMENTS:
		a.handleLedgerMovement(ctx, conn, e)
	}
}

func (a *App) getAssetInfo(
	ctx context.Context, conn *grpc.ClientConn, assetID string, chainID string,
) (asset string, decimals uint64, quantum float64) {

	tdsClient := datanode.NewTradingDataServiceClient(conn)
	assetsReq := &datanode.AssetByIDRequest{Id: assetID}
	assetResp, err := tdsClient.AssetByID(ctx, assetsReq)
	if err != nil {
		log.Error().Err(err).Msg("unable to fetch asset")
		asset = assetID
		decimals = 0
		quantum = 1
	} else {
		asset = assetResp.Asset.Details.Symbol
		decimals = assetResp.Asset.Details.Decimals
		quantum, err := strconv.ParseFloat(assetResp.Asset.Details.Quantum, 64)
		if err != nil {
			log.Error().Err(err).Msg("unable to parse asset quantum")
		} else {
			a.prometheusGauges["assetQuantum"].With(prometheus.Labels{"asset": asset, "chain_id": chainID}).Set(quantum)
		}
		a.prometheusGauges["assetDecimals"].With(prometheus.Labels{"asset": asset, "chain_id": chainID}).Set(float64(decimals))
	}
	return
}

func (a *App) handleWithdrawals(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	w := e.GetWithdrawal()
	chainID := e.GetChainId()
	asset, decimals, _ := a.getAssetInfo(ctx, conn, w.Asset, chainID)

	amount, err := strconv.ParseFloat(w.GetAmount(), 64)
	amount = amount / math.Pow(10, float64(decimals))
	if err != nil {
		log.Error().Err(err).Msg("unable to parse event")
		return
	}
	ethTx := "false"
	if w.GetTxHash() != "" {
		ethTx = "true"
	}
	labels := prometheus.Labels{
		"chain_id": chainID,
		"status":   w.GetStatus().String(),
		"asset":    asset,
		"eth_tx":   ethTx,
	}
	a.prometheusCounters["sumWithdrawals"].With(labels).Add(amount)
	a.prometheusCounters["countWithdrawals"].With(labels).Inc()

	log.Debug().
		Str("_id", e.Id).
		Str("block", e.Block).
		Str("tx_hash", e.TxHash).
		Str("chain_id", chainID).
		Str("type", "WITHDRAWAL").
		Str("asset", asset).
		Float64("amount", amount).
		Str("party_id", w.GetPartyId()).
		Str("ref", w.GetRef()).
		Str("erc20_rcv_addr", w.GetExt().GetErc20().GetReceiverAddress()).
		Str("status", w.GetStatus().String()).
		Int64("expiry", w.GetExpiry()).
		Int64("created_at", w.GetCreatedTimestamp()).
		Int64("withdrawn_at", w.GetWithdrawnTimestamp()).
		Send()
}

func (a *App) getMarketName(
	ctx context.Context, conn *grpc.ClientConn, marketID string,
) (name string) {

	tdsClient := datanode.NewTradingDataServiceClient(conn)
	marketReq := &datanode.MarketByIDRequest{MarketId: marketID}
	marketResp, err := tdsClient.MarketByID(ctx, marketReq)
	if err != nil {
		log.Error().Err(err).Str("market_id", marketID).Msg("unable to fetch market")
		name = marketID
	} else {
		name = marketResp.Market.TradableInstrument.Instrument.Name
	}
	return name
}

func (a *App) handleMarketData(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	md := e.GetMarketData()

	if md.MarketTradingMode == proto.Market_TRADING_MODE_CONTINUOUS {
		sellPrice, err := strconv.ParseFloat(md.BestOfferPrice, 64)
		if err != nil {
			log.Error().Err(err).Msg("unable to parse event err")
			return
		}
		buyPrice, err := strconv.ParseFloat(md.BestBidPrice, 64)
		if err != nil {
			log.Error().Err(err).Msg("unable to parse event err")
			return
		}
		labels := prometheus.Labels{
			"chain_id": e.GetChainId(),
			"market":   a.getMarketName(ctx, conn, md.Market),
		}

		a.prometheusGauges["marketBestOfferPrice"].With(labels).Set(sellPrice)
		a.prometheusGauges["marketBestBidPrice"].With(labels).Set(buyPrice)
	}
}

func (a *App) handleTransfers(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	t := e.GetTransfer()
	chainID := e.GetChainId()
	asset, decimals, _ := a.getAssetInfo(ctx, conn, t.Asset, chainID)

	amount, err := strconv.ParseFloat(t.GetAmount(), 64)
	if err != nil {
		log.Error().Err(err).Msg("unable to parse event err")
		return
	}
	amount = amount / math.Pow(10, float64(decimals))

	labels := prometheus.Labels{
		"chain_id": chainID,
		"status":   t.GetStatus().String(),
		"asset":    asset,
	}

	a.prometheusCounters["sumTransfers"].With(labels).Add(amount)
	a.prometheusCounters["countTransfers"].With(labels).Inc()

	log.Debug().
		Str("_id", e.Id).
		Str("block", e.Block).
		Str("tx_hash", e.TxHash).
		Str("type", "TRANSFER").
		Str("chain_id", chainID).
		Str("asset", asset).
		Float64("amount", amount).
		Str("from_account", t.GetFrom()).
		Str("to_account", t.GetTo()).
		Str("ref", t.GetReference()).
		Str("from_account_type", t.GetFromAccountType().String()).
		Str("to_account_type", t.GetToAccountType().String()).
		Str("status", t.GetStatus().String()).
		Str("oneoff", t.GetOneOff().String()).
		Str("recurring", t.GetRecurring().String()).
		Send()
}

func (a *App) handleLedgerMovement(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	tr := e.GetLedgerMovements()
	chainID := e.GetChainId()

	for _, lm := range tr.LedgerMovements {
		for _, entry := range lm.GetEntries() {
			fromAccount := entry.GetFromAccount()
			toAccount := entry.GetToAccount()
			amount, err := strconv.ParseFloat(entry.GetAmount(), 64)
			if err != nil {
				log.Error().Err(err).Msg("unable to parse event err")
				return
			}
			asset, decimals, _ := a.getAssetInfo(ctx, conn, fromAccount.GetAssetId(), chainID)
			amount = amount / math.Pow(10, float64(decimals))

			if err != nil {
				log.Error().Err(err).Msg("unable to parse event")
				return
			}

			ledgerEvtType := entry.GetType().String()
			fromAccountType := fromAccount.GetType().String()
			toAccountType := toAccount.GetType().String()

			market := ""
			marketID := fromAccount.GetMarketId()
			if marketID != "" {
				market = a.getMarketName(ctx, conn, marketID)
			}

			labels := prometheus.Labels{
				"chain_id":          chainID,
				"asset":             asset,
				"type":              ledgerEvtType,
				"from_account_type": fromAccountType,
				"from_market":       market,
				"to_account_type":   toAccountType,
			}

			a.prometheusCounters["sumLedgerMvt"].With(labels).Add(amount)
			a.prometheusCounters["countLedgerMvt"].With(labels).Inc()

			log.Debug().
				Str("_id", e.Id).
				Str("block", e.Block).
				Str("tx_hash", e.TxHash).
				Str("chain_id", chainID).
				Str("event_type", e.Type.String()).
				Str("type", ledgerEvtType).
				Str("from_account_type", fromAccountType).
				Str("from_account", fromAccount.GetOwner()).
				Str("from_market", market).
				Str("to_account_type", toAccountType).
				Str("to_account", toAccount.GetOwner()).
				Str("asset", asset).
				Float64("amount", amount).
				Send()
		}
	}
}

// WaitSig waits until Terminate or interrupt event is received
func (a *App) waitSig(ctx context.Context, cancel func()) {
	gracefulStop := make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)

	select {
	case sig := <-gracefulStop:
		log.Warn().Str("signal", sig.String()).Msg("Caught signal")
		log.Warn().Msg("closing client connections")
		cancel()
	case <-ctx.Done():
		return
	}
}
