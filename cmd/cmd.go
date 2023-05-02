package cmd

import (
	"fmt"
	"os"

	"code.vegaprotocol.io/vega-exporter/app"
	"github.com/spf13/cobra"
)

var (
	runOpts struct {
		datanodeAddr      string
		datanodeTls       bool
		tendermintAddr    string
		listenAddr        string
		tendermintTls     bool
		ethereumRpcAddr   string
		assetPoolContract string
	}

	rootCmd = &cobra.Command{
		Use:   "vega-exporter",
		Short: "Prometheus Exporter for Vega Protocol",
		RunE:  run,
	}
)

// Execute is the main function of `cmd` package
// Usually called by the `main.main()`
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	flags := rootCmd.Flags()

	flags.StringVarP(&runOpts.datanodeAddr, "datanode", "d", "localhost:3007", "address of datanode grpc")
	flags.BoolVar(&runOpts.datanodeTls, "datanode-tls", false, "use Tls gprc datanode connection")
	flags.StringVarP(&runOpts.tendermintAddr, "tendermint", "t", "localhost:26657", "address of tendermint rpc")
	flags.BoolVar(&runOpts.tendermintTls, "tendermint-tls", false, "use Tls tendermint connection")
	flags.StringVarP(&runOpts.listenAddr, "listen", "l", ":8000", "address used to serve prometheus metrics")
	flags.StringVarP(&runOpts.ethereumRpcAddr, "ethereum", "e", "", "Ethereum RPC address")
	flags.StringVarP(&runOpts.assetPoolContract, "asset-pool", "p", "", "ERC20 Asset Pool smart-contract address")
}

func run(cmd *cobra.Command, args []string) error {
	return app.Run(
		runOpts.datanodeAddr,
		runOpts.tendermintAddr,
		runOpts.ethereumRpcAddr,
		runOpts.assetPoolContract,
		runOpts.listenAddr,
		runOpts.datanodeTls,
		runOpts.tendermintTls,
	)
}
