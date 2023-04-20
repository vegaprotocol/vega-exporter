package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"strconv"
	"sync"
	"time"

	proto "code.vegaprotocol.io/vega/protos/vega"
	api "code.vegaprotocol.io/vega/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/vega/protos/vega/events/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// ReadEvents reads all the events from the server
func (a *App) StartVegaObserver(
	ctx context.Context,
	cancel context.CancelFunc,
	wg *sync.WaitGroup,
) error {
	conn, stream, err := a.connect(ctx)

	if err != nil {
		return fmt.Errorf("failed to connect to datanode event stream: %w", err)
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
					conn, stream, err = a.connect(ctx)
				}
				if err == nil {
					break
				}
			}
		}
	}()
	return nil
}

func (a *App) connect(ctx context.Context) (
	conn *grpc.ClientConn,
	stream api.CoreService_ObserveEventBusClient,
	err error,
) {
	if a.datanodeInsecure {
		conn, err = grpc.Dial(a.datanodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, nil, err
		}
	} else {
		config := &tls.Config{
			InsecureSkipVerify: false,
		}
		conn, err = grpc.Dial(a.datanodeAddr, grpc.WithTransportCredentials(credentials.NewTLS(config)))
		if err != nil {
			return nil, nil, err
		}
	}

	client := api.NewCoreServiceClient(conn)
	stream, err = client.ObserveEventBus(ctx)
	if err != nil {
		conn.Close()
		return conn, stream, err
	}

	busEventTypes := []eventspb.BusEventType{
		eventspb.BusEventType_BUS_EVENT_TYPE_WITHDRAWAL,
		eventspb.BusEventType_BUS_EVENT_TYPE_TRANSFER,
		eventspb.BusEventType_BUS_EVENT_TYPE_MARKET_DATA,
		eventspb.BusEventType_BUS_EVENT_TYPE_LEDGER_MOVEMENTS,
		eventspb.BusEventType_BUS_EVENT_TYPE_SETTLE_MARKET,
		eventspb.BusEventType_BUS_EVENT_TYPE_TIME_UPDATE,
		eventspb.BusEventType_BUS_EVENT_TYPE_REWARD_PAYOUT_EVENT,
		eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL,
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
	case eventspb.BusEventType_BUS_EVENT_TYPE_SETTLE_MARKET:
		a.handleSettlements(ctx, conn, e)
	case eventspb.BusEventType_BUS_EVENT_TYPE_TIME_UPDATE:
		a.handleTimeUpdate(ctx, conn, e)
	case eventspb.BusEventType_BUS_EVENT_TYPE_REWARD_PAYOUT_EVENT:
		a.handleRewardPayout(ctx, conn, e)
	case eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL:
		a.handleProposal(ctx, conn, e)
	}
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
		Str("vega_tx_hash", e.TxHash).
		Str("eth_tx_hash", w.GetTxHash()).
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

func (a *App) handleMarketData(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	md := e.GetMarketData()
	chainID := e.GetChainId()

	if md.MarketTradingMode == proto.Market_TRADING_MODE_CONTINUOUS {
		sellPrice, err := strconv.ParseFloat(md.GetBestOfferPrice(), 64)
		if err != nil {
			log.Error().Err(err).Msg("unable to parse event err")
			return
		}
		buyPrice, err := strconv.ParseFloat(md.GetBestBidPrice(), 64)
		if err != nil {
			log.Error().Err(err).Msg("unable to parse event err")
			return
		}

		labels := prometheus.Labels{
			"chain_id":  chainID,
			"market":    a.getMarketName(ctx, conn, md.GetMarket()),
			"market_id": md.GetMarket(),
		}
		pmb := md.GetPriceMonitoringBounds()
		if len(pmb) > 0 {
			maxValidPrice, maxValidPriceErr := strconv.ParseFloat(pmb[0].GetMaxValidPrice(), 64)
			minValidPrice, minValidPriceErr := strconv.ParseFloat(pmb[0].GetMinValidPrice(), 64)
			if maxValidPriceErr == nil && minValidPriceErr == nil {
				a.prometheusGauges["priceMonitoringBoundsMin"].With(labels).Set(minValidPrice)
				a.prometheusGauges["priceMonitoringBoundsMax"].With(labels).Set(maxValidPrice)
			}

		}

		a.prometheusGauges["marketBestOfferPrice"].With(labels).Set(sellPrice)
		a.prometheusGauges["marketBestBidPrice"].With(labels).Set(buyPrice)
	}
}

func (a *App) handleTransfers(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	t := e.GetTransfer()
	chainID := e.GetChainId()
	asset, decimals, _ := a.getAssetInfo(ctx, conn, t.Asset, chainID)

	rawAmount, err := strconv.ParseFloat(t.GetAmount(), 64)
	if err != nil {
		log.Error().Err(err).Msg("unable to parse event err")
		return
	}
	amount := rawAmount / math.Pow(10, float64(decimals))

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

func (a *App) handleSettlements(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	s := e.GetSettleMarket()
	chainID := e.GetChainId()
	marketID := s.GetMarketId()
	market := marketID
	if marketID != "" {
		market = a.getMarketName(ctx, conn, marketID)
	}

	positionFactor := s.GetPositionFactor()
	assetID, minValidPrice, maxValidPrice, err := a.GetMarketPriceMonitoringBounds(ctx, conn, marketID)
	if err != nil {
		log.Error().Err(err).Msg("unable to parse event err")
		return
	}
	asset, decimals, _ := a.getAssetInfo(ctx, conn, assetID, chainID)

	rawPrice, err := strconv.ParseFloat(s.GetPrice(), 64)
	if err != nil {
		log.Error().Err(err).Msg("unable to parse event")
		return
	}
	price := rawPrice / math.Pow(10, float64(decimals))

	labels := prometheus.Labels{
		"chain_id":  chainID,
		"asset":     asset,
		"market_id": marketID,
		"market":    market,
	}

	a.prometheusGauges["marketSettlementPrice"].With(labels).Set(price)

	log.Debug().
		Str("_id", e.Id).
		Str("event_type", e.Type.String()).
		Str("block", e.Block).
		Str("tx_hash", e.TxHash).
		Str("chain_id", chainID).
		Str("market", market).
		Str("market_id", marketID).
		Str("position_factor", positionFactor).
		Float64("price", price).
		Str("asset", asset).
		Float64("min_valid_price", minValidPrice).
		Float64("max_valid_price", maxValidPrice).
		Send()
}

func (a *App) handleRewardPayout(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	rp := e.GetRewardPayout()
	chainID := e.GetChainId()

	labels := prometheus.Labels{
		"chain_id":    chainID,
		"asset":       rp.GetAsset(),
		"reward_type": rp.GetRewardType(),
	}

	amount, err := strconv.ParseFloat(rp.GetAmount(), 64)
	if err != nil {
		log.Error().Err(err).Msg("unable to parse event")
		return
	}

	rewardPercentage, err := strconv.ParseFloat(rp.GetPercentOfTotalReward(), 64)
	if err != nil {
		log.Error().Err(err).Msg("unable to parse event")
		return
	}

	a.prometheusGauges["totalRewardPayout"].With(labels).Set(amount / rewardPercentage * 100)
	log.Debug().
		Str("_id", e.Id).
		Str("event_type", e.Type.String()).
		Str("block", e.Block).
		Str("tx_hash", e.TxHash).
		Str("chain_id", chainID).
		Str("amount", rp.GetAmount()).
		Str("reward_percentage", rp.GetPercentOfTotalReward()).
		Str("asset", rp.GetAsset()).
		Str("reward_type", rp.GetRewardType()).
		Send()
}

func (a *App) handleTimeUpdate(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	chainID := e.GetChainId()
	partyCount := a.getPartiesCount(ctx, conn)

	labels := prometheus.Labels{
		"chain_id": chainID,
	}

	a.prometheusGauges["partyCountTotal"].With(labels).Set(float64(partyCount))
}

func (a *App) handleProposal(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	chainID := e.GetChainId()
	p := e.GetProposal()

	labels := prometheus.Labels{
		"chain_id": chainID,
		"state":    p.State.String(),
	}

	a.prometheusCounters["proposals"].With(labels).Inc()

	log.Debug().
		Str("_id", e.Id).
		Str("event_type", e.Type.String()).
		Str("block", e.Block).
		Str("tx_hash", e.TxHash).
		Str("chain_id", chainID).
		Str("proposal_id", p.GetId()).
		Str("proposal_party_id", p.GetPartyId()).
		Str("propsal_state", p.GetState().String()).
		Int64("proposal_timestamp", p.GetTimestamp()).
		Str("proposal_terms", p.GetTerms().String()).
		Str("proposal_rationale", p.GetRationale().String()).
		Send()
}
