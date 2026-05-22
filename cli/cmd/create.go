package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// Variables to store the flag inputs
var (
	clusterName string
	configPath  string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates a new KinD cluster ready for susk8s",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Creating KinD cluster '%s'...\n", clusterName)

		// Generate kubeconfig file name
		generatedKubeconfig := fmt.Sprintf("%s.kubeconfig", clusterName)
		kindArgs := []string{"create", "cluster", "--name", clusterName, "--kubeconfig", generatedKubeconfig}

		// If the user provided a KinD config file then append it
		if configPath != "" {
			fmt.Printf("Using config file: %s\n", configPath)
			kindArgs = append(kindArgs, "--config", configPath)
		}

		// Execute the command with the dynamic slice of arguments
		execCmd := exec.Command("kind", kindArgs...)

		// Map the output so you can see it in the terminal
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		if err := execCmd.Run(); err != nil {
			fmt.Printf("Failed to create cluster: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Cluster created successfully! Credentials saved safely to ./%s\n", generatedKubeconfig)
		os.Setenv("KUBECONFIG", generatedKubeconfig)
		// THE INTERNAL HANDOFF
		fmt.Println("Automatically starting susk8s installation...")
		kubeconfig = generatedKubeconfig

		if installCmd != nil && installCmd.Run != nil {
			installCmd.Run(installCmd, []string{})
		} else {
			fmt.Println("Install command not found. Please run 'susk8s install' manually.")
		}
	},
}

func init() {
	// Define the flags and bind them to the variables
	// format: flagName, shorthand, defaultValue, description
	createCmd.Flags().StringVarP(&clusterName, "name", "n", "susk8s-cluster", "Name of the KinD cluster")
	createCmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to a custom KinD config YAML file")

	rootCmd.AddCommand(createCmd)
}
