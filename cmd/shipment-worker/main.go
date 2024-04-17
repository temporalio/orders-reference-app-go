package main

import (
	"github.com/spf13/cobra"
	"github.com/temporalio/orders-reference-app-go/app/shipment"
)

var rootCmd = &cobra.Command{
	Use:   "shipment-worker",
	Short: "Worker for Shipment system",
	RunE: func(cmd *cobra.Command, args []string) error {
		return shipment.RunWorker()
	},
}

func main() {
	cobra.CheckErr(rootCmd.Execute())
}