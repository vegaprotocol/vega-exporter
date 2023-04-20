package app

import (
	"context"
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

func (a *App) connect(ctx context.Context) (*grpc.ClientConn, api.CoreService_ObserveEventBusClient, error) {
	conn, err := grpc.Dial(a.datanodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
		eventspb.BusEventType_BUS_EVENT_TYPE_PROPOSAL,
		eventspb.BusEventType_BUS_EVENT_TYPE_TIME_UPDATE,
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
	case eventspb.BusEventType_BUS_EVENT_TYPE_TIME_UPDATE:
		a.handleTimeUpdate(ctx, conn, e)
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
