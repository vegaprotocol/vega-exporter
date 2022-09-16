package stream

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	datanode "code.vegaprotocol.io/vega/protos/data-node/api/v1"

	api "code.vegaprotocol.io/vega/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/vega/protos/vega/events/v1"
	"github.com/golang/protobuf/jsonpb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

var sumWithdrawals = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "vega_withdrawals_sum_total",
		Help: "Total amount of withdrawals",
	},
	[]string{"chain_id", "status", "asset", "eth_tx"},
)
var countWithdrawals = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "vega_withdrawals_count_total",
		Help: "Total count of withdrawals",
	},
	[]string{"chain_id", "status", "asset", "eth_tx"},
)
var sumTransfers = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "vega_transfers_sum_total",
		Help: "Total amount of transfers",
	},
	[]string{"chain_id", "status", "asset"},
)
var countTransfers = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "vega_transfers_count_total",
		Help: "Total count of transfers",
	},
	[]string{"chain_id", "status", "asset"},
)

var assetQuantum = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "vega_asset_quantum",
		Help: "Quantum value for each asset",
	},
	[]string{"asset"},
)
var assetDecimals = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "vega_asset_decimals",
		Help: "Decimals for each asset",
	},
	[]string{"asset"},
)

func connect(ctx context.Context, batchSize uint, serverAddr string) (*grpc.ClientConn, api.CoreService_ObserveEventBusClient, error) {

	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
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
	}
	if err != nil {
		return conn, stream, err
	}

	req := &api.ObserveEventBusRequest{
		//MarketId:  market,
		//PartyId:   party,
		BatchSize: int64(batchSize),
		Type:      busEventTypes,
	}

	if err := stream.Send(req); err != nil {
		return conn, stream, fmt.Errorf("error when sending initial message in stream: %w", err)
	}
	return conn, stream, nil
}

// ReadEvents reads all the events from the server
func ReadEvents(
	ctx context.Context,
	cancel context.CancelFunc,
	wg *sync.WaitGroup,
	batchSize uint,
	serverAddr string,
	reconnect bool,
) error {
	conn, stream, err := connect(ctx, batchSize, serverAddr)

	if err != nil {
		return fmt.Errorf("failed to connect to event stream: %w", err)
	}

	poll := &api.ObserveEventBusRequest{
		BatchSize: int64(batchSize),
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
					log.Printf("stream closed by server err=%v", err)
					break
				}
				if err != nil {
					log.Printf("stream closed err=%v", err)
					break
				}
				for _, e := range o.Events {
					handleEvents(ctx, conn, e)
				}
				if batchSize > 0 {
					if err := stream.SendMsg(poll); err != nil {
						log.Printf("failed to poll next event batch err=%v", err)
						return
					}
				}
			}

			if reconnect {
				// Keep waiting and retrying until we reconnect
				for {
					select {
					case <-ctx.Done():
						return
					default:
						time.Sleep(time.Second * 5)
						log.Printf("Attempting to reconnect to the node")
						conn, stream, err = connect(ctx, batchSize, serverAddr)
					}
					if err == nil {
						break
					}
				}
			} else {
				break
			}
		}
	}()
	http.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	http.ListenAndServe(":8080", nil)
	return nil
}

