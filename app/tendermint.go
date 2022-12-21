package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"github.com/tendermint/tendermint/rpc/jsonrpc/client"
	jsonrpcTypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"
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
		tmclient.Subscribe(context.Background(), "tm.event = 'Tx' AND command.type = 'Node Vote'")
		tmclient.Subscribe(context.Background(), "tm.event = 'Tx' AND command.type = 'Chain Event'")
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

		if a.datanodeV1 {
			a.nodeList, err = a.getNodesNamesV1(ctx)
		} else {
			a.nodeList, err = a.getNodesNamesV2(ctx)
		}

		if err != nil {
			log.Error().Err(err).Msg("Failed to fetch node list")
		}

		for {
			select {
			case result := <-tmclient.ResponsesCh:
				tmEvent := TmEvent{}
				err = json.Unmarshal(result.Result, &tmEvent)
				if err != nil {
					log.Error().Err(err).Msg("Failed to parse event")
				}

				switch tmEvent.Data.Type {
				case "tendermint/event/NewBlock":
					err = a.handleTendermintBlockEvent(ctx, result)
					if err != nil {
						log.Error().Err(err).Msg("Failed to handle tendermint block event")
					}

				case "tendermint/event/Tx":
					_ = a.handleTendermintTx(ctx, tmEvent)
				}

			case <-quit:
				return
			}
		}
	}()
	return nil
}

func (a *App) handleTendermintBlockEvent(ctx context.Context, event jsonrpcTypes.RPCResponse) (err error) {
	blockEvent := BlockEvent{}
	err = json.Unmarshal(event.Result, &blockEvent)
	if err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal response")
	}

	// Proposer
	address := blockEvent.Data.Value.Block.Header.ProposerAddress
	validatorName := ""
	if val, ok := a.nodeList[address]; ok {
		validatorName = val
	} else {
		// refresh validator list
		a.nodeList, err = a.getNodesNames(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to fetch node list")
		}
		if val, ok := a.nodeList[address]; ok {
			validatorName = val
		}
	}
	a.prometheusCounters["totalProposedBlocks"].With(prometheus.Labels{"address": address, "name": validatorName}).Inc()

	// Signers
	for _, s := range blockEvent.Data.Value.Block.LastCommit.Signatures {
		signerName := ""
		if val, ok := a.nodeList[address]; ok {
			signerName = val
		} else {
			// refresh validator list
			a.nodeList, err = a.getNodesNames(ctx)
			if err != nil {
				log.Error().Err(err).Msg("Failed to fetch node list")
			}
			if val, ok := a.nodeList[s.ValidatorAddress]; ok {
				signerName = val
			}
		}
		a.prometheusCounters["totalSignedBlocks"].With(prometheus.Labels{"address": address, "name": signerName}).Inc()
	}

	return err
}

func (a *App) handleTendermintTx(ctx context.Context, e TmEvent) (err error) {
	if len(e.Event.CommandType) == 0 {
		return errors.New("no command.type found in transaction")
	}

	if len(e.Event.Submitter) == 0 {
		return errors.New("no tx.submitter found in transaction")
	}
	address := e.Event.Submitter[0]
	validatorName := ""
	if val, ok := a.nodeList[address]; ok {
		validatorName = val
	} else {
		// refresh validator list
		a.nodeList, err = a.getNodesNames(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to fetch node list")
		}
		if val, ok := a.nodeList[address]; ok {
			validatorName = val
		}
	}

	switch e.Event.CommandType[0] {
	case "Node Vote":
		a.prometheusCounters["totalNodeVote"].With(prometheus.Labels{"address": address, "name": validatorName}).Inc()
	case "Chain Event":
		fmt.Println("chain event")
		a.prometheusCounters["totalChainEvent"].With(prometheus.Labels{"address": address, "name": validatorName}).Inc()
	}
	return nil
}
