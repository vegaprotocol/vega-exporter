package app

import (
	"context"
	"encoding/json"
	"errors"
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
	proposerLabels := prometheus.Labels{
		"address": address,
		"name":    validatorName,
	}
	a.prometheusCounters["totalProposedBlocks"].With(proposerLabels).Inc()

	// Signers
	for _, s := range blockEvent.Data.Value.Block.LastCommit.Signatures {
		if s.Signature != "" {
			signerName := ""
			if val, ok := a.nodeList[s.ValidatorAddress]; ok {
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
				"address": s.ValidatorAddress,
				"name":    signerName,
			}
			a.prometheusCounters["totalSignedBlocks"].With(signersLabels).Inc()
		}
	}

	return err
}

func (a *App) handleTendermintTx(ctx context.Context, e TmEvent) (err error) {
	references := make(map[string]string)

	if len(e.Events.CommandType) == 0 {
		return errors.New("no command.type found in transaction")
	}

	if len(e.Events.TxSubmitter) == 0 {
		return errors.New("no tx.submitter found in transaction")
	}
	address := e.Events.TxSubmitter[0]
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
		"address": address,
		"name":    validatorName,
	}

	logEvent := log.Debug().
		Str("tmEventType", e.Data.Type).
		Str("address", address).
		Str("validatorName", validatorName).
		Str("commandType", e.Events.CommandType[0])

	switch e.Events.CommandType[0] {
	case "Node Vote":
		logEvent.Str("txReference", e.Events.CommandReference[0])
		logEvent.Bool("counted", false)

		if _, ok := references[address]; !ok {
			references[address] = e.Events.CommandReference[0]
			a.prometheusCounters["totalNodeVote"].With(labels).Inc()
			logEvent.Bool("counted", true)
		}

		if references[address] != e.Events.CommandReference[0] {
			references[address] = e.Events.CommandReference[0]
			a.prometheusCounters["totalNodeVote"].With(labels).Inc()
			logEvent.Bool("counted", true)
		}

		logEvent.Send()

	case "Chain Event":
		a.prometheusCounters["totalChainEvent"].With(labels).Inc()
		logEvent.Send()
	}
	return nil
}
