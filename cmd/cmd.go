package cmd

import (
	"fmt"
	"os"

	"code.vegaprotocol.io/vega-exporter/vega"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "vega-exporter",
	Short: "Prometheus Exporter for Vega Protocol",
}

var (
	streamOpts struct {
		serverAddr string
		listenAddr string
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
	streamCmd.Flags().StringVarP(&streamOpts.serverAddr, "address", "a", "", "address of the grpc server")
	streamCmd.Flags().StringVarP(&streamOpts.listenAddr, "listen", "l", ":8000", "address used to serve prometheus metrics")
	streamCmd.MarkFlagRequired("address")
}

func runStream(cmd *cobra.Command, args []string) error {
	return vega.Run(streamOpts.serverAddr, streamOpts.listenAddr)
}
