package app

import (
	"context"
	"crypto/tls"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"code.vegaprotocol.io/vega-exporter/token"
	datanode "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func (a *App) StartAssetPoolWatch(
	ctx context.Context,
	cancel context.CancelFunc,
	wg *sync.WaitGroup,
) error {

	var conn *grpc.ClientConn
	var err error
	if a.datanodeTls {
		config := &tls.Config{
			InsecureSkipVerify: false,
		}
		conn, err = grpc.Dial(a.datanodeAddr, grpc.WithTransportCredentials(credentials.NewTLS(config)))
		if err != nil {
			return err
		}
	} else {
		conn, err = grpc.Dial(a.datanodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return err
		}
	}

	tdsClient := datanode.NewTradingDataServiceClient(conn)

	gracefulStop := make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	wg.Add(1)
	func() {
		defer wg.Done()
		defer cancel()
	loop:
		for {
			a.getBalances(ctx, tdsClient)
			select {
			case <-ticker.C:
				continue
			case sig := <-gracefulStop:
				log.Warn().Str("signal", sig.String()).Msg("Caught signal")
				log.Warn().Msg("closing client connections")
				cancel()
				break loop
			}
		}
	}()
	return nil
}

func (a *App) getBalances(ctx context.Context, tdsClient datanode.TradingDataServiceClient) {
	assetsResp, _ := tdsClient.ListAssets(ctx, &datanode.ListAssetsRequest{})
	for _, e := range assetsResp.GetAssets().GetEdges() {
		tokenAddress := e.GetNode().GetDetails().GetErc20().GetContractAddress()
		name, balance, err := token.GetERC20Balance(tokenAddress, a.ethereumRpcAddr)

		if err == nil {
			labels := prometheus.Labels{
				"asset": name,
			}
			a.prometheusGauges["erc20AssetBalance"].With(labels).Set(balance)
		}
	}
}
