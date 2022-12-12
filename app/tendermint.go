package app

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"github.com/tendermint/tendermint/rpc/jsonrpc/client"
)

func (a *App) StartTMObserver(
	ctx context.Context,
	cancel context.CancelFunc,
	wg *sync.WaitGroup,
) error {
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		tmclient, _ := client.NewWS("tcp://"+a.tendermintAddr, "/websocket")
		err := tmclient.Start()
		if err != nil {
			log.Error().Err(err).Msg("Failed to start tendermint client")
			os.Exit(1)
		}
		defer tmclient.Stop()

		query := "tm.event = 'NewBlock'"
		err = tmclient.Subscribe(context.Background(), query)
		if err != nil {
			log.Error().Err(err).Str("query", query).Msg("Failed to subscribe to query")
			return
		}

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

		nodeList, err := a.getNodesNames(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to fetch node list")
		}

		for {
			select {
			case result := <-tmclient.ResponsesCh:
				blockEvent := BlockEvent{}
				err = json.Unmarshal(result.Result, &blockEvent)
				if err != nil {
					log.Error().Err(err).Msg("Failed to unmarshal response")
				}

				// Proposer
				address := blockEvent.Data.Value.Block.Header.ProposerAddress
				validatorName := ""
				if val, ok := nodeList[address]; ok {
					validatorName = val
				} else {
					// refresh validator list
					nodeList, err = a.getNodesNames(ctx)
					if err != nil {
						log.Error().Err(err).Msg("Failed to fetch node list")
					}
					if val, ok := nodeList[address]; ok {
						validatorName = val
					}
				}
				a.prometheusCounters["totalProposedBlocks"].With(prometheus.Labels{"address": address, "name": validatorName}).Inc()

				// Signers
				for _, s := range blockEvent.Data.Value.Block.LastCommit.Signatures {
					signerName := ""
					if val, ok := nodeList[address]; ok {
						signerName = val
					} else {
						// refresh validator list
						nodeList, err = a.getNodesNames(ctx)
						if err != nil {
							log.Error().Err(err).Msg("Failed to fetch node list")
						}
						if val, ok := nodeList[s.ValidatorAddress]; ok {
							signerName = val
						}
					}
					a.prometheusCounters["totalSignedBlocks"].With(prometheus.Labels{"address": address, "name": signerName}).Inc()
				}

			case <-quit:
				return
			}
		}
	}()
	return nil
}
