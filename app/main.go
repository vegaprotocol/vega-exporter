package app

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
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
	app.initMetrics(listenAddr)

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
