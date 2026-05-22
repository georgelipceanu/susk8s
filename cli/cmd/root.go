package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "susk8s",
	Short: "susk8s is a CLI for managing carbon-aware Kubernetes clusters",
	Long:  `A command line interface to automate the provisioning and installation of the susk8s carbon-aware operator, scheduler, and observability stack.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
