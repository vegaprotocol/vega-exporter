package cmd

import (
	"fmt"
	"os"

	"code.vegaprotocol.io/vega-exporter/vega"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "vegaexporter",
	Short: "Prometheus Exporter for Vega Protocol",
}

var (
	streamOpts struct {
		batchSize  uint
		party      string
		market     string
		serverAddr string
		logFormat  string
		reconnect  bool
		types      []string
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
	streamCmd.Flags().UintVarP(&streamOpts.batchSize, "batch-size", "b", 0, "size of the event stream batch of events")
	streamCmd.Flags().StringVarP(&streamOpts.serverAddr, "address", "a", "", "address of the grpc server")
	streamCmd.Flags().StringVar(&streamOpts.logFormat, "log-format", "raw", "output stream data in specified format. Allowed values: raw (default), text, json")
	streamCmd.Flags().BoolVarP(&streamOpts.reconnect, "reconnect", "r", false, "if connection dies, attempt to reconnect")
	streamCmd.Flags().StringSliceVarP(&streamOpts.types, "type", "t", nil, "one or more event types to subscribe to (default=ALL)")
	streamCmd.MarkFlagRequired("address")
}

func runStream(cmd *cobra.Command, args []string) error {
	return vega.Run(streamOpts.batchSize,
		streamOpts.serverAddr,
		streamOpts.reconnect,
	)
}