// Run is the main function of `stream` package
func Run(
	batchSize uint,
	serverAddr string,
	reconnect bool,
) error {
	prometheus.MustRegister(sumWithdrawals)
	prometheus.MustRegister(countWithdrawals)
	prometheus.MustRegister(sumTransfers)
	prometheus.MustRegister(countTransfers)
	prometheus.MustRegister(assetQuantum)
	prometheus.MustRegister(assetDecimals)

	flag.Parse()

	if len(serverAddr) <= 0 {
		return fmt.Errorf("error: missing grpc server address")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg := sync.WaitGroup{}
	if err := ReadEvents(ctx, cancel, &wg, batchSize, serverAddr, reconnect); err != nil {
		return fmt.Errorf("error when starting the stream: %v", err)
	}

	WaitSig(ctx, cancel)
	wg.Wait()

	return nil
}

func handleEvents(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	logToStdout(e)

	switch e.Type {
	case eventspb.BusEventType_BUS_EVENT_TYPE_WITHDRAWAL:
		handleWithdrawals(ctx, conn, e)
	case eventspb.BusEventType_BUS_EVENT_TYPE_TRANSFER:
		handleTransfers(ctx, conn, e)
	}
}

func getAssetInfo(
	ctx context.Context, conn *grpc.ClientConn, assetID string,
) (asset string, decimals uint64, quantum float64) {

	tdsClient := datanode.NewTradingDataServiceClient(conn)

	assetsReq := &datanode.AssetByIDRequest{Id: assetID}
	assetResp, err := tdsClient.AssetByID(ctx, assetsReq)
	if err != nil {
		log.Printf("unable to fetch asset err=%v", err)
		asset = assetID
		decimals = 0
		quantum = 1
	} else {
		asset = assetResp.Asset.Details.Symbol
		decimals = assetResp.Asset.Details.Decimals
		quantum, err := strconv.ParseFloat(assetResp.Asset.Details.Quantum, 64)
		if err != nil {
			log.Printf("unable to parse asset quantum err=%v", err)
		} else {
			assetQuantum.With(prometheus.Labels{"asset": asset}).Set(quantum)
		}
		assetDecimals.With(prometheus.Labels{"asset": asset}).Set(float64(decimals))
	}
	return
}

func handleWithdrawals(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	w := e.GetWithdrawal()

	asset, decimals, _ := getAssetInfo(ctx, conn, w.Asset)

	amount, err := strconv.ParseFloat(w.GetAmount(), 64)
	amount = amount / math.Pow(10, float64(decimals))
	if err != nil {
		log.Printf("unable to parse event err=%v", err)
		return
	}
	ethTx := "false"
	if w.GetTxHash() != "" {
		ethTx = "true"
	}
	labels := prometheus.Labels{
		"chain_id": e.GetChainId(),
		"status":   w.GetStatus().String(),
		"asset":    asset,
		"eth_tx":   ethTx,
	}
	sumWithdrawals.With(labels).(prometheus.ExemplarAdder).AddWithExemplar(amount, prometheus.Labels{"id": e.GetId()})
	countWithdrawals.With(labels).(prometheus.ExemplarAdder).AddWithExemplar(1, prometheus.Labels{"id": e.GetId()})
}

func handleTransfers(ctx context.Context, conn *grpc.ClientConn, e *eventspb.BusEvent) {
	t := e.GetTransfer()
	asset, _, _ := getAssetInfo(ctx, conn, t.Asset)

	amount, err := strconv.ParseFloat(t.GetAmount(), 64)
	if err != nil {
		log.Printf("unable to parse event err=%v", err)
		return
	}

	labels := prometheus.Labels{
		"chain_id": e.GetChainId(),
		"status":   t.GetStatus().String(),
		"asset":    asset,
	}

	sumTransfers.With(labels).(prometheus.ExemplarAdder).AddWithExemplar(amount, prometheus.Labels{"id": e.GetId()})
	countTransfers.With(labels).(prometheus.ExemplarAdder).AddWithExemplar(1, prometheus.Labels{"id": e.GetId()})
}

func logToStdout(e *eventspb.BusEvent) {
	m := jsonpb.Marshaler{}
	estr, err := m.MarshalToString(e)
	if err != nil {
		log.Printf("unable to marshal event err=%v", err)
	}
	fmt.Printf("{\"time\":\"%v\",%v\n", time.Now().UTC().Format(time.RFC3339Nano), estr[1:])
}

// WaitSig waits until Terminate or interrupt event is received
func WaitSig(ctx context.Context, cancel func()) {
	gracefulStop := make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)

	select {
	case sig := <-gracefulStop:
		log.Printf("Caught signal name=%v", sig)
		log.Printf("closing client connections")
		cancel()
	case <-ctx.Done():
		return
	}
}
