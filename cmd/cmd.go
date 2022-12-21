package cmd

import (
	"fmt"
	"os"

	"code.vegaprotocol.io/vega-exporter/app"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "vega-exporter",
	Short: "Prometheus Exporter for Vega Protocol",
}

var (
	streamOpts struct {
		datanodeAddr       string
		datanodeInsecure   bool
		tendermintAddr     string
		listenAddr         string
		tendermintInsecure bool
		datanodeV1         bool
	}

	streamCmd = &cobra.Command{
		Use:   "run",
		Short: "Stream events from vega node",
		RunE:  runStream,
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
	rootCmd.AddCommand(streamCmd)
	streamCmd.Flags().StringVarP(&streamOpts.datanodeAddr, "datanode", "d", "localhost:3007", "address of datanode grpc")
	streamCmd.Flags().BoolVar(&streamOpts.datanodeInsecure, "datanode-insecure", false, "use insecure gprc datanode connection")
	streamCmd.Flags().BoolVar(&streamOpts.datanodeV1, "datanode-v1", false, "use datanode v1 api instead of v2")
	streamCmd.Flags().StringVarP(&streamOpts.tendermintAddr, "tendermint", "t", "localhost:26657", "address of tendermint rpc")
	streamCmd.Flags().BoolVar(&streamOpts.tendermintInsecure, "tendermint-insecure", false, "use insecure tendermint connection")
	streamCmd.Flags().StringVarP(&streamOpts.listenAddr, "listen", "l", ":8000", "address used to serve prometheus metrics")
	streamCmd.MarkFlagRequired("address")
}

func runStream(cmd *cobra.Command, args []string) error {
	return app.Run(
		streamOpts.datanodeAddr,
		streamOpts.tendermintAddr,
		streamOpts.listenAddr,
		streamOpts.datanodeInsecure,
		streamOpts.tendermintInsecure,
		streamOpts.datanodeV1,
	)
}
