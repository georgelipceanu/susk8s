package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Variables to hold CarbonInfo CRD flags
var (
	carbonFile      string
	carbonName      string
	carbonNamespace string
	carbonProvider  string
	carbonRegion    string
	carbonOverride  int
	carbonStatic    int
	carbonPoll      int
)

var carbonCmd = &cobra.Command{
	Use:   "carbon",
	Short: "Generates or applies a CarbonInfo configuration to the cluster",
	Run: func(cmd *cobra.Command, args []string) {
		// Setup Environment
		generatedKubeconfig := fmt.Sprintf("%s.kubeconfig", clusterName)
		absKubeconfig, _ := filepath.Abs(generatedKubeconfig)
		env := os.Environ()
		if _, err := os.Stat(absKubeconfig); err == nil {
			env = append(env, fmt.Sprintf("KUBECONFIG=%s", absKubeconfig))
		}

		var execCmd *exec.Cmd

		if carbonFile != "" {
			fmt.Printf("Applying CarbonInfo from file: %s\n", carbonFile)
			execCmd = exec.Command("kubectl", "apply", "-f", carbonFile)
		} else {
			if carbonOverride > 0 {
				fmt.Printf("WARNING: Applying MANUAL OVERRIDE (%d gCO2/kWh) to region '%s'!\n", carbonOverride, carbonRegion)
			} else {
				fmt.Printf("Tracking live carbon data for region '%s' via '%s'...\n", carbonRegion, carbonProvider)
			}

			// Build the YAML dynamically using the exact CarbonInfo spec
			yamlTemplate := `apiVersion: sustainability.susk8s/v1alpha1
kind: CarbonInfo
metadata:
  name: %s
  namespace: %s
spec:
  provider: "%s"
  region: "%s"
  pollSeconds: %d
  overrideIntensity: %d
  staticIntensity: %d
`
			yamlContent := fmt.Sprintf(yamlTemplate, carbonName, carbonNamespace, carbonProvider, carbonRegion, carbonPoll, carbonOverride, carbonStatic)

			// Tell kubectl to read from standard input
			execCmd = exec.Command("kubectl", "apply", "-f", "-")
			execCmd.Stdin = strings.NewReader(yamlContent)
		}

		execCmd.Env = env
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		if err := execCmd.Run(); err != nil {
			fmt.Printf("Failed to apply CarbonInfo: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("CarbonInfo successfully applied to the cluster!")
	},
}

func init() {
	// Re-use global clusterName variable
	carbonCmd.Flags().StringVarP(&clusterName, "cluster", "c", "susk8s-cluster", "Target KinD cluster name")

	// File approach
	carbonCmd.Flags().StringVarP(&carbonFile, "file", "f", "", "Path to a CarbonInfo YAML file")

	// Flag approach matching CRD
	carbonCmd.Flags().StringVarP(&carbonName, "carbon-name", "r", "grid-metrics", "Name of the CarbonInfo CR")
	carbonCmd.Flags().StringVarP(&carbonNamespace, "namespace", "s", "default", "Namespace for the resource")
	carbonCmd.Flags().StringVar(&carbonProvider, "provider", "electricitymaps", "Data provider (e.g., electricitymaps, mock)")
	carbonCmd.Flags().StringVar(&carbonRegion, "region", "GB", "Grid region to track")
	carbonCmd.Flags().IntVar(&carbonPoll, "poll", 60, "How often to poll the API (seconds)")

	// The Demo Flags
	carbonCmd.Flags().IntVarP(&carbonOverride, "override", "o", 0, "Set >0 to simulate a grid spike (skips API)")
	carbonCmd.Flags().IntVarP(&carbonStatic, "static", "t", 400, "Fallback intensity if API fails")

	rootCmd.AddCommand(carbonCmd)
}
