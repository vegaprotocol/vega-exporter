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

		tmclient.Call(ctx, "status", nil)
		statusResp := <-tmclient.ResponsesCh
		status := TmStatusResp{}
		err = json.Unmarshal(statusResp.Result, &status)
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse tendermint status response")
		}

		chainID := status.NodeInfo.Network

		query := "tm.event = 'NewBlock'"
		err = tmclient.Subscribe(ctx, query)
		if err != nil {
			log.Error().Err(err).Str("query", query).Msg("Failed to subscribe to query")
			return
		}
		tmclient.Subscribe(ctx, "tm.event = 'Tx' AND command.type = 'Node Vote'")
		tmclient.Subscribe(ctx, "tm.event = 'Tx' AND command.type = 'Chain Event'")

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

		a.nodeList, err = a.getNodesNames(ctx)
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

				switch tmEvent.Result.Data.Type {
				case "tendermint/event/NewBlock":
					err = a.handleTendermintBlockEvent(ctx, result, chainID)
					if err != nil {
						log.Error().Err(err).Msg("Failed to handle tendermint block event")
					}

				case "tendermint/event/Tx":
					fmt.Printf("NODEVOTE/CHAINEVENT: %v\n", tmEvent)
					_ = a.handleTendermintTx(ctx, tmEvent, chainID)
				}

			case <-quit:
				return
			}
		}
	}()
	return nil
}

func (a *App) handleTendermintBlockEvent(ctx context.Context, event jsonrpcTypes.RPCResponse, chainID string) (err error) {
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
	proposerLabels := prometheus.Labels{
		"chain_id": chainID,
		"address":  address,
		"name":     validatorName,
	}
	a.prometheusCounters["totalProposedBlocks"].With(proposerLabels).Inc()

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
		signersLabels := prometheus.Labels{
			"chain_id": chainID,
			"address":  address,
			"name":     signerName,
		}
		a.prometheusCounters["totalSignedBlocks"].With(signersLabels).Inc()
	}

	return err
}

func (a *App) handleTendermintTx(ctx context.Context, e TmEvent, chainID string) (err error) {
	var references map[string]string

	if len(e.Result.Events.CommandType) == 0 {
		return errors.New("no command.type found in transaction")
	}

	if len(e.Result.Events.TxSubmitter) == 0 {
		return errors.New("no tx.submitter found in transaction")
	}
	address := e.Result.Events.TxSubmitter[0]
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
	labels := prometheus.Labels{
		"chain_id": chainID,
		"address":  address,
		"name":     validatorName,
	}

	switch e.Result.Events.CommandType[0] {
	case "Node Vote":
		fmt.Println("NODEVOTE:" + references[address] + "/" + address)
		if _, ok := references[address]; !ok {
			references[address] = e.Result.Events.CommandReference[0]
			fmt.Println("NODEVOTE:OK")
			a.prometheusCounters["totalNodeVote"].With(labels).Inc()
		}

		if references[address] != e.Result.Events.CommandReference[0] {
			fmt.Println("NODEVOTE:OK")
			references[address] = e.Result.Events.CommandReference[0]
			a.prometheusCounters["totalNodeVote"].With(labels).Inc()
		}

	case "Chain Event":
		a.prometheusCounters["totalChainEvent"].With(labels).Inc()
	}
	return nil
}
