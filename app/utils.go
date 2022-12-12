package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	datanodeV2 "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func (a *App) getMarketName(
	ctx context.Context, conn *grpc.ClientConn, marketID string,
) (name string) {

	tdsClient := datanodeV2.NewTradingDataServiceClient(conn)
	marketReq := &datanodeV2.GetMarketRequest{MarketId: marketID}
	marketResp, err := tdsClient.GetMarket(ctx, marketReq)
	if err != nil {
		log.Error().Err(err).Str("market_id", marketID).Msg("unable to fetch market")
		name = marketID
	} else {
		name = marketResp.GetMarket().GetTradableInstrument().GetInstrument().GetName()
	}
	return name
}

func (a *App) getNodesNames(
	ctx context.Context,

) (map[string]string, error) {
	conn, err := grpc.Dial(a.datanodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	tdsClient := datanodeV2.NewTradingDataServiceClient(conn)
	nodesResp, err := tdsClient.ListNodes(ctx, &datanodeV2.ListNodesRequest{})

	if err != nil {
		log.Error().Err(err).Msg("unable to fetch nodes info")
		return nil, err
	}
	validatorsResp, err := http.Get("http://" + a.tendermintAddr + "/validators")
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
	nodeList := map[string]string{}
	for _, v := range validators.Result.Validators {
		for _, n := range nodesResp.GetNodes().Edges {
			if v.PubKey.Value == n.GetNode().GetTmPubKey() {
				nodeList[v.Address] = n.GetNode().GetName()
			}
		}
	}
	return nodeList, nil
}

func (a *App) getAssetInfo(
	ctx context.Context, conn *grpc.ClientConn, assetID string, chainID string,
) (asset string, decimals uint64, quantum float64) {

	tdsClient := datanodeV2.NewTradingDataServiceClient(conn)
	assetsReq := &datanodeV2.GetAssetRequest{AssetId: assetID}
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
			a.prometheusGauges["assetQuantum"].With(prometheus.Labels{"asset": asset, "chain_id": chainID}).Set(quantum)
		}
		a.prometheusGauges["assetDecimals"].With(prometheus.Labels{"asset": asset, "chain_id": chainID}).Set(float64(decimals))
	}
	return
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
