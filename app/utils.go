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

	datanode "code.vegaprotocol.io/vega/protos/data-node/api/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

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
	tdsClient := datanode.NewTradingDataServiceClient(conn)
	nodesResp, err := tdsClient.GetNodes(ctx, &datanode.GetNodesRequest{})

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
		for _, n := range nodesResp.GetNodes() {
			nodeList[n.GetPubKey()] = n.GetName()
			if v.PubKey.Value == n.GetTmPubKey() {
				nodeList[v.Address] = n.GetName()
			}
		}
	}
	return nodeList, nil
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

func (a *App) getPartiesCount(ctx context.Context, conn *grpc.ClientConn) (partyCount int) {

	tdsClient := datanode.NewTradingDataServiceClient(conn)
	parties, _ := tdsClient.Parties(ctx, &datanode.PartiesRequest{})
	return len(parties.GetParties())
}
