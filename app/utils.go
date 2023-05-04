package app

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	datanode "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func (a *App) getMarketName(
	ctx context.Context, conn *grpc.ClientConn, marketID string,
) (name string) {

	tdsClient := datanode.NewTradingDataServiceClient(conn)
	marketReq := &datanode.GetMarketRequest{MarketId: marketID}
	marketResp, err := tdsClient.GetMarket(ctx, marketReq)
	if err != nil {
		log.Error().Err(err).Str("market_id", marketID).Msg("unable to fetch market")
		name = marketID
	} else {
		name = marketResp.GetMarket().GetTradableInstrument().GetInstrument().GetName()
	}
	return name
}

func (a *App) GetMarketPriceMonitoringBounds(
	ctx context.Context, conn *grpc.ClientConn, marketID string,
) (assetID string, minValidPrice, maxValidPrice float64, err error) {

	tdsClient := datanode.NewTradingDataServiceClient(conn)
	// Price Monitoring Bounds
	marketDataResp, err := tdsClient.GetLatestMarketData(ctx, &datanode.GetLatestMarketDataRequest{MarketId: marketID})
	if err != nil {
		log.Error().Err(err).Str("market_id", marketID).Msg("unable to get market data")
		return "", 0, 0, err
	}
	priceMonitoringBounds := marketDataResp.GetMarketData().GetPriceMonitoringBounds()

	// Settlement Data
	marketResp, err := tdsClient.GetMarket(ctx, &datanode.GetMarketRequest{MarketId: marketID})
	if err != nil {
		log.Error().Err(err).Str("market_id", marketID).Msg("unable to get market")
		return "", 0, 0, err
	}

	assetID = marketResp.GetMarket().GetTradableInstrument().GetInstrument().GetFuture().GetSettlementAsset()
	if len(priceMonitoringBounds) == 0 {
		return "", 0, 0, errors.New("no price monitoring bounds found")
	}
	maxValidPrice, err = strconv.ParseFloat(priceMonitoringBounds[0].GetMaxValidPrice(), 64)
	if err != nil {
		return "", 0, 0, err
	}
	minValidPrice, err = strconv.ParseFloat(priceMonitoringBounds[0].GetMinValidPrice(), 64)
	if err != nil {
		return "", 0, 0, err
	}
	return assetID, minValidPrice, maxValidPrice, err
}

func (a *App) getNodesNames(
	ctx context.Context,
) (nodeList map[string]string, err error) {
	var conn *grpc.ClientConn
	if a.datanodeTls {
		config := &tls.Config{
			InsecureSkipVerify: false,
		}
		conn, err = grpc.Dial(a.datanodeAddr, grpc.WithTransportCredentials(credentials.NewTLS(config)))
		if err != nil {
			return nil, err
		}
	} else {
		conn, err = grpc.Dial(a.datanodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
	}

	tdsClient := datanode.NewTradingDataServiceClient(conn)
	nodesResp, err := tdsClient.ListNodes(ctx, &datanode.ListNodesRequest{})

	if err != nil {
		log.Error().Err(err).Msg("unable to fetch nodes info")
		return nil, err
	}
	scheme := "http://"
	if a.tendermintTls {
		scheme = "https://"
	}
	validatorsResp, err := http.Get(scheme + a.tendermintAddr + "/validators")
	if err != nil {
		return nil, err
	}
	defer validatorsResp.Body.Close()
	validatorsRespBody, err := io.ReadAll(validatorsResp.Body)
	if err != nil {
		return nil, err
	}
	validators := ValidatorsResponse{}
	err = json.Unmarshal(validatorsRespBody, &validators)
	if err != nil {
		return nil, err
	}

	nodeList = make(map[string]string)
	for _, v := range validators.Result.Validators {
		for _, n := range nodesResp.GetNodes().Edges {
			nodeList[n.GetNode().GetPubKey()] = n.GetNode().GetName()
			if v.PubKey.Value == n.GetNode().GetTmPubKey() {
				nodeList[v.Address] = n.GetNode().GetName()
			}
		}
	}
	return nodeList, nil
}

func (a *App) getAssetInfo(
	ctx context.Context, conn *grpc.ClientConn, assetID string,
) (asset string, decimals uint64, quantum float64) {

	tdsClient := datanode.NewTradingDataServiceClient(conn)
	assetsReq := &datanode.GetAssetRequest{AssetId: assetID}
	assetResp, err := tdsClient.GetAsset(ctx, assetsReq)
	if err != nil {
		log.Error().Err(err).Msg("unable to fetch asset")
		asset = assetID
		decimals = 0
		quantum = 1
	} else {
		asset = assetResp.GetAsset().GetDetails().GetSymbol()
		decimals = assetResp.GetAsset().GetDetails().GetDecimals()
		quantum, err := strconv.ParseFloat(assetResp.GetAsset().GetDetails().GetQuantum(), 64)
		if err != nil {
			log.Error().Err(err).Msg("unable to parse asset quantum")
		} else {
			a.prometheusGauges["assetQuantum"].With(prometheus.Labels{"asset": asset}).Set(quantum)
		}
		a.prometheusGauges["assetDecimals"].With(prometheus.Labels{"asset": asset}).Set(float64(decimals))
	}
	return
}

func (a *App) getPartiesCount(ctx context.Context, conn *grpc.ClientConn) (partyCount int) {

	tdsClient := datanode.NewTradingDataServiceClient(conn)
	parties, _ := tdsClient.ListParties(ctx, &datanode.ListPartiesRequest{})
	return len(parties.GetParties().GetEdges())
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
